# Tasks: Remove Stale Dashboard Config on Dashboard Removal

**Input**: Design documents from `/specs/024-fix-dashboard-removal/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/observer-events.md, quickstart.md

**Tests**: Included — Constitution Principle V mandates TDD (unit + integration tests authored red-first, before implementation). Per team practice, integration tests are authored and compile-checked locally; the CI pipeline executes them as the gate.

**Organization**: Tasks are grouped by user story. US1 (removal behavior) and US2 (reporting) share one code path — the removal helper — so US2's implementation tasks extend the helper US1 introduces; US2 remains independently *testable* through its own event/error assertions.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 (P1 removal), US2 (P2 reporting), US3 (P3 test repointing + dead code)

## Phase 1: Setup

**Purpose**: Confirm a green baseline before touching anything — this is a bug fix on existing code, no project scaffolding needed.

- [X] T001 Verify green baseline: run `go build ./...` and `go test ./internal/plugins/gateway/traefik/` at repository root; both must pass before any change

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared test seam used by US1 and US2 unit tests.

- [X] T002 Add a recording stub for the remove seam in `internal/plugins/gateway/traefik/routing_test.go`: `recordRemoveFileFn(t *testing.T) *[]string` — swaps `removeFileFn` for a function that appends each path to the returned slice, restores on `t.Cleanup` (same pattern as `captureWriteFileFn`); add an error-injecting variant `stubRemoveFileError(t *testing.T, err error)`. Leave the existing fail-fast `captureRemoveFileFn` untouched — the `RemoveRoute` orphan-warn policy still uses it

**Checkpoint**: Test seams in place — story phases can begin.

---

## Phase 3: User Story 1 - Removing the dashboard config stops the dashboard (Priority: P1) 🎯 MVP

**Goal**: A deploy with no dashboard configured deletes a stale `__shrine-dashboard.yml`; deploys with nothing to remove stay silent no-ops; per-app route files are never touched.

**Independent Test**: `go test ./internal/plugins/gateway/traefik/` (removal-branch tests) + the new integration scenario: deploy with dashboard → drop the `dashboard:` block → redeploy → file gone, app routes intact.

### Tests for User Story 1 (write FIRST — must FAIL before T008–T009 land) ⚠️

- [X] T003 [US1] Unit test in `internal/plugins/gateway/traefik/routing_test.go`: `TestRoutingBackend_Finalize_NoDashboard_RemovesStaleFile` — `stubLstatPresent`, `recordRemoveFileFn`, cfg without `Dashboard`; assert `Finalize()` returns nil, `removeFileFn` was called exactly once with `<routingDir>/dynamic/__shrine-dashboard.yml`, and the Traefik container is still created (recordingContainerBackend sees 1 create)
- [X] T004 [US1] Unit test in `internal/plugins/gateway/traefik/routing_test.go`: `TestRoutingBackend_Finalize_NoDashboard_AbsentFileIsNoOp` — `stubLstatNotExist`, `recordRemoveFileFn`, cfg without `Dashboard`; assert nil error and zero remove calls (FR-004)
- [X] T005 [US1] Unit test in `internal/plugins/gateway/traefik/routing_test.go`: `TestRoutingBackend_Finalize_WithDashboard_DoesNotRemove` — companion to `TestRoutingBackend_Finalize_GeneratesConfigsAndCreatesContainer`: `stubLstatPresent`, `captureRemoveFileFn` (fail-fast), cfg with credentialed `Dashboard`; assert the generation path still runs (`gateway.dashboard.preserved` emitted) and no removal is attempted
- [X] T006 [P] [US1] Integration test in `tests/integration/traefik_plugin_test.go`: scenario `"should remove stale dashboard dynamic file when dashboard config is removed"` — `writeConfig` with a credentialed `dashboard:` block, `tc.Run("deploy", …).AssertSuccess()`, `tc.AssertFileExists` on `<routingDir>/dynamic/__shrine-dashboard.yml`; then `writeConfig` again *without* the `dashboard:` block, redeploy, `tc.AssertFileNotExists` on the dashboard file and `tc.AssertFileExists` on the per-app route file (untouched, FR-005). Follow the harness pattern at `traefik_plugin_test.go:1100`
- [X] T007 [US1] Verify red: `go test ./internal/plugins/gateway/traefik/` fails on T003–T004 (T005 passes — it characterizes existing behavior); `go vet -tags integration ./tests/integration/...` compiles clean

### Implementation for User Story 1

- [X] T008 [US1] Implement `removeStaleDashboardDynamicConfig(routingDir string, observer engine.Observer) error` in `internal/plugins/gateway/traefik/config_gen.go`, directly below `generateDashboardDynamicConfig`: build path from `dashboardDynamicFileName()`, probe with `isPathPresent`, return nil silently when absent, delete via `removeFileFn` when present (research Decision 1; event emission and error wrapping are refined in US2 — stub them minimally here so T003–T004 pass)
- [X] T009 [US1] Wire the removal branch in `internal/plugins/gateway/traefik/routing.go` `Finalize()` (currently line 219): `if r.hasDashboard() { generate… } else { if err := removeStaleDashboardDynamicConfig(r.routingDir, r.observer); err != nil { return err } }` — before container creation so the recreated container also drops the dashboard port binding
- [X] T010 [US1] Verify green: `go test ./internal/plugins/gateway/traefik/` passes; `go vet -tags integration ./tests/integration/...` compiles clean

**Checkpoint**: The stale-file bug is fixed and independently testable — MVP complete (CI executes T006 as the gate).

---

## Phase 4: User Story 2 - The removal is visible in deploy output (Priority: P2)

**Goal**: Successful removal emits `gateway.dashboard.removed` (StatusInfo, `path` field); nothing-to-remove deploys emit no event; stat/remove failures fail the deploy with a wrapped error naming path and cause.

**Independent Test**: Event and error assertions in `go test ./internal/plugins/gateway/traefik/` plus the deploy-output assertion in the integration scenario.

### Tests for User Story 2 (write FIRST — must FAIL before T017 lands) ⚠️

- [X] T011 [US2] Unit test in `internal/plugins/gateway/traefik/routing_test.go`: `TestRoutingBackend_Finalize_NoDashboard_EmitsRemovedEvent` — `stubLstatPresent`, `recordRemoveFileFn`, `recordingObserver`; assert exactly one `gateway.dashboard.removed` event with `engine.StatusInfo` and `Fields["path"]` = the dashboard file path, emitted only after removal succeeds (contracts/observer-events.md)
- [X] T012 [US2] Unit test in `internal/plugins/gateway/traefik/routing_test.go`: `TestRoutingBackend_Finalize_NoDashboard_AbsentFileEmitsNoEvent` — `stubLstatNotExist`; assert zero `gateway.dashboard.*` events (FR-004/SC-006)
- [X] T013 [US2] Unit test in `internal/plugins/gateway/traefik/routing_test.go`: `TestRoutingBackend_Finalize_NoDashboard_StatErrorFailsDeploy` — `stubLstatError(t, errors.New("permission denied"))`; assert `Finalize()` returns an error containing the dashboard file path and `"permission denied"`, and no event is emitted (research Decision 3)
- [X] T014 [US2] Unit test in `internal/plugins/gateway/traefik/routing_test.go`: `TestRoutingBackend_Finalize_NoDashboard_RemoveErrorFailsDeploy` — `stubLstatPresent` + `stubRemoveFileError(t, errors.New("device busy"))`; assert wrapped error contains the path and `"device busy"`, and no `gateway.dashboard.removed` event is emitted (FR-006)
- [X] T015 [US2] Extend the T006 integration scenario in `tests/integration/traefik_plugin_test.go`: assert the second deploy's output reports the removal (stdout/stderr contains the `gateway.dashboard.removed` rendering with the file path), and that a third deploy with no dashboard produces no removal output (SC-003/SC-006)
- [X] T016 [US2] Verify red: T011, T013, T014 fail (T012 may already pass from the T008 minimal helper); integration compile-check clean

### Implementation for User Story 2

- [X] T017 [US2] Complete `removeStaleDashboardDynamicConfig` in `internal/plugins/gateway/traefik/config_gen.go`: wrap the `isPathPresent` error and the `removeFileFn` error as `fmt.Errorf("traefik plugin: removing stale dashboard dynamic file at %q: %w", path, err)`; on success emit `gateway.dashboard.removed` with `engine.StatusInfo` and `Fields: map[string]string{"path": path}` — matching the write-then-emit convention of `gateway.dashboard.generated`
- [X] T018 [US2] Verify green: `go test ./internal/plugins/gateway/traefik/` passes; `go vet -tags integration ./tests/integration/...` compiles clean

**Checkpoint**: Removal is observable and failure-loud; US1 + US2 fully deliver spec 010's FR-008 promise.

---

## Phase 5: User Story 3 - Port-binding tests protect the shipped behavior (Priority: P3)

**Goal**: Exactly one port-binding implementation remains (the live `RoutingBackend.portBindings()`), covered by tests for all three combinations (gateway port, TLS port, dashboard port); dead code is gone.

**Independent Test**: `go test ./internal/plugins/gateway/traefik/` passes; `grep -rn "func (p \*Plugin) portBindings" internal/` returns nothing; a deliberate fault injected into `RoutingBackend.portBindings()` fails at least one unit test.

### Tests for User Story 3 (characterization of live code — green on arrival by design; they must land BEFORE the deletions so coverage never drops)

- [X] T019 [US3] Add `TestRoutingBackend_PortBindings_OmitsTLS_WhenTLSPortUnset` and `TestRoutingBackend_PortBindings_IncludesTLS443_WhenTLSPortSet` to `internal/plugins/gateway/traefik/routing_test.go`, constructing `&RoutingBackend{cfg: &cfg}` instead of `&Plugin{cfg: &cfg}`; carry over the assertions from `plugin_test.go:117,142` unchanged (FR-009)
- [X] T020 [US3] Add `TestRoutingBackend_PortBindings_IncludesDashboardPort_WhenDashboardSet` to `internal/plugins/gateway/traefik/routing_test.go`: cfg with Port 80 + credentialed Dashboard on 8080 → expect bindings `80:80/tcp` and `8080:8080/tcp`; makes the third FR-009 combination explicit (today only implicit in the Finalize test's `len == 3`)

### Implementation for User Story 3

- [X] T021 [US3] Delete the dead `Plugin.portBindings()` from `internal/plugins/gateway/traefik/plugin.go` (lines 128-142) and drop the now-unused `strconv` import if nothing else uses it — same commit as T022 (deleting one without the other breaks the build)
- [X] T022 [US3] Delete `TestPlugin_PortBindings_OmitsTLS_WhenTLSPortUnset` and `TestPlugin_PortBindings_IncludesTLS443_WhenTLSPortSet` from `internal/plugins/gateway/traefik/plugin_test.go` — same commit as T021
- [X] T023 [US3] Verify cleanup: `go build ./...` and `go test ./internal/plugins/gateway/traefik/` pass; `grep -rn "func (p \*Plugin) portBindings" internal/` returns nothing; `grep -rn "removeFileFn" internal/plugins/gateway/traefik/*.go | grep -v _test` shows the declaration plus exactly one production call site (FR-008/FR-010)

**Checkpoint**: All three stories complete; the test suite guards the shipped port-binding path.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T024 Run the full unit suite at repository root: `go test ./...` — zero failures, zero new warnings (SC-006)
- [X] T025 [P] Update `specs/progress.md`: add the 024 feature entry with its acceptance criteria and gate status per the Development Workflow
- [X] T026 [P] Run `graphify update .` to refresh the knowledge graph after the code changes (project rule, AST-only)
- [ ] T027 Push the branch and confirm the CI integration pipeline is green (executes the T006/T015 scenario — the Constitution V gate); optionally walk `quickstart.md` against a live Docker host

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: after T001 — blocks US1 and US2 test authoring (both use the new stubs)
- **US1 (Phase 3)**: after Phase 2
- **US2 (Phase 4)**: after US1's T008 exists (T017 extends the same helper); US2 *tests* (T011–T014) can be drafted any time after Phase 2
- **US3 (Phase 5)**: independent of US1/US2 — only needs Phase 1; touches `routing_test.go` too, so coordinate merge order if run concurrently
- **Polish (Phase 6)**: after all story phases

### Story Completion Order

```text
T001 → T002 → US1 (T003…T010) → US2 (T011…T018) → Polish
                     US3 (T019…T023) ──────────────┘   (any time after T001; file-coordination with US1/US2 in routing_test.go)
```

### Parallel Opportunities

- **T006 [P]** (integration scenario, `tests/integration/`) can be authored while T003–T005 are written in `routing_test.go` — different files, different authors
- **T021 + T022** are one atomic change across two files (single commit)
- **T025 and T026 [P]** touch disjoint files (`specs/progress.md` vs `graphify-out/`)
- Unit-test tasks within one phase share `routing_test.go` and are therefore sequential for a single author — different test functions can be drafted concurrently but must land in order

## Parallel Example: User Story 1

```bash
# Two authors, different files, at the same time:
Task: "T003–T005: removal-branch unit tests in internal/plugins/gateway/traefik/routing_test.go"
Task: "T006: remove-dashboard-redeploy scenario in tests/integration/traefik_plugin_test.go"
```

## Implementation Strategy

### MVP First (US1 only)

1. T001 → T002, then US1 red tests (T003–T007), then implementation (T008–T010).
2. **STOP and VALIDATE**: unit suite green, integration scenario compiles; CI executes it.
3. This alone fixes the security-relevant bug — the stale file is deleted on redeploy.

### Incremental Delivery

1. US1 → deployable fix (file removed, dashboard stops).
2. US2 → operator-visible confirmation + loud failures (completes spec 010's FR-008 contract, `gateway.dashboard.removed`).
3. US3 → net-negative cleanup; ship any time, ideally the same PR to close issue #35 in one change.

### Format validation

All 27 tasks use the checklist format (`- [ ] Txxx [P?] [Story?] description + exact file path`); story labels appear only in Phases 3–5.
