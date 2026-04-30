# Phase 1 Data Model: Fix Routing-Dir Manifest Scan Crash

This feature does not introduce new manifest kinds or persisted state. The "data model" here is the **classification taxonomy** the scanner applies to files it finds during a directory walk, plus the small Go types added to `internal/manifest/` to represent it.

## Classification Taxonomy

Every file encountered by `manifest.ScanDir` lands in exactly one bucket. The bucket is determined by two ordered checks; once a check disqualifies a file, no later check runs on it.

| Check order | Predicate | If FAIL → bucket | If PASS → next check |
|-------------|-----------|------------------|----------------------|
| 1. Extension | `filepath.Ext(path)` is `".yaml"` or `".yml"` (case-sensitive) | **Non-YAML Sibling** (silently dropped — file is never opened) | go to check 2 |
| 2. YAML well-formed | `yaml.Unmarshal(bytes, &TypeMeta{})` succeeds | **Malformed YAML** (loud error, FR-004) | go to check 3 |
| 3. `apiVersion` regex | `apiVersion` matches `^shrine/v\d+([a-z]+\d+)?$` | **Foreign YAML** (silently collected for the FR-006 notice, then dropped) | **Shrine Candidate** (proceed to `Parse` + `Validate`) |

Once a file reaches **Shrine Candidate**, it goes through the unchanged `manifest.Parse` → `manifest.Validate` pipeline. Defects from that point on (`kind` missing/unrecognised/typo, required-field violations, duplicate `metadata.name`) MUST surface as loud errors with the file path attached — this is FR-003 / FR-007 and is already implemented today.

```
file in walk
   │
   ├── ext != .yaml/.yml ──► [Non-YAML Sibling] ── skipped silently, never opened
   │
   ├── ext OK
   │     │
   │     ├── yaml.Unmarshal fails ──► [Malformed YAML] ── ERROR (file path + parse error)
   │     │
   │     └── yaml.Unmarshal OK
   │            │
   │            ├── apiVersion regex MISS ──► [Foreign YAML] ── skipped silently, listed in FR-006 notice
   │            │
   │            └── apiVersion regex HIT  ──► [Shrine Candidate]
   │                                              │
   │                                              ├── Parse → unknown Kind     ──► ERROR (FR-003)
   │                                              ├── Validate → missing field ──► ERROR (FR-003 / FR-007)
   │                                              ├── duplicate name in set    ──► ERROR (existing behaviour)
   │                                              └── all OK ──► added to ManifestSet
```

### Bucket cross-reference to spec entities

| Spec entity (spec.md §Key Entities) | Bucket above |
|-------------------------------------|--------------|
| Non-YAML Sibling                    | Non-YAML Sibling |
| Foreign YAML File                   | Foreign YAML |
| Mistyped-Shrine File (e.g. `Shrine/v1`) | Foreign YAML — deliberate trade-off (spec §Assumptions) |
| Shrine Manifest File                | Shrine Candidate |
| (Malformed YAML — spec §Edge Cases) | Malformed YAML |

## Go Types Introduced

All new types live in `internal/manifest/` so both `internal/planner/` and `internal/handler/` can import them without layer inversion.

### `Class`

```go
type Class int

const (
    ClassShrine  Class = iota // apiVersion matches shrine regex
    ClassForeign              // .yaml/.yml but apiVersion absent or non-matching
)
```

`Malformed YAML` is **not** a `Class` value — it is an error returned by `Classify`. `Non-YAML Sibling` is also not a class — those files never reach `Classify`.

### `ShrineCandidate`

```go
type ShrineCandidate struct {
    Path     string   // absolute file path, as returned by filepath.WalkDir
    TypeMeta TypeMeta // already-probed apiVersion + kind, reused by the caller's Parse
}
```

### `ScanResult`

```go
type ScanResult struct {
    Shrine  []ShrineCandidate // ordered by walk order (deterministic)
    Foreign []string          // .yaml/.yml paths whose apiVersion failed the regex
}
```

`Foreign` exists solely to power the FR-006 notice. Callers that do not care about the notice may ignore it.

### Public functions (the only new API surface)

```go
// IsShrineAPIVersion reports whether s matches ^shrine/v\d+([a-z]+\d+)?$.
// Pure function over a string — convenient for callers that already have a
// parsed TypeMeta (e.g. test assertions).
func IsShrineAPIVersion(apiVersion string) bool

// Classify reads `path` and returns its Class. Foreign returns (ClassForeign, nil, nil).
// Shrine returns (ClassShrine, &meta, nil) where meta is the probed top-level
// TypeMeta. Unparseable YAML returns (0, nil, error) with the file path wrapped
// into the error chain (FR-004).
func Classify(path string) (Class, *TypeMeta, error)

// ScanDir walks `dir` recursively, applies the .yaml/.yml extension filter, and
// classifies every admitted file via Classify. Returns the bucketed result.
// A single Malformed YAML aborts the scan with the file's error wrapped.
func ScanDir(dir string) (*ScanResult, error)
```

No new persisted state, no new manifest fields, no schema changes — `TypeMeta` and the three manifest kinds are unchanged.

## Validation Rules (mapped from FRs)

| FR | Validation rule | Where enforced |
|----|-----------------|----------------|
| FR-001(a) | Skip files whose extension is not `.yaml`/`.yml` (case-sensitive); do not open them | `ScanDir` (extension check before `Classify` is called) |
| FR-001(b) | Classify a file as Shrine **only** when `apiVersion` matches `^shrine/v\d+([a-z]+\d+)?$` | `Classify` via `IsShrineAPIVersion` |
| FR-002 | Foreign / non-YAML-sibling files MUST NOT raise an error; scan continues | `ScanDir` collects them and returns; `LoadDir` / `ApplyTeams` skip them |
| FR-003 | Shrine-classified files with bad `kind` fail loudly with file path + offending kind | unchanged — `manifest.Parse` and `manifest.Validate`, wrapped by `LoadDir` |
| FR-004 | Files admitted by extension filter that fail `yaml.Unmarshal` produce an error naming file + parse error | `Classify` returns the wrapped error; `ScanDir` propagates it |
| FR-005 | All directory-scanning code paths use the same classification | Only `ScanDir` does directory walks; `LoadDir` and `ApplyTeams` migrated |
| FR-006 | When ≥1 foreign file, emit one info-level notice listing the paths; never affects exit status | `LoadDir` / `ApplyTeams` (the callers, not `ScanDir`) |
| FR-007 | Existing valid-manifest behaviour unchanged | Reuse `Parse`/`Validate` unchanged; integration test fixture covers the no-foreign case |

## State Transitions

None — the scanner is stateless. A file's `Class` is a pure function of its current bytes on disk; nothing mutates between checks. `ManifestSet` (the in-memory output of `LoadDir`) is built once per scan and discarded after the deploy plan is computed; that lifecycle is unchanged from today.
