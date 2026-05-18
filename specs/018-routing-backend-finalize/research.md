# Research — Backend lifecycle finalize step

**Feature**: 018-routing-backend-finalize
**Date**: 2026-05-17

This document resolves the NEEDS CLARIFICATION items from
[spec.md](./spec.md) and records the design decisions reached during
`/speckit-plan`. Each entry is a single decision with its rationale and the
alternatives considered.

## 1. Scope of the new lifecycle seam (FR-009)

**Decision**: Add `Finalize() error` only to `RoutingBackend`. Leave
`ContainerBackend` and `DNSBackend` unchanged.

**Rationale**: The only concrete usage of a deploy-scoped commit phase today
is the Traefik plugin's post-engine publish — a routing concern. No
container or DNS backend has a batching workflow that would benefit. Adding
a `Finalize()` method to all three interfaces now would force every existing
implementation (`local.ContainerBackend`, `dryrun.DryRunContainerBackend`,
DNS impls, dry-run DNS impl) plus every future one to carry boilerplate with
no caller, violating Constitution Principle IV (Simplicity & YAGNI — ≥3
concrete usages required).

**Alternatives considered**:
- *Uniform Finalize on all three backends*: clean mental model but rejected as
  a YAGNI violation; ~6 no-op methods across real and dry-run implementations
  with no functional caller, plus engine code to call them in a deterministic
  order.
- *Optional `Finalizer` interface with type-assertion in engine*: avoids
  boilerplate but introduces structural polymorphism the codebase doesn't use,
  and conflicts with Principle III's "named-interface" rule. Engine code becomes
  harder to read because each backend's lifecycle is no longer visible from its
  interface alone.

## 2. Teardown finalize semantics (FR-005)

**Decision**: `ExecuteTeardown` also invokes `Routing.Finalize()` exactly once
after a successful step loop. If the teardown step loop fails, finalize is not
called (mirrors FR-004). For the current Traefik backend, the teardown-time
Finalize is a no-op — per-route file removals are complete after the loop.

**Rationale**: The deploy/teardown shapes stay parallel, which makes the engine
trivially auditable: a maintainer reads one symmetric template. Backends that
batch removals (atomic swap, remote re-publish) gain a place to do so without
a follow-up spec.

**Alternatives considered**:
- *Deploy-only Finalize*: simpler short term but creates an asymmetry — a
  future backend that batches both adds and removes would need a new spec to
  add the teardown seam, and the engine would have two different shapes for
  what is morally the same flow.

## 3. Placement of Traefik container creation

**Decision**: The Traefik `RoutingBackend.Finalize()` writes the static and
dashboard dynamic config *and* creates the Traefik container via the
`ContainerBackend` it holds. This is the body of today's `Plugin.Deploy()`,
moved unchanged.

**Rationale**: Behavior-preserving 1:1 move keeps SC-003 trivially satisfied
(all pre-existing integration tests pass). The Traefik backend already
needs a `ContainerBackend` reference today (the plugin holds one); we pass
that same reference into the `RoutingBackend` struct at plugin construction
time. The composition root (`app.BuildDeployBundle`) is the only place that
knows about the cross-wiring, which keeps the engine and the handler clean.

**Alternatives considered**:
- *Split — Finalize writes config, container creation stays in a plugin
  method*: a "cleaner" interface but defeats the goal; the handler would still
  need a second plugin-specific call (`plugin.EnsureContainer()`) for the same
  lifecycle, reintroducing the leak FR-007 forbids.
- *Move container creation to a one-time bootstrap in `BuildDeployBundle`*:
  cleanest separation but changes today's behavior (the container is created
  on every deploy currently, which provides drift-correction for free). That
  behavior change is out of scope and would risk regressions in
  `traefik_plugin_test.go` scenarios that depend on idempotent re-creation.

## 4. Method name

**Decision**: `Finalize`.

**Rationale**: The spec uses "finalize" as a placeholder. We adopt it as the
final name. It is verb-first, agnostic about what the backend actually does
(commit vs. flush vs. publish vs. no-op), and pairs naturally with
`WriteRoute` / `RemoveRoute`. `Commit` would suggest transactional rollback
guarantees we explicitly aren't promising (see spec Assumptions). `Flush`
suggests buffering. `Publish` is too Traefik-specific.

**Alternatives considered**: `Commit`, `Flush`, `Publish`, `Apply`, `Close`.
All rejected for the reasons above.

## 5. Error context for finalize failures (FR-003, SC-004)

**Decision**: The engine emits an observer event named `routing.finalize` with
status `error` and a `error` field on failure, then returns the wrapped error
(`fmt.Errorf("routing finalize: %w", err)`). This matches the existing
`emitErr` pattern used for `routing.configure`, `container.create`, etc., and
makes the failure attributable in logs and exit output (SC-004).

**Rationale**: Reuses the existing `Observer` event protocol so terminal and
file-logger observers both get a structured signal. The event name is
distinct from any per-step event, satisfying "distinguish it from a per-step
error".

**Alternatives considered**: A new error type (`type FinalizeError struct{}`)
— rejected as boilerplate; the wrapped error + event name already give callers
all the discrimination they need.

## 6. Dry-run output shape (FR-008)

**Decision**: `DryRunRoutingBackend.Finalize()` prints
`[ROUTE]  Finalize\n` to its output writer.

**Rationale**: Matches the existing `[ROUTE]  WriteRoute: ...` and
`[ROUTE]  RemoveRoute: ...` style (two spaces after the bracket — consistent
with the current code). The line appears once at the end of the dry-run, which
faithfully previews the production lifecycle.

**Alternatives considered**:
- *No output*: rejected — dry-run would no longer be a faithful preview if a
  real lifecycle phase produced no signal.
- *Detailed "would publish" line listing each route*: rejected — duplicates
  the per-route `WriteRoute` lines, adds noise, and is not what the production
  Traefik Finalize actually does (it writes a static config, not a per-route
  enumeration).

## 7. Handling of zero-route deploys (Edge Case)

**Decision**: Finalize is always called on successful deploys, even when no
applications produced a route. Implementation cost is one method call; the
Traefik backend's Finalize is idempotent (mkdir is `MkdirAll`, static config
generation is overwrite-safe, container creation is handled by
`ContainerStore` semantics).

**Rationale**: Determinism — the engine flow is `loop → finalize` regardless
of loop body. Skipping Finalize when no routes were written would mean a
backend can be in two different states after a "successful" deploy, which the
spec explicitly forbids ("must not be left in a half-initialized state").

**Alternatives considered**: Skip Finalize when no route was written —
rejected for the reason above.

## 8. Integration test plan

**Decision**: Extend `tests/integration/traefik_plugin_test.go` with assertions
that the Traefik static config + container appear after deploy *and that no
code path reaches into `Plugin.Deploy()` from the handler*. Add a new scenario
that wires a failing-finalize routing backend (test double) into the bundle
and asserts: (a) deploy exits non-zero, (b) the operator-facing log contains a
`routing.finalize` event with status `error`. Verify with the standard
`NewDockerSuite` harness (Principle V).

**Rationale**: Direct mapping from FR-002/FR-003/FR-007 → integration test.
Black-box subprocess, real Docker daemon — no mocks.

**Alternatives considered**: Unit-only verification — rejected per Principle
V (integration test gate). Unit tests in `internal/engine/engine_test.go`
supplement (assert Finalize is invoked once on success and not invoked on
step-loop failure via fake routing backend), but they don't substitute.
