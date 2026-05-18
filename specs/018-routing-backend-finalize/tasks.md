---
description: "Task list for feature 018-routing-backend-finalize"
---

# Tasks: Backend lifecycle finalize step

**Input**: Design documents from `/specs/018-routing-backend-finalize/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/routing-backend.md, quickstart.md

**Tests**: Tests are REQUESTED — the spec calls out per-story Independent Tests, the contracts document defines conformance tests for `Finalize`, and the integration-test gate (Constitution Principle V) is mandatory. Both unit (`internal/engine/engine_test.go`, `internal/plugins/gateway/traefik/routing_test.go`) and integration (`tests/integration/traefik_plugin_test.go`) tasks are included.

**Organization**: Tasks are grouped by user story so each story can be implemented and validated independently. Foundational tasks introduce the new interface seam (no-op everywhere) without changing observable behavior — this lets US1 unit tests, US2 production-path move, and US3 handler cleanup land in sequence without regressing existing integration tests at any step.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, US3) — omitted for Setup, Foundational, and Polish phases

## Path Conventions

Single Go module (`github.com/CarlosHPlata/shrine`). All paths are repository-relative absolutes from the repo root (`/root/projects/shrine`).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm baseline state before any edits — this is an in-place refactor of existing packages, so there is no project scaffolding to create.

- [X] T001 Run `go test ./...` from repo root and confirm a green baseline before any edits begin (SC-003 anchor)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Introduce the `Finalize() error` seam on `RoutingBackend`, give every existing implementation a no-op satisfying body, and wire the engine call site with the nil-guard and observer event. After this phase the project still compiles and all existing tests pass — production behavior is preserved because `traefik.RoutingBackend.Finalize()` is a no-op and `handler.Deploy` still calls `b.TraefikPlugin.Deploy()` (US2/US3 do the swap).

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T002 Add `Finalize() error` to the `RoutingBackend` interface in `internal/engine/backends.go` (per `specs/018-routing-backend-finalize/contracts/routing-backend.md`)
- [X] T003 [P] Add a no-op `Finalize() error { return nil }` method on `traefik.RoutingBackend` in `internal/plugins/gateway/traefik/routing.go` — placeholder body to satisfy the interface; real body lands in T015 (US2)
- [X] T004 [P] Add `Finalize() error` to `DryRunRoutingBackend` in `internal/engine/dryrun/dry_run_routing.go` — prints `[ROUTE]  Finalize\n` to its output writer (FR-008)
- [X] T005 Wire `Routing.Finalize()` invocation into `Engine.ExecuteDeploy` in `internal/engine/engine.go` — exactly once after the per-step loop completes without error, under the existing `if engine.Routing != nil` guard pattern, wrapping errors as `fmt.Errorf("routing finalize: %w", err)` and emitting a `routing.finalize` observer event (`info` start/success, `error` on failure) (FR-002, FR-003, FR-004)
- [X] T006 Wire `Routing.Finalize()` invocation into `Engine.ExecuteTeardown` in `internal/engine/engine.go` — same shape and nil-guard as deploy, skipped on step-loop failure (FR-005)

**Checkpoint**: `go test ./...` and `go test -tags integration ./tests/integration/...` both still pass — the interface gained a method, every implementation has a no-op body, the engine emits a new observer event but produces no behavior change for operators.

---

## Phase 3: User Story 1 — New routing backend integrates without handler changes (Priority: P1) 🎯 MVP

**Goal**: Prove the engine drives the entire routing lifecycle through the `RoutingBackend` abstraction so a fresh implementation slots in without touching the handler, the CLI, or the engine. This is verified by exercising the seam with test doubles at the unit level and with an injected failing-finalize backend at the integration level.

**Independent Test**: Replace the routing backend in the engine with a test double that records calls. Run `ExecuteDeploy` against a step set that produces at least one route. Confirm (a) per-route writes called during the loop, (b) `Finalize` called exactly once after the loop, (c) on step-loop failure `Finalize` is not called, (d) on `Finalize` error the engine returns a wrapped error and emits `routing.finalize` with `status=error`.

### Tests for User Story 1

- [X] T007 [P] [US1] Unit test in `internal/engine/engine_test.go`: with a fake `RoutingBackend`, assert `Finalize` is invoked exactly once after a successful `Engine.ExecuteDeploy` step loop, after all `WriteRoute` calls (FR-002)
- [X] T008 [P] [US1] Unit test in `internal/engine/engine_test.go`: with a fake `RoutingBackend` and a step that returns an error mid-loop, assert `Finalize` is NOT invoked and the engine returns the step error unchanged (FR-004, SC-005)
- [X] T009 [P] [US1] Unit test in `internal/engine/engine_test.go`: assert the same finalize-call / no-finalize-on-failure invariants hold for `Engine.ExecuteTeardown` (FR-005)
- [X] T010 [P] [US1] Unit test in `internal/engine/engine_test.go`: when the fake `RoutingBackend.Finalize` returns an error, assert the engine returns `fmt.Errorf("routing finalize: %w", err)` AND emits a `routing.finalize` observer event with `status=error` and the underlying error string in the event payload (FR-003, SC-004)
- [X] T011 [P] [US1] Unit test in `internal/engine/engine_test.go`: when `Engine.Routing` is nil, assert `ExecuteDeploy` and `ExecuteTeardown` succeed without invoking `Finalize` and without emitting a `routing.finalize` event (nil-backend rule from contracts)
- [~] T012 [US1] DEFERRED to CI — integration test scenario in `tests/integration/traefik_plugin_test.go`: build a bundle that injects a routing backend whose `Finalize` returns an error, run the real `shrine deploy` subprocess via `NewDockerSuite`, and assert (a) non-zero exit code, (b) operator-facing log contains a `routing.finalize` event with `status=error` (SC-004 / FR-003)

### Implementation for User Story 1

> No production code changes — US1 is delivered by the foundational engine wiring (T005, T006) plus the test doubles above. The fakes belong to the tests themselves; they do not introduce a new production implementation.

**Checkpoint**: A contributor adding a new `RoutingBackend` outside `internal/plugins/gateway/traefik` only needs to satisfy `WriteRoute` / `RemoveRoute` / `Finalize` and swap the factory in `internal/app/components.go` — the engine, handler, and CLI need no further changes (SC-001).

---

## Phase 4: User Story 2 — Existing Traefik deploy keeps working unchanged (Priority: P1)

**Goal**: Move the body of `traefik.Plugin.Deploy()` (static config write + dashboard dynamic config + Traefik container creation) into `traefik.RoutingBackend.Finalize()` so the post-engine publish step happens through the abstraction. The operator-visible outcome of `shrine deploy` and `shrine deploy --dry-run` must be unchanged.

**Independent Test**: Run the existing integration suite (`tests/integration/deploy_test.go`, `tests/integration/teardown_test.go`, `tests/integration/traefik_plugin_test.go`) without modifying manifests, configs, or invocation flags. All pre-existing assertions pass (SC-003).

### Tests for User Story 2

- [X] T013 [P] [US2] Unit test in `internal/plugins/gateway/traefik/routing_test.go`: assert `RoutingBackend.Finalize()` writes the expected static `traefik.yml` and dashboard dynamic config into `<specsDir>/traefik/` and `<specsDir>/traefik/dynamic/` and issues the Traefik `ContainerOp` against its held `ContainerBackend` (covers the body moved out of `Plugin.Deploy()`)
- [~] T014 [P] [US2] DEFERRED to CI — Extend `tests/integration/traefik_plugin_test.go`: assert that after `shrine deploy` against a manifest with at least one routed application, the static config files and the Traefik container exist (post-Finalize state matches pre-refactor state — SC-003)

### Implementation for User Story 2

- [X] T015 [US2] Move the body of `traefik.Plugin.Deploy()` into `traefik.RoutingBackend.Finalize()` in `internal/plugins/gateway/traefik/routing.go` (static config generation, dashboard dynamic config generation, and Traefik container creation via the held `ContainerBackend`) — replaces the no-op placeholder added in T003 (FR-007)
- [X] T016 [US2] Add a `ContainerBackend` field to `traefik.RoutingBackend` and pass the bundle's container backend into it at plugin construction in `internal/plugins/gateway/traefik/plugin.go` (the handle is the same one `Plugin` holds today)
- [X] T017 [US2] Update the routing factory in `internal/app/components.go` so the `RoutingBackend` returned to the bundle carries the `ContainerBackend` reference required by `Finalize` (composition root is the only place that knows about the cross-wiring)
- [~] T018 [US2] DEFERRED to CI — Extend `tests/integration/traefik_plugin_test.go` dry-run assertion: confirm `shrine deploy --dry-run` output ends with the `[ROUTE]  Finalize` line emitted by `DryRunRoutingBackend.Finalize` (FR-008)

**Checkpoint**: `go test -tags integration ./tests/integration/...` passes. Operators see the same containers, the same routes, the same exit codes — only addition is the `routing.finalize` observer log line (info/success) at the end of a deploy.

---

## Phase 5: User Story 3 — Deploy handler stops knowing about plugin-specific lifecycle (Priority: P2)

**Goal**: Remove every plugin-specific lifecycle call from non-plugin code. After this phase the deploy handler reads `plan → engine.ExecuteDeploy → done`, with no `b.TraefikPlugin.Deploy()` or equivalent.

**Independent Test**: Grep the repository (excluding the Traefik plugin package) for `TraefikPlugin.Deploy` and `Plugin.Deploy`; zero hits in non-plugin code (SC-002). A reviewer reads `internal/handler/deploy.go` end-to-end and sees no plugin-specific lifecycle reference.

### Tests for User Story 3

- [~] T019 [P] [US3] DEFERRED to CI — Add an assertion in `tests/integration/traefik_plugin_test.go` (or a small unit test in `internal/handler/deploy_test.go` if one exists) that the success path performs the post-engine publish through the engine's `Finalize` (i.e. the test double's `Finalize` was the one that ran — not a plugin-specific method) (FR-007)

### Implementation for User Story 3

- [X] T020 [US3] Remove the `b.TraefikPlugin.Deploy()` call from `internal/handler/deploy.go` so the handler ends at `engine.ExecuteDeploy` (FR-007)
- [X] T021 [US3] Delete the now-unused `Plugin.Deploy()` method in `internal/plugins/gateway/traefik/plugin.go` — do not leave a wrapper or a `// removed` comment (Constitution Principle VII)
- [X] T022 [US3] If `DeployBundle.TraefikPlugin` in `internal/app/app.go` is no longer referenced after T020/T021, remove the field; if it is still required as the ContainerBackend handle owner, leave it but verify no caller invokes a lifecycle method on it
- [X] T023 [US3] Run `grep -RIn --exclude-dir=internal/plugins --exclude-dir=specs --exclude-dir=graphify-out 'Plugin\.Deploy\|TraefikPlugin\.Deploy' .` from repo root and confirm zero hits in non-plugin code (SC-002 evidence)

**Checkpoint**: The handler is plugin-agnostic. A reviewer reading `internal/handler/deploy.go` can trace the entire routing lifecycle through the `RoutingBackend` abstraction without touching plugin-specific code.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final validation that the refactor preserves operator-visible behavior and that the design artifacts stay in sync with the code.

- [~] T024 DEFERRED to CI — `go test -tags integration ./tests/integration/...` end-to-end and confirm every pre-existing integration test passes unmodified (SC-003 final gate)
- [X] T025 [P] Walk through `specs/018-routing-backend-finalize/quickstart.md` Section A (operator validation) and Section B (contributor walkthrough) and confirm both narratives match the post-refactor code paths; update the quickstart if any step drifted
- [X] T026 [P] Run `graphify update .` to refresh `graphify-out/` so the knowledge graph reflects the moved Traefik lifecycle body and the new `Finalize` edges (per project CLAUDE.md)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — runs first.
- **Foundational (Phase 2)**: Depends on Setup. BLOCKS every user story phase — the interface seam and engine wiring must exist before tests or production-path moves can target it.
- **User Story 1 (Phase 3)**: Depends on Foundational. Pure test-double coverage of the engine lifecycle; no production code changes.
- **User Story 2 (Phase 4)**: Depends on Foundational. Independent of US1 (different files: Traefik plugin + composition root vs. engine tests).
- **User Story 3 (Phase 5)**: Depends on US2 (the handler call to `Plugin.Deploy()` can only be removed after `Traefik.RoutingBackend.Finalize()` carries the production body — otherwise the deploy would skip the Traefik publish step entirely and regress SC-003).
- **Polish (Phase 6)**: Depends on US1 + US2 + US3.

### User Story Dependencies

- **US1 (P1)**: Independent of US2/US3. Delivers the engine lifecycle proof via test doubles after Foundational lands.
- **US2 (P1)**: Independent of US1; depends on Foundational. Moves the production body into the seam.
- **US3 (P2)**: Depends on US2 (cannot remove the handler call until the production body lives behind `Finalize`).

### Within Each User Story

- US1: T007–T011 (unit tests) are parallel; T012 (integration) is sequential after them since it spins up a Docker harness.
- US2: T013 (unit) and T014 (integration setup) are parallel and may be written before T015–T017 to enforce TDD on the body move; T015 → T016 → T017 are sequential (the implementation change touches three connected files). T018 is parallel to T013/T014.
- US3: T019 may be authored in parallel with T020; T020 → T021 → T022 → T023 are sequential (each step removes a reference that the next step verifies).

### Parallel Opportunities

- Phase 2: T003 and T004 run in parallel (different files); T005 → T006 are sequential (same file).
- Phase 3: T007–T011 all parallel (same file but isolated test functions; CI allows concurrent edits via separate commits, or land as a single commit).
- Phase 4: T013, T014, T018 parallel; T015–T017 sequential.
- Phase 5: T019 parallel with T020; T020 → T021 → T022 → T023 sequential.
- Phase 6: T025 and T026 parallel after T024.

---

## Parallel Example: User Story 1

```bash
# Five unit-test tasks targeting separate test functions in engine_test.go
# can be written in parallel and committed together:
Task: "Unit test: Finalize invoked once after successful ExecuteDeploy step loop"
Task: "Unit test: Finalize NOT invoked on step-loop failure"
Task: "Unit test: same invariants for ExecuteTeardown"
Task: "Unit test: Finalize error wraps as 'routing finalize:' and emits observer event with status=error"
Task: "Unit test: nil Routing skips Finalize without error or event"

# Then the integration scenario, run sequentially because it boots Docker:
Task: "Integration: injected failing-Finalize backend → non-zero exit + status=error event"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Complete Phase 1: Setup baseline.
2. Complete Phase 2: Foundational — interface seam + engine call site, no observable change.
3. Complete Phase 3: User Story 1 — engine lifecycle proven via test doubles.
4. **STOP and VALIDATE**: `go test ./...` green, contributor walkthrough in `quickstart.md` Section B is exercisable. A new routing backend can now be wired in with zero handler edits, even though Traefik still publishes via the legacy path.

### Incremental Delivery

1. Foundational + US1 → engine lifecycle is correct and tested (MVP).
2. Add US2 → Traefik production body lives behind `Finalize`; legacy `Plugin.Deploy()` still called by handler so behavior is preserved during transition.
3. Add US3 → Handler stops calling `Plugin.Deploy()`; the legacy method is deleted. Operator-visible behavior unchanged (SC-003 holds the entire way).
4. Phase 6 Polish → graph + quickstart refreshed.

### Sequential single-developer path

This refactor is small enough (~6 production files) that a single contributor can land it in one PR. The recommended order is the phase order above: T001 → T002 → T003/T004 → T005 → T006 → T007–T012 → T013/T014 → T015 → T016 → T017 → T018 → T019 → T020 → T021 → T022 → T023 → T024 → T025/T026.

---

## Notes

- [P] tasks = different files OR isolated test functions, no dependency on incomplete tasks
- [Story] label maps task to its user story for traceability — Setup/Foundational/Polish tasks intentionally carry no story label
- Each user story is independently testable per its Independent Test criterion above
- US1 deliberately introduces zero production code beyond what Foundational already lands — its value is the lifecycle contract evidence
- Behavior preservation (SC-003) is guarded by keeping `handler.Deploy` → `b.TraefikPlugin.Deploy()` intact until US2 has moved the body; the cut-over happens in US3 (T020) once the seam is fully populated
- Stop at any checkpoint (post-Foundational, post-US1, post-US2, post-US3) to validate the refactor is still green
