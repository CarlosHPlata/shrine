# Research: Implicit Dependency Inference

**Feature**: `020-infer-valuefrom-deps` | **Phase**: 0 (Research)
**Date**: 2026-05-20

This document records the design decisions made before Phase 1 (data model
and contracts). All `NEEDS CLARIFICATION` markers from `plan.md` are
resolved here.

---

## Decision 1 — Where the enrichment step sits in the planner pipeline

**Decision**: Insert enrichment between `Resolve()` and the filter switch
inside `internal/planner.Plan()`. The order becomes
`Validate → Resolve → Enrich → (collision-detect) → Order`.

**Rationale**:
- FR-008 mandates the step run *after* validation and *before* `Order`.
- All current callers (`handler.Deploy`, `handler.DryRun`,
  `handler.ApplySingle`) enter the planner through `Plan()`, so a single
  insertion point covers every call site uniformly. No caller can bypass
  enrichment by accident.
- `Resolve()` is what guarantees the references the inference layer
  parses are well-formed (`validateValueFrom` rejects malformed strings,
  missing targets, and bad outputs). Running enrichment after `Resolve()`
  means inference rules never have to re-validate — they can assume any
  `resource.<name>.<output>` they see resolves to a real `Resource` in
  the set or has been ruled out by validation already.

**Alternatives considered**:
- *Move enrichment into `LoadDir`*: would couple file I/O to inference
  semantics and forces every test that constructs a `ManifestSet`
  manually (planner unit tests, integration test fixtures) to also opt
  into enrichment. Rejected.
- *Move enrichment into a new top-level handler step before `Plan()`*:
  would duplicate the call across `handler.Deploy`, `handler.DryRun`,
  `handler.ApplySingle`. Rejected — Plan() is already the funnel point.
- *Run enrichment inside `Order()`*: would couple the topological-sort
  algorithm to env-var semantics and break the separation between graph
  construction and graph traversal. Rejected on Principle III/VII grounds.

---

## Decision 2 — Decorator/chain pattern shape

**Decision**: Define an `Enricher` interface with a single method
`Enrich(*ManifestSet) (*ManifestSet, error)`. A composer
`ChainEnrich(set, rules ...Enricher) (*ManifestSet, []InferredEdge, error)`
threads the result through each rule in order and aggregates the
inferred-edge provenance. Any rule may return an error; the chain stops
at the first error and propagates it unchanged.
`DefaultEnrichers()` returns the rules registered for production use
(`enrichValueFromResource`, `enrichValueFromApplication`). Tests can
construct ad-hoc chains.

**Rationale**:
- FR-007: composable units of inference, additive at the call site, no
  edits to existing rules when adding a new one.
- A single-method interface is the minimum abstraction that satisfies
  the rule.
- Threading `(*ManifestSet, error)` through the chain mirrors how Go
  middleware decorators are typically written: each unit takes a value,
  returns a value plus error, no shared mutable state.
- The error return on `Enrich` is what enables FR-010's fail-fast
  behaviour without inventing a separate validation channel. Rules that
  do not validate (in the future) can simply return `nil` always.

**Alternatives considered**:
- *Each rule mutates the input set in place*: rejected explicitly by the
  user ("new set returned, but can contain same specs structs enriched")
  and by FR-005.
- *A registry that rules self-register into via `init()`*: rejected
  because it hides the rule set from the call site and makes testing
  with an alternative chain awkward. Explicit composition wins.
- *Variadic `func(*ManifestSet) (*ManifestSet, error)` instead of
  an interface*: equivalent in capability but less greppable when a
  contributor wants to find "all current enrichers". Interface + factory
  function (`DefaultEnrichers()`) is the explicit-registry version of
  the same idea.
- *Two-channel return (`(*ManifestSet, []Notice)` plus a separate
  validator step)*: rejected — splitting validation across two passes
  would force every caller and every test to remember to run both, and
  the validation rule is intrinsic to the enrichment rule (same parse,
  same lookup, same gate).

---

## Decision 3 — Immutability strategy (copy-on-write)

**Decision**:
- `ChainEnrich` first calls `copyManifestSetShallow(set)` to allocate a
  fresh `*ManifestSet` with fresh `map` headers. Map values (manifest
  pointers) are copied by reference.
- Each rule iterates the (already-shallow-copied) maps. When a rule
  decides to add edges to a given Application `app`, it constructs a
  **new** `*manifest.ApplicationManifest` value (`*app` copied),
  allocates a new `Dependencies` slice (`append(append([]Dependency{},
  app.Spec.Dependencies...), inferred...)`), and replaces the entry in
  the working map with the new pointer. Applications that gained no
  edges remain pointer-shared with the input set.
- `manifest.Metadata`, `Env`, `Routing`, `Networking`, `Volumes` etc.
  are not deep-copied — they are immutable from the planner's
  perspective and are read-only after `Resolve`. Only the
  `Spec.Dependencies` slice is grown.

**Rationale**:
- FR-005 requires that callers observing the input set after enrichment
  see no changes to `Spec.Dependencies` lengths or contents.
- Edge Case 8 in the spec: callers can reuse the pre-enrichment set for
  diagnostics or to display the raw manifests as authored.
- Deep-copying the entire manifest tree would be wasteful for the common
  case where only one or two slices need to grow. Copy-on-write at the
  slice level is the minimum work that preserves immutability.

**Alternatives considered**:
- *Deep-clone every manifest before passing into the chain*: rejected
  on cost grounds and as gratuitous (no other field is mutated).
- *Persistent/immutable data structure for `[]Dependency`*: overkill;
  Go's slice copy-on-append idiom is sufficient.

---

## Decision 4 — Dedup against explicit dependencies

**Decision**: Before scanning env vars for inference targets, each rule
builds a local `explicit` set keyed on `(Kind, Name)` from the
Application's existing `Spec.Dependencies`. When the rule discovers an
inference candidate, if `(Kind, Name)` is already in `explicit`, the
candidate is dropped silently. The explicit entry is preserved verbatim
in the cloned manifest, including its `Owner` field (even when that
field disagrees with the live `metadata.owner` on the target — FR-006
preserves the operator's authored value; `Resolve()` already raised the
mismatch as an error if it was reachable).

**Rationale**:
- FR-006 (explicit wins on dedup; operator's text never overwritten).
- The dedup key intentionally ignores `Owner` so that a typo in
  `dep.Owner` does not produce a duplicate edge. The explicit entry is
  the one that survives in either case.
- Cross-rule dedup is handled by virtue of each rule running over the
  already-enriched set produced by the previous rule: by the time
  `enrichValueFromApplication` runs, any `Resource:X` edge added by
  `enrichValueFromResource` is in `app.Spec.Dependencies` and will be
  treated as an "explicit" entry for dedup purposes inside the next
  rule. Idempotence (SC-005) falls out for free.

**Alternatives considered**:
- *Dedup on `(Kind, Name, Owner)`*: rejected because the same logical
  edge would be doubly recorded whenever the operator's `Owner` typo
  differs from the live owner.
- *Always replace explicit entries with inferred ones (richer
  metadata)*: rejected — operator-authored text is sacrosanct (FR-006).

---

## Decision 5 — Cross-team `valueFrom` handling (fail-fast)

**Decision**: When a rule sees a `valueFrom` reference whose target
cannot be resolved to a same-team manifest in the current
`ManifestSet`, the rule:

1. Checks the consumer Application's explicit `Spec.Dependencies` for
   an entry matching the reference under the `(Kind, Name)` dedup key.
   If one is present, the explicit entry satisfies the requirement —
   the rule skips the reference and continues.
2. Otherwise the rule returns an `*EnrichmentError` whose
   `Kind == ErrCrossTeamOrUnresolvedValueFrom`. The chain stops at the
   first error; subsequent references in the same run are not
   enumerated. The planner returns the error in `PlanResult.Error`
   and `Steps` is `nil`.

"Cannot be resolved to a same-team manifest" covers two cases that are
treated identically (per Clarifications Q2):
- The target manifest is present in the set but its `metadata.owner`
  differs from the consumer's owner (true cross-team).
- The target manifest is absent from the set entirely (typo, or target
  lives in a team that was not loaded). Because `shrine deploy team <T>`
  loads all manifests owned by `<T>`, absence implies the reference is
  not same-team.

The error message format (printed to `ErrOut` by the handler and used
as `Error()`):

```
enrichment: app "<consumer>" env "<NAME>" references <kind> "<target>.<output>" which is not owned by team "<consumer-owner>"; add an explicit spec.dependencies entry (kind: <Kind>, name: <target>) to declare this dependency
```

**Rationale**:
- FR-010 (post-clarification) requires the planner to fail rather than
  emit an informational nudge. Operators editing manifests benefit from
  a hard failure that prevents an under-specified deploy from running.
- Routing the failure through the rule's error return uses the same
  channel as any other planner error; handlers already propagate
  `PlanResult.Error` to stderr with a non-zero exit. No new wiring
  needed in `handler.Deploy`/`handler.DryRun`/`handler.ApplySingle`
  besides the error propagation already in place.
- The same-team gate is computed by lookup against the set's
  `Applications` and `Resources` maps; absence and cross-ownership are
  both expressible as "no matching same-owner entry in the maps", so
  the rule implementation has one code path for both.

**Alternatives considered**:
- *Print an informational nudge and continue (original design)*:
  rejected by the user during clarification. Letting a cross-team
  reference proceed without an explicit dep leaves the deploy in an
  under-specified state.
- *Collect all offending references in one run and report them
  together*: rejected during clarification — operator chose fail-fast
  (Q3). Simpler implementation; deterministic first-error reporting is
  guaranteed by Decision 8's sort order.
- *Raise the failure inside `Resolve()` rather than during enrichment*:
  rejected — `Resolve()` is about value substitution (templates,
  secrets) and access validation, not ordering semantics. Putting the
  same-team-or-explicit gate next to the inference rule keeps the two
  concerns in one place.

---

## Decision 6 — Dry-run plan summary header (FR-009)

**Decision**: The dry-run engine itself is **unchanged**. Instead,
`handler.DryRun` renders a plan-summary header to stdout *before*
invoking `engine.ExecuteDeploy`. The summary is only rendered when
enrichment succeeded; if enrichment failed (Decision 5), the handler
prints the error to stderr with a non-zero exit and no Docker
operations are attempted. The header is produced by a new helper
`formatDeployPlan(steps, set, provenance)` in
`internal/handler/deploy_plan_format.go`. The output looks like:

```
Deploy order:
  1. Resource:ops-bot-db
  2. Application:ops-bot
       depends on:
         - Resource:ops-bot-db (inferred from env DB_CONNECTION_URL)
```

Explicit deps appear with no provenance tag:
```
       depends on:
         - Resource:shared-db
```

The `provenance` argument is a new field on `PlanResult` —
`InferredEdges map[appName][]InferredEdge` — populated by the
enrichment rules. Each `InferredEdge` carries the target's
`(Kind, Name)` and the originating env var name (one of them, if
multiple env vars referenced the same target).

**Rationale**:
- FR-009 requires inferred edges to be visibly tagged in dry-run output.
- Today's dry-run output (Docker ops only) gives no per-step dep view,
  so we must add one. The spec's Assumption #7 explicitly anticipated this.
- Keeping the renderer in the handler (not the dryrun engine) maintains
  Principle III: the engine's backends print Docker ops; a planner
  *summary* belongs to the planner's output surface.
- The non-dry-run `handler.Deploy` does NOT print the summary by default
  (operator already chose to deploy; the dry-run is where the preview
  matters). Notices still print in both paths.

**Alternatives considered**:
- *Pass the provenance through the steps themselves*: rejected — `PlannedStep`
  is a stable, tiny value type used by every backend; bloating it with
  provenance just for the renderer would leak planner semantics into
  the engine.
- *Render the summary inside the dryrun engine*: rejected per Principle III.

---

## Decision 7 — Vault and non-manifest `valueFrom` handling

**Decision**:
- `valueFrom: vault:...` references are skipped silently by every rule.
  They are tokens, not manifest references, and therefore are outside
  the same-team-gate scope of FR-010.
- Literal `value:` env vars (no `valueFrom`) are not parsed at all.
- `valueFrom: resource.<X>.<Y>` and `valueFrom: application.<X>.<Y>`
  references are NOT silently skipped when `<X>` is absent from the
  current `ManifestSet`. Per Clarifications Q2 and the revised FR-010,
  absence is treated identically to cross-team: the rule returns the
  fail-fast `EnrichmentError` unless the consumer has an explicit
  `Spec.Dependencies` entry covering the reference.

**Rationale**:
- FR-011 (post-clarification) narrows "ignored" to vault and literal
  cases — the categories that, by syntax, are clearly not manifest
  references. Anything that uses the `resource.` / `application.`
  manifest-reference grammar must resolve to a same-team manifest in
  the loaded set or be covered by an explicit dep, otherwise FR-010
  fires.
- This unifies the absent-from-set case with the cross-team case under
  a single error path (Decision 5), which is the smallest possible rule
  surface: one parse, one lookup, one branch.
- The previous "warn would be noisy" concern (which justified silent
  skipping for absent targets) no longer applies — operators must now
  declare an explicit dep when they intentionally reference a target
  outside their team's loaded set. The friction is intentional and is
  the point of FR-010.

**Alternatives considered**:
- *Keep silent-skip for absent targets and fail only for in-set cross-
  team references*: rejected during clarification (Q2). The operator
  cannot tell from the spec output whether an absent reference was
  intentional or a typo; failure is preferable to silent under-
  specification.
- *Distinguish "absent" from "cross-team" with different error kinds*:
  rejected as low-value. The remediation is identical in both cases
  (declare an explicit `spec.dependencies` entry, or fix the typo); a
  single error kind with a clear message keeps the rule simpler.

---

## Decision 8 — Determinism across map iteration (including fail-fast ordering)

**Decision**: Inside each rule, the iteration over `set.Applications`
collects names into a slice, sorts the slice (stdlib `sort.Strings`),
and then processes Applications in sorted order. Within a single
Application, env vars are scanned in declaration order (slice order
preserved by the parser). Target-manifest lookups go through the
already-sorted maps via map access (deterministic for single lookups).

For the fail-fast path (Decision 5), the first offending reference
encountered under this sorted traversal is the one returned in the
error — this guarantees FR-013's determinism requirement extends to
the failure case: the same input always produces the same first error.

**Rationale**:
- Go map iteration order is randomized; without an explicit sort, the
  choice of "first offending reference" reported by FR-010 would
  differ across runs and SC-005's failure-path idempotence would not hold.
- Sorting by Application `metadata.name` is stable, cheap (n log n
  for small n), and uses no allocator-of-doom.
- The choice of "originating env var" when multiple env vars on the
  same Application reference the same target (Edge Case 5) picks the
  first in declaration order — Go slices preserve order, so this is
  already deterministic without extra sorting.

**Alternatives considered**:
- *Stable sort using slice insertion order from the parser*: the
  parser/loader doesn't preserve cross-file ordering; sort by name is
  the simpler, stable input.
- *Sort env vars too*: rejected — declaration order is the operator's
  authored order and the only natural choice for the source-tag in
  dry-run output.

---

## Decision 9 — Error type and handler propagation

**Decision**: Define a typed `*EnrichmentError` in
`internal/planner/enrich.go` with the fields needed to render the
FR-010 message (consumer kind/name/owner, env var name, target
kind/name/output) and a stable `Kind` value
(`ErrCrossTeamOrUnresolvedValueFrom`). The error implements `error`
via a `String()/Error()` method that produces the human-readable
message in Decision 5.

`ChainEnrich` returns `(*ManifestSet, []InferredEdge, error)`.
`Plan()` sets `PlanResult.Error = err` and leaves `Steps == nil` on
enrichment failure. The existing handler error path then prints the
error to `ErrOut` and exits non-zero. No new wiring is required in
`handler.Deploy`/`handler.DryRun`/`handler.ApplySingle` beyond the
existing `if result.Error != nil` branch.

**Rationale**:
- A typed error lets tests assert on `Kind` and on the structured
  fields without parsing the formatted message.
- Reusing the existing `PlanResult.Error` channel means the handler-
  side code path for an enrichment failure is identical to the path
  for a validation failure today, minimizing surface area.
- The error wraps no underlying error (it is the root cause); tests can
  use `errors.As(err, &target)` to extract the typed shape.

**Alternatives considered**:
- *Sentinel error value (`ErrCrossTeamValueFrom = errors.New(...)`)*:
  rejected — the rendered message needs to name the specific reference,
  so a sentinel forces wrapping with `fmt.Errorf` everywhere and loses
  the structured fields for testing.
- *Reuse `internal/manifest.ValidationError`*: rejected — that type
  belongs to manifest validation and would conflate enrichment failures
  with parse/schema failures in the handler's error reporting.

---

## Open questions resolved

| Question | Resolved by |
|---|---|
| Where does enrichment run? | Decision 1 — `Plan()` between `Resolve` and the filter switch. |
| What's the rule contract? | Decision 2 — `Enricher` interface, `(*ManifestSet) → (*ManifestSet, error)`. |
| How are inputs kept immutable? | Decision 3 — shallow-copy maps + copy-on-write for `Spec.Dependencies`. |
| Same-team vs cross-team behaviour? | Decisions 4–5 — dedup for same-team + fail-fast for cross-team/absent. |
| What about absent-from-set targets? | Decision 7 — treated identically to cross-team (Clarifications Q2). |
| How does the dry-run show provenance? | Decision 6 — handler-rendered summary header. |
| Determinism (including failure path)? | Decision 8 — sort iteration by Application name; first-error is deterministic. |
| Error type and handler wiring? | Decision 9 — typed `*EnrichmentError`, routed through existing `PlanResult.Error`. |

No `NEEDS CLARIFICATION` markers remain.
