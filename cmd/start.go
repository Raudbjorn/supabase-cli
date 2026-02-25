package cmd

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/supabase/cli/internal/sandbox"
	"github.com/supabase/cli/internal/start"
	"github.com/supabase/cli/internal/utils"
	cliFlags "github.com/supabase/cli/internal/utils/flags"
)

func validateExcludedContainers(excludedContainers []string) {
	// Validate excluded containers
	validContainers := start.ExcludableContainers()
	var invalidContainers []string

	for _, e := range excludedContainers {
		if !slices.Contains(validContainers, e) {
			invalidContainers = append(invalidContainers, e)
		}
	}

	if len(invalidContainers) > 0 {
		// Sort the names list so it's easier to visually spot the one you looking for
		sort.Strings(validContainers)
		warning := fmt.Sprintf("%s The following container names are not valid to exclude: %s\nValid containers to exclude are: %s\n",
			utils.Yellow("WARNING:"),
			utils.Aqua(strings.Join(invalidContainers, ", ")),
			utils.Aqua(strings.Join(validContainers, ", ")))
		fmt.Fprint(os.Stderr, warning)
	}
}

var (
	allowedContainers  = start.ExcludableContainers()
	excludedContainers []string
	ignoreHealthCheck  bool
	preview            bool
	sandboxMode        bool
	reloadConfig       bool

	startCmd = &cobra.Command{
		GroupID: groupLocalDev,
		Use:     "start",
		Short:   "Start containers for Supabase local development",
		RunE: func(cmd *cobra.Command, args []string) error {
			if reloadConfig && !sandboxMode {
				return fmt.Errorf("--reload requires --sandbox")
			}
			fsys := afero.NewOsFs()
			// Sandbox mode uses process-compose with native binaries instead of Docker Compose
			if sandboxMode {
				// --reload flag: hot-reload config without full restart
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
			}
			validateExcludedContainers(excludedContainers)
			return start.Run(cmd.Context(), fsys, excludedContainers, ignoreHealthCheck)
		},
	}
)

func init() {
	flags := startCmd.Flags()
	names := strings.Join(allowedContainers, ",")
	flags.StringSliceVarP(&excludedContainers, "exclude", "x", []string{}, "Names of containers to not start. ["+names+"]")
	flags.BoolVar(&ignoreHealthCheck, "ignore-health-check", false, "Ignore unhealthy services and exit 0")
	flags.BoolVar(&preview, "preview", false, "Connect to feature preview branch")
	flags.BoolVar(&sandboxMode, "sandbox", false, "Run in sandbox mode using native binaries (experimental)")
	flags.BoolVar(&reloadConfig, "reload", false, "Hot-reload sandbox configuration without full restart (requires --sandbox)")
	cobra.CheckErr(flags.MarkHidden("preview"))
	rootCmd.AddCommand(startCmd)
}
