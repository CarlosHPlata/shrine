# Feature Specification: Strict Apply Failures and Scoped Routing-Collision Detection

**Feature Branch**: `025-fix-apply-parse-collision`
**Created**: 2026-08-24
**Status**: Draft
**Input**: GitHub issue [#36](https://github.com/CarlosHPlata/shrine/issues/36) — "fix: apply teams exits 0 on parse errors; apply -f bypasses routing-collision detection"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - `apply teams` fails loudly when a team manifest is broken (Priority: P1)

An operator keeps their team manifests in the teams directory and runs `shrine apply teams` — often from a script or CI job — to sync them into platform state. One file has a typo in its `kind` (for example `Aplication`). Today the command prints an error line for that file, skips it, announces "Successfully synced N teams", and exits 0. The automation believes the sync succeeded, and the typo goes unnoticed until something downstream breaks. After this change the command exits non-zero, the error names the offending file and the problem, and nothing from that run is written to state — so the operator (or their pipeline) is stopped at the point of the mistake.

**Why this priority**: A declarative sync that silently drops files is the worst failure mode: manifests and state diverge and no automation can detect it. Spec 002 (FR-003/FR-004) already promised loud failure for any shrine-classified manifest with a parse or kind defect; `shrine apply teams` is the one command still violating that promise, and the current integration test pins the broken behaviour rather than the promised one.

**Independent Test**: Place a file with `apiVersion: shrine/v1` and `kind: Aplication` in a teams directory next to a valid team manifest, run `shrine apply teams`, and assert the command exits non-zero with an error naming the file and the offending kind.

**Acceptance Scenarios**:

1. **Given** a teams directory containing a shrine-classified file whose `kind` is missing, empty, or unrecognised, **When** the operator runs `shrine apply teams`, **Then** the command exits non-zero and the error identifies the file path and the offending kind value.
2. **Given** a teams directory containing a shrine-classified file that is malformed YAML or whose body cannot be parsed for its declared kind, **When** the operator runs `shrine apply teams`, **Then** the command exits non-zero and the error identifies the file path and the parse problem.
3. **Given** a teams directory with two broken files and one valid team manifest, **When** the operator runs `shrine apply teams`, **Then** both broken files are reported in that single run, the command exits non-zero, and no team from that run is written to state.
4. **Given** a Team manifest that parses but fails manifest validation (for example a missing name), **When** the operator runs `shrine apply teams`, **Then** the command exits non-zero and the error names the file and the validation problem.
5. **Given** a team that cannot be persisted to state (for example a storage failure), **When** the operator runs `shrine apply teams`, **Then** the command exits non-zero and the error names the team and the reason.
6. **Given** a teams directory that contains only valid team manifests — possibly alongside foreign YAML files and non-team shrine manifests such as Applications — **When** the operator runs `shrine apply teams`, **Then** behaviour is unchanged from today: the teams are synced, non-team shrine manifests are skipped with a notice, foreign files are reported as today, and the command exits 0.

---

### User Story 2 - `apply -f` rejects a manifest that collides with an existing route (Priority: P1)

An operator has deployed application A with `routing.domain: example.com`. Later they — or a colleague — apply a single manifest with `shrine apply -f appB.yml`, where B declares the same domain (or an alias with the same host and path prefix). Today the apply succeeds and both applications now compete for the same route at the gateway; the collision is only discovered later, when somebody else's deploy fails. After this change, `shrine apply -f` performs the same routing-collision validation that `shrine deploy` performs, and rejects B with the same diagnostic — naming both applications and the colliding host+path — before anything is deployed.

**Why this priority**: Single-file apply is the fastest way to introduce an undetected collision, and the collision then surfaces as a confusing failure on unrelated teams' deploys (Story 3). Closing the bypass makes collision detection a property of the manifests, enforced at every entry point — the parity already promised by spec 006 (FR-008a) and spec 016.

**Independent Test**: With a specs directory containing application A that owns `example.com`, run `shrine apply -f appB.yml --path <dir>` where B declares `example.com`; assert the command exits non-zero, the diagnostic names A, B, and the host+path, and B is not deployed. Then run `shrine deploy` on the same directory with B added and confirm the diagnostic identifies the same applications and host+path.

**Acceptance Scenarios**:

1. **Given** a specs directory in which application A declares a host+path, **When** the operator runs `shrine apply -f` with a manifest B whose primary domain and path prefix match A's, **Then** the command exits non-zero, the diagnostic names both applications and the colliding host+path, and B is neither deployed nor written to routing configuration.
2. **Given** application A declares an alias host+path, **When** B's primary domain or one of B's aliases matches that host+path, **Then** the apply is rejected in the same way — primaries and aliases are treated on equal footing.
3. **Given** the same colliding input, **When** the operator compares the `apply -f` diagnostic with the diagnostic `shrine deploy` produces for the same directory, **Then** both identify the same applications and the same host+path.
4. **Given** a manifest B whose routes collide with nothing in the directory, **When** the operator runs `shrine apply -f`, **Then** the apply proceeds exactly as today.
5. **Given** a specs directory in which two other applications C and D collide with each other but B collides with nothing, **When** the operator runs `shrine apply -f` for B, **Then** B is applied successfully — collisions that do not involve B do not block B.
6. **Given** a manifest B that is already present in the directory (a re-apply of an existing application), **When** the operator runs `shrine apply -f` for it, **Then** no collision is reported between B and itself.

---

### User Story 3 - Team-scoped deploy fails only on collisions involving its own applications (Priority: P2)

A platform operator hosts manifests for `ops`, `marketing`, and `platform` in one specs directory. A routing collision exists between two `marketing` applications (or between a `marketing` and a `platform` application). The `ops` operator runs `shrine deploy team ops`. Today the deploy aborts on the marketing collision that `ops` has nothing to do with and no power to fix. After this change, a team-scoped deploy validates its own applications against the routing footprint of the whole directory, as spec 019 (FR-010) intended: it fails when any `ops` application collides with anything (inside or outside `ops`), and it proceeds when the only collisions are entirely between other teams' applications.

**Why this priority**: Without this, a single unchecked collision anywhere in the directory blocks every team's deploys. Story 2 removes the main way such collisions get introduced; this story limits the blast radius when one exists anyway, so a team is never blocked by a conflict it did not cause and cannot resolve.

**Independent Test**: Build a specs directory with a collision between `marketing/a` and `marketing/b` and collision-free `ops` applications. `shrine deploy team ops` succeeds; `shrine deploy team marketing` fails naming `a` and `b`; the bare `shrine deploy` fails as it does today.

**Acceptance Scenarios**:

1. **Given** a collision entirely between applications outside team `ops`, **When** the operator runs `shrine deploy team ops`, **Then** the deploy proceeds without a collision error, and `shrine deploy team ops --dry-run` likewise reports no collision.
2. **Given** an `ops` application colliding with an application owned by another team, **When** the operator runs `shrine deploy team ops`, **Then** the deploy fails and the diagnostic names both applications and the colliding host+path.
3. **Given** two `ops` applications colliding with each other, **When** the operator runs `shrine deploy team ops`, **Then** the deploy fails and the diagnostic names both.
4. **Given** several collisions that each involve at least one `ops` application, **When** the operator runs `shrine deploy team ops`, **Then** every such collision is reported in that single run.
5. **Given** any collision in the directory, **When** the operator runs the bare `shrine deploy` (where every application is in scope), **Then** the deploy fails exactly as it does today.

---

### Edge Cases

- Teams directory contains no shrine-classified files at all: the existing "no team manifests found" message is printed and the command exits 0 — unchanged.
- Teams directory contains Application or Resource manifests that parse and validate correctly: they are skipped with a notice and are not errors for `apply teams`. An Application or Resource file that does *not* parse is still a loud failure, because every shrine-classified file must parse regardless of kind.
- Foreign YAML files (non-shrine `apiVersion`) in the teams directory remain silently skipped (spec 002 FR-002); they never cause a failure.
- `apply -f` with a Resource manifest: resources declare no routes, so the collision check has nothing to evaluate and the apply proceeds.
- `apply -f` when no directory context is available (no `--path` and no configured specs directory): the check runs over the single manifest alone; there is no footprint to collide with, so it passes.
- `apply -f` for a manifest whose name already exists in the directory: the application is evaluated once, never against itself.
- `apply -f` with a file outside the directory while the directory holds a same-named manifest with different routing: which declaration wins is pre-existing behaviour and out of scope here; the collision check evaluates whichever declaration the apply would actually deploy.
- Applications with no `routing.domain` and no aliases never participate in collision detection (spec 016 FR-006).
- A team-scoped deploy whose in-scope application collides with several out-of-scope applications: each colliding pair is reported.
- Team-scoped `--dry-run`: the collision outcome (pass or fail, and the diagnostic) is identical to the real team-scoped deploy, with no side effects.
- Collisions purely among out-of-scope applications are not surfaced by a team-scoped deploy or a single-file apply; the bare `shrine deploy` remains the command that validates the whole directory.

## Requirements *(mandatory)*

### Functional Requirements

**Strict `apply teams`**

- **FR-001**: `shrine apply teams` MUST exit non-zero when any shrine-classified manifest in the scanned directory cannot be parsed — malformed YAML, a missing/empty/unrecognised `kind`, or a body that fails to parse for its declared kind. The error MUST identify the file path and the reason, including the offending kind value when the kind is the defect (per spec 002 FR-003/FR-004).
- **FR-002**: `shrine apply teams` MUST exit non-zero when a Team manifest parses but fails manifest validation, with an error naming the file and the validation problem.
- **FR-003**: A single `shrine apply teams` run MUST report every failing file, not only the first one encountered, so the operator can fix all of them in one pass.
- **FR-004**: When any file fails under FR-001 or FR-002, the run MUST NOT write any team to state — all manifests are checked before any team is persisted. Teams already in state from earlier runs are unaffected.
- **FR-005**: A failure to persist a team to state MUST produce a non-zero exit with an error naming the team and the reason; it MUST NOT be reported as an informational line while the command succeeds.
- **FR-006**: Failures MUST be reported through the same error channel every other failing shrine command uses, not as informational lines on standard output followed by a success summary.
- **FR-007**: For a directory whose shrine-classified files all parse (and whose Team manifests all validate), behaviour MUST be unchanged: teams are synced, non-team shrine manifests are skipped with a notice, foreign files are reported as today, and the command exits 0.

**Collision detection on single-file apply**

- **FR-008**: `shrine apply -f <manifest>` MUST run routing-collision detection before deploying, evaluating the applied application's routes — its primary domain with path prefix and every alias host with path prefix — against the routing footprint of the full manifest set loaded from the specs directory.
- **FR-009**: When a collision involving the applied application is found, `shrine apply -f` MUST exit non-zero, MUST NOT deploy the application or write any routing configuration for it, and MUST emit the same diagnostic `shrine deploy` emits for the same input: the colliding host+path and both participating applications identified by owner and name.
- **FR-010**: When the applied application collides with nothing, `shrine apply -f` MUST behave exactly as today.

**Unified collision scope**

- **FR-011**: For every planning entry point, routing-collision detection MUST evaluate the in-scope step set against the routing footprint of the entire loaded manifest set, and MUST fail the command only for collisions in which at least one participant is in scope. In scope means: every application for the bare `shrine deploy` (and its dry-run); the applications owned by the requested team for `shrine deploy team <name>` (and its dry-run); the single applied application for `shrine apply -f`.
- **FR-012**: Collisions whose participants are all out of scope MUST NOT fail a team-scoped deploy or a single-file apply. Because every application is in scope for the bare `shrine deploy`, its behaviour is unchanged.
- **FR-013**: When several in-scope collisions exist, all of them MUST be reported in a single run, using the existing diagnostic format (spec 016 FR-003 through FR-005).
- **FR-014**: The dry-run form of a team-scoped deploy MUST produce the same collision outcome and diagnostic as the real team-scoped deploy, with no side effects.

**Regression coverage**

- **FR-015**: The existing integration test that accepts a zero exit from `shrine apply teams` on a bad-kind manifest MUST be updated to assert a non-zero exit in addition to the file path and offending kind appearing in the output.
- **FR-016**: Integration coverage MUST include: `shrine apply -f` rejecting a colliding manifest with no deployment side effects; `shrine apply -f` accepting a non-colliding manifest in a directory that contains an unrelated collision; a team-scoped deploy succeeding when the only collision is out of scope and failing when a collision is in scope. Unit coverage MUST exercise the scope rule for each planning scope (all, team, single application, single resource).

### Key Entities

- **Shrine-classified manifest**: A `.yaml`/`.yml` file whose `apiVersion` matches the shrine pattern (spec 002). Once a file self-identifies as shrine, every command that scans it must fail loudly on any parse or kind defect; it is never silently skipped.
- **Teams directory**: The directory `shrine apply teams` scans. It may contain Team manifests, other shrine manifests (Applications, Resources), and foreign YAML; only Team manifests are synced, but every shrine-classified file must parse.
- **Manifest set and routing footprint**: The full collection of manifests loaded for a command, and the set of every host+path each application in it claims (primary domain plus aliases). The footprint is the reference against which in-scope routes are checked.
- **In-scope step set**: The applications a command will actually act on — all of them for a bare deploy, one team's for a team-scoped deploy, one application for a single-file apply. Only collisions touching this set fail the command.
- **Routing collision**: Two different applications claiming the same host+path (primary or alias on either side). Identified in diagnostics by the host+path and both applications' owner/name.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of `shrine apply teams` runs over a directory containing at least one unparseable or invalid shrine-classified manifest exit non-zero, name every offending file in that single run, and write zero teams to state.
- **SC-002**: 100% of `shrine apply -f` invocations whose manifest collides with a route already declared in the directory are rejected before any deployment side effect, and the diagnostic identifies the same applications and host+path as `shrine deploy` does for the same fixture.
- **SC-003**: For a directory whose only collisions are between applications outside team T, `shrine deploy team T` succeeds in 100% of runs; when a collision involves a T application, it fails in 100% of runs and names every participant.
- **SC-004**: A routing collision can no longer be introduced through `shrine apply -f` and later surface on an unrelated team's deploy — zero such cross-team failures across the regression fixtures.
- **SC-005**: Collision-free manifest sets and all-valid team directories produce the same exit codes and output as today across the existing integration suite (zero regressions).
- **SC-006**: An operator can identify every problem — every broken file in a team sync, or every in-scope collision in a deploy — from a single command run, without re-running to discover the next one.

## Assumptions

- "Fail loudly" for `apply teams` is interpreted as validate-all-then-apply: every shrine-classified file is checked first and no team is written if any file fails. This mirrors how `shrine deploy` refuses to plan over a directory with a broken manifest, and avoids a half-synced state that reports failure. The cost — one typo blocks the whole sync until fixed — is the intended behaviour of a loud failure.
- Persistence failures and Team validation failures are included in the loud-failure rule even though the issue names only parse errors; they are the same defect class (a per-file error reported as a line of output while the command exits 0) and leaving them lenient would keep `apply teams` unreliable for automation.
- Non-team shrine manifests in the teams directory (Applications, Resources) are legitimately present and remain skipped with a notice; `apply teams` is not responsible for validating their bodies, only for requiring that they parse like any other shrine-classified file.
- The collision-detection algorithm, diagnostic wording, and host+path matching rules are the existing ones from specs 006, 012, and 016; this feature changes where the check runs and which collisions are fatal, not how collisions are computed.
- The scope rule (in-scope steps against the full footprint) is a single rule applied uniformly to every entry point, so `apply -f`, `deploy team`, and the bare `deploy` cannot drift apart again. Collisions entirely outside the scope are not surfaced by scoped commands; the bare `shrine deploy` remains the whole-directory validation.
- Single-file apply already loads the specs directory as resolution context (spec 019 FR-002/FR-015), so the footprint needed for the collision check is available without new configuration, flags, or commands.
- Which declaration wins when the `-f` file and a same-named manifest in the directory differ is pre-existing behaviour outside this feature's scope.
- Resources declare no routing and are never collision participants; the resource-scoped apply path only needs to be covered to prove the rule is a no-op there.
- No CLI flags, output formats, or commands are added or renamed. Documentation changes, if any, are limited to noting that `apply -f` now validates routing collisions like `deploy`.
