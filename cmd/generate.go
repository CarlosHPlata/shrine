package cmd

import (
	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/spf13/cobra"
)

var generatePath string

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate scaffold manifests",
	Long:  `Generate skeleton manifest files for resources like teams.`,
}

var generateTeamCmd = &cobra.Command{
	Use:   "team [name]",
	Short: "Generate a new team manifest",
	Long:  `Create a skeleton team manifest YAML file in the specified directory.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handler.GenerateTeam(args[0], generatePath)
	},
}

func init() {
	generateCmd.PersistentFlags().StringVarP(&generatePath, "path", "p", "teams", "Directory to write the manifest to")

	rootCmd.AddCommand(generateCmd)
	generateCmd.AddCommand(generateTeamCmd)
}
