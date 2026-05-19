package handler

import (
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/planner"
)

func TestFilterForSingle_Application(t *testing.T) {
	m := &manifest.Manifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Application: &manifest.ApplicationManifest{
			Metadata: manifest.Metadata{Name: "alpha", Owner: "team-a"},
		},
	}
	f, err := filterForSingle(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Kind != planner.FilterApp {
		t.Errorf("kind = %v, want FilterApp", f.Kind)
	}
	if f.Name != "alpha" {
		t.Errorf("name = %q, want alpha", f.Name)
	}
}

func TestFilterForSingle_Resource(t *testing.T) {
	m := &manifest.Manifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ResourceKind, APIVersion: "shrine/v1"},
		Resource: &manifest.ResourceManifest{
			Metadata: manifest.Metadata{Name: "db", Owner: "team-a"},
		},
	}
	f, err := filterForSingle(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Kind != planner.FilterRes {
		t.Errorf("kind = %v, want FilterRes", f.Kind)
	}
	if f.Name != "db" {
		t.Errorf("name = %q, want db", f.Name)
	}
}

func TestFilterForSingle_TeamKindRejected(t *testing.T) {
	m := &manifest.Manifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.TeamKind, APIVersion: "shrine/v1"},
	}
	_, err := filterForSingle(m)
	if err == nil {
		t.Fatal("expected error rejecting Team-kind for apply -f")
	}
	want := "team manifests cannot be applied with --file; use 'shrine apply teams' instead"
	if err.Error() != want {
		t.Errorf("error = %q\nwant      %q", err.Error(), want)
	}
}

func TestFilterForSingle_UnsupportedKind(t *testing.T) {
	m := &manifest.Manifest{
		TypeMeta: manifest.TypeMeta{Kind: "Sidecar", APIVersion: "shrine/v1"},
	}
	_, err := filterForSingle(m)
	if err == nil || !strings.Contains(err.Error(), "Sidecar") {
		t.Errorf("expected unsupported-kind error mentioning Sidecar, got: %v", err)
	}
}

// TestLoadSetForSingle_NoDir verifies the minimal-set construction path used
// when manifestDir == "" (no resolution context). This is the equivalent of
// today's PlanSingle's specsDir=="" branch.
func TestLoadSetForSingle_NoDir(t *testing.T) {
	m := &manifest.Manifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Application: &manifest.ApplicationManifest{
			TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
			Metadata: manifest.Metadata{Name: "alpha", Owner: "team-a"},
		},
	}
	set, err := loadSetForSingle("alpha.yaml", "", m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := set.Applications["alpha"]; !ok {
		t.Error("alpha should be in set")
	}
	if len(set.Applications) != 1 || len(set.Resources) != 0 {
		t.Errorf("expected minimal set (1 app, 0 res), got %d/%d", len(set.Applications), len(set.Resources))
	}
}
