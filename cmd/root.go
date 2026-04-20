package cmd

import (
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/CarlosHPlata/shrine/internal/state/local"
	"github.com/spf13/cobra"
)

var (
	configDirFlag string
	stateDirFlag  string
	paths         *config.Paths
	store         *state.Store
	cfg           *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "shrine",
	Short: "shrine is a CLI tool that interprets declarative YAML manifests and orchestrates Docker containers.",
	Long:  `shrine is a CLI tool that interprets declarative YAML manifests and orchestrates Docker containers.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		paths, err = config.ResolvePaths(configDirFlag, stateDirFlag)
		if err != nil {
			return fmt.Errorf("resolving paths: %w", err)
		}

		cfg, err = config.Load(paths.ConfigDir)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		store, err = local.NewLocalStore(paths.StateDir)
		if err != nil {
			return fmt.Errorf("initializing state store: %w", err)
		}

		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

// SetArgs sets the arguments for the root command. Used for testing.
func SetArgs(args []string) {
	rootCmd.SetArgs(args)
}

// SetOutput sets the output and error writers for the root command. Used for testing.
func SetOutput(w io.Writer) {
	rootCmd.SetOut(w)
	rootCmd.SetErr(w)
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configDirFlag, "config-dir", "", "Configuration directory (default is ~/.config/shrine or /etc/shrine)")
	rootCmd.PersistentFlags().StringVar(&stateDirFlag, "state-dir", "", "State directory (default is ~/.local/share/shrine or /var/lib/shrine)")
}
