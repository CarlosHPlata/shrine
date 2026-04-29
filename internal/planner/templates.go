package planner

import (
	"fmt"
	"text/template"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// validateTemplates validates that every {{.X}} in a template output either
// references a sibling output name or a built-in (team, name), and that each
// template parses as valid text/template syntax.
func validateTemplates(res *manifest.ResourceManifest) []error {
	var errs []error

	valid := map[string]struct{}{
		"team": {},
		"name": {},
		"host": {},
		"port": {},
	}
	for _, o := range res.Spec.Outputs {
		valid[o.Name] = struct{}{}
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
				errs = append(errs, fmt.Errorf("resource %q: template output %q references unknown variable %q",
					res.Metadata.Name, o.Name, ref))
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
