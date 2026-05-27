package planner

import (
	"fmt"
	"slices"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

// Resolve performs dependency resolution, access control checks, quota enforcement,
// and registry alias validation for a set of manifests. It returns all errors found.
func Resolve(set *ManifestSet, store state.TeamStore, registries []config.RegistryConfig) []error {
	var errs []error

	// 0. Name collision check
	errs = append(errs, validateUniqueNames(set)...)

	// Track counts per owner for quota enforcement
	appCounts := make(map[string]int)
	resCounts := make(map[string]int)

	// 1. Resolve dependencies and check access
	for _, app := range set.Applications {
		appCounts[app.Metadata.Owner]++
		errs = append(errs, resolveDependencies(set, appConsumer(app), app.Spec.Dependencies)...)
		errs = append(errs, validateEnvValueFrom(set, "app", app.Metadata.Name, app.Spec.Env)...)
		errs = append(errs, validateEnvTemplates(app)...)
	}

	// 2. Resource checks (counts, deps as a consumer, env valueFrom, templates)
	for _, res := range set.Resources {
		resCounts[res.Metadata.Owner]++
		errs = append(errs, resolveDependencies(set, resourceConsumer(res), res.Spec.Dependencies)...)
		errs = append(errs, validateEnvValueFrom(set, "resource", res.Metadata.Name, res.Spec.Env)...)
		errs = append(errs, validateTemplates(res)...)
		errs = append(errs, validateResourceEnvTemplates(res)...)
	}

	// 3. Quota enforcement
	teamCache, quotaErrs := enforceQuota(store, appCounts, resCounts)
	errs = append(errs, quotaErrs...)

	// 4. Check AllowedResourceTypes
	errs = append(errs, allowedResourceTypes(set, teamCache)...)

	// 5. Validate registry aliases in image references
	errs = append(errs, validateRegistryImages(set, registries)...)

	return errs
}

// validateRegistryImages checks that every image field using the reg:<alias> prefix
// references an alias defined in the provided registry config.
func validateRegistryImages(set *ManifestSet, registries []config.RegistryConfig) []error {
	aliasMap := buildRegistryAliasMap(registries)
	var errs []error

	for _, app := range set.Applications {
		if err := checkImageAlias(app.Spec.Image, "app", app.Metadata.Name, aliasMap); err != nil {
			errs = append(errs, err)
		}
	}
	for _, res := range set.Resources {
		if res.Spec.Image == "" {
			continue
		}
		if err := checkImageAlias(res.Spec.Image, "resource", res.Metadata.Name, aliasMap); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func checkImageAlias(image, kind, name string, aliasMap map[string]string) error {
	if !strings.HasPrefix(image, "reg:") {
		return nil
	}
	rest := image[len("reg:"):]
	slash := strings.Index(rest, "/")
	var alias string
	if slash == -1 {
		alias = rest
	} else {
		alias = rest[:slash]
	}
	if alias == "" {
		return fmt.Errorf(`%s %q: image %q: alias name must not be empty`, kind, name, image)
	}
	if _, ok := aliasMap[alias]; !ok {
		return fmt.Errorf(`%s %q: image %q: alias %q is not defined in config registries`, kind, name, image, alias)
	}
	return nil
}

func buildRegistryAliasMap(registries []config.RegistryConfig) map[string]string {
	m := make(map[string]string, len(registries))
	for _, r := range registries {
		if r.Alias != "" {
			m[r.Alias] = r.Host
		}
	}
	return m
}

// consumer identifies a manifest that declares dependencies / valueFrom refs.
// kindLabel is the operator-facing prefix used in error messages ("app" or
// "resource").
type consumer struct {
	kindLabel string
	name      string
	owner     string
}

func appConsumer(a *manifest.ApplicationManifest) consumer {
	return consumer{kindLabel: "app", name: a.Metadata.Name, owner: a.Metadata.Owner}
}

func resourceConsumer(r *manifest.ResourceManifest) consumer {
	return consumer{kindLabel: "resource", name: r.Metadata.Name, owner: r.Metadata.Owner}
}

// resolveDependencies resolves declared dependencies and performs access /
// reachability checks for a single consumer (Application or Resource).
func resolveDependencies(set *ManifestSet, c consumer, deps []manifest.Dependency) []error {
	var errs []error
	for _, dep := range deps {
		switch dep.Kind {
		case manifest.ResourceKind:
			errs = append(errs, validateResourceDep(set, c, dep)...)
		case manifest.ApplicationKind:
			errs = append(errs, validateApplicationDep(set, c, dep)...)
		default:
			errs = append(errs, fmt.Errorf("%s %q: unsupported dependency kind %q", c.kindLabel, c.name, dep.Kind))
		}
	}
	return errs
}

func validateResourceDep(set *ManifestSet, c consumer, dep manifest.Dependency) []error {
	var errs []error
	res, exists := set.Resources[dep.Name]
	if !exists {
		errs = append(errs, fmt.Errorf("%s %q: depends on missing resource %q", c.kindLabel, c.name, dep.Name))
		return errs
	}

	// Verify owner matches
	if res.Metadata.Owner != dep.Owner {
		errs = append(errs, fmt.Errorf("%s %q: depends on resource %q owned by %q, but manifest specifies owner %q",
			c.kindLabel, c.name, dep.Name, res.Metadata.Owner, dep.Owner))
		return errs
	}

	// Access check
	if !hasAccess(c.owner, res.Metadata.Owner, res.Metadata.Access) {
		errs = append(errs, fmt.Errorf("%s %q (team %q) does not have access to resource %q (owned by %q)",
			c.kindLabel, c.name, c.owner, res.Metadata.Name, res.Metadata.Owner))
	}

	// Reachability check
	if c.owner != res.Metadata.Owner && !res.Spec.Networking.ExposeToPlatform {
		errs = append(errs, fmt.Errorf("%s %q (team %q): resource %q (team %q) is not reachable cross-team — set networking.exposeToPlatform: true on the resource",
			c.kindLabel, c.name, c.owner, res.Metadata.Name, res.Metadata.Owner))
	}

	return errs
}

func validateApplicationDep(set *ManifestSet, c consumer, dep manifest.Dependency) []error {
	var errs []error
	depApp, exists := set.Applications[dep.Name]
	if !exists {
		errs = append(errs, fmt.Errorf("%s %q: depends on missing application %q", c.kindLabel, c.name, dep.Name))
		return errs
	}

	// Verify owner matches
	if depApp.Metadata.Owner != dep.Owner {
		errs = append(errs, fmt.Errorf("%s %q: depends on application %q owned by %q, but manifest specifies owner %q",
			c.kindLabel, c.name, dep.Name, depApp.Metadata.Owner, dep.Owner))
		return errs
	}

	// Access check
	if !hasAccess(c.owner, depApp.Metadata.Owner, depApp.Metadata.Access) {
		errs = append(errs, fmt.Errorf("%s %q (team %q) does not have access to application %q (owned by %q)",
			c.kindLabel, c.name, c.owner, depApp.Metadata.Name, depApp.Metadata.Owner))
	}

	// Reachability check
	if c.owner != depApp.Metadata.Owner && !depApp.Spec.Networking.ExposeToPlatform {
		errs = append(errs, fmt.Errorf("%s %q (team %q): application %q (team %q) is not reachable cross-team — set networking.exposeToPlatform: true on the application",
			c.kindLabel, c.name, c.owner, depApp.Metadata.Name, depApp.Metadata.Owner))
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

// validateEnvValueFrom ensures every env var using valueFrom references a valid
// exported resource output, an application built-in, or a well-formed vault
// path. A resource reference resolves only against the target's exported
// outputs (strict allowlist). It serves both Application and Resource consumers;
// kindLabel and consumerName scope the error messages. Presence and mutual
// exclusivity of value/valueFrom are enforced earlier by manifest.Validate.
func validateEnvValueFrom(set *ManifestSet, kindLabel, consumerName string, env []manifest.EnvVar) []error {
	var errs []error
	for _, e := range env {
		if e.ValueFrom == "" {
			continue
		}

		if strings.HasPrefix(e.ValueFrom, "vault:") {
			errs = append(errs, validateVaultPath(kindLabel, consumerName, "env", e.Name, e.ValueFrom)...)
			continue
		}

		parts := strings.Split(e.ValueFrom, ".")
		if len(parts) != 3 {
			errs = append(errs, fmt.Errorf("%s %q: env %q has invalid valueFrom format %q (expected <kind>.<name>.<output> or vault:<project>/<env>/<key>)",
				kindLabel, consumerName, e.Name, e.ValueFrom))
			continue
		}

		kind := parts[0]
		name := parts[1]
		output := parts[2]

		switch kind {
		case "resource":
			res, exists := set.Resources[name]
			if !exists {
				// Absent target is handled by the enrichment layer (FR-010, Q2):
				// it raises the typed EnrichmentError unless an explicit
				// spec.dependencies entry covers the reference. Skip here.
				continue
			}
			if !isExportedOutput(res, output) {
				errs = append(errs, fmt.Errorf("%s %q: env %q references non-existent output %q on resource %q",
					kindLabel, consumerName, e.Name, output, name))
			}

		case "application":
			_, exists := set.Applications[name]
			if !exists {
				// Same reasoning as the resource branch above.
				continue
			}
			if output != "host" && output != "port" {
				errs = append(errs, fmt.Errorf("%s %q: env %q: application %q has no built-in output %q (only host and port are supported)",
					kindLabel, consumerName, e.Name, name, output))
			}

		default:
			errs = append(errs, fmt.Errorf("%s %q: env %q has invalid valueFrom format %q (expected resource.<name>.<output>, application.<name>.<built-in>, or vault:<project>/<env>/<key>)",
				kindLabel, consumerName, e.Name, e.ValueFrom))
		}
	}
	return errs
}

// isExportedOutput reports whether name is present in a resource's export
// allowlist (spec.outputs).
func isExportedOutput(res *manifest.ResourceManifest, name string) bool {
	for _, out := range res.Spec.Outputs {
		if out.Name == name {
			return true
		}
	}
	return false
}

// validateVaultPath checks that a vault: ref has exactly 3 non-empty path components.
func validateVaultPath(kind, owner, fieldType, fieldName, ref string) []error {
	path := strings.TrimPrefix(ref, "vault:")
	parts := strings.SplitN(path, "/", 4)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return []error{fmt.Errorf("%s %q: %s %q has invalid vault ref %q (expected vault:<project>/<environment>/<secret-name>)",
			kind, owner, fieldType, fieldName, ref)}
	}
	return nil
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
