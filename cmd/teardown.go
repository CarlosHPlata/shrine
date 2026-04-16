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
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("[shrine] Planning teardown for team: %s\n", args[0])
		fmt.Println("[shrine] Teardown is not yet implemented. See: shrine teardown --help")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(teardownCmd)
}
