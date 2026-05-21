# Contract: Enrichment API & Operator-Visible Output

**Feature**: `020-infer-valuefrom-deps` | **Phase**: 1 (Design)
**Scope**: Internal Go API contract within `internal/planner` + the
operator-visible output contract for dry-run and deploy.

This project ships a single binary; the relevant "external" contracts
are (a) the planner's package-public API consumed by `internal/handler`
and (b) the operator-visible stdout/stderr produced by `shrine deploy`
and `shrine deploy --dry-run`. There are no HTTP, RPC, or wire-format
contracts to record.

---

## 1. Planner package API

### `planner.ChainEnrich`

```go
// ChainEnrich applies the given Enrichers in order to a shallow copy
// of set, returning the enriched copy along with the inferred-edge
// provenance.
//
// The input set is observably unmodified after this call returns,
// regardless of success or failure:
//   - set.Applications and set.Resources map headers are the same
//   - every *manifest.ApplicationManifest reachable through the input
//     set has the same Spec.Dependencies length and contents as before.
//
// Rules in the chain operate on the previous rule's output, so an
// Application enriched by rule R[i] is observed by rule R[i+1] with the
// added edges already present in its Spec.Dependencies — this is how
// cross-rule dedup is achieved.
//
// Error handling (fail-fast, FR-010):
//   - If any rule returns a non-nil error (in practice an
//     *EnrichmentError), ChainEnrich stops and returns
//     (nil, nil, err). Subsequent rules are not invoked and
//     subsequent offending references in the same run are not
//     enumerated.
//   - Determinism: for a given input set and rule chain, the same
//     first error is produced on every run (sorted iteration order,
//     Decision 8).
//
// On success, ChainEnrich is idempotent: ChainEnrich(out, rules...)
// where out is the output of a previous ChainEnrich on the same rules
// produces a set with identical dependency edges and an identical
// (possibly empty) []InferredEdge.
//
// ChainEnrich never writes to disk and never invokes user-provided
// callbacks; it is a pure function of set and rules.
func ChainEnrich(
    set *ManifestSet,
    rules ...Enricher,
) (enriched *ManifestSet, edges []InferredEdge, err error)
```

**Errors**: On success, `err == nil`. On failure, `err` is a
`*EnrichmentError` with `Kind == ErrCrossTeamOrUnresolvedValueFrom`
and structured fields naming the offending reference. Callers can
extract the typed error via `errors.As(err, &target)` for structured
inspection in tests; handlers can simply call `err.Error()` for the
operator-facing message.

### `planner.DefaultEnrichers`

```go
// DefaultEnrichers returns the production rule chain in deterministic
// order. Callers (in particular planner.Plan) should pass the returned
// slice verbatim into ChainEnrich.
//
// The chain is, in order:
//   1. enrichValueFromResource — for valueFrom: resource.<name>.<output>
//   2. enrichValueFromApplication — for valueFrom: application.<name>.<output>
//
// Tests MAY construct alternative chains for isolation.
func DefaultEnrichers() []Enricher
```

### `planner.Plan` — extended contract

The signature is unchanged:

```go
func Plan(set *ManifestSet, store state.TeamStore, registries []config.RegistryConfig, filter Filter) PlanResult
```

The contract is extended:

- On success, `PlanResult.ManifestSet` holds the **enriched** set,
  not the loaded one. Callers that need to inspect the enriched
  dependency graph (e.g., for the dry-run summary) MUST use this field.
  `PlanResult.InferredEdges` carries the provenance list for the
  dry-run renderer.
- On enrichment failure, `PlanResult.Error` is a `*EnrichmentError`,
  `PlanResult.Steps == nil`, `PlanResult.ManifestSet == nil`, and
  `PlanResult.InferredEdges == nil`. Handlers detect this with the
  existing `if result.Error != nil` branch.
- The input `*ManifestSet` value remains usable after `Plan()` returns
  in both the success and failure paths.

### `Enricher` (interface contract)

```go
// Enricher.Enrich(set) MUST:
//   - return a *ManifestSet (possibly the input pointer if no changes)
//     when err == nil
//   - return (nil, err) where err is typically a *EnrichmentError on
//     failure
//   - never mutate the input set or any manifest reachable from it
//   - process Applications in deterministic order (sorted by
//     metadata.name) so that the first error reported on failure is
//     stable across runs
//
// Enricher implementations MUST NOT perform I/O, call external
// services, or read filesystem state. They are pure functions of the
// input set.
type Enricher interface {
    Enrich(set *ManifestSet) (*ManifestSet, error)
}
```

(`InferredEdge` provenance flows through a separate channel inside
`ChainEnrich`; see the package-private `inferredEdges` accumulator.
External implementers should not need to surface provenance directly —
the rule helper exposes it.)

### `planner.EnrichmentError`

```go
// EnrichmentError is the typed error returned by enrichment when a
// valueFrom reference cannot be resolved to a same-team manifest in
// the loaded set and the consumer has no explicit Spec.Dependencies
// entry covering it.
type EnrichmentError struct {
    Kind          EnrichmentErrorKind
    ConsumerKind  string
    ConsumerName  string
    ConsumerOwner string
    EnvName       string
    TargetKind    string
    TargetName    string
    TargetOutput  string
}

func (e *EnrichmentError) Error() string

type EnrichmentErrorKind string
const ErrCrossTeamOrUnresolvedValueFrom EnrichmentErrorKind = "cross-team-or-unresolved-valuefrom"
```

The `Error()` format is part of this contract (see §2.1 below).

---

## 2. Operator-visible output contract

### 2.1 Cross-team / unresolved `valueFrom` failure (FR-010)

**Where**: `ErrOut` of the calling command (stderr).

**When**: For every `valueFrom: resource.<name>.<output>` or
`valueFrom: application.<name>.<output>` reference where the target
cannot be resolved to a same-team manifest in the current
`ManifestSet` AND the consumer Application has no explicit
`Spec.Dependencies` entry matching the reference under the
`(Kind, Name)` dedup key. "Cannot be resolved to same-team" covers:
- target manifest present but owned by a different team; OR
- target manifest absent from the loaded set entirely.

Fail-fast: the planner stops at the first such reference. The error
is printed once; subsequent offending references in the same run are
not enumerated.

**Format** (single line, plus the standard handler exit on error):

```
enrichment: app "<consumer>" env "<NAME>" references <kind> "<target>.<output>" which is not owned by team "<consumer-owner>"; add an explicit spec.dependencies entry (kind: <Kind>, name: <target>) to declare this dependency
```

**Example**:

```
enrichment: app "ops-bot" env "CACHE_HOST" references resource "shared-cache.HOST" which is not owned by team "ops_bot"; add an explicit spec.dependencies entry (kind: Resource, name: shared-cache) to declare this dependency
```

**Exit code**: Non-zero (the existing planner-error path on the
handler). No deploy steps run; no Docker calls are made.

### 2.2 Dry-run plan-summary header

**Where**: `Out` of the dry-run command (stdout).

**When**: `handler.DryRun` invocations only, on the success path
(enrichment did not fail). The non-dry-run `handler.Deploy` does not
print the summary. If enrichment failed, the §2.1 error is printed to
stderr and no summary is rendered.

**Format**:

```
Deploy order:
  1. <Kind>:<Name>
  2. <Kind>:<Name>
       depends on:
         - <Kind>:<Name>
         - <Kind>:<Name> (inferred from env <ENV_VAR_NAME>)
```

**Rules**:
- The header line `Deploy order:` is fixed text.
- Each step is `  <ordinal>. <Kind>:<Name>` (1-based ordinal, two-space
  indent, padded ordinal not required because homelab plans are short).
- Steps with no dependencies emit no `depends on:` block.
- Within a step's `depends on:` block, dependencies are listed in the
  order they appear in the Application's (enriched) `Spec.Dependencies`.
- Explicit dependencies have no provenance tag.
- Inferred dependencies are tagged `(inferred from env <NAME>)` where
  `<NAME>` is the originating env var. When the same target is referenced
  by multiple env vars, the first one in declaration order is used.

**Example**:

```
Deploy order:
  1. Resource:ops-bot-db
  2. Application:ops-bot
       depends on:
         - Resource:ops-bot-db (inferred from env DB_CONNECTION_URL)
```

After the summary header, the existing `[DOCKER] …` lines from the
dry-run engine continue as today, unchanged.

### 2.3 Apply-single carve-out

The `handler.ApplySingle` path (powering `shrine apply -f`) is **not**
required to print the plan-summary header — it operates on one
manifest at a time and its output is already minimal. However, the
FR-010 failure rule applies uniformly across all planner entry points,
so a cross-team or unresolved `valueFrom` on the single manifest still
fails the apply with the §2.1 error.

---

## 3. Behavioral guarantees (contract assertions)

These are the assertions that integration and unit tests pin in place.

### 3.1 Ordering guarantee (SC-001, SC-008)

For any input `*ManifestSet` containing:
- an `Application` *A* with `metadata.owner == "team-x"` and an env var with
  `valueFrom: resource.R.<output>`, no explicit deps;
- a `Resource` *R* with `metadata.owner == "team-x"` and an output `<output>`,

`planner.Plan(set, …, ByTeam("team-x"))` returns
`PlanResult{Steps, Error: nil}` in which `Resource:R` appears strictly
before `Application:A` in `Steps`.

### 3.2 No-mutation guarantee (SC-006, SC-007)

For the same input, after `planner.Plan(set, …, filter)` returns,
`len(set.Applications[A.Name].Spec.Dependencies) == 0` (the input
Application is untouched). The enriched edge is visible only on
`result.ManifestSet.Applications[A.Name].Spec.Dependencies`. This
holds in both the success and failure paths.

### 3.3 No-disk-write guarantee (SC-006)

Running `planner.Plan` against any in-memory `ManifestSet` does not
open any file for write. (Validated via a unit test that wraps the
test's working directory and asserts no `WRITE` syscalls; in practice,
the assertion is "no test-side `os.WriteFile`/`os.Create` calls in the
planner package", combined with a code-review check that no new file-
writing import was added.)

### 3.4 Cross-team / unresolved failure guarantee (SC-003, FR-010)

For any `valueFrom: resource.<X>.<Y>` or `valueFrom:
application.<X>.<Y>` reference on a consumer Application where:
- no manifest of the matching kind named `<X>` with
  `metadata.owner == consumer.Metadata.Owner` exists in the loaded set,
  AND
- the consumer's `Spec.Dependencies` contains no entry matching
  `(targetKind, X)`,

`planner.Plan(set, …, filter)` returns `PlanResult{Error:
&EnrichmentError{Kind: ErrCrossTeamOrUnresolvedValueFrom, …}}` with
`Steps == nil` and `ManifestSet == nil`. `handler.Deploy` /
`handler.DryRun` print exactly one corresponding `enrichment: …` line
to `ErrOut` and exit non-zero.

### 3.5 Explicit-dep-satisfies-cross-team guarantee (FR-010 second clause, US3 acceptance scenario 2)

For the same setup as 3.4 except the consumer's `Spec.Dependencies`
contains an entry with `Kind == targetKind` and `Name == X`,
`planner.Plan` does NOT raise an `*EnrichmentError` for that
reference; the explicit entry satisfies the requirement and planning
proceeds to ordering.

### 3.6 Provenance-tagging guarantee (SC-004)

For any inferred edge, the dry-run plan summary's line for that edge
ends with ` (inferred from env <NAME>)`. For any explicit edge
(including one that covers a cross-team reference per 3.5), the line
does NOT end with such a tag. An automated test parses the summary and
asserts the partition.

### 3.7 Determinism in both paths (FR-013, SC-005)

For a given input `*ManifestSet` and rule chain:
- Success path: repeated calls produce identical `Steps` order,
  identical `InferredEdges` slices (byte-for-byte).
- Failure path: repeated calls produce an `*EnrichmentError` with the
  same `Kind`, same `ConsumerName`, same `EnvName`, same `TargetName`,
  same `TargetKind` — i.e., the same offending reference is reported
  first.

---

## 4. What this contract does NOT promise

- **Detection of unreachable inferred edges via cycles**: if a
  same-team `valueFrom` references a target that participates in a
  cycle via other edges, the existing topological sort raises a cycle
  error (unchanged behavior). Enrichment does not pre-check or
  rephrase cycle errors.
- **Cross-team auto-inference**: never. Cross-team coupling is always
  an operator decision and now requires an explicit
  `spec.dependencies` entry on pain of planner failure.
- **Persisted inferred edges**: inferred edges live only in memory.
  They are never written to YAML, to the state store, or to the
  deployment record (FR-004, Assumption 6 in the spec).
- **Other env input shapes**: only `valueFrom: resource.<name>.<output>`
  and `valueFrom: application.<name>.<output>` are recognized at
  landing. Future shapes (vault refs, templates) would require new
  rules in a follow-up. `valueFrom: vault:…` and literal `value:` are
  silently skipped and do NOT trigger the §2.1 failure.
- **Collected (multi-error) reporting**: enrichment is fail-fast; only
  the first offending reference is reported per run. Operators fix
  issues one at a time across successive planner invocations.
