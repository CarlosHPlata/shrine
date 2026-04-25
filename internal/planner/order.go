package planner

import (
	"fmt"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/topo"
)

// PlannedStep represents a single unit of execution in the deployment plan.
type PlannedStep struct {
	Kind string // Resource or Application
	Name string
}

// Order computes the linear execution order for a set of manifests
// using a topological sort to respect dependencies.
func Order(set *ManifestSet) ([]PlannedStep, error) {
	deps := make(map[string]map[string]struct{})

	// Pass 1: Resources (leaf nodes - no outgoing deps)
	for name := range set.Resources {
		key := manifest.ResourceKind + ":" + name
		deps[key] = make(map[string]struct{})
	}

	// Pass 2: Applications (may have deps)
	for name, app := range set.Applications {
		key := manifest.ApplicationKind + ":" + name
		d := make(map[string]struct{})
		for _, dep := range app.Spec.Dependencies {
			d[dep.Kind+":"+dep.Name] = struct{}{}
		}
		deps[key] = d
	}

	order, err := topo.Sort(deps)
	if err != nil {
		return nil, fmt.Errorf("dependency cycle in deployment plan: %w", err)
	}

	var steps []PlannedStep
	for _, key := range order {
		parts := strings.SplitN(key, ":", 2)
		steps = append(steps, PlannedStep{
			Kind: parts[0],
			Name: parts[1],
		})
	}

	return steps, nil
}
