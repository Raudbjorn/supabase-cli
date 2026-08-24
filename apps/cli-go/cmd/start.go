package cmd

// Upstream deleted internal/start (its Docker-based `supabase start`) once the
// TypeScript CLI took ownership of starting the local stack, and dropped the
// `start` command from this binary entirely. This fork follows that decision:
// there is no Docker-based start here anymore.
//
// What remains is fork-only: `supabase start --sandbox` runs the local stack
// natively via process-compose (internal/sandbox) instead of Docker. That
// feature does not exist upstream, so this command is kept solely to expose
// it. It deliberately has no non-sandbox code path.

import (
	"fmt"
	"os"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/supabase/cli/internal/sandbox"
	"github.com/supabase/cli/internal/utils"
	cliFlags "github.com/supabase/cli/internal/utils/flags"
)

var (
	sandboxMode  bool
	reloadConfig bool

	startCmd = &cobra.Command{
		GroupID: groupLocalDev,
		Use:     "start",
		Short:   "Start the local development stack natively (requires --sandbox)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !sandboxMode {
				utils.CmdSuggestion = fmt.Sprintf("Run %s to start the native stack, or use the Supabase CLI for the Docker-based stack.", utils.Aqua("supabase start --sandbox"))
				return fmt.Errorf("only --sandbox is supported by this binary; the Docker-based start lives in the Supabase CLI")
			}
			fsys := afero.NewOsFs()
			// --reload: hot-reload config without a full restart
			if reloadConfig {
				if err := cliFlags.LoadConfig(fsys); err != nil {
					return err
				}
				if !sandbox.IsSandboxRunning(fsys, utils.Config.ProjectId) {
					return fmt.Errorf("sandbox is not running. Start it first with: %s", utils.Aqua("supabase start --sandbox"))
				}
				return sandbox.ReloadConfig(cmd.Context(), fsys, utils.Config.ProjectId, os.Stderr)
			}
			return sandbox.Run(cmd.Context(), fsys)
		},
	}
)

func init() {
	flags := startCmd.Flags()
	flags.BoolVar(&sandboxMode, "sandbox", false, "Run all services natively without Docker (postgres, auth, rest, realtime, storage, analytics, meta, studio)")
	flags.BoolVar(&reloadConfig, "reload", false, "Hot-reload sandbox configuration without full restart (requires --sandbox)")
	rootCmd.AddCommand(startCmd)
}
