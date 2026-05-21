---
description: "Task list for feature 020-infer-valuefrom-deps"
---

# Tasks: Infer Implicit Deploy-Order Dependencies from Same-Owner valueFrom References

**Input**: Design documents from `/specs/020-infer-valuefrom-deps/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/enrichment-api.md, quickstart.md

**Tests**: Tests are REQUIRED for this feature — Principle V of the constitution mandates an integration-test gate and explicit TDD ("integration test files are created before the implementation code"). Unit tests follow TDD conventions where practical.

**Organization**: Tasks are grouped by user story so each story can be implemented and tested independently. The shared enrichment machinery (interface, chain, error type, the `applyEnrichmentRule` helper that contains the fail-fast same-team check, the `Plan()` wiring, the dry-run formatter, the handler error propagation) sits in Phase 2 Foundational because all three user stories depend on it. Each user story then ships its concrete rule wrapper (US1, US2) or its end-to-end failure-mode scenarios (US3).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

Single-project Go CLI. Source under `internal/planner/`, `internal/handler/`, `internal/manifest/`. Unit tests live next to their source files (`*_test.go`). Integration tests live under `tests/integration/` and run against the real shrine binary with `NewDockerSuite` (Principle V).

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the empty source files so subsequent tasks have stable file paths to edit.

- [X] T001 Create empty Go source files `internal/planner/enrich.go` and `internal/planner/enrich_valuefrom.go` with `package planner` declarations and import blocks for `github.com/CarlosHPlata/shrine/internal/manifest` and `sort`. Confirm the new files compile (`go build ./internal/planner/...`).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: All shared enrichment machinery — types, helpers, chain composer, planner wiring, dry-run formatter, handler error propagation. The fail-fast same-team check (FR-010) lives in the shared `applyEnrichmentRule` helper here so US1, US2, and US3 all inherit it.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

### Foundational tests (write FIRST per TDD — these MUST fail before T006–T016 land)

- [X] T002 [P] Write failing unit tests for `parseValueFromRef` in `internal/planner/enrich_valuefrom_test.go` covering: `resource.<name>.<output>` → ok with `Kind=="resource"`; `application.<name>.<output>` → ok with `Kind=="application"`; `vault:proj/env/key` → `(_, false)`; empty string → `(_, false)`; `resource.foo` (two parts) → `(_, false)`; `resource..output` (empty middle) → `(_, false)`; unknown prefix `secret.x.y` → `(_, false)`.

- [X] T003 Write failing unit tests for `copyManifestSetShallow`, `cloneApplicationWithDeps`, `hasExplicitDependency` in `internal/planner/enrich_test.go`. Cover: shallow copy yields a new `*ManifestSet` with fresh maps but pointer-shared values; `cloneApplicationWithDeps` with `len(extra)==0` returns the same pointer; with `extra` returns a new pointer whose `Spec.Dependencies` is the original slice plus appended entries, with the original slice unchanged; `hasExplicitDependency` returns true on `(Kind,Name)` match ignoring `Owner`.

- [X] T004 Write failing unit tests for `ChainEnrich` in `internal/planner/enrich_test.go`. Cover: empty rule chain returns `(shallowCopy, nil, nil)`; two-rule chain threads the set so rule 2 observes rule 1's added edges (cross-rule dedup); first rule returning a non-nil error short-circuits — second rule is NOT invoked; success-path idempotence (running the chain twice on the input produces edges identical to running it once); failure-path idempotence (running on the same failing input twice produces an `*EnrichmentError` with identical structured fields).

- [X] T005 Write failing unit tests for `applyEnrichmentRule` failure paths in `internal/planner/enrich_test.go` using minimal fake `lookupOwner` and `parseFor` callbacks. Cover (FR-010, US3 AS1): cross-team reference with no explicit dep → returns `*EnrichmentError{Kind: ErrCrossTeamOrUnresolvedValueFrom, ...}` with `Steps == nil`; absent-target reference with no explicit dep → same error (Q2 clarification); cross-team reference WITH a matching `(Kind, Name)` explicit dep → succeeds, no error, no inferred edge added (FR-010 second clause, US3 AS2); deterministic first-error across runs when multiple bad refs exist — assert the same offending consumer name + env var name is reported on repeated calls (FR-013 failure-path determinism, Q3 clarification).

### Foundational implementation (TDD — only start once T002–T005 are written and failing)

- [X] T006 Define types in `internal/planner/enrich.go`: the `Enricher` interface (`Enrich(*ManifestSet) (*ManifestSet, error)`); the `ManifestRef` struct (`Kind`, `Name`, `Owner string`); the `InferredEdge` struct (`Consumer`, `Target ManifestRef`, `EnvVar string`); the `EnrichmentError` struct with all fields from data-model.md §New types; the `EnrichmentErrorKind` string type and the `ErrCrossTeamOrUnresolvedValueFrom` constant; the `Error()` method on `*EnrichmentError` returning the exact format specified in contracts/enrichment-api.md §2.1.

- [X] T007 Implement the helper functions in `internal/planner/enrich.go`: `copyManifestSetShallow(set *ManifestSet) *ManifestSet`; `cloneApplicationWithDeps(app *manifest.ApplicationManifest, extra []manifest.Dependency) *manifest.ApplicationManifest` (returns the input pointer if `len(extra) == 0`); `hasExplicitDependency(deps []manifest.Dependency, kind, name string) bool`. T003 unit tests must pass after this task.

- [X] T008 [P] Implement `parseValueFromRef(s string) (valueFromRef, bool)` and the private `valueFromRef` struct in `internal/planner/enrich_valuefrom.go`. Recognize exactly the `resource.<name>.<output>` and `application.<name>.<output>` grammars; return `(_, false)` for vault, literal, malformed, or unknown-prefix inputs (FR-011). T002 unit tests must pass after this task.

- [X] T009 Implement `applyEnrichmentRule(set *ManifestSet, targetKind string, lookupOwner func(string) (string, bool), parseFor func(string) (valueFromRef, bool)) (*ManifestSet, []InferredEdge, error)` in `internal/planner/enrich_valuefrom.go`. Iterate `set.Applications` in sorted-by-name order (Decision 8). For each Application, scan `Spec.Env` in declaration order. For each env var: call `parseFor`; if `(_, false)`, skip silently. If `hasExplicitDependency(app.Spec.Dependencies, targetKind, ref.Name)` is true, skip the env var (explicit wins; no edge, no failure check). Otherwise call `lookupOwner(ref.Name)`: if `!exists || owner != app.Metadata.Owner`, return `(nil, nil, &EnrichmentError{Kind: ErrCrossTeamOrUnresolvedValueFrom, ...})` immediately (fail-fast). On a successful same-owner match, append the inferred edge and replace the app in the working set via `cloneApplicationWithDeps`. T005 unit tests must pass after this task.

- [X] T010 Implement `ChainEnrich(set *ManifestSet, rules ...Enricher) (*ManifestSet, []InferredEdge, error)` in `internal/planner/enrich.go`. Start by calling `copyManifestSetShallow(set)`. Loop over rules, threading the current set through each. If any rule returns a non-nil error, return `(nil, nil, err)` immediately. Aggregate `[]InferredEdge` across rules (each rule's edges appended to the running slice). T004 unit tests must pass after this task.

- [X] T011 Add empty struct stubs `enrichValueFromResource{}` and `enrichValueFromApplication{}` with no-op `Enrich(set *ManifestSet) (*ManifestSet, error)` methods returning `(set, nil)` in `internal/planner/enrich_valuefrom.go`. Implement `DefaultEnrichers() []Enricher` in `internal/planner/enrich.go` returning `[]Enricher{enrichValueFromResource{}, enrichValueFromApplication{}}` in that fixed order. Rule bodies are filled in US1 (T019) and US2 (T024); the stubs let `Plan()` integrate now.

- [X] T012 [P] Add the `InferredEdges []InferredEdge` field to the `PlanResult` struct in `internal/planner/plan.go`. Add a one-line doc comment per data-model.md.

- [X] T013 Wire `ChainEnrich(set, DefaultEnrichers()...)` into `planner.Plan()` in `internal/planner/plan.go`, placed strictly between the existing `Resolve(...)` call and the existing filter switch (per FR-008 + Decision 1). On non-nil error from `ChainEnrich`, return `PlanResult{Error: err}` with `Steps`, `ManifestSet`, and `InferredEdges` all `nil`. On success, replace the working set with the enriched one and populate `PlanResult.InferredEdges`.

- [X] T014 [P] Implement `formatDeployPlan(steps []PlannedStep, set *ManifestSet, edges []InferredEdge) string` in a new file `internal/handler/deploy_plan_format.go`. Render the exact format documented in contracts/enrichment-api.md §2.2: header `Deploy order:`, then per-step `  <ordinal>. <Kind>:<Name>`; for steps with non-empty `Spec.Dependencies`, emit a `       depends on:` block listing deps in the order they appear in `Spec.Dependencies`; tag inferred deps with ` (inferred from env <NAME>)` where the env var name comes from `edges` matched by `(Consumer.Name, Target.Kind, Target.Name)`.

- [X] T015 Wire `formatDeployPlan` into `internal/handler/dryrun.go`. After `Plan()` succeeds (Error == nil), write `formatDeployPlan(result.Steps, result.ManifestSet, result.InferredEdges)` to the command's stdout before calling `engine.ExecuteDeploy`. On Plan() failure, do not render the summary — fall through to the existing error path that prints `result.Error.Error()` to stderr with non-zero exit.

- [X] T016 Verify error propagation in `internal/handler/deploy.go`, `internal/handler/dryrun.go`, `internal/handler/apply_single.go`: confirm each path checks `result.Error != nil` and writes `result.Error.Error()` to ErrOut + returns a non-zero exit. Add or extend a unit test in `internal/handler/deploy_test.go` (or the equivalent existing test file) that constructs a fake `PlanResult{Error: &planner.EnrichmentError{...}}` and asserts the error message lands on stderr with a non-zero exit code.

**Checkpoint**: Foundation ready. `Plan()` runs the chain (with no-op rule bodies), the failure path is wired end-to-end, the dry-run summary header renders. User story work can now begin.

---

## Phase 3: User Story 1 — Application sequenced after same-team Resource (Priority: P1) 🎯 MVP

**Goal**: Filling in the `enrichValueFromResource` rule body so the planner orders a same-team `Resource` strictly before a same-team `Application` whose env contains `valueFrom: resource.<X>.<output>`. This is the original bug fix.

**Independent Test**: Place a same-owner Application + Resource pair in a fresh specs directory where the Application's env uses `valueFrom: resource.<resource-name>.<output>` and the Application has no `spec.dependencies` block. Run `shrine deploy team <owner>`. The plan emits the Resource step before the Application step (US1 AS1). Run `shrine deploy team <owner> --dry-run`. The Application step shows its dependency on `Resource:<X>` tagged `(inferred from env DB_CONNECTION_URL)` (US1 AS2). Add an explicit `spec.dependencies` entry for the same Resource: the plan is identical (no duplicate edge) and the explicit `owner` is preserved (US1 AS3).

### Tests for US1 (TDD — write FIRST)

- [X] T017 [P] [US1] Write failing integration test `TestDeployTeam_SequencesAppAfterSameTeamResource` in `tests/integration/deploy_team_infer_test.go` using `NewDockerSuite`. Set up the `ops_bot` Application + `ops-bot-db` Resource from `quickstart.md` Part A. Run `shrine deploy team ops_bot --dry-run` as a subprocess. Assert: stdout contains `Deploy order:`, `Resource:ops-bot-db` appears before `Application:ops-bot`, and the `Application:ops-bot` step's `depends on:` block lists `Resource:ops-bot-db (inferred from env DB_CONNECTION_URL)`. Add a second assertion in the same test (or a sibling test) that runs the real `shrine deploy team ops_bot`, asserts a clean Docker rollout, and asserts `git status specs/` reports no diff (SC-006).

- [X] T018 [P] [US1] Write failing unit tests for `enrichValueFromResource` in `internal/planner/enrich_valuefrom_test.go`. Cover: same-team resource ref adds the expected inferred edge with `EnvVar` populated from declaration order; cross-team resource ref with no explicit dep raises `*EnrichmentError`; absent-target resource ref with no explicit dep raises `*EnrichmentError`; explicit `spec.dependencies` entry with matching `(Kind, Name)` short-circuits the failure check and skips the inferred edge; an env var with `valueFrom: application.X.Y` is left to the application rule (this rule does NOT touch it); vault and literal env vars are silently skipped (no edge, no error).

### Implementation for US1

- [X] T019 [US1] Fill in `enrichValueFromResource.Enrich(set *ManifestSet)` in `internal/planner/enrich_valuefrom.go`. Replace the no-op stub with a single call to `applyEnrichmentRule(set, manifest.ResourceKind, lookupResourceOwner(set), parseResourceRef)` where `lookupResourceOwner(set)` returns a closure reading from `set.Resources` and `parseResourceRef(s)` calls `parseValueFromRef(s)` and filters to refs where `Kind == "resource"`. T018 unit tests must pass after this task.

- [X] T020 [US1] Verify the integration test from T017 passes end-to-end (`go test -tags integration ./tests/integration/ -run TestDeployTeam_SequencesAppAfterSameTeamResource`). If the dry-run summary text differs from the contract's expected format, fix `formatDeployPlan` in `internal/handler/deploy_plan_format.go` rather than the test.

**Checkpoint**: US1 ships — the original bug is fixed. MVP achieved. The platform can be deployed in this state and the `shrine deploy team ops_bot` reproduction case from the spec now orders correctly.

---

## Phase 4: User Story 2 — App-to-app inference within a team (Priority: P2)

**Goal**: Filling in the `enrichValueFromApplication` rule body so a same-team Application that reads a built-in value from another same-team Application via `valueFrom: application.<other>.<built-in>` is deployed strictly after the producer.

**Independent Test**: Set up two same-owner Applications A and B where A's env contains `valueFrom: application.B.<built-in>` and A has no `spec.dependencies`. Run `shrine deploy team <owner>`. The plan lists `Application:B` strictly before `Application:A` (US2 AS1). The dry-run lists `Application:B` tagged as inferred from the relevant env var (US2 AS2).

### Tests for US2 (TDD — write FIRST)

- [X] T021 [P] [US2] Write failing integration test `TestDeployTeam_SequencesAppAfterSameTeamApplication` in `tests/integration/deploy_team_infer_test.go`. Set up two `ops_bot`-owned Applications where one reads the other's built-in hostname via `valueFrom: application.<producer>.HOST` and has no explicit deps. Run `shrine deploy team ops_bot --dry-run`. Assert the producer's step ordinal is strictly less than the consumer's; assert the consumer's `depends on:` block tags the producer as `(inferred from env <NAME>)`.

- [X] T022 [P] [US2] Write failing unit tests for `enrichValueFromApplication` in `internal/planner/enrich_valuefrom_test.go`. Cover: same-team app-to-app reference adds the expected inferred edge; cross-team app-to-app reference with no explicit dep raises `*EnrichmentError`; absent-target app-to-app reference raises `*EnrichmentError`; explicit `spec.dependencies` entry short-circuits the failure check; an env var with `valueFrom: resource.X.Y` is left to the resource rule (this rule does NOT touch it).

### Implementation for US2

- [X] T023 [US2] Fill in `enrichValueFromApplication.Enrich(set *ManifestSet)` in `internal/planner/enrich_valuefrom.go`. Replace the no-op stub with a single call to `applyEnrichmentRule(set, manifest.ApplicationKind, lookupApplicationOwner(set), parseApplicationRef)` where `lookupApplicationOwner(set)` returns a closure reading from `set.Applications` and `parseApplicationRef(s)` calls `parseValueFromRef(s)` and filters to refs where `Kind == "application"`. T022 unit tests must pass after this task.

- [X] T024 [US2] Verify the integration test from T021 passes end-to-end (`go test -tags integration ./tests/integration/ -run TestDeployTeam_SequencesAppAfterSameTeamApplication`).

**Checkpoint**: US2 ships — both production enrichment rules are active. Any same-team `valueFrom` reference (resource or application) is now inferred as a deploy-order edge.

---

## Phase 5: User Story 3 — Cross-team valueFrom without explicit dependency fails the deploy (Priority: P3)

**Goal**: Demonstrate end-to-end that a `valueFrom` reference whose target is not same-team — whether truly cross-team or absent from the loaded set — fails the planner with the FR-010 error, unless an explicit `spec.dependencies` entry covers the reference. The failure logic itself is implemented in Foundational (T009) and exercised through both rules (US1/US2). US3 is the integration-level verification of that behaviour plus the determinism guarantee.

**Independent Test**: Set up a Resource owned by `team-b` with `metadata.access` granting `team-a`, and an Application owned by `team-a` whose env contains `valueFrom: resource.<team-b-resource>.<output>` and no explicit dep (US3 AS1). Run `shrine deploy team team-a`. The planner fails with the `enrichment: app "..." env "..." references resource "..." which is not owned by team "..."` error on stderr, non-zero exit, and no Docker side-effects. Add the explicit `spec.dependencies` entry — re-run; the deploy succeeds and the dry-run summary shows the dep without an `(inferred ...)` tag (US3 AS2).

### Tests for US3 (TDD — write FIRST)

- [X] T025 [P] [US3] Write failing integration test `TestDeployTeam_FailsOnCrossTeamValueFromWithoutExplicitDep` in `tests/integration/deploy_team_infer_test.go`. Set up the `team_b`-owned `shared-cache` Resource (with `access: [ops_bot]`) and an `ops_bot`-owned Application with `valueFrom: resource.shared-cache.HOST` and no explicit deps. Run `shrine deploy team ops_bot --dry-run`. Assert non-zero exit; assert stderr contains the exact `enrichment: app "ops-bot" env "CACHE_HOST" references resource "shared-cache.HOST" which is not owned by team "ops_bot"; add an explicit spec.dependencies entry (kind: Resource, name: shared-cache) to declare this dependency` line; assert no `[DOCKER]` lines appear on stdout; assert no Docker containers / networks / volumes were created (use the suite's leftover-state assertion).

- [X] T026 [P] [US3] Write failing integration test `TestDeployTeam_SucceedsOnCrossTeamValueFromWithExplicitDep` in `tests/integration/deploy_team_infer_test.go`. Same setup as T025 but with an explicit `spec.dependencies: [{kind: Resource, name: shared-cache, owner: team_b}]` on `ops-bot`. Run `shrine deploy team ops_bot --dry-run`. Assert zero exit; assert the dry-run summary lists `Resource:shared-cache` under `Application:ops-bot`'s deps WITHOUT the `(inferred ...)` tag (SC-004); assert no `enrichment:` error appears on stderr.

- [X] T027 [P] [US3] Write failing integration test `TestDeployTeam_FailsOnAbsentTargetReference` in `tests/integration/deploy_team_infer_test.go`. Set up an `ops_bot`-owned Application with `valueFrom: resource.does-not-exist.HOST` and no explicit deps. Run `shrine deploy team ops_bot --dry-run`. Assert non-zero exit; assert stderr's `enrichment:` line names `does-not-exist` as the unresolved target (Q2 clarification — absent target treated identically to cross-team).

- [X] T028 [P] [US3] Write failing integration test `TestDeployTeam_FailsFastOnFirstOffendingRef` in `tests/integration/deploy_team_infer_test.go`. Set up two `ops_bot`-owned Applications, each with multiple bad refs (mixing cross-team and absent). Run `shrine deploy team ops_bot --dry-run` twice. Assert: exactly one `enrichment:` line appears on stderr per run (fail-fast, Q3); the line is identical byte-for-byte across both runs (FR-013 failure-path determinism); the offending consumer is the first Application by `metadata.name` sort order and the offending env var is the first one in that Application's declaration order (Decision 8).

### Implementation for US3

- [X] T029 [US3] No new production code expected — the FR-010 failure behaviour is delivered by `applyEnrichmentRule` (T009) plus the rule wrappers (T019, T023). Verify each of T025–T028 passes end-to-end (`go test -tags integration ./tests/integration/ -run TestDeployTeam_(FailsOnCrossTeam|Succeeds|FailsOnAbsent|FailsFast)`). If any test fails due to a formatting drift (e.g., the error message in `(*EnrichmentError).Error()` differs from the contract), fix the source in `internal/planner/enrich.go` rather than the test. If the absent-target case fails because the target's owner cannot be determined when the manifest is not in the set, confirm `applyEnrichmentRule`'s `lookupOwner` returns `("", false)` for missing names and that the failure branch fires on `!exists` — both conditions must hit the same error path.

**Checkpoint**: All three user stories shipped. The original bug is fixed, app-to-app same-team inference works, and silent under-specification of cross-team coupling is impossible.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Invariant tests that span all three stories, documentation refresh, and the constitution's Principle V gate.

- [X] T030 [P] Add a no-disk-write invariant unit test in `internal/planner/plan_test.go` (new test `TestPlan_DoesNotWriteToDisk`). Construct an in-memory `ManifestSet` via test fixtures, snapshot the mtime of the working directory, run `Plan()`, assert the mtime is unchanged. Run the same assertion on the failure path (a cross-team setup that triggers FR-010). Covers SC-006.

- [X] T031 [P] Add an input-immutability invariant unit test in `internal/planner/plan_test.go` (new test `TestPlan_DoesNotMutateInputSet`). Snapshot every Application's `len(Spec.Dependencies)` and the slice's contents before calling `Plan()` and assert they are byte-identical after, on both the success path and the FR-010 failure path. Covers SC-007 + invariant 1 from data-model.md.

- [X] T032 [P] Walk through `specs/020-infer-valuefrom-deps/quickstart.md` Part A scenarios manually against the built binary (`go build -o /tmp/shrine ./cmd/shrine` then run each `shrine deploy team ops_bot ...` invocation). Confirm the printed output matches the quickstart's expected blocks. Any discrepancy → fix code (preferred) or update quickstart (only if the design genuinely changed).

- [X] T033 [P] Update `AGENTS.md` with one line under the planner section noting same-team-only enrichment for `valueFrom: resource.*` and `valueFrom: application.*` references and the fail-fast cross-team behaviour, with a pointer to `specs/020-infer-valuefrom-deps/spec.md` and `specs/020-infer-valuefrom-deps/quickstart.md`.

- [X] T034 Run the integration gate (Principle V hard gate): `go test -tags integration ./tests/integration/...` from repo root. All `TestDeployTeam_*` tests from this feature must pass; no pre-existing integration tests should regress.

- [X] T035 Run `graphify update .` to refresh the knowledge graph after the enrichment-layer additions (per project CLAUDE.md "After modifying code, run graphify update").

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup. BLOCKS all user stories. The TDD tests (T002–T005) gate the implementation tasks (T006–T016) by Principle V's "tests before implementation" rule.
- **User Stories (Phase 3–5)**: All depend on Foundational completion.
  - US1 and US2 can be developed in parallel — they edit disjoint `Enrich()` bodies in `enrich_valuefrom.go` (T019, T023). Their unit-test additions to `enrich_valuefrom_test.go` and their integration-test additions to `deploy_team_infer_test.go` are also disjoint test functions, mergeable independently.
  - US3 has no new production code; its tests (T025–T028) can be written in parallel with US1/US2's tests, but T029 (verify-and-fix) requires both US1's and US2's rule bodies to be in place.
- **Polish (Phase 6)**: Depends on US1 + US2 + US3 being complete.

### Within Each Phase

- Tests (T002–T005, T017–T018, T021–T022, T025–T028) MUST be written and failing before the corresponding implementation tasks.
- Within Foundational: T006 (types) before T007/T008/T009/T010/T011 (which use them); T012 (PlanResult field) before T013 (Plan() wiring uses InferredEdges); T013/T014 before T015 (DryRun handler wiring); T013 before T016 (handler error propagation verification).

### Parallel Opportunities

- **In Foundational tests**: T002 [P] (different file) parallel with T003/T004/T005 (same file, sequential among themselves).
- **In Foundational implementation**: T008 [P] (enrich_valuefrom.go) parallel with T006/T007 (enrich.go); T012 [P] (plan.go) parallel with all enrich.go/enrich_valuefrom.go work; T014 [P] (deploy_plan_format.go) parallel with most of Foundational once T006 defines `InferredEdge`.
- **Across US1 + US2**: T017 [P] T018 [P] T021 [P] T022 [P] — all four can be written by different developers in parallel. T019 and T023 edit disjoint Enrich bodies and can land in parallel commits.
- **US3 tests**: T025–T028 are four independent integration test functions in the same file; can be drafted in parallel but committed sequentially to avoid merge churn.
- **Polish**: T030, T031, T032, T033 are all [P] (disjoint files).

---

## Parallel Example: Foundational Tests

```bash
# Four developers can draft these failing tests at the same time:
T002 [P] — parseValueFromRef tests in enrich_valuefrom_test.go
T003     — copyManifestSetShallow/cloneApplicationWithDeps/hasExplicitDependency tests in enrich_test.go
T004     — ChainEnrich tests in enrich_test.go (after T003 merges to avoid same-file conflict)
T005     — applyEnrichmentRule failure-path tests in enrich_test.go (after T004 merges)
```

## Parallel Example: User Stories 1 + 2

```bash
# Once Foundational is done, two developers can work in parallel:
Developer A (US1): T017 [P] → T018 [P] → T019 → T020
Developer B (US2): T021 [P] → T022 [P] → T023 → T024

# US3's tests can be drafted concurrently by either developer:
T025 [P], T026 [P], T027 [P], T028 [P] (sequential commits, parallel drafts)
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Complete Phase 1: Setup (T001).
2. Complete Phase 2: Foundational (T002–T016). This is the bulk of the work; budget for it accordingly.
3. Complete Phase 3: US1 (T017–T020).
4. **STOP and VALIDATE**: The original bug from the feature input is fixed. Same-team `valueFrom: resource.*` references are inferred as deploy-order edges.
5. Optionally deploy / demo / ship.

### Incremental Delivery

1. Foundational + US1 → MVP ships. Cross-team failure protection is already active (lives in `applyEnrichmentRule`, T009), so even though only the resource rule is wired, any cross-team `valueFrom: resource.*` already fails appropriately.
2. Add US2 → app-to-app same-team inference now works too.
3. Add US3 → integration-level coverage of the failure path (no new production code; just tests).
4. Polish → invariants, docs, integration gate, graph refresh.

### Parallel Team Strategy

Once Foundational (Phase 2) is done:
- Developer A: US1 (rule wrapper + tests).
- Developer B: US2 (rule wrapper + tests).
- Developer C: US3 (failure-mode integration tests) + Polish (T030–T033).
- All three merge together; T029 (verify US3 tests pass) gates on US1 + US2 being in.

---

## Notes

- `[P]` strictly means "different file, no upstream dependency"; tasks in the same file are listed without `[P]` even when the underlying changes are logically independent, to avoid merge-conflict surprises.
- The fail-fast same-team check (FR-010) lives in `applyEnrichmentRule` (T009) and is therefore active from the moment Foundational lands — even before US1/US2 fill in their rule bodies. The empty rule stubs from T011 simply return `(set, nil)`, so no `valueFrom` is parsed until the bodies are filled in T019 / T023; cross-team rejection then activates for resource refs (US1) and application refs (US2) as those rules ship.
- Run `graphify update .` (T035) after the implementation to keep the knowledge graph current — CLAUDE.md asks for it after any code modification.
- Constitution Principle V: integration tests via `NewDockerSuite` against the real shrine binary. All `TestDeployTeam_*` tests in this feature MUST use that harness; in-process testing of `cmd.Execute()` belongs in `cmd/cmd_test.go`, not in `tests/integration/`.
