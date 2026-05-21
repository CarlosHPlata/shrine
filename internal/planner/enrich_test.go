package planner

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// helpers tests (T003)

func TestCopyManifestSetShallow_AllocsFreshMapsAndPointerSharesValues(t *testing.T) {
	set := NewManifestSet()
	app := &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "a", Owner: "team-a"},
	}
	set.Applications["a"] = app
	res := &manifest.ResourceManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ResourceKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "r", Owner: "team-a"},
	}
	set.Resources["r"] = res

	copied := copyManifestSetShallow(set)

	if copied == set {
		t.Fatal("expected fresh *ManifestSet pointer")
	}
	if reflect.ValueOf(copied.Applications).Pointer() == reflect.ValueOf(set.Applications).Pointer() {
		t.Error("Applications map header should be fresh")
	}
	if reflect.ValueOf(copied.Resources).Pointer() == reflect.ValueOf(set.Resources).Pointer() {
		t.Error("Resources map header should be fresh")
	}
	if copied.Applications["a"] != app {
		t.Error("expected pointer-shared Application value")
	}
	if copied.Resources["r"] != res {
		t.Error("expected pointer-shared Resource value")
	}
}

func TestCloneApplicationWithDeps_NoExtraReturnsSamePointer(t *testing.T) {
	app := &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "a"},
		Spec: manifest.ApplicationSpec{
			Dependencies: []manifest.Dependency{{Kind: manifest.ResourceKind, Name: "x"}},
		},
	}
	out := cloneApplicationWithDeps(app, nil)
	if out != app {
		t.Error("expected same pointer when extra is empty")
	}
}

func TestCloneApplicationWithDeps_AppendsAndPreservesOriginalSlice(t *testing.T) {
	app := &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "a"},
		Spec: manifest.ApplicationSpec{
			Dependencies: []manifest.Dependency{{Kind: manifest.ResourceKind, Name: "old"}},
		},
	}
	extra := []manifest.Dependency{{Kind: manifest.ResourceKind, Name: "new"}}
	out := cloneApplicationWithDeps(app, extra)

	if out == app {
		t.Fatal("expected fresh *ApplicationManifest pointer")
	}
	if len(out.Spec.Dependencies) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(out.Spec.Dependencies))
	}
	if out.Spec.Dependencies[0].Name != "old" || out.Spec.Dependencies[1].Name != "new" {
		t.Errorf("unexpected deps order: %+v", out.Spec.Dependencies)
	}
	// Original slice unchanged.
	if len(app.Spec.Dependencies) != 1 {
		t.Errorf("original slice mutated: len=%d", len(app.Spec.Dependencies))
	}
}

func TestHasExplicitDependency_MatchesKindAndName_IgnoresOwner(t *testing.T) {
	deps := []manifest.Dependency{
		{Kind: manifest.ResourceKind, Name: "db", Owner: "team-x"},
	}
	if !hasExplicitDependency(deps, manifest.ResourceKind, "db") {
		t.Error("expected match ignoring owner")
	}
	if hasExplicitDependency(deps, manifest.ApplicationKind, "db") {
		t.Error("kind must matter")
	}
	if hasExplicitDependency(deps, manifest.ResourceKind, "other") {
		t.Error("name must matter")
	}
}

// ChainEnrich tests (T004)

type noopEnricher struct{}

func (noopEnricher) Enrich(set *ManifestSet) (*ManifestSet, error) { return set, nil }

// edgeAdder is a test Enricher that appends a hard-coded edge to a target app.
type edgeAdder struct {
	app    string
	dep    manifest.Dependency
	envVar string
}

func (e edgeAdder) Enrich(set *ManifestSet) (*ManifestSet, error) {
	out := copyManifestSetShallow(set)
	app, ok := out.Applications[e.app]
	if !ok {
		return out, nil
	}
	out.Applications[e.app] = cloneApplicationWithDeps(app, []manifest.Dependency{e.dep})
	return out, nil
}

type failEnricher struct {
	calls *int
}

func (f failEnricher) Enrich(set *ManifestSet) (*ManifestSet, error) {
	*f.calls++
	return nil, &EnrichmentError{Kind: ErrCrossTeamOrUnresolvedValueFrom, ConsumerName: "x"}
}

type countingEnricher struct {
	calls *int
}

func (c countingEnricher) Enrich(set *ManifestSet) (*ManifestSet, error) {
	*c.calls++
	return set, nil
}

func TestChainEnrich_EmptyRules_ReturnsShallowCopy(t *testing.T) {
	set := NewManifestSet()
	set.Applications["a"] = &manifest.ApplicationManifest{Metadata: manifest.Metadata{Name: "a"}}
	out, edges, err := ChainEnrich(set)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("expected no edges, got %d", len(edges))
	}
	if out == set {
		t.Error("expected fresh *ManifestSet")
	}
	if out.Applications["a"] != set.Applications["a"] {
		t.Error("expected pointer-shared values inside shallow copy")
	}
}

func TestChainEnrich_TwoRules_SecondObservesFirst(t *testing.T) {
	set := NewManifestSet()
	set.Applications["a"] = &manifest.ApplicationManifest{Metadata: manifest.Metadata{Name: "a"}}

	r1 := edgeAdder{app: "a", dep: manifest.Dependency{Kind: manifest.ResourceKind, Name: "x"}}
	r2 := edgeAdder{app: "a", dep: manifest.Dependency{Kind: manifest.ResourceKind, Name: "y"}}

	out, _, err := ChainEnrich(set, r1, r2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	deps := out.Applications["a"].Spec.Dependencies
	if len(deps) != 2 || deps[0].Name != "x" || deps[1].Name != "y" {
		t.Errorf("expected r1 then r2 edges, got %+v", deps)
	}
}

func TestChainEnrich_FirstErrorShortCircuits(t *testing.T) {
	calls := 0
	failCalls := 0
	rules := []Enricher{
		failEnricher{calls: &failCalls},
		countingEnricher{calls: &calls},
	}
	set := NewManifestSet()
	_, _, err := ChainEnrich(set, rules...)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if failCalls != 1 {
		t.Errorf("fail enricher should run once, got %d", failCalls)
	}
	if calls != 0 {
		t.Errorf("subsequent rule must not run, got %d calls", calls)
	}
}

func TestChainEnrich_SuccessIdempotent(t *testing.T) {
	set := NewManifestSet()
	set.Applications["a"] = &manifest.ApplicationManifest{Metadata: manifest.Metadata{Name: "a"}}

	rule := edgeAdder{app: "a", dep: manifest.Dependency{Kind: manifest.ResourceKind, Name: "x"}}
	out1, _, err := ChainEnrich(set, rule)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	// Running rule on its own output: it would dedup in production code, but here
	// edgeAdder always appends. The idempotence guarantee in the contract is on
	// the chain WHEN the rules themselves dedup (which they do via
	// hasExplicitDependency in applyEnrichmentRule). Use a real-style dedup rule.
	dedupRule := dedupEdgeAdder{app: "a", dep: manifest.Dependency{Kind: manifest.ResourceKind, Name: "x"}}
	out2, _, err := ChainEnrich(set, dedupRule)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	out3, _, err := ChainEnrich(out2, dedupRule)
	if err != nil {
		t.Fatalf("third call error: %v", err)
	}
	if !reflect.DeepEqual(out2.Applications["a"].Spec.Dependencies, out3.Applications["a"].Spec.Dependencies) {
		t.Errorf("idempotence broken: out2=%+v out3=%+v",
			out2.Applications["a"].Spec.Dependencies,
			out3.Applications["a"].Spec.Dependencies)
	}
	_ = out1
}

func TestChainEnrich_FailureDeterministic(t *testing.T) {
	calls1 := 0
	calls2 := 0
	set := NewManifestSet()

	_, _, err1 := ChainEnrich(set, failEnricher{calls: &calls1})
	_, _, err2 := ChainEnrich(set, failEnricher{calls: &calls2})

	var ee1, ee2 *EnrichmentError
	if !errors.As(err1, &ee1) || !errors.As(err2, &ee2) {
		t.Fatalf("expected *EnrichmentError both runs, got %T %T", err1, err2)
	}
	if ee1.Kind != ee2.Kind || ee1.ConsumerName != ee2.ConsumerName {
		t.Errorf("non-deterministic failure: %+v vs %+v", ee1, ee2)
	}
}

// dedupEdgeAdder appends the dep only if not already present.
type dedupEdgeAdder struct {
	app string
	dep manifest.Dependency
}

func (e dedupEdgeAdder) Enrich(set *ManifestSet) (*ManifestSet, error) {
	out := copyManifestSetShallow(set)
	app, ok := out.Applications[e.app]
	if !ok {
		return out, nil
	}
	if hasExplicitDependency(app.Spec.Dependencies, e.dep.Kind, e.dep.Name) {
		return out, nil
	}
	out.Applications[e.app] = cloneApplicationWithDeps(app, []manifest.Dependency{e.dep})
	return out, nil
}

// applyEnrichmentRule failure-path tests (T005)

func TestApplyEnrichmentRule_CrossTeam_Fails(t *testing.T) {
	set := NewManifestSet()
	set.Applications["alpha"] = &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "alpha", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Env: []manifest.EnvVar{{Name: "X", ValueFrom: "resource.shared.HOST"}},
		},
	}
	set.Resources["shared"] = &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "shared", Owner: "team-b"},
	}

	out, edges, err := applyEnrichmentRule(set, manifest.ResourceKind, lookupResourceOwner(set), parseResourceRef)
	if err == nil {
		t.Fatal("expected error for cross-team ref")
	}
	if out != nil {
		t.Errorf("expected nil set on error, got %+v", out)
	}
	if edges != nil {
		t.Errorf("expected nil edges on error, got %+v", edges)
	}
	var ee *EnrichmentError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *EnrichmentError, got %T", err)
	}
	if ee.Kind != ErrCrossTeamOrUnresolvedValueFrom {
		t.Errorf("unexpected kind: %q", ee.Kind)
	}
	if ee.ConsumerName != "alpha" || ee.EnvName != "X" || ee.TargetName != "shared" {
		t.Errorf("unexpected fields: %+v", ee)
	}
}

func TestApplyEnrichmentRule_AbsentTarget_Fails(t *testing.T) {
	set := NewManifestSet()
	set.Applications["alpha"] = &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "alpha", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Env: []manifest.EnvVar{{Name: "X", ValueFrom: "resource.absent.HOST"}},
		},
	}
	_, _, err := applyEnrichmentRule(set, manifest.ResourceKind, lookupResourceOwner(set), parseResourceRef)
	if err == nil {
		t.Fatal("expected error for absent target")
	}
	var ee *EnrichmentError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *EnrichmentError, got %T", err)
	}
	if ee.TargetName != "absent" {
		t.Errorf("unexpected target: %q", ee.TargetName)
	}
}

func TestApplyEnrichmentRule_CrossTeamWithExplicitDep_Succeeds(t *testing.T) {
	set := NewManifestSet()
	set.Applications["alpha"] = &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "alpha", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Env: []manifest.EnvVar{{Name: "X", ValueFrom: "resource.shared.HOST"}},
			Dependencies: []manifest.Dependency{
				{Kind: manifest.ResourceKind, Name: "shared", Owner: "team-b"},
			},
		},
	}
	set.Resources["shared"] = &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "shared", Owner: "team-b"},
	}

	_, edges, err := applyEnrichmentRule(set, manifest.ResourceKind, lookupResourceOwner(set), parseResourceRef)
	if err != nil {
		t.Fatalf("explicit dep must satisfy gate, got %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("explicit dep must short-circuit (no inferred edge), got %+v", edges)
	}
}

func TestApplyEnrichmentRule_DeterministicFirstError(t *testing.T) {
	set := NewManifestSet()
	// Two consumers each with two bad refs.
	set.Applications["beta"] = &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "beta", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Env: []manifest.EnvVar{
				{Name: "B1", ValueFrom: "resource.missing-b1.X"},
				{Name: "B2", ValueFrom: "resource.missing-b2.X"},
			},
		},
	}
	set.Applications["alpha"] = &manifest.ApplicationManifest{
		Metadata: manifest.Metadata{Name: "alpha", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Env: []manifest.EnvVar{
				{Name: "A1", ValueFrom: "resource.missing-a1.X"},
				{Name: "A2", ValueFrom: "resource.missing-a2.X"},
			},
		},
	}

	var first *EnrichmentError
	for i := 0; i < 3; i++ {
		_, _, err := applyEnrichmentRule(set, manifest.ResourceKind, lookupResourceOwner(set), parseResourceRef)
		var ee *EnrichmentError
		if !errors.As(err, &ee) {
			t.Fatalf("run %d: expected *EnrichmentError", i)
		}
		if first == nil {
			first = ee
			continue
		}
		if ee.ConsumerName != first.ConsumerName || ee.EnvName != first.EnvName || ee.TargetName != first.TargetName {
			t.Errorf("run %d non-deterministic: got %+v vs %+v", i, ee, first)
		}
	}
	// Sort-order says alpha < beta, and within alpha the first env var (declaration order) wins.
	if first.ConsumerName != "alpha" || first.EnvName != "A1" {
		t.Errorf("expected first error on alpha/A1, got %+v", first)
	}
}

func TestEnrichmentError_FormatsContractMessage(t *testing.T) {
	ee := &EnrichmentError{
		Kind:          ErrCrossTeamOrUnresolvedValueFrom,
		ConsumerKind:  manifest.ApplicationKind,
		ConsumerName:  "ops-bot",
		ConsumerOwner: "ops_bot",
		EnvName:       "CACHE_HOST",
		TargetKind:    "resource",
		TargetName:    "shared-cache",
		TargetOutput:  "HOST",
	}
	msg := ee.Error()
	want := `enrichment: app "ops-bot" env "CACHE_HOST" references resource "shared-cache.HOST" which is not owned by team "ops_bot"; add an explicit spec.dependencies entry (kind: Resource, name: shared-cache) to declare this dependency`
	if msg != want {
		t.Errorf("wrong format\n got: %s\nwant: %s", msg, want)
	}
	if !strings.Contains(msg, "ops-bot") {
		t.Error("must mention consumer")
	}
}
