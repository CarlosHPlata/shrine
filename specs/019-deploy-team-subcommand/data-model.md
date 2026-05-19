# Phase 1 Data Model

This feature does NOT touch any persisted state (Principle VI is untouched). The
"data model" here is the in-memory Go types that the planner exposes after the
refactor, plus how the existing types in `internal/planner` and
`internal/handler` change shape.

---

## 1. New type: `planner.Filter`

```go
// internal/planner/filter.go
package planner

type FilterKind int

const (
    FilterNone FilterKind = iota // all manifests in the set produce steps
    FilterTeam                   // only manifests whose metadata.owner == Name
    FilterApp                    // only the one Application named Name
    FilterRes                    // only the one Resource named Name
)

type Filter struct {
    Kind FilterKind
    Name string // empty when Kind == FilterNone; required otherwise
}
```

**Construction** — public constructors are the only blessed way to build a Filter:

```go
func NoFilter() Filter                // → Filter{Kind: FilterNone}
func ByTeam(name string) Filter       // → Filter{Kind: FilterTeam, Name: name}
func ByApp(name string) Filter        // → Filter{Kind: FilterApp, Name: name}
func ByResource(name string) Filter   // → Filter{Kind: FilterRes, Name: name}
```

**Validation**:

```go
func (f Filter) Validate(set *ManifestSet) error
```

Returns `nil` when:

- `FilterNone`
- `FilterTeam(name)` and at least one Application or Resource in `set` has
  `metadata.owner == name`
- `FilterApp(name)` and `set.Applications[name]` exists
- `FilterRes(name)` and `set.Resources[name]` exists

Returns a descriptive error otherwise. The `FilterTeam` not-found error
includes the sorted list of discovered owners (FR-008, SC-005).

**Equality / zero value** — `Filter{}` is `FilterNone` with empty Name; this is
the "all manifests" mode and is the safe default.

---

## 2. Modified type: `planner.ManifestSet`

The struct itself is unchanged:

```go
type ManifestSet struct {
    Applications map[string]*manifest.ApplicationManifest
    Resources    map[string]*manifest.ResourceManifest
}
```

Two changes to its API surface:

### 2a. New constructor

```go
func NewManifestSet() *ManifestSet
```

Returns a `*ManifestSet` with both maps allocated. Avoids inline struct
literals in handlers that build an empty set (today: only in
`PlanSingle`'s minimal-set branch).

### 2b. Promote `mapKind` to public `MergeManifest`

Today: `func (set *ManifestSet) mapKind(m *manifest.Manifest, path string) error`
(private; called only from `LoadDir` and `PlanSingle`).

After: `func (set *ManifestSet) MergeManifest(m *manifest.Manifest, path string) error`
(public; same body, better name). Handlers use it when they need to add a
single parsed manifest to a set that may already contain it from a directory
scan.

The function's behavior is unchanged:

- `Application` / `Resource` kind → add to the matching map, error on
  duplicate name (existing `ErrDuplicateManifest` semantic — emerge it as a
  named error so callers can ignore it cleanly).
- `Team` kind → no-op (teams are not part of the deployment plan).
- Unsupported kind → error.

**New named error** (extracted from today's inline error):

```go
var ErrDuplicateManifest = errors.New("manifest already present in set")
```

`MergeManifest` returns this when the named manifest is already in the set;
`handler.ApplySingle` checks `errors.Is(err, ErrDuplicateManifest)` and
treats it as a success (the file is already represented via `LoadDir`).

---

## 3. Modified function: `planner.Plan`

### Today

```go
func Plan(dir string, store state.TeamStore, registries []config.RegistryConfig) PlanResult

func PlanSingle(file, specsDir string, store state.TeamStore, registries []config.RegistryConfig) PlanResult
```

### After

```go
func Plan(set *ManifestSet, store state.TeamStore, registries []config.RegistryConfig, filter Filter) PlanResult

// PlanSingle is DELETED. No deprecation shim.
```

**Behavioral contract** (also captured in
[contracts/planner-api.md](./contracts/planner-api.md)):

1. **Validate filter against set.** Return `PlanResult{Error: err}` on
   failure.
2. **Resolve over full set** — call `Resolve(set, store, registries)`
   unchanged. Return `PlanResult{ValidationErr: errs}` if non-empty.
3. **Branch on filter kind**:
   - `FilterNone` or `FilterTeam`: `DetectRoutingCollisions(set)` (full set)
     → `Order(set)` (full set) → if `FilterTeam`, filter resulting steps to
     those whose `metadata.owner == filter.Name`.
   - `FilterApp` or `FilterRes`: return `PlanResult{Steps: []PlannedStep{{Kind,
     Name}}, ManifestSet: set}` — no collision check, no ordering. Matches
     today's `PlanSingle` behavior verbatim.

**Loading is the caller's job.** `Plan` no longer takes a directory path
or a file path — it operates on an already-loaded `*ManifestSet`. Callers
use `LoadDir(dir)` or `NewManifestSet() + MergeManifest(m)` depending on
their scenario.

---

## 4. Modified handler: `handler.Deploy`

### Today

```go
func Deploy(b *app.DeployBundle, manifestDir string) error
```

Calls `planner.Plan(manifestDir, b.Store.Teams, b.Cfg.Registries)` directly.

### After

```go
func Deploy(b *app.DeployBundle, manifestDir string, filter planner.Filter) error
```

Calls `planner.LoadDir(manifestDir)` then `planner.Plan(set,
b.Store.Teams, b.Cfg.Registries, filter)`. The rest of the body
(error rendering, step-count guard, engine call) is unchanged.

`handler.DryRun` mirrors the same change:

```go
// Today
func DryRun(out io.Writer, manifestDir string, store *state.Store, cfg *config.Config) error

// After
func DryRun(out io.Writer, manifestDir string, store *state.Store, cfg *config.Config, filter planner.Filter) error
```

---

## 5. Modified handler: `handler.ApplySingle`

### Today

```go
func ApplySingle(b *app.ApplyBundle, file, manifestDir string) error
```

Calls `planner.PlanSingle(file, manifestDir, b.Store.Teams, b.Cfg.Registries)`.

### After

Same signature; body now:

1. Parse + validate the file (unchanged from today's `PlanSingle` head).
2. Reject Team-kind with today's exact error message.
3. Build the set: `LoadDir(manifestDir)` then `set.MergeManifest(m, file)`
   (tolerating `ErrDuplicateManifest`), OR `NewManifestSet() +
   MergeManifest` when `manifestDir == ""`.
4. Build the Filter: `ByApp(name)` for Application kind, `ByResource(name)`
   for Resource kind.
5. Call `planner.Plan(set, b.Store.Teams, b.Cfg.Registries, filter)`.
6. Render errors + invoke engine — identical to today.

---

## 6. Modified CLI: `cmd/deploy.go`

### Today

One `deployCmd` with `cobra.NoArgs`. Calls `handler.Deploy(bundle, dir)`
(or `handler.DryRun` for `--dry-run`).

### After

```go
var deployCmd = &cobra.Command{
    Use:   "deploy",
    Short: "Deploy a project from a manifest directory",
    Args:  cobra.NoArgs,
    RunE:  runDeploy(planner.NoFilter()),
}

var deployTeamCmd = &cobra.Command{
    Use:   "team <name>",
    Short: "Deploy only the apps and resources owned by one team",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        return runDeploy(planner.ByTeam(args[0]))(cmd, args)
    },
}

func runDeploy(filter planner.Filter) func(*cobra.Command, []string) error {
    return func(cmd *cobra.Command, args []string) error {
        // resolve dir; print header (team-aware if filter.Kind == FilterTeam);
        // dispatch to handler.DryRun or handler.Deploy with filter
    }
}

func init() {
    deployCmd.PersistentFlags().BoolVarP(&dryRun, "dry-run", "d", false, "...")
    deployCmd.PersistentFlags().StringVarP(&deployPath, "path", "p", "", "...")
    deployCmd.AddCommand(deployTeamCmd)
    rootCmd.AddCommand(deployCmd)
}
```

Flags become persistent on `deployCmd` so `deployTeamCmd` inherits them
(FR-006). `runDeploy` is a closure factory — same shape `cmd/apply.go`
already uses for `applyCmd` vs. `applyTeamsCmd`.

---

## 7. Untouched types and packages

The following are unchanged by this feature and are listed for clarity:

- `internal/manifest/*` — types, parser, validator, scanner all unchanged.
- `internal/engine/*` — engine interfaces, backends, dry-run all unchanged.
- `internal/planner/resolve.go` — cross-team rules unchanged.
- `internal/planner/collisions.go` — routing collision detection unchanged.
- `internal/planner/order.go` — topological ordering unchanged.
- `internal/planner/templates.go` — env template handling unchanged.
- `internal/state/*` — no new state-store keys or migrations.
- `internal/app/*` — composition bundles unchanged; same `DeployBundle` /
  `ApplyBundle` types serve the new code path.
- `PlanTeardown` — operates on deployment state, not the manifest set;
  remains exactly as it is today (SC-007).

---

## 8. Relationships diagram

```text
                ┌──────────────────┐
                │   cmd/deploy.go  │
                │ ─ deployCmd      │── filter = NoFilter()
                │ ─ deployTeamCmd  │── filter = ByTeam(args[0])
                └────────┬─────────┘
                         │
                         │ handler.Deploy(bundle, dir, filter)
                         ▼
                ┌──────────────────────┐
                │ internal/handler     │
                │ ─ Deploy             │
                │ ─ DryRun             │── load set, call Plan, render, execute
                │ ─ ApplySingle        │── parse file, build set, build filter, call Plan
                └────────┬─────────────┘
                         │
                         │ planner.Plan(set, store, registries, filter)
                         ▼
                ┌─────────────────────────┐
                │  internal/planner       │
                │ ─ Plan(set, ..., filter)│
                │ ─ Filter / Validate     │
                │ ─ MergeManifest (exposed)│
                │ ─ Resolve, Collisions,  │
                │   Order (UNCHANGED)     │
                └─────────────────────────┘
```

The arrow direction shows ownership of decisions: the CLI chooses the
filter, the handler does I/O (load), the planner is pure (set + filter →
result). No layer reaches back into one above it.
