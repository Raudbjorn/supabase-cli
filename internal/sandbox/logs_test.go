package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestStreamFromWebSocket_Messages(t *testing.T) {
	messages := []logMessage{
		{ProcessName: "gotrue", Message: "server started on port 9999"},
		{ProcessName: "postgres", Message: "database system is ready"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade error: %v", err)
			return
		}
		defer conn.Close()

		for _, msg := range messages {
			data, _ := json.Marshal(msg)
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}

		// Send close frame
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/process/logs/ws"

	var buf bytes.Buffer
	err := streamFromWebSocket(context.Background(), wsURL, &buf)
	if err != nil {
		t.Fatalf("streamFromWebSocket() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "gotrue | server started on port 9999") {
		t.Errorf("expected gotrue log line in output, got %q", output)
	}
	if !strings.Contains(output, "postgres | database system is ready") {
		t.Errorf("expected postgres log line in output, got %q", output)
	}
}

func TestStreamFromWebSocket_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Send messages slowly so context cancellation has time to fire
		for i := 0; i < 100; i++ {
			msg := logMessage{ProcessName: "test", Message: "line"}
			data, _ := json.Marshal(msg)
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/process/logs/ws"

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	var buf bytes.Buffer
	err := streamFromWebSocket(ctx, wsURL, &buf)
	// Should return nil (clean exit) on context cancellation
	if err != nil {
		t.Fatalf("expected nil error on context cancellation, got %v", err)
	}
}

func TestStreamFromWebSocket_ServerClose(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Send one message then close immediately
		msg := logMessage{ProcessName: "test", Message: "final line"}
		data, _ := json.Marshal(msg)
		conn.WriteMessage(websocket.TextMessage, data)
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/process/logs/ws"

	var buf bytes.Buffer
	err := streamFromWebSocket(context.Background(), wsURL, &buf)
	if err != nil {
		t.Fatalf("expected nil error on normal closure, got %v", err)
	}

	if !strings.Contains(buf.String(), "test | final line") {
		t.Errorf("expected log line in output, got %q", buf.String())
	}
}

func TestStreamFromWebSocket_NonJSONFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		conn.WriteMessage(websocket.TextMessage, []byte("plain text log line"))
		conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/process/logs/ws"

	var buf bytes.Buffer
	err := streamFromWebSocket(context.Background(), wsURL, &buf)
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	if !strings.Contains(buf.String(), "plain text log line") {
		t.Errorf("expected raw text in output, got %q", buf.String())
	}
}

func TestBuildLogWSURL(t *testing.T) {
	tests := []struct {
		name       string
		serverPort int
		pcName     string
		follow     bool
		wantHost   string
		wantParams map[string]string
	}{
		{
			name:       "all processes follow",
			serverPort: 8080,
			follow:     true,
			wantHost:   "ws://127.0.0.1:8080/process/logs/ws",
			wantParams: map[string]string{"follow": "true"},
		},
		{
			name:       "specific process no follow",
			serverPort: 9090,
			pcName:     "gotrue",
			follow:     false,
			wantHost:   "ws://127.0.0.1:9090/process/logs/ws",
			wantParams: map[string]string{"follow": "false", "name": "gotrue"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildLogWSURL(tt.serverPort, tt.pcName, tt.follow)
			if !strings.HasPrefix(got, tt.wantHost) {
				t.Errorf("URL prefix = %q, want %q", got, tt.wantHost)
			}
			for key, val := range tt.wantParams {
				if !strings.Contains(got, key+"="+val) {
					t.Errorf("URL %q missing param %s=%s", got, key, val)
				}
			}
		})
	}
}

func TestStreamFromWebSocket_ConnectionRefused(t *testing.T) {
	err := streamFromWebSocket(context.Background(), "ws://127.0.0.1:1/logs", &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when connection is refused")
	}
}
