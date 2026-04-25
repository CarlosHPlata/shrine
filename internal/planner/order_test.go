package planner

import (
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

func TestOrder(t *testing.T) {
	t.Run("topological property", func(t *testing.T) {
		set := &ManifestSet{
			Applications: map[string]*manifest.ApplicationManifest{
				"app-z": {
					Metadata: manifest.Metadata{Name: "app-z"},
					Spec: manifest.ApplicationSpec{
						Dependencies: []manifest.Dependency{
							{Kind: manifest.ResourceKind, Name: "res-b"},
							{Kind: manifest.ApplicationKind, Name: "app-a"},
						},
					},
				},
				"app-a": {Metadata: manifest.Metadata{Name: "app-a"}},
			},
			Resources: map[string]*manifest.ResourceManifest{
				"res-b": {Metadata: manifest.Metadata{Name: "res-b"}},
			},
		}

		actual, err := Order(set)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Helper to find index of a step
		indexOf := func(kind, name string) int {
			for i, step := range actual {
				if step.Kind == kind && step.Name == name {
					return i
				}
			}
			t.Fatalf("step %s:%s not found in plan", kind, name)
			return -1
		}

		// Assert dependencies appear before their dependents
		if indexOf(manifest.ResourceKind, "res-b") > indexOf(manifest.ApplicationKind, "app-z") {
			t.Errorf("res-b should appear before app-z")
		}
		if indexOf(manifest.ApplicationKind, "app-a") > indexOf(manifest.ApplicationKind, "app-z") {
			t.Errorf("app-a should appear before app-z")
		}
	})

	t.Run("dependency cycle", func(t *testing.T) {
		set := &ManifestSet{
			Applications: map[string]*manifest.ApplicationManifest{
				"app-1": {
					Metadata: manifest.Metadata{Name: "app-1"},
					Spec: manifest.ApplicationSpec{
						Dependencies: []manifest.Dependency{
							{Kind: manifest.ApplicationKind, Name: "app-2"},
						},
					},
				},
				"app-2": {
					Metadata: manifest.Metadata{Name: "app-2"},
					Spec: manifest.ApplicationSpec{
						Dependencies: []manifest.Dependency{
							{Kind: manifest.ApplicationKind, Name: "app-1"},
						},
					},
				},
			},
		}

		_, err := Order(set)
		if err == nil {
			t.Fatal("expected dependency cycle error, got nil")
		}
		if !strings.Contains(err.Error(), "dependency cycle") {
			t.Errorf("expected error containing 'dependency cycle', got: %v", err)
		}
	})

	t.Run("independent nodes", func(t *testing.T) {
		set := &ManifestSet{
			Applications: map[string]*manifest.ApplicationManifest{
				"app-a": {Metadata: manifest.Metadata{Name: "app-a"}},
				"app-b": {Metadata: manifest.Metadata{Name: "app-b"}},
			},
			Resources: map[string]*manifest.ResourceManifest{
				"res-1": {Metadata: manifest.Metadata{Name: "res-1"}},
				"res-2": {Metadata: manifest.Metadata{Name: "res-2"}},
			},
		}

		actual, err := Order(set)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(actual) != 4 {
			t.Errorf("expected 4 steps, got %d", len(actual))
		}

		// Ensure all are present
		found := make(map[string]bool)
		for _, step := range actual {
			found[step.Kind+":"+step.Name] = true
		}

		expected := []string{"Resource:res-1", "Resource:res-2", "Application:app-a", "Application:app-b"}
		for _, exp := range expected {
			if !found[exp] {
				t.Errorf("missing expected step: %s", exp)
			}
		}
	})
}
