package handler

import (
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/app"
	"github.com/CarlosHPlata/shrine/internal/planner"
)

func ApplySingle(b *app.ApplyBundle, file, manifestDir string) error {
	result := planner.PlanSingle(file, manifestDir, b.Store.Teams, b.Cfg.Registries)

	if result.Error != nil {
		return result.Error
	}

	if len(result.ValidationErr) > 0 {
		fmt.Fprintln(b.Out, "Validation errors:")
		for _, err := range result.ValidationErr {
			fmt.Fprintln(b.Out, err)
		}
		return fmt.Errorf("Spec validation errors")
	}

	if err := b.Engine.ExecuteDeploy(result.Steps, result.ManifestSet); err != nil {
		return err
	}

	return nil
}
