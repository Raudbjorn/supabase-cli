package sandbox

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/afero"
)

// KnownServices maps user-facing service names to process-compose process names.
var KnownServices = map[string]string{
	"postgres":  "postgres",
	"db":        "postgres",
	"gotrue":    "gotrue",
	"auth":      "gotrue",
	"postgrest": "postgrest",
	"rest":      "postgrest",
	"proxy":     "proxy",
	"api":       "proxy",
}

// RestartableServices returns the list of user-facing service names.
func RestartableServices() []string {
	return []string{"postgres", "auth", "rest", "api"}
}

// RestartService restarts a single sandbox service by name.
func RestartService(ctx context.Context, fsys afero.Fs, projectId string, service string, w io.Writer) error {
	processName, ok := KnownServices[service]
	if !ok {
		return fmt.Errorf("unknown service %q. Valid services: %v", service, RestartableServices())
	}

	sandboxCtx, err := NewSandboxContext(projectId)
	if err != nil {
		return fmt.Errorf("failed to create sandbox context: %w", err)
	}

	state, err := sandboxCtx.LoadState(fsys)
	if err != nil {
		return fmt.Errorf("sandbox is not running (no state file): %w", err)
	}

	fmt.Fprintf(w, "Restarting %s...\n", service)

	if err := restartProcess(state.Ports.ProcessCompose, processName); err != nil {
		return err
	}

	// Wait for the restarted service to become healthy
	fmt.Fprintf(w, "Waiting for %s to become healthy...\n", service)
	if err := waitForProcessReady(state.Ports.ProcessCompose, processName, DefaultServiceTimeout); err != nil {
		return err
	}

	fmt.Fprintf(w, "Service %s restarted successfully.\n", service)
	return nil
}

// waitForProcessReady polls until a specific process is ready.
func waitForProcessReady(serverPort int, processName string, timeout time.Duration) error {
	return waitForCondition(timeout, fmt.Sprintf("timeout waiting for %s to become healthy", processName), func() bool {
		states, err := getProcessesState(serverPort)
		if err != nil {
			return false
		}
		for _, state := range states.States {
			if state.Name == processName {
				return isStateReady(&state)
			}
		}
		return false
	})
}
