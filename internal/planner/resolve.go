package planner

import (
	"fmt"
	"slices"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

// Resolve performs dependency resolution, access control checks, and quota enforcement
// for a set of manifests. It returns a slice of all validation errors found.
func Resolve(set *ManifestSet, store state.TeamStore) []error {
	var errs []error

	// Track counts per owner for quota enforcement
	appCounts := make(map[string]int)
	resCounts := make(map[string]int)

	// 1. Resolve dependencies and check access
	for _, app := range set.Applications {
		appCounts[app.Metadata.Owner]++
		errs = append(errs, resolveDependencies(set, app)...)
		errs = append(errs, validateValueFrom(set, app)...)
	}

	// 2. Resource-specific checks (counts and types)
	for _, res := range set.Resources {
		resCounts[res.Metadata.Owner]++
	}

	// 3. Quota enforcement
	teamCache, quotaErrs := enforceQuota(store, appCounts, resCounts)
	errs = append(errs, quotaErrs...)

	// 4. Check AllowedResourceTypes
	errs = append(errs, allowedResourceTypes(set, teamCache)...)

	return errs
}

// resolveDependencies resolves dependencies and performs access checks for a single Application.
func resolveDependencies(set *ManifestSet, app *manifest.ApplicationManifest) []error {
	var errs []error
	for _, dep := range app.Spec.Dependencies {
		// Currently we only support Resource dependencies
		if dep.Kind != "Resource" {
			errs = append(errs, fmt.Errorf("app %q: unsupported dependency kind %q (only Resource is supported)", app.Metadata.Name, dep.Kind))
			continue
		}

		res, exists := set.Resources[dep.Name]
		if !exists {
			errs = append(errs, fmt.Errorf("app %q: depends on missing resource %q", app.Metadata.Name, dep.Name))
			continue
		}

		// Verify owner matches
		if res.Metadata.Owner != dep.Owner {
			errs = append(errs, fmt.Errorf("app %q: depends on resource %q owned by %q, but manifest specifies owner %q",
				app.Metadata.Name, dep.Name, res.Metadata.Owner, dep.Owner))
			continue
		}

		// Access check
		if !hasAccess(app.Metadata.Owner, res) {
			errs = append(errs, fmt.Errorf("app %q (team %q) does not have access to resource %q (owned by %q)",
				app.Metadata.Name, app.Metadata.Owner, res.Metadata.Name, res.Metadata.Owner))
		}
	}
	return errs
}

// enforceQuota registers all involved owners, loads their team metadata, and checks
// deployment counts against defined quotas. It returns a cache of loaded teams and any errors.
func enforceQuota(store state.TeamStore, appCounts, resCounts map[string]int) (map[string]*manifest.TeamManifest, []error) {
	var errs []error
	teamCache := make(map[string]*manifest.TeamManifest)
	owners := make(map[string]struct{})
	for owner := range appCounts {
		owners[owner] = struct{}{}
	}
	for owner := range resCounts {
		owners[owner] = struct{}{}
	}

	for owner := range owners {
		team, err := store.LoadTeam(owner)
		if err != nil {
			errs = append(errs, fmt.Errorf("team %q: failed to load for quota check: %w", owner, err))
			continue
		}
		teamCache[owner] = team

		// Check MaxApps
		if team.Spec.Quotas.MaxApps > 0 && appCounts[owner] > team.Spec.Quotas.MaxApps {
			errs = append(errs, fmt.Errorf("team %q: deployment exceeds MaxApps quota (deploying %d, limit %d)",
				owner, appCounts[owner], team.Spec.Quotas.MaxApps))
		}

		// Check MaxResources
		if team.Spec.Quotas.MaxResources > 0 && resCounts[owner] > team.Spec.Quotas.MaxResources {
			errs = append(errs, fmt.Errorf("team %q: deployment exceeds MaxResources quota (deploying %d, limit %d)",
				owner, resCounts[owner], team.Spec.Quotas.MaxResources))
		}
	}
	return teamCache, errs
}

// allowedResourceTypes validates that all resources in the set use types allowed by their
// respective team quotas.
func allowedResourceTypes(set *ManifestSet, teamCache map[string]*manifest.TeamManifest) []error {
	var errs []error
	for _, res := range set.Resources {
		owner := res.Metadata.Owner
		team, ok := teamCache[owner]
		if !ok || len(team.Spec.Quotas.AllowedResourceTypes) == 0 {
			continue
		}

		if !slices.Contains(team.Spec.Quotas.AllowedResourceTypes, res.Spec.Type) {
			errs = append(errs, fmt.Errorf("team %q: resource type %q (on %q) is not allowed by quota",
				owner, res.Spec.Type, res.Metadata.Name))
		}
	}
	return errs
}

// hasAccess returns true if the consumer has access to the resource.
// Access is granted if the consumer is the owner or if the consumer is in the access list.
func hasAccess(consumer string, res *manifest.ResourceManifest) bool {
	if res.Metadata.Owner == consumer {
		return true
	}

	return slices.Contains(res.Metadata.Access, consumer)
}

// validateValueFrom ensures all environment variables using valueFrom reference valid
// resource outputs in the format 'resource.<name>.<output>'.
func validateValueFrom(set *ManifestSet, app *manifest.ApplicationManifest) []error {
	var errs []error
	for _, env := range app.Spec.Env {
		// Env vars must have either value or valueFrom, but not both.
		if env.Value != "" && env.ValueFrom != "" {
			errs = append(errs, fmt.Errorf("app %q: env %q has both value and valueFrom set", app.Metadata.Name, env.Name))
			continue
		}
		if env.Value == "" && env.ValueFrom == "" {
			errs = append(errs, fmt.Errorf("app %q: env %q must have either value or valueFrom set", app.Metadata.Name, env.Name))
			continue
		}

		if env.ValueFrom == "" {
			continue
		}

		// validate valuefrom format matches resource.<name>.<output>
		parts := strings.Split(env.ValueFrom, ".")
		if len(parts) != 3 || parts[0] != "resource" {
			errs = append(errs, fmt.Errorf("app %q: env %q has invalid valueFrom format %q (expected resource.<name>.<output>)",
				app.Metadata.Name, env.Name, env.ValueFrom))
			continue
		}

		// resouce.parts[1].parts[2]
		resName := parts[1]
		outName := parts[2]

		// if resource is not found, add error
		res, exists := set.Resources[resName]
		if !exists {
			errs = append(errs, fmt.Errorf("app %q: env %q references missing resource %q",
				app.Metadata.Name, env.Name, resName))
			continue
		}

		// if output is not found, add error
		found := false
		for _, out := range res.Spec.Outputs {
			if out.Name == outName {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, fmt.Errorf("app %q: env %q references non-existent output %q on resource %q",
				app.Metadata.Name, env.Name, outName, resName))
		}
	}
	return errs
}
