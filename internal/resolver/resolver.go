package resolver

import (
	"bytes"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"text/template"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/plugins/secrets"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/CarlosHPlata/shrine/internal/topo"
)

// generatedSecretLength is the byte length passed to the secret store for
// outputs marked `generated: true`.
const generatedSecretLength = 32

// ResolvedDependencies bundles the exported interface of resources and the
// synthesized built-ins of applications for use during final resolution.
// Resources entries hold only a resource's exported keys (its allowlist), not
// its full environment.
type ResolvedDependencies struct {
	Resources    map[string]map[string]string
	Applications map[string]map[string]string
}

// ResolvedResource separates a resource's container environment from the
// interface it publishes to consumers. Env feeds the container; Exports
// (the spec.output allowlist) feeds deps.Resources for consumers.
type ResolvedResource struct {
	Env     map[string]string
	Exports map[string]string
}

// Resolver materializes manifest env vars and exported outputs into final
// string values. Use NewLiveResolver for real deployments or NewDryRunResolver
// for planning/previewing.
type Resolver interface {
	ResolveResource(res *manifest.ResourceManifest, deps ResolvedDependencies) (ResolvedResource, error)
	ResolveApplication(
		app *manifest.ApplicationManifest,
		deps ResolvedDependencies,
	) (map[string]string, error)
}

type LiveResolver struct {
	Secrets state.SecretStore
	Vault   secrets.SecretsPlugin
}

func NewLiveResolver(secretStore state.SecretStore, vault secrets.SecretsPlugin) Resolver {
	return &LiveResolver{Secrets: secretStore, Vault: vault}
}

// isVaultRef reports whether a valueFrom string is a vault reference.
func isVaultRef(s string) bool {
	return strings.HasPrefix(s, "vault:")
}

// parseVaultPath strips the "vault:" prefix from a vault reference.
func parseVaultPath(s string) string {
	return strings.TrimPrefix(s, "vault:")
}

// ResolveResource builds a resource's container environment from spec.env and
// its exported interface from spec.output. Env values may reference other
// manifests' exports via deps; Exports contains only the keys listed in
// spec.output (the strict allowlist).
func (r *LiveResolver) ResolveResource(res *manifest.ResourceManifest, deps ResolvedDependencies) (ResolvedResource, error) {
	env, err := r.resolveResourceEnv(res, deps)
	if err != nil {
		return ResolvedResource{}, err
	}

	port := ""
	if res.Spec.Port != 0 {
		port = strconv.Itoa(res.Spec.Port)
	}
	exports, err := computeExports(res, env, port)
	if err != nil {
		return ResolvedResource{}, err
	}
	return ResolvedResource{Env: env, Exports: exports}, nil
}

// resolveResourceEnv materializes spec.env into its final values: static
// values, auto-minted secrets, vault/cross-manifest valueFrom, and templates
// (which reference sibling env vars plus the team/name/host/port built-ins;
// port is available only when spec.port is set).
func (r *LiveResolver) resolveResourceEnv(
	res *manifest.ResourceManifest,
	deps ResolvedDependencies,
) (map[string]string, error) {
	env := make(map[string]string, len(res.Spec.Env))
	tmplSrcs := make(map[string]string)

	for _, e := range res.Spec.Env {
		switch {
		case e.Value != "":
			env[e.Name] = e.Value
		case e.Generated:
			key := res.Metadata.Name + "." + e.Name
			secret, _, err := r.Secrets.GetOrGenerate(res.Metadata.Owner, key, generatedSecretLength)
			if err != nil {
				return nil, fmt.Errorf("resource %q: resolving generated env %q: %w",
					res.Metadata.Name, e.Name, err)
			}
			env[e.Name] = secret
		case e.ValueFrom != "":
			val, err := r.lookupValueFrom(e.ValueFrom, deps)
			if err != nil {
				return nil, fmt.Errorf("resource %q: env %q: %w", res.Metadata.Name, e.Name, err)
			}
			env[e.Name] = val
		case e.Template != "":
			tmplSrcs[e.Name] = e.Template
		default:
			return nil, fmt.Errorf("resource %q: env %q has neither value, valueFrom, template nor generated",
				res.Metadata.Name, e.Name)
		}
	}

	if len(tmplSrcs) == 0 {
		return env, nil
	}

	ctx := map[string]string{
		"team": res.Metadata.Owner,
		"name": res.Metadata.Name,
		"host": res.Metadata.Owner + "." + res.Metadata.Name,
	}
	if res.Spec.Port != 0 {
		ctx["port"] = strconv.Itoa(res.Spec.Port)
	}
	maps.Copy(ctx, env)
	rendered, err := renderTemplates(fmt.Sprintf("resource %q", res.Metadata.Name), tmplSrcs, ctx)
	if err != nil {
		return nil, err
	}
	maps.Copy(env, rendered)
	return env, nil
}

// computeExports derives a resource's published interface from spec.output over
// its already-resolved env. Non-template entries re-export an env var or the
// host/port built-in; template entries render against env plus the
// host/port/team/name built-ins. The returned map contains only listed keys.
func computeExports(res *manifest.ResourceManifest, env map[string]string, port string) (map[string]string, error) {
	host := res.Metadata.Owner + "." + res.Metadata.Name
	exports := make(map[string]string, len(res.Spec.Outputs))
	tmplSrcs := make(map[string]string)

	for _, o := range res.Spec.Outputs {
		if o.Template != "" {
			tmplSrcs[o.Name] = o.Template
			continue
		}
		if v, ok := env[o.Name]; ok {
			exports[o.Name] = v
			continue
		}
		switch o.Name {
		case "host":
			exports["host"] = host
		case "port":
			if port == "" {
				return nil, fmt.Errorf("resource %q: output \"port\" requires spec.port to be set", res.Metadata.Name)
			}
			exports["port"] = port
		default:
			return nil, fmt.Errorf("resource %q: output %q has no template and matches no env var or built-in (host, port)",
				res.Metadata.Name, o.Name)
		}
	}

	if len(tmplSrcs) > 0 {
		ctx := map[string]string{
			"team": res.Metadata.Owner,
			"name": res.Metadata.Name,
			"host": host,
		}
		if port != "" {
			ctx["port"] = port
		}
		maps.Copy(ctx, env)
		rendered, err := renderTemplates(fmt.Sprintf("resource %q output", res.Metadata.Name), tmplSrcs, ctx)
		if err != nil {
			return nil, err
		}
		maps.Copy(exports, rendered)
	}

	return exports, nil
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
			val, err := r.lookupValueFrom(e.ValueFrom, deps)
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

func (r *LiveResolver) lookupValueFrom(
	ref string,
	deps ResolvedDependencies,
) (string, error) {
	if isVaultRef(ref) {
		if r.Vault == nil || !r.Vault.IsActive() {
			return "", fmt.Errorf("vault ref %q: no secrets plugin configured", ref)
		}
		return r.Vault.GetSecret(parseVaultPath(ref))
	}
	return lookupResourceOrApplication(ref, deps)
}

func lookupValueFrom(
	ref string,
	deps ResolvedDependencies,
) (string, error) {
	return lookupResourceOrApplication(ref, deps)
}

func lookupResourceOrApplication(
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
