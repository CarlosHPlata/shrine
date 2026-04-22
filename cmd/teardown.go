package cmd

import (
	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/spf13/cobra"
)

var teardownCmd = &cobra.Command{
	Use:   "teardown [team]",
	Short: "Tear down all resources for a team",
	Long:  `Stop and remove all containers, networks, routes, and DNS entries associated with the given team.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Printf("[shrine] Planning teardown for team: %s\n", args[0])
		return handler.Teardown(handler.TeardownOptions{
			Team:   args[0],
			Out:    cmd.OutOrStdout(),
			Paths:  paths,
			Store:  store,
			Config: cfg,
		})
	},
}

func init() {
	rootCmd.AddCommand(teardownCmd)
}
