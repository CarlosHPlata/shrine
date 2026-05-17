# Phase 1 — Data Model: Composition Bundles

**Feature**: 017-refactor-composition-root
**Date**: 2026-05-15

This refactor introduces no domain entities. The "data model" for this feature is the set of value types the new `internal/app/` package exposes — the bundles handlers consume and the helpers that build them.

---

## Public types in `internal/app/`

### `DeployBundle`

The dependency set required to execute `shrine deploy` (live, not dry-run).

| Field | Type | Source | Notes |
|-------|------|--------|-------|
| `Out` | `io.Writer` | caller | stdout writer for user-visible output |
| `Cfg` | `*config.Config` | caller | already-loaded shrine config |
| `Store` | `*state.Store` | caller | already-loaded state store |
| `Paths` | `*config.Paths` | caller | resolved config/state directories |
| `SpecsDir` | `string` | resolved at build time via `Cfg.ResolveSpecsDir(manifestDir)` | passed into the Traefik plugin |
| `Observer` | `engine.Observer` | private `newObserverPair` | a `MultiObserver` over terminal + file logger |
| `Vault` | result of `infisicalplugin.New` | private `newVault` | type matches the engine's `EngineOptions.Vault` field |
| `ContainerBackend` | `engine.ContainerBackend` | private `newContainerBackend` | passed into Traefik plugin and engine |
| `TraefikPlugin` | `*traefik.Plugin` | private `newTraefikPlugin` | nullable *only* if construction errors; bundle return path enforces non-nil on success |
| `Routing` | `engine.RoutingBackend` | derived from `TraefikPlugin.RoutingBackend()` if `TraefikPlugin.IsActive()`; otherwise `nil` | preserves today's behaviour: nil routing backend is silently skipped per Constitution Principle III |
| `Engine` | `engine.DeployEngine` | private `newLocalEngine` | wired with `Store`, `Cfg.Registries`, `Observer`, `Routing`, `Vault` |

Validation:
- `Cfg.ValidateRegistries()` is called inside `BuildDeployBundle` before any construction; an error there short-circuits the build.

### `ApplyBundle`

The dependency set required to execute `shrine apply --file`. A strict subset of `DeployBundle` (no Traefik, no container backend).

| Field | Type | Notes |
|-------|------|-------|
| `Out` | `io.Writer` | as above |
| `Cfg` | `*config.Config` | as above |
| `Store` | `*state.Store` | as above |
| `Paths` | `*config.Paths` | as above |
| `Observer` | `engine.Observer` | as above |
| `Vault` | (vault plugin type) | as above |
| `Engine` | `engine.DeployEngine` | wired with `Store`, `Cfg.Registries`, `Observer`, `Vault` (no `Routing`) |

Validation:
- `Cfg.ValidateRegistries()` is called inside `BuildApplyBundle` before any construction.

### `TeardownBundle`

The dependency set required to execute `shrine teardown`. No vault (teardown does not need to resolve secrets).

| Field | Type | Notes |
|-------|------|-------|
| `Out` | `io.Writer` | as above |
| `Cfg` | `*config.Config` | as above |
| `Store` | `*state.Store` | as above |
| `Paths` | `*config.Paths` | as above |
| `SpecsDir` | `string` | resolved via `Cfg.ResolveSpecsDir("")`, mirroring today's `teardown.go` |
| `Observer` | `engine.Observer` | as above |
| `TraefikPlugin` | `*traefik.Plugin` | constructed with `nil` container backend (mirrors today's behaviour — teardown does not push images) |
| `Routing` | `engine.RoutingBackend` | derived from `TraefikPlugin.RoutingBackend()` if active; nil otherwise |
| `Engine` | `engine.DeployEngine` | wired with `Store`, `Cfg.Registries`, `Observer`, `Routing` (no `Vault`) |

---

## Public constructors in `internal/app/`

```go
func BuildDeployBundle(
    cfg *config.Config,
    store *state.Store,
    paths *config.Paths,
    manifestDir string,
    out io.Writer,
) (*DeployBundle, func() error, error)

func BuildApplyBundle(
    cfg *config.Config,
    store *state.Store,
    paths *config.Paths,
    out io.Writer,
) (*ApplyBundle, func() error, error)

func BuildTeardownBundle(
    cfg *config.Config,
    store *state.Store,
    paths *config.Paths,
    out io.Writer,
) (*TeardownBundle, func() error, error)
```

Returned tuple invariants (per [[research.md]] R3, R4):

- On success: `(*Bundle, func() error /* nonzero, idempotent, errors.Join over closers */, nil)`.
- On failure: `(nil, nil, error)` — any partially-constructed closers are invoked internally before returning.

---

## Private helpers in `internal/app/components.go`

These are package-private functions reused across bundle constructors. Each is a single-responsibility wrapper over an existing leaf constructor.

| Helper | Wraps | Used by |
|--------|-------|---------|
| `newObserverPair(out io.Writer, paths *config.Paths) (engine.Observer, func() error, error)` | `ui.NewTerminalObserver` + `ui.NewFileLogger` + `engine.MultiObserver` composition | Deploy, Apply, Teardown |
| `newVault(cfg *config.Config) (vaultPluginType, error)` | `infisicalplugin.New(cfg.Plugins.Secrets.Infisical)` | Deploy, Apply |
| `newContainerBackend(store *state.Store, cfg *config.Config, observer engine.Observer) (engine.ContainerBackend, error)` | `local.NewContainerBackend` | Deploy |
| `newTraefikPlugin(cfg *config.Config, container engine.ContainerBackend, specsDir string, observer engine.Observer) (*traefik.Plugin, error)` | `traefik.New(cfg.Plugins.Gateway.Traefik, container, specsDir, observer)` | Deploy, Teardown, DryRun (validation-only call site) |
| `newLocalEngine(opts local.EngineOptions) (engine.DeployEngine, error)` | `local.NewLocalEngine` | Deploy, Apply, Teardown |
| `routingFromPlugin(plugin *traefik.Plugin) (engine.RoutingBackend, error)` | `plugin.IsActive() ? plugin.RoutingBackend() : nil, nil` | Deploy, Teardown |

All helpers return errors verbatim with a short `fmt.Errorf("<slot>: %w", err)` wrapping at the bundle-constructor boundary (per R3), not inside the helper itself.

---

## State transitions

Construction is linear; there are no state machines. Each `BuildXBundle` walks a fixed sequence:

```
ValidateRegistries → newObserverPair → [scenario-specific constructors in order] → newLocalEngine → return bundle
```

If any step errors, the constructor returns immediately after invoking the cleanup accumulated so far.

---

## Relationships

- `cmd/<command>.go` — depends on `internal/app/` (calls `BuildXBundle`) and `internal/handler/` (calls `handler.<Command>(bundle, ...)`). Today it depends on `internal/handler/` only; after this change `cmd/` gains an `internal/app/` import and the handler imports listed below shrink.
- `internal/handler/<command>.go` — depends on `internal/app/` (consumes the bundle types). After this change, `handler/deploy.go`, `handler/apply.go`, and `handler/teardown.go` no longer directly import `internal/plugins/...`, `internal/engine/local`, or `internal/ui` (FR-001, SC-001).
- `internal/app/` — depends on `internal/config`, `internal/state`, `internal/engine`, `internal/engine/local`, `internal/plugins/gateway/traefik`, `internal/plugins/secrets/infisical`, `internal/ui`. These are the imports that left `internal/handler/`.

---

## Validation rules from spec requirements

| Spec ref | Modeled by |
|----------|------------|
| FR-001 | Bundle structs hold *constructed* values; handler signatures take `*Bundle` and never call leaf constructors |
| FR-002 | Each leaf has exactly one private helper in `internal/app/components.go` |
| FR-003 | Each scenario has its *own* bundle type with only the fields it needs (TeardownBundle has no `Vault`; ApplyBundle has no `Routing`/`TraefikPlugin`) |
| FR-004 | `BuildXBundle` wraps with `fmt.Errorf("<slot>: %w", err)` — preserves underlying message |
| FR-005 | No bundle field, helper, or call ordering changes from today's behaviour; all downstream calls (engine ExecuteDeploy, plugin.Deploy, etc.) remain in handlers |
| FR-006 | Existing `internal/handler/status_test.go` does not touch the migrated functions |
| FR-007 | New scenarios add a new bundle struct + constructor; reuse existing private helpers |
| FR-008 | Three handler files in scope; verified by R6 inventory in [[research.md]] |
