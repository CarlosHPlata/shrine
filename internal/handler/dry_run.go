package handler

import (
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/planner"
	"github.com/CarlosHPlata/shrine/internal/state"
)

func DryRun(out io.Writer, manifestDir string, store state.TeamStore) error {
	result := planner.Plan(manifestDir, store)

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

	engine := engine.NewDryRunEngine(out)

	if err := engine.ExecuteDeploy(result.Steps, result.ManifestSet); err != nil {
		return err
	}

	return nil
}
