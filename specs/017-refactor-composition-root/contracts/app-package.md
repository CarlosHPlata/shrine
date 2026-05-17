# Contract — `internal/app/` package

**Feature**: 017-refactor-composition-root
**Date**: 2026-05-15

This is the public API contract for the new `internal/app/` package. The package has no external (HTTP / CLI) interface; this contract is what the rest of the Shrine codebase calls.

---

## Imported by

- `cmd/deploy.go`
- `cmd/apply.go`
- `cmd/teardown.go`
- `internal/handler/deploy.go`, `internal/handler/apply.go`, `internal/handler/teardown.go` (for the bundle types only)

## Exports

### Bundle types

```go
package app

// DeployBundle is the dependency set passed to handler.Deploy.
type DeployBundle struct {
    Out              io.Writer
    Cfg              *config.Config
    Store            *state.Store
    Paths            *config.Paths
    SpecsDir         string
    Observer         engine.Observer
    Vault            <vaultPluginType>           // matches local.EngineOptions.Vault
    ContainerBackend engine.ContainerBackend
    TraefikPlugin    *traefik.Plugin
    Routing          engine.RoutingBackend       // nil when plugin is inactive
    Engine           engine.DeployEngine
}

// ApplyBundle is the dependency set passed to handler.ApplySingle.
type ApplyBundle struct {
    Out      io.Writer
    Cfg      *config.Config
    Store    *state.Store
    Paths    *config.Paths
    Observer engine.Observer
    Vault    <vaultPluginType>
    Engine   engine.DeployEngine
}

// TeardownBundle is the dependency set passed to handler.Teardown.
type TeardownBundle struct {
    Out           io.Writer
    Cfg           *config.Config
    Store         *state.Store
    Paths         *config.Paths
    SpecsDir      string
    Observer      engine.Observer
    TraefikPlugin *traefik.Plugin
    Routing       engine.RoutingBackend          // nil when plugin is inactive
    Engine        engine.DeployEngine
}
```

### Constructors

```go
// BuildDeployBundle composes the dependency graph for `shrine deploy`.
//
// On success, the returned cleanup func is non-nil, idempotent, and joins
// any closer errors via errors.Join. Callers MUST defer it.
//
// On failure, all three return values are zero — no cleanup is needed
// because partial state was unwound internally.
func BuildDeployBundle(
    cfg *config.Config,
    store *state.Store,
    paths *config.Paths,
    manifestDir string,
    out io.Writer,
) (*DeployBundle, func() error, error)

// BuildApplyBundle composes the dependency graph for `shrine apply --file`.
// Same return-value contract as BuildDeployBundle.
func BuildApplyBundle(
    cfg *config.Config,
    store *state.Store,
    paths *config.Paths,
    out io.Writer,
) (*ApplyBundle, func() error, error)

// BuildTeardownBundle composes the dependency graph for `shrine teardown`.
// Same return-value contract as BuildDeployBundle.
func BuildTeardownBundle(
    cfg *config.Config,
    store *state.Store,
    paths *config.Paths,
    out io.Writer,
) (*TeardownBundle, func() error, error)
```

---

## Behavioural contract

### Order of operations (each `BuildXBundle`)

1. `cfg.ValidateRegistries()` — return wrapped error on failure.
2. Construct observer pair — return on failure.
3. Construct scenario-specific dependencies in dependency order:
   - Deploy: container backend → Traefik plugin → vault → routing-from-plugin → engine.
   - Apply:  vault → engine.
   - Teardown: Traefik plugin (with `nil` container backend) → routing-from-plugin → engine.
4. Return `(bundle, cleanup, nil)`.

If any step at (3) fails, invoke the partial cleanup accumulated so far and return `(nil, nil, fmt.Errorf("<slot>: %w", err))`.

### Error wrapping prefixes

The following slot prefixes are part of this contract (test-observable):

| Slot | Prefix |
|------|--------|
| Registry validation | `validating registries: ` |
| Observer pair | `observer: ` |
| Container backend | `container backend: ` |
| Traefik plugin | `traefik: ` |
| Vault plugin | `vault: ` |
| Routing backend extraction | `routing: ` |
| Local engine | `engine: ` |

These prefixes preserve the wrapped underlying error verbatim via `%w`. Existing CLI consumers (humans reading stderr, integration tests asserting on substrings) will see `<slot>: <existing error text>`.

### Cleanup semantics

- The returned `func() error` is safe to call once.
- It returns `nil` when no closers errored.
- It returns `errors.Join(...)` when one or more closers errored.
- Today's only closer is `fileLogger.Close()`; future closers append.
- Cleanup is **never** called by `internal/app/` after a successful return — that is the caller's responsibility.

### Concurrency

Bundles are not safe for concurrent use across goroutines. Each CLI invocation builds its own bundle; this matches today's per-handler-call construction.

---

## What this contract does NOT promise

- Bundle struct fields MAY be added in future without a major version bump (handlers would simply leave them unread). Removing or renaming a field IS a breaking change.
- The exact text of an underlying plugin error MAY change if the plugin's own error message changes — only the slot prefix added by `internal/app/` is part of this contract.
- The package does not expose the private helpers (`newObserverPair`, `newVault`, etc.). They are implementation detail and may be reorganized freely.

---

## Verification

This contract is verified by:

- **Static**: SC-001 — `grep -rE 'infisicalplugin\.New|traefik\.New|local\.NewLocalEngine|ui\.NewTerminalObserver|ui\.NewFileLogger' internal/handler/` returns nothing.
- **Static**: SC-002 — `grep -r '<leaf constructor name>' internal/` returns exactly one hit per leaf (inside `internal/app/`).
- **Unit**: `internal/app/app_test.go` calls each `BuildXBundle` with a representative config and asserts (a) success returns non-nil bundle + non-nil cleanup, (b) injecting an invalid plugin config produces an error whose message starts with the documented slot prefix.
- **Integration**: existing `tests/integration/` deploy / apply / teardown scenarios continue to pass — SC-005, Constitution Principle V.
