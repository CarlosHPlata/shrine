package cmd

import (
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/engine/local/dockercontainer"
	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [team]",
	Short: "Show live deployment status",
	Long:  `Show the live container status for all teams, or for a specific team if provided.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		backend, err := dockercontainer.NewDockerBackend(store, cfg.Registries, engine.NoopObserver{})
		if err != nil {
			return err
		}
		if len(args) == 0 {
			teams, err := store.Teams.ListTeams()
			if err != nil {
				return err
			}
			for _, t := range teams {
				cmd.Printf("Team: %s\n", t.Metadata.Name)
				if err := handler.StatusTeam(t.Metadata.Name, store, backend); err != nil {
					return err
				}
			}
			return nil
		}
		return handler.StatusTeam(args[0], store, backend)
	},
}

var statusAppCmd = &cobra.Command{
	Use:     "application [team] [name]",
	Aliases: []string{"app", "apps", "applications"},
	Short:   "Show live status for an application",
	Long:    `Show the live container status for a specific deployed application.`,
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		backend, err := dockercontainer.NewDockerBackend(store, cfg.Registries, engine.NoopObserver{})
		if err != nil {
			return err
		}
		return handler.StatusApplication(args[0], args[1], store, backend)
	},
}

var statusResourceCmd = &cobra.Command{
	Use:     "resource [team] [name]",
	Aliases: []string{"res", "resources"},
	Short:   "Show live status for a resource",
	Long:    `Show the live container status for a specific deployed resource.`,
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		backend, err := dockercontainer.NewDockerBackend(store, cfg.Registries, engine.NoopObserver{})
		if err != nil {
			return err
		}
		return handler.StatusResource(args[0], args[1], store, backend)
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.AddCommand(statusAppCmd)
	statusCmd.AddCommand(statusResourceCmd)
}
