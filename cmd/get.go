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
	Use:     "teams",
	Aliases: []string{"team"},
	Short:   "List all registered teams",
	Long:    `Display all teams currently registered in the platform state.`,
	Args:    cobra.NoArgs,
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

var getTeamFlag string

var getApplicationsCmd = &cobra.Command{
	Use:     "applications",
	Aliases: []string{"apps"},
	Short:   "List deployed applications",
	Long:    `Display all deployed applications, optionally filtered by team.`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handler.ListApplications(getTeamFlag, store)
	},
}

var getResourcesCmd = &cobra.Command{
	Use:     "resources",
	Aliases: []string{"res"},
	Short:   "List deployed resources",
	Long:    `Display all deployed resources, optionally filtered by team.`,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handler.ListResources(getTeamFlag, store)
	},
}

var getDeployedCmd = &cobra.Command{
	Use:   "deployed",
	Short: "List all deployed workloads",
	Long:  `Display all deployed applications and resources, optionally filtered by team.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return handler.ListDeployed(getTeamFlag, store)
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
	getCmd.AddCommand(getTeamsCmd)
	getCmd.AddCommand(getConfigDir)
	getCmd.AddCommand(getStateDir)
	getCmd.PersistentFlags().StringVarP(&getTeamFlag, "team", "t", "", "Filter by team name")
	getCmd.AddCommand(getApplicationsCmd)
	getCmd.AddCommand(getResourcesCmd)
	getCmd.AddCommand(getDeployedCmd)
}
