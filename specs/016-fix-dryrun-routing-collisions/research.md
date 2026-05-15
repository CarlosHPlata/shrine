# Research: Detect Routing Domain Collisions in `--dry-run`

**Phase**: 0 (Outline & Research)
**Feature**: 016-fix-dryrun-routing-collisions

The spec introduced no `[NEEDS CLARIFICATION]` markers. The unknowns at planning time were entirely about *where* and *how* to wire the existing collision check, which is resolvable from current source. Findings are recorded below as decisions so future maintainers see the rationale without re-deriving it.

## Decision 1 — Move `DetectRoutingCollisions` into `planner.Plan`

**Decision**: Invoke `planner.DetectRoutingCollisions` from inside `planner.Plan`, after `Resolve` returns no validation errors. The function's error becomes a single entry in `PlanResult.ValidationErr`.

**Rationale**:
- The collision is a property of the manifest set, not of any backend. `planner.Plan` is the function that already owns manifest-level validation (`Resolve`).
- Both `handler.Deploy` and `handler.DryRun` already call `planner.Plan` and already drain `PlanResult.ValidationErr` through an identical print-and-fail block (`deploy.go:46-52` and `deploy.go:104-110`). Putting the check there gives both paths the same diagnostic for free — directly satisfying SC-001.
- The check becomes independent of the routing backend (Principle III: backend-specific logic stays out of the planner; collision detection was never backend-specific to begin with).

**Alternatives considered**:
- **Add the call to `handler.DryRun` only**: rejected. Duplicates an invocation that already exists in `handler.Deploy`, multiplying the maintenance surface — and the original placement is itself the bug (gated on `routing != nil`). Principle VII (DRY) and the spec's FR-002 both push against this.
- **Add a new top-level `ValidateManifestSet` step in the planner pipeline**: rejected. YAGNI. Today there is exactly one such manifest-level check; an abstraction over one usage is premature (Principle IV).

## Decision 2 — Surface the result via `PlanResult.Error`, not `PlanResult.ValidationErr`

**Decision**: Return the error from `DetectRoutingCollisions` via `PlanResult.Error`. The handlers already pass `Error` straight back to cobra, which renders it to stderr.

**Rationale**:
- The pre-existing `handler.Deploy` returned the collision error directly to cobra, so the diagnostic appeared on **stderr**. The existing integration test `TestTraefikPlugin/should_fail_deploy_when_two_applications_collide_on_host+pathPrefix` (`tests/integration/traefik_plugin_test.go:647`) asserts against stderr. Routing the error through `ValidationErr` would have moved the diagnostic to stdout (under a `Validation errors:` header printed by the handler), silently breaking that test.
- Constitution Principle II: "errors → stderr". Validation feedback that ends the command IS an error; it should go to stderr, not stdout.
- The collision function already aggregates all collisions into a single multi-line error string. A single `Error` value preserves all collisions in one diagnostic — FR-005 ("report all collisions in one invocation") is satisfied without involving a slice.

**Alternatives considered**:
- **Use `PlanResult.ValidationErr`**: rejected. Routes the diagnostic to stdout via the handler's `Validation errors:` printing block, breaking the pre-existing traefik integration assertion and violating Principle II.
- **Split into one `ValidationErr` entry per collision**: rejected for the same reason as above, plus it would require changing `DetectRoutingCollisions`'s return signature (currently `error`, would become `[]error`). The existing single-error shape is well-tested and adequately user-friendly.

**Note**: An earlier iteration of this plan placed the collision in `ValidationErr`. That choice broke the pre-existing traefik collision integration test on CI; this decision corrects it.

## Decision 3 — Remove the redundant call from `handler.Deploy`

**Decision**: Delete the `if routing != nil { planner.DetectRoutingCollisions(result.ManifestSet) }` block in `internal/handler/deploy.go` (lines ~124-128). The planner is the new single source of truth.

**Rationale**:
- Leaving both call sites would mean the same collision is reported twice on a real deploy and once on a dry-run — breaking SC-001's "equivalent collision diagnostics" success criterion.
- Without removal, future changes to collision logic would need to update two call sites — exactly the duplication Principle VII warns against.

**Alternatives considered**:
- **Keep the handler call, return early in the planner if `set == nil`**: rejected. Same duplication problem, plus dead defensive code (`set` is never nil after a successful `Resolve`).

## Decision 4 — Test placement

**Decision**:
- **Unit test**: add a new case in `internal/planner/collisions_test.go` (or a sibling file) that calls `planner.Plan` against a small in-memory fixture and asserts `PlanResult.ValidationErr` contains the collision diagnostic. This proves the wiring, not just the algorithm.
- **Integration test**: add a new scenario inside `tests/integration/deploy_test.go`'s existing `TestDeploy` suite. The scenario runs `shrine deploy --dry-run` against a new `testdata/deploy/routing-collision/` fixture and asserts non-zero exit plus the collision diagnostic appearing in output. Authored before the planner edit (TDD per Principle V).

**Rationale**:
- The unit-level collision algorithm is already well covered by `collisions_test.go`. What is missing is coverage that `Plan()` *invokes* it — that is the bug being fixed.
- An integration test exercising the real binary against the real CLI surface is what Principle V's gate demands. Today's `TestDeploy` is the existing harbour for deploy-command scenarios; we extend it rather than create a new suite.

**Alternatives considered**:
- **Only an integration test**: rejected. The integration suite is slow (~10 min full run, per repo conventions). A planner-level unit test gives a fast regression signal.
- **A new top-level `tests/integration/dry_run_test.go`**: rejected. Premature splitting; one new scenario in the existing suite is the minimum coherent change.

## Decision 5 — Fixture layout

**Decision**: Create `tests/testdata/deploy/routing-collision/` containing two `Application` manifests with the same `routing.domain` (under the same `team-one` to avoid pulling in a new team setup). Reuse the existing `team/` fixture for `BeforeEach` team creation.

**Rationale**:
- The bug report explicitly notes that same-team and cross-team collisions both reproduce. Same-team is the simpler fixture and exercises the same code path.
- Keeping the team setup unchanged limits the diff and avoids touching `BeforeEach` flow in `TestDeploy`.

**Alternatives considered**:
- **Cross-team fixture**: rejected. Requires extending the team fixture or introducing a second team manifest. Same-team reproduces the bug; cross-team is covered by the existing unit tests (`TestDetectRoutingCollisions_PrimaryVsPrimary` uses `team-a` and `team-b`).
