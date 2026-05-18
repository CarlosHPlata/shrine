# Data Model — Backend lifecycle finalize step

**Feature**: 018-routing-backend-finalize
**Date**: 2026-05-17

This feature is an internal interface refactor — there is no persistent
storage, no manifest schema change, and no on-disk file format change.
The "data model" here is the Go types and lifecycle states involved.

## Entities

### RoutingBackend (interface)

**Location**: `internal/engine/backends.go`

Contract that every routing implementation satisfies. After this change:

| Method | Signature | Purpose |
|--------|-----------|---------|
| `WriteRoute` | `WriteRoute(op WriteRouteOp) error` | Per-route write during the engine step loop (unchanged). |
| `RemoveRoute` | `RemoveRoute(team, host string) error` | Per-route removal during the engine teardown step loop (unchanged). |
| `Finalize` | `Finalize() error` | **NEW.** Deploy- and teardown-scoped commit phase invoked once at the end of `ExecuteDeploy` / `ExecuteTeardown` after the step loop completes successfully. May be a no-op. |

**Implementations after this change**:

- `internal/plugins/gateway/traefik.RoutingBackend` — `Finalize` writes the
  static `traefik.yml`, optionally generates the dashboard dynamic config, and
  creates the Traefik container via its held `ContainerBackend`. Today this
  body lives in `traefik.Plugin.Deploy()`; it moves into `RoutingBackend.Finalize`
  unchanged.
- `internal/engine/dryrun.DryRunRoutingBackend` — `Finalize` prints
  `[ROUTE]  Finalize` to `Out` and returns nil. Faithful preview of the
  production lifecycle for `shrine deploy --dry-run`.
- Test doubles (`internal/engine/engine_test.go`, future integration scenarios)
  — record the call for assertion purposes; no side effects.

### Deploy lifecycle (state ordering)

Phases of a single `shrine deploy` invocation, in order:

1. **Plan** — `planner.Plan(...)` produces `ManifestSet` and `[]PlannedStep`.
2. **Engine ExecuteDeploy**:
   1. Pre-resolve every resource.
   2. Create the platform network.
   3. For each step: deploy resource OR deploy application
      (`WriteRoute` + `WriteRecord` happen inside per-application flow).
   4. **`Routing.Finalize()`** — **NEW** — exactly once, after the step loop,
      only if every step succeeded. Skipped on step-loop error (FR-004).
3. Engine returns; handler returns to CLI; process exits.

Phases of a single `shrine teardown` invocation:

1. **Plan** (same shape).
2. **Engine ExecuteTeardown**:
   1. For each step: `RemoveContainer` and (for applications) `RemoveRoute`.
   2. **`Routing.Finalize()`** — **NEW** — exactly once, after the step loop,
      only if every step succeeded.
   3. Remove team network.

### Routing backend implementation (per concrete plugin)

Each concrete `RoutingBackend` chooses what its `Finalize` does:

| Implementation | Finalize body |
|----------------|---------------|
| Traefik (today's only real impl) | Write static config + dashboard dynamic config; create the Traefik container. |
| Dry-run | Print `[ROUTE]  Finalize` line. |
| Hypothetical cloud LB backend | Batch-publish accumulated routes to remote API. |
| Hypothetical atomic-swap backend | Rename staging dir → live dir. |
| Backend that has nothing to do | `return nil`. |

There is no required configuration, no scaffolding — a single one-line method
satisfies the contract for a "I have nothing to do" implementation (FR-006).

## Validation rules

- `RoutingBackend` is an optional engine dependency (`Engine.Routing` may be
  nil). When nil, the engine skips both per-route calls AND the new
  `Finalize` call — consistent with Principle III ("Nil backends MUST be
  silently skipped"). Same nil-guard pattern as the existing
  `WriteRoute`/`RemoveRoute` sites.
- `Finalize` is invoked at most once per `ExecuteDeploy` call and at most once
  per `ExecuteTeardown` call. The engine has a single call site for each.
- If `Finalize` returns a non-nil error, the engine emits a `routing.finalize`
  observer event with `status=error` and returns the wrapped error.
- If any step in the step loop fails, `Finalize` MUST NOT be called for that
  invocation (FR-004).

## State transitions

Not applicable in the persistent-state sense. The only transitional state is
intra-process: `engine.ExecuteDeploy` is the sole orchestrator and holds it on
the stack.

## Observer events affected

| Event name | Status | Trigger | Existing or new |
|------------|--------|---------|-----------------|
| `routing.finalize` | `info` (started) → `info` (success) OR `error` | engine wraps the `Routing.Finalize()` call | **NEW** |
| `routing.configure` | `info` | unchanged | existing |
| (all others) | — | unchanged | existing |

The exact start/success/error event shape follows the existing pattern in
`engine.deployApplication` (see `engine.go`).
