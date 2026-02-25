package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/spf13/afero"
	"github.com/supabase/cli/internal/utils"
)

// logMessage mirrors the process-compose WebSocket log message format.
type logMessage struct {
	ProcessName string `json:"process_name"`
	Message     string `json:"message"`
}

// logStreamDialer is the WebSocket dialer used for log streaming.
// Package-level var allows test override.
var logStreamDialer = websocket.DefaultDialer

// StreamLogs streams live logs from sandbox services to the provided writer.
// If service is empty, streams logs from all services.
// If follow is false, fetches historical logs and returns.
func StreamLogs(ctx context.Context, fsys afero.Fs, projectId string, service string, follow bool, tail int, w io.Writer) error {
	// Resolve service name alias if provided
	processName := ""
	if service != "" {
		name, err := ResolveProcessName(service)
		if err != nil {
			return err
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
	states, err := getProcessesState(context.Background(), serverPort)
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

// buildLogWSURL constructs the WebSocket URL for log streaming.
func buildLogWSURL(serverPort int, pcName string, follow bool) string {
	params := url.Values{}
	if pcName != "" {
		params.Set("name", pcName)
	}
	params.Set("follow", fmt.Sprintf("%t", follow))

	return fmt.Sprintf("ws://127.0.0.1:%d/process/logs/ws?%s", serverPort, params.Encode())
}

// streamFromWebSocket dials a WebSocket and writes formatted log messages to w.
// It respects context cancellation for clean shutdown.
func streamFromWebSocket(ctx context.Context, wsURL string, w io.Writer) error {
	conn, _, err := logStreamDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to log stream: %w", err)
	}

	// Close the connection when context is cancelled
	go func() {
		<-ctx.Done()
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		conn.Close()
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			// Normal closure or context cancellation
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("log stream error: %w", err)
		}

		var msg logMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			// Fall back to raw output if not JSON
			fmt.Fprintln(w, string(message))
			continue
		}

		fmt.Fprintf(w, "%s | %s\n", msg.ProcessName, msg.Message)
	}
}
