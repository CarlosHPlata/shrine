# Implementation Plan: Teardown Headers That Print and Teardown Event Names That Match the Rest of the Log

**Branch**: `028-fix-teardown-event-names` | **Date**: 2026-10-03 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/028-fix-teardown-event-names/spec.md` (GitHub issue #46)

## Summary

`engine.teardownKind` builds its three event names by concatenating the deployment kind exactly as state recorded it (`"Application"` / `"Resource"`), so a teardown emits `Application.teardown`, `Application.remove`, `Application.routing_remove` and their `Resource.*` counterparts. The terminal observer only matches `application.teardown` / `resource.teardown`, so the `🗑️  Tearing down …` headers never print, and `shrine.log` plus the `❌ Error [...]` lines carry the only capitalised event names in the product. Fix: one named local in `teardownKind` — `eventPrefix := strings.ToLower(kind)` — feeds the three names; the route-removal gate (`step.Kind == manifest.ApplicationKind`) and the error prose keep the recorded value, so behaviour, state, and error text are unchanged and no migration is needed. `internal/ui` is untouched: the renderer and file logger were already correct. Coverage: two new engine tests assert an exact interleaved timeline of events and backend calls (names, header-before-removal order, one name per event) and the three failure names with unchanged error text; the handler test that spec 027 pinned to the capitalised names flips to lowercase literals; three existing Docker scenarios gain an assertion each — the application header and the resource header on stdout, and `[started] application.teardown` in the log — authored red-first, compile-checked locally, executed by CI.

## Technical Context

**Language/Version**: Go 1.25 (`go.mod`: `go 1.25.0`; module `github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: stdlib `strings` (already imported in `internal/engine/engine.go`); Cobra unchanged; no new dependencies
**Storage**: N/A — no state or config schema change; `state.Deployment.Kind` stays `Application` / `Resource`; `shrine.log` grammar unchanged, only the name token of teardown entries changes
**Testing**: `go test ./...` for units (package policy: no filesystem — the new tests use in-memory fakes and a shared call timeline); integration via existing `tests/integration/teardown_test.go` and `file_logger_test.go` on `NewDockerSuite` with `-tags integration`, compile-checked locally (`go vet -tags integration ./tests/integration/...`) and executed by CI (`make test-integration`)
**Target Platform**: Linux server (Docker host)
**Project Type**: Single Go CLI
**Performance Goals**: N/A — one `strings.ToLower` per torn-down deployment
**Constraints**: Only event *names* change (FR-003–FR-006); recorded kind, teardown order, route-removal gating, error prose, and exit codes are unchanged (FR-007, FR-008); deploy/apply/status/dry-run output unchanged (FR-009); no Cobra `Long`/flag string changes, so generated `docs/content/cli/*.md` does not drift; no alias for the old names
**Scale/Scope**: 1 production file (`internal/engine/engine.go`, 1 line added + 3 modified), 2 unit-test files, 2 integration-test files (3 added assertions, 0 new scenarios), 3 project docs (`logging-observer.md`, `integration-tests.md`, `progress.md`), 3 spec-027 annotations

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | Pass — no new capability, flag, or manifest field; event naming only |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | N/A — no new command or mutation path; `cmd/teardown.go` is untouched and the new header goes to stdout through the existing observer |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | Pass — the change in `engine.go` is the spelling of three event names; no backend-specific logic enters the engine and no backend is modified |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | Pass — no new abstraction: one local variable; shared event-name constants, a helper function, and a compatibility alias were each considered and rejected (research D1, D3) |
| V. Integration-Test Gate | Does this phase map to an integration test phase in `specs/features/integration-tests.md` using `NewDockerSuite` against a real binary? | Pass — `TestTeardown`, `TestTeardownMultiTeam`, and `TestFileLogger` (all `NewDockerSuite`) gain the header and log assertions before the engine change (research D6); `integration-tests.md` updated |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | N/A — no state read or write changes; the recorded kind is read-only here and a container already gone still tears down as a soft success |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | Pass — `eventPrefix` names the lowercased value once instead of three inline conversions; the handler-test comment that described the defect as intended is deleted; no comment is added |

**Post-Phase-1 re-check**: all gates unchanged — Pass. No Complexity Tracking entries required.

## Project Structure

### Documentation (this feature)

```text
specs/028-fix-teardown-event-names/
├── plan.md              # This file
├── research.md          # Phase 0 — current-state findings + 9 decisions (where to lowercase, what stays capitalised, tests, docs)
├── data-model.md        # Phase 1 — kind vs. event prefix, before/after names, teardown event sequence, invariants
├── quickstart.md        # Phase 1 — manual verification per user story + automated gates + red-first check
├── contracts/
│   └── observer-events.md   # Phase 1 — renamed events, terminal and log surface, non-changes, compatibility
└── tasks.md             # Phase 2 (/speckit-tasks — NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/engine/
├── engine.go            # MODIFIED: teardownKind — eventPrefix := strings.ToLower(kind); the three names
│                        #           (.teardown, .remove, .routing_remove) are built from eventPrefix.
│                        #           step.Kind comparison and fmt.Errorf prose keep `kind` (engine.go:241-265)
└── engine_test.go       # MODIFIED: fakeRoutingBackend gains removeRouteErr; new timelineObserver;
                         #           TestEngine_ExecuteTeardown_AnnouncesEachDeploymentBeforeRemovingIt,
                         #           TestEngine_ExecuteTeardown_NamesFailureEventsInLowercase

internal/handler/
└── teardown_test.go     # MODIFIED: assertions switch to the literals "application.teardown",
                         #           "resource.teardown", "application.remove"; stale comment removed

internal/ui/             # UNCHANGED — terminal_logger.go already renders the lowercase names;
                         #             its tests already pin them (spec 027)
internal/planner/        # UNCHANGED — PlanTeardown keeps passing the recorded kind
internal/state/          # UNCHANGED — Deployment.Kind stays Application / Resource
cmd/                     # UNCHANGED

tests/integration/
├── teardown_test.go     # MODIFIED: TestTeardown asserts "Tearing down Application: whoami (team: <team>)" on stdout;
│                        #           TestTeardownMultiTeam (first scenario) asserts
│                        #           "Tearing down Resource: shared-cache (team: <team-a>)"
└── file_logger_test.go  # MODIFIED: after teardown, the log also holds
                         #           [started] application.teardown name="whoami" team="<team>"

specs/features/logging-observer.md       # MODIFIED: "Event Name Conventions" states the lowercase rule
specs/features/integration-tests.md      # MODIFIED: teardown + file-logger entries list the new assertions
specs/progress.md                        # MODIFIED: fix entry (issue #46); 027 entry points at spec 028
specs/027-app-ui-unit-coverage/
├── tasks.md                             # MODIFIED: Implementation Note 1 annotated "resolved by spec 028"
├── data-model.md                        # MODIFIED: §7 "Implementation correction" annotated likewise
└── contracts/coverage-matrix.md         # MODIFIED: rows B-1 / B-3 name the lowercase events
```

**Structure Decision**: Single-project Go CLI. The production change stays inside the one function that owns the defect — `engine.teardownKind`, the only place in the codebase that builds an event name from data instead of a literal (research, current-state findings). The renderer (`internal/ui`), the planner, and state are deliberately untouched: they were already correct, and changing what is recorded would force a migration the spec forbids (FR-008).

## Design Outline

1. **Integration assertions first** (Constitution V, research D6): extend the three existing teardown runs — `TestTeardown` (`AssertOutputContains` for the application header), `TestTeardownMultiTeam` first scenario (resource header), `TestFileLogger` (`AssertFileContains` for `[started] application.teardown name="whoami" team="<team>"`). Assertions match the ASCII part of the header. Compile-check with `go vet -tags integration ./tests/integration/...`; do not run locally — CI is the gate.
2. **Engine unit tests, red** (research D4): in `internal/engine/engine_test.go` add `removeRouteErr` to `fakeRoutingBackend`, a `timelineObserver` that appends `"Event:"+name` to the shared `calls` slice, and the two tests — the exact timeline `Event:application.teardown → RemoveContainer:team-x/web → Event:resource.teardown → RemoveContainer:team-x/db` (plus the empty-steps case), and the failure table (`application.remove`, `resource.remove`, `application.routing_remove`; every recorded name lowercase; returned error text unchanged).
3. **Handler test, red** (research D5): `internal/handler/teardown_test.go` asserts the three lowercase literals; the stand-in deployments keep the capitalised recorded kind, making this the unit proof of FR-008.
4. **The fix** (research D1, D2): in `teardownKind`, `eventPrefix := strings.ToLower(kind)` and use it for the three names. Nothing else in the function changes.
5. **Green**: `go test ./...` passes; the red tests from steps 2–3 turn green and no other test changes status; `go vet -tags integration ./tests/integration/...` is clean.
6. **Docs and bookkeeping** (research D8, FR-012): state the lowercase rule in `specs/features/logging-observer.md`; list the new assertions in `specs/features/integration-tests.md`; add the fix entry to `specs/progress.md` and repoint the 027 entry; annotate the three spec-027 records. Run `graphify update .` after the code change. The PR is titled `fix: …` and carries the one-line Constitution Check required for changes under `internal/engine/`.

## Complexity Tracking

No Constitution violations — table intentionally empty.
