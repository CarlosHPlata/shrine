package resolver

import (
	"fmt"

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
	resolvedResources map[string]map[string]string,
) (map[string]string, error) {
	env := make(map[string]string, len(app.Spec.Env))
	for _, e := range app.Spec.Env {
		switch {
		case e.Value != "":
			env[e.Name] = e.Value
		case e.ValueFrom != "":
			val, err := lookupValueFrom(e.ValueFrom, resolvedResources)
			if err != nil {
				// In dry-run, we might still fail if the referenced resource
				// wasn't plan-able, but since ExecuteDeploy resolves all
				// resources first, this should be safe.
				return nil, fmt.Errorf("dry-run: app %q: env %q: %w", app.Metadata.Name, e.Name, err)
			}
			env[e.Name] = val
		default:
			return nil, fmt.Errorf("dry-run: app %q: env %q has neither value nor valueFrom", app.Metadata.Name, e.Name)
		}
	}
	return env, nil
}
