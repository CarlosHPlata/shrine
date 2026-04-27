package handler

import (
	"os"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/CarlosHPlata/shrine/internal/state/local"
)

type mockBackend struct {
	engine.ContainerBackend
}

func (m *mockBackend) InspectContainer(id string) (engine.ContainerInfo, error) {
	return engine.ContainerInfo{
		Running: true,
		Status:  "running",
		ImageID: "sha256:12345678901234567890",
	}, nil
}

func TestStatusAutoTeam(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shrine-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := local.NewLocalStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Setup teams
	teamA := &manifest.TeamManifest{Metadata: manifest.Metadata{Name: "team-a"}}
	teamB := &manifest.TeamManifest{Metadata: manifest.Metadata{Name: "team-b"}}
	if err := store.Teams.SaveTeam(teamA); err != nil {
		t.Fatal(err)
	}
	if err := store.Teams.SaveTeam(teamB); err != nil {
		t.Fatal(err)
	}

	// Setup deployments
	dep1 := state.Deployment{Name: "app1", Kind: manifest.ApplicationKind, ContainerID: "c1"}
	dep2 := state.Deployment{Name: "app2", Kind: manifest.ApplicationKind, ContainerID: "c2"}
	dep3 := state.Deployment{Name: "app1", Kind: manifest.ApplicationKind, ContainerID: "c3"}

	if err := store.Deployments.Record("team-a", dep1); err != nil {
		t.Fatal(err)
	}
	if err := store.Deployments.Record("team-b", dep2); err != nil {
		t.Fatal(err)
	}
	if err := store.Deployments.Record("team-b", dep3); err != nil {
		t.Fatal(err)
	}

	backend := &mockBackend{}

	t.Run("StatusApplication", func(t *testing.T) {
		tests := []struct {
			name    string
			team    string
			appName string
			wantErr string
		}{
			{
				name:    "Found in team-a",
				team:    "",
				appName: "app1",
				wantErr: "ambiguous", // app1 is in both team-a and team-b
			},
			{
				name:    "Found unique in team-b",
				team:    "",
				appName: "app2",
				wantErr: "",
			},
			{
				name:    "Found with explicit team",
				team:    "team-a",
				appName: "app1",
				wantErr: "",
			},
			{
				name:    "Not found",
				team:    "",
				appName: "nonexistent",
				wantErr: "not found",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := StatusApplication(tt.team, tt.appName, store, backend)
				if tt.wantErr == "" {
					if err != nil {
						t.Errorf("StatusApplication() error = %v, wantErr %v", err, tt.wantErr)
					}
				} else {
					if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
						t.Errorf("StatusApplication() error = %v, wantErr %v", err, tt.wantErr)
					}
				}
			})
		}
	})

	t.Run("StatusResource", func(t *testing.T) {
		res1 := state.Deployment{Name: "res1", Kind: manifest.ResourceKind, ContainerID: "r1"}
		if err := store.Deployments.Record("team-a", res1); err != nil {
			t.Fatal(err)
		}

		err := StatusResource("", "res1", store, backend)
		if err != nil {
			t.Errorf("StatusResource() error = %v, wantErr nil", err)
		}

		err = StatusResource("", "nonexistent", store, backend)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("StatusResource() error = %v, wantErr not found", err)
		}
	})
}
