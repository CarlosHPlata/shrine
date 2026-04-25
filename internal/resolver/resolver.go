package resolver

import (
	"bytes"
	"fmt"
	"maps"
	"strings"
	"text/template"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/CarlosHPlata/shrine/internal/topo"
)

// generatedSecretLength is the byte length passed to the secret store for
// outputs marked `generated: true`.
const generatedSecretLength = 32

// ResolvedDependencies bundles the materialized outputs of resources and the
// synthesized built-ins of applications for use during final resolution.
type ResolvedDependencies struct {
	Resources    map[string]map[string]string
	Applications map[string]map[string]string
}

// Resolver materializes manifest outputs and application env vars into final
// string values. Use NewLiveResolver for real deployments or NewDryRunResolver
// for planning/previewing.
type Resolver interface {
	ResolveResource(res *manifest.ResourceManifest) (map[string]string, error)
	ResolveApplication(
		app *manifest.ApplicationManifest,
		deps ResolvedDependencies,
	) (map[string]string, error)
}

type LiveResolver struct {
	Secrets state.SecretStore
}

func NewLiveResolver(secrets state.SecretStore) Resolver {
	return &LiveResolver{Secrets: secrets}
}

// ResolveResource returns all output values for a single resource. The returned
// map includes the built-ins `team` and `name` alongside the resource's
// declared outputs.
func (r *LiveResolver) ResolveResource(res *manifest.ResourceManifest) (map[string]string, error) {
	values := map[string]string{
		"team": res.Metadata.Owner,
		"name": res.Metadata.Name,
	}
	var templates []manifest.Output

	// 1. Resolve outputs that don't depend on siblings.
	for _, output := range res.Spec.Outputs {
		switch {
		case output.Value != "":
			values[output.Name] = output.Value

		case output.Generated:
			key := res.Metadata.Name + "." + output.Name
			secret, _, err := r.Secrets.GetOrGenerate(res.Metadata.Owner, key, generatedSecretLength)
			if err != nil {
				return nil, fmt.Errorf("resource %q: resolving generated output %q: %w",
					res.Metadata.Name, output.Name, err)
			}
			values[output.Name] = secret

		case output.Template != "":
			templates = append(templates, output)

		default:
			// Bare output: the CLI only knows how to fill `host`.
			if output.Name == "host" {
				values[output.Name] = res.Metadata.Owner + "." + res.Metadata.Name
				continue
			}
			return nil, fmt.Errorf("resource %q: bare output %q is not a recognized CLI built-in (only \"host\" is supported)",
				res.Metadata.Name, output.Name)
		}
	}

	// Pass 2: topologically sort templates by their sibling references and
	// render them in order.
	tmplSrcs := make(map[string]string, len(templates))
	for _, t := range templates {
		tmplSrcs[t.Name] = t.Template
	}
	rendered, err := renderTemplates(fmt.Sprintf("resource %q", res.Metadata.Name), tmplSrcs, values)
	if err != nil {
		return nil, err
	}
	maps.Copy(values, rendered)

	return values, nil
}

// ResolveApplication returns the materialized env map for an application.
// resolvedResources must contain an entry for every resource referenced via
// valueFrom.
func (r *LiveResolver) ResolveApplication(
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
				return nil, fmt.Errorf("app %q: env %q: %w", app.Metadata.Name, e.Name, err)
			}
			env[e.Name] = val
		case e.Template != "":
			tmplSrcs[e.Name] = e.Template
		default:
			return nil, fmt.Errorf("app %q: env %q has neither value, valueFrom nor template", app.Metadata.Name, e.Name)
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

func lookupValueFrom(
	ref string,
	deps ResolvedDependencies,
) (string, error) {
	parts := strings.Split(ref, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid valueFrom format %q", ref)
	}

	var outputs map[string]string
	var ok bool

	switch parts[0] {
	case "resource":
		outputs, ok = deps.Resources[parts[1]]
	case "application":
		outputs, ok = deps.Applications[parts[1]]
	default:
		return "", fmt.Errorf("invalid valueFrom prefix %q (must be resource or application)", parts[0])
	}

	if !ok {
		return "", fmt.Errorf("unknown %s %q in valueFrom %q", parts[0], parts[1], ref)
	}
	val, ok := outputs[parts[2]]
	if !ok {
		return "", fmt.Errorf("%s %q has no resolved output %q", parts[0], parts[1], parts[2])
	}
	return val, nil
}

// renderTemplates resolves `templates` in topological order based on their
// inter-template references, erroring on cycles. `values` provides the values
// of non-template outputs and built-ins and is not mutated.
func renderTemplates(scope string, templates, values map[string]string) (map[string]string, error) {
	if len(templates) == 0 {
		return nil, nil
	}

	// parsed trees, keyed by output name, so we only parse each once.
	parsed := make(map[string]*template.Template, len(templates))
	// deps[name] = set of sibling template names this template references.
	deps := make(map[string]map[string]struct{}, len(templates))

	for name, src := range templates {
		tmpl, err := template.New(name).Parse(src)
		if err != nil {
			return nil, fmt.Errorf("%s: parsing template %q: %w", scope, name, err)
		}
		parsed[name] = tmpl

		d := make(map[string]struct{})
		for _, ref := range manifest.ExtractFieldRefs(tmpl.Tree) {
			if _, isTmpl := templates[ref]; isTmpl && ref != name {
				d[ref] = struct{}{}
			}
		}
		deps[name] = d
	}

	order, err := topo.Sort(deps)
	if err != nil {
		return nil, fmt.Errorf("%s: template cycle: %w", scope, err)
	}

	// Render in order, accumulating into a local map so earlier templates are
	// available to later ones.
	ctx := make(map[string]string, len(values)+len(templates))
	maps.Copy(ctx, values)

	out := make(map[string]string, len(templates))
	for _, name := range order {
		var buf bytes.Buffer
		if err := parsed[name].Execute(&buf, ctx); err != nil {
			return nil, fmt.Errorf("%s: rendering template %q: %w", scope, name, err)
		}
		ctx[name] = buf.String()
		out[name] = buf.String()
	}
	return out, nil
}
