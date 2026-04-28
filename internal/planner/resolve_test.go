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
							{Kind: manifest.ResourceKind, Name: "db1", Owner: "team-a"},
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
							{Kind: manifest.ResourceKind, Name: "nonexistent", Owner: "team-a"},
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
							{Kind: manifest.ResourceKind, Name: "db1", Owner: "team-b"},
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
							{Kind: manifest.ResourceKind, Name: "db1", Owner: "team-b"},
						},
					},
				},
			},
			Resources: map[string]*manifest.ResourceManifest{
				"db1": {
					Metadata: manifest.Metadata{Name: "db1", Owner: "team-b", Access: []string{"team-a"}},
					Spec:     manifest.ResourceSpec{Type: "postgres", Networking: manifest.Networking{ExposeToPlatform: true}},
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
							{Kind: manifest.ResourceKind, Name: "db1", Owner: "team-a"}, // Specifies team-a
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
							{Name: "WRONG_PREFIX", ValueFrom: "config.db1.url"},   // Wrong prefix
							{Name: "MISSING_RES", ValueFrom: "resource.db2.url"},  // Missing resource
							{Name: "MISSING_OUT", ValueFrom: "resource.db1.host"}, // Missing output
							{Name: "VALID_APP", ValueFrom: "application.worker.host"},
							{Name: "INVALID_APP_OUT", ValueFrom: "application.worker.url"},
							{Name: "MISSING_APP", ValueFrom: "application.ghost.host"},
						},
					},
				},
				"worker": {
					Metadata: manifest.Metadata{Name: "worker", Owner: "team-a"},
					Spec:     manifest.ApplicationSpec{Image: "img", Port: 80},
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
			"application \"worker\" has no built-in output \"url\" (only host and port are supported)",
			"references missing application \"ghost\"",
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
						Port:    5432,
						Outputs: []manifest.Output{
							{Name: "host"},
							{Name: "port"},
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

	t.Run("env template variable validation", func(t *testing.T) {
		set := &ManifestSet{
			Applications: map[string]*manifest.ApplicationManifest{
				"app1": {
					Metadata: manifest.Metadata{Name: "app1", Owner: "team-a"},
					Spec: manifest.ApplicationSpec{
						Env: []manifest.EnvVar{
							{Name: "LITERAL", Value: "a"},
							{Name: "REF", ValueFrom: "resource.db1.url"},
							// Valid: references literal, valueFrom, sibling template, and built-ins.
							{Name: "T1", Template: "http://{{.LITERAL}}:{{.REF}}/{{.T1}}"}, // T1 can ref itself? Wait, validateEnvTemplates doesn't check for cycles yet.
							{Name: "T2", Template: "{{.team}}-{{.name}}"},
							// Invalid: unknown variable.
							{Name: "BAD", Template: "{{.ghost}}"},
							// Invalid: syntax error.
							{Name: "BROKEN", Template: "{{.host"},
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
			"template env \"BAD\" references unknown variable \"ghost\"",
			"template env \"BROKEN\" has invalid syntax",
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
	t.Run("application dependencies", func(t *testing.T) {
		t.Run("same-team app dep", func(t *testing.T) {
			set := &ManifestSet{
				Applications: map[string]*manifest.ApplicationManifest{
					"aterrizar": {
						Metadata: manifest.Metadata{Name: "aterrizar", Owner: "team-a"},
						Spec: manifest.ApplicationSpec{
							Dependencies: []manifest.Dependency{
								{Kind: manifest.ApplicationKind, Name: "worker", Owner: "team-a"},
							},
						},
					},
					"worker": {
						Metadata: manifest.Metadata{Name: "worker", Owner: "team-a"},
						Spec:     manifest.ApplicationSpec{Image: "img", Port: 80},
					},
				},
			}
			errs := Resolve(set, store)
			if len(errs) > 0 {
				t.Errorf("expected no errors, got: %v", errs)
			}
		})

		t.Run("cross-team with access", func(t *testing.T) {
			set := &ManifestSet{
				Applications: map[string]*manifest.ApplicationManifest{
					"aterrizar": {
						Metadata: manifest.Metadata{Name: "aterrizar", Owner: "team-a"},
						Spec: manifest.ApplicationSpec{
							Dependencies: []manifest.Dependency{
								{Kind: manifest.ApplicationKind, Name: "worker", Owner: "team-b"},
							},
						},
					},
					"worker": {
						Metadata: manifest.Metadata{Name: "worker", Owner: "team-b", Access: []string{"team-a"}},
						Spec:     manifest.ApplicationSpec{Image: "img", Port: 80, Networking: manifest.Networking{ExposeToPlatform: true}},
					},
				},
			}
			errs := Resolve(set, store)
			if len(errs) > 0 {
				t.Errorf("expected no errors, got: %v", errs)
			}
		})

		t.Run("cross-team access denied", func(t *testing.T) {
			set := &ManifestSet{
				Applications: map[string]*manifest.ApplicationManifest{
					"aterrizar": {
						Metadata: manifest.Metadata{Name: "aterrizar", Owner: "team-a"},
						Spec: manifest.ApplicationSpec{
							Dependencies: []manifest.Dependency{
								{Kind: manifest.ApplicationKind, Name: "worker", Owner: "team-b"},
							},
						},
					},
					"worker": {
						Metadata: manifest.Metadata{Name: "worker", Owner: "team-b"},
						Spec:     manifest.ApplicationSpec{Image: "img", Port: 80},
					},
				},
			}
			errs := Resolve(set, store)
			found := false
			for _, err := range errs {
				if strings.Contains(err.Error(), "does not have access to application \"worker\"") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected access denied error, got: %v", errs)
			}
		})

		t.Run("missing application", func(t *testing.T) {
			set := &ManifestSet{
				Applications: map[string]*manifest.ApplicationManifest{
					"aterrizar": {
						Metadata: manifest.Metadata{Name: "aterrizar", Owner: "team-a"},
						Spec: manifest.ApplicationSpec{
							Dependencies: []manifest.Dependency{
								{Kind: manifest.ApplicationKind, Name: "ghost", Owner: "team-a"},
							},
						},
					},
				},
			}
			errs := Resolve(set, store)
			found := false
			for _, err := range errs {
				if strings.Contains(err.Error(), "depends on missing application \"ghost\"") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected missing application error, got: %v", errs)
			}
		})

		t.Run("owner mismatch", func(t *testing.T) {
			set := &ManifestSet{
				Applications: map[string]*manifest.ApplicationManifest{
					"aterrizar": {
						Metadata: manifest.Metadata{Name: "aterrizar", Owner: "team-a"},
						Spec: manifest.ApplicationSpec{
							Dependencies: []manifest.Dependency{
								{Kind: manifest.ApplicationKind, Name: "worker", Owner: "team-b"},
							},
						},
					},
					"worker": {
						Metadata: manifest.Metadata{Name: "worker", Owner: "team-a"},
						Spec:     manifest.ApplicationSpec{Image: "img", Port: 80},
					},
				},
			}
			errs := Resolve(set, store)
			found := false
			for _, err := range errs {
				if strings.Contains(err.Error(), "application \"worker\" owned by \"team-a\", but manifest specifies owner \"team-b\"") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected owner mismatch error, got: %v", errs)
			}
		})

		t.Run("cross-team unreachable", func(t *testing.T) {
			set := &ManifestSet{
				Applications: map[string]*manifest.ApplicationManifest{
					"aterrizar": {
						Metadata: manifest.Metadata{Name: "aterrizar", Owner: "team-a"},
						Spec: manifest.ApplicationSpec{
							Dependencies: []manifest.Dependency{
								{Kind: manifest.ApplicationKind, Name: "worker", Owner: "team-b"},
							},
						},
					},
					"worker": {
						Metadata: manifest.Metadata{Name: "worker", Owner: "team-b", Access: []string{"team-a"}},
						Spec:     manifest.ApplicationSpec{Image: "img", Port: 80, Networking: manifest.Networking{ExposeToPlatform: false}},
					},
				},
			}
			errs := Resolve(set, store)
			found := false
			for _, err := range errs {
				if strings.Contains(err.Error(), "is not reachable cross-team") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected reachability error, got: %v", errs)
			}
		})

		t.Run("cross-team unauthorized but reachable", func(t *testing.T) {
			set := &ManifestSet{
				Applications: map[string]*manifest.ApplicationManifest{
					"aterrizar": {
						Metadata: manifest.Metadata{Name: "aterrizar", Owner: "team-a"},
						Spec: manifest.ApplicationSpec{
							Dependencies: []manifest.Dependency{
								{Kind: manifest.ApplicationKind, Name: "worker", Owner: "team-b"},
							},
						},
					},
					"worker": {
						Metadata: manifest.Metadata{Name: "worker", Owner: "team-b", Access: []string{"team-c"}},
						Spec:     manifest.ApplicationSpec{Image: "img", Port: 80, Networking: manifest.Networking{ExposeToPlatform: true}},
					},
				},
			}
			errs := Resolve(set, store)
			hasAccessErr := false
			hasReachErr := false
			for _, err := range errs {
				if strings.Contains(err.Error(), "does not have access") {
					hasAccessErr = true
				}
				if strings.Contains(err.Error(), "is not reachable cross-team") {
					hasReachErr = true
				}
			}
			if !hasAccessErr {
				t.Errorf("expected access error, got: %v", errs)
			}
			if hasReachErr {
				t.Errorf("expected no reachability error (since reachable), got: %v", errs)
			}
		})
	})

	t.Run("resource reachability", func(t *testing.T) {
		t.Run("cross-team resource unreachable", func(t *testing.T) {
			set := &ManifestSet{
				Applications: map[string]*manifest.ApplicationManifest{
					"app1": {
						Metadata: manifest.Metadata{Name: "app1", Owner: "team-a"},
						Spec: manifest.ApplicationSpec{
							Dependencies: []manifest.Dependency{
								{Kind: manifest.ResourceKind, Name: "db1", Owner: "team-b"},
							},
						},
					},
				},
				Resources: map[string]*manifest.ResourceManifest{
					"db1": {
						Metadata: manifest.Metadata{Name: "db1", Owner: "team-b", Access: []string{"team-a"}},
						Spec:     manifest.ResourceSpec{Type: "postgres", Networking: manifest.Networking{ExposeToPlatform: false}},
					},
				},
			}
			errs := Resolve(set, store)
			found := false
			for _, err := range errs {
				if strings.Contains(err.Error(), "is not reachable cross-team") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected reachability error, got: %v", errs)
			}
		})

		t.Run("cross-team resource reachable", func(t *testing.T) {
			set := &ManifestSet{
				Applications: map[string]*manifest.ApplicationManifest{
					"app1": {
						Metadata: manifest.Metadata{Name: "app1", Owner: "team-a"},
						Spec: manifest.ApplicationSpec{
							Dependencies: []manifest.Dependency{
								{Kind: manifest.ResourceKind, Name: "db1", Owner: "team-b"},
							},
						},
					},
				},
				Resources: map[string]*manifest.ResourceManifest{
					"db1": {
						Metadata: manifest.Metadata{Name: "db1", Owner: "team-b", Access: []string{"team-a"}},
						Spec:     manifest.ResourceSpec{Type: "postgres", Networking: manifest.Networking{ExposeToPlatform: true}},
					},
				},
			}
			errs := Resolve(set, store)
			if len(errs) > 0 {
				t.Errorf("expected no errors, got: %v", errs)
			}
		})
	})
	t.Run("app and resource can share name", func(t *testing.T) {
		set := &ManifestSet{
			Applications: map[string]*manifest.ApplicationManifest{
				"shared-name": {
					Metadata: manifest.Metadata{Name: "shared-name", Owner: "team-a"},
					Spec:     manifest.ApplicationSpec{Image: "img", Port: 80},
				},
			},
			Resources: map[string]*manifest.ResourceManifest{
				"shared-name": {
					Metadata: manifest.Metadata{Name: "shared-name", Owner: "team-a"},
					Spec:     manifest.ResourceSpec{Type: "postgres"},
				},
			},
		}

		errs := Resolve(set, store)
		// Should succeed, no "name collision" error
		for _, err := range errs {
			if strings.Contains(err.Error(), "name collision") {
				t.Errorf("unexpected name collision error: %v", err)
			}
		}
	})

	t.Run("metadata name mismatch", func(t *testing.T) {
		set := &ManifestSet{
			Applications: map[string]*manifest.ApplicationManifest{
				"key-a": {
					Metadata: manifest.Metadata{Name: "mismatch-a", Owner: "team-a"},
					Spec:     manifest.ApplicationSpec{Image: "img", Port: 80},
				},
			},
		}

		errs := Resolve(set, store)
		found := false
		for _, err := range errs {
			if strings.Contains(err.Error(), "application \"key-a\" has metadata name mismatch: \"mismatch-a\"") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected metadata mismatch error, got: %v", errs)
		}
	})
}
