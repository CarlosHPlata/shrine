# Data Model: Detect Routing Domain Collisions in `--dry-run`

**Phase**: 1 (Design)
**Feature**: 016-fix-dryrun-routing-collisions

This change introduces no new persisted entities and no new manifest schema. It alters the behavior of one in-memory result type and reads from existing manifest fields. The relevant entities are catalogued here so reviewers can confirm scope.

## Entities (Touched by This Change)

### `PlanResult` (package `internal/planner`)

The in-memory return value of `planner.Plan`. Today:

| Field            | Type              | Meaning                                                                |
|------------------|-------------------|------------------------------------------------------------------------|
| `Steps`          | `[]PlannedStep`   | Ordered execution plan when validation succeeded.                       |
| `ManifestSet`    | `*ManifestSet`    | Resolved manifest set; passed to the engine and to existing handler-level collision check. |
| `Error`          | `error`           | Hard planner faults (load failure, ordering failure).                   |
| `ValidationErr`  | `[]error`         | Manifest-level validation failures from `Resolve`.                      |

**Change**: after `Resolve` succeeds, `planner.Plan` invokes `DetectRoutingCollisions`. If it returns non-nil, the error is appended to `ValidationErr` and `Plan` returns immediately (no `Steps`, no `ManifestSet` propagation to the engine). Field types and shape are unchanged.

**Invariant preserved**: when `len(ValidationErr) > 0`, callers MUST NOT execute `Steps`. Both `handler.Deploy` and `handler.DryRun` already enforce this.

### `ManifestSet` (package `internal/planner`)

The aggregated, post-`Resolve` view of all manifests in the target directory.

| Field          | Type                                            | Notes                                |
|----------------|-------------------------------------------------|--------------------------------------|
| `Applications` | `map[string]*manifest.ApplicationManifest`      | Read by collision check.             |
| `Resources`    | `map[string]*manifest.ResourceManifest`         | Not relevant to this feature.        |

**Change**: none. `DetectRoutingCollisions` already accepts `*ManifestSet` and reads `Applications[*].Spec.Routing.Domain`, `PathPrefix`, and `Aliases[]`. Behaviour preserved exactly.

### `ApplicationManifest.Spec.Routing` (package `internal/manifest`)

The manifest field whose duplication triggers a collision.

| Subfield      | Type             | Meaning                                                                          |
|---------------|------------------|----------------------------------------------------------------------------------|
| `Domain`      | `string`         | Primary hostname requested by the application. Empty means no primary route.     |
| `PathPrefix`  | `string`         | Optional path prefix; normalized by trimming trailing `/` before comparison.     |
| `Aliases`     | `[]RoutingAlias` | Additional `{Host, PathPrefix}` pairs the application also wants to terminate.   |

**Change**: none. Schema and validation rules are untouched.

## State Transitions

The only state transition affected is **plan-time validation outcome**. Before and after this change:

```text
LoadDir → Resolve → (NEW: DetectRoutingCollisions) → Order → return PlanResult
   │         │              │                          │
   │         │              │                          └─ on error → PlanResult{Error}
   │         │              └─ on collision → PlanResult{ValidationErr: [<collision err>]}
   │         └─ on resolve err → PlanResult{ValidationErr: [...]}
   └─ on load err → PlanResult{Error}
```

No Docker state, no on-disk state, and no in-memory `DeploymentStore` records are written on the failure path. The failure path is purely a returned error.

## Identifier Conventions

The collision diagnostic identifies each conflicting application using the existing identifier format already produced by `DetectRoutingCollisions`:

```text
<team>/<application-name>
```

(e.g. `team-one/app-a`). This format is established in `internal/planner/collisions.go` and its tests; this feature reuses it verbatim to keep the diagnostic stable for downstream tooling and the integration assertion harness.
