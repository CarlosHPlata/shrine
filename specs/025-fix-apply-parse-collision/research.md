# Research: Strict Apply Failures and Scoped Routing-Collision Detection

**Feature**: `025-fix-apply-parse-collision` | **Date**: 2026-08-24
**Purpose**: Resolve every design choice behind the plan so implementation has no open questions. The Technical Context in `plan.md` contained no `NEEDS CLARIFICATION` markers; the decisions below record the alternatives weighed for each judgment call.

## Current-state findings (what the code does today)

| Area | Location | Finding |
|------|----------|---------|
| `apply teams` loop | `internal/handler/teams.go:66-101` | Per candidate: `manifest.Parse` error → `fmt.Printf("Error parsing …")` + `continue`; non-Team → skip notice; `SaveTeam` error → printed + `continue`. Always returns `nil` after `Successfully synced %d teams`. No `manifest.Validate` call for Team bodies. |
| Scanner contract | `internal/manifest/scan.go`, `classify.go` | `ScanDir` already fails loudly on malformed YAML (spec 002 FR-004) and returns only shrine-classified candidates (with probed `TypeMeta`) plus foreign paths. `ApplyTeams` propagates that error correctly; the gap is only in the per-candidate loop. |
| Unknown kind | `internal/manifest/parser.go:41-71` | `manifest.Parse` returns `unknown manifest kind: "Aplication"`; wrapping it with the path yields `parsing manifest "…/typo.yaml": unknown manifest kind: "Aplication"` — file path and offending kind in one line (002 FR-003). |
| Error channel | `cmd/root.go`, `main.go:20-22` | Cobra `RunE` error → `Error: …` on stderr; `main` exits 1. No `SilenceUsage`; existing tests already assert with `AssertStderrContains`. |
| Collision gate | `internal/planner/plan.go:47-58` | `DetectHostPortCollisions` runs for every filter; `DetectRoutingCollisions(set)` only inside `case FilterNone, FilterTeam`, before `Order` and `filterStepsByOwner`. `FilterApp`/`FilterRes` return a single step with no routing check. |
| Collision algorithm | `internal/planner/collisions.go:18-96` | Sorted walk over `owner/name` refs; `seen[routeKey] = firstClaimant`; every later claimant of the same key produces one `routing collision: host=%q pathPrefix=%q declared by %q and %q` line; lines sorted and joined under `routing validation failed:`. Only caller: `plan.go:56`. |
| `apply -f` set | `internal/handler/apply.go:64-80` | `loadSetForSingle` loads `LoadDir(manifestDir)` then merges the `-f` manifest, ignoring `ErrDuplicateManifest` (directory copy wins). With `manifestDir == ""` the set holds only the one manifest. |
| Lenient test | `tests/integration/apply_test.go:69-79` | Asserts `typo.yaml` + `Aplication` appear on **stdout**, explicitly "regardless of exit code". Fixture `tests/testdata/apply/bad-kind/` holds `team.yaml` (valid) + `typo.yaml`. |
| Team-scope test | `tests/integration/deploy_team_test.go:95-103` | Scenario E: both colliding apps are team-a → in scope → must keep failing after this change. |
| Docs | `docs/content/guides/team-scoped-deploy.md:33,121-126` | Documents today's whole-directory scan, including "the collision is reported during a team-scoped dry-run even though only marketing is in the deploy scope" — must be rewritten. |

## Decision 1 — `apply teams` failure semantics: validate-all-then-apply

**Decision**: Two phases. Phase 1 parses every shrine-classified candidate and validates every Team manifest, collecting failures. If any failure exists, return an aggregated error and write nothing. Phase 2 persists the collected teams; a `SaveTeam` failure returns immediately with the team name and cause.

**Rationale**: Mirrors `planner.LoadDir`, which refuses to plan over a directory containing a broken manifest, and satisfies spec FR-003/FR-004 (all failures in one run, no partial writes). A declarative sync that both changes state *and* reports failure is ambiguous for automation; a run that changes nothing on failure is not. Teams already in state are untouched, so re-running after the fix is idempotent.

**Alternatives considered**:
- *Fail fast on the first parse error* — simpler loop, but violates Constitution I ("validation MUST produce multi-error reports") and forces one re-run per typo.
- *Best-effort: save valid teams, report failures, exit non-zero* — preserves today's partial progress, but leaves state half-synced under a failing exit code; rejected as the ambiguous outcome the issue is trying to eliminate.

## Decision 2 — Aggregated error format

**Decision**: `apply teams failed:\n- parsing manifest "<path>": <cause>\n- validating manifest "<path>": <cause>` built with `strings.Join(errs, "\n- ")`, one bullet per failing file, in scan order.

**Rationale**: Identical shape to `manifest.Validate` (`validation failed:\n- …`) and `DetectRoutingCollisions` (`routing validation failed:\n- …`), so operators and tests see one multi-error convention across the CLI. The bullet keeps the file path and cause on one line, satisfying FR-001's "file path and reason" and the existing `AssertStderrContains("typo.yaml")`/`("Aplication")` assertions.

**Alternatives considered**:
- `errors.Join` — no bullets or header; inconsistent with the two existing multi-error producers.
- A typed `ApplyTeamsError` with structured fields — no consumer needs structure (YAGNI, Constitution IV).

## Decision 3 — Error channel: return the error, keep stdout notices

**Decision**: `ApplyTeams` returns the aggregated error; Cobra prints it on stderr and `main.go` exits 1. Informational lines (`Skipping …: not a Team manifest`, `Synced team: …`, summary, foreign-file notice) stay as `fmt.Printf` on stdout. No `io.Writer` parameters are added to `ApplyTeams`.

**Rationale**: Constitution II text-in/text-out: user-visible output on stdout, errors on stderr. The handler's sibling functions (`GenerateTeam`, `CreateTeam`, `ListTeams`) also print directly; the only defect is that *errors* were printed instead of returned. Injecting writers would be a refactor with no test benefit, since `ApplyTeams` is filesystem-bound and covered by integration tests either way.

**Alternatives considered**: threading `out/errOut io.Writer` like `handler.Deploy` — deferred; revisit only if a unit test for the print path is ever needed.

## Decision 4 — What `apply teams` validates

**Decision**: Every shrine-classified candidate must `Parse`; only Team manifests are additionally run through `manifest.Validate`. Non-Team shrine manifests that parse are skipped with today's notice.

**Rationale**: FR-001 (all shrine files must parse, per 002 FR-003) plus FR-002 (invalid Team bodies must not reach state — today a Team with no `metadata.name` is saved unvalidated). Application/Resource bodies are validated by `LoadDir` on deploy; validating them here would make `apply teams` fail on defects that belong to a different command and are outside its stated scope (spec Assumptions).

## Decision 5 — Scope API: `DetectRoutingCollisions(set, filter)` + `Filter.isAppInScope`

**Decision**: Add a `filter Filter` parameter to `DetectRoutingCollisions` and a boolean method `func (f Filter) isAppInScope(app *manifest.ApplicationManifest) bool` on the existing `Filter` type.

**Rationale**: Spec 019 made `Filter` "the single concept" that scopes planning; reusing it keeps one notion of scope for step emission and collision detection, so the two cannot drift apart again (spec FR-011). A method on `Filter` follows the existing `Filter.Validate` pattern and Constitution VII's `is*` boolean naming. The function keeps a single caller (`Plan`), so no wrapper or variant is needed.

**Alternatives considered**:
- `inScope func(appName string) bool` predicate parameter — more generic than any current need (one caller); YAGNI.
- A second function `DetectScopedRoutingCollisions` delegating to the old one — two entry points to keep consistent, and the unscoped one would have no production caller.
- Post-filtering the error lines by substring — fragile string parsing of a message that is itself a contract.

## Decision 6 — Placement in `Plan`: one call, every filter

**Decision**: Hoist the call to immediately after `DetectHostPortCollisions`, before the `switch`, and delete it from `case FilterNone, FilterTeam` together with the comment that explained the asymmetry.

**Rationale**: `FilterApp` gains the check (closes gap 2) and `FilterTeam` becomes scoped (closes gap 3) through the same line. `FilterRes` is a natural no-op because `isAppInScope` is always false for it — no special-casing. Ordering relative to `Resolve`/`ChainEnrich` is unchanged, so validation errors still surface before collision errors exactly as today.

**Alternatives considered**: adding a second call inside `case FilterApp` while leaving the team branch as-is — closes gap 2 only, and keeps the special case the issue identifies as the root cause of gap 3.

## Decision 7 — Pair-reporting rule with three or more claimants

**Decision**: A pair `(first claimant, later claimant)` is reported iff at least one of the two is in scope. The `seen` map keeps the first (sorted) claimant of each route key, exactly as today.

**Rationale**: Matches the spec's definition (FR-011: fail only for collisions in which at least one participant is in scope) while preserving the diagnostic's pairing behaviour and sort order, so unscoped output is byte-identical to today (FR-013, SC-005). An out-of-scope first claimant still pairs with an in-scope later claimant, so an in-scope app can never slip past because another team happened to sort first.

## Decision 8 — `apply -f` manifest set: no loader changes

**Decision**: `loadSetForSingle` is untouched. The specs directory it already loads *is* the footprint (FR-008); the merged `-f` manifest is the in-scope application; `manifestDir == ""` yields a one-app set with nothing to collide against (spec edge case).

**Rationale**: Spec 019 FR-015(d) already proves `apply -f` loads the directory as resolution context. The duplicate-precedence quirk (directory copy wins over a differing `-f` file) is pre-existing and explicitly out of scope in the spec's Assumptions; the collision check evaluates whichever declaration `Plan` would deploy, which is the correct invariant.

## Decision 9 — Test strategy and fixtures (TDD per Constitution V)

**Decision**:
- **Unit (in-memory, no filesystem)**: `filter_test.go` — `isAppInScope` for each `FilterKind`; `collisions_test.go` — existing cases pass `NoFilter()`, plus team scope ignores an out-of-scope pair, team scope reports in-scope-vs-out-of-scope, app scope reports only pairs touching the app, resource scope always nil, three-claimant case; `plan_test.go` — `ByApp` returns a collision `Error`, `ByTeam` succeeds when the only pair is out of scope, `ByResource` ignores collisions among apps.
- **Integration — `TestApplyTeams` (`NewSuite`)**: flip the lenient test to `AssertFailure().AssertStderrContains("typo.yaml").AssertStderrContains("Aplication")` and add `AssertTeamNotInState("shrine-apply-test")` (the valid sibling in `bad-kind/` must not be written); new `apply/multi-broken/` fixture (two broken files, one valid team) asserting both paths on stderr and `AssertTeamCount(0)`; new `apply/invalid-team/` fixture (Team without `metadata.name`) asserting `metadata.name is required` on stderr and no team in state; existing foreign-yaml and multi-team success cases unchanged (regression guard for FR-007).
- **Integration — `TestApplyFile` (`NewDockerSuite`)**: new `apply/routing-collision/` fixture (`team.yml`, `app-a.yml` and `app-b.yml` both `collision.apply.local`, `app-c.yml` without routing); `apply -f app-b.yml` → `AssertFailure`, stderr contains `routing collision`, `shrine-apply-test/app-a`, `shrine-apply-test/app-b`, and `AssertContainerNotExists("shrine-apply-test.app-b")`; `apply -f app-c.yml` → `AssertSuccess` + `AssertContainerRunning` (unrelated collision does not block).
- **Integration — `TestDeployTeam`**: keep Scenario E; add `deploy_team/collision-other/` (`alpha.yml` team-a no routing; `beta1.yml`/`beta2.yml` team-b same domain): `deploy team team-a --dry-run` → `AssertSuccess`, `deploy team team-b --dry-run` → `AssertFailure` naming both, bare `deploy --dry-run` → `AssertFailure`; add `deploy_team/collision-cross/` (`alpha.yml` team-a and `beta.yml` team-b same domain): `deploy team team-a --dry-run` → `AssertFailure` naming `shrine-team-a/alpha` and `shrine-team-b/beta`.
- Integration files are authored before implementation, compile-checked with `go vet -tags integration ./tests/integration/...`, and executed by CI (never run locally — project convention).

**Rationale**: Unit tests pin the scope rule per filter without touching disk; integration scenarios pin the operator-visible contract (exit code, stderr, state, containers) for each user story and acceptance scenario. Dry-run is used for the team scenarios so the assertions hinge on planning, not container lifecycle.

## Decision 10 — Documentation surface

**Decision**: Update `docs/content/guides/team-scoped-deploy.md` (scope table row → "Team-owned routes checked against the full directory's routing footprint; collisions entirely between other teams' apps do not block"; rewrite the dry-run cross-team paragraph accordingly), `AGENTS.md` (`apply -f` gains "runs the same routing-collision validation as deploy, scoped to the applied app"; `apply teams` gains "exits non-zero and writes nothing if any shrine manifest in the directory fails to parse or a Team fails validation"), `specs/progress.md` (fix entry), and `specs/features/integration-tests.md` (new scenarios). Cobra `Short`/`Long` strings are **not** changed.

**Rationale**: The guide currently documents the behaviour this feature removes, so leaving it would be actively wrong. Keeping the Cobra strings unchanged means the auto-generated `docs/content/cli/*.md` pages need no regeneration and the CLI drift check stays green without a `make docs-gen-cli` step.

**Alternatives considered**: extending `apply teams`' `Long` text to mention the strict exit — reasonable polish, but it couples this fix to the docs toolchain for one sentence; can be done separately.
