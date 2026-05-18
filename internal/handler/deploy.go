package handler

import (
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/app"
	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine/dryrun"
	"github.com/CarlosHPlata/shrine/internal/planner"
	"github.com/CarlosHPlata/shrine/internal/state"
)

// DryRun runs a dry-run deploy. When cfg is non-nil, registries and the
// Traefik config are validated; the dry-run engine prints route operations
// instead of writing files.
func DryRun(out io.Writer, manifestDir string, store *state.Store, cfg *config.Config) error {
	if cfg != nil {
		if err := cfg.ValidateRegistries(); err != nil {
			return err
		}
		if err := app.ValidateTraefikConfig(cfg); err != nil {
			return err
		}
	}

	result := planner.Plan(manifestDir, store.Teams, cfg.Registries)

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

func Deploy(b *app.DeployBundle, manifestDir string) error {
	result := planner.Plan(manifestDir, b.Store.Teams, b.Cfg.Registries)

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

	if len(result.Steps) == 0 {
		fmt.Fprintln(b.Out, "No steps generated.")
		return nil
	}

	if err := b.Engine.ExecuteDeploy(result.Steps, result.ManifestSet); err != nil {
		return err
	}

	return nil
}
