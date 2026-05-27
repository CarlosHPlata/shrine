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

func (r *DryRunResolver) ResolveResource(res *manifest.ResourceManifest, deps ResolvedDependencies) (ResolvedResource, error) {
	env := make(map[string]string, len(res.Spec.Env))
	tmplSrcs := make(map[string]string)

	for _, e := range res.Spec.Env {
		switch {
		case e.Value != "":
			env[e.Name] = e.Value
		case e.Generated:
			env[e.Name] = "[GENERATED]"
		case isVaultRef(e.ValueFrom):
			env[e.Name] = "[VAULT:" + parseVaultPath(e.ValueFrom) + "]"
		case e.ValueFrom != "":
			val, err := lookupValueFrom(e.ValueFrom, deps)
			if err != nil {
				return ResolvedResource{}, fmt.Errorf("dry-run: resource %q: env %q: %w", res.Metadata.Name, e.Name, err)
			}
			env[e.Name] = val
		case e.Template != "":
			tmplSrcs[e.Name] = e.Template
		default:
			return ResolvedResource{}, fmt.Errorf("dry-run: resource %q: env %q has neither value, valueFrom, template nor generated",
				res.Metadata.Name, e.Name)
		}
	}

	if len(tmplSrcs) > 0 {
		ctx := map[string]string{
			"team": res.Metadata.Owner,
			"name": res.Metadata.Name,
			"host": res.Metadata.Owner + "." + res.Metadata.Name,
			"port": "[PORT]",
		}
		maps.Copy(ctx, env)
		rendered, err := renderTemplates(fmt.Sprintf("resource %q", res.Metadata.Name), tmplSrcs, ctx)
		if err != nil {
			return ResolvedResource{}, err
		}
		maps.Copy(env, rendered)
	}

	exports, err := computeExports(res, env, "[PORT]")
	if err != nil {
		return ResolvedResource{}, err
	}
	return ResolvedResource{Env: env, Exports: exports}, nil
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
		case isVaultRef(e.ValueFrom):
			env[e.Name] = "[VAULT:" + parseVaultPath(e.ValueFrom) + "]"
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
