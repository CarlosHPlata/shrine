package planner

import (
	"sort"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// PlannedStep represents a single unit of execution in the deployment plan.
type PlannedStep struct {
	Kind string // Resource or Application
	Name string
}

// Order computes the linear execution order for a set of manifests.
// Currently, dependencies only flow from Applications to Resources, so the rule
// is simple: create all Resources first, then all Applications.
//
// NOTE: This implementation does not currently perform a full topological sort
// as internal Resource-to-Resource dependencies are not yet supported. When that
// functionality is added, this function should be updated to use a graph-based
// sorting algorithm.
func Order(set *ManifestSet) []PlannedStep {
	var steps []PlannedStep

	// 1. Resources First
	// We sort names alphabetically to ensure a deterministic deployment plan.
	resourceNames := make([]string, 0, len(set.Resources))
	for name := range set.Resources {
		resourceNames = append(resourceNames, name)
	}
	sort.Strings(resourceNames)
	steps = appendSteps(steps, resourceNames, manifest.ResourceKind)

	// 2. Applications Second
	// We sort names alphabetically to ensure a deterministic deployment plan.
	appNames := make([]string, 0, len(set.Applications))
	for name := range set.Applications {
		appNames = append(appNames, name)
	}
	sort.Strings(appNames)
	steps = appendSteps(steps, appNames, manifest.ApplicationKind)

	return steps
}

func appendSteps(steps []PlannedStep, names []string, kind string) []PlannedStep {
	for _, name := range names {
		steps = append(steps, PlannedStep{
			Kind: kind,
			Name: name,
		})
	}
	return steps
}
