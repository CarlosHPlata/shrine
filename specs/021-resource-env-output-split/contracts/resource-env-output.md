# Contract: Resource `env` / `output` schema + resolver

This feature has two consumer-facing contracts: the **manifest schema** (what
operators write) and the **resolver interface** (the internal boundary the
engine depends on). Both are versioned by behaviour, not by a wire format.

## A. Manifest schema contract (`apiVersion: shrine/v1`, kind `Resource`)

### A.1 `spec.env` (NEW)

A list of runtime environment variables for the resource container. Each entry:

| Field | Type | Notes |
|-------|------|-------|
| `name` | string (required) | Must not be a reserved built-in (`host`, `port`, `team`, `name`). Unique within the resource. |
| `value` | string | Literal value. |
| `valueFrom` | string | `vault:<project>/<env>/<key>`, or `resource.<name>.<exportedKey>`, or `application.<name>.<host\|port>`. |
| `template` | string | Go `text/template`; references sibling env names + `team`/`name`. |
| `generated` | bool | Auto-mint a secret (resource-only). |

**Exactly one** of `value` / `valueFrom` / `template` / `generated` MUST be set.

### A.2 `spec.output` (RESHAPED)

A list declaring the resource's export allowlist. Each entry:

| Field | Type | Notes |
|-------|------|-------|
| `name` | string (required) | Unique within the resource. The key consumers reference. |
| `template` | string (optional) | Go `text/template`; references declared env names + `host`/`port`/`team`/`name`. |

**Forbidden** on an output entry: `value`, `valueFrom`, `generated`. Setting any
is a validation error (breaking change vs. pre-split manifests).

Resolution of an output entry:
- **With `template`** → exported value is the rendered template.
- **Without `template`** → `name` must equal a declared env var (exports that
  env var's resolved value) or `host`/`port` (exports the built-in). Otherwise
  validation fails.

### A.3 Consumption (any manifest)

`valueFrom: resource.<name>.<key>` resolves **iff** `<key>` is present in the
target resource's `output`. A declared-but-unexported env var is not
consumable. `valueFrom: application.<name>.<key>` resolves only for
`host`/`port`.

### A.4 Validation error examples (operator-visible)

```text
# old-style output (FR-011)
resource "pg": output "POSTGRES_PASSWORD" must not set value/valueFrom/generated —
those fields are deprecated on outputs; declare it under spec.env and list its
name under spec.outputs to export it

# reserved env name
resource "pg": env "host" uses a reserved built-in name (host, port, team, name)

# non-template output with no matching env var or built-in (FR-006)
resource "pg": output "FOO" has no template and matches no env var or built-in (host, port)

# consumer references an un-exported key (FR-009)
app "api": env "PW" references resource "pg" output "POSTGRES_PASSWORD" which is not exported
```

### A.5 Compatibility

Breaking change: a resource manifest that declared `value`/`valueFrom`/
`generated` on `outputs` no longer validates. Migration: move the field to
`spec.env` (keeping the same `name`) and list the `name` under `spec.output` if
it should remain consumable. Keeping the same name preserves any
already-generated secret (same secret-store key).

## B. Resolver interface contract (`internal/resolver`)

```go
type ResolvedResource struct {
	Env     map[string]string // → resource container environment
	Exports map[string]string // → deps.Resources[name] for consumers
}

type Resolver interface {
	ResolveResource(res *manifest.ResourceManifest, deps ResolvedDependencies) (ResolvedResource, error)
	ResolveApplication(app *manifest.ApplicationManifest, deps ResolvedDependencies) (map[string]string, error)
}
```

### B.1 `ResolveResource` behaviour

1. Build the env map from `spec.env`: `value` literal; `generated` →
   `Secrets.GetOrGenerate(owner, "<resource>.<name>", 32)`; `valueFrom: vault:`
   → vault; `valueFrom: resource/application.…` → `deps` lookup; `template` →
   rendered against (resolved env + `team`/`name`).
2. Build the exports map from `spec.output`: template entries rendered against
   (resolved env + `host`/`port`/`team`/`name`); non-template entries copy the
   matching env var or the built-in `host`/`port`.
3. Return `{Env, Exports}`. `port` export requires `spec.port` (else error).

**Guarantees**:
- `Exports` contains **only** keys listed in `spec.output` (strict allowlist).
- A key in `Env` but not in `spec.output` never appears in `Exports`.
- `deps` is read-only.

### B.2 `DryRunResolver` behaviour

Same shape; `Env` uses placeholders (`[GENERATED]`, `[VAULT:<path>]`, `[PORT]`)
and performs no secret generation or vault reads. `Exports` is computed from the
output list over those placeholders.

### B.3 Engine consumption order

The engine resolves resources following the topological `Order` output, so a
resource referencing another resource's export sees it already populated in
`deps.Resources`. `deps.Resources[name]` is set to `resolved.Exports`; the
container is created from `resolved.Env`.

## C. Contract tests (unit-level)

- Resource env: each source kind resolves; `generated` reuses an existing key;
  reserved-name rejected; exactly-one-source enforced.
- Output: template references private env var → present in `Exports`; private
  env var absent from `Exports`; non-template name maps to env/built-in; `port`
  without `spec.port` → error.
- Cross-manifest: `resource.X.exported` resolves; `resource.X.unexported` →
  validation error; resource consuming `resource.Y.key` ordered after `Y`.
- Old manifest: `generated`/`value`/`valueFrom` on output → rejection error text
  matches A.4.
