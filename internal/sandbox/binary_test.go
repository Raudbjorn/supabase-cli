package sandbox

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/spf13/afero"
)

func createTestZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip entry %q: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("failed to write zip entry %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}
	return buf.Bytes()
}

func TestExtractZipWithName(t *testing.T) {
	t.Run("extracts named binary", func(t *testing.T) {
		binContent := []byte("fake-binary-content")
		data := createTestZip(t, map[string][]byte{
			"process-compose": binContent,
			"README.md":       []byte("readme"),
		})

		fsys := afero.NewMemMapFs()
		binPath := "/tmp/test-bin/process-compose"

		if err := extractZipWithName(data, binPath, "process-compose", fsys); err != nil {
			t.Fatalf("extractZipWithName() error = %v", err)
		}

		got, err := afero.ReadFile(fsys, binPath)
		if err != nil {
			t.Fatalf("failed to read extracted binary: %v", err)
		}
		if !bytes.Equal(got, binContent) {
			t.Errorf("binary content = %q, want %q", got, binContent)
		}
	})

	t.Run("extracts .exe variant", func(t *testing.T) {
		binContent := []byte("windows-binary")
		data := createTestZip(t, map[string][]byte{
			"process-compose.exe": binContent,
		})

		fsys := afero.NewMemMapFs()
		binPath := "/tmp/test-bin/process-compose.exe"

		if err := extractZipWithName(data, binPath, "process-compose", fsys); err != nil {
			t.Fatalf("extractZipWithName() error = %v", err)
		}

		got, err := afero.ReadFile(fsys, binPath)
		if err != nil {
			t.Fatalf("failed to read extracted binary: %v", err)
		}
		if !bytes.Equal(got, binContent) {
			t.Errorf("binary content = %q, want %q", got, binContent)
		}
	})

	t.Run("returns error when binary not found", func(t *testing.T) {
		data := createTestZip(t, map[string][]byte{
			"other-file": []byte("not the binary"),
		})

		fsys := afero.NewMemMapFs()
		err := extractZipWithName(data, "/tmp/test-bin/process-compose", "process-compose", fsys)
		if err == nil {
			t.Fatal("expected error when binary not in archive")
		}
	})

	t.Run("returns error for invalid zip", func(t *testing.T) {
		fsys := afero.NewMemMapFs()
		err := extractZipWithName([]byte("not a zip"), "/tmp/test-bin/pc", "pc", fsys)
		if err == nil {
			t.Fatal("expected error for invalid zip data")
		}
	})
}
