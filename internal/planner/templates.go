package planner

import (
	"fmt"
	"text/template"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// validateTemplates validates that every {{.X}} in a template output references
// a declared env var name or a built-in (team, name, host, port), and that each
// template parses as valid text/template syntax. Output templates may reference
// any declared env var — exported or not — but not another output.
func validateTemplates(res *manifest.ResourceManifest) []error {
	var errs []error

	valid := map[string]struct{}{
		"team": {},
		"name": {},
		"host": {},
		"port": {},
	}
	for _, e := range res.Spec.Env {
		valid[e.Name] = struct{}{}
	}

	for _, o := range res.Spec.Outputs {
		if o.Template == "" {
			continue
		}
		tmpl, err := template.New(o.Name).Parse(o.Template)
		if err != nil {
			errs = append(errs, fmt.Errorf("resource %q: template output %q has invalid syntax: %w",
				res.Metadata.Name, o.Name, err))
			continue
		}
		for _, ref := range manifest.ExtractFieldRefs(tmpl.Tree) {
			if _, ok := valid[ref]; !ok {
				errs = append(errs, fmt.Errorf("resource %q: template output %q references unknown variable %q (expected an env var or built-in: host, port, team, name)",
					res.Metadata.Name, o.Name, ref))
			}
		}
	}
	return errs
}

// validateResourceEnvTemplates validates that every {{.X}} in a resource env
// template references a sibling env name or a built-in (team, name, host, port).
func validateResourceEnvTemplates(res *manifest.ResourceManifest) []error {
	var errs []error

	valid := map[string]struct{}{
		"team": {},
		"name": {},
		"host": {},
		"port": {},
	}
	for _, e := range res.Spec.Env {
		valid[e.Name] = struct{}{}
	}

	for _, e := range res.Spec.Env {
		if e.Template == "" {
			continue
		}
		tmpl, err := template.New(e.Name).Parse(e.Template)
		if err != nil {
			errs = append(errs, fmt.Errorf("resource %q: template env %q has invalid syntax: %w",
				res.Metadata.Name, e.Name, err))
			continue
		}
		for _, ref := range manifest.ExtractFieldRefs(tmpl.Tree) {
			if _, ok := valid[ref]; !ok {
				errs = append(errs, fmt.Errorf("resource %q: template env %q references unknown variable %q",
					res.Metadata.Name, e.Name, ref))
			}
		}
	}
	return errs
}
func validateEnvTemplates(app *manifest.ApplicationManifest) []error {
	var errs []error

	valid := map[string]struct{}{
		"team": {},
		"name": {},
	}
	for _, e := range app.Spec.Env {
		valid[e.Name] = struct{}{}
	}

	for _, e := range app.Spec.Env {
		if e.Template == "" {
			continue
		}
		tmpl, err := template.New(e.Name).Parse(e.Template)
		if err != nil {
			errs = append(errs, fmt.Errorf("app %q: template env %q has invalid syntax: %w",
				app.Metadata.Name, e.Name, err))
			continue
		}
		for _, ref := range manifest.ExtractFieldRefs(tmpl.Tree) {
			if _, ok := valid[ref]; !ok {
				errs = append(errs, fmt.Errorf("app %q: template env %q references unknown variable %q",
					app.Metadata.Name, e.Name, ref))
			}
		}
	}
	return errs
}
