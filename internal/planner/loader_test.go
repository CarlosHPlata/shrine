package planner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// nullTeamStore is a minimal state.TeamStore stub for unit tests that only need
// the store interface to satisfy PlanSingle's signature without any real I/O.
type nullTeamStore struct{}

func (nullTeamStore) SaveTeam(*manifest.TeamManifest) error              { return nil }
func (nullTeamStore) LoadTeam(string) (*manifest.TeamManifest, error)    { return nil, fmt.Errorf("not found") }
func (nullTeamStore) ListTeams() ([]*manifest.TeamManifest, error)       { return nil, nil }
func (nullTeamStore) DeleteTeam(string) error                            { return nil }

func TestLoadDir(t *testing.T) {
	tmp := t.TempDir()

	// 1. Create a valid App
	appYAML := `
apiVersion: shrine/v1
kind: Application
metadata:
  name: my-app
  owner: team-a
spec:
  image: nginx
  port: 80
`
	if err := os.WriteFile(filepath.Join(tmp, "app.yml"), []byte(appYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Create a valid Resource (.yaml extension)
	resYAML := `
apiVersion: shrine/v1
kind: Resource
metadata:
  name: my-db
  owner: team-a
spec:
  type: postgres
  version: "16"
`
	if err := os.WriteFile(filepath.Join(tmp, "db.yaml"), []byte(resYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Create a Team (should be skipped)
	teamYAML := `
apiVersion: shrine/v1
kind: Team
metadata:
  name: team-a
spec:
  displayName: "Team A"
  contact: "a@a.com"
`
	if err := os.WriteFile(filepath.Join(tmp, "team.yml"), []byte(teamYAML), 0644); err != nil {
		t.Fatal(err)
	}

	set, err := LoadDir(tmp)
	if err != nil {
		t.Fatalf("LoadDir failed: %v", err)
	}

	if len(set.Applications) != 1 {
		t.Errorf("expected 1 application, got %d", len(set.Applications))
	}
	if _, ok := set.Applications["my-app"]; !ok {
		t.Error("my-app not found in Applications map")
	}

	if len(set.Resources) != 1 {
		t.Errorf("expected 1 resource, got %d", len(set.Resources))
	}
	if _, ok := set.Resources["my-db"]; !ok {
		t.Error("my-db not found in Resources map")
	}
}

func TestLoadDir_Duplicates(t *testing.T) {
	tmp := t.TempDir()

	yaml := `
apiVersion: shrine/v1
kind: Resource
metadata:
  name: conflict
  owner: team-a
spec:
  type: redis
  version: "7"
`
	_ = os.WriteFile(filepath.Join(tmp, "res1.yml"), []byte(yaml), 0644)
	_ = os.WriteFile(filepath.Join(tmp, "res2.yml"), []byte(yaml), 0644)

	_, err := LoadDir(tmp)
	if err == nil {
		t.Error("expected error on duplicate resource names, got nil")
	}
}

func TestLoadDir_ValidPlusForeign(t *testing.T) {
	tmp := t.TempDir()

	// A valid shrine Application manifest.
	appYAML := `
apiVersion: shrine/v1
kind: Application
metadata:
  name: foreign-test-app
  owner: team-b
spec:
  image: nginx
  port: 80
`
	if err := os.WriteFile(filepath.Join(tmp, "app.yml"), []byte(appYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// A foreign YAML file (no apiVersion — mirrors a Traefik routing file).
	foreignYAML := `
entryPoints:
  web:
    address: ":80"
providers:
  file:
    directory: /etc/traefik/dynamic
`
	if err := os.WriteFile(filepath.Join(tmp, "traefik.yml"), []byte(foreignYAML), 0644); err != nil {
		t.Fatal(err)
	}

	set, err := LoadDir(tmp)
	if err != nil {
		t.Fatalf("LoadDir failed: %v", err)
	}
	if len(set.Applications) != 1 {
		t.Errorf("expected 1 application, got %d", len(set.Applications))
	}
	if _, ok := set.Applications["foreign-test-app"]; !ok {
		t.Error("foreign-test-app not found in Applications map")
	}
	// The foreign file must not have been turned into any manifest entry.
	if len(set.Resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(set.Resources))
	}
}

func TestLoadDir_ForeignOnly(t *testing.T) {
	tmp := t.TempDir()

	// Only a foreign YAML file — no shrine manifests at all.
	foreignYAML := `
http:
  routers:
    my-router:
      rule: "Host('example.com')"
`
	if err := os.WriteFile(filepath.Join(tmp, "foreign-only.yml"), []byte(foreignYAML), 0644); err != nil {
		t.Fatal(err)
	}

	set, err := LoadDir(tmp)
	if err != nil {
		t.Fatalf("LoadDir returned unexpected error: %v", err)
	}
	if len(set.Applications) != 0 {
		t.Errorf("expected 0 applications, got %d", len(set.Applications))
	}
	if len(set.Resources) != 0 {
		t.Errorf("expected 0 resources, got %d", len(set.Resources))
	}
}

func TestLoadDir_ValidPlusMalformed(t *testing.T) {
	tmp := t.TempDir()

	// A valid shrine Application manifest.
	appYAML := `
apiVersion: shrine/v1
kind: Application
metadata:
  name: valid-app
  owner: team-c
spec:
  image: nginx
  port: 80
`
	if err := os.WriteFile(filepath.Join(tmp, "app.yml"), []byte(appYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// A shrine manifest (apiVersion: shrine/v1) with malformed YAML — ScanDir will
	// error on this file because yaml.Unmarshal fails before classification.
	malformedYAML := `apiVersion: shrine/v1
kind: [unclosed`
	if err := os.WriteFile(filepath.Join(tmp, "broken.yaml"), []byte(malformedYAML), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadDir(tmp)
	if err == nil {
		t.Fatal("expected LoadDir to return an error for the malformed file, got nil")
	}
	if !strings.Contains(err.Error(), filepath.Join(tmp, "broken.yaml")) {
		t.Errorf("expected error to mention broken.yaml path, got: %v", err)
	}
}

// TestPlanSingle_BadKind_WrapsFilePath pins the FR-007 guarantee that PlanSingle
// includes the target file path in the error message when manifest.Parse fails on
// a shrine manifest with an unknown kind. This was a regression introduced during
// the Phase 3/4 refactor where PlanSingle returned the bare parse error without
// wrapping it with the file path — T036 restores the wrapping.
func TestPlanSingle_BadKind_WrapsFilePath(t *testing.T) {
	tmp := t.TempDir()

	typoYAML := `apiVersion: shrine/v1
kind: Aplication
metadata:
  name: typo-app
  owner: team-x
spec:
  image: traefik/whoami
  port: 80
`
	typoFile := filepath.Join(tmp, "typo.yaml")
	if err := os.WriteFile(typoFile, []byte(typoYAML), 0644); err != nil {
		t.Fatal(err)
	}

	result := PlanSingle(typoFile, "", nullTeamStore{})
	if result.Error == nil {
		t.Fatal("expected PlanSingle to return an error for bad-kind manifest, got nil")
	}

	msg := result.Error.Error()
	if !strings.Contains(msg, "typo.yaml") {
		t.Errorf("expected error to contain file name %q, got: %v", "typo.yaml", msg)
	}
	if !strings.Contains(msg, "Aplication") {
		t.Errorf("expected error to contain offending kind %q, got: %v", "Aplication", msg)
	}
}
