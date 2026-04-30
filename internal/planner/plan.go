package planner

import (
	"fmt"
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

	steps, err := Order(set)
	if err != nil {
		return PlanResult{Error: err}
	}

	return PlanResult{Steps: steps, ManifestSet: set}
}

// PlanSingle plans the deployment of a single manifest file.
// If specsDir is non-empty it loads the full directory as resolution context;
// otherwise a minimal ManifestSet containing only the parsed manifest is used.
func PlanSingle(file, specsDir string, store state.TeamStore) PlanResult {
	// Step 1: Parse and validate the target manifest.
	m, err := manifest.Parse(file)
	if err != nil {
		return PlanResult{Error: fmt.Errorf("parsing manifest %q: %w", file, err)}
	}
	if err := manifest.Validate(m); err != nil {
		return PlanResult{Error: fmt.Errorf("validating manifest %q: %w", file, err)}
	}

	// Derive the name from the concrete sub-manifest.
	var name string
	switch m.Kind {
	case manifest.ApplicationKind:
		name = m.Application.Metadata.Name
	case manifest.ResourceKind:
		name = m.Resource.Metadata.Name
	case manifest.TeamKind:
		return PlanResult{Error: fmt.Errorf("team manifests cannot be applied with --file; use 'shrine apply teams' instead")}
	default:
		return PlanResult{Error: fmt.Errorf("unsupported manifest kind %q for single-file apply", m.Kind)}
	}

	var set *ManifestSet

	if specsDir != "" {
		// Step 2a: Load the full directory for dependency resolution context.
		set, err = LoadDir(specsDir)
		if err != nil {
			return PlanResult{Error: err}
		}

		// Add the target manifest to the set if it is not already present.
		alreadyPresent := false
		switch m.Kind {
		case manifest.ApplicationKind:
			_, alreadyPresent = set.Applications[name]
		case manifest.ResourceKind:
			_, alreadyPresent = set.Resources[name]
		}

		if !alreadyPresent {
			if err := set.mapKind(m, file); err != nil {
				return PlanResult{Error: err}
			}
		}
	} else {
		// Step 2b: Minimal set — only the single manifest.
		set = &ManifestSet{
			Applications: make(map[string]*manifest.ApplicationManifest),
			Resources:    make(map[string]*manifest.ResourceManifest),
		}
		if err := set.mapKind(m, file); err != nil {
			return PlanResult{Error: err}
		}
	}

	// Step 3: Resolve dependencies, quotas, and access control.
	if errs := Resolve(set, store); len(errs) > 0 {
		return PlanResult{ValidationErr: errs}
	}

	// Step 4: Return a single-step plan; ManifestSet is carried for callers that
	// need the resolution context (e.g. value injection).
	return PlanResult{
		Steps:       []PlannedStep{{Kind: m.Kind, Name: name}},
		ManifestSet: set,
	}
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
