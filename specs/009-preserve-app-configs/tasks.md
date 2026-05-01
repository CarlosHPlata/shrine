# Tasks: Preserve Operator-Edited Per-App Routing Files

**Input**: Design documents from `/specs/009-preserve-app-configs/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/log-events.md, quickstart.md

**Tests**: TDD is required for the integration scenario (Constitution V — integration test files are created before the implementation code). Unit tests are added alongside or before the implementation they exercise. Test tasks are explicitly listed below.

**Organization**: Tasks are grouped by user story so each P1/P2 story can be implemented and verified independently. FR-007 (stat-error handling) folds into US1's deploy-axis work. FR-009 (orphan-warn on teardown) is broader than any single user story in spec.md and lives in a separate cross-cutting phase.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, US3) — Setup/Foundational/Cross-cutting/Polish phases have no story label
- All file paths are absolute relative to the repo root; this is a single-project Go module.

## Path Conventions

- Source: `internal/...`, `cmd/...` at repo root
- Tests (unit): `internal/<pkg>/*_test.go` alongside the production code
- Tests (integration): `tests/integration/*_test.go` (separate package; uses `NewDockerSuite` against a real binary)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: This is an extension of an existing Go module. There is no project initialization — module, dependencies, lint, and CI are already in place. The single setup task confirms the working tree is on the feature branch and the existing test suites are green before any change is made.

- [X] T001 Confirm working tree is on branch `009-preserve-app-configs` and that `go test ./internal/plugins/gateway/traefik/...` and `go test ./internal/engine/...` both pass with zero failures on `main` (baseline for non-regression assertions in later phases)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Three small structural changes that every user story phase depends on — the existence-check helper extraction, the observer wiring on `RoutingBackend`, and the shared `recordingObserver` test fixture. After this phase, `go test ./internal/...` must still pass byte-for-byte (no behavior change yet — only restructuring).

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T002 [P] Extract `isPathPresent(path string) (bool, error)` from the body of `isStaticConfigPresent` in `internal/plugins/gateway/traefik/config_gen.go`; rewrite `isStaticConfigPresent(routingDir string)` as a one-liner that calls `isPathPresent(filepath.Join(routingDir, "traefik.yml"))`; preserve the package-level `lstatFn` test seam unchanged; verify `go test ./internal/plugins/gateway/traefik/...` still passes (existing `gateway.config.preserved` / `gateway.config.generated` assertions in `config_gen_test.go` must continue to pass)
- [X] T003 [P] Add `observer engine.Observer` field to the `RoutingBackend` struct in `internal/plugins/gateway/traefik/routing.go`; update `Plugin.RoutingBackend()` in `internal/plugins/gateway/traefik/plugin.go` to construct `&RoutingBackend{routingDir: routingDir, observer: p.observer}`; do NOT yet emit any events from `WriteRoute` or `RemoveRoute` — that comes in US1 / Cross-Cutting phases
- [X] T004 [P] Create `internal/plugins/gateway/traefik/helpers_test.go` containing the shared `recordingObserver` test type (move the existing definition from `internal/plugins/gateway/traefik/config_gen_test.go:16-19` into the new file so both `config_gen_test.go` and `routing_test.go` can use the same type without redefinition); update `config_gen_test.go` to remove the local definition; update `newTestBackend()` in `internal/plugins/gateway/traefik/routing_test.go:37-39` to default `observer` to `engine.NoopObserver{}` so existing tests do not panic on event emission added in later phases

**Checkpoint**: After T002–T004, `go test ./internal/...` passes with zero new test additions and zero behavior change. The codebase now has the observer plumbing and the shared helper required by every subsequent task.

---

## Phase 3: User Story 1 - Operator-Edited Per-App Routing File Survives Re-Deploys (Priority: P1) 🎯 MVP

**Goal**: After this phase, `WriteRoute` writes the per-app file only when the path is Absent; if the file is present, Shrine leaves it untouched and emits `gateway.route.preserved`; if `Lstat` fails for any reason other than `IsNotExist`, Shrine emits `gateway.route.stat_error` (warning) and returns nil so the deploy continues. This delivers the spec's core defect-fix and FR-001/003/006/007/008/010/011 on the deploy axis.

**Independent Test**: Deploy an app via `shrine deploy`, hand-edit `<routingDir>/dynamic/<team>-<service>.yml` to add a sentinel comment, run `shrine deploy` again, verify the file's bytes are byte-for-byte identical to the edited version and the deploy log shows `gateway.route.preserved` for that app. Codified as the integration scenario in T005.

### Tests for User Story 1 (TDD — written and FAILING before T009 implementation)

- [X] T005 [P] [US1] Add integration scenario `TestTraefikPlugin_PerAppFile_PreservedAcrossRedeploys` in `tests/integration/traefik_plugin_test.go`: use `NewDockerSuite` per the existing pattern in this file; deploy a single app with a routing.Domain set; capture the bytes of `<routingDir>/dynamic/<team>-<service>.yml`; append a sentinel comment line (`# operator sentinel`) to the file; run `shrine deploy` a second time; assert the file's bytes equal `original + sentinel` byte-for-byte (use `os.ReadFile` and `bytes.Equal`); use the suite's existing cleanup (`BeforeEach`/`AfterEach`) — do NOT add new cleanup code; this is the ONLY new integration scenario in this feature (per [memory: integration tests are slow ~10 min])
- [X] T006 [P] [US1] Add unit test `TestWriteRoute_FilePresent_Preserves` in `internal/plugins/gateway/traefik/routing_test.go`: uses `lstatFn` fake returning a non-nil `os.FileInfo` (file present); constructs `&RoutingBackend{routingDir: "/fake", observer: rec}` with `rec` a `recordingObserver`; calls `WriteRoute(baseOp())`; asserts the captured `writeFileFn` was NOT called (no write happened); asserts `rec.events` contains exactly one event with `Name == "gateway.route.preserved"`, `Status == engine.StatusInfo`, and fields `team`, `name`, `path` set per the contract in `contracts/log-events.md`
- [X] T007 [US1] Add unit test `TestWriteRoute_FreshWrite_EmitsGenerated` in `internal/plugins/gateway/traefik/routing_test.go`: uses `lstatFn` fake returning `os.ErrNotExist` (file absent); calls `WriteRoute(baseOp())`; asserts `writeFileFn` WAS called with the expected YAML bytes (reuse the existing TestWriteRoute_NoAliases YAML-content assertion as the baseline); asserts `rec.events` contains exactly one event with `Name == "gateway.route.generated"`, `Status == engine.StatusInfo` (sequential after T006 — same file)
- [X] T008 [US1] Add unit test `TestWriteRoute_StatError_EmitsWarningAndContinues` in `internal/plugins/gateway/traefik/routing_test.go`: uses `lstatFn` fake returning a permission-denied-style error (e.g., `errors.New("permission denied")`); calls `WriteRoute(baseOp())`; asserts the function returns `nil` (deploy continues — FR-007 / FR-011); asserts `writeFileFn` was NOT called; asserts `rec.events` contains exactly one event with `Name == "gateway.route.stat_error"`, `Status == engine.StatusWarning`, and an `error` field equal to the underlying error string

### Implementation for User Story 1

- [X] T009 [US1] Modify `WriteRoute` in `internal/plugins/gateway/traefik/routing.go`: after the `mkdirAllFn(r.dynamicDir(), 0o755)` call and before the YAML marshal, compute `path := filepath.Join(r.dynamicDir(), routeFileName(op.Team, op.ServiceName))`; call `present, err := isPathPresent(path)`; if `err != nil`, emit `gateway.route.stat_error` (Status=Warning, fields `team`, `name`, `path`, `error`) via `r.observer.OnEvent` and return nil; if `present`, emit `gateway.route.preserved` (Status=Info, fields `team`, `name`, `path`) and return nil; otherwise proceed with the existing YAML-marshal-and-write path; after the existing `writeFileFn` call succeeds, emit `gateway.route.generated` (Status=Info, fields `team`, `name`, `path`) before returning nil; preserve all existing error-return paths (mkdir failure, marshal failure, write failure on fresh write) byte-for-byte
- [X] T010 [US1] Update existing routing_test.go cases (`TestWriteRoute_NoAliases`, `TestWriteRoute_OneAlias_Strip`, `TestWriteRoute_OneAlias_NoStrip`, `TestWriteRoute_HostOnlyAlias`, `TestWriteRoute_ThreeAliases_SparseStrip` — all in `internal/plugins/gateway/traefik/routing_test.go`) to fake `lstatFn` to return `os.ErrNotExist` (file absent) so they continue to exercise the fresh-write path; verify their existing YAML-content assertions still pass byte-for-byte; do NOT add new event assertions to these cases (T007 owns event coverage on the fresh-write path)

**Checkpoint**: After T005–T010, `go test ./internal/plugins/gateway/traefik/...` passes (including all new and existing unit tests); `go test -tags integration ./tests/integration/... -run TestTraefikPlugin_PerAppFile_PreservedAcrossRedeploys` passes against a real Docker daemon. User Story 1 is independently demonstrable via the integration test or via the quickstart.md walkthrough sections 1–4.

---

## Phase 4: User Story 2 - First Deploy of an App Still Bootstraps a Working Route (Priority: P1)

**Goal**: After Phase 3, the existing first-deploy path still works (FR-002 / FR-004 / SC-002). This phase is verification-only — the implementation behavior is delivered by T009/T010, but US2's acceptance scenarios are independently checkable.

**Independent Test**: On a host with no per-app file for `<team>/<service>`, run `shrine deploy` for an app declared in the manifest; verify `<routingDir>/dynamic/<team>-<service>.yml` is created, contains the expected YAML, and the deploy log shows `gateway.route.generated` (Info) for that app. Already exercised by T007 (unit) and the first half of T005 (integration). This phase adds no new code — it confirms US2 is satisfied by the same code paths US1 introduced.

### Tests for User Story 2

(No new tests — coverage is already provided by T007 (`TestWriteRoute_FreshWrite_EmitsGenerated`) and the existing routing_test.go cases updated in T010, plus the first deploy iteration of T005's integration scenario.)

### Implementation for User Story 2

(No new implementation — covered by T009.)

### Verification for User Story 2

- [X] T011 [US2] Run `go test ./internal/plugins/gateway/traefik/... -run TestWriteRoute_NoAliases` and confirm the fresh-write path produces the same YAML byte-for-byte as on `main` (no regression vs. baseline captured in T001); spot-check one of the other updated cases (e.g., `TestWriteRoute_ThreeAliases_SparseStrip`) the same way
- [X] T012 [US2] Run the integration scenario from T005 isolated to its first deploy phase (assert the file is created from manifest with expected content) — this is implicit in the full T005 scenario but should be confirmed as part of US2's independent acceptance

**Checkpoint**: After T011–T012, US2 is verified to be unaffected by the US1 change. SC-002 (zero regression on first-deploy bootstrap) is met.

---

## Phase 5: User Story 3 - Operator Can Force Regeneration by Removing the File (Priority: P2)

**Goal**: After Phase 3, the operator can `rm` the per-app file and the next `shrine deploy` regenerates it from the current manifest (FR-002 again, exercised via the Absent → Generated transition after a previous Generated state). This is the SC-006 "single-step recovery" criterion.

**Independent Test**: With an existing per-app file in place (from a prior `shrine deploy`), run `rm <routingDir>/dynamic/<team>-<service>.yml`; run `shrine deploy`; verify a fresh file is written from the current manifest (including any manifest changes the operator made between the original and current deploy) and the deploy log shows `gateway.route.generated`. The integration scenario T005 exercises Absent → Generated → Operator-Owned (preserved); this phase adds the Operator-Owned → Absent (operator rm) → Generated transition.

### Tests for User Story 3

- [X] T013 [US3] Add unit test `TestWriteRoute_AbsentAfterPreviousPresent_RegeneratesFromManifest` in `internal/plugins/gateway/traefik/routing_test.go`: uses `lstatFn` fake returning `os.ErrNotExist`; calls `WriteRoute(opWithChangedAlias)` where `opWithChangedAlias` has a routing field that differs from `baseOp()` (e.g., adds an alias); asserts the captured `writeFileFn` bytes contain the new alias's router rule (i.e., the fresh write reflects the *current* op, not any prior cached state); asserts `gateway.route.generated` is emitted

### Implementation for User Story 3

(No new implementation — covered by T009. The "after previous present" framing is a logical state, not a code path; `WriteRoute` is stateless across calls.)

### Verification for User Story 3

- [X] T014 [US3] Manually walk through quickstart.md sections 1–4 (deploy, edit, redeploy preserve, rm-and-redeploy regenerate) against a homelab host or the integration suite's fixture — confirm the regenerated file reflects manifest changes, not the original first-deploy content

**Checkpoint**: After T013–T014, US3 is verified. SC-006 (single-step recovery via `rm`) is met.

---

## Phase 6: Cross-Cutting — Orphan-File Warning on Teardown (FR-009)

**Purpose**: Implement FR-009 — when an app is removed from the manifest, the next `shrine teardown` MUST NOT delete the per-app file but MUST emit `gateway.route.orphan` (Warning) naming the file the operator must clean up. Today `RemoveRoute` has zero callers; this phase wires it in and changes its behavior.

(No `[USx]` label — FR-009 is broader than any single user story in spec.md. It is a cross-cutting requirement that emerged from the 2026-05-01 clarification session.)

### Tests for Cross-Cutting Phase (TDD — written and FAILING before T018 implementation)

- [X] T015 [P] Add unit test `TestRemoveRoute_FilePresent_EmitsOrphanWarning` in `internal/plugins/gateway/traefik/routing_test.go`: uses `lstatFn` fake returning a non-nil `os.FileInfo`; calls `RemoveRoute("team", "service")`; asserts the function returns nil (FR-011); asserts `os.Remove` is NOT invoked (use a captured `removeFileFn` test seam — this seam does not exist today; introduce it as `var removeFileFn = os.Remove` in `routing.go` so the test can fake it; the production path uses `removeFileFn`); asserts `rec.events` contains exactly one event with `Name == "gateway.route.orphan"`, `Status == engine.StatusWarning`, and fields `team`, `name`, `path`
- [X] T016 Add unit test `TestRemoveRoute_FileAbsent_IsNoOp` in `internal/plugins/gateway/traefik/routing_test.go`: uses `lstatFn` fake returning `os.ErrNotExist`; calls `RemoveRoute("team", "service")`; asserts the function returns nil; asserts `removeFileFn` is NOT called; asserts `rec.events` is empty (sequential after T015 — same file)
- [X] T017 Add unit test `TestRemoveRoute_StatError_EmitsWarningAndContinues` in `internal/plugins/gateway/traefik/routing_test.go`: uses `lstatFn` fake returning a permission-denied error; calls `RemoveRoute("team", "service")`; asserts the function returns nil (deploy/teardown continues); asserts `removeFileFn` is NOT called; asserts `rec.events` contains a single `gateway.route.stat_error` event with `error` field set
- [X] T018 [P] Add unit test `TestEngine_TeardownApplication_CallsRoutingRemove` in `internal/engine/engine_test.go` (create the file if it does not exist): construct an `Engine` with a fake `RoutingBackend` (recording its `RemoveRoute` calls) and a fake `ContainerBackend`; call `engine.ExecuteTeardown("team", []planner.PlannedStep{{Kind: manifest.ApplicationKind, Name: "service"}})`; assert the fake routing backend received `RemoveRoute("team", "service")` exactly once; assert the call happens AFTER the container-remove call (Constitution VI ordering); assert that when `engine.Routing == nil`, the teardown still succeeds (the call is gated on `engine.Routing != nil`)

### Implementation for Cross-Cutting Phase

- [X] T019 Modify `RemoveRoute` in `internal/plugins/gateway/traefik/routing.go`: introduce a package-level `var removeFileFn = os.Remove` test seam at the top of the file (alongside existing `writeFileFn`/`mkdirAllFn`); compute `path := filepath.Join(r.dynamicDir(), routeFileName(team, host))`; call `present, err := isPathPresent(path)`; if `err != nil`, emit `gateway.route.stat_error` (Status=Warning) and return nil; if `present`, emit `gateway.route.orphan` (Status=Warning, fields `team`, `name=host`, `path`) and return nil — do NOT call `removeFileFn`; if absent, return nil silently (no event); the existing `os.Remove`-and-translate-NotExist branch is removed entirely
- [X] T020 Modify `engine.teardownKind` in `internal/engine/engine.go`: after the existing `engine.Container.RemoveContainer(op)` call (line ~227) and before the `return nil`, add a gated routing-removal block: `if step.Kind == manifest.ApplicationKind && engine.Routing != nil { if err := engine.Routing.RemoveRoute(team, step.Name); err != nil { return engine.emitErr(kind+".routing_remove", map[string]string{"team": team, "name": step.Name}, fmt.Errorf("%s %q routing: %w", kind, step.Name, err)) } }`; the gate ensures we only call this for applications (not resources) and only when a routing backend is configured; `RemoveRoute`'s own internal stat-error / orphan-warn paths return nil so they never trigger the `emitErr` branch — only true I/O failures (which the new implementation does not produce; this is defensive)

**Checkpoint**: After T015–T020, `go test ./internal/plugins/gateway/traefik/...` and `go test ./internal/engine/...` pass with the new test additions. FR-009 is delivered. The user can run `shrine teardown <team>` against a torn-down app and see `gateway.route.orphan` in the deploy log naming the orphan file path.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Final verification, documentation sync, and the integration-test gate.

- [X] T021 Run the full unit-test suite: `go test ./...` — assert zero failures, including the new test files in `internal/plugins/gateway/traefik/` and `internal/engine/`
- [X] T022 Run the integration-test gate: `go test -tags integration ./tests/integration/...` (or `make test-integration`) — assert zero failures including the new `TestTraefikPlugin_PerAppFile_PreservedAcrossRedeploys` scenario; this is the Constitution V gate and is expected to take ~10 minutes per [memory: integration tests are slow]
- [X] T023 [P] Manual quickstart.md walkthrough end-to-end against a homelab host: deploy → edit → redeploy → preserve → tear down → orphan warn → rm; confirm the operator-facing log lines match `contracts/log-events.md` (event names, fields, severities)
- [X] T024 [P] Verify `cmd/cmd_test.go:71`'s existing dry-run assertion still passes; if there is a dry-run teardown test that now produces a new `[ROUTE]  RemoveRoute: domain=... (team=...)` line (per Decision 3 in `research.md`), update its expected output to include that line — otherwise no change needed
- [X] T025 Constitution Check final pass: confirm Principle III (engine.RoutingBackend interface unchanged), Principle IV (only one helper extraction `isPathPresent`; no new types beyond the `removeFileFn` test seam), Principle V (one new integration scenario added), Principle VI (RemoveRoute called AFTER RemoveContainer in teardownKind), Principle VII (no WHAT comments added; one WHY comment on `isPathPresent` explaining the `Lstat` choice is acceptable)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1, T001)**: No dependencies — baseline confirmation
- **Foundational (Phase 2, T002–T004)**: Depends on T001 — BLOCKS all subsequent phases
- **User Story 1 (Phase 3, T005–T010)**: Depends on T002–T004 — delivers the MVP and unblocks US2/US3 verification
- **User Story 2 (Phase 4, T011–T012)**: Depends on Phase 3 (verification only — no new implementation)
- **User Story 3 (Phase 5, T013–T014)**: Depends on Phase 3 (one new unit test on the same code path)
- **Cross-Cutting Phase 6 (T015–T020)**: Depends on T002–T004 only — can run **in parallel with Phase 3** if a second developer is available, since it touches `RemoveRoute` (Phase 3 touches `WriteRoute`) and a different engine path (`teardownKind` vs. `deployApplication`); the only shared file is `routing_test.go` which both phases extend, so the test extensions must be sequenced even if implementation is parallel
- **Polish (Phase 7, T021–T025)**: Depends on Phases 3, 5, 6 (Phase 4 is verification-only and folds into Phase 7's full-suite run)

### Within-Phase Dependencies

- **Phase 2**: T002, T003, T004 are all `[P]` — different files, no inter-task dependency
- **Phase 3**: T005 (`tests/integration/traefik_plugin_test.go`) and T006 (`routing_test.go`) are `[P]` (different files); T007 and T008 are sequential after T006 (same file). T009 depends on T002 (uses `isPathPresent`) and on T003 (uses `r.observer`) — implementation comes after the tests are written and failing. T010 depends on T009 (existing tests need their `lstatFn` fakes added to keep the fresh-write path exercised after the new pre-check)
- **Phase 6**: T015 (`routing_test.go`) and T018 (`engine_test.go`) are `[P]` (different files); T016 and T017 are sequential after T015 (same file). T019 depends on T002 (`isPathPresent`) and T003 (observer); T020 depends on T019 (calls `RemoveRoute` whose new behavior must already be in place to not delete files)

### Parallel Opportunities

- T002, T003, T004 can run in parallel (Foundational; different files)
- T005 (integration test, `tests/integration/traefik_plugin_test.go`) can run in parallel with T006 (`internal/plugins/gateway/traefik/routing_test.go`); T007 and T008 are sequential after T006 because they share `routing_test.go`
- T015 (`routing_test.go`) can run in parallel with T018 (`internal/engine/engine_test.go`); T016 and T017 are sequential after T015 because they share `routing_test.go`
- Phase 3 implementation (T009) and Phase 6 implementation (T019) can be done by two developers because they touch different methods in `routing.go`; merge conflicts are line-level and trivial. Test additions for the two phases sequence on `routing_test.go` even when implementations are parallel.
- T023, T024 can run in parallel within Phase 7 (different concerns)

---

## Parallel Example: Phase 3 (User Story 1)

```bash
# After Phase 2 completes, launch the two file-disjoint US1 test tasks in parallel (TDD — write tests first, watch them fail):
Task: "T005 Add integration scenario TestTraefikPlugin_PerAppFile_PreservedAcrossRedeploys in tests/integration/traefik_plugin_test.go"
Task: "T006 Add unit test TestWriteRoute_FilePresent_Preserves in internal/plugins/gateway/traefik/routing_test.go"

# Then add the remaining unit tests to routing_test.go sequentially (same file as T006):
Task: "T007 Add unit test TestWriteRoute_FreshWrite_EmitsGenerated in internal/plugins/gateway/traefik/routing_test.go"
Task: "T008 Add unit test TestWriteRoute_StatError_EmitsWarningAndContinues in internal/plugins/gateway/traefik/routing_test.go"

# Then run the implementation tasks sequentially (same file, single method):
Task: "T009 Modify WriteRoute in internal/plugins/gateway/traefik/routing.go to pre-check isPathPresent and emit gateway.route.{generated,preserved,stat_error}"
Task: "T010 Update existing routing_test.go cases to fake lstatFn to ErrNotExist so the fresh-write path stays exercised"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (T001) — baseline.
2. Complete Phase 2 (T002–T004) — helper, observer plumbing, shared test fixture.
3. Complete Phase 3 (T005–T010) — preserve-on-write + stat-error handling.
4. **STOP and VALIDATE**: Run `go test ./internal/plugins/gateway/traefik/...` and the new integration scenario. The MVP is shippable: operators get the bug fix for the deploy axis. The teardown axis (FR-009) is still pre-fix at this point, but its absence does not break anything — today's behavior is "silent orphan," and that continues unchanged until Phase 6 lands.

### Incremental Delivery

1. **Increment 1 (MVP)**: Phases 1 → 2 → 3 → ship. Operators no longer lose hand-edits to per-app routing files on redeploy.
2. **Increment 2**: Phase 6 (T015–T020). Operators now also see explicit warnings about orphan files when they tear apps down.
3. **Increment 3**: Phases 4 + 5 verification + Phase 7 polish. Final gate before declaring the feature complete.

### Single-Developer Strategy

Sequential by phase, in the order listed (Phases 1 → 2 → 3 → 6 → 4 → 5 → 7). Estimated effort: T001–T020 are mostly small (10–30 line diffs each); the integration test (T005) is the largest single task. Total wall-clock estimate: 4–6 hours of focused work, plus ~10 minutes for the integration test gate.

---

## Notes

- `[P]` tasks = different files OR different test functions in the same file with no inter-test dependency
- `[Story]` label maps task to specific user story for traceability; cross-cutting and polish tasks have no story label
- TDD is required for the integration scenario per Constitution V; unit tests are listed as `[P]` to the implementation but should be written first when single-threaded
- Verify tests fail (or skip cleanly) before running implementation tasks
- Commit after each task or logical group; Phase boundaries are natural commit points
- Stop at any checkpoint to validate independently — Phase 3 alone is the MVP and is shippable without Phase 6
- Avoid: same-file race conditions on `routing_test.go` between Phase 3 and Phase 6 unit-test additions; sequence test additions even when implementations are parallel
