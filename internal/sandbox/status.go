package sandbox

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/afero"
	"github.com/supabase/cli/internal/utils"
)

// ServiceStatus represents the health status of a service.
type ServiceStatus struct {
	Name    string
	Status  string
	Port    int
	Healthy bool
}

// Status checks the health of all sandbox services using the process-compose API.
// This provides DAG-aware readiness by leveraging process-compose's built-in health
// probes and dependency graph, rather than doing independent health checks.
func Status(ctx context.Context, projectId string, fsys afero.Fs) ([]ServiceStatus, error) {
	sandboxCtx, err := NewSandboxContext(projectId)
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox context: %w", err)
	}

	state, err := sandboxCtx.LoadState(fsys)
	if err != nil {
		return nil, fmt.Errorf("sandbox is not running (no state file): %w", err)
	}

	// Use process-compose API to get all process states at once.
	// This is superior to individual HTTP health checks because process-compose
	// already monitors health probes and understands the dependency graph.
	pcStates, err := getProcessesState(ctx, state.Ports.ProcessCompose)
	if err != nil {
		return nil, fmt.Errorf("failed to query process-compose: %w", err)
	}

	// Port mapping for user-facing display
	portMap := map[string]int{
		"postgres":  state.Ports.Postgres,
		"gotrue":    state.Ports.GoTrue,
		"postgrest": state.Ports.PostgREST,
		"proxy":     state.Ports.API,
	}

	// User-facing name mapping
	displayName := map[string]string{
		"proxy": "api",
	}

	var statuses []ServiceStatus
	for _, ps := range pcStates.States {
		// Skip one-shot init/migrate processes from user-facing status
		if strings.HasSuffix(ps.Name, "-init") || strings.HasSuffix(ps.Name, "-migrate") {
			continue
		}

		port := portMap[ps.Name]
		name := ps.Name
		if dn, ok := displayName[ps.Name]; ok {
			name = dn
		}

		healthy := isStateReady(&ps)
		status := ps.Status
		if healthy {
			status = "running"
		} else if ps.Status == pcStatusLaunching {
			status = "starting"
		} else if ps.Status == pcStatusCompleted {
			status = "completed"
		}

		// Append health probe info if available
		if ps.HasHealthProbe && !healthy && ps.Status == pcStatusRunning {
			status = "waiting for health check"
		}

		statuses = append(statuses, ServiceStatus{
			Name:    name,
			Status:  status,
			Port:    port,
			Healthy: healthy,
		})
	}

	return statuses, nil
}

// ShowStatus checks sandbox status and prints it using the same format as Docker mode.
func ShowStatus(ctx context.Context, projectId string, fsys afero.Fs) error {
	sandboxCtx, err := NewSandboxContext(projectId)
	if err != nil {
		return fmt.Errorf("failed to create sandbox context: %w", err)
	}

	// Load state
	state, err := sandboxCtx.LoadState(fsys)
	if err != nil {
		return fmt.Errorf("sandbox is not running: %w", err)
	}

	// Set ports from state so PrettyPrintSandbox can use them
	sandboxCtx.Ports = &state.Ports

	// Get status of all services to check for unhealthy ones
	statuses, err := Status(ctx, projectId, fsys)
	if err != nil {
		return err
	}

	// Print per-service health checklist
	var unhealthy []string
	for _, s := range statuses {
		if s.Healthy {
			if s.Port > 0 {
				fmt.Fprintf(os.Stderr, "  %s %s (port %d)\n", utils.Green("✓"), s.Name, s.Port)
			} else {
				fmt.Fprintf(os.Stderr, "  %s %s\n", utils.Green("✓"), s.Name)
			}
		} else {
			fmt.Fprintf(os.Stderr, "  %s %s: %s\n", utils.Red("✗"), s.Name, s.Status)
			unhealthy = append(unhealthy, s.Name)
		}
	}
	fmt.Fprintln(os.Stderr)

	if len(unhealthy) > 0 {
		fmt.Fprintf(os.Stderr, "%s Unhealthy services: %v\n\n", utils.Yellow("WARNING:"), unhealthy)
	}

	// Print status message matching Docker mode
	fmt.Fprintf(os.Stderr, "%s local development setup is running.\n\n", utils.Aqua("supabase"))

	// Print tables using the same format as start --sandbox
	PrettyPrintSandbox(os.Stdout, sandboxCtx)
	return nil
}
