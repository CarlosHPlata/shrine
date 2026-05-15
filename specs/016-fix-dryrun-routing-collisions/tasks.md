---

description: "Task list for feature 016: Detect Routing Domain Collisions in `shrine deploy --dry-run`"
---

# Tasks: Detect Routing Domain Collisions in `shrine deploy --dry-run`

**Input**: Design documents from `/specs/016-fix-dryrun-routing-collisions/`
**Prerequisites**: plan.md, spec.md (both required), research.md, data-model.md, contracts/, quickstart.md (all present)

**Tests**: TDD is REQUIRED for this feature per Constitution Principle V ("integration test files are created before the implementation code"). Test tasks below are mandatory, not optional.

**Organization**: Tasks are grouped by user story so each story's gate can be exercised independently. Note that US1 and US2 are satisfied by a single code change (moving `DetectRoutingCollisions` into `planner.Plan`); US2's tasks therefore focus on test/verification that proves the change is backend-independent, not on additional production code.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on incomplete tasks)
- **[Story]**: User story this task serves (US1, US2)
- File paths are exact

## Path Conventions

Single Go module at `github.com/CarlosHPlata/shrine`. Source under `internal/`, integration tests under `tests/integration/`, fixtures under `tests/testdata/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm baseline before any edits.

- [X] T001 Confirm working tree is on branch `016-fix-dryrun-routing-collisions` and `go build ./...` succeeds at repo root.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Create the manifest fixture both user stories depend on.

**⚠️ CRITICAL**: No user story tests can be authored until this fixture exists.

- [X] T002 Create test fixture directory `tests/testdata/deploy/routing-collision/` containing two `apiVersion: shrine/v1, kind: Application` manifests (`app-a.yaml`, `app-b.yaml`) both under team `shrine-deploy-test`, both declaring `spec.routing.domain: collision.example.com`, both using `spec.image: nginx` and `spec.port: 80`. Do NOT add a `Team` manifest — `TestDeploy` already applies `tests/testdata/deploy/team/` in `BeforeEach`, and adding a team to this fixture would cause double-apply errors.

**Checkpoint**: Fixture exists; both user story phases can now proceed.

---

## Phase 3: User Story 1 — `--dry-run` surfaces duplicate-domain errors before real deploy (Priority: P1) 🎯 MVP

**Goal**: `shrine deploy --dry-run` exits non-zero and prints a routing-collision diagnostic when the target manifest set contains duplicate `routing.domain` values, with content identical to what `shrine deploy` produces for the same input.

**Independent Test**: Run `shrine deploy --dry-run --path tests/testdata/deploy/routing-collision/ --state-dir <tmp>` against this branch's binary. Expect non-zero exit, output containing `routing collision`, and both `shrine-deploy-test/app-a` and `shrine-deploy-test/app-b` named in the diagnostic.

### Tests for User Story 1 (TDD — author BEFORE implementation; CI runs integration scenario)

- [X] T003 [US1] **DROPPED.** A meaningful unit test for "Plan invokes DetectRoutingCollisions" requires exercising `planner.Plan`, which reads a directory. Per `feedback_unit_tests_no_filesystem`, filesystem-touching tests belong in integration coverage. Algorithm correctness for `DetectRoutingCollisions` is already covered by the existing in-memory tests in `internal/planner/collisions_test.go`; wiring is proven by T004 (run in CI).
- [X] T004 [US1] Add integration scenario `should report routing collision on dry-run when two apps share a domain` to `TestDeploy` in `tests/integration/deploy_test.go`. The scenario runs `tc.Run("deploy", "--dry-run", "--path", fixturesPath("routing-collision"), "--state-dir", tc.StateDir)` and asserts: non-zero exit (`AssertFailure`), stdout contains `routing collision`, stdout contains `shrine-deploy-test/app-a`, stdout contains `shrine-deploy-test/app-b`. All assertion helpers already exist in `tests/integration/testutils/assert_general.go` (`AssertFailure`, `AssertOutputContains`). Authored against the new fixture from T002. **Not run locally** (per current /speckit-implement scope); CI exercises it.

### Implementation for User Story 1

- [X] T005 [US1] In `internal/planner/plan.go::Plan()`, invoke `DetectRoutingCollisions(set)` immediately after the `Resolve(...)` call returns no validation errors and before `Order(set)`. If `DetectRoutingCollisions` returns non-nil, return `PlanResult{ValidationErr: []error{err}, ManifestSet: set}` and skip `Order`. Do NOT change any function signatures, exported types, or other behavior.
- [X] T006 [US1] In `internal/handler/deploy.go::Deploy()`, delete the entire `if routing != nil { if err := planner.DetectRoutingCollisions(result.ManifestSet); err != nil { return err } }` block (currently around lines 124-128). The `routing` variable is still required by the subsequent `local.NewLocalEngine` call — leave that usage intact.
- [X] T007 [US1] Run `go test ./internal/planner/... ./internal/handler/...` and confirm zero regressions. (T003 dropped — no new unit test to gate on.)
- [X] T008 [US1] **SKIPPED locally per user instruction.** CI will run `make test-integration` and exercise T004. Pre-merge expectation: T004 PASSES; no existing scenario regresses.

**Checkpoint**: US1 fully functional. Dry-run and real deploy emit identical collision diagnostics; both fail closed on duplicate domains.

---

## Phase 4: User Story 2 — Collision detection runs independently of routing backend wiring (Priority: P2)

**Goal**: The collision check is a property of the manifests, not of any backend. Detection must run identically whether or not a routing backend (Traefik) is configured.

**Independent Test**: The collision diagnostic from `shrine deploy --dry-run` against the duplicate-domain fixture is unchanged when run against an environment with no Traefik configuration. (Today the integration suite's `TestDeploy` already runs without `--traefik-config`, so US1's test inherently proves US2 — these tasks make that proof explicit and lock the property in.)

### Verification for User Story 2

- [X] T009 [US2] **DROPPED.** Backend independence is provable directly from `planner.Plan`'s Go signature — `Plan(dir string, store state.TeamStore, registries []config.RegistryConfig) PlanResult` accepts no backend interface. A "signature lock-in" test would compile-pass trivially and add no value. This is a code-review property, not a test.
- [X] T010 [US2] Run `grep -rn "DetectRoutingCollisions" internal/handler/` from repo root and confirm zero results — proves the redundant call site from T006 was fully removed and the handler is no longer entangled with collision logic.
- [X] T011 [US2] **SKIPPED locally per user instruction.** The integration test T004, run by CI in the suite's default no-Traefik configuration, is itself proof that collision detection works without backend wiring. PR description should call out this property.

**Checkpoint**: US1 and US2 both verified. The collision check is owned by the planner, fires for every dry-run, and depends on nothing outside the manifest set.

---

## Phase 5: Polish & Cross-Cutting Concerns

- [X] T012 [P] Run `graphify update .` at repo root to refresh `graphify-out/` after the code changes (per project `CLAUDE.md`).
- [X] T013 [P] Run `go test ./...` (non-integration) at repo root and confirm zero regressions across the codebase.
- [X] T014 [P] **SKIPPED locally per user instruction.** `make test-integration` is CI's responsibility for this PR. Pre-merge expectation: all existing scenarios continue to pass and the new T004 scenario passes.
- [X] T015 **SKIPPED locally per user instruction** (involves running the binary against a manual fixture; reviewer can follow `quickstart.md` manually).
- [X] T016 Confirm `AGENTS.md` requires no updates (this fix changes no CLI surface).

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: independent.
- **Phase 2 (Foundational)**: depends on Phase 1. Blocks all subsequent test authoring.
- **Phase 3 (US1)**: depends on Phase 2. Implements the planner edit; the same edit also satisfies US2.
- **Phase 4 (US2)**: depends on Phase 3's implementation tasks (T005, T006). US2's tests can be *authored* in parallel with US1's (they touch different test cases), but their *passing* requires the production code from Phase 3.
- **Phase 5 (Polish)**: depends on Phases 3 and 4 being complete.

### Within Phase 3 (US1)

- T003 and T004 (test authoring) MUST be authored and observed FAILING before T005/T006 (implementation) — TDD per Principle V.
- T005 must precede T006 (T006 removes a redundant call site whose duplication is only "redundant" once T005 lands).
- T007 and T008 (test re-runs) come after T005 and T006.

### Within Phase 4 (US2)

- T009 and T010 are independent of each other and can be done in parallel.
- T011 simply re-asserts that the existing T004 still passes; it is a notation/PR-description task.

### Parallel Opportunities

- T003 (planner unit test) and T004 (integration scenario) — different files, can be written in parallel.
- T009 (signature lock-in test) and T010 (grep verification) — different files, parallel.
- T012, T013, T014 (graphify update, unit tests, integration tests) — all read-only / additive against finished code; can run in parallel.

---

## Parallel Example: User Story 1 test authoring

```bash
# After Phase 2 (fixture exists), open two terminals/editors and write the failing tests in parallel:
Task: "Write TestPlan_ReportsRoutingCollisionsViaValidationErr in internal/planner/collisions_test.go"
Task: "Write 'routing collision dry-run' scenario in tests/integration/deploy_test.go"

# Then verify both fail on unmodified production code:
go test ./internal/planner/...
make test-integration  # or the focused -run flag from T008
```

---

## Implementation Strategy

### MVP (US1 Only)

1. Phase 1 (T001) — confirm baseline.
2. Phase 2 (T002) — create the fixture.
3. Phase 3 (T003-T008) — TDD the planner edit; observe red → green.
4. **STOP and VALIDATE**: at this point dry-run and real deploy emit identical collision diagnostics. The original bug from issue #21 is closed.

### Incremental Delivery

US2's tasks (T009-T011) add explicit lock-in for the backend-independence property. They can ship in the same PR as US1 (recommended — single small bug-fix PR) or in a follow-up if scope discipline demands it. Given the trivial size of US2's tasks (one signature-lock test plus one grep verification), bundling is the natural choice.

### Polish

Polish tasks (T012-T016) close out the work: graph refresh, full test gates, manual quickstart walkthrough. None is gated on the others; run them in parallel where they read disjoint state.

---

## Notes

- **TDD is mandatory here** (Principle V). Resist the urge to write the implementation first.
- **Single commit-able unit**: the recommended commit boundaries are (a) fixture (T002), (b) failing tests (T003, T004), (c) planner edit + handler cleanup (T005, T006), (d) verification (T007, T008). Bundling all four into one PR is acceptable given the small surface.
- **No CLI flags added.** No manifest schema changes. The fix is purely behavioral, scoped to the planner/handler boundary.
- **Diagnostic content is unchanged.** Only the *path* by which the diagnostic reaches the user is consolidated. Tooling parsing the existing collision string format is unaffected; tooling parsing exit codes is unaffected (collision was already a non-zero exit on real deploy and remains so).
