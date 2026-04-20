package planner

import (
	"errors"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// MockStore implements state.TeamStore for testing
type MockStore struct {
	Teams map[string]*manifest.TeamManifest
}

func (m *MockStore) SaveTeam(team *manifest.TeamManifest) error { return nil }
func (m *MockStore) LoadTeam(name string) (*manifest.TeamManifest, error) {
	team, ok := m.Teams[name]
	if !ok {
		return nil, errors.New("team not found")
	}
	return team, nil
}
func (m *MockStore) ListTeams() ([]*manifest.TeamManifest, error) { return nil, nil }
func (m *MockStore) DeleteTeam(name string) error                 { return nil }

func TestResolve(t *testing.T) {
	// Setup standard team
	teamA := &manifest.TeamManifest{
		Metadata: manifest.Metadata{Name: "team-a"},
		Spec: manifest.TeamSpec{
			Quotas: manifest.Quotas{
				MaxApps:              2,
				MaxResources:         1,
				AllowedResourceTypes: []string{"postgres"},
			},
		},
	}

	store := &MockStore{
		Teams: map[string]*manifest.TeamManifest{
			"team-a": teamA,
			"team-b": {Metadata: manifest.Metadata{Name: "team-b"}},
		},
	}

	t.Run("successful resolution", func(t *testing.T) {
		set := &ManifestSet{
			Applications: map[string]*manifest.ApplicationManifest{
				"app1": {
					Metadata: manifest.Metadata{Name: "app1", Owner: "team-a"},
					Spec: manifest.ApplicationSpec{
						Dependencies: []manifest.Dependency{
							{Kind: "Resource", Name: "db1", Owner: "team-a"},
						},
					},
				},
			},
			Resources: map[string]*manifest.ResourceManifest{
				"db1": {
					Metadata: manifest.Metadata{Name: "db1", Owner: "team-a"},
					Spec:     manifest.ResourceSpec{Type: "postgres"},
				},
			},
		}

		errs := Resolve(set, store)
		if len(errs) > 0 {
			t.Errorf("expected no errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("missing dependency", func(t *testing.T) {
		set := &ManifestSet{
			Applications: map[string]*manifest.ApplicationManifest{
				"app1": {
					Metadata: manifest.Metadata{Name: "app1", Owner: "team-a"},
					Spec: manifest.ApplicationSpec{
						Dependencies: []manifest.Dependency{
							{Kind: "Resource", Name: "nonexistent", Owner: "team-a"},
						},
					},
				},
			},
			Resources: make(map[string]*manifest.ResourceManifest),
		}

		errs := Resolve(set, store)
		if len(errs) == 0 {
			t.Error("expected missing dependency error, got none")
		}
	})

	t.Run("access denied", func(t *testing.T) {
		set := &ManifestSet{
			Applications: map[string]*manifest.ApplicationManifest{
				"app1": {
					Metadata: manifest.Metadata{Name: "app1", Owner: "team-a"},
					Spec: manifest.ApplicationSpec{
						Dependencies: []manifest.Dependency{
							{Kind: "Resource", Name: "db1", Owner: "team-b"},
						},
					},
				},
			},
			Resources: map[string]*manifest.ResourceManifest{
				"db1": {
					Metadata: manifest.Metadata{Name: "db1", Owner: "team-b", Access: []string{"team-c"}},
					Spec:     manifest.ResourceSpec{Type: "postgres"},
				},
			},
		}

		errs := Resolve(set, store)
		if len(errs) == 0 {
			t.Error("expected access denied error, got none")
		}
	})

	t.Run("access granted", func(t *testing.T) {
		set := &ManifestSet{
			Applications: map[string]*manifest.ApplicationManifest{
				"app1": {
					Metadata: manifest.Metadata{Name: "app1", Owner: "team-a"},
					Spec: manifest.ApplicationSpec{
						Dependencies: []manifest.Dependency{
							{Kind: "Resource", Name: "db1", Owner: "team-b"},
						},
					},
				},
			},
			Resources: map[string]*manifest.ResourceManifest{
				"db1": {
					Metadata: manifest.Metadata{Name: "db1", Owner: "team-b", Access: []string{"team-a"}},
					Spec:     manifest.ResourceSpec{Type: "postgres"},
				},
			},
		}

		errs := Resolve(set, store)
		if len(errs) > 0 {
			t.Errorf("expected no errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("resource owner mismatch", func(t *testing.T) {
		set := &ManifestSet{
			Applications: map[string]*manifest.ApplicationManifest{
				"app1": {
					Metadata: manifest.Metadata{Name: "app1", Owner: "team-a"},
					Spec: manifest.ApplicationSpec{
						Dependencies: []manifest.Dependency{
							{Kind: "Resource", Name: "db1", Owner: "team-a"}, // Specifies team-a
						},
					},
				},
			},
			Resources: map[string]*manifest.ResourceManifest{
				"db1": {
					Metadata: manifest.Metadata{Name: "db1", Owner: "team-b"}, // Actual is team-b
				},
			},
		}

		errs := Resolve(set, store)
		found := false
		for _, err := range errs {
			if strings.Contains(err.Error(), "owned by \"team-b\", but manifest specifies owner \"team-a\"") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected owner mismatch error, got: %v", errs)
		}
	})

	t.Run("max apps quota exceeded", func(t *testing.T) {
		set := &ManifestSet{
			Applications: map[string]*manifest.ApplicationManifest{
				"app1": {Metadata: manifest.Metadata{Name: "app1", Owner: "team-a"}},
				"app2": {Metadata: manifest.Metadata{Name: "app2", Owner: "team-a"}},
				"app3": {Metadata: manifest.Metadata{Name: "app3", Owner: "team-a"}},
			},
		}

		errs := Resolve(set, store)
		found := false
		for _, err := range errs {
			if strings.Contains(err.Error(), "exceeds MaxApps quota") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected MaxApps quota error, got: %v", errs)
		}
	})

	t.Run("disallowed resource type", func(t *testing.T) {
		set := &ManifestSet{
			Resources: map[string]*manifest.ResourceManifest{
				"db1": {
					Metadata: manifest.Metadata{Name: "db1", Owner: "team-a"},
					Spec:     manifest.ResourceSpec{Type: "mysql"}, // team-a only allows postgres
				},
			},
		}

		errs := Resolve(set, store)
		found := false
		for _, err := range errs {
			if strings.Contains(err.Error(), "not allowed by quota") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected disallowed type error, got: %v", errs)
		}
	})

	t.Run("valueFrom validation", func(t *testing.T) {
		set := &ManifestSet{
			Applications: map[string]*manifest.ApplicationManifest{
				"app1": {
					Metadata: manifest.Metadata{Name: "app1", Owner: "team-a"},
					Spec: manifest.ApplicationSpec{
						Env: []manifest.EnvVar{
							{Name: "DB_URL", ValueFrom: "resource.db1.url"},       // Valid
							{Name: "INVALID_FMT", ValueFrom: "resource.db1"},      // Invalid format
							{Name: "WRONG_PREFIX", ValueFrom: "config.db1.url"},  // Wrong prefix
							{Name: "MISSING_RES", ValueFrom: "resource.db2.url"},  // Missing resource
							{Name: "MISSING_OUT", ValueFrom: "resource.db1.host"}, // Missing output
						},
					},
				},
			},
			Resources: map[string]*manifest.ResourceManifest{
				"db1": {
					Metadata: manifest.Metadata{Name: "db1", Owner: "team-a"},
					Spec: manifest.ResourceSpec{
						Outputs: []manifest.Output{{Name: "url"}},
					},
				},
			},
		}

		errs := Resolve(set, store)
		
		expectedErrors := []string{
			"invalid valueFrom format \"resource.db1\"",
			"invalid valueFrom format \"config.db1.url\"",
			"references missing resource \"db2\"",
			"references non-existent output \"host\" on resource \"db1\"",
		}

		for _, expected := range expectedErrors {
			found := false
			for _, err := range errs {
				if strings.Contains(err.Error(), expected) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error containing %q, but not found in: %v", expected, errs)
			}
		}
	})

	t.Run("template variable validation", func(t *testing.T) {
		set := &ManifestSet{
			Resources: map[string]*manifest.ResourceManifest{
				"db1": {
					Metadata: manifest.Metadata{Name: "db1", Owner: "team-a"},
					Spec: manifest.ResourceSpec{
						Type:    "postgres",
						Version: "16",
						Outputs: []manifest.Output{
							{Name: "host"},
							{Name: "port", Value: "5432"},
							{Name: "password", Generated: true},
							// Valid: references siblings + built-ins.
							{Name: "url", Template: "{{.team}}/{{.name}}://{{.host}}:{{.port}}:{{.password}}"},
							// Invalid: references unknown variable.
							{Name: "bad", Template: "{{.missing}}"},
							// Invalid: syntax error.
							{Name: "broken", Template: "{{.host"},
						},
					},
				},
			},
		}

		errs := Resolve(set, store)

		expectedErrors := []string{
			"template output \"bad\" references unknown variable \"missing\"",
			"template output \"broken\" has invalid syntax",
		}
		for _, expected := range expectedErrors {
			found := false
			for _, err := range errs {
				if strings.Contains(err.Error(), expected) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error containing %q, got: %v", expected, errs)
			}
		}
	})

}
