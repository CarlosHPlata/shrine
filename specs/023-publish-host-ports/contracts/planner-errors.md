# Contract: host-port collision errors (dry-run and deploy)

**Feature**: 023-publish-host-ports

`planner.DetectHostPortCollisions` returns a single joined error following the
`DetectRoutingCollisions` discipline: every conflict in the invocation, one line
each, deterministically sorted.

## Aggregate shape

```text
host port validation failed:
- host port collision: port 8080 declared by "media/jellyfin" and "ops/dashboard"
- host port reserved: port 8443 declared by "ops/edge" is reserved by the platform gateway
- host port taken: port 8090 declared by "ops/metrics" is already allocated to "media/photos"
```

## Message formats

| Case | Format |
|---|---|
| Duplicate explicit claims in the manifest set | `host port collision: port %d declared by %q and %q` |
| Explicit claim on a gateway-reserved port | `host port reserved: port %d declared by %q is reserved by the platform gateway` |
| Explicit claim on another app's persisted allocation | `host port taken: port %d declared by %q is already allocated to %q` |

App references are `owner/name`, matching the routing-collision precedent.

## Guarantees

- **Both paths, before any change**: the same detector runs from `planner.Plan`
  for `shrine deploy`, `shrine deploy --dry-run`, `shrine deploy team <name>`,
  and single-file apply. A returned error aborts planning; zero engine
  operations execute (FR-005, SC-002).
- **Completeness**: all conflicts of all three kinds are reported in one
  invocation (FR-006).
- **Determinism**: duplicate-claim pairs list apps in sorted order; lines are
  sorted before joining. Two runs over the same inputs produce identical output.
- **Self-adoption is not a conflict**: an app explicitly claiming the port its
  own persisted allocation already holds passes silently (FR-010).
- **Independence**: detection needs no routing backend, no Docker daemon, and
  performs no writes.

## Runtime (non-planner) errors

| Case | Surface |
|---|---|
| Automatic range exhausted | `AllocateHostPort` → `ErrNoAvailableHostPorts`, wrapped with team/app, emitted through the engine's existing error-event path; deploy of that app fails, no allocation recorded (FR-016) |
| Port held by a non-shrine process | Docker container start fails; the engine error event names the app and the underlying Docker error (spec edge case — not pre-detectable) |
| Store claim race (should be planner-caught) | `ClaimHostPort` → `ErrHostPortTaken`, wrapped with both app refs — defense in depth only |
