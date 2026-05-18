# Implementation Plan: Backend lifecycle finalize step

**Branch**: `018-routing-backend-finalize` | **Date**: 2026-05-17 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/018-routing-backend-finalize/spec.md`

## Summary

Add a `Finalize() error` method to the `RoutingBackend` interface so the deploy
engine owns the full routing lifecycle. The Traefik plugin's post-engine publish
step (today reached from `handler.Deploy` via `b.TraefikPlugin.Deploy()`)
becomes the body of `Traefik.RoutingBackend.Finalize()`. The engine calls
`Routing.Finalize()` exactly once at the end of `ExecuteDeploy` and
`ExecuteTeardown` on success; if the per-step loop fails, Finalize is not
called. The handler stops referencing any plugin-specific lifecycle method.
Dry-run prints a `[ROUTE]  Finalize` line to remain a faithful preview.

Scope is strictly `RoutingBackend`: `ContainerBackend` and `DNSBackend`
interfaces are unchanged (FR-009 resolved — see [research.md](./research.md)).

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: Standard library + existing internal packages
(`internal/engine`, `internal/engine/local`, `internal/engine/dryrun`,
`internal/plugins/gateway/traefik`, `internal/handler`, `internal/app`). No new
third-party imports.
**Storage**: N/A — internal interface change. Static Traefik config keeps writing
to `<specsDir>/traefik/` and `<specsDir>/traefik/dynamic/` exactly as today.
**Testing**: `go test ./...` for units; `go test -tags integration ./tests/integration/...`
via `NewDockerSuite` for the integration gate (Principle V).
**Target Platform**: Linux server with Docker daemon (homelab use case).
**Project Type**: CLI tool (`shrine`) — single-binary Cobra app.
**Performance Goals**: Refactor is behavior-preserving; no new latency budget.
The Traefik `Finalize` runs once per deploy, same cost as today's
`plugin.Deploy()` call.
**Constraints**: No operator-facing change (no new manifest field, no new flag,
no migration). All pre-existing deploy and teardown integration tests must pass
unchanged (SC-003).
**Scale/Scope**: Touches ~6 files: `internal/engine/backends.go`,
`internal/engine/engine.go`, `internal/engine/dryrun/dry_run_routing.go`,
`internal/plugins/gateway/traefik/routing.go`,
`internal/plugins/gateway/traefik/plugin.go`, `internal/handler/deploy.go`.
Plus tests: `internal/engine/engine_test.go`,
`tests/integration/traefik_plugin_test.go` (assertions only — no manifest changes).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] N/A — pure internal refactor; no new manifest fields, no CLI flags. |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] N/A — no new commands. Existing `shrine deploy` and `shrine deploy --dry-run` behavior is preserved; dry-run gains a `[ROUTE]  Finalize` line (FR-008). |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass — the change moves logic *onto* the `RoutingBackend` interface and out of plugin-specific code reached from the handler. Engine has no backend-specific branches. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — `Finalize()` has two real usages today (Traefik plugin's publish, dry-run's preview line) and one no-op shape that future backends will reuse. We explicitly do NOT add `Finalize()` to `ContainerBackend` or `DNSBackend` (FR-009 resolution) precisely to honor this principle. |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass — `tests/integration/traefik_plugin_test.go` already exercises the Traefik deploy round-trip; assertions are extended to confirm finalize runs once after the step loop. A new scenario injects a failing-finalize routing backend to verify FR-003 and SC-004 (exit non-zero, attributable error). |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] N/A — no state-store writes are added or moved. Traefik container creation moves under `Finalize`, but its existing `DeploymentStore` semantics are preserved by `local.ContainerBackend.CreateContainer`. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | [x] Pass — `Finalize` is intention-revealing; the Traefik plugin's `Deploy()` body moves into `RoutingBackend.Finalize()` and the now-empty `Plugin.Deploy()` is deleted (not left as a wrapper). No comment block is added describing what `Finalize` does — the name and interface contract carry the meaning. |

> No violations. Complexity Tracking table is empty.

## Project Structure

### Documentation (this feature)

```text
specs/018-routing-backend-finalize/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── routing-backend.md   # Phase 1 output — RoutingBackend interface contract
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/
├── engine/
│   ├── backends.go              # RoutingBackend gains Finalize() error
│   ├── engine.go                # ExecuteDeploy + ExecuteTeardown call Routing.Finalize() after step loop
│   ├── engine_test.go           # unit: finalize called once on success; not called on step-loop error
│   ├── dryrun/
│   │   └── dry_run_routing.go   # DryRunRoutingBackend.Finalize() prints a preview line
│   └── local/
│       └── local_engine.go      # composition unchanged; Routing wiring already in place
├── plugins/gateway/traefik/
│   ├── plugin.go                # Plugin.Deploy() removed; container creation moves into RoutingBackend.Finalize
│   ├── routing.go               # RoutingBackend gains Finalize() — writes static+dashboard config, creates Traefik container
│   └── routing_test.go          # unit: Finalize() generates expected files and ContainerOp
├── handler/
│   └── deploy.go                # `b.TraefikPlugin.Deploy()` line removed; handler is plan→execute→done
└── app/
    ├── app.go                   # DeployBundle.TraefikPlugin field stays for now (still owns container backend handle) but is no longer called from handler.Deploy
    └── components.go            # routing factory unchanged

tests/integration/
├── traefik_plugin_test.go       # Assertions extended: Finalize fires once after step loop; FR-003 failing-finalize scenario
├── deploy_test.go               # Verifies SC-003 (no operator-visible regression) — passes unmodified
└── teardown_test.go             # Verifies symmetric teardown-Finalize is a no-op for Traefik today
```

**Structure Decision**: The codebase already follows the structure above. This
change is in-place; no new packages, no new top-level directories. The Traefik
`RoutingBackend` (today a small struct in `routing.go`) acquires the
ContainerBackend handle it needs to start the Traefik container — that handle
is already constructed in `app.BuildDeployBundle` and is currently held by the
`*traefik.Plugin`. We route it into `RoutingBackend` at plugin construction
time so the bundle wiring is the only place that knows about both pieces.

## Complexity Tracking

> *No Constitution Check violations — table empty.*

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
