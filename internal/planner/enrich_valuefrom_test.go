package planner

import (
	"errors"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

func TestParseValueFromRef_ResourcePrefix(t *testing.T) {
	ref, ok := parseValueFromRef("resource.db.HOST")
	if !ok {
		t.Fatal("expected ok=true for resource.db.HOST")
	}
	if ref.Kind != "resource" || ref.Name != "db" || ref.Output != "HOST" {
		t.Errorf("unexpected parse: %+v", ref)
	}
}

func TestParseValueFromRef_ApplicationPrefix(t *testing.T) {
	ref, ok := parseValueFromRef("application.api.host")
	if !ok {
		t.Fatal("expected ok=true for application.api.host")
	}
	if ref.Kind != "application" || ref.Name != "api" || ref.Output != "host" {
		t.Errorf("unexpected parse: %+v", ref)
	}
}

func TestParseValueFromRef_RejectsVault(t *testing.T) {
	if _, ok := parseValueFromRef("vault:proj/env/key"); ok {
		t.Error("vault refs must return false")
	}
}

func TestParseValueFromRef_RejectsEmpty(t *testing.T) {
	if _, ok := parseValueFromRef(""); ok {
		t.Error("empty string must return false")
	}
}

func TestParseValueFromRef_RejectsTwoParts(t *testing.T) {
	if _, ok := parseValueFromRef("resource.foo"); ok {
		t.Error("two-part refs must return false")
	}
}

func TestParseValueFromRef_RejectsEmptyMiddle(t *testing.T) {
	if _, ok := parseValueFromRef("resource..output"); ok {
		t.Error("empty middle must return false")
	}
}

func TestParseValueFromRef_RejectsUnknownPrefix(t *testing.T) {
	if _, ok := parseValueFromRef("secret.x.y"); ok {
		t.Error("unknown prefix must return false")
	}
}

// US1 unit tests for enrichValueFromResource.

func ownerAppSet(t *testing.T) *ManifestSet {
	t.Helper()
	set := NewManifestSet()
	set.Resources["db"] = &manifest.ResourceManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ResourceKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "db", Owner: "team-a"},
		Spec: manifest.ResourceSpec{
			Type:    "postgres",
			Version: "16",
			Outputs: []manifest.Output{{Name: "URL", Value: "v"}},
		},
	}
	set.Applications["alpha"] = &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "alpha", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Image: "nginx",
			Env: []manifest.EnvVar{
				{Name: "DB_URL", ValueFrom: "resource.db.URL"},
			},
		},
	}
	return set
}

func TestEnrichValueFromResource_AddsInferredEdge(t *testing.T) {
	set := ownerAppSet(t)
	out, edges, err := applyEnrichmentRule(set, manifest.ResourceKind, lookupResourceOwner(set), parseResourceRef)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 inferred edge, got %d", len(edges))
	}
	if edges[0].Consumer.Name != "alpha" || edges[0].Target.Name != "db" || edges[0].EnvVar != "DB_URL" {
		t.Errorf("unexpected edge: %+v", edges[0])
	}
	app := out.Applications["alpha"]
	if len(app.Spec.Dependencies) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(app.Spec.Dependencies))
	}
	if app.Spec.Dependencies[0].Kind != manifest.ResourceKind || app.Spec.Dependencies[0].Name != "db" {
		t.Errorf("unexpected dep: %+v", app.Spec.Dependencies[0])
	}
	// Input set untouched.
	if len(set.Applications["alpha"].Spec.Dependencies) != 0 {
		t.Error("input set was mutated")
	}
}

func TestEnrichValueFromResource_CrossTeamWithoutExplicitDep_Fails(t *testing.T) {
	set := ownerAppSet(t)
	set.Resources["db"].Metadata.Owner = "team-b"

	_, _, err := applyEnrichmentRule(set, manifest.ResourceKind, lookupResourceOwner(set), parseResourceRef)
	if err == nil {
		t.Fatal("expected error for cross-team ref")
	}
	var ee *EnrichmentError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *EnrichmentError, got %T: %v", err, err)
	}
	if ee.Kind != ErrCrossTeamOrUnresolvedValueFrom {
		t.Errorf("unexpected kind: %q", ee.Kind)
	}
	if ee.ConsumerName != "alpha" || ee.EnvName != "DB_URL" || ee.TargetName != "db" {
		t.Errorf("unexpected error fields: %+v", ee)
	}
}

func TestEnrichValueFromResource_AbsentTargetWithoutExplicitDep_Fails(t *testing.T) {
	set := ownerAppSet(t)
	delete(set.Resources, "db")

	_, _, err := applyEnrichmentRule(set, manifest.ResourceKind, lookupResourceOwner(set), parseResourceRef)
	if err == nil {
		t.Fatal("expected error for absent target ref")
	}
	var ee *EnrichmentError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *EnrichmentError, got %T", err)
	}
}

func TestEnrichValueFromResource_ExplicitDepShortCircuitsFailureAndDedups(t *testing.T) {
	set := ownerAppSet(t)
	set.Resources["db"].Metadata.Owner = "team-b" // would otherwise fail
	set.Applications["alpha"].Spec.Dependencies = []manifest.Dependency{
		{Kind: manifest.ResourceKind, Name: "db", Owner: "team-b"},
	}

	out, edges, err := applyEnrichmentRule(set, manifest.ResourceKind, lookupResourceOwner(set), parseResourceRef)
	if err != nil {
		t.Fatalf("expected no error when explicit dep covers ref, got %v", err)
	}
	if len(edges) != 0 {
		t.Fatalf("explicit dep must dedup — expected 0 inferred edges, got %d", len(edges))
	}
	// Original explicit entry preserved.
	app := out.Applications["alpha"]
	if len(app.Spec.Dependencies) != 1 || app.Spec.Dependencies[0].Owner != "team-b" {
		t.Errorf("explicit owner field must be preserved verbatim: %+v", app.Spec.Dependencies)
	}
}

func TestEnrichValueFromResource_IgnoresApplicationPrefix(t *testing.T) {
	set := ownerAppSet(t)
	set.Applications["alpha"].Spec.Env = []manifest.EnvVar{
		{Name: "HOST", ValueFrom: "application.alpha.host"},
	}
	_, edges, err := applyEnrichmentRule(set, manifest.ResourceKind, lookupResourceOwner(set), parseResourceRef)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("resource rule must ignore application.* refs, got edges: %+v", edges)
	}
}

func TestEnrichValueFromResource_SkipsVaultAndLiteral(t *testing.T) {
	set := ownerAppSet(t)
	set.Applications["alpha"].Spec.Env = []manifest.EnvVar{
		{Name: "S", ValueFrom: "vault:proj/env/key"},
		{Name: "L", Value: "literal"},
	}
	_, edges, err := applyEnrichmentRule(set, manifest.ResourceKind, lookupResourceOwner(set), parseResourceRef)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("vault/literal env vars must be skipped: %+v", edges)
	}
}

// US2 unit tests for enrichValueFromApplication.

func appAppSet(t *testing.T) *ManifestSet {
	t.Helper()
	set := NewManifestSet()
	set.Applications["producer"] = &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "producer", Owner: "team-a"},
		Spec:     manifest.ApplicationSpec{Image: "nginx"},
	}
	set.Applications["consumer"] = &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "consumer", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Image: "nginx",
			Env: []manifest.EnvVar{
				{Name: "PROD_HOST", ValueFrom: "application.producer.host"},
			},
		},
	}
	return set
}

func TestEnrichValueFromApplication_AddsInferredEdge(t *testing.T) {
	set := appAppSet(t)
	out, edges, err := applyEnrichmentRule(set, manifest.ApplicationKind, lookupApplicationOwner(set), parseApplicationRef)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 inferred edge, got %d", len(edges))
	}
	if edges[0].Consumer.Name != "consumer" || edges[0].Target.Name != "producer" {
		t.Errorf("unexpected edge: %+v", edges[0])
	}
	app := out.Applications["consumer"]
	if len(app.Spec.Dependencies) != 1 || app.Spec.Dependencies[0].Kind != manifest.ApplicationKind {
		t.Errorf("unexpected dep: %+v", app.Spec.Dependencies)
	}
}

func TestEnrichValueFromApplication_CrossTeamWithoutExplicitDep_Fails(t *testing.T) {
	set := appAppSet(t)
	set.Applications["producer"].Metadata.Owner = "team-b"

	_, _, err := applyEnrichmentRule(set, manifest.ApplicationKind, lookupApplicationOwner(set), parseApplicationRef)
	if err == nil {
		t.Fatal("expected error for cross-team ref")
	}
	var ee *EnrichmentError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *EnrichmentError, got %T", err)
	}
	if ee.TargetName != "producer" {
		t.Errorf("unexpected target name: %q", ee.TargetName)
	}
}

func TestEnrichValueFromApplication_AbsentTargetWithoutExplicitDep_Fails(t *testing.T) {
	set := appAppSet(t)
	delete(set.Applications, "producer")

	_, _, err := applyEnrichmentRule(set, manifest.ApplicationKind, lookupApplicationOwner(set), parseApplicationRef)
	if err == nil {
		t.Fatal("expected error for absent app target")
	}
}

func TestEnrichValueFromApplication_ExplicitDepShortCircuits(t *testing.T) {
	set := appAppSet(t)
	set.Applications["producer"].Metadata.Owner = "team-b"
	set.Applications["consumer"].Spec.Dependencies = []manifest.Dependency{
		{Kind: manifest.ApplicationKind, Name: "producer", Owner: "team-b"},
	}
	_, edges, err := applyEnrichmentRule(set, manifest.ApplicationKind, lookupApplicationOwner(set), parseApplicationRef)
	if err != nil {
		t.Fatalf("explicit dep must short-circuit failure, got %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("dedup against explicit must skip inferred edge: %+v", edges)
	}
}

func TestEnrichValueFromApplication_IgnoresResourcePrefix(t *testing.T) {
	set := appAppSet(t)
	set.Applications["consumer"].Spec.Env = []manifest.EnvVar{
		{Name: "X", ValueFrom: "resource.something.URL"},
	}
	_, edges, err := applyEnrichmentRule(set, manifest.ApplicationKind, lookupApplicationOwner(set), parseApplicationRef)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("application rule must ignore resource.* refs, got edges: %+v", edges)
	}
}
