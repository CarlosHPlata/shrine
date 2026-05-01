# Phase 1: Data Model

**Feature**: Fix Traefik Dashboard Generated in Static Config
**Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Research**: [research.md](./research.md)

This feature is a config-generation bug fix. It does not introduce new manifest kinds, persisted state, or wire-format entities. The "data model" below documents the changes to the YAML shapes and Go structs the plugin emits and consumes — the smallest set of structural changes that the implementation will make.

## Files written by the plugin (operator-visible YAML)

### 1. `<routing-dir>/traefik.yml` (Traefik static configuration) — MODIFIED

**Shape after this fix** (only valid Traefik static keys remain):

```yaml
entryPoints:
  web:
    address: ":80"
  traefik:                # present only when dashboard.port is configured
    address: ":8080"
api:                      # present only when dashboard.port is configured
  dashboard: true
providers:
  file:
    directory: /etc/traefik/dynamic
    watch: true
```

**What is removed**: the entire top-level `http:` block (with `middlewares.dashboard-auth` and `routers.dashboard`). It now lives in the dashboard dynamic file (entity 2 below).

**Operator edits**: same preservation regime as today — if the file exists when `shrine deploy` runs, it is left untouched.

### 2. `<routing-dir>/dynamic/__shrine-dashboard.yml` (NEW dashboard dynamic configuration)

**Shape**:

```yaml
http:
  middlewares:
    dashboard-auth:
      basicAuth:
        users:
          - "admin:{SHA}<base64-sha1-of-password>"
  routers:
    dashboard:
      rule: "PathPrefix(`/dashboard`) || PathPrefix(`/api`)"
      service: api@internal
      entryPoints:
        - traefik
      middlewares:
        - dashboard-auth
```

**Generation rules**:
- File is created on `shrine deploy` if and only if `plugins.gateway.traefik.dashboard.port > 0` AND the file does not already exist.
- On subsequent deploys: if the file exists, it is preserved unchanged (Decision 3 in research.md). If it has been deleted by the operator, it is regenerated from the current Shrine config.
- Filename is fixed (`__shrine-dashboard.yml`) and namespaced via the `__` prefix to be non-collidable with per-app routing files (FR-009; Decision 1).
- Content is exactly the dashboard router and the `dashboard-auth` middleware — nothing else.

### 3. `<routing-dir>/dynamic/<team>-<service>.yml` (existing per-app dynamic files) — UNCHANGED

The new dashboard file is a sibling of these files and is processed by the same Traefik file provider.

## Go types changed in `internal/plugins/gateway/traefik/spec.go`

### `staticConfig` — MODIFIED

```go
type staticConfig struct {
    EntryPoints map[string]entryPoint `yaml:"entryPoints"`
    API         *apiConfig            `yaml:"api,omitempty"`
    Providers   providersConfig       `yaml:"providers"`
    // HTTP field REMOVED — dashboard surface now lives in the dynamic file.
}
```

### `httpConfig`, `middleware`, `basicAuth`, `router` — UNCHANGED

These types remain, both because `routing.go`'s per-app route generator depends on them and because the new dashboard dynamic file re-uses the same shapes. No new types are introduced.

### New helper types (private, in `config_gen.go`)

```go
// Wraps the http section the dashboard dynamic file emits.
// Identical in shape to the per-app routing wrapper in routing.go.
type dashboardDynamicDoc struct {
    HTTP httpConfig `yaml:"http"`
}

// Single-field shadow used only by the legacy-block detector.
// Decoupled from staticConfig so the detector does not break when
// the static struct's field set evolves.
type legacyHTTPProbe struct {
    HTTP *yaml.Node `yaml:"http"`
}
```

Both types are package-private and have no semantic identity beyond being YAML-marshal targets.

## State transitions across redeploys

```
                  ┌───────────────────────────────────────────────────┐
                  │ shrine deploy with dashboard configured           │
                  └─────────────────────┬─────────────────────────────┘
                                        │
                  ┌─────────────────────┼─────────────────────────────┐
                  │                     │                             │
       traefik.yml absent        traefik.yml present          traefik.yml present
       AND dashboard file        AND dashboard file           AND dashboard file
       absent                    absent                       present
                  │                     │                             │
                  ▼                     ▼                             ▼
       generate static          preserve static              preserve static
       (no http block)          (warn if legacy http         (warn if legacy http
       generate dashboard       block detected)              block detected)
       dynamic                  generate dashboard           preserve dashboard
                                dynamic                      dynamic
```

Three discrete observer events flank the transitions:

| Event | Status | When |
|-------|--------|------|
| `gateway.config.generated` | Info | static `traefik.yml` written for the first time |
| `gateway.config.preserved` | Info | static `traefik.yml` already exists; left untouched |
| `gateway.config.legacy_http_block` | **Warning** | static `traefik.yml` present and contains an `http:` block (legacy buggy artefact) |
| `gateway.dashboard.generated` | Info | dashboard dynamic file written for the first time |
| `gateway.dashboard.preserved` | Info | dashboard dynamic file already exists; left untouched |

The legacy-block warning is independent of the static-file generated/preserved pair: it is emitted once per deploy in addition to the relevant generated/preserved event, every deploy where the block remains. See Decision 2 in research.md.

## Data not modeled here

- **`DeploymentStore` records**: untouched by this feature. The dashboard's accessibility is a function of the file provider's contents, not of Shrine's internal state.
- **Manifest schema (`shrine/v1`)**: untouched. The fix is invisible at the manifest level — operators see the same `plugins.gateway.traefik.dashboard` config block they had before.
- **Dry-run output**: no new printed lines beyond the new event names surfacing through the existing terminal logger (which is a UI concern documented in `contracts/observer-events.md`, not a data-model concern).
