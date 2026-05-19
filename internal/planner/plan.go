package planner

import (
	"sort"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

type PlanResult struct {
	Steps         []PlannedStep
	ManifestSet   *ManifestSet
	Error         error
	ValidationErr []error
}

type PlanTeardownResult struct {
	Steps []PlannedStep
	Error error
}

// Plan is the single planning entry point. It walks a pre-loaded ManifestSet
// and emits the deploy steps selected by filter.
//
// Loading is the caller's job: use LoadDir for a full directory, or
// NewManifestSet + MergeManifest to assemble a set from individual files.
func Plan(set *ManifestSet, store state.TeamStore, registries []config.RegistryConfig, filter Filter) PlanResult {
	if err := filter.Validate(set); err != nil {
		return PlanResult{Error: err}
	}

	if errs := Resolve(set, store, registries); len(errs) > 0 {
		return PlanResult{ValidationErr: errs}
	}

	switch filter.Kind {
	case FilterNone, FilterTeam:
		if err := DetectRoutingCollisions(set); err != nil {
			return PlanResult{Error: err}
		}
		steps, err := Order(set)
		if err != nil {
			return PlanResult{Error: err}
		}
		if filter.Kind == FilterTeam {
			steps = filterStepsByOwner(steps, set, filter.Name)
		}
		return PlanResult{Steps: steps, ManifestSet: set}

	case FilterApp:
		return PlanResult{
			Steps:       []PlannedStep{{Kind: manifest.ApplicationKind, Name: filter.Name}},
			ManifestSet: set,
		}

	case FilterRes:
		return PlanResult{
			Steps:       []PlannedStep{{Kind: manifest.ResourceKind, Name: filter.Name}},
			ManifestSet: set,
		}
	}

	// Unreachable: Validate already rejected unknown kinds.
	return PlanResult{}
}

func filterStepsByOwner(steps []PlannedStep, set *ManifestSet, owner string) []PlannedStep {
	out := make([]PlannedStep, 0, len(steps))
	for _, step := range steps {
		if stepOwner(set, step) == owner {
			out = append(out, step)
		}
	}
	return out
}

func stepOwner(set *ManifestSet, step PlannedStep) string {
	switch step.Kind {
	case manifest.ApplicationKind:
		if app, ok := set.Applications[step.Name]; ok {
			return app.Metadata.Owner
		}
	case manifest.ResourceKind:
		if res, ok := set.Resources[step.Name]; ok {
			return res.Metadata.Owner
		}
	}
	return ""
}

func PlanTeardown(team string, store state.DeploymentStore) PlanTeardownResult {
	deployments, err := store.List(team)
	if err != nil {
		return PlanTeardownResult{Error: err}
	}

	var apps, resources []PlannedStep
	for _, d := range deployments {
		step := PlannedStep{Kind: d.Kind, Name: d.Name}
		switch d.Kind {
		case manifest.ApplicationKind:
			apps = append(apps, step)
		case manifest.ResourceKind:
			resources = append(resources, step)
		}
	}

	sort.Slice(apps, func(i, j int) bool { return apps[i].Name < apps[j].Name })
	sort.Slice(resources, func(i, j int) bool { return resources[i].Name < resources[j].Name })

	return PlanTeardownResult{Steps: append(apps, resources...)}
}
