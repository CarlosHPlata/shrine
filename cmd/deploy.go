package cmd

import (
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/handler"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		checkForUpdate(cmd)

		dir, err := cfg.ResolveSpecsDir(deployPath)
		if err != nil {
			return err
		}
		cmd.Printf("[shrine] Planning deployment from: %s\n", dir)
		if dryRun {
			return handler.DryRun(cmd.OutOrStdout(), dir, store, cfg)
		}
		return handler.Deploy(handler.DeployOptions{
			Out:         cmd.OutOrStdout(),
			ManifestDir: dir,
			Store:       store,
			Config:      cfg,
			Paths:       paths,
		})
	},
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
	deployCmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Dry run, do not apply changes to the platform and show what would be done")
	deployCmd.Flags().StringVarP(&deployPath, "path", "p", "", "Directory containing manifest files (overrides specsDir in config.yml)")
	rootCmd.AddCommand(deployCmd)
}
