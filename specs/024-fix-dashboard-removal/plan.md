# Implementation Plan: Remove Stale Dashboard Config on Dashboard Removal

**Branch**: `024-fix-dashboard-removal` | **Date**: 2026-08-23 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/024-fix-dashboard-removal/spec.md` (GitHub issue #35)

## Summary

When the Traefik dashboard configuration is removed, `RoutingBackend.Finalize` currently skips dashboard generation and leaves the stale `__shrine-dashboard.yml` routing the dashboard with old credentials forever. The fix adds the missing removal branch to `Finalize` — probe, delete via the existing (currently dead) `removeFileFn` seam, and emit the `gateway.dashboard.removed` event whose name spec 010 already reserved — failing the deploy loudly if deletion fails. Bundled cleanup from the same incomplete spec-010 work: delete the dead duplicate `Plugin.portBindings()` and repoint the spec-011 port-binding unit tests at the live `RoutingBackend.portBindings()`, adding an explicit dashboard-port case. No engine, handler, CLI, manifest, or dry-run changes.

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: `gopkg.in/yaml.v3` (config marshalling), Docker SDK (indirect — container recreation picks up the changed port bindings); no new dependencies
**Storage**: Filesystem only — one generated file under `<routingDir>/dynamic/`; no state-store, manifest, or config schema changes
**Testing**: `go test ./...` for units (package policy: unit tests never touch the filesystem — all fs access goes through the stubbable `lstatFn`/`writeFileFn`/`mkdirAllFn`/`removeFileFn` seams); integration via `tests/integration/` `NewDockerSuite` harness with `-tags integration`, authored + compile-checked locally, executed by CI
**Target Platform**: Linux server (Docker host)
**Project Type**: Single Go CLI
**Performance Goals**: N/A — one extra `lstat` (and at most one `remove`) per deploy
**Constraints**: Observer event contract is additive-only (spec 010 contract); `RemoveRoute`'s orphan-warn policy (never delete per-app files) must remain untouched; dry-run must remain mutation-free by construction
**Scale/Scope**: 3 production files touched in `internal/plugins/gateway/traefik/` (~30 net new lines, ~15 deleted), 2 test files, 1 integration scenario

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | Pass — no new capability surface; behavior fix keyed off the existing `plugins.gateway.traefik.dashboard` config |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | N/A — no new commands; dry-run stays mutation-free via the print-only routing backend (research Decision 5) |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | Pass — all changes live inside the Traefik plugin's `RoutingBackend`; `internal/engine/` untouched |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | Pass — no new abstractions; reuses existing seams/helpers and deletes a dead duplicate (net-negative complexity) |
| V. Integration-Test Gate | Does this phase map to an integration test scenario using `NewDockerSuite` against a real binary? | Pass — new remove-dashboard-redeploy scenario in `tests/integration/traefik_plugin_test.go`, TDD-ordered (tests authored red-first; CI executes the gate) |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | N/A — no state records touched; the only mutation is a config-file deletion, and the removal event is emitted only after `os.Remove` succeeds |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | Pass — `removeStaleDashboardDynamicConfig` mirrors `generateDashboardDynamicConfig`; duplicate `portBindings` implementation deleted rather than kept in parallel |

**Post-Phase-1 re-check**: all gates unchanged — Pass. No Complexity Tracking entries required.

## Project Structure

### Documentation (this feature)

```text
specs/024-fix-dashboard-removal/
├── plan.md              # This file
├── research.md          # Phase 0 — 7 decisions (placement, event, failure semantics, trigger, dry-run, dead code, tests)
├── data-model.md        # Phase 1 — dashboard-file lifecycle state machine + removed-code table
├── quickstart.md        # Phase 1 — manual verification walkthrough
├── contracts/
│   └── observer-events.md  # Phase 1 — gateway.dashboard.removed event contract
└── tasks.md             # Phase 2 (/speckit-tasks — NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/plugins/gateway/traefik/
├── routing.go           # MODIFIED: Finalize() gains the else-branch calling the removal helper (routing.go:219)
├── config_gen.go        # MODIFIED: new removeStaleDashboardDynamicConfig(routingDir, observer) beside its generate mirror
├── plugin.go            # MODIFIED: delete dead Plugin.portBindings() (plugin.go:128-142)
├── plugin_test.go       # MODIFIED: delete TestPlugin_PortBindings_* (moved to routing_test.go)
└── routing_test.go      # MODIFIED: repointed TestRoutingBackend_PortBindings_* + new dashboard-port case
                         #           + Finalize removal-branch unit tests (present/absent/stat-error/remove-error/configured)

tests/integration/
└── traefik_plugin_test.go  # MODIFIED: remove-dashboard-redeploy scenario (deploy → drop dashboard block → redeploy → file gone)
```

**Structure Decision**: Single-project Go CLI; every production change is confined to the existing `internal/plugins/gateway/traefik/` package, keeping the fix behind the `RoutingBackend` interface per Constitution III. `internal/engine/dryrun/` is deliberately untouched (research Decision 5).

## Design Outline

1. **Removal helper** (`config_gen.go`): `removeStaleDashboardDynamicConfig(routingDir string, observer engine.Observer) error` — build the path from `dashboardDynamicFileName()`, `isPathPresent` probe (stat error → wrapped error), absent → return nil silently (FR-004), present → `removeFileFn(path)` (failure → wrapped error naming path and cause, FR-006), success → emit `gateway.dashboard.removed` `StatusInfo` `{path}` (FR-003).
2. **Finalize branch** (`routing.go:219`): `if r.hasDashboard() { generate… } else { remove… }` (FR-001/FR-002). Ordering within Finalize is unchanged — the branch runs after static-config generation, before container creation, so the recreated container also drops the dashboard port binding.
3. **Dead code** (`plugin.go`): delete `Plugin.portBindings()`; `removeFileFn` becomes live (FR-008/FR-010).
4. **Tests**: TDD-ordered per Constitution V — unit tests for the removal branch and the repointed/new port-binding tests are written red-first; the integration scenario is authored alongside and compile-checked (`go vet -tags integration ./tests/integration/...`), with CI as the executing gate (FR-009, SC-005).

## Complexity Tracking

No Constitution violations — table intentionally empty.
