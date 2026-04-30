# Phase 1 Data Model: Routing Aliases

**Feature**: 006-routing-aliases
**Date**: 2026-04-30

This document defines the data shapes introduced or modified by this feature. It is the source of truth for field names, types, defaults, and validation rules. The integration test fixtures and Phase 2 task list are derived directly from it.

---

## 1. Manifest types (`internal/manifest/types.go`)

### 1.1 New: `RoutingAlias`

```go
// RoutingAlias declares an additional address (host + optional path prefix)
// at which an Application is reachable, in addition to its primary
// routing.domain. Consumed by the Traefik gateway plugin; silently ignored
// when no gateway plugin is active.
type RoutingAlias struct {
    Host        string `yaml:"host"`
    PathPrefix  string `yaml:"pathPrefix,omitempty"`
    // StripPrefix uses a pointer so we can distinguish "unset" (default true
    // when PathPrefix is non-empty) from "explicitly false". When PathPrefix
    // is empty, StripPrefix has no effect regardless of value.
    StripPrefix *bool  `yaml:"stripPrefix,omitempty"`
}
```

### 1.2 Modified: `Routing`

```go
type Routing struct {
    Domain     string         `yaml:"domain"`
    PathPrefix string         `yaml:"pathPrefix,omitempty"`
    Aliases    []RoutingAlias `yaml:"aliases,omitempty"`   // ← NEW
}
```

`ApplicationSpec.Routing` is unchanged in name/position; only the embedded `Routing` struct gains the slice.

---

## 2. Validation rules (`internal/manifest/validate.go`)

A new helper `validateRoutingAliases(routing Routing) []string` is invoked from `validateApplicationSpec`. Errors follow the existing multi-error pattern (return `[]string`, joined into one error in `Validate`).

| ID | Rule | Mapped FR | Error message shape |
|----|------|-----------|---------------------|
| V1 | If `len(routing.Aliases) > 0` then `routing.Domain` MUST be non-empty | FR-004 | `spec.routing.aliases is set but spec.routing.domain is empty` |
| V2 | Each alias's `Host` MUST be non-empty | FR-003 | `spec.routing.aliases[%d].host is required` |
| V3 | `Host` MUST NOT contain spaces or control characters | FR-005 | `spec.routing.aliases[%d].host %q contains invalid characters` |
| V4 | If `PathPrefix` is set, it MUST start with `/` | FR-005a | `spec.routing.aliases[%d].pathPrefix %q must start with "/"` |
| V5 | `PathPrefix` MUST NOT be the literal `/` (use empty/omitted instead) | FR-005a | `spec.routing.aliases[%d].pathPrefix must not be just "/"` |
| V6 | `PathPrefix` MUST NOT contain spaces or control characters | FR-005 | `spec.routing.aliases[%d].pathPrefix %q contains invalid characters` |
| V7 | Within one application, no two routes (primary + aliases) MAY share the same `(host, normalizedPathPrefix)` pair | FR-008 | `spec.routing: duplicate route %s%s declared on alias[%d]` |

Rules V4–V7 use `normalizedPathPrefix(p)` = `strings.TrimRight(p, "/")` so trailing-slash differences are collapsed before comparison.

> Note: V7 implements the loud-failure semantics of FR-008 (validation error on within-app duplicate, parallel to FR-008a's cross-app failure). Within-app duplication is treated as an operator mistake — analogous to a duplicate `volumes[].name` — rather than something to silently dedup.

---

## 3. Engine projection (`internal/engine/backends.go`)

### 3.1 Modified: `WriteRouteOp`

```go
type WriteRouteOp struct {
    Team             string
    Domain           string
    ServiceName      string
    ServicePort      int
    PathPrefix       string
    AdditionalRoutes []AliasRoute   // ← NEW
}

// AliasRoute is the engine-level projection of a manifest RoutingAlias.
// StripPrefix here is a plain bool: the engine resolves the manifest's
// pointer-with-default into a concrete value (default true when PathPrefix
// is non-empty, irrelevant when empty).
type AliasRoute struct {
    Host        string
    PathPrefix  string
    StripPrefix bool
}
```

### 3.2 Engine assembly (`internal/engine/engine.go`)

In `deployApplication`, the existing block

```go
routingOp := WriteRouteOp{
    Team:        application.Metadata.Owner,
    Domain:      application.Spec.Routing.Domain,
    ServiceName: application.Metadata.Name,
    ServicePort: application.Spec.Port,
    PathPrefix:  application.Spec.Routing.PathPrefix,
}
```

is extended with a small loop that maps each `manifest.RoutingAlias` to an `AliasRoute`, applying the default-strip rule. Helper: `resolveAliasRoutes(aliases []manifest.RoutingAlias) []AliasRoute`.

The `routing.configure` observer event fields are extended with `aliases` (sorted, comma-joined `host[+pathPrefix]` list) when the slice is non-empty (R5).

---

## 4. Planner cross-app collision check (`internal/planner/`)

### 4.1 New helper

```go
// detectRoutingCollisions scans the full manifest set for any two
// applications that produce a router for the same (host, pathPrefix) pair —
// whether via primary routing.domain or via routing.aliases on either side.
// Returns a multi-error formatted like the manifest validator's output.
func detectRoutingCollisions(set *ManifestSet) error
```

### 4.2 Algorithm

1. Build `seen := map[routeKey]appRef` where `routeKey = struct{ host, pathPrefix string }` (path normalized via `strings.TrimRight(p, "/")`).
2. For each application, iterate primary `(Domain, PathPrefix)` first, then each alias.
3. For each route, if `seen` already has the key with a different application, append a collision error: `routing collision: host=%q pathPrefix=%q declared by %q and %q`. Otherwise record.
4. After scan, if errors exist, join with `\n- ` and return as one multi-error.

### 4.3 Wiring

`detectRoutingCollisions` is called from the deploy command pipeline (`internal/handler/deploy.go`) immediately after the planner returns the `ManifestSet` and before `engine.ExecuteDeploy`. Failure short-circuits with no engine, no Traefik, no Docker calls — meeting FR-008a's "MUST NOT write or update gateway config" guarantee.

### 4.4 Same-app duplicates

Same-app duplicates are caught by `validateApplicationSpec` (rule V7 above) before the planner sees them. The collision detector therefore only ever fires on cross-app conflicts; within-app ones never reach it.

---

## 5. Traefik dynamic-config rendering (`internal/plugins/gateway/traefik/routing.go`)

### 5.1 File layout

One file per app at `<routingDir>/dynamic/<team>-<service>.yml`. Filename unchanged.

### 5.2 Structure

```yaml
http:
  routers:
    <team>-<service>:                 # primary
      rule: "<primaryRule>"
      service: <team>-<service>
      entryPoints: [web]
    <team>-<service>-alias-0:         # first alias
      rule: "<aliasRule0>"
      service: <team>-<service>
      entryPoints: [web]
      middlewares: [<team>-<service>-strip-0]   # only when StripPrefix=true and PathPrefix!=""
    <team>-<service>-alias-1:
      ...
  middlewares:
    <team>-<service>-strip-0:
      stripPrefix:
        prefixes: [<aliasPathPrefix0>]
  services:
    <team>-<service>:
      loadBalancer:
        servers:
          - url: http://<team>.<service>:<port>
```

**Indexing convention**: alias router names use the alias's position in `Aliases[]` as the suffix (`-alias-<i>`). Strip middleware names follow the same convention (`-strip-<i>`) — *the same index* — so the alias→middleware mapping is 1:1 and trivially traceable. The middleware-name index is therefore *sparse*: if `Aliases[1]` has no `PathPrefix`, no `-strip-1` is emitted; the next strip middleware is `-strip-2`, not `-strip-1`. Sparse-but-stable names survive a manifest reorder of unrelated aliases without churning unrelated middleware names.

### 5.3 Rule construction

Helper `buildRouterRule(host, pathPrefix string) string`:
- `pathPrefix == ""` → `Host(\`<host>\`)`
- otherwise → `Host(\`<host>\`) && PathPrefix(\`<pathPrefix>\`)`

The trailing slash on `pathPrefix` is normalized away in the manifest layer (rule V4 doesn't reject a trailing slash; the engine's `resolveAliasRoutes` does the trim) before the rule is built.

### 5.4 Spec types update

`internal/plugins/gateway/traefik/spec.go` `middleware` struct gains a pointer field:

```go
type middleware struct {
    BasicAuth   *basicAuth   `yaml:"basicAuth,omitempty"`
    StripPrefix *stripPrefix `yaml:"stripPrefix,omitempty"`   // ← NEW
}

type stripPrefix struct {
    Prefixes []string `yaml:"prefixes"`
}
```

### 5.5 Determinism

Routers and middlewares are emitted in alias-list order (the manifest's stated order). Map keys in YAML output are sorted by `yaml.v3`'s default key-sort behavior, which matches the existing `WriteRoute` output's stability properties.

---

## 6. State transitions

This feature is stateless. Reconciliation is implicit:

| Operator action | Filesystem effect | Traefik effect |
|---|---|---|
| Add an alias and `shrine deploy` | App's dynamic config file is rewritten; one new router (and possibly one new middleware) appears | Traefik file watcher reloads; new address resolves |
| Remove an alias and `shrine deploy` | App's dynamic config file is rewritten without that router/middleware | Traefik file watcher reloads; old address stops resolving |
| `shrine teardown` for the app | Dynamic config file is deleted via existing `RemoveRoute(team, name)` | Traefik file watcher reloads; all addresses stop resolving |

No new state files, no new `DeploymentStore` records.

---

## 7. Backward compatibility

| Scenario | Behavior |
|---|---|
| Manifest with no `aliases` field | YAML unmarshal yields `nil` slice; engine projects no `AdditionalRoutes`; Traefik backend takes the existing single-router code path. Output byte-identical to today (SC-005). |
| Existing `Routing.PathPrefix` on primary domain | Unchanged. No `stripPrefix` is added to the primary router (R4). |
| Manifest with `aliases` on a host with no Traefik plugin | Manifest parses & validates. Engine `deployApplication` checks `engine.Routing != nil`; when nil, neither primary nor alias routers are written. No warning emitted (FR-010). |
| Existing integration test fixtures | Continue to pass; no fixture edits required for non-alias scenarios. |
