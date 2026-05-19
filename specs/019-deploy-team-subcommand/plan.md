# Implementation Plan: Unified Planner Filter + `shrine deploy team <name>` Subcommand

**Branch**: `019-deploy-team-subcommand` | **Date**: 2026-05-18 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/019-deploy-team-subcommand/spec.md`

## Summary

Consolidate `internal/planner.Plan` and `internal/planner.PlanSingle` into a single
filter-aware entry point: `Plan(set, store, registries, filter) PlanResult`. The
filter selects which manifests in the loaded `ManifestSet` emit deploy steps — one
of `NoFilter()`, `ByTeam(name)`, `ByApp(name)`, `ByResource(name)`. Resolution
context (dependency lookup, env templating, routing-collision detection) always
uses the full set; only step emission is filtered. The pre-existing `PlanSingle`
function is removed; `handler.ApplySingle` migrates onto the unified API by
constructing `ByApp`/`ByResource` from the parsed manifest's kind+name. A new
Cobra subcommand `shrine deploy team <name>` is added under `cmd/deploy.go`,
passing `ByTeam(name)` to the same handler that today serves bare `shrine deploy`.

Operator-visible surface change is exactly one new verb (`shrine deploy team`).
All other commands — `shrine deploy`, `shrine apply -f`, `shrine apply teams`,
`teardown`, `status`, `describe`, `get`, `delete`, `create`, `generate` — behave
exactly as today. The refactor's correctness is anchored by the existing
integration test suites for `deploy` and `apply -f`, which must pass unmodified
(SC-003, SC-004).

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: Standard library + existing internal packages
(`internal/planner`, `internal/manifest`, `internal/handler`, `internal/app`,
`cmd/`). No new third-party imports.
**Storage**: N/A — internal API change. No new state-store keys, no migration.
**Testing**: `go test ./...` for units (filter switch coverage, error messages);
`go test -tags integration ./tests/integration/...` via `NewDockerSuite` for the
integration gate (Principle V). Existing `deploy_test.go`, `apply_*_test.go`,
and any team-aware integration tests must pass unchanged.
**Target Platform**: Linux server with Docker daemon (homelab use case).
**Project Type**: CLI tool (`shrine`) — single-binary Cobra app.
**Performance Goals**: Refactor is behavior-preserving for bare `deploy` and
`apply -f`. New `deploy team <name>` plans in time proportional to M (team-owned
manifests), not K (full directory) — SC-002. Resolve still walks the full set
because cross-team deps need it (FR-009), but routing-collision detection and
ordering are the only O(K) costs paid; step execution is O(M).
**Constraints**: Zero operator-visible regression for the two pre-existing call
sites (SC-003, SC-004). The unified `Plan` function MUST cover all four filter
modes; `PlanSingle` MUST be deleted, not deprecated (FR-002, SC-007).
**Scale/Scope**: Touches ~15 files across code and docs.
- **Planner (~4)**: `internal/planner/plan.go`, `internal/planner/loader.go`
  (expose `MergeManifest`), new `internal/planner/filter.go`, new
  `internal/planner/plan_test.go` extensions.
- **Handlers (2)**: `internal/handler/deploy.go`, `internal/handler/apply.go`.
- **CLI (1)**: `cmd/deploy.go` — refactor `deployCmd`, add `deployTeamCmd`.
- **Tests (~3)**: `internal/planner/*_test.go` for unit coverage of each filter,
  `tests/integration/deploy_team_test.go` for the new subcommand,
  `tests/integration/deploy_test.go` and `tests/integration/apply_*_test.go`
  asserted as-is for regression coverage.
- **Docs (~5)**: auto-regenerated `docs/content/cli/deploy.md`, new
  `docs/content/cli/deploy_team.md` (both via `make docs-gen-cli`);
  hand-written `docs/content/getting-started/quick-start.md` (callout added),
  new `docs/content/guides/team-scoped-deploy.md`, and `AGENTS.md` (quick-ref
  table updated). See the Documentation Updates section below for the full
  cross-reference.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] N/A — `metadata.owner` already encodes team ownership. No new manifest fields. The new capability is purely a CLI/planner ergonomic. |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] Pass — `shrine deploy team <name>` is verb-noun-name (matches `shrine describe team`, `shrine get teams`). `--dry-run` and `--path` are inherited (FR-006, FR-011). |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] N/A — no engine changes; backends are untouched. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — the `Filter` abstraction has exactly three concrete call sites at landing time: bare `deploy` (`NoFilter`), `apply -f` migrated off `PlanSingle` (`ByApp`/`ByResource`), and the new `deploy team` (`ByTeam`). Meets the "three or more concrete usages" bar. We explicitly do NOT add per-name CLI subcommands (`apply app`, `apply res`) in this feature — those land in a follow-up spec when the third concrete user appears. |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass — `tests/integration/deploy_team_test.go` is added for the new verb; existing `deploy_test.go` and apply-single tests gate the refactor (must pass unmodified). FR-015 enumerates the four required scenarios. |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] N/A — no state writes added or reordered. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | [x] Pass — the refactor's whole point is DRY: today's `Plan` and `PlanSingle` are 80% duplicated logic. After refactor, the common path (Resolve → optional collision-detect → optional Order → step emission) lives in one function. Names follow the constitution: `NoFilter()`, `ByTeam()`, `ByApp()`, `ByResource()`, `filterStepsByOwner()`. The constructor functions read like sentences ("plan, ordered by team alpha"). |

> No violations. Complexity Tracking table is empty.

## Project Structure

### Documentation (this feature)

```text
specs/019-deploy-team-subcommand/
├── plan.md              # This file
├── research.md          # Phase 0 — API shape decisions
├── data-model.md        # Phase 1 — Filter / PlanResult / handler wiring
├── quickstart.md        # Phase 1 — operator + contributor walkthrough
├── contracts/
│   └── planner-api.md       # Phase 1 — Plan() contract + Filter constructors
├── checklists/
│   └── requirements.md      # Created by /speckit-specify
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/
├── planner/
│   ├── filter.go               # NEW — Filter struct, FilterKind, NoFilter/ByTeam/ByApp/ByResource constructors, Validate()
│   ├── filter_test.go          # NEW — unit: each constructor; Validate() against empty/missing/present manifests
│   ├── plan.go                 # MODIFIED — Plan(set, store, registries, filter); PlanSingle removed; PlanTeardown unchanged
│   ├── plan_test.go            # NEW — unit: each filter mode produces expected steps; cross-team resolution context preserved
│   ├── loader.go               # MODIFIED — expose MergeManifest (was private mapKind) for ApplySingle's load-with-extra-file path
│   ├── loader_test.go          # MODIFIED — coverage for MergeManifest's public contract
│   ├── resolve.go              # UNCHANGED
│   ├── collisions.go           # UNCHANGED
│   ├── order.go                # UNCHANGED
│   └── templates.go            # UNCHANGED
├── handler/
│   ├── deploy.go               # MODIFIED — Deploy() and DryRun() take a planner.Filter; load set, call Plan(set, ..., filter)
│   └── apply.go                # MODIFIED — ApplySingle parses file, loads set (dir + merge), builds ByApp/ByResource, calls Plan
└── manifest/                   # UNCHANGED

cmd/
└── deploy.go                   # MODIFIED — existing deployCmd refactored to pass NoFilter; new deployTeamCmd registered as subcommand with cobra.ExactArgs(1) and ByTeam(args[0])

tests/integration/
├── deploy_test.go              # UNCHANGED at the test level — exercises bare `shrine deploy`, gates SC-003
├── deploy_team_test.go         # NEW — exercises `shrine deploy team <name>` in a multi-team specs dir; verifies other teams untouched and unknown-team error
├── apply_file_test.go          # UNCHANGED at the test level — exercises `shrine apply -f`, gates SC-004
└── testutils/                  # UNCHANGED

docs/
├── content/cli/deploy.md                  # REGENERATED by `make docs-gen-cli` — now lists `team` subcommand under SEE ALSO
├── content/cli/deploy_team.md             # NEW (auto-generated) — `shrine deploy team` reference page (mirrors apply_teams.md style)
├── content/getting-started/quick-start.md # MODIFIED — short callout for `shrine deploy team <name>` after the bare `shrine deploy` example
├── content/guides/team-scoped-deploy.md   # NEW — hand-written guide: when to use, cross-team-dep rule, typo-error UX, dry-run preview
├── content/guides/_index.md               # MODIFIED — add link to the new guide page
└── public/                                # REGENERATED on `make docs` (Hugo build) — committed because GitHub Pages serves from here

AGENTS.md                                  # MODIFIED — CLI quick-reference table gains `shrine deploy team <name>` row
```

**Structure Decision**: This change is in-place — no new packages, no new top-level
directories. The only new file under `internal/planner/` is `filter.go` (plus its
test). All other modifications are in pre-existing files. The new Cobra command
nests under `cmd/deploy.go`'s existing `init()` via `deployCmd.AddCommand(deployTeamCmd)`,
mirroring how `cmd/apply.go` already nests `applyTeamsCmd`.

### Documentation Updates

The CLI verb change is a user-visible surface change, so docs are first-class
deliverables (FR-012), not a follow-up. The work splits cleanly into
**auto-generated** and **hand-written**.

**Auto-generated** — covered by running `make docs-gen-cli` once after the
Cobra wiring lands. The docsgen tool walks the live command tree:

| File | Action | Source of truth |
|---|---|---|
| `docs/content/cli/deploy.md` | Regenerated | `cmd/deploy.go`'s `deployCmd` declaration (`Use`, `Short`, `Long`, flags). SEE ALSO section auto-includes the new `team` subcommand. |
| `docs/content/cli/deploy_team.md` | New (auto-generated) | `cmd/deploy.go`'s new `deployTeamCmd` declaration. Mirrors the existing `docs/content/cli/apply_teams.md` pattern. |
| `docs/public/cli/deploy/`, `docs/public/cli/deploy_team/` | Regenerated | `make docs` (Hugo build) — committed because GitHub Pages serves from `docs/public/`. |

**Hand-written** — discoverability work that auto-gen can't do:

| File | Edit | Why |
|---|---|---|
| `docs/content/getting-started/quick-start.md` | Add a one-paragraph callout after the bare `shrine deploy` example: "If your specs directory contains manifests owned by multiple teams, `shrine deploy team <name>` deploys only one team's stack." Optionally show a 2-line example. | Quick-start is the first surface a new operator reads; they need to know the verb exists. |
| `docs/content/guides/team-scoped-deploy.md` | **NEW** guide page. Covers: (a) when to use vs. bare deploy; (b) the cross-team dependency rule (Clarification Session 2026-05-18, Q1 — manifest-only check, no plan-time state lookup); (c) the typo-error UX (FR-008, SC-005); (d) `--dry-run` parity (SC-006). Length: ~80–120 lines, similar density to `docs/content/guides/custom-registries.md`. | Operators need a single page that explains the *workflow* and edge cases, not just the flag reference. The auto-generated CLI page documents the verb; the guide explains the design. |
| `docs/content/guides/_index.md` | Add a link entry for `team-scoped-deploy.md` in the existing list. | Hugo's guide index needs to list the new page so it shows up in the sidebar. |
| `AGENTS.md` | Add a row to the CLI quick-reference table for `shrine deploy team <name>`. | Constitution mandates: "`AGENTS.md` MUST be kept consistent" with CLI changes (Governance section). |

**Out of scope for this feature** — other docs that already mention `shrine
deploy` (`docs/content/guides/traefik.md`, `tls.md`, `custom-registries.md`,
`secrets-vault.md`, `README.md`) do NOT need updates because bare `shrine
deploy` is unchanged (FR-005). Any drift there is a pre-existing concern
unrelated to this feature.

**Verification**: the integration test `tests/integration/deploy_team_test.go`
shells out to the real binary; after the test passes, a smoke check confirms
docs-gen freshness:

```bash
make docs-gen-cli
git diff --exit-code docs/content/cli/
# expected: empty diff (the regen produced no changes a human didn't already commit)
```

This is the same docs-freshness gate the project uses today; the spec's
SC-003 (zero regression in bare-deploy docs) and SC-007 (no `PlanSingle`
leftovers) compose naturally with it.

## Complexity Tracking

> *No Constitution Check violations — table empty.*

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
