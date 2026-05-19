package cmd

import (
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/app"
	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/CarlosHPlata/shrine/internal/planner"
	"github.com/CarlosHPlata/shrine/internal/updater"
	"github.com/spf13/cobra"
)

var dryRun bool
var deployPath string

var deployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy a project from a manifest directory",
	Long:  `Parse YAML manifests from the given path, resolve dependencies, and deploy containers, routes, and DNS entries.`,
	Args:  cobra.NoArgs,
	RunE:  runDeploy(planner.NoFilter()),
}

var deployTeamCmd = &cobra.Command{
	Use:   "team <name>",
	Short: "Deploy only the apps and resources owned by one team",
	Long:  `Deploy only the manifests whose metadata.owner matches the given team name. Cross-team dependencies are still resolved from the specs directory but are not redeployed.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDeploy(planner.ByTeam(args[0]))(cmd, args)
	},
}

func runDeploy(filter planner.Filter) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		checkForUpdate(cmd)

		dir, err := cfg.ResolveSpecsDir(deployPath)
		if err != nil {
			return err
		}
		printDeployHeader(cmd, filter, dir)

		if dryRun {
			return handler.DryRun(cmd.OutOrStdout(), dir, store, cfg, filter)
		}
		bundle, cleanup, err := app.BuildDeployBundle(cfg, store, paths, dir, cmd.OutOrStdout())
		if err != nil {
			return err
		}
		defer cleanup()
		return handler.Deploy(bundle, dir, filter)
	}
}

func printDeployHeader(cmd *cobra.Command, filter planner.Filter, dir string) {
	if filter.Kind == planner.FilterTeam {
		cmd.Printf("[shrine] Planning deployment for team %q from: %s\n", filter.Name, dir)
		return
	}
	cmd.Printf("[shrine] Planning deployment from: %s\n", dir)
}

func checkForUpdate(cmd *cobra.Command) {
	latest, err := updater.LatestVersion()
	if err != nil {
		return
	}
	if updater.IsNewer(Version, latest) {
		fmt.Fprintf(cmd.OutOrStdout(), "\n[shrine] Update available: %s → %s. Run 'shrine update' to install.\n\n", Version, latest)
	}
}

func init() {
	deployCmd.PersistentFlags().BoolVarP(&dryRun, "dry-run", "d", false, "Dry run, do not apply changes to the platform and show what would be done")
	deployCmd.PersistentFlags().StringVarP(&deployPath, "path", "p", "", "Directory containing manifest files (overrides specsDir in config.yml)")
	deployCmd.AddCommand(deployTeamCmd)
	rootCmd.AddCommand(deployCmd)
}
