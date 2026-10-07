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
		Image:   "docker.io/traefik/whoami:latest",
	}, nil
}

func TestStatusAutoTeam(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "shrine-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := local.NewLocalStore(tmpDir, nil)
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

func TestFormatStatusTable_AddsImageBetweenStatusAndImageID(t *testing.T) {
	out := formatStatusTable([]containerStatusRow{{
		Name:    "whoami-pinned",
		Kind:    "Application",
		Running: true,
		Status:  "running",
		Image:   "127.0.0.1:5000/shrine/whoami@3f2a9c1b4d7e",
		ImageID: "sha256:3f2a9c1b4d7e",
	}})

	header, separator, rows := tableLines(t, out)
	assertInOrder(t, header, "NAME", "KIND", "RUNNING", "STATUS", "IMAGE", "IMAGE ID")
	if len(separator) != len(header) || strings.Trim(separator, "-") != "" {
		t.Errorf("separator must be dashes as long as the header, got %q", separator)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d:\n%s", len(rows), out)
	}
	assertInOrder(t, rows[0], "whoami-pinned", "Application", "true", "running", "127.0.0.1:5000/shrine/whoami@3f2a9c1b4d7e", "sha256:3f2a9c1b4d7e")
}

func TestShortImageReference(t *testing.T) {
	const digest = "sha256:3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a"
	cases := map[string]string{
		"":                                       "-",
		"repo@" + digest:                         "repo@3f2a9c1b4d7e",
		"127.0.0.1:5000/shrine/whoami@" + digest: "127.0.0.1:5000/shrine/whoami@3f2a9c1b4d7e",
		"docker.io/traefik/whoami:latest":        "docker.io/traefik/whoami:latest",
		"postgres":                               "postgres",
	}
	for ref, want := range cases {
		t.Run(ref, func(t *testing.T) {
			if got := shortImageReference(ref); got != want {
				t.Errorf("shortImageReference(%q) = %q, want %q", ref, got, want)
			}
		})
	}
}

func TestInspectDeployments_FillsTheImageCell(t *testing.T) {
	rows, err := inspectDeployments([]teamedDeployment{{
		Team:       "lab",
		Deployment: state.Deployment{Name: "api", Kind: manifest.ApplicationKind, ContainerID: "c1"},
	}}, &mockBackend{})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Image != "docker.io/traefik/whoami:latest" {
		t.Errorf("Image = %q, want the reference the backend reported", rows[0].Image)
	}
}
