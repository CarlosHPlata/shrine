package cmd

import (
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/spf13/cobra"
)

var dryRun bool

var deployCmd = &cobra.Command{
	Use:   "deploy [path]",
	Short: "Deploy a project from a manifest directory",
	Long:  `Parse YAML manifests from the given path, resolve dependencies, and deploy containers, routes, and DNS entries.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("[shrine] Planning deployment from: %s\n", args[0])
		if dryRun {
			return handler.DryRun(args[0], store)
		}
		return nil
	},
}

func init() {
	deployCmd.PersistentFlags().BoolVarP(&dryRun, "dry-run", "d", false, "Dry run, do not apply changes to the platform and show what would be done")
	rootCmd.AddCommand(deployCmd)
}
