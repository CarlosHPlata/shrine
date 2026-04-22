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

type TeardownOptions struct {
	Out    io.Writer
	Store  *state.Store
	Team   string
	Paths  *config.Paths
	Config *config.Config
}

func Teardown(opts TeardownOptions) error {
	result := planner.PlanTeardown(opts.Team, opts.Store.Deployments)
	if result.Error != nil {
		return result.Error
	}

	terminal := ui.NewTerminalObserver(opts.Out)
	fileLogger, err := ui.NewFileLogger(opts.Paths.StateDir)
	if err != nil {
		return fmt.Errorf("initializing file logger: %w", err)
	}
	defer fileLogger.Close()

	observer := engine.MultiObserver{terminal, fileLogger}

	localEngine, err := local.NewLocalEngine(opts.Store, opts.Config.Registries, observer)
	if err != nil {
		return err
	}

	if err := localEngine.ExecuteTeardown(opts.Team, result.Steps); err != nil {
		return err
	}

	return nil
}
