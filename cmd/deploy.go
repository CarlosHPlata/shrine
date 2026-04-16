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
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("[shrine] Planning deployment from: %s\n", args[0])
		fmt.Println("[shrine] Deploy is not yet implemented. See: shrine deploy --help")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deployCmd)
}
