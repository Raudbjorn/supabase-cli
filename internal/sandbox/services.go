package sandbox

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/afero"
)

const (
	// ServiceActionTimeout is the maximum time to wait for a service action (restart/start/stop) to complete.
	ServiceActionTimeout = 60 * time.Second
)

// ServiceInfo holds the status of a single user-facing service.
type ServiceInfo struct {
	Name      string // user-facing name (e.g. "db", "auth")
	PCName    string // process-compose name (e.g. "postgres", "gotrue")
	Status    string // process-compose status
	Health    string // health probe state
	IsReady   bool
}

// loadSandboxState is a helper that creates a sandbox context, loads state, and
// returns the process-compose server port. Used by all service operations.
func loadSandboxState(fsys afero.Fs, projectId string) (int, error) {
	sandboxCtx, err := NewSandboxContext(projectId)
	if err != nil {
		return 0, fmt.Errorf("failed to create sandbox context: %w", err)
	}

	state, err := sandboxCtx.LoadState(fsys)
	if err != nil {
		return 0, fmt.Errorf("sandbox is not running (no state file): %w", err)
	}

	if state.Ports.ProcessCompose == 0 {
		return 0, fmt.Errorf("sandbox state missing process-compose port")
	}

	return state.Ports.ProcessCompose, nil
}

// RestartService restarts a single sandbox service by user-facing name.
func RestartService(ctx context.Context, fsys afero.Fs, projectId string, serviceName string, w io.Writer) error {
	pcName, err := ResolveProcessName(serviceName)
	if err != nil {
		return err
	}

	serverPort, err := loadSandboxState(fsys, projectId)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Restarting %s...\n", serviceName)

	if err := restartProcess(ctx, serverPort, pcName); err != nil {
		return fmt.Errorf("failed to restart %s: %w", serviceName, err)
	}

	// Wait for the service to become healthy again
	if err := waitForCondition(ctx, ServiceActionTimeout,
		fmt.Sprintf("timeout waiting for %s to become healthy after restart", serviceName),
		func(ctx context.Context) bool {
			state, err := getProcessState(ctx, serverPort, pcName)
			if err != nil {
				return false
			}
			return isStateReady(state)
		},
	); err != nil {
		return err
	}

	fmt.Fprintf(w, "%s restarted successfully.\n", serviceName)
	return nil
}

// StopService stops a single sandbox service by user-facing name.
func StopService(ctx context.Context, fsys afero.Fs, projectId string, serviceName string, w io.Writer) error {
	pcName, err := ResolveProcessName(serviceName)
	if err != nil {
		return err
	}

	serverPort, err := loadSandboxState(fsys, projectId)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Stopping %s...\n", serviceName)

	if err := stopProcess(ctx, serverPort, pcName); err != nil {
		return fmt.Errorf("failed to stop %s: %w", serviceName, err)
	}

	fmt.Fprintf(w, "%s stopped.\n", serviceName)
	return nil
}

// StartService starts a single sandbox service by user-facing name.
func StartService(ctx context.Context, fsys afero.Fs, projectId string, serviceName string, w io.Writer) error {
	pcName, err := ResolveProcessName(serviceName)
	if err != nil {
		return err
	}

	serverPort, err := loadSandboxState(fsys, projectId)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Starting %s...\n", serviceName)

	if err := startProcess(ctx, serverPort, pcName); err != nil {
		return fmt.Errorf("failed to start %s: %w", serviceName, err)
	}

	// Wait for the service to become healthy
	if err := waitForCondition(ctx, ServiceActionTimeout,
		fmt.Sprintf("timeout waiting for %s to become healthy", serviceName),
		func(ctx context.Context) bool {
			state, err := getProcessState(ctx, serverPort, pcName)
			if err != nil {
				return false
			}
			return isStateReady(state)
		},
	); err != nil {
		return err
	}

	fmt.Fprintf(w, "%s started successfully.\n", serviceName)
	return nil
}

// ListServices returns the status of all user-facing services.
func ListServices(ctx context.Context, fsys afero.Fs, projectId string) ([]ServiceInfo, error) {
	serverPort, err := loadSandboxState(fsys, projectId)
	if err != nil {
		return nil, err
	}

	states, err := getProcessesState(ctx, serverPort)
	if err != nil {
		return nil, fmt.Errorf("failed to get process states: %w", err)
	}

	return mapProcessStatesToServiceInfo(states), nil
}

// mapProcessStatesToServiceInfo converts process-compose states to user-facing ServiceInfo.
// Only includes processes that have a user-facing mapping (excludes one-shot processes).
func mapProcessStatesToServiceInfo(states *processesState) []ServiceInfo {
	if states == nil {
		return nil
	}

	// Build reverse lookup: PC name -> user-facing name
	reverseLookup := make(map[string]string, len(serviceMapping))
	for userFacing, pcName := range serviceMapping {
		reverseLookup[pcName] = userFacing
	}

	var services []ServiceInfo
	for _, state := range states.States {
		userFacing, ok := reverseLookup[state.Name]
		if !ok {
			continue // skip one-shot processes
		}
		services = append(services, ServiceInfo{
			Name:    userFacing,
			PCName:  state.Name,
			Status:  state.Status,
			Health:  state.Health,
			IsReady: isStateReady(&state),
		})
	}
	return services
}

// ReloadConfig regenerates the process-compose.yaml from config and hot-reloads.
func ReloadConfig(ctx context.Context, fsys afero.Fs, projectId string, w io.Writer) error {
	serverPort, err := loadSandboxState(fsys, projectId)
	if err != nil {
		return err
	}

	fmt.Fprintln(w, "Reloading configuration...")

	if err := reloadProject(ctx, serverPort); err != nil {
		return fmt.Errorf("failed to reload project: %w", err)
	}

	fmt.Fprintln(w, "Configuration reloaded.")
	return nil
}
