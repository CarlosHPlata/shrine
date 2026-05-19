package planner

import (
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// stubTeamStore returns a permissive team for every name (no quota limits).
type stubTeamStore struct{}

func (stubTeamStore) SaveTeam(*manifest.TeamManifest) error { return nil }
func (stubTeamStore) LoadTeam(name string) (*manifest.TeamManifest, error) {
	return &manifest.TeamManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.TeamKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: name},
		Spec:     manifest.TeamSpec{DisplayName: name},
	}, nil
}
func (stubTeamStore) ListTeams() ([]*manifest.TeamManifest, error) { return nil, nil }
func (stubTeamStore) DeleteTeam(string) error                     { return nil }

// twoTeamSet builds a manifest set spanning team-a and team-b with no
// cross-team coupling so the routing/order paths run cleanly.
func twoTeamSet() *ManifestSet {
	set := NewManifestSet()
	set.Applications["alpha"] = &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "alpha", Owner: "team-a"},
		Spec:     manifest.ApplicationSpec{Image: "nginx", Port: 80},
	}
	set.Applications["beta"] = &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "beta", Owner: "team-b"},
		Spec:     manifest.ApplicationSpec{Image: "nginx", Port: 80},
	}
	set.Resources["db-a"] = &manifest.ResourceManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ResourceKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "db-a", Owner: "team-a"},
		Spec:     manifest.ResourceSpec{Type: "postgres", Version: "16"},
	}
	set.Resources["db-b"] = &manifest.ResourceManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ResourceKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "db-b", Owner: "team-b"},
		Spec:     manifest.ResourceSpec{Type: "redis", Version: "7"},
	}
	return set
}

func TestPlan_NoFilter_EmitsAllSteps(t *testing.T) {
	set := twoTeamSet()
	result := Plan(set, stubTeamStore{}, nil, NoFilter())

	if result.Error != nil {
		t.Fatalf("unexpected Error: %v", result.Error)
	}
	if len(result.ValidationErr) != 0 {
		t.Fatalf("unexpected ValidationErr: %v", result.ValidationErr)
	}
	if len(result.Steps) != 4 {
		t.Errorf("expected 4 steps (2 apps + 2 resources), got %d: %+v", len(result.Steps), result.Steps)
	}
	if result.ManifestSet != set {
		t.Error("ManifestSet should be returned unchanged")
	}
}

func TestPlan_ByTeam_EmitsOnlyOwnerSteps(t *testing.T) {
	set := twoTeamSet()
	result := Plan(set, stubTeamStore{}, nil, ByTeam("team-a"))

	if result.Error != nil {
		t.Fatalf("unexpected Error: %v", result.Error)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps for team-a (alpha + db-a), got %d: %+v", len(result.Steps), result.Steps)
	}
	for _, step := range result.Steps {
		owner := stepOwner(set, step)
		if owner != "team-a" {
			t.Errorf("step %+v has owner %q, expected team-a", step, owner)
		}
	}
}

func TestPlan_ByTeam_UnknownTeam_ReturnsError(t *testing.T) {
	set := twoTeamSet()
	result := Plan(set, stubTeamStore{}, nil, ByTeam("team-ghost"))

	if result.Error == nil {
		t.Fatal("expected Error for unknown team")
	}
	msg := result.Error.Error()
	if !strings.Contains(msg, "team-ghost") {
		t.Errorf("error should name requested team, got: %v", result.Error)
	}
	if !strings.Contains(msg, "team-a") || !strings.Contains(msg, "team-b") {
		t.Errorf("error should list known teams, got: %v", result.Error)
	}
}

func TestPlan_ByApp_SingleStep(t *testing.T) {
	set := twoTeamSet()
	result := Plan(set, stubTeamStore{}, nil, ByApp("alpha"))

	if result.Error != nil {
		t.Fatalf("unexpected Error: %v", result.Error)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}
	step := result.Steps[0]
	if step.Kind != manifest.ApplicationKind || step.Name != "alpha" {
		t.Errorf("expected {Application, alpha}, got %+v", step)
	}
}

func TestPlan_ByResource_SingleStep(t *testing.T) {
	set := twoTeamSet()
	result := Plan(set, stubTeamStore{}, nil, ByResource("db-b"))

	if result.Error != nil {
		t.Fatalf("unexpected Error: %v", result.Error)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}
	step := result.Steps[0]
	if step.Kind != manifest.ResourceKind || step.Name != "db-b" {
		t.Errorf("expected {Resource, db-b}, got %+v", step)
	}
}

func TestPlan_ByApp_Missing_ReturnsError(t *testing.T) {
	set := twoTeamSet()
	result := Plan(set, stubTeamStore{}, nil, ByApp("nope"))
	if result.Error == nil {
		t.Fatal("expected Error for missing app")
	}
	if !strings.Contains(result.Error.Error(), "nope") {
		t.Errorf("error should name missing app, got: %v", result.Error)
	}
}

// TestPlan_ByTeam_CrossTeamDepResolution proves that even when a team-scoped
// filter restricts step emission, the full ManifestSet remains available as
// resolution context — so cross-team valueFrom references stay valid (Clarification Q1).
func TestPlan_ByTeam_CrossTeamDepResolution(t *testing.T) {
	set := NewManifestSet()
	// team-platform owns a shared resource exposed to platform.
	set.Resources["shared-db"] = &manifest.ResourceManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ResourceKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "shared-db", Owner: "team-platform", Access: []string{"team-a"}},
		Spec: manifest.ResourceSpec{
			Type:    "postgres",
			Version: "16",
			Outputs: []manifest.Output{
				{Name: "host", Value: "shared-db.svc"},
			},
			Networking: manifest.Networking{ExposeToPlatform: true},
		},
	}
	// team-a depends on it via valueFrom.
	set.Applications["alpha"] = &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "alpha", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Image: "nginx",
			Port:  80,
			Dependencies: []manifest.Dependency{
				{Kind: manifest.ResourceKind, Name: "shared-db", Owner: "team-platform"},
			},
			Env: []manifest.EnvVar{
				{Name: "DB_HOST", ValueFrom: "resource.shared-db.host"},
			},
		},
	}

	result := Plan(set, stubTeamStore{}, nil, ByTeam("team-a"))

	if result.Error != nil {
		t.Fatalf("unexpected Error: %v", result.Error)
	}
	if len(result.ValidationErr) != 0 {
		t.Fatalf("cross-team valueFrom should resolve, got ValidationErr: %v", result.ValidationErr)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step (only alpha re-deployed), got %d", len(result.Steps))
	}
	if result.Steps[0].Name != "alpha" {
		t.Errorf("expected alpha to be the only emitted step, got %+v", result.Steps[0])
	}
	// The shared-db must NOT appear as a step (different team).
	for _, step := range result.Steps {
		if step.Name == "shared-db" {
			t.Error("shared-db (team-platform) must not be emitted under ByTeam(team-a)")
		}
	}
}
