# Contract: Planner API

**Feature**: `025-fix-apply-parse-collision` | **Date**: 2026-08-24
**Package**: `internal/planner` (internal API; consumers are `internal/handler` and the package's own tests)

## `DetectRoutingCollisions`

```go
// Before
func DetectRoutingCollisions(set *ManifestSet) error

// After
func DetectRoutingCollisions(set *ManifestSet, filter Filter) error
```

**Guarantees**

1. Considers every route key claimed by every application in `set` (primary `routing.domain` + normalised `pathPrefix`, and every alias `host` + normalised `pathPrefix`); applications with no domain and no aliases claim nothing.
2. Returns `nil` when no reported pair exists.
3. A pair `(first, later)` sharing a route key is **reported iff** `filter.isAppInScope(first) || filter.isAppInScope(later)`.
4. Error text: `routing validation failed:\n- <line>[\n- <line>…]`, lines sorted, each `routing collision: host=%q pathPrefix=%q declared by %q and %q` with refs `owner/name` in lexical order. **Unchanged** from spec 016.
5. `DetectRoutingCollisions(set, NoFilter())` returns exactly what `DetectRoutingCollisions(set)` returns today for the same set (byte-identical error text or `nil`).
6. Pure: does not mutate `set`; deterministic for a given input.

## `Filter.isAppInScope` (new, unexported)

```go
func (f Filter) isAppInScope(app *manifest.ApplicationManifest) bool
```

| `f.Kind` | Result |
|----------|--------|
| `FilterNone` | `true` |
| `FilterTeam` | `app.Metadata.Owner == f.Name` |
| `FilterApp` | `app.Metadata.Name == f.Name` |
| `FilterRes` | `false` |
| other | `false` (unreachable after `Filter.Validate`) |

Comparison is exact (case-sensitive), matching `filterStepsByOwner` and `Filter.Validate`.

## `Plan` (ordering guarantee)

```go
func Plan(set *ManifestSet, store state.TeamStore, registries []config.RegistryConfig, ports PortContext, filter Filter) PlanResult
```

Sequence, for **every** `filter.Kind`:

1. `filter.Validate(set)` → `PlanResult{Error}` on failure
2. `Resolve(set, store, registries)` → `PlanResult{ValidationErr}` on failure
3. `ChainEnrich(set, DefaultEnrichers()...)` → `PlanResult{Error}` on failure
4. `DetectHostPortCollisions(set, ports)` → `PlanResult{Error}` on failure
5. **`DetectRoutingCollisions(set, filter)` → `PlanResult{Error}` on failure** ← now runs for all four kinds
6. Kind-specific step emission (unchanged): `FilterNone`/`FilterTeam` → `Order` (+ `filterStepsByOwner`); `FilterApp`/`FilterRes` → single step

**Consequences for callers**

- `handler.ApplySingle` (FilterApp/FilterRes): a collision surfaces as `result.Error` before `Engine.ExecuteDeploy`; no handler change required.
- `handler.Deploy` / `handler.DryRun` (FilterNone/FilterTeam): unchanged call sites; team-scoped results no longer fail on out-of-scope pairs.
- `PlanResult` shape is unchanged.

## `handler.ApplyTeams` (behavioural contract)

```go
func ApplyTeams(manifestDir string, store state.TeamStore) error
```

| Condition | Return | `store.SaveTeam` calls |
|-----------|--------|------------------------|
| `ScanDir` error | that error (wrapped `searching for manifests: …`) | none |
| zero shrine candidates | `nil` (prints not-found notice) | none |
| any candidate fails `Parse`, or any Team fails `Validate` | `apply teams failed:\n- …` (one bullet per failure, scan order) | **none** |
| all parse/validate; a `SaveTeam` fails | `saving team %q to state: %w` | up to and including the failing one |
| all parse/validate/save | `nil` | one per Team manifest, in scan order |

Non-Team shrine candidates that parse are skipped (stdout notice) and never affect the return value.
