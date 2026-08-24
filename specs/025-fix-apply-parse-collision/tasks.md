# Tasks: Strict Apply Failures and Scoped Routing-Collision Detection

**Input**: Design documents from `/specs/025-fix-apply-parse-collision/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli-behaviour.md, contracts/planner-api.md, quickstart.md

**Tests**: Included — Constitution Principle V mandates TDD (tests authored red-first, before implementation). Per team practice, unit tests run locally and never touch the filesystem; integration tests are authored and compile-checked locally (`go vet -tags integration ./tests/integration/...`) and executed by the CI pipeline as the gate — never run locally.

**Organization**: Tasks are grouped by user story. US2 (`apply -f` collisions) and US3 (team-scoped collisions) are delivered by one planner rule — "in-scope steps against the full footprint" — so the scope predicate and scoped detector live in the Foundational phase (behaviour-neutral until wired), US2 wires it into `Plan` for every filter (which by construction also delivers US3's behaviour), and US3 adds the tests, fixtures, and documentation that pin the team-scope contract. US1 (`apply teams`) is fully independent.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 (P1 strict `apply teams`), US2 (P1 `apply -f` collision detection), US3 (P2 team-scoped collision scope)

## Phase 1: Setup

**Purpose**: Confirm a green baseline — this is a bug fix on existing code, no scaffolding needed.

- [X] T001 Verify green baseline at repository root: `go build ./...`, `go test ./internal/planner/ ./internal/handler/`, and `go vet -tags integration ./tests/integration/...` all pass before any change

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The scope predicate and the scoped collision detector that both US2 and US3 depend on. This phase is behaviour-neutral: the existing call site passes `NoFilter()`, so `shrine deploy` / `deploy team` / `apply -f` behave exactly as today until US2 wires the filter through.

### Tests (write FIRST — must FAIL/not compile before T005–T006 land) ⚠️

- [X] T002 Unit tests in `internal/planner/filter_test.go`: `TestFilter_IsAppInScope` table test over `NoFilter()`, `ByTeam("team-a")`, `ByApp("alpha")`, `ByResource("db-a")` against an app `{Owner: "team-a", Name: "alpha"}` and an app `{Owner: "team-b", Name: "beta"}` — expect true/true/true/false for the first app under None/Team/App/Res, and true/false/false/false for the second (contracts/planner-api.md table); build apps in memory via `manifest.ApplicationManifest{Metadata: manifest.Metadata{...}}`, no filesystem
- [X] T003 Update `internal/planner/collisions_test.go`: change every existing `DetectRoutingCollisions(set)` call to `DetectRoutingCollisions(set, NoFilter())` (unchanged expectations — pins byte-identical unscoped output), then add scoped cases using the existing `makeApp`/`setWith` helpers: `TestDetectRoutingCollisions_TeamScope_IgnoresOutOfScopePair` (team-b/beta1 + team-b/beta2 share a host; `ByTeam("team-a")` → nil), `TestDetectRoutingCollisions_TeamScope_ReportsInScopeVsOutOfScope` (team-a/alpha + team-b/beta share a host; `ByTeam("team-a")` → error naming both refs and the host), `TestDetectRoutingCollisions_AppScope_ReportsOnlyPairsTouchingApp` (alpha collides with beta, gamma collides with delta; `ByApp("alpha")` → exactly one `routing collision:` line, naming alpha and beta, not gamma/delta), `TestDetectRoutingCollisions_ResScope_AlwaysNil` (colliding apps; `ByResource("db")` → nil), `TestDetectRoutingCollisions_ThreeClaimants_OutOfScopeFirstStillPairsWithInScope` (team-a/aaa sorts first and is out of scope for `ByTeam("team-b")`; team-b/bbb and team-b/ccc share its host → two lines, each naming aaa with one of the team-b apps)
- [X] T004 Verify red: `go test ./internal/planner/` fails to compile (undefined `isAppInScope`, wrong arity on `DetectRoutingCollisions`)

### Implementation

- [X] T005 Add `func (f Filter) isAppInScope(app *manifest.ApplicationManifest) bool` to `internal/planner/filter.go` below `ByResource`: switch on `f.Kind` — `FilterNone` → `true`; `FilterTeam` → `app.Metadata.Owner == f.Name`; `FilterApp` → `app.Metadata.Name == f.Name`; `FilterRes` and default → `false`. Add the `internal/manifest` import. No comment needed — the name is the documentation
- [X] T006 Change `DetectRoutingCollisions` in `internal/planner/collisions.go` to `func DetectRoutingCollisions(set *ManifestSet, filter Filter) error`: keep the sorted walk, `seen` map, message format, and sort/join untouched; have the walk record the `*manifest.ApplicationManifest` alongside each ref (extend `refToApp`'s entry with an `app *manifest.ApplicationManifest` field, or keep a parallel `refToManifest map[string]*manifest.ApplicationManifest`) so `addRoute` can evaluate `filter.isAppInScope(existingApp) || filter.isAppInScope(currentApp)` and append the collision line only when that holds (research D7). Update the single call site in `internal/planner/plan.go` (`case FilterNone, FilterTeam`) to `DetectRoutingCollisions(set, NoFilter())` so this phase stays behaviour-neutral
- [X] T007 Verify green: `go test ./internal/planner/` passes (T002–T003 green, every pre-existing planner test unchanged); `go build ./...` clean

**Checkpoint**: Scoped detection exists and is proven in isolation; production behaviour unchanged.

---

## Phase 3: User Story 1 - `apply teams` fails loudly when a team manifest is broken (Priority: P1) 🎯 MVP

**Goal**: `shrine apply teams` exits non-zero, reports every unparseable shrine file and every invalid Team manifest on stderr in one run, and writes nothing to state when any file fails; clean directories behave exactly as today.

**Independent Test**: `TestApplyTeams` scenarios in `tests/integration/apply_test.go` (executed by CI): bad-kind → exit 1 + `typo.yaml` + `Aplication` on stderr + no team in state; multi-broken → both files reported, zero teams; invalid-team → `metadata.name is required`, no team. `ApplyTeams` is filesystem-bound by design, so it has no in-memory unit tests (project policy); red state is by construction — today's code exits 0 on these fixtures.

### Tests for User Story 1 (write FIRST) ⚠️

- [X] T008 [US1] Rewrite the lenient test `"should apply teams fail loudly when shrine manifest has bad kind"` in `tests/integration/apply_test.go` (currently lines 69-79): replace the "regardless of exit code" comment and `AssertOutputContains` calls with `.AssertFailure().AssertStderrContains("typo.yaml").AssertStderrContains("Aplication")` followed by `tc.AssertTeamNotInState("shrine-apply-test")` — the valid `team.yaml` sibling in `tests/testdata/apply/bad-kind/` must not be written (FR-001, FR-004; contracts/cli-behaviour.md T2)
- [X] T009 [P] [US1] Create fixture `tests/testdata/apply/multi-broken/` with three files: `typo.yaml` (copy of `tests/testdata/apply/bad-kind/typo.yaml`, `kind: Aplication`), `noname.yaml` (`apiVersion: shrine/v1`, `kind: Team`, `metadata: {}` — no name — `spec: {displayName: "No Name", contact: "noname@example.com"}`), and `team.yaml` (valid Team named `shrine-apply-multi` with displayName, contact, quotas `maxApps: 10`/`maxResources: 10`, `registryUser: shrine-apply-multi`)
- [X] T010 [P] [US1] Create fixture `tests/testdata/apply/invalid-team/noname.yaml`: `apiVersion: shrine/v1`, `kind: Team`, `metadata: {}`, `spec: {displayName: "Invalid Team", contact: "invalid@example.com", quotas: {maxApps: 1, maxResources: 1}}` — parses, fails `manifest.Validate` with `metadata.name is required`
- [X] T011 [US1] Add two scenarios to `TestApplyTeams` in `tests/integration/apply_test.go` (after T008's test): `"should report every broken manifest and write no team"` — `tc.Run("apply", "teams", "--path", applyFixturesPath("multi-broken"), "--state-dir", tc.StateDir).AssertFailure().AssertStderrContains("apply teams failed").AssertStderrContains("typo.yaml").AssertStderrContains("noname.yaml")` then `tc.AssertTeamCount(0)` (FR-003, FR-004); and `"should reject a Team manifest that fails validation"` — same shape against `applyFixturesPath("invalid-team")`, asserting stderr contains `validating manifest`, `noname.yaml`, and `metadata.name is required`, then `tc.AssertTeamCount(0)` (FR-002)
- [X] T012 [US1] Verify: `go vet -tags integration ./tests/integration/...` compiles clean; confirm by reading `internal/handler/teams.go:66-101` that the current loop returns `nil` after a parse error (the scenarios are red by construction and will be executed by CI)

### Implementation for User Story 1

- [X] T013 [US1] Refactor `ApplyTeams` in `internal/handler/teams.go` into two phases (research D1–D4, contracts/planner-api.md `ApplyTeams` table): keep `ScanDir` error handling and the zero-candidate `No team manifests found` early return; extract `collectTeamManifests(candidates []manifest.ShrineCandidate) ([]*manifest.TeamManifest, []string)` — per candidate in order: `manifest.Parse(c.Path)` error → append `fmt.Sprintf("parsing manifest %q: %v", c.Path, err)`; `m.Team == nil` → `fmt.Printf("Skipping %s: not a Team manifest\n", c.Path)`; `manifest.Validate(m)` error → append `fmt.Sprintf("validating manifest %q: %v", c.Path, err)`; else collect `m.Team`. If any failures: `return fmt.Errorf("apply teams failed:\n- %s", strings.Join(failures, "\n- "))` before any store call. Extract `saveTeams(teams []*manifest.TeamManifest, store state.TeamStore) (int, error)` — `store.SaveTeam` error → `return count, fmt.Errorf("saving team %q to state: %w", team.Metadata.Name, err)`; success → `fmt.Printf("Synced team: %s\n", …)` and increment. Then print `Successfully synced %d teams to state.` and the foreign-file notice as today. Remove every `fmt.Printf("Error …")` + `continue`; add the `strings` import
- [X] T014 [US1] Verify green: `go build ./...`, `go test ./internal/handler/`, `go vet -tags integration ./tests/integration/...` all pass; `gofmt -l internal/handler/` prints nothing

**Checkpoint**: `apply teams` is strict and atomic — MVP complete for issue #36 gap 1 (CI executes T008/T011 as the gate).

---

## Phase 4: User Story 2 - `apply -f` rejects a manifest that collides with an existing route (Priority: P1)

**Goal**: `shrine apply -f` runs routing-collision detection against the full specs directory before deploying, rejects the applied app with the same diagnostic `deploy` emits, and is not blocked by collisions that do not involve the applied app.

**Independent Test**: `go test ./internal/planner/` (`ByApp`/`ByResource` cases) plus `TestApplyFile` scenarios in `tests/integration/apply_test.go` (CI): `apply -f app-b.yml` → exit 1, `routing collision`, both refs, no `shrine-apply-test.app-b` container; `apply -f app-c.yml` → exit 0 despite the unrelated a/b collision.

### Tests for User Story 2 (write FIRST — must FAIL before T019 lands) ⚠️

- [X] T015 [US2] Unit tests in `internal/planner/plan_test.go` using `NewManifestSet()` + `stubTeamStore{}` + `PortContext{}` (same pattern as `TestPlan_ByApp_SingleStep`), with apps that set `Spec.Image`, `Spec.Port`, and `Spec.Routing.Domain`: `TestPlan_ByApp_ReportsCollisionInvolvingApp` (team-a/alpha and team-b/beta both `clash.example.com`; `Plan(set, …, ByApp("alpha"))` → `result.Error` non-nil containing `routing collision`, `team-a/alpha`, `team-b/beta`, and `result.Steps` empty); `TestPlan_ByApp_IgnoresCollisionNotInvolvingApp` (alpha has its own domain; beta and gamma collide; `ByApp("alpha")` → `result.Error == nil`, exactly one step for alpha); `TestPlan_ByResource_IgnoresRoutingCollisions` (alpha and beta collide; resource db-a; `ByResource("db-a")` → `result.Error == nil`, one step) (FR-008–FR-012)
- [X] T016 [P] [US2] Create fixture `tests/testdata/apply/routing-collision/`: `team.yml` (copy of `tests/testdata/apply/success/team.yml`), `app-a.yml` and `app-b.yml` (Applications `app-a`/`app-b`, owner `shrine-apply-test`, image `traefik/whoami`, port 80, replicas 1, `routing: {domain: collision.apply.local}`, `networking: {exposeToPlatform: false}`), and `app-c.yml` (Application `app-c`, same owner/image/port, no `routing` block, `exposeToPlatform: false`)
- [X] T017 [US2] Add two scenarios to `TestApplyFile` in `tests/integration/apply_test.go` (Docker suite; `BeforeEach` already seeds the `shrine-apply-test` team): `"should reject apply -f when the manifest collides with an existing route"` — `tc.Run("apply", "-f", applyFixturesPath("routing-collision", "app-b.yml"), "--path", applyFixturesPath("routing-collision"), "--state-dir", tc.StateDir).AssertFailure().AssertStderrContains("routing validation failed").AssertStderrContains("routing collision").AssertStderrContains("shrine-apply-test/app-a").AssertStderrContains("shrine-apply-test/app-b")` then `tc.AssertContainerNotExists(applyTestTeam + ".app-b")` (contracts/cli-behaviour.md F1); and `"should apply -f succeed when the only collision does not involve the applied app"` — same flags with `app-c.yml`, `.AssertSuccess()` then `tc.AssertContainerRunning(applyTestTeam + ".app-c")` (F3)
- [X] T018 [US2] Verify red: `go test ./internal/planner/` fails on `TestPlan_ByApp_ReportsCollisionInvolvingApp` (FilterApp path returns no error today); `go vet -tags integration ./tests/integration/...` compiles clean

### Implementation for User Story 2

- [X] T019 [US2] Wire the scoped check in `internal/planner/plan.go` `Plan()` (research D6): immediately after the `DetectHostPortCollisions` block, add `if err := DetectRoutingCollisions(set, filter); err != nil { return PlanResult{Error: err} }`; delete the `DetectRoutingCollisions(set, NoFilter())` call inside `case FilterNone, FilterTeam`; replace the three-line "Unlike routing collisions below…" comment with one WHY line above the routing check, e.g. `// Scoped to filter: only pairs touching an in-scope app fail (spec 019 FR-010).` — `FilterApp`/`FilterRes` branches and `Order`/`filterStepsByOwner` are otherwise untouched. No change to `internal/handler/apply.go`: `loadSetForSingle` already loads the directory and `ApplySingle` returns `result.Error` before `ExecuteDeploy` (research D8)
- [X] T020 [US2] Verify green: `go test ./internal/planner/ ./internal/handler/` passes (including every pre-existing `TestPlan_*` case); `go vet -tags integration ./tests/integration/...` compiles clean; `gofmt -l internal/planner/` prints nothing

**Checkpoint**: `apply -f` can no longer introduce an undetected collision; because the same line now serves `FilterTeam`, team-scoped deploys are already scoped — US3 pins and documents that.

---

## Phase 5: User Story 3 - Team-scoped deploy fails only on collisions involving its own applications (Priority: P2)

**Goal**: `shrine deploy team T` (and `--dry-run`) fails when any T-owned app collides with anything, and proceeds when the only collisions are entirely between other teams' apps; bare `shrine deploy` is unchanged; the guide documents the scoped rule.

**Independent Test**: `go test ./internal/planner/` (`ByTeam` cases) plus `TestDeployTeam` scenarios in `tests/integration/deploy_team_test.go` (CI): collision-other → team-a passes, team-b fails, bare deploy fails; collision-cross → team-a fails naming both apps; existing Scenario E still fails.

### Tests for User Story 3 (characterisation of the behaviour T019 delivers — author them before T019 to observe red, otherwise they are green on arrival by design)

- [X] T021 [US3] Unit tests in `internal/planner/plan_test.go` (same builders as T015): `TestPlan_ByTeam_IgnoresOutOfScopeCollision` (team-a/alpha with its own domain; team-b/beta1 and team-b/beta2 share a domain; `ByTeam("team-a")` → `result.Error == nil`, steps contain only alpha); `TestPlan_ByTeam_ReportsCrossTeamCollision` (team-a/alpha and team-b/beta share a domain; `ByTeam("team-a")` → `result.Error` names `team-a/alpha` and `team-b/beta`); `TestPlan_NoFilter_ReportsEveryCollision` (same set as the first case; `NoFilter()` → `result.Error` names beta1 and beta2 — bare deploy unchanged, FR-012)
- [X] T022 [P] [US3] Create fixture `tests/testdata/deploy_team/collision-other/`: `alpha.yml` (Application `alpha`, owner `shrine-team-a`, `traefik/whoami`, port 80, replicas 1, no `routing`, `exposeToPlatform: false`), `beta1.yml` and `beta2.yml` (Applications `beta1`/`beta2`, owner `shrine-team-b`, same image/port, `routing: {domain: collide-other.test.local}`, `exposeToPlatform: false`) — model on `tests/testdata/deploy_team/collision/alpha1.yml`
- [X] T023 [P] [US3] Create fixture `tests/testdata/deploy_team/collision-cross/`: `alpha.yml` (owner `shrine-team-a`, `routing: {domain: collide-cross.test.local}`) and `beta.yml` (owner `shrine-team-b`, same domain), otherwise identical to the T022 shape
- [X] T024 [US3] Add scenarios to `TestDeployTeam` in `tests/integration/deploy_team_test.go` directly after Scenario E (keep Scenario E unchanged — both apps are team-a, so it must still fail): Scenario E2 `"deploy team ignores collisions entirely outside its scope"` — `tc.Run("deploy", "team", teamA, "--dry-run", "--path", deployTeamFixturesPath("collision-other"), "--state-dir", tc.StateDir).AssertSuccess().AssertOutputNotContains("routing collision")` (contracts/cli-behaviour.md D3); Scenario E3 `"deploy team still fails on its own team's collision"` — same directory with `teamB` → `.AssertFailure().AssertStderrContains("shrine-team-b/beta1").AssertStderrContains("shrine-team-b/beta2")` (D4); Scenario E4 `"bare deploy still fails on any collision"` — `tc.Run("deploy", "--dry-run", "--path", deployTeamFixturesPath("collision-other"), "--state-dir", tc.StateDir).AssertFailure().AssertStderrContains("routing collision")` (D5); Scenario E5 `"deploy team fails when its app collides with another team's app"` — `teamA` against `deployTeamFixturesPath("collision-cross")` → `.AssertFailure().AssertStderrContains("shrine-team-a/alpha").AssertStderrContains("shrine-team-b/beta").AssertStderrContains("collide-cross.test.local")` (D2)
- [X] T025 [US3] Verify: `go test ./internal/planner/` passes (T021 green once T019 is in; if authored before T019, `TestPlan_ByTeam_IgnoresOutOfScopeCollision` must be red first); `go vet -tags integration ./tests/integration/...` compiles clean

### Implementation for User Story 3

- [X] T026 [US3] Update `docs/content/guides/team-scoped-deploy.md`: change the scope table row (line 33) to `| Routing-collision detection | Team-owned routes are checked against the **full** directory's routing footprint; a collision fails the deploy only when at least one participant belongs to the requested team |`, and rewrite the dry-run paragraph (lines 121-126) so it states that a collision between two `marketing` apps, or between `marketing/blog.yml` and `ops/monitor.yml`, fails `shrine deploy team marketing`, while a collision purely between `ops` apps does **not** block `marketing` — bare `shrine deploy` remains the whole-directory check (research D10)

**Checkpoint**: All three stories complete; the team-scope contract is pinned by unit + integration tests and documented.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T027 [P] Update `AGENTS.md` CLI reference: under `### shrine apply -f <file>` add "Runs the same routing-collision validation as `deploy`, scoped to the applied app: the manifest is rejected (non-zero, same diagnostic) if its routes collide with any app in the specs directory"; under `### shrine apply teams` add "Exits non-zero and writes nothing to state if any shrine manifest in the directory fails to parse or any Team manifest fails validation; all failures are reported in one run"
- [X] T028 [P] Update `specs/features/integration-tests.md`: under Phase 3 (`apply teams`) list the strict bad-kind, multi-broken, and invalid-team cases; under Phase 7 (`apply -f` deploy cases) list the routing-collision reject/accept cases; add a short "deploy team — collision scope" note listing Scenarios E2–E5 with fixture names
- [X] T029 [P] Add the fix entry to `specs/progress.md` (above the 024 entry, same style): `- [x] **Fix: strict apply teams + scoped routing-collision detection** — see `specs/025-fix-apply-parse-collision/` (issue #36) …` summarising the two-phase `ApplyTeams`, `DetectRoutingCollisions(set, filter)` + `Filter.isAppInScope`, the hoisted check in `Plan`, acceptance SC-001–SC-006, and the gate (`TestApplyTeams`/`TestApplyFile`/`TestDeployTeam` scenarios, CI executes)
- [X] T030 Run the full local gate at repository root: `gofmt -l .` prints nothing, `go vet ./...` clean, `go test ./...` zero failures, `go vet -tags integration ./tests/integration/...` clean (SC-005)
- [X] T031 [P] Run `graphify update .` to refresh the knowledge graph after the code changes (project rule, AST-only)
- [ ] T032 Push the branch and confirm the CI integration pipeline is green — it executes the T008/T011/T017/T024 scenarios that form the Constitution V gate; optionally walk `specs/025-fix-apply-parse-collision/quickstart.md` against a live Docker host

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: after T001 — blocks US2 and US3 (both use `isAppInScope` and the scoped detector); does **not** block US1
- **US1 (Phase 3)**: after T001 only — touches `internal/handler/teams.go` and `apply_test.go` `TestApplyTeams`; fully independent of the planner work
- **US2 (Phase 4)**: after Phase 2 — T019 is the single production change that wires scoping into `Plan`
- **US3 (Phase 5)**: tests/fixtures/docs only; its behaviour is delivered by T019. Author T021/T024 before T019 to see them red, or accept them as characterisation tests afterwards
- **Polish (Phase 6)**: after all story phases

### Story Completion Order

```text
T001 ─┬─▶ US1 (T008…T014) ──────────────────────────────────────┐
      └─▶ Foundational (T002…T007) ─▶ US2 (T015…T020) ─▶ US3 (T021…T026) ─┴─▶ Polish (T027…T032)
```

### Parallel Opportunities

- **US1 and Phase 2** touch disjoint files (`internal/handler/` + `apply_test.go` `TestApplyTeams` vs `internal/planner/`) — two authors can run them concurrently from T001
- **T009 + T010 [P]** (US1 fixtures) and **T016 [P]** (US2 fixture) and **T022 + T023 [P]** (US3 fixtures) are new directories with no dependencies
- **T015 and T017** (US2 unit vs integration tests) are different files; likewise **T021 and T024** for US3
- **T027, T028, T029, T031 [P]** touch disjoint files
- `apply_test.go` is shared by US1 (`TestApplyTeams`) and US2 (`TestApplyFile`) — different functions, but coordinate merge order if two authors edit it concurrently; `plan_test.go` is shared by T015 and T021 in the same way

## Parallel Example: Foundational + User Story 1

```bash
# Two authors, disjoint files, right after T001:
Task: "T002–T007: isAppInScope + scoped DetectRoutingCollisions in internal/planner/ (filter.go, filter_test.go, collisions.go, collisions_test.go, plan.go)"
Task: "T008–T014: strict ApplyTeams in internal/handler/teams.go + TestApplyTeams scenarios in tests/integration/apply_test.go + fixtures under tests/testdata/apply/"
```

## Parallel Example: User Story 2

```bash
# After Phase 2, before T019:
Task: "T015: ByApp/ByResource collision cases in internal/planner/plan_test.go"
Task: "T016: tests/testdata/apply/routing-collision/ fixture"
Task: "T017: TestApplyFile collision scenarios in tests/integration/apply_test.go"
```

## Implementation Strategy

### MVP First (US1 only)

1. T001, then US1 red tests (T008–T012), then T013–T014.
2. **STOP and VALIDATE**: handler unit suite green, integration scenarios compile; CI executes them.
3. This alone closes issue #36 gap 1 — `apply teams` can no longer report success while skipping a broken file.

### Incremental Delivery

1. US1 → strict, atomic `apply teams` (deployable on its own).
2. Phase 2 + US2 → `apply -f` collision detection; the scoped rule lands here and closes gaps 2 and 3 in one production line.
3. US3 → pins the team-scope contract with tests and fixes the guide that currently documents the old whole-directory behaviour.
4. Polish → docs/reference/progress + CI gate; ship as one PR closing issue #36.

### Format validation

All 32 tasks use the checklist format (`- [ ] Txxx [P?] [Story?] description + exact file path`); story labels appear only in Phases 3–5.
