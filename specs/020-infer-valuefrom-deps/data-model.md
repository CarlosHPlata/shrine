# Data Model: Enrichment Layer

**Feature**: `020-infer-valuefrom-deps` | **Phase**: 1 (Design)
**Date**: 2026-05-20

This document specifies the internal types introduced by this feature
and how they relate to existing planner types. No persistent data model
changes (no YAML schema additions, no state-store keys).

---

## New types

### `Enricher` (interface)

```go
// Enricher transforms a ManifestSet into an enriched copy by adding
// inferred deploy-order dependencies according to its own rule.
// Implementations MUST NOT mutate the input set or any manifest within it.
//
// Enrich returns a non-nil error when the rule encounters a valueFrom
// reference that does not resolve to a same-team manifest in the loaded
// set and is not already covered by an explicit Spec.Dependencies entry
// on the consumer. The chain stops at the first such error (fail-fast).
type Enricher interface {
    Enrich(set *ManifestSet) (*ManifestSet, error)
}
```

- One method, pure function shape.
- Defined in `internal/planner/enrich.go`.
- Implementations live in sibling files (e.g., `enrich_valuefrom.go`)
  so a new rule never requires editing an existing rule's file (FR-007).

### `EnrichmentError`

```go
// EnrichmentError is the typed error returned by an Enricher when a
// valueFrom reference cannot be enriched and is not covered by an
// explicit Spec.Dependencies entry. It implements the error interface.
//
// EnrichmentError carries the structured fields needed for tests to
// assert on the offending reference without parsing the rendered
// message, and for handlers to render a clear stderr line.
type EnrichmentError struct {
    Kind          EnrichmentErrorKind
    ConsumerKind  string // manifest.ApplicationKind
    ConsumerName  string // consumer Application's metadata.name
    ConsumerOwner string // consumer Application's metadata.owner
    EnvName       string // env var on the consumer that triggered the failure
    TargetKind    string // "resource" | "application" (from the valueFrom prefix)
    TargetName    string // <name> from valueFrom: <kind>.<name>.<output>
    TargetOutput  string // <output> from valueFrom: <kind>.<name>.<output>
}

func (e *EnrichmentError) Error() string

type EnrichmentErrorKind string
const ErrCrossTeamOrUnresolvedValueFrom EnrichmentErrorKind = "cross-team-or-unresolved-valuefrom"
```

- `Kind` is a stable identifier tests can assert on via
  `errors.As(err, &target)`.
- `Error()` renders a single-line message of the form:
  `enrichment: app "<consumer>" env "<NAME>" references <kind> "<target>.<output>" which is not owned by team "<consumer-owner>"; add an explicit spec.dependencies entry (kind: <Kind>, name: <target>) to declare this dependency`
- Only one kind exists at landing; `EnrichmentErrorKind` is defined as
  a string type so future kinds can be added without breaking existing
  matchers.

### `InferredEdge`

```go
// InferredEdge records one inferred dependency edge for dry-run provenance.
// The planner emits these; the dry-run handler renders them with the
// "(inferred from env <NAME>)" tag.
type InferredEdge struct {
    Consumer ManifestRef // the Application that gained the edge
    Target   ManifestRef // the Resource or Application it now depends on
    EnvVar   string      // originating env var name on Consumer
}
```

- When multiple env vars on the same Application reference the same
  target, only one `InferredEdge` is emitted; `EnvVar` is the first one
  in declaration order (stable across runs — FR-013).
- Only emitted in the success path; on enrichment failure, the
  planner returns `Steps == nil` and `InferredEdges == nil`.

### `ManifestRef`

```go
// ManifestRef identifies a target manifest in the current set.
type ManifestRef struct {
    Kind  string // manifest.ApplicationKind or manifest.ResourceKind
    Name  string
    Owner string
}
```

- Defined in `internal/planner/enrich.go`.
- Mirrors the existing `manifest.Dependency` shape but lives in the
  planner package because it is internal to enrichment bookkeeping;
  the on-disk schema uses `manifest.Dependency`.

---

## Modified types

### `PlanResult` (`internal/planner/plan.go`)

```go
type PlanResult struct {
    Steps         []PlannedStep
    ManifestSet   *ManifestSet    // already present — now holds the ENRICHED set on success
    Error         error           // already present — now also carries *EnrichmentError on enrichment failure
    ValidationErr []error
    InferredEdges []InferredEdge  // NEW — populated by enrichment in the success path
}
```

- The existing `ManifestSet` field's contract changes subtly: callers
  receive the **post-enrichment** set on success. Existing callers
  (`handler.Deploy`, `handler.DryRun`, `handler.ApplySingle`) all
  pass `result.ManifestSet` straight into the engine; they never
  compare it against the original. The enrichment is intentionally
  invisible to them aside from the new edges on Application manifests
  they did not write — which is exactly the desired effect.
- On enrichment failure: `Steps == nil`, `ManifestSet == nil`,
  `InferredEdges == nil`, and `Error` is a `*EnrichmentError`. Handlers
  detect this with the same `if result.Error != nil` branch they
  already use for validation errors.
- No `Notices` field is introduced — the previous "nudge" channel has
  been removed in favour of fail-fast errors.

---

## Helper functions

All defined in `internal/planner/enrich.go` unless noted.

### `ChainEnrich`

```go
func ChainEnrich(set *ManifestSet, rules ...Enricher) (*ManifestSet, []InferredEdge, error)
```

- Threads the set through each rule. After each rule, the set returned
  by that rule becomes the input to the next.
- Stops at the first error from any rule and returns
  `(nil, nil, err)` unchanged. Subsequent rules are not invoked.
- Aggregates `[]InferredEdge` across rules in deterministic order
  (the order each rule produces them, which is itself deterministic
  per Decision 8).
- Idempotent on the success path: `ChainEnrich(set, R)` followed by
  `ChainEnrich(setOut, R)` yields a set whose dependency edges are
  identical to `setOut`'s (SC-005). On the failure path, a re-run on
  the same input produces the same first `*EnrichmentError` (SC-005,
  FR-013).

### `DefaultEnrichers`

```go
func DefaultEnrichers() []Enricher
```

- Returns the production rule chain in deterministic order:
  1. `enrichValueFromResource{}`
  2. `enrichValueFromApplication{}`
- Tests may supply their own chains. Plan() always uses `DefaultEnrichers()`.

### `copyManifestSetShallow`

```go
func copyManifestSetShallow(set *ManifestSet) *ManifestSet
```

- Allocates a new `*ManifestSet` and copies the `Applications` and
  `Resources` maps' entries by reference. Each map header is fresh; the
  pointer values inside are shared with the input.
- Called once by `ChainEnrich` before the first rule runs. Rules
  receive the shallow copy and replace map entries by pointer when
  they need to enrich an Application.

### `cloneApplicationWithDeps`

```go
func cloneApplicationWithDeps(
    app *manifest.ApplicationManifest,
    extra []manifest.Dependency,
) *manifest.ApplicationManifest
```

- Returns a new Application whose `Spec.Dependencies` is
  `append(copyOf(app.Spec.Dependencies), extra...)`. All other fields
  share their values with the input — slice headers in `Spec.Env`,
  `Spec.Volumes`, etc. are pointer-shared (they are immutable from the
  planner's view).
- Used by rule helpers when they have decided to enrich an Application.
- If `len(extra) == 0`, returns `app` unchanged (no-op — preserves
  pointer equality for the common case).

### `hasExplicitDependency`

```go
func hasExplicitDependency(deps []manifest.Dependency, kind, name string) bool
```

- Returns true if any entry in `deps` matches `(kind, name)`. Used by
  rule code to both dedup inferred edges against existing explicit ones
  AND to skip the FR-010 failure when an explicit entry already covers
  a cross-team reference.

---

## Rule-internal types (in `enrich_valuefrom.go`)

### `valueFromRef` (private)

```go
type valueFromRef struct {
    Kind   string // "resource" | "application"
    Name   string
    Output string
}

func parseValueFromRef(s string) (valueFromRef, bool)
```

- Returns `(_, false)` for `vault:…` strings, literal values, malformed
  strings, and any string that does not split into exactly three dot-
  separated non-empty parts with a known kind prefix. Rules that
  recognize only one prefix (resource OR application) further filter
  by `Kind`.
- A `false` return is the rule's signal to skip the env var silently
  (per FR-011) — vault and literal values never trigger the FR-010
  failure.
- Used by both `enrichValueFromResource` and `enrichValueFromApplication`.

### `applyEnrichmentRule` (private helper)

```go
func applyEnrichmentRule(
    set *ManifestSet,
    targetKind string,                              // "Resource" or "Application"
    lookupOwner func(name string) (owner string, exists bool),
    parseFor   func(string) (valueFromRef, bool),
) (*ManifestSet, []InferredEdge, error)
```

- Encapsulates the shared loop:
  1. Iterate Applications in sorted name order (Decision 8).
  2. For each Application, scan `Spec.Env` in declaration order.
  3. Call `parseFor(env.ValueFrom)`. If `false`, skip the env var.
  4. Filter parsed refs whose `Kind` does not match this rule
     (e.g., the resource rule skips `application.*` parses); skip silently.
  5. If `hasExplicitDependency(app.Spec.Dependencies, targetKind,
     ref.Name)` is true, skip the env var entirely (explicit wins;
     no inferred edge, no failure check).
  6. Call `lookupOwner(ref.Name)`. If the target does not exist OR
     its owner differs from `app.Metadata.Owner`, return
     `(nil, nil, &EnrichmentError{...})` — fail-fast.
  7. Otherwise append the inferred edge to the rule's accumulator
     and replace the Application in the working set via
     `cloneApplicationWithDeps`.
- Returns the enriched set, the inferred-edge provenance list, and
  the first error encountered (or `nil`).
- The two concrete rules are tiny wrappers around this helper:
  - `enrichValueFromResource.Enrich(set)` passes
    `targetKind = manifest.ResourceKind`, `lookupOwner` reading from
    `set.Resources`, and `parseFor` returning only refs where
    `Kind == "resource"`.
  - `enrichValueFromApplication.Enrich(set)` passes
    `targetKind = manifest.ApplicationKind`, lookups in
    `set.Applications`, refs where `Kind == "application"`.

This single shared helper is the DRY answer to Principle VII — the only
code touched per new rule is the wrapper that supplies the constants.

---

## Relationship diagram

```text
                ┌──────────────────────────────────────────┐
                │           planner.Plan(set, …)            │
                │                                            │
                │   filter.Validate(set)                     │
                │   Resolve(set, store, registries)          │
                │   ───────────────────────────────────────  │
                │   set, edges, err := ChainEnrich(set,      │
                │       enrichValueFromResource{},           │
                │       enrichValueFromApplication{},        │
                │   )                                        │
                │   if err != nil { return PlanResult{       │
                │       Error: err,  // *EnrichmentError     │
                │   } }                                      │
                │   ───────────────────────────────────────  │
                │   DetectRoutingCollisions(set)             │
                │   steps, _ := Order(set)                   │
                │   return PlanResult{                       │
                │       Steps:         steps,                │
                │       ManifestSet:   set,  // enriched     │
                │       InferredEdges: edges,                │
                │   }                                        │
                └──────────────────────────────────────────┘
                                  │
                                  ▼ (success path)
                ┌──────────────────────────────────────────┐
                │   handler.DryRun                          │
                │     - if result.Error != nil:             │
                │         print Error to ErrOut; exit non-0 │
                │     - print formatDeployPlan(             │
                │         result.Steps,                     │
                │         result.ManifestSet,               │
                │         result.InferredEdges,             │
                │       ) to Out                            │
                │     - engine.ExecuteDeploy(steps, set)    │
                └──────────────────────────────────────────┘
```

The pre-enrichment set is only visible to code that lives inside
`Plan()` between `Resolve` and `ChainEnrich`. Outside callers see the
enriched set on `PlanResult.ManifestSet` (success) or no set at all
(failure).

---

## Invariants (verified by unit tests)

1. **Input immutability** — for every Application `app` in the original
   set, `len(app.Spec.Dependencies)` and the contents of that slice are
   identical before and after `Plan()` returns, in both the success and
   failure paths (FR-005, SC-007). Verified by snapshotting the input
   set's deps before calling Plan and comparing afterwards.
2. **No file I/O** — running enrichment does not open any file for
   write under the manifests directory (SC-006). Verified by running
   enrichment against an in-memory `ManifestSet` and asserting the
   working directory's mtime is unchanged.
3. **Idempotence on success** — `ChainEnrich(ChainEnrich(set))` produces
   edges identical to `ChainEnrich(set)` (SC-005, success path).
4. **Idempotence on failure** — re-running `ChainEnrich` on the same
   input that previously failed produces a `*EnrichmentError` with the
   same `Kind` and identical structured fields (SC-005, failure path).
5. **Determinism** — repeated `Plan()` calls on the same input produce
   identical `InferredEdges` slices (success) or identical first
   `*EnrichmentError` (failure), byte for byte (FR-013).
6. **Same-owner gate** — for any inferred edge `(consumer, target)`,
   `consumer.Metadata.Owner == target.Metadata.Owner` (FR-002).
7. **Dedup correctness** — for every inferred edge added, there is no
   pre-existing explicit `Dependency` in the consumer's deps with the
   same `(Kind, Name)` (FR-006).
8. **Cross-team / unresolved failure** — for every `valueFrom`
   reference whose target is not same-team AND not covered by an
   explicit `Spec.Dependencies` entry, `Plan()` returns
   `PlanResult{Error: &EnrichmentError{Kind:
   ErrCrossTeamOrUnresolvedValueFrom, …}}` and `Steps == nil` (FR-010,
   SC-003).
9. **Explicit dep satisfies cross-team** — when the consumer's
   `Spec.Dependencies` contains an entry matching a cross-team or
   absent `valueFrom` reference under the `(Kind, Name)` dedup key,
   `Plan()` succeeds and no `*EnrichmentError` is raised for that
   reference (FR-010 second clause; US3 acceptance scenario 2).
