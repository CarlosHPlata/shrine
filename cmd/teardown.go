package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var teardownCmd = &cobra.Command{
	Use:   "teardown [team]",
	Short: "Tear down all resources for a team",
	Long:  `Stop and remove all containers, networks, routes, and DNS entries associated with the given team.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Tearing down team: %s\n", args[0])
	},
}

func init() {
	rootCmd.AddCommand(teardownCmd)
}
