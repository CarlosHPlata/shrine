package resolver

import (
	"bytes"
	"fmt"
	"maps"
	"strings"
	"text/template"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

// generatedSecretLength is the byte length passed to the secret store for
// outputs marked `generated: true`.
const generatedSecretLength = 32

// Resolver materializes manifest outputs and application env vars into final
// string values. Use NewLiveResolver for real deployments or NewDryRunResolver
// for planning/previewing.
type Resolver interface {
	ResolveResource(res *manifest.ResourceManifest) (map[string]string, error)
	ResolveApplication(app *manifest.ApplicationManifest, resolvedResources map[string]map[string]string) (map[string]string, error)
}

type LiveResolver struct {
	Secrets state.SecretStore
}

func NewLiveResolver(secrets state.SecretStore) *LiveResolver {
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
	rendered, err := renderTemplates(res.Metadata.Name, templates, values)
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
				return nil, fmt.Errorf("app %q: env %q: %w", app.Metadata.Name, e.Name, err)
			}
			env[e.Name] = val
		default:
			return nil, fmt.Errorf("app %q: env %q has neither value nor valueFrom", app.Metadata.Name, e.Name)
		}
	}
	return env, nil
}

func lookupValueFrom(ref string, resolvedResources map[string]map[string]string) (string, error) {
	parts := strings.Split(ref, ".")
	if len(parts) != 3 || parts[0] != "resource" {
		return "", fmt.Errorf("invalid valueFrom format %q", ref)
	}
	outputs, ok := resolvedResources[parts[1]]
	if !ok {
		return "", fmt.Errorf("unknown resource %q in valueFrom %q", parts[1], ref)
	}
	val, ok := outputs[parts[2]]
	if !ok {
		return "", fmt.Errorf("resource %q has no resolved output %q", parts[1], parts[2])
	}
	return val, nil
}

// renderTemplates resolves `templates` in topological order based on their
// inter-template references, erroring on cycles. `values` provides the values
// of non-template outputs and built-ins and is not mutated.
func renderTemplates(resName string, templates []manifest.Output, values map[string]string) (map[string]string, error) {
	if len(templates) == 0 {
		return nil, nil
	}

	// parsed trees, keyed by output name, so we only parse each once.
	parsed := make(map[string]*template.Template, len(templates))
	// deps[name] = set of sibling template names this template references.
	deps := make(map[string]map[string]struct{}, len(templates))
	// tmplSet: set of template-output names, used to distinguish template
	// refs from non-template refs when building the dep graph.
	tmplSet := make(map[string]struct{}, len(templates))
	for _, t := range templates {
		tmplSet[t.Name] = struct{}{}
	}

	for _, t := range templates {
		tmpl, err := template.New(t.Name).Parse(t.Template)
		if err != nil {
			return nil, fmt.Errorf("resource %q: parsing template %q: %w", resName, t.Name, err)
		}
		parsed[t.Name] = tmpl

		d := make(map[string]struct{})
		for _, ref := range manifest.ExtractFieldRefs(tmpl.Tree) {
			if _, isTmpl := tmplSet[ref]; isTmpl && ref != t.Name {
				d[ref] = struct{}{}
			}
		}
		deps[t.Name] = d
	}

	order, err := topoSort(deps)
	if err != nil {
		return nil, fmt.Errorf("resource %q: %w", resName, err)
	}

	// Render in order, accumulating into a local map so earlier templates are
	// available to later ones.
	ctx := make(map[string]string, len(values)+len(templates))
	maps.Copy(ctx, values)

	out := make(map[string]string, len(templates))
	for _, name := range order {
		var buf bytes.Buffer
		if err := parsed[name].Execute(&buf, ctx); err != nil {
			return nil, fmt.Errorf("resource %q: rendering template %q: %w", resName, name, err)
		}
		ctx[name] = buf.String()
		out[name] = buf.String()
	}
	return out, nil
}

// topoSort returns the template names in an order where each name appears
// after its dependencies, using Kahn's algorithm. Cycles produce an error
// listing the unresolved nodes.
func topoSort(deps map[string]map[string]struct{}) ([]string, error) {
	// reverse[x] = set of nodes that depend on x.
	reverse := make(map[string]map[string]struct{}, len(deps))
	indeg := make(map[string]int, len(deps))
	for node := range deps {
		indeg[node] = 0
		reverse[node] = make(map[string]struct{})
	}
	for node, ds := range deps {
		for d := range ds {
			if _, ok := deps[d]; !ok {
				// dependency isn't a template — already resolved, skip.
				continue
			}
			reverse[d][node] = struct{}{}
			indeg[node]++
		}
	}

	var queue []string
	for node, deg := range indeg {
		if deg == 0 {
			queue = append(queue, node)
		}
	}

	var order []string
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)
		for dep := range reverse[n] {
			indeg[dep]--
			if indeg[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(deps) {
		var stuck []string
		for node, deg := range indeg {
			if deg > 0 {
				stuck = append(stuck, node)
			}
		}
		return nil, fmt.Errorf("template cycle involving outputs: %v", stuck)
	}
	return order, nil
}
