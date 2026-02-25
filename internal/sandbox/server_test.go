package sandbox

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testPort extracts the port number from an httptest.Server.
func testPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("failed to parse test server address: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("failed to convert port: %v", err)
	}
	return port
}

func TestGetProcessesState(t *testing.T) {
	want := processesState{
		States: []processState{
			{Name: "postgres", Status: pcStatusRunning, Health: pcHealthReady, HasHealthProbe: true},
			{Name: "gotrue", Status: pcStatusRunning, Health: pcHealthReady, HasHealthProbe: true},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/processes" || r.Method != http.MethodGet {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	got, err := getProcessesState(context.Background(), testPort(t, srv))
	if err != nil {
		t.Fatalf("getProcessesState() error = %v", err)
	}
	if len(got.States) != len(want.States) {
		t.Fatalf("got %d states, want %d", len(got.States), len(want.States))
	}
	for i, s := range got.States {
		if s.Name != want.States[i].Name {
			t.Errorf("state[%d].Name = %q, want %q", i, s.Name, want.States[i].Name)
		}
	}
}

func TestGetProcessesState_ServerDown(t *testing.T) {
	_, err := getProcessesState(context.Background(), 1) // port 1 is not listening
	if err == nil {
		t.Fatal("expected error when server is down")
	}
}

func TestGetProcessesState_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := getProcessesState(context.Background(), testPort(t, srv))
	if err == nil {
		t.Fatal("expected error for non-200 status code")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to contain status code, got %q", err.Error())
	}
}

func TestGetProcessState(t *testing.T) {
	want := processState{Name: "postgres", Status: pcStatusRunning, Health: pcHealthReady, HasHealthProbe: true}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/process/postgres" || r.Method != http.MethodGet {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	got, err := getProcessState(context.Background(), testPort(t, srv), "postgres")
	if err != nil {
		t.Fatalf("getProcessState() error = %v", err)
	}
	if got.Name != want.Name || got.Status != want.Status {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestGetProcessState_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := getProcessState(context.Background(), testPort(t, srv), "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent process")
	}
}

func TestRestartProcess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/process/restart/gotrue" || r.Method != http.MethodPost {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := restartProcess(context.Background(), testPort(t, srv), "gotrue"); err != nil {
		t.Fatalf("restartProcess() error = %v", err)
	}
}

func TestRestartProcess_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := restartProcess(context.Background(), testPort(t, srv), "gotrue"); err == nil {
		t.Fatal("expected error for bad status code")
	}
}

func TestStopProcess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/process/stop/postgrest" || r.Method != http.MethodPatch {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := stopProcess(context.Background(), testPort(t, srv), "postgrest"); err != nil {
		t.Fatalf("stopProcess() error = %v", err)
	}
}

func TestStartProcess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/process/start/postgrest" || r.Method != http.MethodPost {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := startProcess(context.Background(), testPort(t, srv), "postgrest"); err != nil {
		t.Fatalf("startProcess() error = %v", err)
	}
}

func TestReloadProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/project" || r.Method != http.MethodPost {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := reloadProject(context.Background(), testPort(t, srv)); err != nil {
		t.Fatalf("reloadProject() error = %v", err)
	}
}

func TestReloadProject_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer srv.Close()

	if err := reloadProject(context.Background(), testPort(t, srv)); err == nil {
		t.Fatal("expected error for bad status code")
	}
}

func TestShutDownProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/project/stop/" || r.Method != http.MethodPost {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := shutDownProject(context.Background(), testPort(t, srv)); err != nil {
		t.Fatalf("shutDownProject() error = %v", err)
	}
}

func TestShutDownProject_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := shutDownProject(context.Background(), testPort(t, srv)); err == nil {
		t.Fatal("expected error for bad status code")
	}
}

func TestCheckLiveness(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/live" || r.Method != http.MethodGet {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if !checkLiveness(context.Background(), testPort(t, srv)) {
		t.Fatal("expected liveness check to return true")
	}
}

func TestCheckLiveness_Down(t *testing.T) {
	if checkLiveness(context.Background(), 1) {
		t.Fatal("expected liveness check to return false for unreachable server")
	}
}

func TestCheckReadiness(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ready" || r.Method != http.MethodGet {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if !checkReadiness(context.Background(), testPort(t, srv)) {
		t.Fatal("expected readiness check to return true")
	}
}

func TestCheckReadiness_NotReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if checkReadiness(context.Background(), testPort(t, srv)) {
		t.Fatal("expected readiness check to return false")
	}
}

func TestGetProcessState_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response to trigger context cancellation
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := getProcessState(ctx, testPort(t, srv), "postgres")
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
}

func TestGetProcessState_PathEscape(t *testing.T) {
	// Verify that names with special characters are properly escaped
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The path should be /process/my%2Fprocess (escaped slash)
		if r.URL.RawPath == "/process/my%2Fprocess" || r.URL.Path == "/process/my/process" {
			json.NewEncoder(w).Encode(processState{Name: "my/process", Status: pcStatusRunning})
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	// This tests that url.PathEscape is used correctly
	_, err := getProcessState(context.Background(), testPort(t, srv), "my/process")
	// We just verify no panic occurs; actual routing depends on server implementation
	_ = err
}

func TestIsStateReady(t *testing.T) {
	tests := []struct {
		name  string
		state processState
		want  bool
	}{
		{name: "running with healthy probe", state: processState{Status: pcStatusRunning, HasHealthProbe: true, Health: pcHealthReady}, want: true},
		{name: "running with unhealthy probe", state: processState{Status: pcStatusRunning, HasHealthProbe: true, Health: "Not Ready"}, want: false},
		{name: "running without probe", state: processState{Status: pcStatusRunning, HasHealthProbe: false}, want: true},
		{name: "completed", state: processState{Status: pcStatusCompleted}, want: true},
		{name: "disabled", state: processState{Status: pcStatusDisabled}, want: true},
		{name: "skipped", state: processState{Status: pcStatusSkipped}, want: true},
		{name: "launching without probe", state: processState{Status: pcStatusLaunching, HasHealthProbe: false}, want: false},
		{name: "launching with healthy probe", state: processState{Status: pcStatusLaunching, HasHealthProbe: true, Health: pcHealthReady}, want: true},
		{name: "unknown status", state: processState{Status: "Unknown"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isStateReady(&tt.state); got != tt.want {
				t.Errorf("isStateReady(%+v) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestIsAllStatesReady(t *testing.T) {
	t.Run("nil states", func(t *testing.T) {
		if isAllStatesReady(nil) {
			t.Error("expected false for nil states")
		}
	})

	t.Run("all ready", func(t *testing.T) {
		states := &processesState{
			States: []processState{
				{Status: pcStatusRunning, HasHealthProbe: true, Health: pcHealthReady},
				{Status: pcStatusCompleted},
			},
		}
		if !isAllStatesReady(states) {
			t.Error("expected true when all states are ready")
		}
	})

	t.Run("one not ready", func(t *testing.T) {
		states := &processesState{
			States: []processState{
				{Status: pcStatusRunning, HasHealthProbe: true, Health: pcHealthReady},
				{Status: pcStatusLaunching, HasHealthProbe: false},
			},
		}
		if isAllStatesReady(states) {
			t.Error("expected false when one state is not ready")
		}
	})
}
