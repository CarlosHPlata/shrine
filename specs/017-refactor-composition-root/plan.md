# Implementation Plan: Separate Composition Root from `internal/handler/`

**Branch**: `017-refactor-composition-root` | **Date**: 2026-05-15 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/017-refactor-composition-root/spec.md`

## Summary

Move shared dependency construction (Infisical vault plugin, Traefik routing plugin, local deploy engine, terminal observer, file logger) out of `internal/handler/` and into a new `internal/app/` package that owns the composition root. Each command-shaped scenario (deploy, apply, teardown, dry-run) gets a `Build<Scenario>Bundle` constructor in `internal/app/` that returns a struct of fully-constructed dependencies plus a cleanup function. Handlers consume the bundle as a value and contain only request-shaped logic (planning, output formatting, error mapping).

This is **Option B** from issue #24 (a dedicated composition package). Option C (push composition into `cmd/`) is rejected because Constitution Principle II requires `cmd/` to remain a thin dispatcher and business logic to live in `internal/handler/` — composition does not belong in either, so it gets its own package. Option A (leave it) is rejected because the duplication is *already* present across `deploy.go`, `apply.go`, and `teardown.go`, and the constitution's DRY rule (Principle VII) forbids leaving it.

## Technical Context

**Language/Version**: Go 1.24+
**Primary Dependencies**: existing only — `internal/config`, `internal/state`, `internal/engine`, `internal/engine/local`, `internal/plugins/gateway/traefik`, `internal/plugins/secrets/infisical`, `internal/ui`, `internal/planner`. No new third-party imports.
**Storage**: N/A (purely a structural refactor of in-process wiring)
**Testing**: `go test ./...` for unit tests; `make test-integration` (Constitution Principle V gate) using `tests/integration/` `NewDockerSuite` harness against the real `shrine` binary.
**Target Platform**: Linux server / homelab (Docker daemon)
**Project Type**: Single Go CLI (`cmd/` + `internal/`)
**Performance Goals**: No change in CLI startup time, command latency, or memory footprint. Construction is performed once per invocation; the bundle is a value pass.
**Constraints**: Zero CLI behaviour change (FR-005, SC-004). Construction-error wording must remain at least as informative (FR-004). Existing tests must pass without weakened assertions (FR-006, SC-005).
**Scale/Scope**: Three in-scope handler files (`deploy.go`, `apply.go`, `teardown.go`) plus `DryRun` co-located in `deploy.go`. Approximately 200 LOC moved out of `internal/handler/`, replaced with bundle consumption. New `internal/app/` package estimated at ~300 LOC including helpers and tests.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] N/A — no manifest or CLI surface change |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] N/A — no new commands; `cmd/` stays a thin dispatcher (now even thinner) |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass — no engine core change; backend interfaces unchanged |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — bundles correspond to existing scenarios (deploy, apply, teardown, dry-run); no speculative abstractions. Private helpers (`newObserverPair`, `newVault`, `newTraefikPlugin`, `newLocalEngine`) each have ≥2 concrete callers from day one and are introduced specifically to remove existing duplication, which Principle VII *requires* |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass — the existing `tests/integration/` suite (deploy, apply, teardown scenarios) is the gate. No new integration test files are needed because no behaviour changes; SC-005 requires the existing suite to continue passing |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] N/A — refactor preserves existing engine call ordering verbatim |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | [x] Pass — this refactor *is* the application of Principle VII to handler-layer wiring. Helper names (`newObserverPair`, `newVault`, `buildDeployBundle`) are intention-revealing |

**No violations.** Complexity Tracking table is empty.

## Project Structure

### Documentation (this feature)

```text
specs/017-refactor-composition-root/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0: option A/B/C decision and bundle-shape decision
├── data-model.md        # Phase 1: bundle types and their fields
├── quickstart.md        # Phase 1: how to add a new handler that uses composed deps
├── contracts/
│   └── app-package.md   # Phase 1: public API of internal/app
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # Phase 2 output (/speckit-tasks command)
```

### Source Code (repository root)

```text
cmd/
├── deploy.go            # MODIFIED: calls app.BuildDeployBundle, then handler.Deploy(bundle, dir)
├── apply.go             # MODIFIED: calls app.BuildApplyBundle, then handler.ApplySingle(bundle, file, dir)
├── teardown.go          # MODIFIED: calls app.BuildTeardownBundle, then handler.Teardown(bundle, team)
├── ...                  # other cmd files unchanged
└── cmd_test.go          # unchanged

internal/
├── app/                 # NEW PACKAGE — composition root
│   ├── app.go           # Public bundle types and Build<Scenario>Bundle constructors
│   ├── components.go    # Private helpers: newObserverPair, newVault, newTraefikPlugin, newLocalEngine
│   └── app_test.go      # Unit tests with stand-in configs
│
├── handler/             # MODIFIED — no more direct plugin/engine/observer construction
│   ├── deploy.go        # Deploy(bundle *app.DeployBundle, manifestDir string) + DryRun(out, dir, store, cfg) (validation only — no infra construction)
│   ├── apply.go         # ApplySingle(bundle *app.ApplyBundle, file, dir string) + ApplyTeams unchanged
│   ├── teardown.go      # Teardown(bundle *app.TeardownBundle, team string)
│   ├── apps.go          # unchanged (no shared-dep construction today)
│   ├── apps_resources.go etc. # unchanged
│   ├── deployments.go   # unchanged
│   ├── resources.go     # unchanged
│   ├── status.go        # unchanged
│   ├── status_test.go   # unchanged
│   └── teams.go         # unchanged
│
├── config/              # unchanged
├── engine/              # unchanged
├── manifest/            # unchanged
├── planner/             # unchanged
├── plugins/             # unchanged
├── resolver/            # unchanged
├── state/               # unchanged
├── topo/                # unchanged
├── ui/                  # unchanged
└── updater/             # unchanged

tests/
└── integration/         # unchanged — existing scenarios are the gate (SC-005, Principle V)
```

**Structure Decision**: Single Go module, `cmd/` + `internal/` layout (existing). The only structural change is the addition of `internal/app/` as the composition root and the simplification of `internal/handler/{deploy,apply,teardown}.go`. No package boundary changes elsewhere; no new external dependencies.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| _(none)_  | _(n/a)_    | _(n/a)_                              |
