package handler

import (
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/planner"
	"github.com/CarlosHPlata/shrine/internal/state"
)

func DryRun(manifestDir string, store state.TeamStore) error {
	result := planner.Plan(manifestDir, store)

	if result.Error != nil {
		return result.Error
	}

	if len(result.ValidationErr) > 0 {
		fmt.Println("Validation errors:")
		for _, err := range result.ValidationErr {
			fmt.Println(err)
		}
		return fmt.Errorf("Spec validation errors")
	}

	if len(result.Steps) == 0 {
		fmt.Println("No steps generated.")
		return nil
	}

	engine := engine.NewDryRunEngine()

	if err := engine.ExecuteDeploy(result.Steps, result.ManifestSet); err != nil {
		return err
	}

	return nil
}
