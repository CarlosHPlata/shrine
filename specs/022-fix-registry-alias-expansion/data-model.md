# Data Model: Expand `reg:` Registry Aliases Before the Container Is Created

**Feature**: `022-fix-registry-alias-expansion` | **Date**: 2026-08-14

No new persistent entities, manifest fields, or state records. The feature
corrects how one existing value flows through existing types.

## Image reference — one value, two forms

| Form | Example | Who sees it |
|---|---|---|
| **Alias form** | `reg:myregistry/traefik/whoami:latest` | Manifest authors; plan/dry-run output; `CreateContainerOp.Image` as handed to the backend |
| **Expanded form** | `docker.io/traefik/whoami:latest` | Everything inside `DockerBackend.CreateContainer` after the expansion point: pull, credential lookup, config hash input (via digest), container spec, error messages |

**Invariant (new, enforced by this feature)**: the expanded form is computed
exactly once per container creation, at the top of
`DockerBackend.CreateContainer`, and every subsequent reader of the image
reference within that creation reads the same expanded value. No reader of the
alias form exists downstream of the expansion point.

**Invariant (existing, now pinned by test)**: `configHash` depends on the
resolved image *digest*, never on the reference string — so the two forms of
the same image always hash identically (FR-010).

## Types touched (no shape changes)

- **`engine.CreateContainerOp`** (`internal/engine/backends.go`): unchanged.
  `Image` continues to carry whatever the manifest declared; the semantic
  clarification is that alias expansion is the container backend's
  responsibility, applied to its by-value copy.
- **`DockerBackend`** (`internal/engine/local/dockercontainer/docker_backend.go`):
  the `client` field's type changes from `*client.Client` to the new
  package-internal `dockerAPI` interface (12 methods, structurally satisfied by
  `*client.Client`). No field added or removed; `NewDockerBackend` behaviour
  unchanged.
- **`dockerAPI`** (new, `internal/engine/local/dockercontainer/docker_api.go`):
  test seam only — see research.md D2 for the method list. Not an engine-level
  backend interface; not exported.
- **`engine.Event`** (`internal/engine/events.go`): unchanged shape.
  `container.create` error events gain an `"image"` entry in their existing
  `Fields` map (see contracts/deploy-diagnostics.md).

## Validation rules

Unchanged, and required to stay bit-for-bit identical:

- Unknown alias → plan-time error naming the alias (`validateRegistryImages`).
- Empty alias (`reg:/…`) → error at both plan time and expansion time.
- Both the planner's `checkImageAlias` and the backend's
  `expandRegistryAlias` parse `reg:<alias>[/tail]` the same way (alias with no
  path segment is accepted by both and expands to the bare host); the spec's
  edge case demands these two never disagree, which the shared parsing rule
  already guarantees today.

## State transitions

None. Deployment records, secret stores, and subnet stores are untouched;
`recordDeployment` continues to run only after `ContainerStart` succeeds
(Principle VI unaffected).
