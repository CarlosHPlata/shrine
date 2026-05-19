package planner

import (
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

func TestFilterConstructors(t *testing.T) {
	cases := []struct {
		name     string
		got      Filter
		wantKind FilterKind
		wantName string
	}{
		{"NoFilter", NoFilter(), FilterNone, ""},
		{"ByTeam", ByTeam("team-a"), FilterTeam, "team-a"},
		{"ByApp", ByApp("alpha"), FilterApp, "alpha"},
		{"ByResource", ByResource("db"), FilterRes, "db"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got.Kind != tc.wantKind {
				t.Errorf("kind = %v, want %v", tc.got.Kind, tc.wantKind)
			}
			if tc.got.Name != tc.wantName {
				t.Errorf("name = %q, want %q", tc.got.Name, tc.wantName)
			}
		})
	}
}

func TestFilterZeroValueIsNoFilter(t *testing.T) {
	if (Filter{}).Kind != FilterNone {
		t.Error("zero-value Filter should be FilterNone")
	}
}

func TestFilterValidate_NoFilter(t *testing.T) {
	set := NewManifestSet()
	if err := NoFilter().Validate(set); err != nil {
		t.Errorf("NoFilter on empty set should pass, got: %v", err)
	}
	if err := NoFilter().Validate(setWithTeams("team-a")); err != nil {
		t.Errorf("NoFilter on populated set should pass, got: %v", err)
	}
}

func TestFilterValidate_Team(t *testing.T) {
	set := setWithTeams("team-a", "team-b")

	if err := ByTeam("team-a").Validate(set); err != nil {
		t.Errorf("known team should pass, got: %v", err)
	}

	err := ByTeam("").Validate(set)
	if err == nil || !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("empty team name should fail with 'non-empty' message, got: %v", err)
	}

	err = ByTeam("markting").Validate(set)
	if err == nil {
		t.Fatal("unknown team should fail")
	}
	if !strings.Contains(err.Error(), `"markting"`) {
		t.Errorf("error should quote the requested team, got: %v", err)
	}
	if !strings.Contains(err.Error(), "team-a") || !strings.Contains(err.Error(), "team-b") {
		t.Errorf("error should list known teams, got: %v", err)
	}
}

func TestFilterValidate_Team_EmptySet(t *testing.T) {
	err := ByTeam("any").Validate(NewManifestSet())
	if err == nil {
		t.Fatal("expected error on empty set")
	}
	if !strings.Contains(err.Error(), "no Application or Resource manifests") {
		t.Errorf("expected empty-set error, got: %v", err)
	}
}

func TestFilterValidate_App(t *testing.T) {
	set := setWithApps("alpha", "beta")

	if err := ByApp("alpha").Validate(set); err != nil {
		t.Errorf("known app should pass, got: %v", err)
	}

	err := ByApp("").Validate(set)
	if err == nil || !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("empty app name should fail with 'non-empty', got: %v", err)
	}

	err = ByApp("gamma").Validate(set)
	if err == nil {
		t.Fatal("missing app should fail")
	}
	if !strings.Contains(err.Error(), `"gamma"`) {
		t.Errorf("error should quote missing app, got: %v", err)
	}
}

func TestFilterValidate_Resource(t *testing.T) {
	set := setWithResources("db", "cache")

	if err := ByResource("db").Validate(set); err != nil {
		t.Errorf("known resource should pass, got: %v", err)
	}

	err := ByResource("").Validate(set)
	if err == nil || !strings.Contains(err.Error(), "non-empty") {
		t.Errorf("empty resource name should fail with 'non-empty', got: %v", err)
	}

	err = ByResource("queue").Validate(set)
	if err == nil {
		t.Fatal("missing resource should fail")
	}
	if !strings.Contains(err.Error(), `"queue"`) {
		t.Errorf("error should quote missing resource, got: %v", err)
	}
}

func TestFilterValidate_UnknownKind(t *testing.T) {
	bad := Filter{Kind: FilterKind(99), Name: "x"}
	err := bad.Validate(setWithTeams("team-a"))
	if err == nil || !strings.Contains(err.Error(), "unknown filter kind") {
		t.Errorf("expected unknown-kind error, got: %v", err)
	}
}

func TestDiscoveredOwners_SortedAndDedup(t *testing.T) {
	set := NewManifestSet()
	set.Applications["alpha"] = appWithOwner("zebra")
	set.Applications["beta"] = appWithOwner("apple")
	set.Resources["db"] = resWithOwner("apple")
	set.Resources["cache"] = resWithOwner("mango")

	got := discoveredOwners(set)
	want := []string{"apple", "mango", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// --- helpers ---

func setWithTeams(owners ...string) *ManifestSet {
	set := NewManifestSet()
	for i, owner := range owners {
		name := "app-" + owner
		set.Applications[name] = appWithOwner(owner)
		_ = i
	}
	return set
}

func setWithApps(names ...string) *ManifestSet {
	set := NewManifestSet()
	for _, name := range names {
		set.Applications[name] = appWithOwner("team-default")
	}
	return set
}

func setWithResources(names ...string) *ManifestSet {
	set := NewManifestSet()
	for _, name := range names {
		set.Resources[name] = resWithOwner("team-default")
	}
	return set
}

func appWithOwner(owner string) *manifest.ApplicationManifest {
	return &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "app-" + owner, Owner: owner},
		Spec:     manifest.ApplicationSpec{Image: "nginx", Port: 80},
	}
}

func resWithOwner(owner string) *manifest.ResourceManifest {
	return &manifest.ResourceManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ResourceKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "res-" + owner, Owner: owner},
		Spec:     manifest.ResourceSpec{Type: "postgres", Version: "16"},
	}
}
