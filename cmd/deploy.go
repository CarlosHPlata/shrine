package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:   "deploy [path]",
	Short: "Deploy a project from a manifest directory",
	Long:  `Parse YAML manifests from the given path, resolve dependencies, and deploy containers, routes, and DNS entries.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Deploy project from path: %s\n", args[0])
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)
}
