package cmd

import (
	"github.com/CarlosHPlata/shrine/internal/handler"
	"github.com/spf13/cobra"
)

var generatePath string
var generateTeam string

// App flags
var appPort int
var appReplicas int
var appDomain string
var appPathPrefix string
var appExpose bool
var appImage string

// Resource flags
var resType string
var resVersion string
var resExpose bool

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

var generateAppCmd = &cobra.Command{
	Use:   "app [name]",
	Short: "Generate a new application manifest",
	Long:  `Create a skeleton application manifest YAML file in the specified directory.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		// Dynamic defaults if flags not set
		image := appImage
		if image == "" {
			image = name + ":latest"
		}
		domain := appDomain
		if domain == "" {
			domain = name + ".home.lab"
		}
		pathPrefix := appPathPrefix
		if pathPrefix == "" {
			pathPrefix = "/" + name
		}

		return handler.GenerateApp(handler.AppOptions{
			Name:             name,
			Team:             generateTeam,
			OutputDir:        generatePath,
			Port:             appPort,
			Replicas:         appReplicas,
			Domain:           domain,
			PathPrefix:       pathPrefix,
			ExposeToPlatform: appExpose,
			Image:            image,
		})
	},
}

var generateResourceCmd = &cobra.Command{
	Use:   "resource [name]",
	Short: "Generate a new resource manifest",
	Long:  `Create a skeleton resource manifest YAML file in the specified directory.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return handler.GenerateResource(handler.ResourceOptions{
			Name:             args[0],
			Team:             generateTeam,
			OutputDir:        generatePath,
			Type:             resType,
			Version:          resVersion,
			ExposeToPlatform: resExpose,
		})
	},
}

func init() {
	generateCmd.PersistentFlags().StringVarP(&generatePath, "path", "p", ".", "Directory to write the manifest to")

	// App flags
	generateAppCmd.PersistentFlags().StringVarP(&generateTeam, "team", "t", "default-team", "Team that owns the resource")
	generateAppCmd.Flags().IntVar(&appPort, "port", 8080, "Port the application listens on")
	generateAppCmd.Flags().IntVar(&appReplicas, "replicas", 1, "Number of replicas to run")
	generateAppCmd.Flags().StringVar(&appDomain, "domain", "", "Public domain for the application (defaults to [name].home.lab)")
	generateAppCmd.Flags().StringVar(&appPathPrefix, "pathprefix", "", "Path prefix for routing (defaults to /[name])")
	generateAppCmd.Flags().BoolVar(&appExpose, "expose", false, "Expose to Platform network")
	generateAppCmd.Flags().StringVar(&appImage, "image", "", "Docker image to run (defaults to [name]:latest)")

	// Resource flags
	generateResourceCmd.PersistentFlags().StringVarP(&generateTeam, "team", "t", "default-team", "Team that owns the resource")
	generateResourceCmd.Flags().StringVar(&resType, "type", "postgres", "Type of resource")
	generateResourceCmd.Flags().StringVar(&resVersion, "version", "16", "Version of the resource")
	generateResourceCmd.Flags().BoolVar(&resExpose, "expose", false, "Expose Toplatform network")

	rootCmd.AddCommand(generateCmd)
	generateCmd.AddCommand(generateTeamCmd)
	generateCmd.AddCommand(generateAppCmd)
	generateCmd.AddCommand(generateResourceCmd)
}
