package handler

import (
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/engine/dryrun"
	"github.com/CarlosHPlata/shrine/internal/engine/local"
	"github.com/CarlosHPlata/shrine/internal/planner"
	"github.com/CarlosHPlata/shrine/internal/plugins/gateway/traefik"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/CarlosHPlata/shrine/internal/ui"
)

// DeployOptions bundles the inputs needed to run a deployment.
type DeployOptions struct {
	Out         io.Writer
	ManifestDir string
	Store       *state.Store
	Config      *config.Config
	Paths       *config.Paths
}

// DryRun runs a dry-run deploy. When cfg is non-nil the Traefik plugin is
// constructed (which validates its config); the dry-run engine prints route
// operations instead of writing files.
func DryRun(out io.Writer, manifestDir string, store *state.Store, cfg *config.Config) error {
	if cfg != nil {
		if _, err := traefik.New(cfg.Plugins.Gateway.Traefik, nil, "", nil); err != nil {
			return err
		}
	}

	result := planner.Plan(manifestDir, store.Teams)

	if result.Error != nil {
		return result.Error
	}

	if len(result.ValidationErr) > 0 {
		fmt.Fprintln(out, "Validation errors:")
		for _, err := range result.ValidationErr {
			fmt.Fprintln(out, err)
		}
		return fmt.Errorf("Spec validation errors")
	}

	if len(result.Steps) == 0 {
		fmt.Fprintln(out, "No steps generated.")
		return nil
	}

	engineInst := dryrun.NewDryRunEngine(out)
	if err := engineInst.ExecuteDeploy(result.Steps, result.ManifestSet); err != nil {
		return err
	}

	return nil
}

func Deploy(opts DeployOptions) error {
	specsDir, _ := opts.Config.ResolveSpecsDir(opts.ManifestDir)

	terminal := ui.NewTerminalObserver(opts.Out)
	fileLogger, err := ui.NewFileLogger(opts.Paths.StateDir)
	if err != nil {
		return fmt.Errorf("initializing file logger: %w", err)
	}
	defer fileLogger.Close()

	observer := engine.MultiObserver{terminal, fileLogger}

	containerBackend, err := local.NewContainerBackend(opts.Store, opts.Config.Registries, observer)
	if err != nil {
		return err
	}

	plugin, err := traefik.New(opts.Config.Plugins.Gateway.Traefik, containerBackend, specsDir, observer)
	if err != nil {
		return err
	}

	result := planner.Plan(opts.ManifestDir, opts.Store.Teams)

	if result.Error != nil {
		return result.Error
	}

	if len(result.ValidationErr) > 0 {
		fmt.Fprintln(opts.Out, "Validation errors:")
		for _, err := range result.ValidationErr {
			fmt.Fprintln(opts.Out, err)
		}
		return fmt.Errorf("Spec validation errors")
	}

	if len(result.Steps) == 0 {
		fmt.Fprintln(opts.Out, "No steps generated.")
		return nil
	}

	var routing engine.RoutingBackend
	if plugin.IsActive() {
		if routing, err = plugin.RoutingBackend(); err != nil {
			return err
		}
	}

	deployEngine, err := local.NewLocalEngineWithRouting(opts.Store, opts.Config.Registries, observer, routing)
	if err != nil {
		return err
	}

	if err := deployEngine.ExecuteDeploy(result.Steps, result.ManifestSet); err != nil {
		return err
	}

	if plugin.IsActive() {
		if err := plugin.Deploy(); err != nil {
			return err
		}
	}

	return nil
}
