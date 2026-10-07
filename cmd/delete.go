package cmd

import (
	"github.com/CarlosHPlata/shrine/internal/app"
	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/CarlosHPlata/shrine/internal/manifest"
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
	deleteResTeam   string
	deleteResDryRun bool
)

var deleteApplicationCmd = &cobra.Command{
	Use:   "application [name]",
	Short: "Delete an application from state and release its host port and image pin",
	Long: `Forget an application: release its published host port allocation and its
image pin, and drop its stale deployment record. The application's container
must already be torn down — Docker state is authoritative and a live container
blocks the delete.`,
	Args: cobra.ExactArgs(1),
	RunE: runDelete(manifest.ApplicationKind, &deleteAppTeam, &deleteAppDryRun),
}

var deleteResourceCmd = &cobra.Command{
	Use:   "resource [name]",
	Short: "Delete a resource from state and release its image pin",
	Long: `Forget a resource: release its image pin and drop its stale deployment
record. The resource's container must already be torn down — Docker state is
authoritative and a live container blocks the delete.`,
	Args: cobra.ExactArgs(1),
	RunE: runDelete(manifest.ResourceKind, &deleteResTeam, &deleteResDryRun),
}

// runDelete builds the RunE shared by delete application and delete
// resource; the flag pointers are read at run time, after Cobra parsed them.
func runDelete(kind string, team *string, dryRun *bool) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		backend, err := app.NewQueryContainerBackend(cfg, store)
		if err != nil {
			return err
		}
		opts := handler.DeleteOptions{Name: args[0], Team: *team, DryRun: *dryRun}
		if kind == manifest.ResourceKind {
			return handler.DeleteResource(store, backend, opts)
		}
		return handler.DeleteApplication(store, backend, opts)
	}
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.AddCommand(deleteTeamCmd)
	deleteCmd.AddCommand(deleteApplicationCmd)
	deleteCmd.AddCommand(deleteResourceCmd)
	deleteApplicationCmd.Flags().StringVarP(&deleteAppTeam, "team", "t", "", "Team owning the application (searched automatically when omitted)")
	deleteApplicationCmd.Flags().BoolVar(&deleteAppDryRun, "dry-run", false, "Print what would be released without changing state")
	deleteResourceCmd.Flags().StringVarP(&deleteResTeam, "team", "t", "", "Team owning the resource (searched automatically when omitted)")
	deleteResourceCmd.Flags().BoolVar(&deleteResDryRun, "dry-run", false, "Print what would be released without changing state")
}
