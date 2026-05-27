package planner

import (
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

func resourceWith(env []manifest.EnvVar, outputs []manifest.Output) *manifest.ResourceManifest {
	return &manifest.ResourceManifest{
		Metadata: manifest.Metadata{Name: "pg", Owner: "team-a"},
		Spec:     manifest.ResourceSpec{Type: "postgres", Version: "16", Env: env, Outputs: outputs},
	}
}

func errorsContain(errs []error, want string) bool {
	for _, e := range errs {
		if strings.Contains(e.Error(), want) {
			return true
		}
	}
	return false
}

func TestValidateTemplates_OutputReferences(t *testing.T) {
	t.Run("references env var (exported or not) and built-ins", func(t *testing.T) {
		res := resourceWith(
			[]manifest.EnvVar{{Name: "DB", Value: "app"}, {Name: "PW", Generated: true}},
			[]manifest.Output{{Name: "URL", Template: "{{.team}}/{{.name}}://{{.host}}:{{.port}}/{{.DB}}?pw={{.PW}}"}},
		)
		if errs := validateTemplates(res); len(errs) != 0 {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("references unknown variable", func(t *testing.T) {
		res := resourceWith(nil, []manifest.Output{{Name: "URL", Template: "{{.ghost}}"}})
		if errs := validateTemplates(res); !errorsContain(errs, `references unknown variable "ghost"`) {
			t.Errorf("expected unknown-variable error, got: %v", errs)
		}
	})

	t.Run("output cannot reference another output", func(t *testing.T) {
		res := resourceWith(
			[]manifest.EnvVar{{Name: "DB", Value: "app"}},
			[]manifest.Output{
				{Name: "DB"},
				{Name: "URL", Template: "{{.OTHER}}"},
				{Name: "OTHER", Template: "{{.DB}}"},
			},
		)
		if errs := validateTemplates(res); !errorsContain(errs, `references unknown variable "OTHER"`) {
			t.Errorf("expected output-to-output reference to fail, got: %v", errs)
		}
	})

	t.Run("syntax error reported", func(t *testing.T) {
		res := resourceWith(nil, []manifest.Output{{Name: "URL", Template: "{{.host"}})
		if errs := validateTemplates(res); !errorsContain(errs, "has invalid syntax") {
			t.Errorf("expected syntax error, got: %v", errs)
		}
	})
}

func TestValidateResourceEnvTemplates(t *testing.T) {
	t.Run("references sibling env and built-ins", func(t *testing.T) {
		res := resourceWith(
			[]manifest.EnvVar{{Name: "A", Value: "a"}, {Name: "B", Template: "{{.team}}-{{.name}}-{{.A}}"}},
			nil,
		)
		if errs := validateResourceEnvTemplates(res); len(errs) != 0 {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("references host and port built-ins", func(t *testing.T) {
		// host/port are available to env templates just like output templates.
		res := resourceWith([]manifest.EnvVar{{Name: "CONN", Template: "redis://{{.host}}:{{.port}}"}}, nil)
		if errs := validateResourceEnvTemplates(res); len(errs) != 0 {
			t.Errorf("expected no errors, got: %v", errs)
		}
	})

	t.Run("references unknown variable", func(t *testing.T) {
		res := resourceWith([]manifest.EnvVar{{Name: "B", Template: "{{.ghost}}"}}, nil)
		if errs := validateResourceEnvTemplates(res); !errorsContain(errs, `references unknown variable "ghost"`) {
			t.Errorf("expected unknown-variable error for ghost, got: %v", errs)
		}
	})
}
