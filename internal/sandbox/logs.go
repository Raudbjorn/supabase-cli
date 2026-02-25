package sandbox

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/afero"
	"github.com/supabase/cli/internal/utils"
)

// StreamLogs streams live logs from sandbox services to the provided writer.
// If service is empty, streams logs from all services.
// If follow is false, fetches historical logs and returns.
func StreamLogs(ctx context.Context, fsys afero.Fs, projectId string, service string, follow bool, tail int, w io.Writer) error {
	// Resolve service name alias if provided
	processName := ""
	if service != "" {
		name, ok := KnownServices[service]
		if !ok {
			return fmt.Errorf("unknown service %q. Valid services: %v", service, RestartableServices())
		}
		processName = name
	}

	sandboxCtx, err := NewSandboxContext(projectId)
	if err != nil {
		return fmt.Errorf("failed to create sandbox context: %w", err)
	}

	state, err := sandboxCtx.LoadState(fsys)
	if err != nil {
		return fmt.Errorf("sandbox is not running (no state file): %w", err)
	}

	if follow {
		return streamProcessLogs(ctx, state.Ports.ProcessCompose, processName, w)
	}

	// Non-follow mode: fetch historical logs
	return fetchAndPrintLogs(state.Ports.ProcessCompose, processName, tail, w)
}

// fetchAndPrintLogs fetches historical logs and prints them.
func fetchAndPrintLogs(serverPort int, processName string, tail int, w io.Writer) error {
	if tail <= 0 {
		tail = 100
	}

	if processName != "" {
		// Fetch logs for a single service
		lines, err := fetchProcessLogs(serverPort, processName, tail)
		if err != nil {
			return err
		}
		for _, line := range lines {
			fmt.Fprintf(w, "%s %s\n", utils.Aqua(fmt.Sprintf("[%s]", processName)), line)
		}
		return nil
	}

	// Fetch logs for all known services
	states, err := getProcessesState(serverPort)
	if err != nil {
		return fmt.Errorf("failed to get process states: %w", err)
	}

	for _, state := range states.States {
		// Skip one-shot init processes
		if strings.HasSuffix(state.Name, "-init") || strings.HasSuffix(state.Name, "-migrate") {
			continue
		}
		if state.Status == pcStatusDisabled || state.Status == pcStatusSkipped {
			continue
		}

		lines, err := fetchProcessLogs(serverPort, state.Name, tail)
		if err != nil {
			fmt.Fprintf(w, "%s failed to fetch logs: %v\n", utils.Aqua(fmt.Sprintf("[%s]", state.Name)), err)
			continue
		}
		for _, line := range lines {
			fmt.Fprintf(w, "%s %s\n", utils.Aqua(fmt.Sprintf("[%s]", state.Name)), line)
		}
	}

	return nil
}
