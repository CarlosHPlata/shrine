package planner

import (
	"sort"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// valueFromRef is the parsed shape of `<kind>.<name>.<output>` env references.
type valueFromRef struct {
	Kind   string // "resource" | "application"
	Name   string
	Output string
}

// parseValueFromRef recognizes the `resource.<name>.<output>` and
// `application.<name>.<output>` grammars. Returns (_, false) for vault refs,
// literal values, malformed strings, or any string that does not split into
// exactly three dot-separated non-empty parts with a known kind prefix.
func parseValueFromRef(s string) (valueFromRef, bool) {
	if s == "" {
		return valueFromRef{}, false
	}
	if strings.HasPrefix(s, "vault:") {
		return valueFromRef{}, false
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return valueFromRef{}, false
	}
	if parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return valueFromRef{}, false
	}
	if parts[0] != "resource" && parts[0] != "application" {
		return valueFromRef{}, false
	}
	return valueFromRef{Kind: parts[0], Name: parts[1], Output: parts[2]}, true
}

// applyEnrichmentRule is the shared loop both production rules use. It
// iterates Applications in sorted-by-name order; for each Application it scans
// Spec.Env in declaration order, parses the valueFrom via parseFor, dedups
// against the consumer's existing Spec.Dependencies on (Kind, Name), and
// either fails fast on a cross-team / absent target or appends an inferred
// edge plus the dependency to a cloned manifest.
func applyEnrichmentRule(
	set *ManifestSet,
	targetKind string,
	lookupOwner func(name string) (string, bool),
	parseFor func(string) (valueFromRef, bool),
) (*ManifestSet, []InferredEdge, error) {
	out := copyManifestSetShallow(set)

	names := make([]string, 0, len(out.Applications))
	for name := range out.Applications {
		names = append(names, name)
	}
	sort.Strings(names)

	var edges []InferredEdge
	for _, name := range names {
		app := out.Applications[name]
		var extra []manifest.Dependency
		seenInThisRule := make(map[string]struct{})
		for _, env := range app.Spec.Env {
			if env.ValueFrom == "" {
				continue
			}
			ref, ok := parseFor(env.ValueFrom)
			if !ok {
				continue
			}
			// Explicit dep covers this reference — skip both inference and
			// the fail-fast check.
			if hasExplicitDependency(app.Spec.Dependencies, targetKind, ref.Name) {
				continue
			}
			// Resolve same-team gate.
			owner, exists := lookupOwner(ref.Name)
			if !exists || owner != app.Metadata.Owner {
				return nil, nil, &EnrichmentError{
					Kind:          ErrCrossTeamOrUnresolvedValueFrom,
					ConsumerKind:  manifest.ApplicationKind,
					ConsumerName:  app.Metadata.Name,
					ConsumerOwner: app.Metadata.Owner,
					EnvName:       env.Name,
					TargetKind:    ref.Kind,
					TargetName:    ref.Name,
					TargetOutput:  ref.Output,
				}
			}
			// Dedup within this rule (multiple env vars referencing same target).
			if _, dup := seenInThisRule[ref.Name]; dup {
				continue
			}
			seenInThisRule[ref.Name] = struct{}{}
			extra = append(extra, manifest.Dependency{
				Kind:  targetKind,
				Name:  ref.Name,
				Owner: owner,
			})
			edges = append(edges, InferredEdge{
				Consumer: ManifestRef{Kind: manifest.ApplicationKind, Name: app.Metadata.Name, Owner: app.Metadata.Owner},
				Target:   ManifestRef{Kind: targetKind, Name: ref.Name, Owner: owner},
				EnvVar:   env.Name,
			})
		}
		if len(extra) > 0 {
			out.Applications[name] = cloneApplicationWithDeps(app, extra)
		}
	}
	return out, edges, nil
}

// lookupResourceOwner returns a closure resolving resource name → owner.
func lookupResourceOwner(set *ManifestSet) func(string) (string, bool) {
	return func(name string) (string, bool) {
		res, ok := set.Resources[name]
		if !ok {
			return "", false
		}
		return res.Metadata.Owner, true
	}
}

// lookupApplicationOwner returns a closure resolving app name → owner.
func lookupApplicationOwner(set *ManifestSet) func(string) (string, bool) {
	return func(name string) (string, bool) {
		app, ok := set.Applications[name]
		if !ok {
			return "", false
		}
		return app.Metadata.Owner, true
	}
}

// parseResourceRef filters to refs whose Kind is "resource".
func parseResourceRef(s string) (valueFromRef, bool) {
	ref, ok := parseValueFromRef(s)
	if !ok || ref.Kind != "resource" {
		return valueFromRef{}, false
	}
	return ref, true
}

// parseApplicationRef filters to refs whose Kind is "application".
func parseApplicationRef(s string) (valueFromRef, bool) {
	ref, ok := parseValueFromRef(s)
	if !ok || ref.Kind != "application" {
		return valueFromRef{}, false
	}
	return ref, true
}

// --- production rules ---

type enrichValueFromResource struct{}

func (enrichValueFromResource) Enrich(set *ManifestSet) (*ManifestSet, error) {
	out, _, err := enrichValueFromResource{}.enrichWithEdges(set)
	return out, err
}

func (enrichValueFromResource) enrichWithEdges(set *ManifestSet) (*ManifestSet, []InferredEdge, error) {
	return applyEnrichmentRule(set, manifest.ResourceKind, lookupResourceOwner(set), parseResourceRef)
}

type enrichValueFromApplication struct{}

func (enrichValueFromApplication) Enrich(set *ManifestSet) (*ManifestSet, error) {
	out, _, err := enrichValueFromApplication{}.enrichWithEdges(set)
	return out, err
}

func (enrichValueFromApplication) enrichWithEdges(set *ManifestSet) (*ManifestSet, []InferredEdge, error) {
	return applyEnrichmentRule(set, manifest.ApplicationKind, lookupApplicationOwner(set), parseApplicationRef)
}
