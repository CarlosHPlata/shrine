# Quickstart — Backend lifecycle finalize step

**Feature**: 018-routing-backend-finalize
**Date**: 2026-05-17

Two audiences: (a) operators verifying nothing regressed after the refactor,
and (b) contributors adding a new routing backend on top of the new
interface.

---

## A. Operator: verify nothing regressed (SC-003)

The change is an internal refactor. There is no new flag, no new manifest
field, no migration. After this feature merges, running `shrine deploy` on an
existing setup MUST produce the same outcome as before.

### 1. Sanity-check against the existing integration suite

```bash
# requires a working local Docker daemon
make test-integration
# or, equivalently:
go test -tags integration ./tests/integration/...
```

All pre-existing tests (`deploy_test.go`, `teardown_test.go`,
`traefik_plugin_test.go`, `apply_test.go`, ...) MUST pass without changes to
manifests, configs, or invocation flags. This is SC-003.

### 2. Round-trip a real deploy

```bash
# from a fresh checkout of the branch
go build -o ./shrine .

# against your existing manifests directory
./shrine deploy --dir /path/to/your/specs
```

Expected: identical outcome to pre-feature behavior — containers running,
Traefik serving routes. The only operator-visible difference is one new
observer log line for `routing.finalize` (status `info` / `success`) at the
end of the deploy.

### 3. Round-trip a dry-run

```bash
./shrine deploy --dry-run --dir /path/to/your/specs
```

Expected: same per-route `[ROUTE]  WriteRoute: ...` lines as before, plus one
new line at the end:

```
[ROUTE]  Finalize
```

This is FR-008 — dry-run remains a faithful preview.

---

## B. Contributor: add a new routing backend

The whole point of this feature is that a new routing implementation needs
zero edits outside its own package (SC-001).

### 1. Implement the interface

```go
// internal/plugins/gateway/myroute/routing.go
package myroute

import "github.com/CarlosHPlata/shrine/internal/engine"

type RoutingBackend struct {
    // your implementation state
}

func (r *RoutingBackend) WriteRoute(op engine.WriteRouteOp) error {
    // stage a route (do not commit yet)
    return nil
}

func (r *RoutingBackend) RemoveRoute(team, host string) error {
    // stage a removal (do not commit yet)
    return nil
}

func (r *RoutingBackend) Finalize() error {
    // commit everything staged this invocation —
    // batch-publish to a remote API, atomic-swap a directory,
    // or just `return nil` if nothing to do
    return nil
}
```

That is the entire required surface area. No other interface needs to be
satisfied.

### 2. Wire it into the deploy bundle

`internal/app/components.go` contains a single factory (`routingFromPlugin`)
that returns the routing backend the bundle injects into the engine. Swap your
implementation in there. The handler, the CLI command code, and the engine
remain untouched (SC-001).

### 3. Add an integration test

Create `tests/integration/myroute_plugin_test.go` using the existing
`NewDockerSuite` harness. It MUST:

- Run the real `shrine` binary as a subprocess.
- Use a real Docker daemon (no mocks — Principle V).
- Assert that after `shrine deploy`, your backend's committed state is
  observable (whatever "committed" means for your backend).
- Assert that a forced `Finalize` failure causes `shrine deploy` to exit
  non-zero AND the operator-facing log contains a `routing.finalize` event
  with `status=error` (SC-004).

### 4. Verify the engine does NOT call your `Finalize` on partial failure

A unit test in `internal/engine/engine_test.go` already covers this (the
fake routing backend's `Finalize` MUST NOT have been called when the step
loop fails). Make sure your integration tests don't depend on the opposite
behavior.

---

## What changed under the hood

For reviewers / future maintainers:

| Before | After |
|--------|-------|
| `handler.Deploy` calls `b.TraefikPlugin.Deploy()` after `engine.ExecuteDeploy`. | `handler.Deploy` ends at `engine.ExecuteDeploy`. The engine internally calls `Routing.Finalize()`. |
| `RoutingBackend` interface had `WriteRoute` + `RemoveRoute`. | `RoutingBackend` adds `Finalize() error`. |
| Traefik's static-config-write + container creation lived in `traefik.Plugin.Deploy()`. | Same body lives in `traefik.RoutingBackend.Finalize()`. `Plugin.Deploy()` is removed. |
| Dry-run printed `WriteRoute` lines only. | Dry-run also prints `[ROUTE]  Finalize` at the end. |
| Teardown ended at the step loop. | Teardown also calls `Routing.Finalize()` (no-op for Traefik today, hook for future batching backends). |

There are no schema migrations, no configuration changes, and no operator-facing
behavior changes.
