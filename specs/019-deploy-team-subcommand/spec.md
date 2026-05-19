# Feature Specification: Unified Planner Filter + `shrine deploy team <name>` Subcommand

**Feature Branch**: `019-deploy-team-subcommand`
**Created**: 2026-05-18
**Status**: Draft
**Input**: User description: "new feature, now deploy works standalone, but it should work with several sub commands, shrine deploy should work as today, shrine deploy team <teamname> should deploy one team dependencies"

## Clarifications

### Session 2026-05-18

- Q: For `shrine deploy team X`, when an in-scope manifest references a dependency owned by another team Y, what should the planner verify at plan time? → A: Manifest-only check — same rule as today's bare `shrine deploy`. The dep's manifest must be in the specs dir with correct owner and reachability (`networking.exposeToPlatform`); no deployment-state lookup is added. Engine-time will surface real failures if values are genuinely missing. Keeps `deploy team` and bare `deploy` behaviorally aligned and avoids YAGNI.
- Q: Should the planner gain a generic filter (none / team / app / res), and does this feature ship `apply res` / `apply app` CLI surfaces, or only `deploy team`? → A: Consolidate. Replace today's `Plan` + `PlanSingle` with a single `Plan(set, filter)` where `filter` is one of `{none, team:<name>, app:<name>, res:<name>}`. The single CLI surface added by this feature is `shrine deploy team <name>` (filter=team). `apply res` and `apply app` already exist at the CLI level via today's `shrine apply -f <file>` flow and remain unchanged at the CLI; what changes is that their handler (`ApplySingle`) is migrated off `PlanSingle` and onto the new `Plan(set, filter)` API with `filter=app:<name>` or `filter=res:<name>` derived from the parsed manifest's kind+name. Result: one planner, four call sites, zero behavior change for existing users beyond the new `deploy team` verb.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator deploys a single team's stack (Priority: P1)

A platform operator runs a homelab that hosts manifests for multiple teams in one specs directory (e.g., `marketing`, `ops`, `platform`). When the operator only wants to deploy the apps and resources belonging to one team — for example after editing only that team's manifests — they invoke `shrine deploy team <teamname>` and Shrine plans and applies just that team's stack. The bare `shrine deploy` command continues to deploy the entire specs directory as it does today, with no behavioral change for current users.

**Why this priority**: This is the entire purpose of the feature. Without it, operators must either deploy everything (slow, noisy, risks touching unrelated teams) or hand-pick `--file` invocations one manifest at a time (tedious and error-prone). Scoping deploys to a team is the natural next granularity between "everything" and "one file."

**Independent Test**: Given a specs directory containing manifests owned by two teams (`team-a` and `team-b`), running `shrine deploy team team-a` deploys only `team-a`'s apps and resources; `team-b`'s manifests are untouched in Docker and in the deployment state. Running `shrine deploy` (no subcommand) afterwards still deploys both teams.

**Acceptance Scenarios**:

1. **Given** a specs directory with applications and resources owned by `team-a` and `team-b`, **When** the operator runs `shrine deploy team team-a`, **Then** only the manifests whose `metadata.owner == team-a` are planned and executed, and the output header identifies the team scope (e.g., `Planning deployment for team: team-a`).
2. **Given** the same specs directory, **When** the operator runs `shrine deploy` (no subcommand, no arguments), **Then** behavior is identical to today: every manifest in the directory is planned and executed.
3. **Given** a successful `shrine deploy team team-a` run, **When** the operator inspects deployments via `shrine status`, **Then** only `team-a`'s applications appear as newly reconciled; previously deployed `team-b` containers are still running and untouched.

---

### User Story 2 - Operator previews a team-scoped deploy with `--dry-run` (Priority: P1)

The operator wants to see exactly what `shrine deploy team <teamname>` will do before applying any changes — same safety guarantee as today's `shrine deploy --dry-run`. The team subcommand MUST accept `--dry-run` and produce identical (but team-scoped) preview output: the list of planned steps, manifest set, and routing operations, with no side effects on Docker or routing files.

**Why this priority**: Principle II ("Every write operation MUST support `--dry-run`") makes this non-negotiable. Operators rely on dry-run as a final review gate; shipping a new write command without dry-run support would violate the constitution and erode trust.

**Independent Test**: Running `shrine deploy team team-a --dry-run` against a populated specs directory prints the planned step list (apps and resources owned by `team-a` only), produces no Docker calls, writes no routing files, and exits zero. State files and live containers are byte-identical before and after.

**Acceptance Scenarios**:

1. **Given** a specs directory with `team-a` and `team-b` manifests, **When** the operator runs `shrine deploy team team-a --dry-run`, **Then** the preview lists only `team-a`'s planned steps and the engine performs no mutations.
2. **Given** a manifest set with a routing-domain collision among `team-a`'s apps, **When** the operator runs `shrine deploy team team-a --dry-run`, **Then** the collision is detected and reported as a planning error (same behavior as today's full-directory dry-run).

---

### User Story 3 - Contributor consolidates the planner around a single filter contract (Priority: P1)

A Shrine contributor opens `internal/planner/plan.go` to extend the planner and finds **one** entry point — `Plan(dir, store, registries, filter)` — with a filter argument that selects either "all manifests" (today's `Plan`) or "exactly this manifest" (today's `PlanSingle`) or "this team's manifests" (the new behavior). The old `PlanSingle` no longer exists; `handler.ApplySingle` and `handler.Deploy` both delegate to the unified API by constructing the appropriate filter. The contributor doesn't have to choose which planner function to call or duplicate logic across two near-identical code paths.

**Why this priority**: Without this consolidation, `shrine deploy team` becomes a third near-duplicate plan function and the planner code rots. The user (operator) does not see this directly, but every future planner change (new validation, new ordering rule, new diagnostic) pays the cost of being implemented three times instead of once. Principle VII (Clean Code & DRY) mandates the extraction. The unified filter is the abstraction; this feature is the right moment to introduce it because it's the third concrete usage (`deploy`, `apply -f`, `deploy team`) — meeting Principle IV's "three or more concrete usages" bar.

**Independent Test**: After the refactor, grep across `internal/handler/` and `cmd/` shows no remaining calls to `planner.PlanSingle`; all callers go through `planner.Plan(set, filter)`. The existing test suites for `shrine deploy` and `shrine apply -f <manifest>` pass unchanged (no behavioral regression). A new unit test exercises each filter mode (`none`, `team:<name>`, `app:<name>`, `res:<name>`) and asserts the emitted step set matches expectation.

**Acceptance Scenarios**:

1. **Given** the refactored planner, **When** `planner.Plan(set, FilterNone)` is called, **Then** the returned `PlanResult` is identical (steps, ManifestSet, errors) to what today's `Plan(set)` returns for the same input — proving zero regression for the bare `shrine deploy`.
2. **Given** a manifest set with two applications and the refactored planner, **When** `planner.Plan(set, FilterApp{Name: "alpha"})` is called, **Then** the returned `PlanResult` is equivalent to today's `PlanSingle("alpha.yaml", dir, ...)` — proving zero regression for `shrine apply -f alpha.yaml`.
3. **Given** a manifest set spanning `team-a` and `team-b`, **When** `planner.Plan(set, FilterTeam{Name: "team-a"})` is called, **Then** only `team-a`-owned applications and resources appear in `PlanResult.Steps`, while `ManifestSet` retains the full directory for value resolution — proving the new `deploy team` behavior.

---

### User Story 4 - Operator gets a clear error when the team has no manifests (Priority: P2)

If the operator types a team name that no manifest in the specs directory claims as `metadata.owner` — either a typo (`shrine deploy team markting`) or a team that simply has no apps/resources defined — Shrine MUST exit non-zero with a clear message naming the requested team and listing the teams that *were* found in the directory. It MUST NOT silently succeed with an empty plan, because a silent no-op masks the typo and leaves the operator believing they deployed.

**Why this priority**: This is operator ergonomics, not a blocker for the core flow, but typos in team names are the most likely day-one mistake and a silent success is a high-cost UX failure (the operator walks away thinking the deploy ran).

**Independent Test**: With a specs directory containing only `team-a` manifests, running `shrine deploy team markting` exits non-zero, prints an error mentioning `markting` is unknown in this specs directory, and lists `team-a` as the team that was found. No Docker or routing side effects occur.

**Acceptance Scenarios**:

1. **Given** a specs directory whose manifests are all owned by `team-a`, **When** the operator runs `shrine deploy team team-b`, **Then** the command exits with a non-zero status and the error message names `team-b` as not found and lists the owners discovered (`team-a`).
2. **Given** an empty or manifest-less specs directory, **When** the operator runs `shrine deploy team anything`, **Then** the command exits with a non-zero status and the message indicates no manifests were found in the directory.

---

### Edge Cases

- **Cross-team dependencies**: An application owned by `team-a` declares a `Dependency` whose `owner` is `team-platform` (e.g., a shared Postgres resource). The team-scoped deploy MUST resolve that cross-team dependency from the loaded `ManifestSet` for value injection (env vars, templates, outputs) using the same rules `internal/planner/resolve.go` already applies — manifest presence, owner match, access list, and `networking.exposeToPlatform` for cross-team reachability. The cross-team dependency MUST NOT be re-deployed: its manifest is loaded purely as resolution context and no step is emitted for it. The planner does NOT perform a deployment-state lookup at plan time; if the cross-team dep has in fact never been deployed, the failure surfaces at engine-time (the dependent app's container will fail to reach the missing dependency) — identical to today's bare `shrine deploy` behavior.
- **Mixed manifest directory with foreign files**: Existing behavior of `manifest.ScanDir` (warning on non-shrine YAML) MUST be preserved unchanged for the team subcommand.
- **Routing collisions across teams**: When `shrine deploy team team-a --dry-run` runs, collision detection MUST still consider routes owned by *other* teams that are already accounted for in the loaded manifest set (so an operator cannot accidentally introduce a route that collides with another team's existing route). Reported collisions still surface as planning errors.
- **Team subcommand with `--path`**: The `--path` flag MUST work identically to today on the bare `shrine deploy` — it overrides the configured `specsDir`. Same precedence rules.
- **Team-name argument is missing**: `shrine deploy team` with no name MUST print usage help and exit non-zero (Cobra's standard behavior for required positional args).
- **Case sensitivity**: Team-name matching MUST be exact (case-sensitive), consistent with how `metadata.owner` is compared elsewhere in the planner.

## Requirements *(mandatory)*

### Functional Requirements

#### Planner Consolidation

- **FR-001**: The `internal/planner` package MUST expose a single planning entry point with a filter parameter: conceptually `Plan(set, filter) PlanResult`. The filter MUST express one of four cases — none (all manifests), team-by-name, application-by-name, resource-by-name. The exact API shape (struct vs. sum type vs. functional option) is a planning-phase concern, but the contract MUST be expressible as those four cases and no others.
- **FR-002**: The pre-existing `planner.PlanSingle` function MUST be removed; all of its callers (today: `internal/handler/apply.go`'s `ApplySingle`) MUST be migrated to the unified `Plan(set, filter)` API by constructing the appropriate `app:<name>` or `res:<name>` filter derived from the parsed manifest's kind+name. No call to `PlanSingle` MUST remain after this feature lands.
- **FR-003**: The unified planner MUST preserve today's observable behavior for both pre-existing call sites: `Plan(set, FilterNone)` MUST produce results equivalent to today's `Plan(dir, store, registries)`, and `Plan(set, FilterApp/Res)` MUST produce results equivalent to today's `PlanSingle(file, dir, store, registries)` for the same manifest.

#### `shrine deploy team` CLI

- **FR-004**: Shrine MUST add a new subcommand `shrine deploy team <teamname>` that delegates to the unified planner with `filter = team:<teamname>`.
- **FR-005**: The bare `shrine deploy` command (no subcommand) MUST continue to behave exactly as it does today — same flags, same scope (entire specs directory), same exit codes, same output format. No breaking changes for existing users or automation.
- **FR-006**: The new subcommand MUST accept the same flags as the bare `shrine deploy`: `--dry-run`/`-d` and `--path`/`-p`, with identical semantics.
- **FR-007**: When `filter = team:<teamname>`, the planner MUST load the full specs directory into the `ManifestSet` (used as resolution context for cross-team dependency references, env templates, and routing-collision detection) but MUST emit deploy steps **only** for applications and resources whose `metadata.owner` matches the requested team.
- **FR-008**: When the requested team has zero matching manifests in the specs directory, the command MUST exit non-zero with an error message that (a) names the requested team, (b) states it was not found, and (c) lists the team names discovered in the directory.
- **FR-009**: Cross-team dependency resolution MUST reuse the existing rules in `internal/planner/resolve.go` (`resolveDependencies` / `validateValueFrom`) unchanged: the cross-team dep's manifest must be present in the loaded `ManifestSet`, the declared owner must match, the consumer must satisfy the access list, and `networking.exposeToPlatform` must be true. No additional plan-time deployment-state lookup is introduced for cross-team deps — behavior matches today's bare `shrine deploy`.
- **FR-010**: Routing collision detection MUST run over the team-scoped step set against the full manifest set's routing footprint, so that a team-scoped deploy cannot introduce a route that collides with another team's already-defined route.
- **FR-011**: The `--dry-run` form of the team subcommand MUST produce no Docker, routing, DNS, or state-file side effects, identical to today's `shrine deploy --dry-run` guarantee.
- **FR-012**: Documentation MUST reflect the new verb across all surfaces operators consult:
  - **CLI help** (`shrine deploy --help` and `shrine deploy team --help`) MUST document the new subcommand, its argument, and its flags. (Provided automatically by Cobra once the command is registered.)
  - **Auto-generated CLI reference** under `docs/content/cli/` MUST be regenerated (`make docs-gen-cli`): `deploy.md` updated, new `deploy_team.md` added.
  - **Hand-written getting-started flow** (`docs/content/getting-started/quick-start.md`) MUST mention `shrine deploy team <name>` as the team-scoped variant of `shrine deploy`, at minimum in a short callout — not a full rewrite.
  - **`AGENTS.md`** at the repo root MUST list `shrine deploy team <name>` in its CLI quick-reference table, per the constitution's directive that `AGENTS.md` "MUST be kept consistent" with CLI changes.
  - **A new guide page** `docs/content/guides/team-scoped-deploy.md` MUST be added, covering: when to use team-scoped deploy, the cross-team-dependency rule (Clarification 1), and the typo-error UX. Linked from `docs/content/guides/_index.md`.
  - **Other guides** (`traefik.md`, `tls.md`, `custom-registries.md`, `secrets-vault.md`) that already reference `shrine deploy` do NOT need updates — they remain accurate because bare `shrine deploy` is unchanged.
- **FR-013**: The team-scoped deploy MUST emit an output header identifying the team scope (e.g., `Planning deployment for team: <teamname> from: <dir>`), distinct from the bare deploy's header, so operators can confirm at a glance which scope is in effect.

#### Tests

- **FR-014**: Unit tests under `internal/planner/` MUST exercise each filter mode (`none`, `team:<name>`, `app:<name>`, `res:<name>`) and assert the returned step set matches the expected subset of the manifest set.
- **FR-015**: Integration tests under `tests/integration/` MUST cover (a) team-scoped deploy of a single team in a multi-team specs directory leaves other teams' state untouched, (b) the bare `shrine deploy` continues to deploy all teams unchanged, (c) requesting an unknown team produces the expected error and no side effects, and (d) `shrine apply -f <manifest>` continues to apply exactly one manifest (proving the `PlanSingle` → `Plan(filter=app/res)` migration is behaviorally invisible).

### Key Entities

- **Plan filter**: A value supplied to `planner.Plan(set, filter)` that selects which subset of the loaded `ManifestSet` should produce deploy steps. It has exactly four shapes:
  - **None** — all applications and resources in the set produce steps. Used by the bare `shrine deploy`.
  - **Team(name)** — only applications and resources whose `metadata.owner == name` produce steps. Used by `shrine deploy team <name>`.
  - **App(name)** — only the single Application whose `metadata.name == name` produces a step. Used by `shrine apply -f <appfile>` after migration off `PlanSingle`.
  - **Res(name)** — only the single Resource whose `metadata.name == name` produces a step. Used by `shrine apply -f <resfile>` after migration off `PlanSingle`.

  In every case the filter only restricts which manifests yield deploy steps; the full `ManifestSet` is retained as resolution context for dependency lookup, env templating, and routing-collision detection. The filter is the single concept that replaces today's two-function `Plan` / `PlanSingle` split.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator with a specs directory containing N teams can deploy any one team's stack with a single command (`shrine deploy team <name>`) without touching any other team's containers, routing files, or deployment state.
- **SC-002**: For a specs directory with K applications, a team-scoped deploy that touches M of those (M < K) completes in time proportional to M, not K — operators do not pay deploy-time cost for manifests outside the requested scope.
- **SC-003**: Zero regressions in the bare `shrine deploy` workflow: existing integration tests that exercise `shrine deploy` and `shrine deploy --dry-run` continue to pass unmodified.
- **SC-004**: Zero regressions in the `shrine apply -f <manifest>` workflow: after migrating `ApplySingle` off `PlanSingle` and onto `Plan(set, filter=app/res)`, the existing apply-single integration tests pass unmodified.
- **SC-005**: When an operator mistypes a team name, the error message they see lets them correct it on the next try without consulting docs or grep'ing manifests — the message includes both the typo and the list of valid team names discovered in the directory.
- **SC-006**: Dry-run parity: a team-scoped dry-run produces zero side effects (verified by byte-comparing state files and listing Docker containers before and after).
- **SC-007**: After this feature, the `internal/planner` package exposes exactly one top-level planning function for the apps+resources lifecycle (`Plan`); `PlanSingle` is gone. `PlanTeardown` remains as-is because it operates on deployment state, not the manifest set.

## Assumptions

- **Team identity = `metadata.owner`**: A manifest "belongs to" team `T` when its `metadata.owner` field equals `T`. This is already the canonical ownership marker throughout the planner and engine; the new subcommand uses the same comparison rule (case-sensitive, exact match) without introducing a new concept.
- **Cross-team dependencies are resolved, not redeployed**: When a team-scoped deploy references a dependency owned by another team (Application or Resource), Shrine resolves that dependency from the loaded `ManifestSet` for env-var injection, output references, and template expansion — same code path as today's bare `deploy` (`internal/planner/resolve.go`). The subcommand is not transitive across team boundaries: no deploy step is emitted for the cross-team dependency, even if it has pending manifest changes. The operator is expected to have already deployed (or to separately deploy) the other team's stack. Plan-time does NOT verify that the cross-team dependency is currently running — that check is left to engine-time, matching bare-deploy behavior and Principle VI (Docker-Authoritative State).
- **Team manifests themselves are out of scope for `deploy`**: As today, `Team` kind manifests are platform-sync artifacts handled by `shrine apply teams`, not by `shrine deploy`. The new subcommand follows the same rule — it does not create or update the Team record itself; it only deploys the team's Application and Resource manifests.
- **CLI surface change is limited to one new verb**: This feature adds exactly one new Cobra subcommand: `shrine deploy team <name>`. The `apply -f <file>` flow, `shrine apply teams`, `teardown`, `status`, `describe`, `get`, `delete`, `create`, and `generate` are unchanged at the CLI level. Internally, `handler.ApplySingle` is migrated from `planner.PlanSingle` to `planner.Plan(set, filter)` — a refactor invisible to operators. Future per-name CLI subcommands (e.g., `shrine apply app <name>` or `shrine apply res <name>`) are not delivered here; the unified planner makes them trivial follow-ups but they ship in a separate spec.
- **No new manifest fields**: This is a CLI ergonomics feature, not a manifest schema change. Existing `metadata.owner` semantics are sufficient.
- **`shrine.yml` configuration unchanged**: No new config keys are introduced; `specsDir`, `--path`, and config-discovery rules behave the same as today.
- **Docs regeneration is part of the implementation**: Per the constitution's CLI self-documentation rule and Workflow item 7, `docs/public/cli/` content is regenerated as part of implementing this feature so `shrine deploy team` is discoverable in the published CLI reference.
