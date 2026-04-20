package cmd

import (
	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/spf13/cobra"
)

var applyPath string

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply manifests to state",
	Long:  `Declaratively sync manifest files from a directory into the platform state.`,
}

var applyTeamsCmd = &cobra.Command{
	Use:   "teams",
	Short: "Apply all team manifests to state",
	Long:  `Scan a directory for team manifest YAML files and sync them into the platform state.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handler.ApplyTeams(applyPath, store.Teams)
	},
}

func init() {
	applyCmd.PersistentFlags().StringVarP(&applyPath, "path", "p", ".", "Directory containing manifest files")

	rootCmd.AddCommand(applyCmd)
	applyCmd.AddCommand(applyTeamsCmd)
}
