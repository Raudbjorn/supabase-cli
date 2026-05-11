package sandbox

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-errors/errors"
	"github.com/spf13/afero"
	"github.com/supabase/cli/internal/utils"
	"github.com/supabase/cli/pkg/config"
	"github.com/ulikunitz/xz"
)

const (
	// Binary versions (GitHub release downloads)
	GotrueVersion         = "2.186.0" // Local build for darwin-arm64
	PostgrestVersion      = "14.4"
	PostgresVersion       = "17.6.1.081-cli"
	ProcessComposeVersion = "1.90.0"

	// SpinnerTickInterval is how often the download spinner animation updates.
	SpinnerTickInterval = 80 * time.Millisecond
)

// GetGotruePath returns the path to the gotrue binary.
// Binaries are cached with versioning: ~/.supabase/bin/gotrue/<version>/gotrue
func GetGotruePath(binDir string) string {
	name := "gotrue"
	if runtime.GOOS == "windows" {
		name = "gotrue.exe"
	}
	return filepath.Join(binDir, "gotrue", GotrueVersion, name)
}

// GetPostgrestPath returns the path to the postgrest binary.
// Binaries are cached with versioning: ~/.supabase/bin/postgrest/<version>/postgrest
func GetPostgrestPath(binDir string) string {
	name := "postgrest"
	if runtime.GOOS == "windows" {
		name = "postgrest.exe"
	}
	return filepath.Join(binDir, "postgrest", PostgrestVersion, name)
}

// GetProcessComposePath returns the path to the process-compose binary.
// Binaries are cached with versioning: ~/.supabase/bin/process-compose/<version>/process-compose
func GetProcessComposePath(binDir string) string {
	name := "process-compose"
	if runtime.GOOS == "windows" {
		name = "process-compose.exe"
	}
	return filepath.Join(binDir, "process-compose", ProcessComposeVersion, name)
}

// GetPostgresDir returns the postgres installation directory.
// Unlike single-binary tools, PostgreSQL is a full directory with bin/, lib/, share/.
// Cached at: ~/.supabase/bin/postgres/<version>/
func GetPostgresDir(binDir, version string) string {
	return filepath.Join(binDir, "postgres", version)
}

// GetPostgresBinPath returns the path to a specific postgres binary.
func GetPostgresBinPath(binDir, version, binary string) string {
	name := binary
	if runtime.GOOS == "windows" {
		name = binary + ".exe"
	}
	return filepath.Join(GetPostgresDir(binDir, version), "bin", name)
}

// GetPostgresLibDir returns the path to postgres shared libraries.
func GetPostgresLibDir(binDir, version string) string {
	return filepath.Join(GetPostgresDir(binDir, version), "lib")
}

// GetServicesDir returns the base directory for Docker-extracted services.
// Located at: ~/.supabase/bin/services/
func GetServicesDir(binDir string) string {
	return filepath.Join(binDir, "services")
}

// GetServicePath returns the path to a specific extracted service directory.
// e.g., ~/.supabase/bin/services/realtime/
func GetServicePath(binDir, service string) string {
	return filepath.Join(GetServicesDir(binDir), service)
}

// dockerService describes a service to extract from a Docker image.
type dockerService struct {
	Name      string // display name and target directory
	Image     string // Docker image reference
	SrcPath   string // path inside the container to copy from
	NeedsNode bool   // whether this service requires Node.js on host
}

// getDockerServices returns the list of services to extract from Docker images.
// Image tags are read from config.Images (parsed from pkg/config/templates/Dockerfile)
// so they stay in sync with the CLI's Docker Compose setup automatically.
func getDockerServices() []dockerService {
	return []dockerService{
		{Name: "realtime", Image: config.Images.Realtime, SrcPath: "/app/."},
		{Name: "logflare", Image: config.Images.Logflare, SrcPath: "/opt/app/rel/logflare/."},
		{Name: "storage", Image: config.Images.Storage, SrcPath: "/app/.", NeedsNode: true},
		{Name: "pgmeta", Image: config.Images.Pgmeta, SrcPath: "/usr/src/app/.", NeedsNode: true},
		{Name: "studio", Image: config.Images.Studio, SrcPath: "/app/.", NeedsNode: true},
	}
}

// InstallDockerServices extracts services from Docker images if not already cached.
// Uses `docker create` + `docker cp` + `docker rm` to avoid running containers.
func InstallDockerServices(ctx context.Context, binDir string) error {
	// Docker images contain Linux-amd64 binaries; extraction is only
	// meaningful on Linux hosts.
	if runtime.GOOS != "linux" {
		return fmt.Errorf("Docker service extraction is only supported on Linux (current: %s)", runtime.GOOS)
	}

	servicesDir := GetServicesDir(binDir)
	if err := os.MkdirAll(servicesDir, 0755); err != nil {
		return fmt.Errorf("failed to create services directory: %w", err)
	}

	// Check which services need extraction
	var missing []dockerService
	for _, svc := range getDockerServices() {
		svcDir := GetServicePath(binDir, svc.Name)
		if !dirExists(svcDir) {
			missing = append(missing, svc)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	// Verify docker is available
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker is required to extract service binaries: %w", err)
	}

	// Check if Node.js is needed and available
	needsNode := false
	for _, svc := range missing {
		if svc.NeedsNode {
			needsNode = true
			break
		}
	}
	if needsNode {
		if _, err := exec.LookPath("node"); err != nil {
			return fmt.Errorf("node is required for storage/pgmeta/studio services: %w", err)
		}
	}

	// Build status display for Docker extraction
	statuses := make([]*BinaryStatus, len(missing))
	for i, svc := range missing {
		statuses[i] = &BinaryStatus{Name: svc.Name, Downloading: true}
	}

	// Print initial status
	for _, s := range statuses {
		icon := utils.Aqua(spinnerFrames[0])
		fmt.Fprintf(os.Stderr, " %s %s Extracting from Docker\n", icon, s.Name)
	}

	// Start spinner
	done := make(chan struct{})
	spinnerDone := make(chan struct{})
	go func() {
		defer close(spinnerDone)
		frame := 0
		ticker := time.NewTicker(SpinnerTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				frame = (frame + 1) % len(spinnerFrames)
				printDockerStatus(statuses, frame, false)
			}
		}
	}()

	// Extract in parallel
	var wg sync.WaitGroup
	errChan := make(chan error, len(missing))

	for i, svc := range missing {
		wg.Add(1)
		go func(idx int, s dockerService) {
			defer wg.Done()
			if err := extractDockerService(ctx, binDir, s); err != nil {
				statuses[idx].markError(err)
				errChan <- fmt.Errorf("%s: %w", s.Name, err)
				return
			}
			statuses[idx].markDone()
		}(i, svc)
	}

	wg.Wait()
	close(errChan)

	// Stop spinner
	close(done)
	<-spinnerDone

	// Print final status
	printDockerStatus(statuses, 0, true)

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	// Post-extraction fixups
	if err := applyPostExtractionFixups(binDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: post-extraction fixups: %v\n", err)
	}

	return nil
}

// extractDockerService extracts a single service from its Docker image.
func extractDockerService(ctx context.Context, binDir string, svc dockerService) error {
	destDir := GetServicePath(binDir, svc.Name)
	containerName := fmt.Sprintf("supabase-extract-%s-%d", svc.Name, os.Getpid())

	// Pull image if not available locally
	pullCmd := exec.CommandContext(ctx, "docker", "pull", svc.Image)
	if out, err := pullCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker pull %s failed: %s: %w", svc.Image, string(out), err)
	}

	// Create container (not started)
	createCmd := exec.CommandContext(ctx, "docker", "create", "--name", containerName, svc.Image)
	if out, err := createCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker create %s failed: %s: %w", svc.Image, string(out), err)
	}

	// Ensure cleanup — log errors so stale containers are diagnosable
	defer func() {
		rmCmd := exec.Command("docker", "rm", containerName)
		if out, err := rmCmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove container %s: %s: %v\n", containerName, string(out), err)
		}
	}()

	// Create destination
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", destDir, err)
	}

	// Copy files out
	cpCmd := exec.CommandContext(ctx, "docker", "cp", containerName+":"+svc.SrcPath, destDir+"/")
	if out, err := cpCmd.CombinedOutput(); err != nil {
		// Cleanup partial extraction on failure
		os.RemoveAll(destDir)
		return fmt.Errorf("docker cp from %s failed: %s: %w", svc.Image, string(out), err)
	}

	return nil
}

// applyPostExtractionFixups applies host-specific fixups after Docker extraction.
// Ported from supabase-unified/extract.sh.
func applyPostExtractionFixups(binDir string) error {
	// Rebuild fs-xattr native addon for host Node.js (storage service).
	// Only attempt if npm is available to avoid noisy failures on systems
	// where Node is installed without npm.
	fsXattrDir := filepath.Join(GetServicePath(binDir, "storage"), "node_modules", "fs-xattr")
	if dirExists(fsXattrDir) {
		if _, err := exec.LookPath("npm"); err == nil {
			cmd := exec.Command("npm", "rebuild")
			cmd.Dir = fsXattrDir
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: npm rebuild fs-xattr failed: %s: %v\n", string(out), err)
			}
		}
	}

	// Sentry CPU profiler ABI symlink for postgres-meta
	profilerDir := filepath.Join(GetServicePath(binDir, "pgmeta"), "node_modules", "@sentry-internal", "node-cpu-profiler", "lib")
	if dirExists(profilerDir) {
		fixSentryABI(profilerDir)
	}

	return nil
}

// fixSentryABI creates an ABI symlink for the Sentry CPU profiler.
// The Docker image ships binaries for Node 18/20/22 (ABI 108/115/127).
// If the host has a newer ABI, create a symlink to suppress warnings.
func fixSentryABI(profilerDir string) {
	// Get host Node.js ABI
	out, err := exec.Command("node", "-e", "process.stdout.write(process.versions.modules)").Output()
	if err != nil {
		return
	}
	hostABI := strings.TrimSpace(string(out))
	if hostABI == "" {
		return
	}

	target := filepath.Join(profilerDir, fmt.Sprintf("sentry_cpu_profiler-linux-x64-glibc-%s.node", hostABI))
	if fileExistsOS(target) {
		return
	}

	// Find the highest available ABI binary (deterministic: sorted by name,
	// ABI numbers are zero-padded in filenames so lexicographic == numeric order).
	entries, err := os.ReadDir(profilerDir)
	if err != nil {
		return
	}

	const prefix = "sentry_cpu_profiler-linux-x64-glibc-"
	var best string
	var bestABI int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".node") {
			continue
		}
		// Extract ABI number: "sentry_cpu_profiler-linux-x64-glibc-127.node" → "127"
		abiStr := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".node")
		abi := 0
		for _, c := range abiStr {
			if c >= '0' && c <= '9' {
				abi = abi*10 + int(c-'0')
			}
		}
		if abi > bestABI {
			bestABI = abi
			best = name
		}
	}
	if best != "" {
		if err := os.Symlink(best, target); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create Sentry ABI symlink: %v\n", err)
		}
	}
}

// printDockerStatus prints the Docker extraction status display.
func printDockerStatus(statuses []*BinaryStatus, spinnerFrame int, final bool) {
	for range statuses {
		fmt.Fprint(os.Stderr, "\033[A\033[K")
	}

	for _, s := range statuses {
		s.mu.Lock()
		var icon, status string
		if s.Error != nil {
			icon = utils.Red("✗")
			status = fmt.Sprintf("Error - %v", s.Error)
		} else if s.Downloading {
			icon = utils.Aqua(spinnerFrames[spinnerFrame])
			status = "Extracting from Docker"
		} else {
			icon = utils.Green("✔")
			status = "Extracted"
		}
		fmt.Fprintf(os.Stderr, " %s %s %s\n", icon, s.Name, status)
		s.mu.Unlock()
	}
}

// dirExists checks if a directory exists and contains at least one entry.
func dirExists(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) > 0
}

// fileExistsOS checks if a file exists using the OS filesystem.
func fileExistsOS(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// BinaryStatus represents the installation status of a binary.
type BinaryStatus struct {
	Name            string
	InitiallyCached bool // Was already in cache before this run
	Cached          bool // Is now cached (either initially or after download)
	Downloading     bool
	Error           error
	mu              sync.Mutex
}

// markError records an installation failure on the status.
func (s *BinaryStatus) markError(err error) {
	s.mu.Lock()
	s.Error = err
	s.Downloading = false
	s.mu.Unlock()
}

// markDone records a successful installation on the status.
func (s *BinaryStatus) markDone() {
	s.mu.Lock()
	s.Cached = true
	s.Downloading = false
	s.mu.Unlock()
}

// Spinner frames for animated progress display
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// InstallBinaries downloads and installs all required binaries if not already present.
// Shows a Docker-like status display when binaries need to be downloaded.
// Returns the postgres version that was installed/found.
func InstallBinaries(ctx context.Context, fsys afero.Fs, binDir string) (postgresVersion string, err error) {
	// Ensure bin directory exists
	if err := fsys.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Get paths and check cache status
	gotruePath := GetGotruePath(binDir)
	postgrestPath := GetPostgrestPath(binDir)
	pcPath := GetProcessComposePath(binDir)

	postgresVersion = PostgresVersion
	postgresBin := GetPostgresBinPath(binDir, postgresVersion, "postgres")

	// Check which binaries are already cached
	gotrueCached := fileExists(fsys, gotruePath)
	postgrestCached := fileExists(fsys, postgrestPath)
	postgresCached := fileExists(fsys, postgresBin)
	pcCached := fileExists(fsys, pcPath)

	// If all GitHub binaries are cached, skip download phase but still check Docker services
	if gotrueCached && postgrestCached && postgresCached && pcCached {
		if err := InstallDockerServices(ctx, binDir); err != nil {
			return "", fmt.Errorf("docker service extraction: %w", err)
		}
		return postgresVersion, nil
	}

	// Show download status like Docker
	statuses := []*BinaryStatus{
		{Name: "auth", InitiallyCached: gotrueCached, Cached: gotrueCached, Downloading: !gotrueCached},
		{Name: "postgrest", InitiallyCached: postgrestCached, Cached: postgrestCached, Downloading: !postgrestCached},
		{Name: "postgres", InitiallyCached: postgresCached, Cached: postgresCached, Downloading: !postgresCached},
		{Name: "process-compose", InitiallyCached: pcCached, Cached: pcCached, Downloading: !pcCached},
	}

	// Print initial status lines (without moving cursor up)
	for _, s := range statuses {
		var icon, status string
		if s.InitiallyCached {
			icon = utils.Green("✔")
			status = "Skipped - Image is already present locally"
		} else {
			icon = utils.Aqua(spinnerFrames[0])
			status = "Pulling"
		}
		fmt.Fprintf(os.Stderr, " %s %s %s\n", icon, s.Name, status)
	}

	// Start spinner animation in background
	done := make(chan struct{})
	spinnerDone := make(chan struct{})
	go func() {
		defer close(spinnerDone)
		frame := 0
		ticker := time.NewTicker(SpinnerTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				frame = (frame + 1) % len(spinnerFrames)
				printBinaryStatus(statuses, frame, false)
			}
		}
	}()

	// Install binaries in parallel
	var wg sync.WaitGroup
	errChan := make(chan error, 4)

	// GoTrue
	if !gotrueCached {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := installGotrueFromLocalOrDownloadQuiet(ctx, fsys, gotruePath); err != nil {
				statuses[0].markError(err)
				errChan <- fmt.Errorf("auth: %w", err)
				return
			}
			statuses[0].markDone()
		}()
	}

	// PostgREST
	if !postgrestCached {
		wg.Add(1)
		go func() {
			defer wg.Done()
			postgrestURL, err := getPostgrestDownloadURL()
			if err != nil {
				statuses[1].markError(err)
				errChan <- fmt.Errorf("postgrest: %w", err)
				return
			}
			if err := installBinaryIfMissingXZQuiet(ctx, fsys, postgrestPath, postgrestURL); err != nil {
				statuses[1].markError(err)
				errChan <- fmt.Errorf("postgrest: %w", err)
				return
			}
			statuses[1].markDone()
		}()
	}

	// PostgreSQL
	if !postgresCached {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := installPostgresQuiet(ctx, fsys, binDir, postgresVersion); err != nil {
				statuses[2].markError(err)
				errChan <- fmt.Errorf("postgres: %w", err)
				return
			}
			statuses[2].markDone()
		}()
	}

	// process-compose
	if !pcCached {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pcURL, err := getProcessComposeDownloadURL()
			if err != nil {
				statuses[3].markError(err)
				errChan <- fmt.Errorf("process-compose: %w", err)
				return
			}
			var installErr error
			if strings.HasSuffix(pcURL, ".zip") {
				installErr = installBinaryFromZipQuiet(ctx, fsys, pcPath, pcURL, "process-compose")
			} else {
				installErr = installBinaryFromArchiveQuiet(ctx, fsys, pcPath, pcURL, "process-compose")
			}
			if installErr != nil {
				statuses[3].markError(installErr)
				errChan <- fmt.Errorf("process-compose: %w", installErr)
				return
			}
			statuses[3].markDone()
		}()
	}

	// Wait for all downloads to complete
	wg.Wait()
	close(errChan)

	// Stop spinner animation and wait for goroutine to exit
	close(done)
	<-spinnerDone

	// Print final status
	printBinaryStatus(statuses, 0, true)

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}

	// Extract Docker-based services (realtime, logflare, storage, pgmeta, studio)
	if err := InstallDockerServices(ctx, binDir); err != nil {
		return "", fmt.Errorf("docker service extraction: %w", err)
	}

	return postgresVersion, nil
}

// fileExists checks if a file exists.
func fileExists(fsys afero.Fs, path string) bool {
	_, err := fsys.Stat(path)
	return err == nil
}

// printBinaryStatus prints the Docker-like status display with animated spinners.
// spinnerFrame is the current frame index for the spinner animation.
func printBinaryStatus(statuses []*BinaryStatus, spinnerFrame int, final bool) {
	// Move cursor up to overwrite previous status
	for range statuses {
		fmt.Fprint(os.Stderr, "\033[A\033[K") // Move up and clear line
	}

	for _, s := range statuses {
		s.mu.Lock()
		var icon, status string

		if s.Error != nil {
			icon = utils.Red("✗")
			status = fmt.Sprintf("Error - %v", s.Error)
		} else if s.Downloading {
			icon = utils.Aqua(spinnerFrames[spinnerFrame])
			status = "Pulling"
		} else if s.InitiallyCached {
			// Was already in cache before this run
			icon = utils.Green("✔")
			if final {
				status = "Pulled"
			} else {
				status = "Skipped - Image is already present locally"
			}
		} else {
			// Just finished installing or default state
			icon = utils.Green("✔")
			status = "Pulled"
		}

		fmt.Fprintf(os.Stderr, " %s %s %s\n", icon, s.Name, status)
		s.mu.Unlock()
	}
}

// installGotrueFromLocalOrDownloadQuiet installs gotrue without printing progress.
func installGotrueFromLocalOrDownloadQuiet(ctx context.Context, fsys afero.Fs, binPath string) error {
	// Ensure parent directory exists
	if err := fsys.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		return fmt.Errorf("failed to create binary directory: %w", err)
	}

	// For darwin/arm64, check for a locally built binary first
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		localBinaryName := fmt.Sprintf("auth-v%s-darwin-arm64", GotrueVersion)
		if cliDir, err := getCliDir(); err == nil {
			localPath := filepath.Join(cliDir, localBinaryName)
			if data, err := os.ReadFile(localPath); err == nil {
				return afero.WriteFile(fsys, binPath, data, 0755)
			}
		}
	}

	// Fall back to download from GitHub releases
	gotrueURL, err := getGotrueDownloadURL()
	if err != nil {
		return err
	}
	return installBinaryFromArchiveQuiet(ctx, fsys, binPath, gotrueURL, "auth")
}

// installBinaryIfMissingXZQuiet handles .tar.xz archives without printing progress.
func installBinaryIfMissingXZQuiet(ctx context.Context, fsys afero.Fs, binPath, downloadURL string) error {
	// Ensure parent directory exists
	if err := fsys.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		return fmt.Errorf("failed to create binary directory: %w", err)
	}

	// Download the file
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed to download %s: HTTP %d", downloadURL, resp.StatusCode)
	}

	return extractTarXz(resp.Body, binPath, fsys)
}

// installBinaryFromArchiveQuiet downloads and extracts a binary without printing progress.
func installBinaryFromArchiveQuiet(ctx context.Context, fsys afero.Fs, binPath, downloadURL, srcBinName string) error {
	// Ensure parent directory exists
	if err := fsys.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		return fmt.Errorf("failed to create binary directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed to download %s: HTTP %d", downloadURL, resp.StatusCode)
	}

	return extractTarGzWithName(resp.Body, binPath, srcBinName, fsys)
}

// installBinaryFromZipQuiet downloads a .zip archive and extracts a named binary.
// Used for Windows process-compose releases which ship as .zip instead of .tar.gz.
func installBinaryFromZipQuiet(ctx context.Context, fsys afero.Fs, binPath, downloadURL, srcBinName string) error {
	if err := fsys.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		return fmt.Errorf("failed to create binary directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed to download %s: HTTP %d", downloadURL, resp.StatusCode)
	}

	// zip.Reader needs io.ReaderAt, so buffer the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	return extractZipWithName(body, binPath, srcBinName, fsys)
}

// extractZipWithName extracts a named binary from a zip archive.
func extractZipWithName(data []byte, binPath, srcBinName string, fsys afero.Fs) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("failed to open zip archive: %w", err)
	}

	for _, f := range zr.File {
		name := filepath.Base(f.Name)
		if name != srcBinName && name != srcBinName+".exe" {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in zip: %w", err)
		}
		defer rc.Close()

		binData, err := io.ReadAll(rc)
		if err != nil {
			return fmt.Errorf("failed to read binary from zip: %w", err)
		}

		if err := afero.WriteFile(fsys, binPath, binData, 0755); err != nil {
			return fmt.Errorf("failed to write binary: %w", err)
		}

		return nil
	}

	return errors.Errorf("binary %s not found in zip archive", srcBinName)
}

// installPostgresQuiet installs PostgreSQL without printing progress (except codesign warnings).
func installPostgresQuiet(ctx context.Context, fsys afero.Fs, binDir, version string) error {
	postgresDir := GetPostgresDir(binDir, version)

	// Ensure parent directory exists
	if err := fsys.MkdirAll(postgresDir, 0755); err != nil {
		return fmt.Errorf("failed to create postgres directory: %w", err)
	}

	// Download from GitHub releases
	downloadURL, err := getPostgresDownloadURL()
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed to download postgres: HTTP %d", resp.StatusCode)
	}

	// Extract tar.gz, stripping the top-level directory
	if err := extractTarGzToDir(resp.Body, postgresDir); err != nil {
		return fmt.Errorf("failed to extract postgres archive: %w", err)
	}

	// Fix permissions: the Nix-built archive ships without execute bits
	if err := fixPostgresPermissions(postgresDir); err != nil {
		return fmt.Errorf("failed to fix permissions: %w", err)
	}

	// On macOS, re-sign all binaries and libraries (suppress warnings)
	if runtime.GOOS == "darwin" {
		codesignPostgresDirQuiet(postgresDir)
	}

	return nil
}

// fixPostgresPermissions makes binaries, libraries, and scripts executable.
// The Nix-built tar archive ships all files without execute bits.
func fixPostgresPermissions(postgresDir string) error {
	return filepath.Walk(postgresDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		// Make files in bin/ and any .dylib or .sh files executable
		rel, _ := filepath.Rel(postgresDir, path)
		inBin := strings.HasPrefix(rel, "bin/") || strings.HasPrefix(rel, "bin\\")
		if inBin || strings.HasSuffix(path, ".dylib") || strings.HasSuffix(path, ".sh") {
			if err := os.Chmod(path, info.Mode()|0755); err != nil {
				return err
			}
		}
		return nil
	})
}

// codesignPostgresDirQuiet re-signs binaries without printing warnings.
func codesignPostgresDirQuiet(postgresDir string) {
	filepath.Walk(postgresDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		needsSign := info.Mode()&0111 != 0 || strings.HasSuffix(path, ".dylib")
		if needsSign {
			exec.Command("codesign", "-f", "-s", "-", path).Run()
		}
		return nil
	})
}

// extractTarGzWithName extracts a named binary from a .tar.gz archive.
func extractTarGzWithName(r io.Reader, binPath, srcBinName string, fsys afero.Fs) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	return extractTarWithName(gzr, binPath, srcBinName, fsys)
}

// extractTarGzToDir extracts a .tar.gz archive to a destination directory,
// stripping the first path component (top-level wrapper directory).
func extractTarGzToDir(r io.Reader, destDir string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}

		// Strip the first path component
		parts := strings.SplitN(header.Name, "/", 2)
		if len(parts) < 2 || parts[1] == "" {
			continue
		}
		relPath := parts[1]

		destPath := filepath.Join(destDir, relPath)

		// Path traversal protection
		if !strings.HasPrefix(destPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in archive: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory for %s: %w", destPath, err)
			}
			f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", destPath, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("failed to write file %s: %w", destPath, err)
			}
			f.Close()
		}
	}

	return nil
}

// extractTarXz extracts the first executable from a .tar.xz archive.
func extractTarXz(r io.Reader, binPath string, fsys afero.Fs) error {
	xzr, err := xz.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to create xz reader: %w", err)
	}

	return extractTarWithName(xzr, binPath, filepath.Base(binPath), fsys)
}

// extractTarWithName extracts a binary from a tar archive.
// srcBinName is the name to look for in the archive, binPath is the destination path.
func extractTarWithName(r io.Reader, binPath, srcBinName string, fsys afero.Fs) error {
	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar: %w", err)
		}

		// Look for the binary file
		name := filepath.Base(header.Name)
		if name == srcBinName || name == srcBinName+".exe" {
			if header.Typeflag != tar.TypeReg {
				continue
			}

			data, err := io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("failed to read binary from archive: %w", err)
			}

			if err := afero.WriteFile(fsys, binPath, data, 0755); err != nil {
				return fmt.Errorf("failed to write binary: %w", err)
			}

			return nil
		}
	}

	return errors.Errorf("binary %s not found in archive", srcBinName)
}

// getGotrueDownloadURL returns the download URL for GoTrue based on the current platform.
// GoTrue releases are .tar.gz archives.
func getGotrueDownloadURL() (string, error) {
	base := fmt.Sprintf("https://github.com/supabase/auth/releases/download/v%s/", GotrueVersion)

	switch {
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return base + "auth-v" + GotrueVersion + "-arm64.tar.gz", nil
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		return base + "auth-v" + GotrueVersion + "-x86_64.tar.gz", nil
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return base + "auth-v" + GotrueVersion + "-x86_64.tar.gz", nil
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm64":
		return base + "auth-v" + GotrueVersion + "-arm64.tar.gz", nil
	default:
		return "", errors.Errorf("unsupported platform for gotrue: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

// getPostgrestDownloadURL returns the download URL for PostgREST based on the current platform.
// PostgREST releases are .tar.xz archives.
func getPostgrestDownloadURL() (string, error) {
	base := fmt.Sprintf("https://github.com/PostgREST/postgrest/releases/download/v%s/", PostgrestVersion)

	switch {
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return base + "postgrest-v" + PostgrestVersion + "-macos-aarch64.tar.xz", nil
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		return base + "postgrest-v" + PostgrestVersion + "-macos-x64.tar.xz", nil
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return base + "postgrest-v" + PostgrestVersion + "-linux-static-x64.tar.xz", nil
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm64":
		return base + "postgrest-v" + PostgrestVersion + "-linux-static-aarch64.tar.xz", nil
	default:
		return "", errors.Errorf("unsupported platform for postgrest: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

// getProcessComposeDownloadURL returns the download URL for process-compose based on the current platform.
// process-compose releases are .tar.gz archives containing a single binary.
func getProcessComposeDownloadURL() (string, error) {
	base := fmt.Sprintf("https://github.com/F1bonacc1/process-compose/releases/download/v%s/", ProcessComposeVersion)

	switch {
	case runtime.GOOS == "darwin" && runtime.GOARCH == "arm64":
		return base + "process-compose_darwin_arm64.tar.gz", nil
	case runtime.GOOS == "darwin" && runtime.GOARCH == "amd64":
		return base + "process-compose_darwin_amd64.tar.gz", nil
	case runtime.GOOS == "linux" && runtime.GOARCH == "amd64":
		return base + "process-compose_linux_amd64.tar.gz", nil
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm64":
		return base + "process-compose_linux_arm64.tar.gz", nil
	case runtime.GOOS == "windows" && runtime.GOARCH == "amd64":
		return base + "process-compose_windows_amd64.zip", nil
	default:
		return "", errors.Errorf("unsupported platform for process-compose: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

// getPostgresDownloadURL returns the download URL for PostgreSQL based on the current platform.
// PostgreSQL releases are .tar.gz archives.
func getPostgresDownloadURL() (string, error) {
	base := fmt.Sprintf("https://github.com/supabase/postgres/releases/download/v%s/", PostgresVersion)

	var arch string
	switch runtime.GOARCH {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	default:
		return "", errors.Errorf("unsupported architecture for postgres: %s", runtime.GOARCH)
	}

	switch runtime.GOOS {
	case "darwin":
		if runtime.GOARCH == "amd64" {
			return "", errors.Errorf("no postgres build available for darwin/amd64")
		}
		return base + "supabase-postgres-v" + PostgresVersion + "-darwin-" + arch + ".tar.gz", nil
	case "linux":
		return base + "supabase-postgres-v" + PostgresVersion + "-linux-" + arch + ".tar.gz", nil
	default:
		return "", errors.Errorf("unsupported platform for postgres: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

// setLibraryPath sets the appropriate library path environment variable for the command.
func setLibraryPath(cmd *exec.Cmd, libDir string) {
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	switch runtime.GOOS {
	case "darwin":
		cmd.Env = append(cmd.Env, "DYLD_LIBRARY_PATH="+libDir)
	case "linux":
		cmd.Env = append(cmd.Env, "LD_LIBRARY_PATH="+libDir)
	}
}

// getCliDir returns the directory containing the CLI binary, resolving symlinks.
// This allows finding local binaries when the CLI is symlinked (e.g., /usr/local/bin/supa -> /path/to/cli/supa).
func getCliDir() (string, error) {
	cliPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks to find the actual binary location
	realPath, err := filepath.EvalSymlinks(cliPath)
	if err != nil {
		// If we can't resolve symlinks, fall back to the original path
		realPath = cliPath
	}

	return filepath.Dir(realPath), nil
}
