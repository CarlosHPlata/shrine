package cmd

import (
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/updater"
	"github.com/spf13/cobra"
)

var checkOnly bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update shrine to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		latest, err := updater.LatestVersion()
		if err != nil {
			return fmt.Errorf("checking for updates: %w", err)
		}

		if !updater.IsNewer(Version, latest) {
			cmd.Printf("shrine is already up to date (%s)\n", Version)
			return nil
		}

		cmd.Printf("New version available: %s (current: %s)\n", latest, Version)

		if checkOnly {
			cmd.Printf("Run 'shrine update' to install it.\n")
			return nil
		}

		return updater.Update(cmd.OutOrStdout())
	},
}

func init() {
	updateCmd.Flags().BoolVar(&checkOnly, "check", false, "Only check for a new version, do not install")
	rootCmd.AddCommand(updateCmd)
}
