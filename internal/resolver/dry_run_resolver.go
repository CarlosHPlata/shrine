package resolver

import (
	"fmt"
	"maps"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// DryRunResolver implements the Resolver interface but returns placeholder values
// instead of hitting the secret store or rendering actual templates. This is
// used by the dry-run engine to show a plan without side effects.
type DryRunResolver struct{}

func NewDryRunResolver() *DryRunResolver {
	return &DryRunResolver{}
}

func (r *DryRunResolver) ResolveResource(res *manifest.ResourceManifest) (map[string]string, error) {
	values := map[string]string{
		"team": res.Metadata.Owner,
		"name": res.Metadata.Name,
	}

	for _, o := range res.Spec.Outputs {
		switch {
		case o.Value != "":
			values[o.Name] = o.Value
		case o.Generated:
			values[o.Name] = "[GENERATED]"
		case o.Template != "":
			values[o.Name] = o.Template
		default:
			if o.Name == "host" {
				values[o.Name] = res.Metadata.Owner + "." + res.Metadata.Name
				continue
			}
			return nil, fmt.Errorf("dry-run: resource %q: bare output %q is not a recognized CLI built-in",
				res.Metadata.Name, o.Name)
		}
	}

	return values, nil
}

func (r *DryRunResolver) ResolveApplication(
	app *manifest.ApplicationManifest,
	deps ResolvedDependencies,
) (map[string]string, error) {
	env := make(map[string]string, len(app.Spec.Env))
	tmplSrcs := make(map[string]string)

	for _, e := range app.Spec.Env {
		switch {
		case e.Value != "":
			env[e.Name] = e.Value
		case e.ValueFrom != "":
			val, err := lookupValueFrom(e.ValueFrom, deps)
			if err != nil {
				return nil, fmt.Errorf("dry-run: app %q: env %q: %w", app.Metadata.Name, e.Name, err)
			}
			env[e.Name] = val
		case e.Template != "":
			tmplSrcs[e.Name] = e.Template
		default:
			return nil, fmt.Errorf("dry-run: app %q: env %q has neither value, valueFrom nor template", app.Metadata.Name, e.Name)
		}
	}

	if len(tmplSrcs) == 0 {
		return env, nil
	}

	// Seed render context with built-ins and sibling non-template envs.
	ctx := map[string]string{
		"team": app.Metadata.Owner,
		"name": app.Metadata.Name,
	}
	maps.Copy(ctx, env)

	rendered, err := renderTemplates(fmt.Sprintf("app %q", app.Metadata.Name), tmplSrcs, ctx)
	if err != nil {
		return nil, err
	}
	maps.Copy(env, rendered)

	return env, nil
}
