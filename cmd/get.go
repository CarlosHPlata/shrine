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
		return handler.ListTeams(store.Teams)
	},
}

var getConfigDir = &cobra.Command{
	Use:   "config-dir",
	Short: "Display the configuration directory",
	Long:  `Display the configuration directory.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Printf("%s\n", paths.ConfigDir)
		return nil
	},
}

var getStateDir = &cobra.Command{
	Use:   "state-dir",
	Short: "Display the state directory",
	Long:  `Display the state directory.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Printf("%s\n", paths.StateDir)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
	getCmd.AddCommand(getTeamsCmd)
	getCmd.AddCommand(getConfigDir)
	getCmd.AddCommand(getStateDir)
}
