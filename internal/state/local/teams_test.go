package local

import (
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

func TestTeamStore_Operations(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewTeamStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create TeamStore: %v", err)
	}

	team := &manifest.TeamManifest{
		TypeMeta: manifest.TypeMeta{
			APIVersion: "shrine/v1",
			Kind:       manifest.TeamKind,
		},
		Metadata: manifest.Metadata{
			Name: "team-a",
		},
		Spec: manifest.TeamSpec{
			DisplayName: "Team Alpha",
			Contact:     "alpha@example.com",
		},
	}

	// 1. Save
	if err := store.SaveTeam(team); err != nil {
		t.Fatalf("SaveTeam failed: %v", err)
	}

	// Verify ResourceID was generated
	if team.Metadata.ResourceID == "" {
		t.Error("SaveTeam did not generate a ResourceID")
	}

	// 2. Load
	loaded, err := store.LoadTeam("team-a")
	if err != nil {
		t.Fatalf("LoadTeam failed: %v", err)
	}
	if loaded.Metadata.Name != team.Metadata.Name || loaded.Spec.DisplayName != team.Spec.DisplayName {
		t.Errorf("loaded team mismatch: got %+v, want %+v", loaded, team)
	}
	if loaded.Metadata.ResourceID != team.Metadata.ResourceID {
		t.Errorf("ResourceID mismatch after load: got %q, want %q", loaded.Metadata.ResourceID, team.Metadata.ResourceID)
	}

	// 3. List
	teams, err := store.ListTeams()
	if err != nil {
		t.Fatalf("ListTeams failed: %v", err)
	}
	if len(teams) != 1 || teams[0].Metadata.Name != "team-a" {
		t.Errorf("ListTeams: got %d teams, expected 1 with name team-a", len(teams))
	}

	// 4. Delete
	if err := store.DeleteTeam("team-a"); err != nil {
		t.Fatalf("DeleteTeam failed: %v", err)
	}

	_, err = store.LoadTeam("team-a")
	if err == nil {
		t.Fatal("expected LoadTeam to fail after DeleteTeam")
	}
}

func TestTeamStore_LoadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	store, _ := NewTeamStore(tmpDir)

	_, err := store.LoadTeam("ghost")
	if err == nil {
		t.Error("expected LoadTeam to fail for non-existent team")
	}
}

func TestTeamStore_ListPersistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create two teams in first store instance
	store1, _ := NewTeamStore(tmpDir)
	store1.SaveTeam(&manifest.TeamManifest{Metadata: manifest.Metadata{Name: "team1"}})
	store1.SaveTeam(&manifest.TeamManifest{Metadata: manifest.Metadata{Name: "team2"}})

	// Re-load in second store instance
	store2, _ := NewTeamStore(tmpDir)
	teams, err := store2.ListTeams()
	if err != nil {
		t.Fatalf("ListTeams failed: %v", err)
	}

	if len(teams) != 2 {
		t.Errorf("ListTeams: got %d teams, want 2", len(teams))
	}
}
