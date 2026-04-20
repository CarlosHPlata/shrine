package handler

import (
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine/dryrun"
	"github.com/CarlosHPlata/shrine/internal/engine/local"
	"github.com/CarlosHPlata/shrine/internal/planner"
	"github.com/CarlosHPlata/shrine/internal/state"
)

func DryRun(out io.Writer, manifestDir string, store *state.Store) error {
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

	engine := dryrun.NewDryRunEngine(out)

	if err := engine.ExecuteDeploy(result.Steps, result.ManifestSet); err != nil {
		return err
	}

	return nil
}

func Deploy(out io.Writer, manifestDir string, store *state.Store, cfg *config.Config) error {
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

	engine, err := local.NewLocalEngine(store, cfg.Registries)
	if err != nil {
		return err
	}

	if err := engine.ExecuteDeploy(result.Steps, result.ManifestSet); err != nil {
		return err
	}

	return nil

}
