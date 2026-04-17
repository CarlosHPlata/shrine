package cmd

import (
	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete resources from state",
	Long:  `Remove resources from the platform state.`,
}

var deleteTeamCmd = &cobra.Command{
	Use:   "team [name]",
	Short: "Delete a team from state",
	Long:  `Remove a team from the platform state. This does not delete the manifest file.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handler.DeleteTeam(args[0], store.Teams)
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.AddCommand(deleteTeamCmd)
}
