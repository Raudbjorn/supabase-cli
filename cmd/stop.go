package cmd

import (
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/supabase/cli/internal/stop"
)

var (
	noBackup       bool
	projectId      string
	all            bool
	stopServiceName string

	stopCmd = &cobra.Command{
		GroupID: groupLocalDev,
		Use:     "stop",
		Short:   "Stop all local Supabase containers",
		Long: `Stop all local Supabase services, or stop a single service with --service.

In sandbox mode, --service stops only the named service while keeping
the rest of the stack running. This is useful for freeing resources
(e.g., stopping realtime to save battery) without tearing down the
entire development environment.`,
		Example: `  supabase stop
  supabase stop --service auth
  supabase stop --no-backup`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return stop.Run(cmd.Context(), !noBackup, projectId, all, stopServiceName, afero.NewOsFs())
		},
	}
)

func init() {
	flags := stopCmd.Flags()
	flags.Bool("backup", true, "Backs up the current database before stopping.")
	flags.StringVar(&projectId, "project-id", "", "Local project ID to stop.")
	cobra.CheckErr(flags.MarkHidden("backup"))
	flags.BoolVar(&noBackup, "no-backup", false, "Deletes all data volumes after stopping.")
	flags.BoolVar(&all, "all", false, "Stop all local Supabase instances from all projects across the machine.")
	flags.StringVar(&stopServiceName, "service", "", "Stop only the specified service (sandbox mode only).")
	stopCmd.MarkFlagsMutuallyExclusive("project-id", "all")
	stopCmd.MarkFlagsMutuallyExclusive("service", "all")
	stopCmd.MarkFlagsMutuallyExclusive("service", "no-backup")
	rootCmd.AddCommand(stopCmd)
}
