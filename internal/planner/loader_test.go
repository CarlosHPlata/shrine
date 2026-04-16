package planner

import (
	"os"
	"path/filepath"
	"testing"
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
