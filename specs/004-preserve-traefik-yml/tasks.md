---

description: "Task list for feature 004-preserve-traefik-yml"
---

# Tasks: Preserve Operator-Edited traefik.yml

**Input**: Design documents from `/specs/004-preserve-traefik-yml/`
**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/observer-events.md](contracts/observer-events.md), [quickstart.md](quickstart.md)

**Tests**: Required by Constitution Principle V — integration test files MUST be created before the implementation code. Unit tests cover the stat-branching logic; per saved feedback, unit tests MUST NOT touch the filesystem (use a swappable stat shim).

**Organization**: Tasks are grouped by user story. The fix is a single small code path, so the foundational phase carries the observer plumbing and the user-story phases carry the existence-probe behavior plus their respective test scenarios.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- All file paths below are repository-relative.

## Path Conventions

Single Go module — `internal/`, `tests/integration/` at the repository root. No new directories are created by this feature.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: N/A for this feature.

The Go module, Cobra CLI, observer event bus, terminal logger, and integration-test harness all already exist and are unchanged by this feature. No setup tasks.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Plumb the existing `engine.Observer` into the Traefik gateway plugin so the per-story phases can emit the new info events without further wiring changes. Does not change behavior on its own — every existing test must remain green after this phase.

**⚠️ CRITICAL**: User Story 1, 2, and 3 work all depend on this phase.

- [X] T001 Extend `traefik.New(...)` in [internal/plugins/gateway/traefik/plugin.go](internal/plugins/gateway/traefik/plugin.go) to accept an `observer engine.Observer` fourth parameter; store it on the `Plugin` struct; normalize a nil argument to `engine.NoopObserver{}` inside the constructor so `Deploy()` never branches on nil.
- [X] T002 Update the single production caller in [internal/handler/deploy.go](internal/handler/deploy.go) (around line 80, where `traefik.New(...)` is invoked) to pass the existing `observer` variable (the `engine.MultiObserver` constructed earlier in `Deploy(...)`) as the new fourth argument.
- [X] T003 [P] Add two `case` branches in [internal/ui/terminal_logger.go](internal/ui/terminal_logger.go) for `gateway.config.preserved` (renders `  📄 Preserving operator-owned traefik.yml: <path>`) and `gateway.config.generated` (renders `  📝 Generated default traefik.yml: <path>`); both read the absolute path from `e.Fields["path"]`. No spinner, no started/finished pairing — these are point-in-time `StatusInfo` events.

**Checkpoint**: All existing unit and integration tests still pass; no behavior change is observable yet (no event is emitted because `generateStaticConfig` has not been changed yet).

---

## Phase 3: User Story 1 — Operator-Edited traefik.yml Survives Re-Deploys (Priority: P1) 🎯 MVP

**Goal**: On every `shrine deploy` against a host that already has any entry at the `traefik.yml` path, Shrine performs no write — content, mode, owner, and mtime stay byte-for-byte identical — and the deploy log carries a `gateway.config.preserved` info line naming the file.

**Independent Test**: From [quickstart.md](quickstart.md#story-1--operator-edits-survive-redeploys) — first-deploy generates the default; operator hand-edits the file; second deploy completes successfully and `sha256sum` is identical before and after.

### Tests for User Story 1 ⚠️

> **NOTE: Per Constitution Principle V, integration tests below MUST be written and FAIL before the implementation tasks (T012–T015) start. The fakes/shims for unit tests are part of the implementation tasks since they're a code-design choice.**

- [X] T004 [US1] Add integration scenario "should preserve operator-edited traefik.yml across redeploys" to [tests/integration/traefik_plugin_test.go](tests/integration/traefik_plugin_test.go) inside `TestTraefikPlugin`: deploy → `os.WriteFile` a unique marker into `routingDir/traefik.yml` → deploy again → `tc.AssertFileExists`, read the file, and assert the marker bytes are still present unchanged. Also assert the deploy stdout contains `Preserving operator-owned traefik.yml`.
- [X] T005 [US1] Add integration scenario "should preserve a broken symlink at traefik.yml across deploy" to [tests/integration/traefik_plugin_test.go](tests/integration/traefik_plugin_test.go): pre-create `routingDir/traefik.yml` as a symlink to `/nonexistent/path` → deploy → assert the symlink's target is unchanged via `os.Readlink` and `/nonexistent/path` was NOT created on disk.
- [X] T006 [US1] Add integration scenario "should preserve a directory at the traefik.yml path" to [tests/integration/traefik_plugin_test.go](tests/integration/traefik_plugin_test.go): pre-create `routingDir/traefik.yml` as a directory → deploy with `AssertSuccess()` (Shrine's responsibility ends at "do not touch") → assert the directory still exists and is still a directory after the deploy.
- [X] T007 [US1] Add integration scenario "should fail deploy with a clear error when stat on traefik.yml fails for a reason other than NotExist" to [tests/integration/traefik_plugin_test.go](tests/integration/traefik_plugin_test.go): pre-create `routingDir/traefik.yml` and `chmod 0o000` the routing directory (use `t.Cleanup` to restore mode) → deploy with `AssertFailure()` → assert the captured stderr names `traefik.yml` and contains the underlying cause.

### Implementation for User Story 1

- [X] T008 [US1] Introduce a swappable stat shim at the top of [internal/plugins/gateway/traefik/config_gen.go](internal/plugins/gateway/traefik/config_gen.go): `var lstatFn = os.Lstat` (package-private function variable). All filesystem checks in the plugin's existence-probe path go through this variable so unit tests can swap it without filesystem I/O. (Required by the "unit tests must not touch the filesystem" feedback.)
- [X] T009 [US1] Add the helper `func isStaticConfigPresent(routingDir string) (bool, error)` in [internal/plugins/gateway/traefik/config_gen.go](internal/plugins/gateway/traefik/config_gen.go). Returns `(true, nil)` if `lstatFn` returns no error; `(false, nil)` if `os.IsNotExist(err)` is true; `(false, fmt.Errorf("traefik plugin: checking traefik.yml at %q: %w", path, err))` otherwise. Uses `filepath.Join(routingDir, "traefik.yml")` to build the path.
- [X] T010 [US1] Refactor `generateStaticConfig` in [internal/plugins/gateway/traefik/config_gen.go](internal/plugins/gateway/traefik/config_gen.go) to take an `observer engine.Observer` argument, call `isStaticConfigPresent` first, and: (a) on `present=true` emit `engine.Event{Name: "gateway.config.preserved", Status: engine.StatusInfo, Fields: map[string]string{"path": path}}` and return nil without writing; (b) on `present=false` perform the existing `os.WriteFile` and emit `engine.Event{Name: "gateway.config.generated", Status: engine.StatusInfo, Fields: map[string]string{"path": path}}`; (c) on stat error propagate the wrapped error.
- [X] T011 [US1] Update `Plugin.Deploy()` in [internal/plugins/gateway/traefik/plugin.go](internal/plugins/gateway/traefik/plugin.go) to pass `p.observer` into the new `generateStaticConfig` signature. The error wrapper (`"traefik plugin: generating static config: %w"`) is removed for the existence-error path so the more specific message from `isStaticConfigPresent` surfaces unchanged; keep the wrapper for the actual write-failure path.
- [X] T012 [US1] Add unit tests in a new file `internal/plugins/gateway/traefik/config_gen_test.go` (this is `package traefik`, so it compiles without the `integration` build tag): cover `isStaticConfigPresent` for all four branches (regular file present, broken symlink present, directory present, NotExist) using a fake `lstatFn` swapped via the package var; cover `generateStaticConfig`'s skip-vs-write branching using a `recordingObserver` (a struct that captures emitted events) and a fake `lstatFn`. NO `t.TempDir()`, NO `os.WriteFile`, NO `os.MkdirAll` — all I/O is mocked at the stat-shim level; the write branch is covered by the integration scenario in T004, not here. (This task implements both the design that makes mocking possible and the tests that consume it.)

**Checkpoint**: User Story 1 is fully functional. Operator-edited `traefik.yml` survives redeploys (T004), broken symlinks survive (T005), directories at the path survive (T006), stat errors fail loudly (T007). The MVP slice can ship after this phase even without User Story 2 and 3 work.

---

## Phase 4: User Story 2 — First Deploy Still Bootstraps a Working Gateway (Priority: P1)

**Goal**: First deploy on a clean host still writes the default `traefik.yml`, the gateway container still starts, and the deploy log carries a `gateway.config.generated` info line. This is a no-regression slice — the only new visible artifact is the log signal.

**Independent Test**: From [quickstart.md](quickstart.md#story-2--first-deploy-still-bootstraps-a-working-gateway) — clean directory, run `shrine deploy`, file present with default content, container reachable.

### Tests for User Story 2 ⚠️

- [X] T013 [US2] Extend the existing scenario "should deploy traefik container when plugin block is populated" in [tests/integration/traefik_plugin_test.go](tests/integration/traefik_plugin_test.go) with an `tc.AssertOutputContains("Generated default traefik.yml")` line so SC-004's "log clearly indicates… generated or preserved" is enforced on the bootstrap path. No new test function — the existing first-deploy scenario already proves bootstrap correctness; this task is purely an additional assertion on the same `tc.Run("deploy", ...)` output buffer.

### Implementation for User Story 2

No new implementation work. The `gateway.config.generated` event emitted in T010 (already required for the existence-probe write path) IS the implementation for this story. The terminal-logger render added in T003 turns it into the asserted log line.

**Checkpoint**: User Stories 1 AND 2 are now both verified. First deploy still bootstraps (existing assertions plus the new log assertion); operator edits still survive redeploys.

---

## Phase 5: User Story 3 — Operator Can Force Regeneration by Removing the File (Priority: P3)

**Goal**: An operator who deletes `traefik.yml` and re-runs `shrine deploy` gets a fresh default file written; the deploy log carries `gateway.config.generated`.

**Independent Test**: From [quickstart.md](quickstart.md#story-3--operator-can-opt-back-into-the-default-by-deleting-the-file) — file present → `rm` it → redeploy → file present again with default content.

### Tests for User Story 3 ⚠️

- [X] T014 [US3] Add integration scenario "should regenerate default traefik.yml after operator deletes the file" to [tests/integration/traefik_plugin_test.go](tests/integration/traefik_plugin_test.go): deploy (file appears) → `os.Remove` the file → deploy again → assert file exists, content matches the current default-generator output (compare against a fresh `generateStaticConfig` invocation in a test scratch dir, OR assert the file is non-empty and contains the canonical entry-point YAML key — choice left to the implementer based on local convention) → assert the deploy stdout contains `Generated default traefik.yml`.

### Implementation for User Story 3

No new implementation work. The fall-through branch in `generateStaticConfig` (T010, `present=false`) IS the implementation for this story. T014 only adds the verifying scenario.

**Checkpoint**: All three user stories pass independently and the deploy log clearly distinguishes generated vs. preserved (SC-004) on every run.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Run the canonical Principle V gate and confirm the quickstart walkthrough still matches reality.

- [X] T015 Run `go test ./...` from the repository root; expect green. Then run `make test-integration` (or `go test -tags integration ./tests/integration/...`); expect green, including the four new `TestTraefikPlugin` scenarios from T004–T007 and T013–T014. This is the Phase V integration-test gate per the constitution.
- [X] T016 [P] Re-read [quickstart.md](quickstart.md) end-to-end against the implemented binary on a real Docker host (or smock equivalent if no clean host is available); update any drift between documented log lines and actual stdout. The documented lines are `📄 Preserving operator-owned traefik.yml:` and `📝 Generated default traefik.yml:` — confirm exact match with the renderer added in T003.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: N/A — no setup tasks.
- **Foundational (Phase 2 — T001–T003)**: BLOCKS all user-story phases. T002 depends on T001 (`traefik.New` signature). T003 has no dependency on T001/T002 and can run in parallel with either of them ([P] marker reflects this).
- **User Stories (Phases 3–5)**: All depend on Foundational completion. Within each phase, integration tests (T004–T007, T013, T014) are authored BEFORE the implementation tasks per Principle V; they are expected to fail until the implementation tasks land.
- **Polish (Phase 6)**: Depends on every prior task. T016 has no source dependency on T015 and is parallelizable, but in practice T015 should pass before manual quickstart re-verification.

### User Story Dependencies

- **US1 (P1)**: Independent of US2/US3 once Foundational is done. Implementation in T008–T012 is the substrate that makes US2 and US3 work; nonetheless, US1 can ship as MVP and validate independently.
- **US2 (P1)**: Independent of US3. Adds one assertion (T013) to verify FR-006 on the bootstrap path. The implementation for the assertion was already done as part of US1's T010.
- **US3 (P3)**: Independent of US2. Single new test scenario (T014). The implementation was already done as part of US1's T010.

### Within Each User Story

- Integration tests authored first (T004–T007 for US1; T013 for US2; T014 for US3) — they MUST fail before T008–T012 land per Principle V.
- For US1, T008 (stat shim) → T009 (helper) → T010 (gating + emit) → T011 (Deploy wrapper rewording) → T012 (unit tests against the mocked shim).
- T011 depends on T010 (signature of `generateStaticConfig` changes there).
- T012 depends on T008 (the swappable shim) and T009 (the helper) and T010 (the observer signature).

### Parallel Opportunities

- T003 [P] in Phase 2 (different file from T001/T002).
- T016 [P] in Phase 6 (manual walkthrough, independent of T015).
- All user-story phases (US1, US2, US3) share the same test file `tests/integration/traefik_plugin_test.go`, so the integration-test tasks (T004, T005, T006, T007, T013, T014) cannot be marked [P] under the strict "different files" rule even though they are conceptually independent test cases. A team that wants to parallelize them can serialize the file commits.

---

## Parallel Example: Phase 2 Foundational

```bash
# T001 (plugin.go constructor) and T002 (handler/deploy.go caller) are sequential — same dependency chain.
# T003 (terminal_logger.go) touches a different file and has no dependency on T001/T002:
Task: "Add gateway.config.preserved + gateway.config.generated render cases in internal/ui/terminal_logger.go"
# can run in parallel with whoever is doing T001 → T002.
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1: Setup — N/A, skip.
2. Phase 2: Foundational (T001–T003) — plumb the observer into the plugin. Existing tests must stay green.
3. Phase 3: User Story 1 — author T004–T007 first (they fail), then T008–T012 (they pass), then T012 unit tests pass without touching the filesystem.
4. **STOP and VALIDATE**: Run `make test-integration`. Confirm preserve-on-redeploy works against a real Docker host (T004 scenario). MVP can ship here.

### Incremental Delivery

- Foundational + US1 → MVP. Ship.
- Add US2 verification (T013) → confirms FR-006 on the bootstrap path. Ship in the same release or a follow-up.
- Add US3 verification (T014) → confirms the delete-then-redeploy story. Ship.
- Polish (T015–T016) → final integration gate + quickstart re-verification before the release tag.

### Solo Strategy (most likely for this feature)

Single-developer execution:

1. T001 → T002 → T003.
2. T004, T005, T006, T007 (write all four failing scenarios up front so the implementation hits a clear target).
3. T008 → T009 → T010 → T011.
4. T012 (unit tests against the shim).
5. T013, T014 (the additional verification scenarios for US2 and US3).
6. T015, T016.

Total estimated edits: 3 production source files (plugin.go, config_gen.go, deploy.go), 1 UI file (terminal_logger.go), 1 new unit-test file (config_gen_test.go), 1 integration-test file extended (traefik_plugin_test.go).

---

## Notes

- [P] tasks = different files, no dependencies. Most tasks in this feature share files and are therefore not parallelizable under the strict definition.
- [Story] label on T004–T014 maps each task to its user story for traceability against [spec.md](spec.md) acceptance scenarios.
- Per Constitution Principle V, the integration-test files (T004–T007, T013, T014) MUST land before T008–T012; they're expected to fail until the implementation tasks complete.
- Per the saved feedback memory, unit tests in T012 MUST NOT touch the filesystem — the swappable `lstatFn` shim from T008 is what makes that possible.
- Per the saved feedback memory, the integration test fixtures in `tests/integration/testutils/` are intentionally isolated from `internal/` packages — duplicating small helpers there (e.g., `os.Symlink` setup for T005) is fine and not a DRY violation.
- Commit boundary suggestion: one commit per phase (Foundational, US1, US2, US3, Polish), not one per task.
