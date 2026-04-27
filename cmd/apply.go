package cmd

import (
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/spf13/cobra"
)

var applyPath string
var applyFile string

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply manifests to state",
	Long:  `Declaratively sync manifest files from a directory into the platform state.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if applyFile != "" {
			return handler.ApplySingle(handler.ApplySingleOptions{
				Out:    cmd.OutOrStdout(),
				File:   applyFile,
				Store:  store,
				Config: cfg,
				Paths:  paths,
			})
		}
		return fmt.Errorf("specify a subcommand (e.g. shrine apply teams) or use --file/-f to apply a single manifest")
	},
}

var applyTeamsCmd = &cobra.Command{
	Use:   "teams",
	Short: "Apply all team manifests to state",
	Long:  `Scan a directory for team manifest YAML files and sync them into the platform state.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := cfg.ResolveSpecsDir(applyPath)
		if err != nil {
			return err
		}
		return handler.ApplyTeams(dir, store.Teams)
	},
}

func init() {
	applyCmd.PersistentFlags().StringVarP(&applyPath, "path", "p", "", "Directory containing manifest files (overrides specsDir in config.yml)")
	applyCmd.Flags().StringVarP(&applyFile, "file", "f", "", "Manifest file to apply (infers kind from the manifest's kind field)")

	rootCmd.AddCommand(applyCmd)
	applyCmd.AddCommand(applyTeamsCmd)
}
