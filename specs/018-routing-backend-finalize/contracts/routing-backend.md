# Contract — `RoutingBackend` interface

**Feature**: 018-routing-backend-finalize
**Date**: 2026-05-17
**Type**: Go interface (in-process contract between `internal/engine` and
routing backend implementations)

This is the canonical contract a concrete routing backend MUST satisfy after
this feature lands. It supersedes the pre-feature interface (which had only
`WriteRoute` and `RemoveRoute`).

## Interface

```go
// internal/engine/backends.go

type RoutingBackend interface {
    WriteRoute(op WriteRouteOp) error
    RemoveRoute(team string, host string) error
    Finalize() error
}
```

`WriteRouteOp`, `AliasRoute` and the `RemoveRoute` parameters are unchanged
from the pre-feature interface; see `internal/engine/backends.go`.

## Method contracts

### `WriteRoute(op WriteRouteOp) error`

- **Unchanged from today.**
- Invoked once per application step that produces a route, during
  `Engine.ExecuteDeploy`'s step loop.
- Errors propagate up via the engine's `routing.configure` observer event.
- Idempotency: implementations SHOULD treat a pre-existing route file/record as
  a preservation case (the Traefik backend logs `gateway.route.preserved`).

### `RemoveRoute(team, host string) error`

- **Unchanged from today.**
- Invoked once per application step during `Engine.ExecuteTeardown`'s step
  loop.
- Errors propagate via the engine's `<kind>.routing_remove` observer event.

### `Finalize() error` — NEW

#### Ordering guarantees (engine side)

- The engine invokes `Finalize` **exactly once** per `ExecuteDeploy`
  invocation, **after** every step in the step loop has completed without
  error.
- The engine invokes `Finalize` **exactly once** per `ExecuteTeardown`
  invocation, **after** every step in the step loop has completed without
  error.
- If any step in the loop returns an error, the engine MUST NOT call
  `Finalize` for that invocation (FR-004).
- The engine calls `Finalize` even when the step loop produced zero
  per-route writes (i.e. no application had a `routing.domain`). Implementations
  MUST handle this case (return nil or perform the same idempotent commit they
  would otherwise perform).

#### Engine-side error handling

- If `Finalize` returns a non-nil error, the engine:
  1. Emits an observer event `routing.finalize` with `status=error` and a
     field `error=<err.Error()>`.
  2. Returns `fmt.Errorf("routing finalize: %w", err)` to the caller.
- The error MUST be distinguishable from any per-step error by event name
  (SC-004).

#### Implementation-side requirements

- Implementations MUST NOT panic on a no-op finalize.
- Implementations MUST be safe to call after zero, one, or many `WriteRoute`
  calls in the same invocation.
- Implementations MAY perform side effects (file writes, container creation,
  remote API calls) that they previously performed via plugin-specific
  lifecycle methods. After this feature lands there MUST be no other code path
  by which the deploy engine triggers those side effects (FR-007).
- A backend that has nothing to do MAY return `nil` directly. No
  scaffolding, no required configuration (FR-006).

## Nil-backend rule

- `Engine.Routing` MAY be nil. When nil, the engine MUST skip all three
  methods (`WriteRoute`, `RemoveRoute`, `Finalize`) without error and without
  emitting an error event. This matches Principle III ("Nil backends MUST be
  silently skipped"). The new `Finalize` site MUST follow the same `if
  engine.Routing != nil { ... }` guard pattern that the existing
  `WriteRoute` and `RemoveRoute` sites use.

## Consumers

| Consumer | Responsibility |
|----------|----------------|
| `internal/engine.Engine` | Calls `WriteRoute`, `RemoveRoute`, `Finalize` in the order documented above. Owns the nil-guard. |
| `internal/handler.Deploy` | Does NOT call `Finalize` directly. After this feature, the handler MUST NOT reference any plugin-specific lifecycle method (FR-007). |
| `internal/handler.Teardown` | Same — no direct lifecycle calls. |
| `internal/app.BuildDeployBundle` / `BuildTeardownBundle` | Wires the concrete `RoutingBackend` into the `Engine`. Does NOT invoke any lifecycle method itself. |

## Implementations after this feature

| Implementation | Package | Notes |
|----------------|---------|-------|
| `traefik.RoutingBackend` | `internal/plugins/gateway/traefik` | `Finalize` writes static config + dashboard config, then creates the Traefik container via the held `ContainerBackend`. Body moved from `traefik.Plugin.Deploy()`. |
| `dryrun.DryRunRoutingBackend` | `internal/engine/dryrun` | `Finalize` prints `[ROUTE]  Finalize\n`. |

## Test contracts

Conformance tests for any new `RoutingBackend` implementation:

1. `WriteRoute` is called N times → `Finalize` is called exactly once
   afterward → all per-route writes are observable as a coherent committed
   state (definition of "coherent" is implementation-specific; for Traefik
   it's "the dynamic dir contains exactly the written routes and the static
   config is present").
2. `WriteRoute` is called and then the caller skips `Finalize` → it is
   acceptable for the implementation to leave uncommitted state. The engine
   guarantees `Finalize` is reached on the success path; partial state on the
   failure path is intentional (FR-004).
3. `Finalize` is invoked with zero prior `WriteRoute` calls → returns nil
   without side effects, OR performs an idempotent no-op commit.
4. `Finalize` returns a non-nil error → engine surfaces it via a
   `routing.finalize` observer event with `status=error`.
