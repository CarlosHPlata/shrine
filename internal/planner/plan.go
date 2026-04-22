package planner

import (
	"sort"

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

func Plan(dir string, store state.TeamStore) PlanResult {
	set, err := LoadDir(dir)
	if err != nil {
		return PlanResult{Error: err}
	}

	if errs := Resolve(set, store); len(errs) > 0 {
		return PlanResult{ValidationErr: errs}
	}

	return PlanResult{Steps: Order(set), ManifestSet: set}
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
