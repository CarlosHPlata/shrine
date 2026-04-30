package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanDir(t *testing.T) {
	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		result, err := ScanDir(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Shrine) != 0 {
			t.Errorf("Shrine len = %d, want 0", len(result.Shrine))
		}
		if len(result.Foreign) != 0 {
			t.Errorf("Foreign len = %d, want 0", len(result.Foreign))
		}
	})

	t.Run("dir with only non-YAML files", func(t *testing.T) {
		dir := t.TempDir()

		// Write a JSON file and make it unreadable to prove it's never opened
		jsonPath := filepath.Join(dir, "config.json")
		if err := os.WriteFile(jsonPath, []byte(`{"note": "should not be opened"}`), 0644); err != nil {
			t.Fatalf("WriteFile json: %v", err)
		}
		if err := os.Chmod(jsonPath, 0000); err != nil {
			t.Fatalf("Chmod: %v", err)
		}

		// Write a markdown file
		mdPath := filepath.Join(dir, "README.md")
		if err := os.WriteFile(mdPath, []byte("# Readme"), 0644); err != nil {
			t.Fatalf("WriteFile md: %v", err)
		}

		// Write an extensionless file
		extPath := filepath.Join(dir, "Makefile")
		if err := os.WriteFile(extPath, []byte("all:\n\t@echo hi"), 0644); err != nil {
			t.Fatalf("WriteFile extensionless: %v", err)
		}

		result, err := ScanDir(dir)
		if err != nil {
			t.Fatalf("unexpected error (extension filter must never open .json): %v", err)
		}
		if len(result.Shrine) != 0 {
			t.Errorf("Shrine len = %d, want 0", len(result.Shrine))
		}
		if len(result.Foreign) != 0 {
			t.Errorf("Foreign len = %d, want 0", len(result.Foreign))
		}
	})

	t.Run("dir with one valid and one foreign YAML", func(t *testing.T) {
		dir := t.TempDir()

		shrineContent := []byte("apiVersion: shrine/v1\nkind: Application\nmetadata:\n  name: myapp\n  owner: team\n")
		foreignContent := []byte("apiVersion: traefik.containo.us/v1alpha1\nkind: IngressRoute\n")

		shrinePath := filepath.Join(dir, "app.yaml")
		foreignPath := filepath.Join(dir, "route.yaml")

		if err := os.WriteFile(shrinePath, shrineContent, 0644); err != nil {
			t.Fatalf("WriteFile shrine: %v", err)
		}
		if err := os.WriteFile(foreignPath, foreignContent, 0644); err != nil {
			t.Fatalf("WriteFile foreign: %v", err)
		}

		result, err := ScanDir(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Shrine) != 1 {
			t.Errorf("Shrine len = %d, want 1", len(result.Shrine))
		}
		if len(result.Foreign) != 1 {
			t.Errorf("Foreign len = %d, want 1", len(result.Foreign))
		}
		if len(result.Shrine) == 1 && result.Shrine[0].Path != shrinePath {
			t.Errorf("Shrine[0].Path = %q, want %q", result.Shrine[0].Path, shrinePath)
		}
		if len(result.Shrine) == 1 && result.Shrine[0].TypeMeta.Kind != "Application" {
			t.Errorf("Shrine[0].TypeMeta.Kind = %q, want %q", result.Shrine[0].TypeMeta.Kind, "Application")
		}
		if len(result.Shrine) == 1 && result.Shrine[0].TypeMeta.APIVersion != "shrine/v1" {
			t.Errorf("Shrine[0].TypeMeta.APIVersion = %q, want %q", result.Shrine[0].TypeMeta.APIVersion, "shrine/v1")
		}
		if len(result.Foreign) == 1 && result.Foreign[0] != foreignPath {
			t.Errorf("Foreign[0] = %q, want %q", result.Foreign[0], foreignPath)
		}
	})

	t.Run("dir with one malformed YAML", func(t *testing.T) {
		dir := t.TempDir()

		brokenPath := filepath.Join(dir, "broken.yaml")
		if err := os.WriteFile(brokenPath, []byte("apiVersion: shrine/v1\nkind: [unclosed"), 0644); err != nil {
			t.Fatalf("WriteFile broken: %v", err)
		}

		_, err := ScanDir(dir)
		if err == nil {
			t.Fatalf("expected error for malformed YAML, got nil")
		}
		if !strings.Contains(err.Error(), brokenPath) {
			t.Errorf("error %q does not contain file path %q", err.Error(), brokenPath)
		}
	})

	t.Run("nested subdir with foreign YAML", func(t *testing.T) {
		dir := t.TempDir()

		// Create a nested subdirectory mirroring specsDir/traefik/
		subdir := filepath.Join(dir, "traefik")
		if err := os.MkdirAll(subdir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}

		foreignContent := []byte("entryPoints:\n  web:\n    address: \":80\"\n")
		foreignPath := filepath.Join(subdir, "traefik.yml")
		if err := os.WriteFile(foreignPath, foreignContent, 0644); err != nil {
			t.Fatalf("WriteFile foreign nested: %v", err)
		}

		result, err := ScanDir(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Shrine) != 0 {
			t.Errorf("Shrine len = %d, want 0", len(result.Shrine))
		}
		if len(result.Foreign) != 1 {
			t.Errorf("Foreign len = %d, want 1", len(result.Foreign))
		}
		if len(result.Foreign) == 1 && result.Foreign[0] != foreignPath {
			t.Errorf("Foreign[0] = %q, want %q", result.Foreign[0], foreignPath)
		}
	})
}
