package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/supabase/cli/internal/sandbox"
	"github.com/supabase/cli/internal/utils"
	"github.com/supabase/cli/internal/utils/flags"
)

var (
	restartCmd = &cobra.Command{
		GroupID: groupLocalDev,
		Use:     "restart [service]",
		Short:   "Restart a local Supabase service without tearing down the stack",
		Long: `Restart a single service in the running sandbox without stopping other services.

This is much faster than 'supabase stop && supabase start' because it only
restarts the specified service while keeping the rest of the stack running.

Requires sandbox mode (--sandbox flag on start).`,
		Example: `  supabase restart auth
  supabase restart postgres
  supabase restart rest`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fsys := afero.NewOsFs()
			if err := flags.LoadConfig(fsys); err != nil {
				return err
			}

			if !sandbox.IsSandboxRunning(fsys, utils.Config.ProjectId) {
				utils.CmdSuggestion = fmt.Sprintf("Run %s to start the local sandbox first.", utils.Aqua("supabase start --sandbox"))
				return fmt.Errorf("sandbox is not running")
			}

			return sandbox.RestartService(cmd.Context(), fsys, utils.Config.ProjectId, args[0], os.Stderr)
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return sandbox.RestartableServices(), cobra.ShellCompDirectiveNoFileComp
		},
	}
)

func init() {
	restartCmd.Long += "\n\nValid services: " + strings.Join(sandbox.RestartableServices(), ", ")
	rootCmd.AddCommand(restartCmd)
}
