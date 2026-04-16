package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [team]",
	Short: "Show deployment status",
	Long:  `Show the current deployment status for all teams, or for a specific team if provided.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			fmt.Println("[shrine] Showing platform status...")
		} else {
			fmt.Printf("[shrine] Showing status for team: %s\n", args[0])
		}
		fmt.Println("[shrine] Status is not yet implemented. See: shrine status --help")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
