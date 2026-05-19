package planner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

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

func TestNewManifestSet(t *testing.T) {
	set := NewManifestSet()
	if set == nil {
		t.Fatal("NewManifestSet returned nil")
	}
	if set.Applications == nil {
		t.Error("Applications map not allocated")
	}
	if set.Resources == nil {
		t.Error("Resources map not allocated")
	}
	if len(set.Applications) != 0 || len(set.Resources) != 0 {
		t.Error("expected empty maps")
	}
}

func TestMergeManifest_Application(t *testing.T) {
	set := NewManifestSet()
	m := &manifest.Manifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Application: &manifest.ApplicationManifest{
			TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
			Metadata: manifest.Metadata{Name: "alpha", Owner: "team-a"},
		},
	}
	if err := set.MergeManifest(m, "alpha.yaml"); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	if _, ok := set.Applications["alpha"]; !ok {
		t.Error("alpha not in Applications map")
	}

	if err := set.MergeManifest(m, "alpha.yaml"); err == nil {
		t.Fatal("expected duplicate merge to error, got nil")
	} else if !errors.Is(err, ErrDuplicateManifest) {
		t.Errorf("expected errors.Is(err, ErrDuplicateManifest), got: %v", err)
	}
}

func TestMergeManifest_Resource_Duplicate(t *testing.T) {
	set := NewManifestSet()
	m := &manifest.Manifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ResourceKind, APIVersion: "shrine/v1"},
		Resource: &manifest.ResourceManifest{
			TypeMeta: manifest.TypeMeta{Kind: manifest.ResourceKind, APIVersion: "shrine/v1"},
			Metadata: manifest.Metadata{Name: "db", Owner: "team-a"},
		},
	}
	if err := set.MergeManifest(m, "db.yaml"); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	err := set.MergeManifest(m, "db.yaml")
	if !errors.Is(err, ErrDuplicateManifest) {
		t.Errorf("expected ErrDuplicateManifest, got: %v", err)
	}
}

func TestMergeManifest_TeamKindIsNoOp(t *testing.T) {
	set := NewManifestSet()
	m := &manifest.Manifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.TeamKind, APIVersion: "shrine/v1"},
		Team: &manifest.TeamManifest{
			TypeMeta: manifest.TypeMeta{Kind: manifest.TeamKind, APIVersion: "shrine/v1"},
			Metadata: manifest.Metadata{Name: "team-a"},
		},
	}
	if err := set.MergeManifest(m, "team.yaml"); err != nil {
		t.Fatalf("team merge should be no-op, got error: %v", err)
	}
	if len(set.Applications) != 0 || len(set.Resources) != 0 {
		t.Error("Team merge should not populate Applications or Resources")
	}
}

func TestMergeManifest_UnsupportedKind(t *testing.T) {
	set := NewManifestSet()
	m := &manifest.Manifest{TypeMeta: manifest.TypeMeta{Kind: "Sidecar", APIVersion: "shrine/v1"}}
	err := set.MergeManifest(m, "sidecar.yaml")
	if err == nil {
		t.Fatal("expected error for unsupported kind, got nil")
	}
	if !strings.Contains(err.Error(), "Sidecar") || !strings.Contains(err.Error(), "sidecar.yaml") {
		t.Errorf("error should name the kind and file, got: %v", err)
	}
}
