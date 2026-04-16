package cmd

import (
	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/spf13/cobra"
)

var describeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Show detailed info about a resource",
	Long:  `Display the full configuration and status for a specific resource.`,
}

var describeTeamCmd = &cobra.Command{
	Use:   "team [name]",
	Short: "Show details for a team",
	Long:  `Display the full configuration from state for a specific team.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handler.DescribeTeam(args[0], store)
	},
}

func init() {
	rootCmd.AddCommand(describeCmd)
	describeCmd.AddCommand(describeTeamCmd)
}
