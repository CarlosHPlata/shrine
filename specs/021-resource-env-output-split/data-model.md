# Phase 1 Data Model: Split Resource `env` and `output` (SRP)

All types live in `internal/manifest` (schema) and `internal/resolver`
(resolution result). No new package.

## 1. Manifest types (`internal/manifest/types.go`)

### 1.1 `ResourceSpec` — MODIFIED

Add an `Env` block mirroring `ApplicationSpec.Env`. `Outputs` remains but its
*valid* surface shrinks to `name` + `template`.

```go
type ResourceSpec struct {
	Type            string        `yaml:"type"`
	Version         string        `yaml:"version"`
	Port            int           `yaml:"port,omitempty"`
	Image           string        `yaml:"image,omitempty"`
	Env             []EnvVar      `yaml:"env,omitempty"`      // NEW — runtime configuration
	Outputs         []Output      `yaml:"outputs,omitempty"`  // export allowlist (name + optional template)
	Networking      Networking    `yaml:"networking,omitempty"`
	Volumes         []VolumeMount `yaml:"volumes,omitempty"`
	ImagePullPolicy string        `yaml:"imagePullPolicy,omitempty"`
}
```

### 1.2 `EnvVar` — REUSED + EXTENDED

Resource env reuses the Application `EnvVar` but additionally honours
`generated`. The simplest path is to add `Generated` to the shared `EnvVar`
(Applications never set it; validation may optionally reject `generated` on an
Application env to keep app semantics unchanged).

```go
type EnvVar struct {
	Name      string `yaml:"name"`
	Value     string `yaml:"value,omitempty"`
	ValueFrom string `yaml:"valueFrom,omitempty"`
	Template  string `yaml:"template,omitempty" json:"template,omitempty"`
	Generated bool   `yaml:"generated,omitempty" json:"generated,omitempty"` // NEW — resource-only auto-mint
}
```

**Per-entry rule** (resource env): exactly one of
`value` / `valueFrom` / `template` / `generated` MUST be set.
**Per-entry rule** (application env): unchanged — exactly one of
`value` / `valueFrom` / `template`; `generated` is not valid on Applications.

### 1.3 `Output` — REPURPOSED (export allowlist entry)

The struct keeps its value-bearing fields **only so old manifests still
unmarshal and can be rejected** (Decision 6). The valid surface is `Name` +
`Template`.

```go
// Output declares one item in a Resource's export allowlist. Valid fields are
// Name and an optional Template. Value/Generated/ValueFrom are retained solely
// to detect and reject the pre-split schema (see validateResourceSpec).
type Output struct {
	Name      string `yaml:"name" json:"name"`
	Template  string `yaml:"template,omitempty" json:"template,omitempty"`
	Value     string `yaml:"value,omitempty" json:"value,omitempty"`         // DEPRECATED — rejected if set
	Generated bool   `yaml:"generated,omitempty" json:"generated,omitempty"` // DEPRECATED — rejected if set
	ValueFrom string `yaml:"valueFrom,omitempty" json:"valueFrom,omitempty"` // DEPRECATED — rejected if set
}
```

## 2. Resolution result (`internal/resolver`)

### 2.1 `ResolvedResource` — NEW

```go
// ResolvedResource separates a resource's container environment from the
// interface it publishes to consumers.
type ResolvedResource struct {
	Env     map[string]string // fully resolved env → container (flattened by engine)
	Exports map[string]string // export allowlist → deps.Resources[name] (consumers)
}
```

### 2.2 `Resolver` interface — MODIFIED

```go
type Resolver interface {
	ResolveResource(res *manifest.ResourceManifest, deps ResolvedDependencies) (ResolvedResource, error)
	ResolveApplication(app *manifest.ApplicationManifest, deps ResolvedDependencies) (map[string]string, error)
}
```

`ResolvedDependencies` is unchanged in shape; `deps.Resources[name]` now holds a
resource's **exports** (not its full env).

## 3. Reserved built-in names

`host`, `port`, `team`, `name` are reserved. They are:
- forbidden as a resource `env` entry `name` (validation error);
- always available to output templates and as non-template output names
  (`host`/`port` exported only when listed; `team`/`name` are template-only
  context).

## 4. Validation rules (`internal/manifest/validate.go` → `validateResourceSpec`)

Accumulate ALL errors (multi-error; Principle I).

### 4.1 Env (new)

| Rule | Error condition |
|------|-----------------|
| name required | `env[i].name` empty |
| reserved name | `env[i].name` ∈ {host, port, team, name} |
| exactly-one source | count of (`value`, `valueFrom`, `template`, `generated`) ≠ 1 |
| unique names | duplicate `env[i].name` |

(vault-ref shape and cross-manifest `valueFrom` resolution are checked in the
planner — see contracts.)

### 4.2 Output (reshaped)

| Rule | Error condition |
|------|-----------------|
| name required | `output[i].name` empty |
| unique names | duplicate `output[i].name` |
| **no value-bearing fields** (FR-005, FR-011) | `output[i].value` / `generated` / `valueFrom` set → error naming the resource+output and pointing to `env` |
| non-template must resolve a target (FR-006) | `template` empty AND `name` is neither a declared env var nor `host`/`port` |
| `port` needs `spec.port` | `name == "port"` listed but `spec.port == 0` (resolve-time error preserved) |

### 4.3 Output templates (`internal/planner/templates.go`)

- Parse each `template`; syntax errors reported per output.
- Referenceable names: declared resource `env` var names ∪ {host, port, team,
  name}. Any other referenced field → error (FR-007).
- An output template MUST NOT reference another output name.

## 5. Cross-manifest reference rules (planner; FR-009, FR-014)

- A `valueFrom: resource.<name>.<key>` (from an Application **or** a Resource)
  is valid only if `<key>` ∈ the target resource's `output` names
  (strict allowlist). Referencing a declared-but-unexported env var → error.
- A `valueFrom: application.<name>.<key>` is valid only for `host`/`port`
  (unchanged; applications expose no env).
- Resource consumers are subject to the same access / reachability checks as
  Application consumers, and to feature-020 same-team deploy-order inference.

## 6. Deploy-order graph (`internal/planner/order.go`)

- Resources are no longer leaves: each resource contributes outgoing edges from
  its `Spec.Dependencies` (`dep.Kind + ":" + dep.Name`), exactly as Applications
  do today.
- A resource→resource cycle is reported as a dependency cycle.

## 7. Resolution & deploy flow (`internal/engine/engine.go`)

1. Synthesize Application built-ins (`host`, `port`) up front (unchanged).
2. Walk the topologically-ordered `steps`; for each **resource** step, call
   `ResolveResource(res, deps)`, store `resolved.Exports` in
   `deps.Resources[name]`, then create the container from `resolved.Env`.
3. For each **application** step, resolve and deploy as today (its resource/app
   deps already resolved/available because of ordering).

State-record timing (Principle VI) is unchanged: resolution and secret
generation precede backend calls exactly as before.

## 8. Worked example

```yaml
apiVersion: shrine/v1
kind: Resource
metadata: { name: pg, owner: data }
spec:
  type: postgres
  version: "16"
  port: 5432
  env:
    - name: POSTGRES_DB        # value
      value: app
    - name: POSTGRES_PASSWORD  # auto-minted secret, kept private
      generated: true
    - name: API_KEY            # from vault
      valueFrom: vault:data/prod/pg-api
  output:
    - name: POSTGRES_DB        # re-export an env var
    - name: host               # built-in
    - name: DB_URL             # derived, may reference the private password
      template: "postgres://app:{{.POSTGRES_PASSWORD}}@{{.host}}:{{.port}}/{{.POSTGRES_DB}}"
```

- **Container env** (`Env`): `POSTGRES_DB`, `POSTGRES_PASSWORD`, `API_KEY`.
- **Exports** (`Exports`, consumer-visible): `POSTGRES_DB`, `host`, `DB_URL`.
- A consumer's `valueFrom: resource.pg.POSTGRES_PASSWORD` → **rejected**
  (not exported). `resource.pg.DB_URL` → resolves (password folded in by the
  operator's own template).
