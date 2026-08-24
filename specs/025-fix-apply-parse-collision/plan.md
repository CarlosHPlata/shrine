# Implementation Plan: Strict Apply Failures and Scoped Routing-Collision Detection

**Branch**: `025-fix-apply-parse-collision` | **Date**: 2026-08-24 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/025-fix-apply-parse-collision/spec.md` (GitHub issue #36)

## Summary

Two `apply` paths break promises earlier specs made. (1) `handler.ApplyTeams` prints each parse error to stdout, `continue`s, and returns `nil`, so `shrine apply teams` exits 0 on a `kind: Aplication` typo — violating spec 002 FR-003, with the lenient behaviour pinned by `tests/integration/apply_test.go` ("regardless of exit code"). Fix: a two-phase sync — parse every shrine-classified candidate and validate every Team manifest first, aggregate all failures into one multi-error return (Cobra prints it on stderr, `main.go` exits 1), and write teams only when the whole directory is clean; persistence failures also return instead of being printed. (2) `planner.Plan` calls `DetectRoutingCollisions` only under `FilterNone`/`FilterTeam`, so `shrine apply -f` (`FilterApp`) never checks collisions, while a team-scoped deploy scans the *entire* set and blocks unrelated teams — misaligned with spec 019 FR-010. Fix: `DetectRoutingCollisions(set, filter)`, hoisted out of the filter switch so it runs for every filter and reports only colliding pairs with at least one in-scope participant (`Filter.isAppInScope`). No engine, backend, CLI-flag, manifest-schema, or dry-run-engine changes; the collision diagnostic text is unchanged.

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: Cobra (error plumbing already in place: `RunE` error → `Error: …` on stderr, `main.go` exits 1), `gopkg.in/yaml.v3` (indirect via `manifest.Parse`); no new dependencies
**Storage**: `state.TeamStore` JSON team records — only the *timing* of writes changes (after full-directory validation); no schema change
**Testing**: `go test ./...` for units (package policy: unit tests never touch the filesystem — planner tests build `ManifestSet` structs in memory; `ApplyTeams` is filesystem-bound by design, so its coverage stays integration-only as today); integration via `tests/integration/` (`NewSuite` for `apply teams`, `NewDockerSuite` for `apply -f` and `deploy team`) with `-tags integration`, authored red-first and compile-checked locally (`go vet -tags integration ./tests/integration/...`), executed by CI
**Target Platform**: Linux server (Docker host)
**Project Type**: Single Go CLI
**Performance Goals**: N/A — collision detection is linear in declared routes over tens of manifests; one extra `manifest.Validate` per Team file
**Constraints**: The collision diagnostic is a contract (spec 016 FR-003/FR-004: `routing validation failed:` header, `routing collision: host=… pathPrefix=… declared by … and …` lines) and must not change; bare `shrine deploy` behaviour must be unchanged; `apply teams` informational lines (`Synced team:`, `Skipping …: not a Team manifest`, foreign-file notice) stay on stdout while failures move to stderr via the returned error
**Scale/Scope**: 4 production files (`internal/planner/{collisions,filter,plan}.go`, `internal/handler/teams.go`; ~60 net lines), 4 unit-test files, 2 integration-test files, 5 new fixture directories, 4 documentation files

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | Pass — no new capability surface, flags, or manifest fields; `apply teams` now produces the multi-error report Principle I mandates (all failing files in one run, research D1/D2) |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | Pass — no new commands; text-in/text-out honoured (notices → stdout, failures → stderr via returned error, research D3). `apply -f` has never had `--dry-run`; that pre-existing gap is out of scope and unchanged |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | N/A — changes are confined to `internal/planner/` and `internal/handler/`; `internal/engine/` and all backends untouched |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | Pass — no new types or interfaces; one parameter added to an existing function, one boolean helper on the existing `Filter`, and a per-filter special case *removed* from `Plan` (research D5/D6). No writer injection into `ApplyTeams` (research D3) |
| V. Integration-Test Gate | Does this phase map to an integration test scenario using `NewDockerSuite` against a real binary? | Pass — scenarios in `tests/integration/apply_test.go` (Phase 3 `apply teams`, Phase 7 `apply -f`) and `tests/integration/deploy_team_test.go`, TDD-ordered (authored red-first; CI executes the gate). `specs/features/integration-tests.md` updated with the new cases |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | Pass — no Docker operations involved; team records are written only after every manifest in the directory has been parsed and validated (research D1), so a failed run leaves state exactly as it was |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | Pass — `ApplyTeams` splits into intention-revealing helpers (`collectTeamManifests`, `saveTeams`); scope logic lives in `Filter.isAppInScope` (boolean `is*` naming); the stale "unlike routing collisions below" comment in `plan.go` is deleted with the special case it described |

**Post-Phase-1 re-check**: all gates unchanged — Pass. No Complexity Tracking entries required.

## Project Structure

### Documentation (this feature)

```text
specs/025-fix-apply-parse-collision/
├── plan.md              # This file
├── research.md          # Phase 0 — 10 decisions (failure semantics, error format/channel, scope API, placement, pair rule, tests, docs)
├── data-model.md        # Phase 1 — scope predicate table, collision-pair rule, apply-teams run state machine
├── quickstart.md        # Phase 1 — manual verification walkthrough per user story
├── contracts/
│   ├── cli-behaviour.md # Phase 1 — exit codes / stdout / stderr per command and scenario
│   └── planner-api.md   # Phase 1 — DetectRoutingCollisions(set, filter) + Filter.isAppInScope contract
└── tasks.md             # Phase 2 (/speckit-tasks — NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/planner/
├── collisions.go        # MODIFIED: DetectRoutingCollisions(set, filter) — a colliding pair is reported only when
│                        #           at least one participant is in scope (collisions.go:18, addRoute closure)
├── collisions_test.go   # MODIFIED: existing cases pass NoFilter(); new team/app/resource-scope cases
├── filter.go            # MODIFIED: func (f Filter) isAppInScope(app *manifest.ApplicationManifest) bool
├── filter_test.go       # MODIFIED: isAppInScope per FilterKind
├── plan.go              # MODIFIED: collision check hoisted out of the switch, runs for every filter after
│                        #           DetectHostPortCollisions (plan.go:47-58); stale comment removed
└── plan_test.go         # MODIFIED: ByApp detects collisions; ByTeam ignores out-of-scope pair; ByResource no-op

internal/handler/
└── teams.go             # MODIFIED: ApplyTeams → collectTeamManifests (parse all + validate Teams, aggregate errors)
                         #           then saveTeams (persist, return on failure); no per-file continue (teams.go:66-101)

tests/integration/
├── apply_test.go        # MODIFIED: lenient bad-kind test → AssertFailure + stderr assertions + AssertTeamNotInState;
│                        #           new multi-broken / invalid-team cases; new apply -f collision cases (Docker suite)
└── deploy_team_test.go  # MODIFIED: out-of-scope collision passes; cross-team in-scope collision fails

tests/testdata/
├── apply/bad-kind/               # EXISTING (team.yaml + typo.yaml) — reused: proves no team is written
├── apply/multi-broken/           # NEW: two broken shrine files + one valid team → both reported
├── apply/invalid-team/           # NEW: Team manifest missing metadata.name → validation failure
├── apply/routing-collision/      # NEW: team.yml, app-a (domain X), app-b (domain X), app-c (no routing)
├── deploy_team/collision-other/  # NEW: team-a app clean; two team-b apps collide
└── deploy_team/collision-cross/  # NEW: team-a app and team-b app collide

docs/content/guides/team-scoped-deploy.md   # MODIFIED: scope table row + dry-run paragraph describe the scoped rule
AGENTS.md                                   # MODIFIED: apply -f / apply teams reference lines
specs/progress.md                           # MODIFIED: fix entry (issue #36)
specs/features/integration-tests.md         # MODIFIED: new scenarios under Phase 3 / Phase 7 / deploy team
```

**Structure Decision**: Single-project Go CLI; production changes stay inside the two packages that own the defects (`internal/planner/` for scope, `internal/handler/` for the sync). `cmd/apply.go` is untouched — Cobra's existing `RunE` error path already delivers stderr + exit 1 once the handler returns an error. The Cobra `Long` strings are deliberately unchanged so the auto-generated `docs/content/cli/*.md` pages do not drift (research D10).

## Design Outline

1. **Scope predicate** (`filter.go`): `func (f Filter) isAppInScope(app *manifest.ApplicationManifest) bool` — `FilterNone` → true; `FilterTeam` → `app.Metadata.Owner == f.Name` (exact match, same rule as `filterStepsByOwner`); `FilterApp` → `app.Metadata.Name == f.Name`; `FilterRes` → false (FR-011).
2. **Scoped detection** (`collisions.go`): `DetectRoutingCollisions(set *ManifestSet, filter Filter) error`. The walk, `seen` map, sorting, and message format are unchanged; `addRoute` appends a pair only when `filter.isAppInScope(existingApp) || filter.isAppInScope(currentApp)` (FR-011/FR-012/FR-013). The app pointer for each ref is already at hand in the sorted walk, so no lookup by string is needed.
3. **Uniform placement** (`plan.go`): call `DetectRoutingCollisions(set, filter)` immediately after `DetectHostPortCollisions`, before the `switch`; delete the call inside `case FilterNone, FilterTeam` and the comment that contrasted the two checks. `FilterApp`/`FilterRes` branches otherwise unchanged (FR-008/FR-009/FR-010/FR-014). `handler.ApplySingle` needs no change: `loadSetForSingle` already loads the specs directory and `Plan` returns `result.Error` before `ExecuteDeploy` (research D8).
4. **Two-phase team sync** (`teams.go`): `ApplyTeams` = `ScanDir` → `collectTeamManifests(candidates) ([]*manifest.TeamManifest, []string)` — for each candidate `manifest.Parse` (failure → `parsing manifest %q: %v`), non-Team → stdout skip notice, Team → `manifest.Validate` (failure → `validating manifest %q: %v`) → collected; if any failures, return `fmt.Errorf("apply teams failed:\n- %s", strings.Join(errs, "\n- "))` without touching the store (FR-001..FR-004, FR-006). Then `saveTeams` persists each, returning `saving team %q to state: %w` on the first failure (FR-005), printing `Synced team:` and the summary on success; foreign-file notice unchanged (FR-007).
5. **Tests** (TDD per Constitution V): unit tests for `isAppInScope`, scoped `DetectRoutingCollisions`, and `Plan` per filter are written red-first; integration scenarios (contracts/cli-behaviour.md) are authored alongside with new fixtures and compile-checked via `go vet -tags integration ./tests/integration/...`; the existing lenient `apply teams` test flips to `AssertFailure().AssertStderrContains("typo.yaml").AssertStderrContains("Aplication")` plus `AssertTeamNotInState` (FR-015/FR-016).
6. **Docs**: rewrite the "Routing-collision detection" row and the dry-run cross-team paragraph in `team-scoped-deploy.md` to the scoped rule; update the `apply -f` / `apply teams` lines in `AGENTS.md`; add the fix entry to `specs/progress.md`; list the new scenarios in `specs/features/integration-tests.md`.

## Complexity Tracking

No Constitution violations — table intentionally empty.
