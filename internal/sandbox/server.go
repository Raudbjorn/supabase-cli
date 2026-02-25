package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	// PollingInterval is how often to check service status.
	PollingInterval = 2 * time.Second
	// InitialStartupDelay gives the server time to start before polling.
	InitialStartupDelay = 1 * time.Second
	// HTTPClientTimeout is the timeout for HTTP requests to the process-compose API.
	HTTPClientTimeout = 5 * time.Second
)

// processState mirrors the process-compose API response for a single process.
type processState struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	Health         string `json:"is_ready"`
	HasHealthProbe bool   `json:"has_ready_probe"`
}

// processesState mirrors the process-compose API response for all processes.
type processesState struct {
	States []processState `json:"data"`
}

// Process status constants matching process-compose API values.
const (
	pcStatusRunning    = "Running"
	pcStatusLaunched   = "Launched"
	pcStatusCompleted  = "Completed"
	pcStatusDisabled   = "Disabled"
	pcStatusSkipped    = "Skipped"
	pcStatusLaunching  = "Launching"
	pcHealthReady      = "Ready"
)

// waitForCondition polls until the check function returns true or timeout is reached.
func waitForCondition(timeout time.Duration, timeoutMsg string, check func() bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(PollingInterval)
	defer ticker.Stop()

	// Initial delay to let the server start
	time.Sleep(InitialStartupDelay)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf(timeoutMsg)
		case <-ticker.C:
			if check() {
				return nil
			}
		}
	}
}

// getProcessesState fetches all process states from the process-compose HTTP API.
func getProcessesState(serverPort int) (*processesState, error) {
	client := &http.Client{Timeout: HTTPClientTimeout}
	url := fmt.Sprintf("http://127.0.0.1:%d/processes", serverPort)

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var states processesState
	if err := json.NewDecoder(resp.Body).Decode(&states); err != nil {
		return nil, fmt.Errorf("failed to decode process states: %w", err)
	}
	return &states, nil
}

// shutDownProject sends a shutdown request to the process-compose HTTP API.
func shutDownProject(serverPort int) error {
	client := &http.Client{Timeout: HTTPClientTimeout}
	url := fmt.Sprintf("http://127.0.0.1:%d/project/stop/", serverPort)

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to stop project - unexpected status code: %s", resp.Status)
	}
	return nil
}

// WaitForServerReady polls the process-compose server until all services are healthy.
func WaitForServerReady(serverPort int, timeout time.Duration) error {
	return waitForCondition(timeout, "timeout waiting for services to become healthy", func() bool {
		states, err := getProcessesState(serverPort)
		if err != nil {
			return false
		}
		return isAllStatesReady(states)
	})
}

// isAllStatesReady checks if all services are ready based on the API response.
func isAllStatesReady(states *processesState) bool {
	if states == nil {
		return false
	}
	for _, state := range states.States {
		if !isStateReady(&state) {
			return false
		}
	}
	return true
}

// isStateReady checks if a single service is ready.
func isStateReady(state *processState) bool {
	switch state.Status {
	case pcStatusCompleted, pcStatusDisabled, pcStatusSkipped:
		return true
	case pcStatusRunning, pcStatusLaunched:
		if state.HasHealthProbe {
			return state.Health == pcHealthReady
		}
		return true
	case pcStatusLaunching:
		// Daemon processes might stay in Launching but be healthy
		if state.HasHealthProbe && state.Health == pcHealthReady {
			return true
		}
		return false
	default:
		return false
	}
}

// WaitForPostgresReady polls the process-compose server until postgres and postgres-init are ready.
// This allows migrations to run before other services are fully healthy.
func WaitForPostgresReady(serverPort int, timeout time.Duration) error {
	return waitForCondition(timeout, "timeout waiting for postgres to become healthy", func() bool {
		states, err := getProcessesState(serverPort)
		if err != nil {
			return false
		}
		return isPostgresReady(states)
	})
}

// isPostgresReady checks if postgres and postgres-init are ready.
func isPostgresReady(states *processesState) bool {
	if states == nil {
		return false
	}

	postgresReady := false
	postgresInitReady := false

	for _, state := range states.States {
		switch state.Name {
		case "postgres":
			postgresReady = isStateReady(&state)
		case "postgres-init":
			postgresInitReady = isStateReady(&state)
		}
	}

	return postgresReady && postgresInitReady
}
