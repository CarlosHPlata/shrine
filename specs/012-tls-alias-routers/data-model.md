# Data Model: Per-Alias TLS Opt-In for Routing Aliases

**Feature**: 012-tls-alias-routers
**Phase**: 1 (Design)
**Date**: 2026-05-02

This feature does not introduce any persistent data store, new manifest kind, or new state-package fields. The "data model" here is limited to the operator-facing manifest schema delta, three small in-memory Go struct deltas (manifest → engine → traefik), and the deterministic projection table from manifest input to generated dynamic config.

## 1. Operator-facing manifest schema

### Before this feature (spec 006 + spec 008 contract)

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: personal-finances
  owner: shrine-team
spec:
  image: example/finances:1.0
  port: 8080
  routing:
    domain: finances.home.lab
    aliases:
      - host: gateway.tail9a6ddb.ts.net
        pathPrefix: /finances
        stripPrefix: false
```

### After this feature

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: personal-finances
  owner: shrine-team
spec:
  image: example/finances:1.0
  port: 8080
  routing:
    domain: finances.home.lab
    aliases:
      - host: gateway.tail9a6ddb.ts.net
        pathPrefix: /finances
        stripPrefix: false
        tls: true                 # NEW — optional, default false
```

### Field contract

| Field | Type | Required | Default | Constraints |
|-------|------|----------|---------|-------------|
| `spec.routing.aliases[].tls` | bool | no | `false` | None at the validator layer beyond YAML structural type. Non-boolean values are rejected by `yaml.Unmarshal` with a path-bearing error (e.g., `routing.aliases[1].tls: cannot unmarshal !!str into bool`). |

When unset (omitted entirely), behavior is identical to the pre-feature release — alias router attaches to `web` only, no `tls: {}` block emitted. When set to `false` explicitly, behavior is identical to omission (no error, no warning).

The field is **only** valid inside a `routing.aliases[]` entry. Declaring `tls` at `spec.routing.tls` (top level) or anywhere outside an alias entry is rejected by `yaml.Unmarshal` because the field is not declared on the `Routing` struct — the rejection produces a clear `field tls not found in type manifest.Routing` error from the parser. This satisfies FR-005 structurally; no validator code is needed.

## 2. In-memory Go struct deltas

### `internal/manifest/types.go`

```go
type RoutingAlias struct {
    Host        string `yaml:"host"`
    PathPrefix  string `yaml:"pathPrefix,omitempty"`
    StripPrefix *bool  `yaml:"stripPrefix,omitempty"`
    TLS         bool   `yaml:"tls,omitempty"`              // NEW
}
```

One field added (`TLS bool`). Plain bool, not `*bool`, because there are only two semantic states (omitted/false collapse). YAML tag is lowercase `tls` matching the issue's exemplar; Go field is uppercase `TLS` per the project's MixedCaps convention for acronyms (the same convention used by `TLSPort` in spec 011).

### `internal/engine/backends.go`

```go
type AliasRoute struct {
    Host        string
    PathPrefix  string
    StripPrefix bool
    TLS         bool                                       // NEW
}
```

One field added. Field is populated by `resolveAliasRoutes` in `engine.go` directly from `manifest.RoutingAlias.TLS`. `WriteRouteOp.AdditionalRoutes []AliasRoute` carries the new field for free; no `WriteRouteOp` change.

### `internal/plugins/gateway/traefik/spec.go`

```go
type router struct {
    Rule        string   `yaml:"rule"`
    Service     string   `yaml:"service"`
    EntryPoints []string `yaml:"entryPoints"`
    Middlewares []string `yaml:"middlewares,omitempty"`
    TLS         *tlsBlock `yaml:"tls,omitempty"`           // NEW
}

type tlsBlock struct{}                                     // NEW — marshals to {}
```

One field added on `router` (a pointer so `omitempty` works), plus one new empty-struct type. The empty struct is the YAML-marshal-friendly way to produce literal `tls: {}`.

### `internal/plugins/gateway/traefik/routing.go`

```go
type RoutingBackend struct {
    routingDir       string
    staticConfigPath string                                 // NEW — for hasWebsecureEntrypoint probe
    observer         engine.Observer
}
```

One field added. Populated by `Plugin.RoutingBackend()` as `filepath.Join(routingDir, "traefik.yml")`. Used only by the new `emitAliasTLSNoWebsecureSignal` helper.

## 3. Manifest → engine projection (resolveAliasRoutes)

```go
// internal/engine/engine.go (extended)
func resolveAliasRoutes(aliases []manifest.RoutingAlias) []AliasRoute {
    routes := make([]AliasRoute, 0, len(aliases))
    for _, alias := range aliases {
        prefix := strings.TrimRight(alias.PathPrefix, "/")
        var strip bool
        if alias.StripPrefix != nil {
            strip = *alias.StripPrefix
        } else {
            strip = prefix != ""
        }
        routes = append(routes, AliasRoute{
            Host:        alias.Host,
            PathPrefix:  prefix,
            StripPrefix: strip,
            TLS:         alias.TLS,                         // NEW — straight pass-through
        })
    }
    return routes
}
```

The pass-through is deliberate: the engine is a projection, not a policy layer. The Traefik plugin decides what to do with `TLS == true`.

## 4. Engine → Traefik dynamic-config projection

For each application with at least one alias, `WriteRoute` produces one router per alias. The router's shape is the only thing that changes per-alias:

| `ar.TLS` | Generated alias router fields |
|----------|-------------------------------|
| `false` (omitted or explicit false) | `rule: Host(...)[ && PathPrefix(...)]`, `service: <team>-<svc>`, `entryPoints: [web]`, optional `middlewares: [<strip-key>]`, no `tls` field. |
| `true` | `rule: Host(...)[ && PathPrefix(...)]`, `service: <team>-<svc>`, `entryPoints: [web, websecure]`, optional `middlewares: [<strip-key>]`, `tls: {}`. |

**The primary-domain router shape is invariant under alias `tls` flips.** It always declares `entryPoints: [web]` and never carries a `tls` field, regardless of how many aliases on the same application set `tls: true`. This is the structural guarantee behind FR-003.

### Generated dynamic config example (mixed TLS-on / TLS-off)

For a manifest with `routing.domain: finances.home.lab`, alias[0] = `host: a.example.com` (no tls), alias[1] = `host: b.example.com, pathPrefix: /finances, stripPrefix: false, tls: true`:

```yaml
http:
  routers:
    shrine-team-personal-finances:
      rule: Host(`finances.home.lab`)
      service: shrine-team-personal-finances
      entryPoints:
        - web
    shrine-team-personal-finances-alias-0:
      rule: Host(`a.example.com`)
      service: shrine-team-personal-finances
      entryPoints:
        - web
    shrine-team-personal-finances-alias-1:
      rule: Host(`b.example.com`) && PathPrefix(`/finances`)
      service: shrine-team-personal-finances
      entryPoints:
        - web
        - websecure
      tls: {}
  services:
    shrine-team-personal-finances:
      loadBalancer:
        servers:
          - url: http://shrine-team.personal-finances:8080
```

The middlewares block is empty here because alias[0] is host-only (no strip needed) and alias[1] sets `stripPrefix: false`.

## 5. Edge-case truth table

For an application with one alias entry, holding `host` and `pathPrefix` constant:

| `stripPrefix` | `tls` | Generated alias router YAML shape |
|---------------|-------|------------------------------------|
| omitted, `pathPrefix` set | omitted | `entryPoints: [web]`, has strip middleware, no tls |
| omitted, `pathPrefix` set | `false` | (same as above) |
| omitted, `pathPrefix` set | `true` | `entryPoints: [web, websecure]`, has strip middleware, `tls: {}` |
| `false`, `pathPrefix` set | `true` | `entryPoints: [web, websecure]`, no strip middleware, `tls: {}` |
| `false`, `pathPrefix` set | omitted | `entryPoints: [web]`, no strip middleware, no tls |
| omitted, `pathPrefix` unset | `true` | `entryPoints: [web, websecure]`, no strip middleware, `tls: {}` |
| omitted, `pathPrefix` unset | omitted | `entryPoints: [web]`, no strip middleware, no tls |

`stripPrefix` and `tls` compose without conflict — the middleware decision is independent of the entrypoint/TLS decision.

## 6. Validation pipeline (unchanged)

```
application.yml
    │ YAML parse (tls accepted as bool; non-bool rejected with path-bearing error per FR-004)
    │ a top-level routing.tls is rejected as "field tls not found" per FR-005
    ▼
ApplicationManifest{Spec.Routing.Aliases: [...]}
    │ manifest.Validate(...)
    │   └─ validateRoutingAliases (UNCHANGED — keys on (host, normalizedPathPrefix); ignores tls per FR-006)
    ▼
ResolvePlan → engine.resolveAliasRoutes
    │ → engine.AliasRoute{..., TLS: <pass-through>}
    ▼
plugin RoutingBackend.WriteRoute(op)
    │ For each ar in op.AdditionalRoutes:
    │   if ar.TLS == true → router.EntryPoints=[web, websecure], router.TLS=&tlsBlock{}
    │   else → router.EntryPoints=[web], router.TLS=nil
    │ If any ar.TLS == true AND !hasWebsecureEntrypoint(staticConfigPath) → emit gateway.alias.tls_no_websecure
    ▼
Generated dynamic config file: <routing-dir>/dynamic/<team>-<service>.yml
```

No new validation functions, no new error types, no new error namespaces. The collision check defined by spec 006 (FR-008/008a) inherits unchanged — `tls` is invisible to it.

## 7. State transitions across deploys

| Previous deploy | Current manifest | Resulting actions |
|-----------------|------------------|-------------------|
| Alias has no `tls` | Alias has no `tls` | No-op for this feature; existing alias-shape semantics apply. |
| Alias has no `tls` | Alias gains `tls: true` | Dynamic config regenerated with `tls: {}` block on the alias router (subject to spec 009 preservation — if file is operator-owned, no rewrite). |
| Alias has `tls: true` | Alias has no `tls` (or `tls: false`) | Dynamic config regenerated without `tls: {}` block; alias router returns to `entryPoints: [web]` only. |
| Any | New alias added with `tls: true` | New alias router emitted with TLS shape; primary domain unchanged. |
| Any | Alias removed | Alias router disappears from generated file (subject to spec 009 preservation). |

All file rewrites flow through the existing `RoutingBackend.WriteRoute` path — no new lifecycle is introduced. Spec 009's preservation regime applies unchanged: once an operator has marked a per-app dynamic config file as operator-owned, Shrine does NOT rewrite it in response to `tls` flips (FR-008). The operator must delete the file (or hand-edit it) for `tls` changes to take effect on a preserved file.

## 8. No persisted state

This feature does NOT add any new files to the state directory, no entries to `DeploymentStore`, no sidecar config, no new fields to existing state types, and no new event-store entries. The only state-adjacent surface is the in-memory `RoutingBackend.staticConfigPath` field, which is computed once per `Plugin.RoutingBackend()` call and never persisted.
