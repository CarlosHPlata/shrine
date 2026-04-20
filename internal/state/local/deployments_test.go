package local

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/state"
)

func TestDeploymentStore_LoadTeam(t *testing.T) {
	tmpDir := t.TempDir()
	team := "team-a"
	teamDir := filepath.Join(tmpDir, team)
	if err := os.MkdirAll(teamDir, 0700); err != nil {
		t.Fatalf("failed to create team dir: %v", err)
	}

	data := `
# Team deployments
container web abc123
  # indented comment
container api def456 # inline comment

invalid-line
service db ghi789
`
	if err := os.WriteFile(filepath.Join(teamDir, "deployments.txt"), []byte(data), 0600); err != nil {
		t.Fatalf("failed to setup test file: %v", err)
	}

	store, err := NewDeploymentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewDeploymentStore failed: %v", err)
	}

	s := store.(*DeploymentStore)
	deployments, err := s.loadTeam(team)
	if err != nil {
		t.Fatalf("loadTeam failed: %v", err)
	}

	expected := map[string]state.Deployment{
		"web": {Kind: "container", Name: "web", ContainerID: "abc123"},
		"api": {Kind: "container", Name: "api", ContainerID: "def456"},
		"db":  {Kind: "service", Name: "db", ContainerID: "ghi789"},
	}

	if len(deployments) != len(expected) {
		t.Errorf("got %d deployments, want %d", len(deployments), len(expected))
	}

	for name, want := range expected {
		got, ok := deployments[name]
		if !ok {
			t.Errorf("deployment %q missing", name)
			continue
		}
		if got != want {
			t.Errorf("deployment %q: got %+v, want %+v", name, got, want)
		}
	}
}

func TestDeploymentStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	team := "team-x"

	// 1. Record a deployment
	store1, err := NewDeploymentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewDeploymentStore failed: %v", err)
	}
	dep := state.Deployment{Kind: "container", Name: "web", ContainerID: "abc123"}
	if err := store1.Record(team, dep); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	// 2. Re-load in a new store instance
	store2, err := NewDeploymentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewDeploymentStore (re-load) failed: %v", err)
	}

	deployments, err := store2.List(team)
	if err != nil {
		t.Fatalf("List on re-loaded store failed: %v", err)
	}

	if len(deployments) != 1 || deployments[0] != dep {
		t.Errorf("persistence failed: got %+v, want [%+v]", deployments, dep)
	}
}

func TestDeploymentStore_Interface(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewDeploymentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewDeploymentStore failed: %v", err)
	}
	team := "team-a"

	web := state.Deployment{Kind: "container", Name: "web", ContainerID: "abc123"}
	api := state.Deployment{Kind: "container", Name: "api", ContainerID: "def456"}

	// 1. Record (new)
	if err := store.Record(team, web); err != nil {
		t.Fatalf("Record web failed: %v", err)
	}
	if err := store.Record(team, api); err != nil {
		t.Fatalf("Record api failed: %v", err)
	}

	// 2. Record (update existing)
	webUpdated := state.Deployment{Kind: "container", Name: "web", ContainerID: "newid"}
	if err := store.Record(team, webUpdated); err != nil {
		t.Fatalf("Record web update failed: %v", err)
	}

	// 3. List
	got, err := store.List(team)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d deployments, want 2", len(got))
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })
	want := []state.Deployment{api, webUpdated}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("List[%d]: got %+v, want %+v", i, got[i], want[i])
		}
	}

	// 4. Remove
	if err := store.Remove(team, "web"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	got, err = store.List(team)
	if err != nil {
		t.Fatalf("List after Remove failed: %v", err)
	}
	if len(got) != 1 || got[0] != api {
		t.Errorf("after Remove: got %+v, want [%+v]", got, api)
	}

	// 5. Remove non-existent (should not error)
	if err := store.Remove(team, "non-existent"); err != nil {
		t.Errorf("Remove non-existent should not error: %v", err)
	}
}

func TestDeploymentStore_EmptyTeam(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewDeploymentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewDeploymentStore failed: %v", err)
	}

	got, err := store.List("unknown-team")
	if err != nil {
		t.Fatalf("List on empty team failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d deployments for unknown team, want 0", len(got))
	}
}

func TestDeploymentStore_TeamIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewDeploymentStore(tmpDir)
	if err != nil {
		t.Fatalf("NewDeploymentStore failed: %v", err)
	}

	depA := state.Deployment{Kind: "container", Name: "web", ContainerID: "aaa"}
	depB := state.Deployment{Kind: "container", Name: "web", ContainerID: "bbb"}

	if err := store.Record("team-a", depA); err != nil {
		t.Fatalf("Record team-a failed: %v", err)
	}
	if err := store.Record("team-b", depB); err != nil {
		t.Fatalf("Record team-b failed: %v", err)
	}

	gotA, _ := store.List("team-a")
	if len(gotA) != 1 || gotA[0] != depA {
		t.Errorf("team-a: got %+v, want [%+v]", gotA, depA)
	}

	gotB, _ := store.List("team-b")
	if len(gotB) != 1 || gotB[0] != depB {
		t.Errorf("team-b: got %+v, want [%+v]", gotB, depB)
	}
}
