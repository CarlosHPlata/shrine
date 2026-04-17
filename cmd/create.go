package cmd

import (
	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/spf13/cobra"
)

var createFilePath string

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a resource in state from a manifest file",
	Long:  `Parse a single manifest file and register it in the platform state.`,
}

var createTeamCmd = &cobra.Command{
	Use:   "team",
	Short: "Create a team in state from a manifest file",
	Long:  `Parse a team manifest YAML file and save it to the platform state.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handler.CreateTeam(createFilePath, store.Teams)
	},
}

func init() {
	createTeamCmd.Flags().StringVarP(&createFilePath, "file", "f", "", "Path to the team manifest file")
	createTeamCmd.MarkFlagRequired("file")

	rootCmd.AddCommand(createCmd)
	createCmd.AddCommand(createTeamCmd)
}
