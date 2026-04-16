package cmd

import (
	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Display one or many resources",
	Long:  `List resources registered in the platform state.`,
}

var getTeamsCmd = &cobra.Command{
	Use:   "teams",
	Short: "List all registered teams",
	Long:  `Display all teams currently registered in the platform state.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handler.ListTeams(store)
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
	getCmd.AddCommand(getTeamsCmd)
}
