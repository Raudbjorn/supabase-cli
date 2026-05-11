package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

// setupTestSandbox creates a test sandbox with a mock process-compose server and
// writes state.json so loadSandboxState can find it. Returns the filesystem,
// project ID, and a cleanup function.
func setupTestSandbox(t *testing.T, handler http.Handler) (afero.Fs, string, func()) {
	t.Helper()

	srv := httptest.NewServer(handler)
	port := testPort(t, srv)

	// Use a real temp directory for state file since SandboxContext constructs paths
	// based on home dir and TempDir. We'll use afero.OsFs with temp dir override.
	fs := afero.NewMemMapFs()
	projectId := "test-project"

	// Create sandbox context to find state file path
	sandboxCtx, err := NewSandboxContext(projectId)
	if err != nil {
		t.Fatalf("NewSandboxContext() error = %v", err)
	}

	// Write state file with the mock server port
	state := SandboxState{
		PID: os.Getpid(), // current process so it "looks" running
		Ports: AllocatedPorts{
			ProcessCompose: port,
			Postgres:       5432,
			GoTrue:         9999,
			PostgREST:      3000,
			API:            8000,
		},
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	stateDir := filepath.Dir(sandboxCtx.StateFilePath())
	if err := fs.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := afero.WriteFile(fs, sandboxCtx.StateFilePath(), data, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return fs, projectId, func() { srv.Close() }
}

func TestRestartService(t *testing.T) {
	// Mock: restart returns 200, then process state shows Running+Ready
	callCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/process/restart/gotrue" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/process/gotrue" && r.Method == http.MethodGet:
			callCount++
			state := processState{Name: "gotrue", Status: pcStatusRunning, Health: pcHealthReady, HasHealthProbe: true}
			json.NewEncoder(w).Encode(state)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	fs, projectId, cleanup := setupTestSandbox(t, handler)
	defer cleanup()

	var buf bytes.Buffer
	ctx := context.Background()
	if err := RestartService(ctx, fs, projectId, "auth", &buf); err != nil {
		t.Fatalf("RestartService() error = %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("Restarting auth")) {
		t.Errorf("expected 'Restarting auth' in output, got %q", output)
	}
	if !bytes.Contains([]byte(output), []byte("restarted successfully")) {
		t.Errorf("expected 'restarted successfully' in output, got %q", output)
	}
}

func TestRestartService_InvalidName(t *testing.T) {
	var buf bytes.Buffer
	fs := afero.NewMemMapFs()
	err := RestartService(context.Background(), fs, "proj", "invalid", &buf)
	if err == nil {
		t.Fatal("expected error for invalid service name")
	}
}

func TestStopService(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/process/stop/postgrest" && r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	fs, projectId, cleanup := setupTestSandbox(t, handler)
	defer cleanup()

	var buf bytes.Buffer
	if err := StopService(context.Background(), fs, projectId, "rest", &buf); err != nil {
		t.Fatalf("StopService() error = %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("Stopping rest")) {
		t.Errorf("expected 'Stopping rest' in output, got %q", output)
	}
}

func TestStartService(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/process/start/proxy" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/process/proxy" && r.Method == http.MethodGet:
			state := processState{Name: "proxy", Status: pcStatusRunning, Health: pcHealthReady, HasHealthProbe: true}
			json.NewEncoder(w).Encode(state)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	fs, projectId, cleanup := setupTestSandbox(t, handler)
	defer cleanup()

	var buf bytes.Buffer
	if err := StartService(context.Background(), fs, projectId, "api", &buf); err != nil {
		t.Fatalf("StartService() error = %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("started successfully")) {
		t.Errorf("expected 'started successfully' in output, got %q", output)
	}
}

func TestListServices(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/processes" && r.Method == http.MethodGet {
			states := processesState{
				States: []processState{
					{Name: "postgres", Status: pcStatusRunning, Health: pcHealthReady, HasHealthProbe: true},
					{Name: "postgres-init", Status: pcStatusCompleted},
					{Name: "gotrue-migrate", Status: pcStatusCompleted},
					{Name: "gotrue", Status: pcStatusRunning, Health: pcHealthReady, HasHealthProbe: true},
					{Name: "postgrest", Status: pcStatusRunning, Health: pcHealthReady, HasHealthProbe: true},
					{Name: "proxy", Status: pcStatusRunning, Health: pcHealthReady, HasHealthProbe: true},
				},
			}
			json.NewEncoder(w).Encode(states)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	fs, projectId, cleanup := setupTestSandbox(t, handler)
	defer cleanup()

	services, err := ListServices(context.Background(), fs, projectId)
	if err != nil {
		t.Fatalf("ListServices() error = %v", err)
	}

	// Should only include 4 user-facing services (not postgres-init, gotrue-migrate)
	if len(services) != 4 {
		t.Fatalf("got %d services, want 4", len(services))
	}

	// Verify all are ready
	for _, svc := range services {
		if !svc.IsReady {
			t.Errorf("service %q is not ready", svc.Name)
		}
	}
}

func TestListServices_NoState(t *testing.T) {
	fs := afero.NewMemMapFs()
	_, err := ListServices(context.Background(), fs, "no-such-project")
	if err == nil {
		t.Fatal("expected error when state file is missing")
	}
}

func TestMapProcessStatesToServiceInfo(t *testing.T) {
	t.Run("nil states", func(t *testing.T) {
		result := mapProcessStatesToServiceInfo(nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("filters one-shot processes", func(t *testing.T) {
		states := &processesState{
			States: []processState{
				{Name: "postgres", Status: pcStatusRunning},
				{Name: "postgres-init", Status: pcStatusCompleted},
				{Name: "gotrue-migrate", Status: pcStatusCompleted},
			},
		}
		result := mapProcessStatesToServiceInfo(states)
		if len(result) != 1 {
			t.Fatalf("got %d services, want 1 (only postgres mapped to db)", len(result))
		}
		if result[0].Name != "db" {
			t.Errorf("expected user-facing name 'db', got %q", result[0].Name)
		}
	})
}

func TestReloadConfig(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/project" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	})

	fs, projectId, cleanup := setupTestSandbox(t, handler)
	defer cleanup()

	var buf bytes.Buffer
	if err := ReloadConfig(context.Background(), fs, projectId, &buf); err != nil {
		t.Fatalf("ReloadConfig() error = %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("reloaded")) {
		t.Errorf("expected 'reloaded' in output, got %q", output)
	}
}
