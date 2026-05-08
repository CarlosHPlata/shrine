# Research: Registry Aliases

**Feature**: 014-registry-alias | **Date**: 2026-05-08

## Findings

### Decision 1 — Where does alias validation happen?

**Decision**: In the planner (`internal/planner/resolve.go`), as a new
`validateRegistryImages` step called from `Plan`/`PlanSingle` after the existing
`Resolve` pass.

**Rationale**: The planner is already the single gate for all manifest validation
(dependency resolution, quota enforcement, access control). Placing alias validation
here ensures it fires on both the dry-run path (`DryRun` calls `planner.Plan`) and
the live path (`Deploy` calls `planner.Plan`). Errors surface before any Docker
operation is attempted.

**Alternatives considered**:
- At config load time (`Config.Load`) — rejected: config load happens before any
  manifests are read; there is nothing to validate aliases against.
- Inside the resolver (`internal/resolver`) — rejected: the resolver is invoked
  per-manifest during execution, not as a batch validation pass; errors would be
  discovered later and only for the current manifest.
- Inside the engine (`engine.go`) — rejected: the engine is execution-time only;
  errors would not surface on dry-run.

---

### Decision 2 — Where does alias expansion happen?

**Decision**: Inside `DockerBackend`, in both `ensureImage` and `resolveImage`,
immediately before the image reference is passed to the Docker SDK.

**Rationale**: This is the correct Constitution III placement — infrastructure logic
in the backend, not in engine core. The dry-run backend (`DryRunContainerBackend`)
receives the raw `reg:` string and prints it as-is with zero code changes, which is
the desired dry-run behaviour confirmed in clarification Q1.

**Alternatives considered**:
- In `engine.go` before building `CreateContainerOp` — rejected: would expand the
  alias for the dry-run path too (dry-run backend prints `op.Image`), contradicting
  the user's requirement that dry-run shows the alias.
- In `resolver.go` — rejected: the resolver handles env vars and output templates,
  not image references; expanding there would couple image logic into the wrong layer.

---

### Decision 3 — Config validation: `ValidateRegistries()` method or inline in `Load`?

**Decision**: New `ValidateRegistries() error` method on `*Config`, called explicitly
by handlers after `Load`. This mirrors the existing pattern: `Load` unmarshals
without side effects; callers validate as needed.

**Rationale**: Keeps `Load` a pure unmarshal function (consistent with current
behaviour). Handlers that deliberately want a partial or legacy config can skip
validation; the main `cmd` path always calls it.

**Alternatives considered**:
- Inline validation in `Load` — rejected: breaks the established pattern; `Load`
  currently returns an empty struct for missing files without error, suggesting it
  intentionally avoids validation.

---

### Decision 4 — Alias lookup data structure

**Decision**: Build a `map[string]string` (alias → host) once in the planner
validation step and once in the docker backend. No shared cache.

**Rationale**: The registry list is tiny (≤10 entries in practice). A map built
inline from a slice is zero additional complexity and needs no global state.

**Alternatives considered**:
- Pre-computed map stored on `Config` — rejected: YAGNI; adds a derived field that
  needs to stay in sync with the slice.
- Linear scan — rejected: map lookup is idiomatic Go and clearer at the call site.

---

### Decision 5 — Alias format validation location

**Decision**: `Config.ValidateRegistries()` validates alias format (alphanumeric,
hyphens, underscores only; non-empty if provided) and uniqueness. This runs at
startup before any manifest is loaded.

**Rationale**: Config errors should be caught as early as possible. Format and
uniqueness are config-layer concerns independent of manifests.
