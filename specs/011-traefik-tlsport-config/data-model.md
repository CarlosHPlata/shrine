# Data Model: Traefik Plugin `tlsPort` Config Option

**Feature**: 011-traefik-tlsport-config
**Phase**: 1 (Design)
**Date**: 2026-05-01

This feature does not introduce any persistent data store or new manifest kind. The "data model" here is limited to the operator-facing YAML schema delta, the in-memory Go struct delta, and the runtime port-binding shape — all of which are deterministic projections of the operator's config.

## 1. Operator-facing config schema (`~/.config/shrine/config.yml`)

### Before this feature

```yaml
plugins:
  gateway:
    traefik:
      image: "traefik:v3.7.0-rc.2"
      routing-dir: /etc/shrine/traefik   # optional
      port: 80                            # optional, default 80
      dashboard:                          # optional
        port: 8080
        username: legion
        password: secret
```

### After this feature

```yaml
plugins:
  gateway:
    traefik:
      image: "traefik:v3.7.0-rc.2"
      routing-dir: /etc/shrine/traefik
      port: 80
      tlsPort: 443                        # NEW — optional, no default; when set, publishes <tlsPort>:443/tcp and adds websecure entrypoint
      dashboard:
        port: 8080
        username: legion
        password: secret
```

### Field contract

| Field | Type | Required | Default | Constraints |
|-------|------|----------|---------|-------------|
| `tlsPort` | int | no | unset (zero-value) | When set: `1 ≤ tlsPort ≤ 65535`, `tlsPort != port`, `tlsPort != dashboard.port`. |

When unset (omitted entirely), behavior is identical to the pre-feature release — no `443/tcp` host mapping, no `websecure` entrypoint. The zero-value (`tlsPort: 0`) is treated as unset and accepted; any other invalid value is rejected at validation time.

## 2. In-memory Go struct (`internal/config/plugin_traefik.go`)

```go
type TraefikPluginConfig struct {
    Image      string                  `yaml:"image,omitempty"`
    RoutingDir string                  `yaml:"routing-dir,omitempty"`
    Port       int                     `yaml:"port,omitempty"`
    TLSPort    int                     `yaml:"tlsPort,omitempty"`     // NEW
    Dashboard  *TraefikDashboardConfig `yaml:"dashboard,omitempty"`
}
```

Only one field added (`TLSPort int`), parallel to `Port`. The Go field name uses `TLS` (uppercase acronym) per Go's `golint` convention; the YAML tag preserves the issue's proposed `tlsPort` camelCase verbatim.

## 3. Port-binding projection (runtime)

The `Plugin.portBindings()` helper in `internal/plugins/gateway/traefik/plugin.go` projects the config into a slice of `engine.PortBinding`:

| Condition | Resulting port bindings |
|-----------|-------------------------|
| Plugin active, no dashboard, no `tlsPort` | `[{HostPort: <port>, ContainerPort: <port>, Protocol: "tcp"}]` |
| Plugin active, dashboard set, no `tlsPort` | `[{<port>:<port>/tcp}, {<dashboard.port>:<dashboard.port>/tcp}]` |
| Plugin active, no dashboard, `tlsPort` set | `[{<port>:<port>/tcp}, {<tlsPort>:443/tcp}]` |
| Plugin active, dashboard set, `tlsPort` set | `[{<port>:<port>/tcp}, {<tlsPort>:443/tcp}, {<dashboard.port>:<dashboard.port>/tcp}]` |

Container side of the TLS binding is the literal string `"443"` — fixed, not operator-configurable, per Decision 2 in research.md.

## 4. Generated static config (`<routing-dir>/traefik.yml`) entrypoint shape

The `staticConfig.EntryPoints` map projection follows the same conditional structure:

| Condition | `entryPoints` keys generated |
|-----------|------------------------------|
| No dashboard, no `tlsPort` | `web` only |
| Dashboard set, no `tlsPort` | `web`, `traefik` |
| No dashboard, `tlsPort` set | `web`, `websecure` |
| Dashboard set, `tlsPort` set | `web`, `traefik`, `websecure` |

The `websecure` entrypoint is always `entryPoint{Address: ":443"}` — no other fields. The existing `entryPoint` struct in `spec.go` only has an `Address` field, so this constraint is enforced structurally.

This projection only runs when `traefik.yml` is being generated (file does not yet exist). When the file is preserved (operator-edited), the projection is skipped; the operator's existing `entryPoints` content stands as-is.

## 5. ConfigHash inputs (drift detection)

Per Decision 3 in research.md, `state.ConfigHash` is extended to take port-binding specs alongside its current inputs:

```go
// Before:
func ConfigHash(image string, env []string, volSpecs []string, exposeToPlatform bool) string

// After:
func ConfigHash(image string, env []string, volSpecs []string, portSpecs []string, exposeToPlatform bool) string
```

`portSpecs` are `"<hostPort>:<containerPort>/<proto>"` strings, sorted internally by the helper for stable hashing. The single caller (`dockercontainer.configHash`) projects `op.PortBindings` into this slice.

This is a behavior change for **all** containers, not just Traefik — it is the only sensible scope, because the bug it fixes (port-binding drift not triggering recreation) is engine-wide. Operational impact: one-time recreate of every existing container on first deploy after upgrade.

## 6. Validation pipeline

```
config.yml
    │ YAML parse (tlsPort accepted as `int`)
    ▼
TraefikPluginConfig{TLSPort: N}
    │ traefik.New(cfg, ...)
    │ → Plugin.validate()
    │     ├─ existing: dashboard.port without credentials → reject
    │     ├─ NEW:      tlsPort out of range → reject
    │     ├─ NEW:      tlsPort == resolvedPort() → reject
    │     └─ NEW:      tlsPort == dashboard.port → reject
    ▼
Plugin (validated)
    │ Plugin.Deploy()
    ▼
Container created with PortBindings (incl. <tlsPort>:443/tcp when set)
+ traefik.yml with websecure entrypoint (when set AND not preserved)
```

Each rejection produces an error message that names the offending field (`tlsPort`) per FR-005.

## 7. State transitions across deploys

| Previous deploy | Current config | Resulting actions |
|-----------------|----------------|-------------------|
| `tlsPort` unset | `tlsPort` unset | No-op (no drift). |
| `tlsPort` unset | `tlsPort` = N | Container recreated with new `<N>:443/tcp` binding. If `traefik.yml` is generated (not preserved), regenerated with `websecure` entrypoint. If preserved without `websecure`, deploy succeeds with `gateway.config.tls_port_no_websecure` warning. |
| `tlsPort` = N₁ | `tlsPort` = N₂ (different host port) | Container recreated with new binding. Generated `traefik.yml` unchanged structurally (still has `websecure: address: :443`); preserved file untouched. |
| `tlsPort` = N | `tlsPort` unset | Container recreated without `443/tcp` binding. Generated `traefik.yml` regenerated without `websecure` entrypoint; preserved file untouched. |

All recreations flow through the existing `removeStaleContainer` → `createFreshContainer` path; no new lifecycle is introduced.

## 8. No persisted state

This feature does NOT add any new files to the state directory, no entries to `DeploymentStore`, no sidecar config, and no new fields to existing state types. The only state-package change is the `ConfigHash` signature extension (Section 5). The hash itself continues to live in the existing `Deployment.ConfigHash` field.
