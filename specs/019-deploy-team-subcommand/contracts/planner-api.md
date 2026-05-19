# Contract: `internal/planner` Public API

This contract specifies the post-refactor public surface of `internal/planner`.
It is the binding document for `internal/handler` and any future caller; the
implementation in `plan.go` and `filter.go` MUST satisfy every clause below.

> Constitutional anchors: Principle II (Kubectl-Style CLI — thin dispatchers
> require a clear handler/planner boundary), Principle IV (YAGNI — exactly
> four filter kinds, no more), Principle VII (Clean Code & DRY — one plan
> function, one filter type).

---

## 1. Types

### 1.1 `FilterKind`

```go
type FilterKind int

const (
    FilterNone FilterKind = iota
    FilterTeam
    FilterApp
    FilterRes
)
```

**Contract**:
- `FilterNone` MUST be the zero value.
- The constants MUST be declared in this exact order (existing tests and
  switches MAY rely on values, though new code SHOULD always reference by
  name).
- No additional kinds MAY be added without updating this contract and
  re-evaluating the Constitution Check (Principle IV: three-or-more concrete
  usages).

### 1.2 `Filter`

```go
type Filter struct {
    Kind FilterKind
    Name string
}
```

**Contract**:
- A `Filter` value with `Kind != FilterNone` and `Name == ""` is invalid.
  `Filter.Validate` MUST return a non-nil error for it.
- The zero value `Filter{}` is equivalent to `NoFilter()` and is valid.
- The struct MUST remain exported and concrete. Callers MAY construct it
  inline if a constructor is inconvenient (e.g., table-driven tests).

### 1.3 `ManifestSet`

(Existing — unchanged structurally.)

```go
type ManifestSet struct {
    Applications map[string]*manifest.ApplicationManifest
    Resources    map[string]*manifest.ResourceManifest
}
```

**New methods added by this feature**:

- `func NewManifestSet() *ManifestSet`
- `func (set *ManifestSet) MergeManifest(m *manifest.Manifest, path string) error`

### 1.4 `PlanResult`

(Existing — unchanged.)

```go
type PlanResult struct {
    Steps         []PlannedStep
    ManifestSet   *ManifestSet
    Error         error
    ValidationErr []error
}
```

---

## 2. Constructors

### 2.1 `NoFilter`

```go
func NoFilter() Filter
```

Returns `Filter{Kind: FilterNone}`. Always valid. Equivalent to `Filter{}`.

### 2.2 `ByTeam`

```go
func ByTeam(name string) Filter
```

Returns `Filter{Kind: FilterTeam, Name: name}`. The constructor does NOT
validate `name`; callers receive the same struct regardless of input.
Validation against an actual `ManifestSet` happens in `Filter.Validate`.

### 2.3 `ByApp`

```go
func ByApp(name string) Filter
```

Returns `Filter{Kind: FilterApp, Name: name}`. Same contract as `ByTeam`.

### 2.4 `ByResource`

```go
func ByResource(name string) Filter
```

Returns `Filter{Kind: FilterRes, Name: name}`. Same contract as `ByTeam`.

---

## 3. `Filter.Validate`

```go
func (f Filter) Validate(set *ManifestSet) error
```

**Pre-conditions**: `set` MUST be non-nil. (Passing `nil` is a programmer
error — callers are expected to have loaded a set via `LoadDir` or
`NewManifestSet`.)

**Post-conditions**:

| f.Kind | f.Name | Pass conditions | Failure error |
|---|---|---|---|
| `FilterNone` | (ignored) | Always passes. | — |
| `FilterTeam` | `""` | — | `"team filter requires a non-empty name"` |
| `FilterTeam` | non-empty | At least one Application or Resource in `set` has `metadata.owner == f.Name`. | `"team %q not found in specs directory: known teams = [%s]"` when other teams exist; `"team %q not found: specs directory contains no Application or Resource manifests"` when set is empty. |
| `FilterApp` | `""` | — | `"application filter requires a non-empty name"` |
| `FilterApp` | non-empty | `set.Applications[f.Name]` exists. | `"application %q not found in manifest set"` |
| `FilterRes` | `""` | — | `"resource filter requires a non-empty name"` |
| `FilterRes` | non-empty | `set.Resources[f.Name]` exists. | `"resource %q not found in manifest set"` |
| any other | — | — | `"unknown filter kind: %v"` |

**Idempotency**: `Validate` MUST NOT mutate `set` or `f`.

**Error type**: Errors returned by `Validate` MUST be plain `error` values
(no need for sentinel error variables). Tests assert on substrings, not
identity.

---

## 4. `Plan` — the single planning entry point

```go
func Plan(set *ManifestSet, store state.TeamStore, registries []config.RegistryConfig, filter Filter) PlanResult
```

### 4.1 Pre-conditions

- `set` MUST be non-nil.
- `filter` MUST be a valid `Filter` (constructed via `NoFilter` / `ByTeam` /
  `ByApp` / `ByResource`, or a literal that matches the same shape).
- `store` and `registries` follow today's contracts (unchanged).

### 4.2 Execution order

The function MUST execute in this order. Any deviation is a contract
violation:

1. **Filter validation**:
   - Call `filter.Validate(set)`.
   - On error → return `PlanResult{Error: err}`. Stop.

2. **Dependency / quota / template resolution**:
   - Call `Resolve(set, store, registries)` — UNCHANGED helper.
   - If the returned slice is non-empty → return
     `PlanResult{ValidationErr: errs}`. Stop.

3. **Step emission — branched on `filter.Kind`**:

   - **`FilterNone`**:
     1. `DetectRoutingCollisions(set)` → on error, return `PlanResult{Error: err}`.
     2. `Order(set)` → on error, return `PlanResult{Error: err}`.
     3. Return `PlanResult{Steps: <all Order steps>, ManifestSet: set}`.

   - **`FilterTeam`**:
     1. `DetectRoutingCollisions(set)` (full set) → on error, return
        `PlanResult{Error: err}`.
     2. `Order(set)` (full set) → on error, return `PlanResult{Error: err}`.
     3. Filter the returned step slice to those whose corresponding manifest
        in `set` has `metadata.owner == filter.Name`. Order MUST be preserved.
     4. Return `PlanResult{Steps: <filtered>, ManifestSet: set}`.

   - **`FilterApp`**:
     1. Return `PlanResult{Steps: []PlannedStep{{Kind: manifest.ApplicationKind, Name: filter.Name}}, ManifestSet: set}`.

   - **`FilterRes`**:
     1. Return `PlanResult{Steps: []PlannedStep{{Kind: manifest.ResourceKind, Name: filter.Name}}, ManifestSet: set}`.

### 4.3 Behavioral equivalence guarantees

| Old call | New call (must be equivalent) |
|---|---|
| `Plan(dir, store, registries)` | `set, _ := LoadDir(dir); Plan(set, store, registries, NoFilter())` |
| `PlanSingle(file, "", store, registries)` for Application | `set := NewManifestSet(); m, _ := manifest.Parse(file); _ = set.MergeManifest(m, file); Plan(set, store, registries, ByApp(m.Application.Metadata.Name))` |
| `PlanSingle(file, "", store, registries)` for Resource | `set := NewManifestSet(); m, _ := manifest.Parse(file); _ = set.MergeManifest(m, file); Plan(set, store, registries, ByResource(m.Resource.Metadata.Name))` |
| `PlanSingle(file, dir, store, registries)` for Application | `set, _ := LoadDir(dir); m, _ := manifest.Parse(file); set.MergeManifest(m, file); Plan(set, store, registries, ByApp(name))` (where `ErrDuplicateManifest` is tolerated) |

"Equivalent" means: same `Steps`, same `ManifestSet`, same `Error`, same
`ValidationErr`. The integration tests for `shrine deploy` and `shrine
apply -f` validate this empirically.

### 4.4 Constraints

- `Plan` MUST NOT perform I/O. No file reads, no Docker calls, no network.
  All I/O is the caller's responsibility (typically `LoadDir` +
  `MergeManifest` in the handler).
- `Plan` MUST NOT mutate `set` after step emission begins. (It MAY add
  resolved metadata via existing `Resolve` calls; that's pre-existing
  behavior.)
- `Plan` MUST be safe to call concurrently with different `set` values.
  (Today's planner has no shared mutable state; the refactor preserves that.)

---

## 5. `LoadDir` and `MergeManifest`

### 5.1 `LoadDir` (existing — unchanged)

```go
func LoadDir(dir string) (*ManifestSet, error)
```

Unchanged contract: scan `dir` for shrine YAML files, parse + validate each,
populate the returned `ManifestSet`. Foreign-file warnings are emitted via
`manifest.ReportForeignFiles` as today.

### 5.2 `NewManifestSet` (new)

```go
func NewManifestSet() *ManifestSet
```

Returns a `*ManifestSet` with both maps allocated to empty (non-nil) maps.
The caller MAY use `MergeManifest` immediately.

### 5.3 `MergeManifest` (promoted from private `mapKind`)

```go
func (set *ManifestSet) MergeManifest(m *manifest.Manifest, path string) error
```

**Pre-conditions**: `set` non-nil with both maps allocated; `m` non-nil and
already validated by `manifest.Validate`.

**Behavior**:

| `m.Kind` | Effect | Error |
|---|---|---|
| `ApplicationKind` | Add to `set.Applications[m.Application.Metadata.Name]`. | `ErrDuplicateManifest` (wrapped with the name) if already present. |
| `ResourceKind` | Add to `set.Resources[m.Resource.Metadata.Name]`. | `ErrDuplicateManifest` (wrapped) if already present. |
| `TeamKind` | No-op (teams are not part of the deployment plan). | `nil` |
| (other) | — | `fmt.Errorf("unsupported manifest kind for deployment: %q (file: %s)", m.Kind, path)` |

**Sentinel error**:

```go
var ErrDuplicateManifest = errors.New("manifest already present in set")
```

Callers (notably `handler.ApplySingle`) MAY use `errors.Is(err,
ErrDuplicateManifest)` to detect the duplicate case and continue.

---

## 6. Deletion

The following symbol MUST be removed by this feature; no deprecation shim is
permitted (SC-007):

```go
// REMOVED — was internal/planner/plan.go
func PlanSingle(file, specsDir string, store state.TeamStore, registries []config.RegistryConfig) PlanResult
```

A grep for `PlanSingle` in `internal/`, `cmd/`, and `tests/` after the
refactor MUST return zero matches.

The private `(*ManifestSet).mapKind` method MUST be renamed in place to
`MergeManifest`; no wrapper of the old name may remain.

---

## 7. Untouched symbols (explicit exclusion list)

The following remain exactly as they are today and MUST NOT be modified by
this feature:

- `Resolve(set, store, registries) []error`
- `DetectRoutingCollisions(set) error`
- `Order(set) ([]PlannedStep, error)`
- `PlanResult` (struct fields)
- `PlannedStep` (struct fields)
- `PlanTeardownResult` (struct fields)
- `PlanTeardown(team, store) PlanTeardownResult`
- All helpers in `resolve.go`, `collisions.go`, `order.go`, `templates.go`

Any change to those symbols would broaden the blast radius beyond what this
spec authorizes.

---

## 8. CLI contract (downstream, for cross-reference)

For completeness — owned by `cmd/deploy.go`, not by the planner:

```text
shrine deploy [flags]
shrine deploy team <name> [flags]

Flags (both forms, persistent on `deploy`):
  -d, --dry-run       Dry run; show what would be done; no side effects.
  -p, --path string   Directory containing manifest files (overrides specsDir).

Exit codes:
  0   Success.
  1   Spec validation errors, planning errors (incl. unknown team), or engine failure.
  2   Usage error (e.g., `shrine deploy team` with no argument).
```

Output headers:
- `shrine deploy`: `[shrine] Planning deployment from: <dir>`
- `shrine deploy team <name>`: `[shrine] Planning deployment for team "<name>" from: <dir>`
