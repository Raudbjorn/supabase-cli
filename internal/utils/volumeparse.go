package utils

import (
	"fmt"
	"path/filepath"
	"strings"
)

// VolumeSpec represents a parsed Docker volume specification.
// Replaces github.com/docker/cli/cli/compose/loader.ParseVolume.
type VolumeSpec struct {
	Type   string // "bind" or "volume"
	Source string
	Target string
	Mode   string // "rw", "ro", etc.
}

// ParseVolume parses a Docker volume specification string.
// Format: [source:]target[:mode]
// If source is an absolute path or relative path, it's a bind mount.
// Otherwise it's a named volume.
func ParseVolume(spec string) (VolumeSpec, error) {
	parts := strings.SplitN(spec, ":", 3)

	switch len(parts) {
	case 1:
		// Just a target path (anonymous volume)
		return VolumeSpec{
			Type:   "volume",
			Target: parts[0],
		}, nil
	case 2:
		// Could be source:target or target:mode
		if isMode(parts[1]) {
			return VolumeSpec{
				Type:   "volume",
				Target: parts[0],
				Mode:   parts[1],
			}, nil
		}
		return VolumeSpec{
			Type:   volumeType(parts[0]),
			Source: parts[0],
			Target: parts[1],
		}, nil
	case 3:
		// source:target:mode
		return VolumeSpec{
			Type:   volumeType(parts[0]),
			Source: parts[0],
			Target: parts[1],
			Mode:   parts[2],
		}, nil
	default:
		return VolumeSpec{}, fmt.Errorf("invalid volume specification: %s", spec)
	}
}

// volumeType determines if a source is a bind mount or named volume.
func volumeType(source string) string {
	if filepath.IsAbs(source) || strings.HasPrefix(source, ".") || strings.HasPrefix(source, "~") {
		return "bind"
	}
	return "volume"
}

// isMode checks if a string is a valid volume mode.
func isMode(s string) bool {
	switch s {
	case "rw", "ro", "z", "Z", "nocopy", "delegated", "cached", "consistent":
		return true
	}
	return false
}
