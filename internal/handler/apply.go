package handler

import (
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/engine/local"
	"github.com/CarlosHPlata/shrine/internal/planner"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/CarlosHPlata/shrine/internal/ui"
)

// ApplySingleOptions bundles the inputs needed to apply a single manifest file.
type ApplySingleOptions struct {
	Out    io.Writer
	File   string
	Store  *state.Store
	Config *config.Config
	Paths  *config.Paths
}

func ApplySingle(opts ApplySingleOptions) error {
	specsDir, _ := opts.Config.ResolveSpecsDir("")

	result := planner.PlanSingle(opts.File, specsDir, opts.Store.Teams)

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

	terminal := ui.NewTerminalObserver(opts.Out)
	fileLogger, err := ui.NewFileLogger(opts.Paths.StateDir)
	if err != nil {
		return fmt.Errorf("initializing file logger: %w", err)
	}
	defer fileLogger.Close()

	observer := engine.MultiObserver{terminal, fileLogger}

	deployEngine, err := local.NewLocalEngine(opts.Store, opts.Config.Registries, observer)
	if err != nil {
		return err
	}

	if err := deployEngine.ExecuteDeploy(result.Steps, result.ManifestSet); err != nil {
		return err
	}

	return nil
}
