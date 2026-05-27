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

// consumerView is the read-only slice of a manifest the enrichment loop needs:
// its identity plus the env vars and explicit deps used to infer edges.
type consumerView struct {
	kind  string
	name  string
	owner string
	env   []manifest.EnvVar
	deps  []manifest.Dependency
}

// applyEnrichmentRule is the shared loop both production rules use. It scans
// every consumer — Applications and Resources alike (FR-014) — in
// sorted-by-name order; for each it inspects Spec.Env in declaration order,
// parses the valueFrom via parseFor, dedups against the consumer's existing
// Spec.Dependencies on (Kind, Name), and either fails fast on a cross-team /
// absent target or appends an inferred edge plus the dependency to a cloned
// manifest.
func applyEnrichmentRule(
	set *ManifestSet,
	targetKind string,
	lookupOwner func(name string) (string, bool),
	parseFor func(string) (valueFromRef, bool),
) (*ManifestSet, []InferredEdge, error) {
	out := copyManifestSetShallow(set)
	var edges []InferredEdge

	for _, name := range sortedKeys(out.Applications) {
		app := out.Applications[name]
		cv := consumerView{kind: manifest.ApplicationKind, name: app.Metadata.Name, owner: app.Metadata.Owner, env: app.Spec.Env, deps: app.Spec.Dependencies}
		extra, e, err := inferConsumerDeps(cv, targetKind, lookupOwner, parseFor)
		if err != nil {
			return nil, nil, err
		}
		edges = append(edges, e...)
		if len(extra) > 0 {
			out.Applications[name] = cloneApplicationWithDeps(app, extra)
		}
	}

	for _, name := range sortedKeys(out.Resources) {
		res := out.Resources[name]
		cv := consumerView{kind: manifest.ResourceKind, name: res.Metadata.Name, owner: res.Metadata.Owner, env: res.Spec.Env, deps: res.Spec.Dependencies}
		extra, e, err := inferConsumerDeps(cv, targetKind, lookupOwner, parseFor)
		if err != nil {
			return nil, nil, err
		}
		edges = append(edges, e...)
		if len(extra) > 0 {
			out.Resources[name] = cloneResourceWithDeps(res, extra)
		}
	}

	return out, edges, nil
}

// inferConsumerDeps scans one consumer's env for valueFrom references matching
// the rule, returning the inferred dependencies and edges, or a fail-fast
// EnrichmentError on a cross-team / absent target not covered by an explicit dep.
func inferConsumerDeps(
	cv consumerView,
	targetKind string,
	lookupOwner func(name string) (string, bool),
	parseFor func(string) (valueFromRef, bool),
) ([]manifest.Dependency, []InferredEdge, error) {
	var extra []manifest.Dependency
	var edges []InferredEdge
	seenInThisRule := make(map[string]struct{})

	for _, env := range cv.env {
		if env.ValueFrom == "" {
			continue
		}
		ref, ok := parseFor(env.ValueFrom)
		if !ok {
			continue
		}
		// Explicit dep covers this reference — skip both inference and the
		// fail-fast check.
		if hasExplicitDependency(cv.deps, targetKind, ref.Name) {
			continue
		}
		// Resolve same-team gate.
		owner, exists := lookupOwner(ref.Name)
		if !exists || owner != cv.owner {
			return nil, nil, &EnrichmentError{
				Kind:          ErrCrossTeamOrUnresolvedValueFrom,
				ConsumerKind:  cv.kind,
				ConsumerName:  cv.name,
				ConsumerOwner: cv.owner,
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
		extra = append(extra, manifest.Dependency{Kind: targetKind, Name: ref.Name, Owner: owner})
		edges = append(edges, InferredEdge{
			Consumer: ManifestRef{Kind: cv.kind, Name: cv.name, Owner: cv.owner},
			Target:   ManifestRef{Kind: targetKind, Name: ref.Name, Owner: owner},
			EnvVar:   env.Name,
		})
	}
	return extra, edges, nil
}

// sortedKeys returns a map's string keys in ascending order for deterministic
// iteration.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
