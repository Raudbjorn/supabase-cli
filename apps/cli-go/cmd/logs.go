package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/supabase/cli/internal/sandbox"
	"github.com/supabase/cli/internal/utils"
	"github.com/supabase/cli/internal/utils/flags"
)

var (
	followLogs bool
	tailLines  int

	logsCmd = &cobra.Command{
		GroupID: groupLocalDev,
		Use:     "logs [service]",
		Short:   "Stream logs from local Supabase services",
		Long: `View and stream logs from sandbox services.

Without --follow, shows the most recent log lines. With --follow,
streams logs in real-time until interrupted with Ctrl+C.

If no service is specified, shows logs from all running services.

Requires sandbox mode (--sandbox flag on start).`,
		Example: `  supabase logs
  supabase logs auth
  supabase logs postgres --follow
  supabase logs rest --tail 50`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fsys := afero.NewOsFs()
			if err := flags.LoadConfig(fsys); err != nil {
				return err
			}

			if !sandbox.IsSandboxRunning(fsys, utils.Config.ProjectId) {
				utils.CmdSuggestion = fmt.Sprintf("Run %s to start the local sandbox first.", utils.Aqua("supabase start --sandbox"))
				return fmt.Errorf("sandbox is not running")
			}

			service := ""
			if len(args) > 0 {
				service = args[0]
			}

			return sandbox.StreamLogs(cmd.Context(), fsys, utils.Config.ProjectId, service, followLogs, tailLines, os.Stdout)
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return sandbox.ValidServiceNames(), cobra.ShellCompDirectiveNoFileComp
		},
	}
)

func init() {
	f := logsCmd.Flags()
	f.BoolVarP(&followLogs, "follow", "f", false, "Stream logs in real-time (like tail -f)")
	f.IntVar(&tailLines, "tail", 100, "Number of recent log lines to show")
	rootCmd.AddCommand(logsCmd)
}
