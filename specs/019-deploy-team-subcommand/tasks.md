# Tasks: Unified Planner Filter + `shrine deploy team <name>` Subcommand

**Input**: Design documents from `/specs/019-deploy-team-subcommand/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/planner-api.md, quickstart.md

**Tests**: Tests are INCLUDED per Principle V (Integration-Test Gate) and per Principle IV's TDD note ("we will enforce TDD so integration test files are created before the implementation code"). Integration tests are written locally but **executed in the CI pipeline on cloud** — local tasks gate on unit tests only.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing. US3 (planner refactor) is foundational and blocks US1 / US2 / US4.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: User story label (US1..US4); omitted for Setup and Polish
- All paths are repository-relative; the repo root is `/root/projects/shrine`

## Path Conventions

Go single-binary CLI rooted at `/root/projects/shrine`:

- `internal/planner/` — pure planner logic (no I/O)
- `internal/handler/` — orchestration; loads sets, invokes engine
- `cmd/` — Cobra command tree
- `tests/integration/` — black-box subprocess + Docker integration suite (`NewDockerSuite`)
- `docs/content/` — Hugo content; `cli/` is auto-generated, `guides/` and `getting-started/` are hand-written
- `AGENTS.md`, `CLAUDE.md` — repo-root agent guidance

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish baseline so regressions are detectable.

- [X] T001 Confirm clean working tree on branch `019-deploy-team-subcommand` and run baseline `go build ./...` plus `go test ./...` to record the current pass set. No code changes; documents the "green" baseline before refactor.
- [X] T002 Capture the current list of integration test files under `tests/integration/` (especially `deploy_test.go` and any `apply_*_test.go`) so the regression gates (SC-003, SC-004) are anchored to specific files. Anchors: `tests/integration/deploy_test.go` (SC-003), `tests/integration/apply_test.go` (SC-004).

---

## Phase 2: Foundational

**Purpose**: None required outside US3 — the planner refactor IS the foundation, and the workflow's "story label required" rule places it under US3 below. This phase is intentionally empty.

---

## Phase 3: User Story 3 — Planner Consolidation (Priority: P1) 🛑 BLOCKS US1 / US2 / US4

**Goal**: Replace `planner.Plan` + `planner.PlanSingle` with a single `Plan(set, store, registries, filter)`. Migrate `handler.ApplySingle` off `PlanSingle`. Delete `PlanSingle`. After this phase, the abstraction needed by US1/US2/US4 exists.

**Independent Test**: `grep -r "PlanSingle" internal/ cmd/ tests/` returns zero matches. `go test ./internal/planner/... ./internal/handler/...` passes including the new filter unit tests. Existing `tests/integration/apply_*_test.go` continues to pass under CI.

### Tests (TDD — write before implementation)

- [X] T003 [US3] Write unit tests in `internal/planner/filter_test.go` covering: (a) each constructor (`NoFilter`, `ByTeam`, `ByApp`, `ByResource`) returns the expected struct values; (b) `Filter.Validate` happy paths for each `FilterKind`; (c) `Filter.Validate` sad paths — empty name for named filters, missing entity in set, empty set, unknown owner. Assert the unknown-team error string contains both the typo and the sorted list of discovered owners (FR-008, SC-005).
- [X] T004 [P] [US3] Write unit tests in `internal/planner/plan_test.go` (new file) covering: (a) `Plan(set, store, regs, NoFilter())` produces a step set behaviorally equivalent to today's `Plan(dir, store, regs)`; (b) `Plan(..., ByApp(name))` produces a single-step result equivalent to today's `PlanSingle` for an Application file; (c) `Plan(..., ByResource(name))` same for a Resource; (d) `Plan(..., ByTeam(name))` emits only steps whose owning manifest matches; (e) team-scoped Plan still resolves cross-team `valueFrom` references that point at other teams' resources (Clarification Q1). Use t.Setenv where needed; do NOT touch the filesystem per memory rule.
- [X] T005 [P] [US3] Extend `internal/planner/loader_test.go` to cover `MergeManifest`'s public contract: Application duplicate returns `ErrDuplicateManifest`, Resource duplicate returns `ErrDuplicateManifest`, Team kind is a silent no-op, unsupported kind returns a descriptive error. Add a separate `TestNewManifestSet` asserting both maps are allocated.
- [X] T006 [P] [US3] Extend `internal/handler/apply_test.go` (creating it if absent) with cases: (a) Application file is dispatched with `ByApp(name)` filter; (b) Resource file is dispatched with `ByResource(name)`; (c) Team-kind file returns today's exact error (`"team manifests cannot be applied with --file; use 'shrine apply teams' instead"`); (d) `manifestDir == ""` builds a minimal set via `NewManifestSet + MergeManifest`. Mock the planner via a thin function-variable seam or table-drive against a real `NewManifestSet` to avoid filesystem use (per memory rule).

### Implementation

- [X] T007 [US3] Create new file `internal/planner/filter.go`: declare `FilterKind` constants (`FilterNone`, `FilterTeam`, `FilterApp`, `FilterRes`), the `Filter` struct with `Kind` + `Name` fields, and the four constructors (`NoFilter`, `ByTeam`, `ByApp`, `ByResource`). Per contracts/planner-api.md §1–§2.
- [X] T008 [US3] In `internal/planner/filter.go`, implement `Filter.Validate(set *ManifestSet) error` per contracts/planner-api.md §3, including the `discoveredOwners(set) []string` private helper (sorted, deduplicated). Error messages MUST exactly match the table in §3 so T003 assertions pass.
- [X] T009 [P] [US3] In `internal/planner/loader.go`, add `func NewManifestSet() *ManifestSet` that returns a struct with both maps allocated.
- [X] T010 [P] [US3] In `internal/planner/loader.go`, rename the private `(*ManifestSet).mapKind` to public `MergeManifest`, and declare `var ErrDuplicateManifest = errors.New("manifest already present in set")` at package scope. Wrap the existing duplicate errors so callers can use `errors.Is`. Update the in-package caller in `LoadDir` to use the new name.
- [X] T011 [US3] Refactor `internal/planner/plan.go`: replace `Plan(dir, store, registries)` with `Plan(set *ManifestSet, store state.TeamStore, registries []config.RegistryConfig, filter Filter) PlanResult`. Implement the four-branch switch per contracts/planner-api.md §4.2. Add a private helper `filterStepsByOwner(steps []PlannedStep, set *ManifestSet, owner string) []PlannedStep` for the `FilterTeam` branch.
- [X] T012 [US3] In `internal/planner/plan.go`, delete `PlanSingle` outright (no deprecation shim). `PlanTeardown` stays — it operates on deployment state, not the manifest set. Verify with `grep -r "PlanSingle" .` returning zero matches in `internal/`, `cmd/`, `tests/`.
- [X] T013 [US3] Migrate `internal/handler/apply.go`'s `ApplySingle` to the new API: parse + validate the file (unchanged), call new helper `loadSetForSingle(file, manifestDir, m)` that does `LoadDir + MergeManifest` (tolerating `ErrDuplicateManifest`) when `manifestDir != ""` or `NewManifestSet + MergeManifest` when empty. Build the filter via a switch on `m.Kind` (`ByApp` / `ByResource` / Team-rejected). Call `planner.Plan(set, b.Store.Teams, b.Cfg.Registries, filter)`. The tail (error rendering + engine call) is unchanged.
- [X] T014 [P] [US3] Migrate `internal/handler/deploy.go`'s `Deploy(b, manifestDir)` and `DryRun(out, manifestDir, store, cfg)` signatures to accept a trailing `filter planner.Filter` argument. Inside each, call `planner.LoadDir(manifestDir)` then `planner.Plan(set, ..., filter)`. The error rendering, step-count guard, and engine call are unchanged.
- [X] T015 [US3] In `cmd/deploy.go`, update the existing `deployCmd.RunE` (only) to pass `planner.NoFilter()` to `handler.Deploy` / `handler.DryRun`. Do NOT add the team subcommand yet — that's US1's task. This keeps US3 a self-contained refactor with zero CLI surface change.
- [X] T016 [US3] Local gate: run `go build ./...` and `go test ./internal/... ./cmd/... ./...` (unit tier). All previously-passing tests MUST still pass; new filter/plan/handler tests MUST pass. The integration tier runs on CI per user direction.

**Checkpoint**: After T016 passes, the planner has exactly one entry point, `PlanSingle` is gone (SC-007), and observable behavior of `shrine deploy` and `shrine apply -f` is unchanged.

---

## Phase 4: User Story 1 — Operator deploys a single team's stack (Priority: P1)

**Depends on**: US3 (Filter type + unified Plan must exist).

**Goal**: Add the `shrine deploy team <name>` Cobra subcommand. It composes `planner.ByTeam(args[0])` with the same handler code path US3 just unified.

**Independent Test**: In a multi-team specs dir, `shrine deploy team team-a` mutates only team-a's containers / state / routing; `shrine deploy` (bare) still deploys both. Verified by `tests/integration/deploy_team_test.go` (Scenarios A + B below) on CI.

### Tests (TDD)

- [X] T017 [US1] Create `tests/integration/deploy_team_test.go` using the existing `NewDockerSuite` harness. **Scenario A** — multi-team isolation: fixture specs dir contains apps owned by `team-a` and `team-b`; run `shrine deploy team team-a`; assert (i) team-a's containers exist and are running, (ii) team-b's containers were not created (or, if pre-existing, were not touched — same container IDs as before), (iii) routing files under the configured routing dir reflect only team-a. **Scenario B** — bare deploy still works: after Scenario A, run `shrine deploy` (no subcommand); assert all teams reconcile.
- [X] T018 [P] [US1] In `tests/integration/deploy_team_test.go`, add **Scenario C** — output header: assert stdout for `shrine deploy team team-a` matches `[shrine] Planning deployment for team "team-a" from: <dir>` (FR-013).

### Implementation

- [X] T019 [US1] In `cmd/deploy.go`, move `--dry-run` and `--path` from `deployCmd.Flags()` to `deployCmd.PersistentFlags()` so the team subcommand inherits them (FR-006).
- [X] T020 [US1] In `cmd/deploy.go`, introduce a private closure factory `runDeploy(filter planner.Filter) func(*cobra.Command, []string) error` that contains today's `deployCmd.RunE` body, but: (a) takes the filter as a parameter, (b) prints the team-aware header `[shrine] Planning deployment for team %q from: %s` when `filter.Kind == planner.FilterTeam`, otherwise prints today's `[shrine] Planning deployment from: %s`, (c) passes the filter to `handler.DryRun` / `handler.Deploy`.
- [X] T021 [US1] In `cmd/deploy.go`, replace `deployCmd.RunE` with `runDeploy(planner.NoFilter())`.
- [X] T022 [US1] In `cmd/deploy.go`, declare `deployTeamCmd` with `Use: "team <name>"`, `Args: cobra.ExactArgs(1)`, `Short: "Deploy only the apps and resources owned by one team"`, and `RunE: func(cmd, args) error { return runDeploy(planner.ByTeam(args[0]))(cmd, args) }`. Register it in `init()` via `deployCmd.AddCommand(deployTeamCmd)`.
- [X] T023 [US1] Add a unit test `cmd/deploy_test.go` (or extend the existing one) that asserts: (a) `shrine deploy team` with no name exits non-zero with Cobra's usage error (Edge Case: missing argument); (b) `shrine deploy team foo` does NOT call `handler.Deploy` when `Filter.Validate` would reject (this is tested in T003 at the planner layer; the cmd test only verifies wiring). Use Cobra's `SetOut`/`SetErr` capture, not the filesystem.
- [X] T024 [US1] Local gate: `go build ./...`, `go test ./cmd/...`. Build the binary; smoke `./shrine deploy team --help` manually to confirm Cobra emits the new subcommand.

**Checkpoint**: After T024, the verb exists and the unit + cmd tests pass locally. CI's `tests/integration/deploy_team_test.go` (T017, T018) verifies the end-to-end behavior.

---

## Phase 5: User Story 2 — Operator previews team-scoped deploy with `--dry-run` (Priority: P1)

**Depends on**: US1 (the `deployTeamCmd` exists and inherits `--dry-run` from US1's T019).

**Goal**: Verify `shrine deploy team <name> --dry-run` produces zero side effects and reports routing collisions, identical to bare `shrine deploy --dry-run`.

**Independent Test**: `tests/integration/deploy_team_test.go` scenarios D + E (added below) on CI confirm dry-run parity and collision detection.

### Tests

- [X] T025 [US2] Extend `tests/integration/deploy_team_test.go` with **Scenario D** — dry-run parity: take a snapshot of Docker container IDs and a checksum of the state-dir before running `shrine deploy team team-a --dry-run`; assert (i) command exits 0, (ii) Docker container set is byte-identical after, (iii) state-dir checksum is byte-identical after, (iv) routing dir is unchanged (SC-006).
- [X] T026 [US2] Extend the same file with **Scenario E** — collision under team scope: fixture where two of `team-a`'s apps declare the same routing domain; run `shrine deploy team team-a --dry-run`; assert non-zero exit and that stderr names the colliding apps (matches today's behavior for bare `deploy --dry-run` — uses the existing `DetectRoutingCollisions` path unchanged per US3).

### Implementation

US2 requires **no new implementation code**. `runDeploy` (T020) already dispatches to `handler.DryRun` when `--dry-run` is set, and `DetectRoutingCollisions` is part of the `FilterNone | FilterTeam` branch in `Plan` (T011). T025 and T026 serve as the verification gate.

- [X] T027 [US2] Local gate: `go vet ./...` to confirm no new lints from US1 changes leaked through. CI runs the new dry-run scenarios.

**Checkpoint**: After T027, the dry-run path is verified at the unit-test boundary (US3's T004) and queued for CI integration verification (T025, T026).

---

## Phase 6: User Story 4 — Operator gets a clear error when team has no manifests (Priority: P2)

**Depends on**: US3 (the unknown-team error comes from `Filter.Validate`).

**Goal**: Ensure the unknown-team error surfaces correctly through the CLI path. The error logic already exists in `Filter.Validate` (US3's T008). This phase is verification + integration coverage.

**Independent Test**: `tests/integration/deploy_team_test.go` scenarios F + G on CI confirm the typo case and the empty-dir case both exit non-zero with the expected message.

### Tests

- [X] T028 [US4] Extend `tests/integration/deploy_team_test.go` with **Scenario F** — typo case: specs dir contains apps owned by `team-a` only; run `shrine deploy team markting`; assert (i) exit code is non-zero, (ii) stderr contains `team "markting" not found`, (iii) stderr contains `team-a` (the known-teams list), (iv) zero Docker side effects.
- [X] T029 [US4] Extend the same file with **Scenario G** — empty specs dir: temp dir contains no manifests; run `shrine deploy team anything`; assert (i) exit code is non-zero, (ii) stderr contains `specs directory contains no Application or Resource manifests`, (iii) zero Docker side effects.

### Implementation

- [X] T030 [US4] Confirm `Filter.Validate`'s error strings (implemented in T008) match the assertions in T028 and T029 verbatim. If they don't, adjust the messages in `internal/planner/filter.go` and re-run T003. This is the only code touch US4 may need.

**Checkpoint**: After T030, all four user stories are complete at the code level. Remaining work is documentation + polish.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Goal**: Documentation parity (FR-012), graph freshness, final unit gate.

### Documentation

- [X] T031 [P] Run `make docs-gen-cli` at the repo root. This regenerates `docs/content/cli/deploy.md` (updated SEE ALSO list) and creates `docs/content/cli/deploy_team.md` automatically from the live Cobra tree. Commit both files. Verify with `git diff --exit-code docs/content/cli/` returning clean after a second run (idempotent).
- [X] T032 [P] Edit `docs/content/getting-started/quick-start.md` to add a short callout (3–5 lines) after the existing `shrine deploy` example: introduce `shrine deploy team <name>` and link to the new guide page (T033). Do not rewrite the quick-start.
- [X] T033 Create `docs/content/guides/team-scoped-deploy.md` (~80–120 lines, density similar to `docs/content/guides/custom-registries.md`). Sections: (1) When to use vs. bare `shrine deploy`; (2) How team scoping works — load full set as context, emit steps only for owner-match (FR-007); (3) Cross-team dependencies — manifest-only check, deploy the owning team separately (Clarification Q1, FR-009); (4) Dry-run parity (FR-011); (5) Error UX — typo case and empty-dir case (FR-008).
- [X] T034 [P] Edit `docs/content/guides/_index.md` to add a link entry for `team-scoped-deploy.md` so it shows up in Hugo's sidebar / index.
- [X] T035 [P] Edit `AGENTS.md`'s CLI quick-reference table at the repo root to add a row: `shrine deploy team <name>` — Deploys only one team's apps and resources. Constitution mandates `AGENTS.md` stays in sync with CLI changes.
- [X] T036 Run `make docs` (Hugo build) to regenerate `docs/public/`. Commit the regenerated output — the repo serves GitHub Pages from `docs/public/`, so it must stay in sync (memory: docs served at `/shrine/`).

### Maintenance

- [X] T037 Run `graphify update .` at the repo root to refresh `graphify-out/` with the new files (`filter.go`, `deploy_team.md`, `team-scoped-deploy.md`) and the deleted `PlanSingle`. AST-only; no API cost.
- [X] T038 Final local gate: `go test ./...` and `go vet ./...` both green. Confirm `grep -r "PlanSingle" internal/ cmd/ tests/` returns zero matches (SC-007). The integration tier is on CI per user direction.

**Checkpoint**: Feature is complete. CI's integration tier (all scenarios A–G plus pre-existing `deploy_test.go` and `apply_*_test.go`) is the merge gate.

---

## Dependencies & Story Completion Order

```text
Phase 1 (Setup)
   ↓
Phase 3 — US3 (Planner Refactor)  ←─── BLOCKS all CLI work
   ↓
   ├──→ Phase 4 — US1 (deploy team verb)
   │       ↓
   │       └──→ Phase 5 — US2 (dry-run parity)
   │
   └──→ Phase 6 — US4 (unknown-team error UX)   ◀── can run in parallel with US1/US2
                                                    once US3 lands
   ↓
Phase 7 (Polish & Docs)
```

**Hard dependencies**:

- US1, US2, US4 all require US3 (no `Filter` type otherwise).
- US2 requires US1 (the `deployTeamCmd` must exist to dry-run against).
- T032, T033, T034, T035, T036 (docs) require the CLI to be wired (T022); they describe behavior that must already exist.
- T037 (graphify) runs after all code + docs are committed so the graph captures the final state.

**MVP** = US3 + US1 only (T001–T024). That delivers the planner consolidation and the new `shrine deploy team` verb. US2 (dry-run integration verification) and US4 (error-UX verification) add coverage but no new operator-visible behavior beyond what US3+US1 already provide.

---

## Parallel Execution Opportunities

**Within US3** (after T002):

```text
T003 (filter_test.go)        ─┐
T004 (plan_test.go)          ─┤   different files,
T005 (loader_test.go)        ─┤   no dependencies →
T006 (apply_test.go)         ─┘   all four [P] together

then sequentially in filter.go:
T007 (filter.go declarations) → T008 (filter.Validate)

T009 (NewManifestSet)        ─┐   different sections of loader.go
T010 (MergeManifest rename)  ─┘   — careful, same file: run sequentially OR coordinate via a single PR-staging edit

T011 (Plan refactor)         ─┐   same file (plan.go) — sequential
T012 (delete PlanSingle)     ─┘

T013 (apply.go migration)    ─┐
T014 (deploy.go migration)   ─┘   different files — [P]

T015 (cmd/deploy.go RunE)        ← depends on T014
T016 (local unit gate)            ← depends on everything above
```

**Within Polish (Phase 7)** — most docs tasks are independent files:

```text
T031 (auto-gen CLI ref) ─┐
T032 (quick-start)      ─┤   four different files — all [P]
T034 (guides _index)    ─┤
T035 (AGENTS.md)        ─┘

T033 (new guide page)        ← must precede T036 because the page must exist when Hugo builds
T036 (make docs)             ← depends on T031 + T033
T037 (graphify update)       ← depends on all docs committed
T038 (final unit gate)       ← last
```

---

## Implementation Strategy

**MVP-first**: Land US3 + US1 (T001–T024) as a first commit / PR. At that point a user can run `shrine deploy team <name>` and the planner is consolidated. US2, US4, and Polish can land as a second PR if needed, or in the same one if scope stays manageable.

**CI vs. local split**: per user direction, integration tests run on the cloud CI pipeline. All `tests/integration/*.go` tasks (T017, T018, T025, T026, T028, T029) ship as code but are not gated locally. Local gates are `go build ./...`, `go test ./...` (unit tier), `go vet ./...`. CI's pre-existing `make test-integration` target picks up the new file automatically.

**TDD discipline (Principle V)**: test tasks precede implementation tasks within every user story phase. The order in this file reflects that. Reviewers should reject implementation commits that arrive before their corresponding test commits.

**Refactor safety (SC-003, SC-004)**: US3's success is gated by the *unchanged* integration tests for `shrine deploy` and `shrine apply -f` passing on CI. These tests live in `tests/integration/deploy_test.go` and `tests/integration/apply_*_test.go`; T002 captures them as the regression anchor. Any task that would require modifying those files indicates a behavioral regression and must be re-evaluated.

**Docs as a gate, not an afterthought (FR-012)**: T031–T036 are not optional polish; they're constitutional (Principle II — CLI self-documenting — and the Governance line about `AGENTS.md`). A PR landing US1's code without T035 (`AGENTS.md`) is incomplete.

---

## Task Count & Coverage

- **Total tasks**: 38
- **Setup**: 2 (T001–T002)
- **US3 (P1, foundational)**: 14 (T003–T016)
- **US1 (P1)**: 8 (T017–T024)
- **US2 (P1)**: 3 (T025–T027)
- **US4 (P2)**: 3 (T028–T030)
- **Polish**: 8 (T031–T038)

**Parallel opportunities**: 14 tasks tagged [P] — see sections above.

**Independent test criteria** (matches spec.md Independent Test sections):

- US3: `grep -r PlanSingle` returns zero; `go test ./internal/...` green.
- US1: CI scenarios A + B + C on `tests/integration/deploy_team_test.go`.
- US2: CI scenarios D + E on the same file.
- US4: CI scenarios F + G on the same file.
