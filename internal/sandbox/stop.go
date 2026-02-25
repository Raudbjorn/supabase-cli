package sandbox

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/spf13/afero"
)

const (
	// ShutdownGracePeriod is the time to wait for processes to shut down gracefully.
	ShutdownGracePeriod = 2 * time.Second
)

// Stop stops all sandbox services and cleans up resources.
// Uses process-compose HTTP API for graceful shutdown with proper dependency ordering.
// Falls back to killing the server PID if HTTP API is unavailable.
// If backup is false, also removes the postgres data directory.
func Stop(ctx context.Context, fsys afero.Fs, projectId string, backup bool, w io.Writer) error {
	sandboxCtx, err := NewSandboxContext(projectId)
	if err != nil {
		return fmt.Errorf("failed to create sandbox context: %w", err)
	}

	// Load state to find server PID and process-compose port
	state, err := sandboxCtx.LoadState(fsys)
	if err != nil {
		return fmt.Errorf("sandbox is not running (no state file): %w", err)
	}

	fmt.Fprintln(w, "Stopping services...")

	// Try graceful shutdown via REST API first
	stopped := false
	if state.Ports.ProcessCompose > 0 {
		if err := shutDownProject(ctx, state.Ports.ProcessCompose); err == nil {
			stopped = true
			// Give processes time to shut down gracefully
			time.Sleep(ShutdownGracePeriod)
		}
	}

	// Fallback: kill server PID (process-compose will clean up children)
	if !stopped && state.PID > 0 {
		fmt.Fprintf(w, "HTTP API unavailable, terminating server (PID %d)...\n", state.PID)
		if err := terminateProcess(state.PID); err != nil {
			fmt.Fprintf(w, "Warning: failed to terminate server: %v\n", err)
		}
		time.Sleep(ShutdownGracePeriod)
	}

	// Clean up all sandbox files (state, yaml, logs)
	if err := fsys.RemoveAll(sandboxCtx.ConfigDir); err != nil {
		fmt.Fprintf(w, "Warning: failed to cleanup sandbox files: %v\n", err)
	}

	// If no backup requested, also remove the postgres data directory
	if !backup {
		pgDataDir := sandboxCtx.PgDataDir()
		fmt.Fprintf(w, "Removing postgres data directory %s...\n", pgDataDir)
		if err := fsys.RemoveAll(pgDataDir); err != nil {
			fmt.Fprintf(w, "Warning: failed to remove postgres data dir: %v\n", err)
		}
	}

	return nil
}

// terminateProcess sends a termination signal to a process.
// On Unix, it sends SIGTERM for graceful shutdown.
// On Windows, it uses taskkill for graceful shutdown.
func terminateProcess(pid int) error {
	if pid <= 0 {
		return nil
	}

	if runtime.GOOS == "windows" {
		// On Windows, use taskkill for graceful shutdown
		cmd := exec.Command("taskkill", "/PID", strconv.Itoa(pid))
		return cmd.Run()
	}

	// On Unix, send SIGTERM for graceful shutdown
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGTERM)
}

// StopService stops a single sandbox service by name without tearing down the stack.
func StopService(ctx context.Context, fsys afero.Fs, projectId string, service string, w io.Writer) error {
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

	fmt.Fprintf(w, "Stopping %s...\n", service)

	if err := stopProcess(state.Ports.ProcessCompose, processName); err != nil {
		return err
	}

	fmt.Fprintf(w, "Service %s stopped. Use 'supabase restart %s' to start it again.\n", service, service)
	return nil
}

// Cleanup removes all sandbox-related resources for a project.
// This includes the config directory and postgres data directory.
func Cleanup(ctx context.Context, fsys afero.Fs, projectId string, w io.Writer) error {
	sandboxCtx, err := NewSandboxContext(projectId)
	if err != nil {
		return fmt.Errorf("failed to create sandbox context: %w", err)
	}

	// First stop everything (with backup=true since Cleanup handles pgdata removal separately)
	if err := Stop(ctx, fsys, projectId, true, w); err != nil {
		fmt.Fprintf(w, "Warning: stop failed: %v\n", err)
	}

	// Remove postgres data directory
	pgDataDir := sandboxCtx.PgDataDir()
	fmt.Fprintf(w, "Removing postgres data directory %s...\n", pgDataDir)
	if err := fsys.RemoveAll(pgDataDir); err != nil {
		fmt.Fprintf(w, "Warning: failed to remove postgres data dir: %v\n", err)
	}

	// Remove config directory
	fmt.Fprintf(w, "Removing config directory %s...\n", sandboxCtx.ConfigDir)
	if err := fsys.RemoveAll(sandboxCtx.ConfigDir); err != nil {
		fmt.Fprintf(w, "Warning: failed to remove config dir: %v\n", err)
	}

	// Remove postgres version file
	if err := fsys.Remove(SandboxPostgresVersionPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(w, "Warning: failed to remove postgres version file: %v\n", err)
	}

	fmt.Fprintln(w, "Sandbox cleanup complete.")
	return nil
}
