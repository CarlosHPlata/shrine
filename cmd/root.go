package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "shrine",
	Short: "shrine is a CLI tool that interprets declarative YAML manifests and orchestrates Docker containers.",
	Long: `shrine is a CLI tool that interprets declarative YAML manifests and orchestrates Docker containers.`,
}

func Execute() error {
	return rootCmd.Execute()
}