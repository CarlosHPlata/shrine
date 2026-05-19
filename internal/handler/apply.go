package handler

import (
	"errors"
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/app"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/planner"
)

func ApplySingle(b *app.ApplyBundle, file, manifestDir string) error {
	m, err := manifest.Parse(file)
	if err != nil {
		return fmt.Errorf("parsing manifest %q: %w", file, err)
	}
	if err := manifest.Validate(m); err != nil {
		return fmt.Errorf("validating manifest %q: %w", file, err)
	}

	filter, err := filterForSingle(m)
	if err != nil {
		return err
	}

	set, err := loadSetForSingle(file, manifestDir, m)
	if err != nil {
		return err
	}

	result := planner.Plan(set, b.Store.Teams, b.Cfg.Registries, filter)

	if result.Error != nil {
		return result.Error
	}

	if len(result.ValidationErr) > 0 {
		fmt.Fprintln(b.ErrOut, "Validation errors:")
		for _, err := range result.ValidationErr {
			fmt.Fprintln(b.ErrOut, err)
		}
		return fmt.Errorf("Spec validation errors")
	}

	if err := b.Engine.ExecuteDeploy(result.Steps, result.ManifestSet); err != nil {
		return err
	}

	return nil
}

func filterForSingle(m *manifest.Manifest) (planner.Filter, error) {
	switch m.Kind {
	case manifest.ApplicationKind:
		return planner.ByApp(m.Application.Metadata.Name), nil
	case manifest.ResourceKind:
		return planner.ByResource(m.Resource.Metadata.Name), nil
	case manifest.TeamKind:
		return planner.Filter{}, fmt.Errorf("team manifests cannot be applied with --file; use 'shrine apply teams' instead")
	default:
		return planner.Filter{}, fmt.Errorf("unsupported manifest kind %q for single-file apply", m.Kind)
	}
}

func loadSetForSingle(file, manifestDir string, m *manifest.Manifest) (*planner.ManifestSet, error) {
	if manifestDir == "" {
		set := planner.NewManifestSet()
		if err := set.MergeManifest(m, file); err != nil {
			return nil, err
		}
		return set, nil
	}

	set, err := planner.LoadDir(manifestDir)
	if err != nil {
		return nil, err
	}
	if err := set.MergeManifest(m, file); err != nil && !errors.Is(err, planner.ErrDuplicateManifest) {
		return nil, err
	}
	return set, nil
}
