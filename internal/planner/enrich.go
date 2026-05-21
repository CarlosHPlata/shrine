package planner

import (
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// Enricher transforms a ManifestSet into an enriched copy by adding inferred
// deploy-order dependencies. Implementations must not mutate the input set.
//
// Enrich returns a non-nil error when a valueFrom reference cannot be resolved
// to a same-team manifest and is not already covered by an explicit
// Spec.Dependencies entry on the consumer; the chain stops at the first such
// error.
type Enricher interface {
	Enrich(set *ManifestSet) (*ManifestSet, error)
}

// edgeAwareEnricher is the package-internal richer contract that production
// rules implement so ChainEnrich can aggregate the rule's inferred-edge
// provenance. External Enricher implementations (e.g., tests passing
// arbitrary rules) simply contribute no edges.
type edgeAwareEnricher interface {
	Enricher
	enrichWithEdges(set *ManifestSet) (*ManifestSet, []InferredEdge, error)
}

// ManifestRef identifies a target manifest in the current set.
type ManifestRef struct {
	Kind  string
	Name  string
	Owner string
}

// InferredEdge records one inferred dependency edge for dry-run provenance.
type InferredEdge struct {
	Consumer ManifestRef
	Target   ManifestRef
	EnvVar   string
}

// EnrichmentErrorKind is a stable identifier for the failure category.
type EnrichmentErrorKind string

const ErrCrossTeamOrUnresolvedValueFrom EnrichmentErrorKind = "cross-team-or-unresolved-valuefrom"

// EnrichmentError is the typed error returned when a valueFrom reference cannot
// be enriched and is not covered by an explicit Spec.Dependencies entry.
type EnrichmentError struct {
	Kind          EnrichmentErrorKind
	ConsumerKind  string
	ConsumerName  string
	ConsumerOwner string
	EnvName       string
	TargetKind    string // "resource" | "application"
	TargetName    string
	TargetOutput  string
}

func (e *EnrichmentError) Error() string {
	return fmt.Sprintf(
		`enrichment: app %q env %q references %s %q which is not owned by team %q; add an explicit spec.dependencies entry (kind: %s, name: %s) to declare this dependency`,
		e.ConsumerName,
		e.EnvName,
		e.TargetKind,
		e.TargetName+"."+e.TargetOutput,
		e.ConsumerOwner,
		titleKind(e.TargetKind),
		e.TargetName,
	)
}

func titleKind(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-32) + s[1:]
	}
	return s
}

// ChainEnrich applies the given Enrichers in order to a shallow copy of set,
// returning the enriched copy along with the inferred-edge provenance. The
// input set is observably unmodified after this call. Rules operate on the
// previous rule's output. On any non-nil error, returns (nil, nil, err).
func ChainEnrich(set *ManifestSet, rules ...Enricher) (*ManifestSet, []InferredEdge, error) {
	current := copyManifestSetShallow(set)
	var all []InferredEdge

	for _, rule := range rules {
		if ea, ok := rule.(edgeAwareEnricher); ok {
			out, edges, err := ea.enrichWithEdges(current)
			if err != nil {
				return nil, nil, err
			}
			current = out
			all = append(all, edges...)
			continue
		}
		out, err := rule.Enrich(current)
		if err != nil {
			return nil, nil, err
		}
		current = out
	}
	return current, all, nil
}

// DefaultEnrichers returns the production rule chain in deterministic order.
func DefaultEnrichers() []Enricher {
	return []Enricher{
		enrichValueFromResource{},
		enrichValueFromApplication{},
	}
}

// copyManifestSetShallow allocates a new *ManifestSet whose maps are fresh
// (different headers) but whose values are pointer-shared with the input.
func copyManifestSetShallow(set *ManifestSet) *ManifestSet {
	out := NewManifestSet()
	for name, app := range set.Applications {
		out.Applications[name] = app
	}
	for name, res := range set.Resources {
		out.Resources[name] = res
	}
	return out
}

// cloneApplicationWithDeps returns a new ApplicationManifest whose
// Spec.Dependencies is the original slice plus extra appended. If extra is
// empty, returns app unchanged.
func cloneApplicationWithDeps(app *manifest.ApplicationManifest, extra []manifest.Dependency) *manifest.ApplicationManifest {
	if len(extra) == 0 {
		return app
	}
	clone := *app
	deps := make([]manifest.Dependency, 0, len(app.Spec.Dependencies)+len(extra))
	deps = append(deps, app.Spec.Dependencies...)
	deps = append(deps, extra...)
	clone.Spec.Dependencies = deps
	return &clone
}

// hasExplicitDependency returns true if any entry in deps matches (kind, name).
// Owner is intentionally ignored.
func hasExplicitDependency(deps []manifest.Dependency, kind, name string) bool {
	for _, d := range deps {
		if d.Kind == kind && d.Name == name {
			return true
		}
	}
	return false
}
