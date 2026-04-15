package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var teamCmd = &cobra.Command{
	Use:   "team",
	Short: "Manage team registrations",
	Long:  `Add, list, inspect, and sync team definitions used for access control and quota enforcement.`,
}

var teamAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Register a new team",
	Long:  `Create a new team manifest in the teams/ directory and register it in state.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Adding team: %s\n", args[0])
	},
}

var teamRemoveCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Remove an existing team",
	Long:  `Remove a team manifest in the teams/ directory and remove it from state.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Removing team: %s\n", args[0])
	},
}

var teamListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered teams",
	Long:  `Display all teams currently registered in the platform state.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Listing teams")
	},
}

var teamShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show details for a team",
	Long:  `Display the full configuration, quotas, and current resource usage for a specific team.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Showing team: %s\n", args[0])
	},
}

var teamSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync team manifests into state",
	Long:  `Read all teams/*.yml manifests and write parsed state to the platform state directory.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Syncing all teams")
	},
}

func init() {
	rootCmd.AddCommand(teamCmd)
	teamCmd.AddCommand(teamAddCmd)
	teamCmd.AddCommand(teamListCmd)
	teamCmd.AddCommand(teamShowCmd)
	teamCmd.AddCommand(teamSyncCmd)
	teamCmd.AddCommand(teamRemoveCmd)
}
