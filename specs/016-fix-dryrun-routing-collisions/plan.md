# Implementation Plan: Detect Routing Domain Collisions in `shrine deploy --dry-run`

**Branch**: `016-fix-dryrun-routing-collisions` | **Date**: 2026-05-15 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/016-fix-dryrun-routing-collisions/spec.md`
**Source Issue**: [GitHub #21](https://github.com/CarlosHPlata/shrine/issues/21)

## Summary

`shrine deploy --dry-run` currently exits cleanly on manifest sets that contain duplicate `routing.domain` values. The collision check (`planner.DetectRoutingCollisions`) only runs inside `handler.Deploy`, and only when a routing backend (Traefik) was successfully constructed. The check is a property of the manifests, not of runtime configuration, so it belongs inside the planner — alongside `Resolve` and `Order` — where both `Deploy` and `DryRun` already converge.

Approach: move `DetectRoutingCollisions` into `planner.Plan`. The returned error is surfaced via `PlanResult.ValidationErr`, which both `handler.Deploy` and `handler.DryRun` already drain through a single uniform print-and-fail path. Remove the redundant call from `handler.Deploy`. The check no longer depends on backend wiring.

## Technical Context

**Language/Version**: Go 1.24+ (module `github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: stdlib only for the planner change; integration suite uses `testutils.NewDockerSuite` + Docker SDK
**Storage**: N/A — change operates on in-memory `*ManifestSet` produced by `planner.LoadDir`
**Testing**: `go test ./internal/planner/...` for unit coverage; `go test -tags integration ./tests/integration/...` (or `make test-integration`) for the end-to-end gate
**Target Platform**: Linux (Docker-host development and integration); CLI is platform-agnostic
**Project Type**: CLI binary (`shrine`) — single Go module
**Performance Goals**: Collision check is O(N) over applications with their aliases. Typical manifest set is tens of applications; runtime overhead is negligible (≪1ms) and invisible to interactive users.
**Constraints**: Must not change the manifest schema, the CLI surface, or the planner's public `PlanResult` shape. Must not introduce a regression on collision-free manifest sets.
**Scale/Scope**: One Go file modified in `internal/planner/`, one in `internal/handler/`, plus one new unit test case and one new integration-test scenario. No new packages, no new exported types.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] N/A — no new capability; restoring parity of existing validation on an existing field (`spec.routing.domain`). No manifest or CLI flag is added. |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] N/A — no new command. The fix specifically restores the contract that the existing `--dry-run` is a faithful preview of plan-time validation (Principle II's own rule). |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass — the change moves validation out of the handler/backend boundary and into the planner, where it belongs. Collision detection is manifest-level, not backend-level. The engine and backends are untouched. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — no new abstraction. The change consolidates an existing function's invocation site. Net code delta is small (one new call site in `planner.Plan`, one removed call site in `handler.Deploy`). |
| V. Integration-Test Gate | Does this phase map to an integration test phase in `tests/integration/` using `NewDockerSuite` against a real binary? | [x] Pass — a new integration scenario covers `shrine deploy --dry-run` against a manifest fixture with two apps sharing a `routing.domain`, asserting non-zero exit and a collision diagnostic. Test is authored before the planner edit (TDD per Principle V). |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] N/A — the change is pre-execution validation. No Docker operations and no state mutation occur on the failure path. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | [x] Pass — the change *removes* duplication risk by ensuring a single call site for `DetectRoutingCollisions`. No new comments are added; the planner method name (`DetectRoutingCollisions`) is already self-documenting. |

No violations. The Complexity Tracking table is intentionally omitted.

## Project Structure

### Documentation (this feature)

```text
specs/016-fix-dryrun-routing-collisions/
├── plan.md              # This file (/speckit-plan output)
├── spec.md              # Feature specification (/speckit-specify output)
├── research.md          # Phase 0 output (/speckit-plan)
├── data-model.md        # Phase 1 output (/speckit-plan)
├── quickstart.md        # Phase 1 output (/speckit-plan)
├── contracts/
│   └── cli-dry-run.md   # CLI exit-code & stderr contract changes
├── checklists/
│   └── requirements.md  # Spec quality checklist (already produced by /speckit-specify)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/
├── handler/
│   └── deploy.go                # MODIFY: remove the `if routing != nil { DetectRoutingCollisions(...) }` block in Deploy(); the planner now owns this check.
├── planner/
│   ├── plan.go                  # MODIFY: invoke DetectRoutingCollisions inside Plan() after Resolve() succeeds and before/after Order(); surface result via PlanResult.ValidationErr.
│   ├── collisions.go            # UNCHANGED: detection algorithm is correct.
│   └── collisions_test.go       # ADD test: plan-level integration verifying Plan() returns ValidationErr when a duplicate-domain fixture is loaded.

tests/
├── integration/
│   └── deploy_test.go           # ADD scenario: `shrine deploy --dry-run` on a duplicate-domain manifest set exits non-zero, diagnostic mentions "routing collision" and both app refs.
└── testdata/deploy/
    └── routing-collision/       # NEW fixture: two Application manifests sharing one routing.domain.
        ├── team.yaml            # (or reuse existing team fixture)
        ├── app-a.yaml
        └── app-b.yaml
```

**Structure Decision**: Single-project Go layout (already established). The change is contained in two existing files (`internal/planner/plan.go`, `internal/handler/deploy.go`), one new unit test, one new integration scenario, and a tiny new fixture directory. No new packages, no new manifest kinds, no new exported APIs.

## Phase 0: Outline & Research

See [research.md](./research.md). All technical unknowns were resolvable from current source — no NEEDS CLARIFICATION markers were introduced.

## Phase 1: Design & Contracts

- **Data model**: [data-model.md](./data-model.md) — documents the `PlanResult` and `ManifestSet` entities touched by this change.
- **Contracts**: [contracts/cli-dry-run.md](./contracts/cli-dry-run.md) — describes the `shrine deploy --dry-run` exit-code and stderr behavior before vs. after the change.
- **Quickstart**: [quickstart.md](./quickstart.md) — concrete reproduction and verification steps for reviewers.
- **Agent context**: `CLAUDE.md` SPECKIT marker block updated to point at this plan.

## Complexity Tracking

> No violations. Section retained per template, intentionally empty.
