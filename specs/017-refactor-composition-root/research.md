# Phase 0 — Research: Composition Root Placement

**Feature**: 017-refactor-composition-root
**Date**: 2026-05-15

This document resolves the architectural unknowns deferred by `spec.md`. The spec constrains *outcomes* (no construction in `internal/handler/`, single source per shared dependency, no CLI behaviour change); this research picks the *structure* that satisfies those outcomes.

---

## R1. Where does the composition root live?

### Decision

A new package, **`internal/app/`**, owns composition. Each command-shaped scenario receives a `Build<Scenario>Bundle` constructor that returns a struct of fully-constructed dependencies plus a `func() error` cleanup.

### Rationale

The issue offers three options. Two are eliminated by a current-state read of the codebase plus the constitution:

- **Option A (leave it, document convention)** — already failing. `deploy.go`, `apply.go`, and `teardown.go` each construct the observer pair (`ui.NewTerminalObserver` + `ui.NewFileLogger` + `engine.MultiObserver`). `deploy.go` and `apply.go` each construct the vault. `deploy.go` and `teardown.go` each construct the Traefik plugin. Issue #3 was a previous instance of the same drift. Constitution Principle VII ("repeated logic MUST be extracted into a shared private or helper function") forbids continuing to leave it.

- **Option C (push composition into `cmd/`)** — rejected. Constitution Principle II says "Commands MUST be thin dispatchers — business logic lives in `internal/handler/`, not in `cmd/`." Composition is not business logic, but moving 30+ lines of plugin / engine wiring into each Cobra command would push `cmd/` away from "thin dispatcher." The current `cmd/deploy.go` and `cmd/apply.go` files are roughly 50 lines each, mostly flag wiring; expanding them with construction code would obscure the dispatch they exist for.

- **Option B (new package)** — selected. A composition package keeps `cmd/` thin (it asks `app/` for a bundle and passes it to a handler) and keeps `handler/` request-shaped (it takes a bundle and runs business logic).

Naming: `internal/app/` is preferred over `internal/wire/` because the latter evokes Google's `google/wire` code-generation tool and would mislead readers into expecting compile-time DI. Plain `app/` is conventional Go for "the application graph."

### Alternatives considered

- `internal/composition/` — accurate but unwieldy.
- `internal/bootstrap/` — implies one-time process startup; misleading because each command invocation rebuilds the graph.
- `internal/runtime/` — conflicts with the Go runtime concept.
- Keeping wiring in a new file `internal/handler/wire.go` — would technically remove duplication while leaving wiring inside `handler/`. Rejected because it dodges the actual issue (handler/ is doing composition work) and would simply produce the same package-name-promise mismatch the issue calls out.

---

## R2. What shape does the composition API take?

### Decision

**Per-scenario bundle constructors**, with private helpers shared between them. Public API:

```go
package app

type DeployBundle struct {
    Out             io.Writer
    Cfg             *config.Config
    Store           *state.Store
    Paths           *config.Paths
    Observer        engine.Observer
    Vault           plugins.SecretsPlugin     // or whatever the existing interface is
    TraefikPlugin   *traefik.Plugin
    ContainerBackend engine.ContainerBackend
    Engine          engine.DeployEngine
    SpecsDir        string
}

type ApplyBundle struct {
    Out      io.Writer
    Cfg      *config.Config
    Store    *state.Store
    Paths    *config.Paths
    Observer engine.Observer
    Vault    plugins.SecretsPlugin
    Engine   engine.DeployEngine
}

type TeardownBundle struct {
    Out           io.Writer
    Cfg           *config.Config
    Store         *state.Store
    Paths         *config.Paths
    Observer      engine.Observer
    TraefikPlugin *traefik.Plugin
    Routing       engine.RoutingBackend  // nil if plugin inactive
    Engine        engine.DeployEngine
}

func BuildDeployBundle(cfg *config.Config, store *state.Store, paths *config.Paths, manifestDir string, out io.Writer) (*DeployBundle, func() error, error)
func BuildApplyBundle(cfg *config.Config, store *state.Store, paths *config.Paths, out io.Writer) (*ApplyBundle, func() error, error)
func BuildTeardownBundle(cfg *config.Config, store *state.Store, paths *config.Paths, out io.Writer) (*TeardownBundle, func() error, error)
```

DryRun does not get a bundle: it does not construct shared infrastructure today (only validates registries and constructs a Traefik plugin purely to invoke its config validation, then a `dryrun.NewDryRunEngine`). Its handler signature stays as today — `func DryRun(out, dir, store, cfg)` — and it consumes the same `internal/app/` private helper for Traefik validation as the live deploy bundle does. This satisfies FR-003 (subset principle: don't force unused deps).

### Rationale

Three shapes were considered:

1. **One god-bundle with all fields** (`*app.App`). Rejected because it violates FR-003 — every command would have to either accept fields it doesn't use or have nil-check protocol.
2. **Spray of micro-constructors** (`app.NewVault`, `app.NewObserver`, etc., called by handlers). Rejected because it leaves the *order* and *combination* of construction up to each handler — the duplication moves from "calls plugin constructors" to "calls app constructors." SC-002 requires one source location per shared dep; that location is the bundle constructor in `app/`, not a sequence of calls in `handler/`.
3. **Per-scenario bundles backed by shared private helpers** (selected). Each scenario has exactly one bundle constructor; each leaf dependency has exactly one private helper. New scenarios add a new bundle and reuse helpers. Constitution Principle IV (YAGNI) is respected: bundles correspond to *existing* scenarios, not hypothetical ones, and we add a new bundle only when a new command needs one.

### Alternatives considered

- A `Builder` fluent API (`app.New().WithVault().WithRouting().Build()`). Rejected as ceremony for ceremony's sake; only three current scenarios.
- Functional options on a single `App` struct. Rejected because option-discovery via field-of-options is a poor fit when the resulting field set differs *fundamentally* by scenario (deploy needs vault+routing; apply needs vault only; teardown needs routing only).
- Interfaces for each bundle (so handlers depend on `interface{ Vault() ... }`). Rejected as over-abstraction (Principle IV — no third concrete usage exists yet); revisit if a fourth scenario lands and the interface boundary becomes load-bearing for testing.

---

## R3. How are construction errors surfaced?

### Decision

Each `Build<Scenario>Bundle` returns the **first** construction error wrapped with the dependency name, e.g. `fmt.Errorf("composing deploy bundle: vault: %w", err)`. The cleanup function returned alongside the bundle is `nil`-safe and idempotent; on construction failure, the partially-constructed cleanup is invoked internally before returning, so callers always either get `(bundle, cleanup, nil)` or `(nil, nil, err)`.

### Rationale

FR-004 requires the surface to remain at least as informative as today. Today, each handler returns errors from `infisicalplugin.New(...)`, `traefik.New(...)`, etc. with the same `%w` wrapping idiom. By wrapping in `app/` with a short prefix (`vault:`, `routing:`, `engine:`), the user gets *more* context (which bundle slot failed) without losing the underlying wrapped cause.

### Alternatives considered

- Multi-error aggregation. Rejected — construction is sequential and later steps depend on earlier ones (the engine needs the observer; the routing backend needs the container backend), so multi-error doesn't help.
- A typed `BuildError` with named fields. Rejected as YAGNI; `fmt.Errorf` with `%w` is sufficient and matches existing idiom.

---

## R4. Cleanup function semantics

### Decision

The returned `func() error` closes anything that needs closing — currently only the file logger (`fileLogger.Close()` from `ui.NewFileLogger`). It is invoked by the caller (`cmd/`) via `defer`. If multiple closers exist, errors are joined with `errors.Join`.

### Rationale

Today each handler does `defer fileLogger.Close()`. Centralizing this in the bundle keeps the handler ignorant of which deps need cleanup. Returning a function (instead of an `io.Closer`) avoids forcing every bundle struct to implement a method when one currently doesn't need any.

### Alternatives considered

- Make every bundle implement `Close() error`. Rejected — adds method to a value type for one closer; can revisit if the count grows.
- Use `t.Cleanup`-style registration. Rejected — that's a `*testing.T` idiom; in production code a returned `func()` is clearer.

---

## R5. Test strategy for `internal/app/`

### Decision

- Add `internal/app/app_test.go` as a unit test file. Following user feedback ([[feedback_unit_tests_no_filesystem]]), unit tests do not touch the filesystem; they assert that `BuildXBundle` returns a usable bundle when given a config that the real plugins accept, and an error when given an obviously invalid config (e.g., missing required fields). Plugin construction itself is the unit under test only as a smoke check; deeper plugin behaviour is covered by existing plugin tests.
- Existing `tests/integration/` scenarios remain the Constitution Principle V gate. They exercise the real `shrine` binary against a real Docker daemon; if any bundle wiring is wrong, deploy/apply/teardown integration scenarios break.
- Per [[feedback_integration_tests_slow]], the integration suite runs only as the final gate, not on every iteration.

### Rationale

The refactor changes *who constructs* the dependency graph, not *what* the graph does at runtime. Existing integration coverage is therefore the most cost-effective regression detector. Per-scenario unit tests in `internal/app/` add a fast smoke layer specifically for the wiring code itself.

### Alternatives considered

- New integration tests targeting `internal/app/` directly. Rejected — `app/` is internal wiring; black-box tests at the CLI level (which already exist) provide the same regression signal.
- Mock-based unit tests of every leaf constructor. Rejected — would reproduce the upstream plugin's own tests with no added confidence.

---

## R6. Scope inventory: which handlers are in scope?

### Decision

In scope (handlers that today directly construct one of the named shared dependencies):

| File | Constructs today |
|------|-------------------|
| `internal/handler/deploy.go` (`Deploy`) | observer pair, container backend, traefik plugin, vault, local engine |
| `internal/handler/deploy.go` (`DryRun`) | traefik plugin (validation-only), dryrun engine |
| `internal/handler/apply.go` (`ApplySingle`) | observer pair, vault, local engine |
| `internal/handler/teardown.go` (`Teardown`) | observer pair, traefik plugin, routing backend, local engine |

Out of scope (no shared-dep construction today):

- `internal/handler/apps.go` — manifest skeleton generators only
- `internal/handler/apply.go` (`ApplyTeams`) — only touches `state.Store`
- `internal/handler/deployments.go`, `resources.go`, `teams.go`, `status.go`, `status_test.go` — formatting / state-reading helpers, no plugin or engine construction (verified by `grep` for `infisicalplugin.New|traefik.New|local.NewLocalEngine|ui.NewTerminalObserver|ui.NewFileLogger` — only `deploy.go`, `apply.go`, `teardown.go` match)

### Rationale

FR-008 ("MUST cover, at minimum, the wiring duplicated today between `deploy.go` and `apply.go`") is satisfied; `teardown.go` is included because grep confirmed it's a third instance of the same duplication, and leaving it would re-grow the issue immediately. Out-of-scope handlers genuinely have no shared deps to compose, so touching them would violate Principle IV.

### Alternatives considered

- Migrate every handler "for consistency." Rejected — formatting helpers don't need composed deps, so dragging them through the bundle pattern would be ceremony without benefit.

---

## Summary of decisions

| ID | Decision | Drives |
|----|----------|--------|
| R1 | New package `internal/app/` | Project structure |
| R2 | Per-scenario bundle structs (`DeployBundle`, `ApplyBundle`, `TeardownBundle`) with shared private helpers | Data model + contracts |
| R3 | Wrap construction errors with bundle-slot context; idempotent cleanup on failure | Contracts |
| R4 | Returned `func() error` cleanup, `errors.Join` for multiple closers | Contracts |
| R5 | Unit smoke tests in `internal/app/`; existing integration suite as gate | Tasks (Phase 2) |
| R6 | In scope: `deploy.go`, `apply.go`, `teardown.go`. Out of scope: rest of `internal/handler/` | Tasks (Phase 2) |

All NEEDS CLARIFICATION markers from spec.md are now resolved (the spec deliberately had none; the architectural option choice was the only outstanding question, resolved here as R1 + R2).
