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

	// 0. Name collision check
	errs = append(errs, validateUniqueNames(set)...)

	// Track counts per owner for quota enforcement
	appCounts := make(map[string]int)
	resCounts := make(map[string]int)

	// 1. Resolve dependencies and check access
	for _, app := range set.Applications {
		appCounts[app.Metadata.Owner]++
		errs = append(errs, resolveDependencies(set, app)...)
		errs = append(errs, validateValueFrom(set, app)...)
		errs = append(errs, validateEnvTemplates(app)...)
	}

	// 2. Resource-specific checks (counts, template refs)
	for _, res := range set.Resources {
		resCounts[res.Metadata.Owner]++
		errs = append(errs, validateTemplates(res)...)
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
		switch dep.Kind {
		case manifest.ResourceKind:
			errs = append(errs, validateResourceDep(set, app, dep)...)
		case manifest.ApplicationKind:
			errs = append(errs, validateApplicationDep(set, app, dep)...)
		default:
			errs = append(errs, fmt.Errorf("app %q: unsupported dependency kind %q", app.Metadata.Name, dep.Kind))
		}
	}
	return errs
}

func validateResourceDep(set *ManifestSet, app *manifest.ApplicationManifest, dep manifest.Dependency) []error {
	var errs []error
	res, exists := set.Resources[dep.Name]
	if !exists {
		errs = append(errs, fmt.Errorf("app %q: depends on missing resource %q", app.Metadata.Name, dep.Name))
		return errs
	}

	// Verify owner matches
	if res.Metadata.Owner != dep.Owner {
		errs = append(errs, fmt.Errorf("app %q: depends on resource %q owned by %q, but manifest specifies owner %q",
			app.Metadata.Name, dep.Name, res.Metadata.Owner, dep.Owner))
		return errs
	}

	// Access check
	if !hasAccess(app.Metadata.Owner, res.Metadata.Owner, res.Metadata.Access) {
		errs = append(errs, fmt.Errorf("app %q (team %q) does not have access to resource %q (owned by %q)",
			app.Metadata.Name, app.Metadata.Owner, res.Metadata.Name, res.Metadata.Owner))
	}

	// Reachability check
	if app.Metadata.Owner != res.Metadata.Owner && !res.Spec.Networking.ExposeToPlatform {
		errs = append(errs, fmt.Errorf("app %q (team %q): resource %q (team %q) is not reachable cross-team — set networking.exposeToPlatform: true on the resource",
			app.Metadata.Name, app.Metadata.Owner, res.Metadata.Name, res.Metadata.Owner))
	}

	return errs
}

func validateApplicationDep(set *ManifestSet, app *manifest.ApplicationManifest, dep manifest.Dependency) []error {
	var errs []error
	depApp, exists := set.Applications[dep.Name]
	if !exists {
		errs = append(errs, fmt.Errorf("app %q: depends on missing application %q", app.Metadata.Name, dep.Name))
		return errs
	}

	// Verify owner matches
	if depApp.Metadata.Owner != dep.Owner {
		errs = append(errs, fmt.Errorf("app %q: depends on application %q owned by %q, but manifest specifies owner %q",
			app.Metadata.Name, dep.Name, depApp.Metadata.Owner, dep.Owner))
		return errs
	}

	// Access check
	if !hasAccess(app.Metadata.Owner, depApp.Metadata.Owner, depApp.Metadata.Access) {
		errs = append(errs, fmt.Errorf("app %q (team %q) does not have access to application %q (owned by %q)",
			app.Metadata.Name, app.Metadata.Owner, depApp.Metadata.Name, depApp.Metadata.Owner))
	}

	// Reachability check
	if app.Metadata.Owner != depApp.Metadata.Owner && !depApp.Spec.Networking.ExposeToPlatform {
		errs = append(errs, fmt.Errorf("app %q (team %q): application %q (team %q) is not reachable cross-team — set networking.exposeToPlatform: true on the application",
			app.Metadata.Name, app.Metadata.Owner, depApp.Metadata.Name, depApp.Metadata.Owner))
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
func hasAccess(consumer string, owner string, accessList []string) bool {
	if owner == consumer {
		return true
	}

	return slices.Contains(accessList, consumer)
}

// validateValueFrom ensures all environment variables using valueFrom reference valid
// resource outputs in the format 'resource.<name>.<output>'. Presence and mutual
// exclusivity of value/valueFrom are enforced earlier by manifest.Validate.
func validateValueFrom(set *ManifestSet, app *manifest.ApplicationManifest) []error {
	var errs []error
	for _, env := range app.Spec.Env {
		if env.ValueFrom == "" {
			continue
		}

		parts := strings.Split(env.ValueFrom, ".")
		if len(parts) != 3 {
			errs = append(errs, fmt.Errorf("app %q: env %q has invalid valueFrom format %q (expected <kind>.<name>.<output>)",
				app.Metadata.Name, env.Name, env.ValueFrom))
			continue
		}

		kind := parts[0]
		name := parts[1]
		output := parts[2]

		switch kind {
		case "resource":
			res, exists := set.Resources[name]
			if !exists {
				errs = append(errs, fmt.Errorf("app %q: env %q references missing resource %q",
					app.Metadata.Name, env.Name, name))
				continue
			}

			found := false
			for _, out := range res.Spec.Outputs {
				if out.Name == output {
					found = true
					break
				}
			}
			if !found {
				errs = append(errs, fmt.Errorf("app %q: env %q references non-existent output %q on resource %q",
					app.Metadata.Name, env.Name, output, name))
			}

		case "application":
			_, exists := set.Applications[name]
			if !exists {
				errs = append(errs, fmt.Errorf("app %q: env %q references missing application %q",
					app.Metadata.Name, env.Name, name))
				continue
			}
			if output != "host" && output != "port" {
				errs = append(errs, fmt.Errorf("app %q: env %q: application %q has no built-in output %q (only host and port are supported)",
					app.Metadata.Name, env.Name, name, output))
			}

		default:
			errs = append(errs, fmt.Errorf("app %q: env %q has invalid valueFrom format %q (expected resource.<name>.<output> or application.<name>.<built-in>)",
				app.Metadata.Name, env.Name, env.ValueFrom))
		}
	}
	return errs
}

func validateUniqueNames(set *ManifestSet) []error {
	var errs []error
	// Per user instruction: an Application and a Resource CAN share the same name.
	// Within-kind uniqueness (e.g., no two Applications with the same name) is
	// implicitly enforced by the ManifestSet map structure.
	// We check that the metadata names match their map keys to ensure integrity.
	for name, app := range set.Applications {
		if app.Metadata.Name != name {
			errs = append(errs, fmt.Errorf("application %q has metadata name mismatch: %q", name, app.Metadata.Name))
		}
	}
	for name, res := range set.Resources {
		if res.Metadata.Name != name {
			errs = append(errs, fmt.Errorf("resource %q has metadata name mismatch: %q", name, res.Metadata.Name))
		}
	}
	return errs
}
