package sandbox

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// PollingInterval is how often to check service status.
	PollingInterval = 2 * time.Second
	// InitialStartupDelay gives the server time to start before polling.
	InitialStartupDelay = 1 * time.Second
	// pcAPITimeout is the timeout for HTTP requests to the process-compose REST API.
	pcAPITimeout = 5 * time.Second
)

// pcClient is a shared HTTP client for process-compose API calls.
// Reused across calls to benefit from connection pooling.
var pcClient = &http.Client{Timeout: pcAPITimeout}

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
	pcStatusRunning   = "Running"
	pcStatusLaunched  = "Launched"
	pcStatusCompleted = "Completed"
	pcStatusDisabled  = "Disabled"
	pcStatusSkipped   = "Skipped"
	pcStatusLaunching = "Launching"
	pcHealthReady     = "Ready"
)

// waitForCondition polls until the check function returns true, the parent context
// is cancelled, or the timeout is reached.
func waitForCondition(ctx context.Context, timeout time.Duration, timeoutMsg string, check func(ctx context.Context) bool) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(PollingInterval)
	defer ticker.Stop()

	// Initial delay to let the server start (cancellable, unlike time.Sleep)
	if InitialStartupDelay > 0 {
		timer := time.NewTimer(InitialStartupDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return ctx.Err()
			}
			return fmt.Errorf("%s: %w", timeoutMsg, ctx.Err())
		case <-timer.C:
		}
	}

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				return ctx.Err()
			}
			return fmt.Errorf("%s: %w", timeoutMsg, ctx.Err())
		case <-ticker.C:
			if check(ctx) {
				return nil
			}
		}
	}
}

// getProcessesState fetches all process states from the process-compose REST API.
func getProcessesState(ctx context.Context, serverPort int) (*processesState, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/processes", serverPort), nil)
	if err != nil {
		return nil, err
	}
	resp, err := pcClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("process-compose API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var states processesState
	if err := json.NewDecoder(resp.Body).Decode(&states); err != nil {
		return nil, fmt.Errorf("failed to decode process states: %w", err)
	}
	return &states, nil
}

// shutDownProject sends a shutdown request to the process-compose REST API.
func shutDownProject(ctx context.Context, serverPort int) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/project/stop/", serverPort), nil)
	if err != nil {
		return err
	}
	resp, err := pcClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to stop project - unexpected status code: %s", resp.Status)
	}
	return nil
}

// getProcessState fetches a single process state from the process-compose REST API.
func getProcessState(ctx context.Context, serverPort int, name string) (*processState, error) {
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/process/%s", serverPort, url.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := pcClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get process %q state: %s", name, resp.Status)
	}

	var state processState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, fmt.Errorf("failed to decode process state: %w", err)
	}
	return &state, nil
}

// restartProcess sends a restart request for a single process.
func restartProcess(ctx context.Context, serverPort int, name string) error {
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/process/restart/%s", serverPort, url.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, nil)
	if err != nil {
		return err
	}
	resp, err := pcClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to restart process %q: %s", name, resp.Status)
	}
	return nil
}

// stopProcess sends a stop request for a single process.
func stopProcess(ctx context.Context, serverPort int, name string) error {
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/process/stop/%s", serverPort, url.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, apiURL, nil)
	if err != nil {
		return err
	}
	resp, err := pcClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to stop process %q: %s", name, resp.Status)
	}
	return nil
}

// startProcess sends a start request for a single process.
func startProcess(ctx context.Context, serverPort int, name string) error {
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/process/start/%s", serverPort, url.PathEscape(name))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, nil)
	if err != nil {
		return err
	}
	resp, err := pcClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to start process %q: %s", name, resp.Status)
	}
	return nil
}

// reloadProject sends a hot-reload request to process-compose.
func reloadProject(ctx context.Context, serverPort int) error {
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/project", serverPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, nil)
	if err != nil {
		return err
	}
	resp, err := pcClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to reload project: %s", resp.Status)
	}
	return nil
}

// checkLiveness checks if the process-compose server is alive.
func checkLiveness(ctx context.Context, serverPort int) bool {
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/live", serverPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return false
	}
	resp, err := pcClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// checkReadiness checks if all process-compose managed processes are ready.
func checkReadiness(ctx context.Context, serverPort int) bool {
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/ready", serverPort)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return false
	}
	resp, err := pcClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// WaitForServerReady polls the process-compose server until all services are healthy.
func WaitForServerReady(ctx context.Context, serverPort int, timeout time.Duration) error {
	return waitForCondition(ctx, timeout, "timeout waiting for services to become healthy", func(ctx context.Context) bool {
		states, err := getProcessesState(ctx, serverPort)
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
func WaitForPostgresReady(ctx context.Context, serverPort int, timeout time.Duration) error {
	return waitForCondition(ctx, timeout, "timeout waiting for postgres to become healthy", func(ctx context.Context) bool {
		states, err := getProcessesState(ctx, serverPort)
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

// processLog represents a single log message from the process-compose WebSocket API.
type processLog struct {
	ProcessName string `json:"process_name"`
	Message     string `json:"message"`
	Timestamp   string `json:"timestamp"`
	IsStderr    bool   `json:"is_stderr"`
}

// streamProcessLogs connects to the process-compose WebSocket log endpoint and
// writes log messages to the provided writer until the context is cancelled.
// GET /process/logs/ws (WebSocket upgrade)
func streamProcessLogs(ctx context.Context, serverPort int, processName string, w io.Writer) error {
	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/process/logs/ws", serverPort)
	if processName != "" {
		wsURL += "?name=" + url.QueryEscape(processName)
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to log stream: %w", err)
	}
	defer conn.Close()

	// Close connection when context is cancelled
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil // Context cancelled, normal shutdown
			}
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				return nil
			}
			return fmt.Errorf("log stream error: %w", err)
		}

		var logMsg processLog
		if err := json.Unmarshal(message, &logMsg); err != nil {
			// Not JSON, write raw message
			fmt.Fprintln(w, string(message))
			continue
		}

		fmt.Fprintln(w, formatLogMessage(&logMsg))
	}
}

// formatLogMessage formats a log message for terminal output.
func formatLogMessage(log *processLog) string {
	return fmt.Sprintf("[%s] %s", log.ProcessName, strings.TrimRight(log.Message, "\n"))
}

// fetchProcessLogs retrieves historical logs for a process via the REST API.
// GET /process/logs/:name/:endOffset/:limit
func fetchProcessLogs(serverPort int, name string, limit int) ([]string, error) {
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/process/logs/%s/0/%d", serverPort, url.PathEscape(name), limit)

	resp, err := pcClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch logs for %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch logs for %s (status %s): %s", name, resp.Status, strings.TrimSpace(string(body)))
	}

	var lines []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// getProjectState fetches the full project state including dependency graph info.
// GET /project/state
func getProjectState(serverPort int) (map[string]interface{}, error) {
	apiURL := fmt.Sprintf("http://127.0.0.1:%d/project/state", serverPort)

	resp, err := pcClient.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var state map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, fmt.Errorf("failed to decode project state: %w", err)
	}
	return state, nil
}
