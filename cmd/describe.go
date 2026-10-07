package cmd

import (
	"github.com/CarlosHPlata/shrine/internal/app"
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

var describeAppCmd = &cobra.Command{
	Use:   "app [name]",
	Short: "Show details for a deployed application",
	Long: `Display the deployment record for a specific application.

If --team is omitted, all teams are searched automatically. If the application
name is found in more than one team you will be prompted to disambiguate with
--team.

The record shows the image the manifest named and the effective pull policy.
Under the Pinned policy it also shows the pin (exact version, readable version,
and date) and, when the container runtime can be reached, the image the running
container was started from; a pin that differs from the running image has been
recorded but not yet deployed.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		team, _ := cmd.Flags().GetString("team")
		backend, err := app.NewQueryContainerBackend(cfg, store)
		if err != nil {
			return err
		}
		return handler.DescribeApplication(team, args[0], store, backend)
	},
}

var describeResourceCmd = &cobra.Command{
	Use:   "resource [name]",
	Short: "Show details for a deployed resource",
	Long: `Display the deployment record for a specific resource.

If --team is omitted, all teams are searched automatically. If the resource
name is found in more than one team you will be prompted to disambiguate with
--team.

The record shows the image the manifest named and the effective pull policy.
Under the Pinned policy it also shows the pin (exact version, readable version,
and date) and, when the container runtime can be reached, the image the running
container was started from; a pin that differs from the running image has been
recorded but not yet deployed.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		team, _ := cmd.Flags().GetString("team")
		backend, err := app.NewQueryContainerBackend(cfg, store)
		if err != nil {
			return err
		}
		return handler.DescribeResource(team, args[0], store, backend)
	},
}

func init() {
	rootCmd.AddCommand(describeCmd)
	describeCmd.AddCommand(describeTeamCmd)
	describeCmd.AddCommand(describeAppCmd)
	describeCmd.AddCommand(describeResourceCmd)

	describeAppCmd.Flags().String("team", "", "Restrict search to this team (optional)")
	describeResourceCmd.Flags().String("team", "", "Restrict search to this team (optional)")
}
