package cmd

import (
	"github.com/CarlosHPlata/shrine/internal/app"
	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete resources from state",
	Long:  `Remove resources from the platform state.`,
}

var deleteTeamCmd = &cobra.Command{
	Use:   "team [name]",
	Short: "Delete a team from state",
	Long:  `Remove a team from the platform state. This does not delete the manifest file.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handler.DeleteTeam(args[0], store)
	},
}

var (
	deleteAppTeam   string
	deleteAppDryRun bool
)

var deleteApplicationCmd = &cobra.Command{
	Use:   "application [name]",
	Short: "Delete an application from state and release its published host port",
	Long: `Forget an application: release its published host port allocation and drop
its stale deployment record. The application's container must already be torn
down — Docker state is authoritative and a live container blocks the delete.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		backend, err := app.NewQueryContainerBackend(cfg, store)
		if err != nil {
			return err
		}
		return handler.DeleteApplication(store, backend, handler.DeleteApplicationOptions{
			Name:   args[0],
			Team:   deleteAppTeam,
			DryRun: deleteAppDryRun,
		})
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.AddCommand(deleteTeamCmd)
	deleteCmd.AddCommand(deleteApplicationCmd)
	deleteApplicationCmd.Flags().StringVarP(&deleteAppTeam, "team", "t", "", "Team owning the application (searched automatically when omitted)")
	deleteApplicationCmd.Flags().BoolVar(&deleteAppDryRun, "dry-run", false, "Print what would be released without changing state")
}
