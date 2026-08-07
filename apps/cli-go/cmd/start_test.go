package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pins the cobra registration/flag surface of cmd/start.go. Unlike upstream,
// this fork keeps internal/start functional (see internal/start/start.go):
// this package ships the bare Go binary directly, with no TypeScript CLI
// wrapping it, so `supabase start` must still work without --sandbox.
func TestStartCommandSurface(t *testing.T) {
	require.Same(t, rootCmd, startCmd.Parent())

	excludeFlag := startCmd.Flags().Lookup("exclude")
	require.NotNil(t, excludeFlag)
	assert.False(t, excludeFlag.Hidden)

	ignoreHealthCheckFlag := startCmd.Flags().Lookup("ignore-health-check")
	require.NotNil(t, ignoreHealthCheckFlag)
	assert.False(t, ignoreHealthCheckFlag.Hidden)

	previewFlag := startCmd.Flags().Lookup("preview")
	require.NotNil(t, previewFlag)
	assert.True(t, previewFlag.Hidden)
}
