---
description: "Task list for Fix Routing-Dir Manifest Scan Crash"
---

# Tasks: Fix Routing-Dir Manifest Scan Crash

**Feature**: `002-fix-routing-dir-manifest-scan`
**Plan**: [plan.md](./plan.md) | **Spec**: [spec.md](./spec.md) | **Contracts**: [contracts/scanner-contract.md](./contracts/scanner-contract.md)
**Tests**: REQUIRED — Constitution V mandates an integration-test gate, and the user explicitly requested TDD ordering ("integration tests shall be created first, and it's expected for them to fail").

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Different file, no in-flight dependency — safe to run in parallel
- **[Story]**: User story this task belongs to (US1, US2, US3) — Setup / Foundational / Polish phases carry no story label

## Path Conventions

Single-binary Go CLI. Sources under `internal/...` and `cmd/...`; integration suite under `tests/integration/` with fixtures under `tests/testdata/`. Paths in this file are relative to the repository root `/root/projects/shrine/`.

## TDD Ordering Rule (per user request + Constitution V)

Within EVERY story phase, integration tests MUST be created and committed first. Run them with `make test-integration` and **verify they FAIL** before writing any implementation task in the same phase. The Foundational phase's unit tests follow the same rule (`go test ./internal/manifest/...` must fail before `Classify` / `ScanDir` exist).

The implementation strategy section at the bottom of this file enforces the order; do not reorder phases.

---

## Phase 1: Setup (Shared Fixtures)

**Purpose**: Build the test-data fixtures every story consumes. No production code.

- [X] T001 [P] Create deploy-side foreign-yaml fixture skeleton at [tests/testdata/deploy/foreign-yaml/](../../tests/testdata/deploy/foreign-yaml/) with `team.yaml` (`shrine-deploy-test` team, mirrors [tests/testdata/deploy/team.yaml](../../tests/testdata/deploy/team.yaml)) and `app.yaml` (a single `whoami` Application named `whoami` belonging to `shrine-deploy-test`, port 80, no dependencies — mirrors [tests/testdata/deploy/basic/app.yml](../../tests/testdata/deploy/basic/app.yml)).
- [X] T002 [P] Add Traefik static config file at `tests/testdata/deploy/foreign-yaml/traefik/traefik.yml` containing a YAML body with NO `apiVersion` field (mirrors what [internal/plugins/gateway/traefik/config_gen.go](../../internal/plugins/gateway/traefik/config_gen.go) writes — `entryPoints`, `providers.file.directory`, etc.).
- [X] T003 [P] Add Traefik dynamic route file at `tests/testdata/deploy/foreign-yaml/traefik/dynamic/team-foo-app.yml` containing an `http.routers...` body with NO `apiVersion`.
- [X] T004 [P] Add a non-YAML sibling at `tests/testdata/deploy/foreign-yaml/notes.json` containing `{"note": "operator scratchpad"}` to exercise the FR-001(a) extension filter (the file MUST never be opened by the scanner).
- [X] T005 [P] Create apply-side foreign-yaml fixture skeleton at [tests/testdata/apply/foreign-yaml/](../../tests/testdata/apply/foreign-yaml/) with `team.yaml` (`shrine-apply-test` team), `app.yaml` (a `whoami-apply-foreign` Application), and `traefik/traefik.yml` (no `apiVersion`).
- [X] T006 [P] Create malformed-YAML fixture at `tests/testdata/deploy/malformed-yaml/` containing the same `team.yaml` + `app.yaml` as T001 PLUS a `broken.yaml` whose body is `apiVersion: shrine/v1\nkind: [unclosed`. This fixture is used by US3 to verify FR-004.
- [X] T007 [P] Create shrine-with-bad-kind fixture at `tests/testdata/deploy/bad-kind/` containing `team.yaml` + a `typo.yaml` with `apiVersion: shrine/v1`, `kind: Aplication` (typo on purpose), valid metadata, valid spec. Used by US3 to verify FR-003.
- [X] T008 [P] Create apply-side bad-kind fixture at `tests/testdata/apply/bad-kind/` mirroring T007 plus a `team.yaml` for `shrine-apply-test`.

**Checkpoint**: All fixtures in place. No production code touched yet.

---

## Phase 2: Foundational — Shared Classifier (Blocking Prerequisites)

**Purpose**: Build the `Classify` + `ScanDir` API in `internal/manifest/`. Every story phase below relies on it. TDD: unit tests first, run them to confirm they fail, then implement.

**⚠️ CRITICAL**: No story-phase work may begin until Phase 2 is green.

### TDD Tests for Foundational (write FIRST, must FAIL)

- [X] T009 Write table-driven unit tests for `IsShrineAPIVersion` and `Classify` in [internal/manifest/classify_test.go](../../internal/manifest/classify_test.go) covering every row of the contract table in [contracts/scanner-contract.md §3](./contracts/scanner-contract.md): strict v1 / v1beta1 / v10alpha7 → `ClassShrine`; capital-S / plural / no-version-suffix / trailing-space-in-quoted-scalar / empty / missing / foreign-apiVersion / empty-file / comments-only → `ClassForeign`; malformed YAML → error containing the file path; shrine/v1 with bogus kind → `ClassShrine` (kind is checked downstream). Use `t.TempDir()` + `os.WriteFile` to materialise per-case content.
- [X] T010 [P] Write `ScanDir` unit tests in [internal/manifest/scan_test.go](../../internal/manifest/scan_test.go): empty dir → empty `ScanResult`, no error; dir with only non-YAML files (`.json`, `.md`, extensionless, set `chmod 000`) → empty `ScanResult` and no error (proves extension filter never opens the files); dir with one valid + one foreign → `Shrine` len 1, `Foreign` len 1; nested subdir mirroring `specsDir/traefik/foo.yml` → foreign path collected with the nested path; one malformed YAML → error wrapping the file path.
- [X] T011 Run `go test ./internal/manifest/...` and confirm both new test files FAIL because `Classify` / `ScanDir` / `Class` / `ShrineCandidate` / `ScanResult` don't exist yet. Record the failure in the commit message of the test files.

### Implementation for Foundational

- [X] T012 Implement `IsShrineAPIVersion`, `Class` constants, `Classify`, and the package-level compiled regex `^shrine/v\d+([a-z]+\d+)?$` in [internal/manifest/classify.go](../../internal/manifest/classify.go). `Classify` returns `(Class, *TypeMeta, error)` per [data-model.md §Go Types](./data-model.md#go-types-introduced); reuses the existing `probeKind` logic from [internal/manifest/parser.go:31](../../internal/manifest/parser.go#L31) (extract `probeKind` to be exported, or duplicate the unmarshal — pick the lower-churn path and document the choice in a commit message).
- [X] T013 Implement `ScanDir` and the `ScanResult` / `ShrineCandidate` types in [internal/manifest/scan.go](../../internal/manifest/scan.go) per [contracts/scanner-contract.md §1](./contracts/scanner-contract.md). The walker MUST apply the `.yaml`/`.yml` extension filter BEFORE calling `Classify` (so disallowed files are never opened — invariant 1).
- [X] T014 Run `go test ./internal/manifest/...` and confirm T009 + T010 now PASS.

**Checkpoint**: Shared classifier is green at the unit-test level. Story phases unblocked.

---

## Phase 3: User Story 1 — Default Routing-Dir Layout Deploys (Priority: P1) 🎯 MVP

**Goal**: `shrine deploy` succeeds when the Traefik plugin's default `routing-dir = {specsDir}/traefik/` is in effect, on a project that crashes today (SC-001 / SC-005).

**Independent Test**: Configure the Traefik plugin with the default routing-dir, run `shrine deploy` against a project containing valid manifests + Traefik-shaped YAML in `{specsDir}/traefik/`, and confirm exit `0` with the app container running.

### TDD Integration Tests for US1 (write FIRST, must FAIL on `main`)

- [X] T015 [US1] Add integration test `should_deploy_succeed_when_routing-dir_is_inside_specsDir` to [tests/integration/traefik_plugin_test.go](../../tests/integration/traefik_plugin_test.go). The test enables the Traefik plugin with `routing-dir: {specsDir}/traefik` (the default), uses `traefikFixturePath()`, runs `shrine deploy` ONCE to generate the routing files, then runs `shrine deploy` a SECOND time against the now-populated tree (this is the canonical SC-001 path). Assert both invocations `AssertSuccess()` and that `traefikContainerName` is running after the second one.
- [X] T016 [P] [US1] Add integration test `should_deploy_succeed_when_specsDir_contains_foreign_YAML_files` to [tests/integration/deploy_test.go](../../tests/integration/deploy_test.go). Use the new `fixturesPath("foreign-yaml")` (T001-T004 fixture). Assert `shrine deploy` exits `0`, `testTeam + ".whoami"` is running, and `testTeam + ".traefik"` does NOT exist (proves the foreign file produced no container).
- [X] T017 [US1] Run `make test-integration -- -run 'TestTraefikPlugin/should_deploy_succeed_when_routing-dir|TestDeploy/should_deploy_succeed_when_specsDir_contains_foreign'` and confirm both tests FAIL with the `unknown manifest kind: ""` (or equivalent) error from [internal/manifest/parser.go:66](../../internal/manifest/parser.go#L66). Capture the failure output in the commit message — this is the regression we are fixing.

### Implementation for US1

- [X] T018 [US1] Refactor `LoadDir` in [internal/planner/loader.go](../../internal/planner/loader.go) to call `manifest.ScanDir`. Replace the inlined `getFiles` walker with consumption of `ScanResult.Shrine`; iterate candidates, call `manifest.Parse` (unchanged), `manifest.Validate` (unchanged), and `set.mapKind` (unchanged). Keep the existing duplicate-name and unknown-kind error wrapping intact (FR-007). Delete `getFiles` once unreferenced.
- [X] T019 [US1] In `LoadDir`, after the parse/validate loop, emit the FR-006 informational notice to stdout when `len(ScanResult.Foreign) > 0`: a single line of the form `shrine: ignored N non-shrine YAML file(s) under <dir>: <comma-separated paths>`. Do NOT emit the notice when `Foreign` is empty. The notice must not affect return value or exit code.
- [X] T020 [US1] Update unit tests in [internal/planner/loader_test.go](../../internal/planner/loader_test.go) to add cases: (a) directory containing one valid Application + one foreign YAML → no error, foreign file ignored, `set.Applications` has the valid one; (b) directory containing only foreign YAML → no error, empty `set`; (c) directory containing valid + malformed `.yaml` → error referencing the malformed file (FR-004).
- [X] T021 [US1] Run `make test-integration -- -run 'TestTraefikPlugin|TestDeploy'` and confirm T015 + T016 now PASS plus every existing case in those files still passes (regression guard for FR-007).

**Checkpoint**: SC-001 + SC-005 satisfied. The MVP fix is live for `shrine deploy`. The same refactor also fixes `shrine apply -f` (because [internal/planner/plan.go:71](../../internal/planner/plan.go#L71) `PlanSingle` calls the same `LoadDir`) — US2 will add explicit coverage for that path.

---

## Phase 4: User Story 2 — Foreign YAML Coexists Across All Scan Commands (Priority: P2)

**Goal**: Every shrine command that scans a directory (`deploy`, `apply teams`, `apply -f`) silently skips foreign YAML and non-YAML siblings without crashing (SC-002 / FR-005). The user explicitly called out that the planner change touches all three command surfaces, so each one needs its own test.

**Independent Test**: Place foreign YAML and non-YAML files alongside valid manifests; run each of the three commands; confirm exit `0` and that only the legitimate manifests took effect.

### TDD Integration Tests for US2 (write FIRST, must FAIL or be missing on `main`)

- [X] T022 [P] [US2] Add integration test `should_succeed_when_specsDir_contains_a_yaml_without_shrine_apiVersion` to [tests/integration/deploy_test.go](../../tests/integration/deploy_test.go) using the T001-T003 fixture. Asserts `deploy` exits `0` and the app is running (covers the user-listed case "`shrine deploy` should not fail with a yaml/yml file without shrine version"). STATUS: already-covered-by-T016 — T016's assertions are a superset; adding a duplicate would provide no coverage. Documented in a comment in deploy_test.go.
- [X] T023 [P] [US2] Add integration test `should_succeed_when_specsDir_contains_non_yaml_files` to [tests/integration/deploy_test.go](../../tests/integration/deploy_test.go). Uses the T004 `notes.json` plus a programmatically-added `chmod 000`-ed `config.json` placed in `tc.Path("")` to prove the extension filter never opens disallowed files (covers "`shrine deploy` should not fail with non-.yaml/.yml files in specsDir").
- [X] T024 [P] [US2] Add integration test `should_apply_teams_succeed_when_specsDir_contains_foreign_yaml` to [tests/integration/apply_test.go](../../tests/integration/apply_test.go) using the T005 fixture (`apply/foreign-yaml/`). Asserts `apply teams` exits `0` and `tc.AssertTeamInState("shrine-apply-test")` succeeds (covers "`shrine apply teams` should not fail").
- [X] T025 [P] [US2] Add integration test `should_apply_file_succeed_when_specsDir_contains_foreign_yaml` to [tests/integration/apply_test.go](../../tests/integration/apply_test.go). Uses `apply -f` against the `app.yaml` in the T005 fixture with `--path` pointing at the same directory (which contains the foreign `traefik/traefik.yml`). Asserts exit `0` and the resulting container is running (covers "`shrine apply -f` should not fail").
- [X] T026 [US2] Run `make test-integration -- -run 'TestDeploy/should_succeed_when_specsDir_contains|TestApplyTeams/should_apply_teams_succeed_when|TestApplyFile/should_apply_file_succeed_when'` and confirm T022, T024, T025 FAIL on the current branch state (T023 may pass since today's planner already filters by extension — record actual outcome in commit message). T024 will fail because `walkYAMLFiles` in `internal/handler/teams.go` still hands every YAML to `manifest.Parse` and the foreign one yields `unknown manifest kind`.

### Implementation for US2

- [X] T027 [US2] Refactor `walkYAMLFiles` and `ApplyTeams` in [internal/handler/teams.go](../../internal/handler/teams.go) to call `manifest.ScanDir` instead of the inlined walker. Iterate `ScanResult.Shrine`, call `manifest.Parse`, keep only candidates whose `m.Team != nil`, call `store.SaveTeam` exactly as today. Delete `walkYAMLFiles` once unreferenced (it has only one caller — `ApplyTeams`).
- [X] T028 [US2] In `ApplyTeams`, after the loop, emit the same FR-006 foreign-files notice as T019 when `len(ScanResult.Foreign) > 0`. Match the wording so operators see one consistent message regardless of command. Do NOT emit when empty.
- [X] T029 [US2] Verify `apply -f` (`PlanSingle` in [internal/planner/plan.go](../../internal/planner/plan.go)) inherits the fix automatically through T018: trace the call once and add a one-line code comment in `PlanSingle` only if there is a non-obvious WHY (per Principle VII: do not add WHAT comments).
- [X] T030 [US2] Run `make test-integration -- -run 'TestDeploy|TestApplyTeams|TestApplyFile'` and confirm T022-T025 now PASS plus every existing case still passes. The FR-006 notice should be visible in the captured stdout for the foreign-yaml fixtures only.

**Checkpoint**: SC-002 satisfied across all three commands. Constitution V's "every backend implementation MUST be covered by an integration test scenario" is upheld for the scanner-side change.

---

## Phase 5: User Story 3 — Genuinely Broken Shrine Manifests Still Fail Loudly (Priority: P2)

**Goal**: Files that self-identify as shrine (`apiVersion: shrine/v1`) but have a missing/typo'd `kind`, or files that are unparseable YAML, MUST cause loud failures with the file path in the error message — across all three commands (FR-003, FR-004, SC-004).

**Independent Test**: Place a file with `apiVersion: shrine/v1` + `kind: Aplication` in the scanned tree; run each of the three commands; confirm non-zero exit and stderr names the file.

### TDD Integration Tests for US3 (write FIRST, must FAIL or be missing)

- [X] T031 [P] [US3] Add integration test `should_deploy_fail_loudly_when_shrine_manifest_has_bad_kind` to [tests/integration/deploy_test.go](../../tests/integration/deploy_test.go) using the T007 `bad-kind` fixture. Asserts `AssertFailure()` and `AssertStderrContains("typo.yaml")` (the file path) AND `AssertStderrContains("Aplication")` (the offending kind value). Covers SC-004.
- [X] T032 [P] [US3] Add integration test `should_deploy_fail_loudly_when_yaml_is_malformed` to [tests/integration/deploy_test.go](../../tests/integration/deploy_test.go) using the T006 `malformed-yaml` fixture. Asserts `AssertFailure()` and `AssertStderrContains("broken.yaml")` (covers FR-004 / "`shrine deploy` should fail with a malformed .yml and .yaml files").
- [X] T033 [P] [US3] Add integration test `should_apply_teams_fail_loudly_when_shrine_manifest_has_bad_kind` to [tests/integration/apply_test.go](../../tests/integration/apply_test.go) using the T008 `apply/bad-kind` fixture. Note the existing `ApplyTeams` today silently skips non-Team files via `Skipping %s: not a Team manifest`; after T027 it still parses every Shrine-classified file via `manifest.Parse`, so the `Aplication` kind WILL produce an error printed to stdout via the existing `Error parsing %s: %v` line. Assert that the bad-kind file's path and the kind value appear in the captured output, even if exit code stays 0 (current `ApplyTeams` semantics) — record the chosen behaviour in the test comment so future readers know it is intentional.
- [X] T034 [P] [US3] Add integration test `should_apply_file_fail_loudly_when_target_is_shrine_manifest_with_bad_kind` to [tests/integration/apply_test.go](../../tests/integration/apply_test.go). Use `apply -f` pointing at the typo'd file from T007. Assert `AssertFailure()` and `AssertStderrContains("typo")` and `AssertStderrContains("Aplication")`. (`TestApplyFileErrors` already has a similar case — extend it; do not duplicate.)
- [X] T035 [US3] Run `make test-integration -- -run 'TestDeploy/should_deploy_fail_loudly|TestApplyTeams/should_apply_teams_fail_loudly|TestApplyFile.*should_apply_file_fail_loudly'` and confirm: T031 + T032 + T034 FAIL today on `main` *only because* the test files don't yet exist; once added, they should immediately PASS on this branch (no implementation change needed) because [internal/manifest/validate.go:23](../../internal/manifest/validate.go#L23) and [internal/manifest/parser.go:66](../../internal/manifest/parser.go#L66) already produce the loud errors. Document this expected "tests written → already pass" outcome in the commit message — it is the FR-007 "no regression" guarantee in test form.

### Implementation for US3

> **No production code changes expected.** US3 is a regression-guard story: the existing `manifest.Parse` / `manifest.Validate` paths already raise the loud errors the spec demands. The Phase 2 + Phase 3 + Phase 4 refactors deliberately route shrine-classified files through these unchanged code paths (FR-007). If any of T031-T034 fail after T021 + T030, that is a regression in the refactor and the FIX is to restore the existing error-wrapping in `LoadDir` / `ApplyTeams` — NOT to add a new error path.

- [X] T036 [US3] T034 failed: `PlanSingle` in `internal/planner/plan.go` returned the bare `manifest.Parse` error without wrapping the file path. Fixed by adding `fmt.Errorf("parsing manifest %q: %w", file, err)` wrapping in `PlanSingle` (consistent with `LoadDir`). Added `TestPlanSingle_BadKind_WrapsFilePath` in `internal/planner/loader_test.go` to pin this wrapping behaviour. Also fixed `describe_test.go`, `get_test.go`, `status_test.go`, `teardown_test.go` BeforeEach blocks (pre-existing regression from Phase 1: those tests used `fixturesPath()` pointing at the full deploy testdata root which now contains `malformed-yaml/` and `bad-kind/` fixtures; updated to use `fixturesPath("team")` matching the pattern already applied to `TestDeploy`).

**Checkpoint**: SC-004 satisfied. All three commands now uniformly skip foreign files AND fail loudly on broken shrine manifests / malformed YAML.

---

## Phase 6: Polish & Cross-Cutting

- [X] T037 [P] Walk through every step of [quickstart.md](./quickstart.md) on a real Docker host. Confirm the foreign-files notice text matches what the quickstart documents; if it differs, update the quickstart (the notice text is informational, not contractual — see FR-006).
- [X] T038 [P] Run `grep -rn "filepath.WalkDir\|filepath.Walk" cmd/ internal/` and confirm `ScanDir` is the only manifest-directory walker remaining (FR-005). Any new walker discovered must be migrated or justified in a code comment with a WHY.
- [X] T039 Run the full unit + integration suite as the Constitution V gate: `go test ./... && make test-integration`. Capture the full output. Both must be green before the branch is mergeable.
- [X] T040 Run a `git diff main...HEAD -- 'specs/002-fix-routing-dir-manifest-scan/**'` final pass to confirm no `NEEDS CLARIFICATION` markers leaked into any phase-output document.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately. All T001-T008 are `[P]` and touch different fixture files.
- **Phase 2 (Foundational)**: Depends on Phase 1 (the unit tests in T009/T010 are content-only and don't need fixtures, but ScanDir tests reference temp-dir patterns mirrored in fixtures). T009 + T010 are parallel; T011 (verify-fail) blocks T012-T013; T014 (verify-pass) blocks every Phase 3+.
- **Phase 3 (US1)**: Depends on Phase 2. T015 + T016 parallel; T017 (verify-fail) blocks T018-T021. T020 may run in parallel with T018 (different files) but logical-order keeps it after.
- **Phase 4 (US2)**: Depends on Phase 3 (T021 must be green so T022/T023 can run). T022-T025 are all `[P]` (different test functions in two files). T026 (verify-fail) blocks T027-T030.
- **Phase 5 (US3)**: Depends on Phase 4 (T030 green) so the regression guard is meaningful. T031-T034 parallel; T035 (verify-classification) blocks T036.
- **Phase 6 (Polish)**: Depends on every story phase being checkpoint-complete.

### Parallel Opportunities

- T001..T008 — eight fixture-creation tasks, all independent files.
- T009 + T010 — independent test files in `internal/manifest/`.
- T015 + T016 — independent test functions in two different integration test files.
- T022 + T023 + T024 + T025 — four independent integration test cases across two test files.
- T031 + T032 + T033 + T034 — four independent regression-guard test cases.
- T037 + T038 — quickstart + grep audit don't touch each other.

---

## Parallel Example: Phase 4 (US2) test authoring

```bash
# Open four editor panes and write these test cases simultaneously:
Task: "T022 - deploy with foreign YAML in tests/integration/deploy_test.go"
Task: "T023 - deploy with non-YAML siblings in tests/integration/deploy_test.go"
Task: "T024 - apply teams with foreign YAML in tests/integration/apply_test.go"
Task: "T025 - apply -f with foreign YAML in tests/integration/apply_test.go"

# Then run them all together to confirm they fail before T027:
make test-integration -- -run 'TestDeploy/should_succeed_when_specsDir_contains|TestApplyTeams/should_apply_teams_succeed_when|TestApplyFile/should_apply_file_succeed_when'
```

---

## Implementation Strategy

### MVP (User Story 1 only)

1. Phase 1 → fixtures
2. Phase 2 → shared classifier
3. Phase 3 → `shrine deploy` succeeds on the default Traefik routing-dir layout (SC-001)
4. **STOP & VALIDATE**: This alone closes the original bug report. Operators with the Traefik plugin enabled can deploy.
5. Decide whether to ship MVP or continue to US2 / US3 in the same PR.

### Incremental Delivery

1. MVP (above) → ship.
2. Phase 4 → uniform foreign-file handling across `apply teams` and `apply -f` (FR-005).
3. Phase 5 → regression guard for loud-failure behaviour (SC-004).
4. Phase 6 → docs sweep + final integration gate.

### Why no parallel team strategy for this feature

Phases 3 → 4 → 5 share the same source files (`internal/planner/loader.go`, `internal/handler/teams.go`, both integration test files). Splitting them across developers would cause merge churn for negligible savings on a 40-task feature. One developer running phases sequentially is the right shape.

---

## Notes

- TDD is non-negotiable here per the user's explicit instruction AND Constitution V. Every test-writing task ends with a "verify FAIL" gate (T011, T017, T026, T035). Skipping that gate makes the test useless as regression coverage.
- `[P]` tasks touch different files. When two tasks both edit the same integration test file (e.g. T022 and T023 both edit `deploy_test.go`), they are still marked `[P]` because they add different `s.Test(...)` blocks — coordinate via Git, not by serialising work.
- Phase 5 deliberately has no expected production code change. That is the point of a regression-guard phase: it proves the refactors in Phases 3 + 4 did not weaken existing behaviour (FR-007).
- The foreign-files notice (FR-006) is emitted at the call-site (`LoadDir`, `ApplyTeams`), not inside `ScanDir`, so unit tests of the scanner stay deterministic. T019 + T028 add it; T020's regression test does not assert on the notice text (it is informational, not a contract).
