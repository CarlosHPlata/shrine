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
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			fmt.Println("Showing all status")
		} else {
			fmt.Printf("Status of team: %s\n", args[0])
		}
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
