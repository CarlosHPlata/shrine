# Research: Remove Stale Dashboard Config on Dashboard Removal

**Feature**: `024-fix-dashboard-removal` | **Date**: 2026-08-23
**Input**: GitHub issue #35, spec.md, spec 010 (FR-008 + observer-events contract), current `internal/plugins/gateway/traefik/` implementation

No NEEDS CLARIFICATION markers existed in the Technical Context; the decisions below resolve the design choices the spec left open.

## Decision 1: Where the removal lives

**Decision**: Add an else-branch to `RoutingBackend.Finalize()` (`internal/plugins/gateway/traefik/routing.go:219`): when `!r.hasDashboard()`, call a new helper `removeStaleDashboardDynamicConfig(routingDir, observer)` placed in `config_gen.go` next to its mirror `generateDashboardDynamicConfig`.

**Rationale**: Finalize is exactly the lifecycle point that would otherwise regenerate the file, so create/update/remove become symmetric in one place. The helper lives beside the generator so the reserved filename (`dashboardDynamicFileName()`), the `isPathPresent` probe, and the event vocabulary stay in a single file. No engine or handler changes — the fix stays entirely behind the `RoutingBackend` interface (Constitution III).

**Alternatives considered**:
- *A new engine-level operation (e.g. `RemoveDashboard`)* — rejected: the engine has no dashboard concept; leaking one violates Principle III.
- *A standalone cleanup command* — rejected: spec FR-001 requires removal on the next deploy, not a new operator step (also Principle II YAGNI on new commands).

## Decision 2: Observer event name and shape

**Decision**: Emit `gateway.dashboard.removed` with `Status: engine.StatusInfo` and a single field `path` (absolute path of the removed file), after the removal succeeds.

**Rationale**: Spec 010's observer-events contract explicitly reserved this exact name for this fix ("if added later, the natural name is reserved here"). `StatusInfo` because the removal is successful convergence to the operator's requested state — not a problem to warn about. Emitting after `os.Remove` succeeds matches the plugin's write-then-emit convention (`gateway.route.generated`, `gateway.dashboard.generated`).

**Alternatives considered**:
- *`StatusWarning`* — rejected: warnings in this plugin mean "operator must act" (`gateway.route.orphan`); here Shrine acted.
- *Reusing `gateway.route.orphan`* — rejected: orphan is per-app route vocabulary with warn-only semantics; the dashboard file is Shrine-owned and actually deleted.

## Decision 3: Failure semantics (stat error, remove error)

**Decision**: Both a non-`IsNotExist` stat error and an `os.Remove` failure return a wrapped error from `Finalize` (`traefik plugin: removing stale dashboard dynamic file at %q: %w`), failing the deploy with the path and cause in the message. No warning-and-continue.

**Rationale**: Mirrors the generation side: `generateDashboardDynamicConfig` fails the deploy on stat and write errors. A stale dashboard file is security-relevant (old credentials keep working) — silently continuing past a failed removal is exactly the bug class this feature fixes (spec FR-006). The wrapped error satisfies "surface path and reason" without inventing a new error event.

**Alternatives considered**:
- *Warn-and-continue like `WriteRoute`'s `gateway.route.stat_error`* — rejected: that policy exists for operator-owned per-app files where deploys must not be blocked by one app's file; the dashboard file is Shrine-owned and its removal is the whole point of the branch.

## Decision 4: Removal trigger and scope boundary

**Decision**: The trigger is `!r.hasDashboard()` inside a `Finalize` that is already running (i.e. the gateway plugin is still active). `hasDashboard()` (`Dashboard != nil && Dashboard.Port > 0`) covers every production-reachable "would not generate" state: block deleted, or port unset/zero. Dashboard-with-port-but-no-credentials cannot reach Finalize — `traefik.New` fails validation first.

**Scope boundary (documented limitation)**: if the operator removes the *entire* `plugins.gateway.traefik` config, the plugin is inactive, no routing backend is constructed, and no cleanup can run — Shrine no longer knows the routing directory. This matches the spec's edge case ("routing directory does not exist → proceed") and spec 010's framing: cleanup is a responsibility of the still-active gateway plugin.

## Decision 5: Dry-run behavior

**Decision**: No dry-run code change. Dry-run deploys use the print-only `DryRunRoutingBackend` (`internal/engine/dryrun/dry_run_routing.go`), so the Traefik backend — and therefore the deletion — can never execute in dry-run. The pending removal is covered by the existing `[ROUTE]  Finalize` line, the same granularity at which dashboard *generation* is (not) itemized today.

**Rationale**: FR-007's hard requirement ("the file MUST NOT be deleted") is guaranteed architecturally. Itemizing the removal would require the dryrun package to probe the filesystem and know Traefik plugin internals, violating the print-only backend design (Constitution III) and adding plugin knowledge for one line of output (Constitution IV). Generation/removal parity in dry-run output is preserved.

**Alternatives considered**:
- *Teach `DryRunRoutingBackend` to print the planned dashboard removal* — rejected per above; would be the first filesystem-aware code path in the dryrun package.

## Decision 6: Dead code removal and test repointing

**Decision**:
- Delete `Plugin.portBindings()` (`plugin.go:128-142`) — zero production call sites; `RoutingBackend.portBindings()` (`routing.go:264`) is the live implementation used by `Finalize`.
- Keep the `removeFileFn` seam (`routing.go:20`) — it goes from dead to live as the removal helper's write seam, preserving the package's established unit-test isolation pattern (`writeFileFn`, `mkdirAllFn`, `lstatFn` are all stubbed in tests; unit tests never touch the real filesystem).
- Move/repoint `TestPlugin_PortBindings_OmitsTLS_WhenTLSPortUnset` and `TestPlugin_PortBindings_IncludesTLS443_WhenTLSPortSet` (`plugin_test.go:117,142`) to `routing_test.go` as `TestRoutingBackend_PortBindings_*`, constructing `&RoutingBackend{cfg: &cfg}` instead of `&Plugin{cfg: &cfg}`. Assertions carry over unchanged.
- Add one new case, `TestRoutingBackend_PortBindings_IncludesDashboardPort_WhenDashboardSet`, making the third combination from FR-009 (dashboard port) explicit — today it is only implicitly asserted via the Finalize test's `len(op.PortBindings) == 3`.

**Rationale**: Issue #35 scope, spec FR-008/FR-009/FR-010. The `captureRemoveFileFn` helper's comment ("removeFileFn must not be called" for `RemoveRoute`) stays true — the orphan-warn policy for per-app routes is untouched; only the dashboard branch calls the seam.

## Decision 7: Test strategy

**Decision**:
- **Unit (TDD, red-first)** in `internal/plugins/gateway/traefik/`: removal-branch tests using the existing stubs (`stubLstatPresent/NotExist/Error`, a capture variant of `removeFileFn`, `recordingObserver`): present→removed+event, absent→no-op+no event, stat error→Finalize error, remove error→Finalize error, dashboard-configured→no removal attempted; plus the repointed/new port-binding tests; plus an updated `TestRoutingBackend_Finalize_GeneratesConfigsAndCreatesContainer` sibling asserting the removal branch is *not* taken when the dashboard is configured. No filesystem access in any unit test (established package policy).
- **Integration (authored + compile-checked; CI executes)** in `tests/integration/traefik_plugin_test.go`: one scenario — deploy with dashboard (assert `__shrine-dashboard.yml` exists) → rewrite config without the `dashboard:` block → redeploy → assert the file is gone, per-app route files are untouched, and deploy output reports the removal. Follows the existing `writeConfig` + `tc.Run("deploy", ...)` harness pattern at `traefik_plugin_test.go:1100`.

**Rationale**: Constitution V (integration gate, TDD) combined with the project's working agreement that integration tests are authored locally but executed by the CI pipeline.
