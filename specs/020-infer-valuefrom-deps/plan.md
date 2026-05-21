# Implementation Plan: Infer Implicit Deploy-Order Dependencies from Same-Owner valueFrom References

**Branch**: `020-infer-valuefrom-deps` | **Date**: 2026-05-20 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/020-infer-valuefrom-deps/spec.md`

## Summary

Add an in-memory enrichment step between manifest validation and topological
ordering inside `internal/planner.Plan()` that derives implicit deploy-order
dependencies for an `Application` from its env vars whose `valueFrom`
references another manifest in the current `ManifestSet`.

Behaviour boundary (post-clarification, 2026-05-20):

- **Same-team reference** → inferred edge added; dedup against existing
  explicit `spec.dependencies` by `(Kind, Name)`.
- **Cross-team reference** (target manifest present but owned by a different
  team than the consumer) → enrichment fails the planner with a
  fail-fast error unless the consumer's explicit `spec.dependencies` already
  covers the reference.
- **Reference whose target is absent from the loaded `ManifestSet`** (typo,
  or target in a team that was not loaded) → same fail-fast error as
  cross-team. Because `shrine deploy team <T>` loads all manifests owned
  by `<T>`, absence implies the reference is not same-team.
- **Vault references and literal `value:`** → silently skipped.

Technical approach: implement the enrichment step as a chain of `Enricher`
units (`Enricher.Enrich(set) (*ManifestSet, error)`) composed by a
`ChainEnrich` helper, with copy-on-write at the `Spec.Dependencies` slice
level so the input set is observably unmodified. Two rules ship at
landing — `enrichValueFromResource` and `enrichValueFromApplication` —
sharing a single `applyEnrichmentRule` helper. The dry-run handler renders
a plan-summary header that tags inferred edges with their originating
env var name.

## Technical Context

**Language/Version**: Go 1.24+
**Primary Dependencies**: existing `internal/manifest`, `internal/planner`,
`internal/handler` packages; no new third-party dependencies.
**Storage**: N/A — enrichment is purely in-memory and never touches disk
(FR-004, SC-006).
**Testing**: `go test ./...` for unit gates (`internal/planner/...`,
`internal/handler/...`); `go test -tags integration ./tests/integration/...`
for the Principle V gate using `NewDockerSuite` against the real shrine
binary.
**Target Platform**: Linux server (homelab Docker host); same as the
shrine binary today.
**Project Type**: Single-binary Go CLI (`cmd/shrine`).
**Performance Goals**: enrichment is O(applications × env vars) with map
lookups for target resolution. Typical team size (~10–50 manifests, ~10
env vars each) puts enrichment under 10 ms on commodity hardware — well
inside the existing `Plan()` budget.
**Constraints**:
- No file I/O during enrichment (FR-004).
- Input `ManifestSet` is observably unmodified after `Plan()` returns
  (FR-005, SC-007).
- Determinism across runs, including the choice of "first offending
  reference" reported by the fail-fast error (FR-013).
- Enrichment runs strictly between `Resolve()` and the existing ordering
  step (FR-008).
**Scale/Scope**: tens of manifests per team, low single-digit teams
per homelab. The enrichment cost is dominated by validation and Docker
operations elsewhere; this feature does not introduce a hot path.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] Pass — uses the existing `env[].valueFrom` field; no schema change (FR-012). |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] N/A — no new CLI subcommands; behaviour is layered behind existing `shrine deploy`/`shrine deploy --dry-run`. |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] N/A — enrichment lives in `internal/planner`; no backend interface is touched. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass (with note) — the `Enricher` interface has 2 concrete implementations at landing, but FR-007 explicitly mandates a composable chain so a future rule can be added without modifying existing rules, and a shared `applyEnrichmentRule` helper provides immediate DRY benefit across both rules. Documented in Complexity Tracking. |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass — new `tests/integration/deploy_team_infer_test.go` covers US1, US2, US3 (failure case), and the no-disk-write invariant. |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] N/A — enrichment runs entirely before any backend call; no state-record changes. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | [x] Pass — both rules share `applyEnrichmentRule`; helpers named `parseValueFromRef`, `cloneApplicationWithDeps`, `hasExplicitDependency`. Boolean names use `has`/`is`/`should` per Principle VII. |

> Violations MUST be documented in the Complexity Tracking table below.

## Project Structure

### Documentation (this feature)

```text
specs/020-infer-valuefrom-deps/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
│   └── enrichment-api.md
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/
├── planner/
│   ├── enrich.go                 # NEW — Enricher interface, ChainEnrich, DefaultEnrichers, helpers
│   ├── enrich_valuefrom.go       # NEW — enrichValueFromResource, enrichValueFromApplication, applyEnrichmentRule, parseValueFromRef
│   ├── enrich_test.go            # NEW — unit tests for ChainEnrich + helpers
│   ├── enrich_valuefrom_test.go  # NEW — unit tests for the two production rules
│   └── plan.go                   # MODIFIED — calls ChainEnrich between Resolve and the filter switch; propagates enrichment errors as PlanResult.Error
├── handler/
│   ├── deploy.go                 # MODIFIED — propagate planner errors (including enrichment failures) to stderr + non-zero exit
│   ├── dryrun.go                 # MODIFIED — same error propagation; on success, render plan-summary header
│   ├── apply_single.go           # MODIFIED — same error propagation
│   └── deploy_plan_format.go     # NEW — formatDeployPlan(steps, set, edges) renders the dry-run summary with provenance tags
└── manifest/                     # NOT MODIFIED — no schema change (FR-012)

tests/
└── integration/
    └── deploy_team_infer_test.go # NEW — covers US1, US2, US3 (failure), no-disk-write invariant, dedup vs explicit
```

**Structure Decision**: Single-binary Go CLI; new code lives under
`internal/planner` and `internal/handler`. No new top-level package and no
new module. The split between `enrich.go` (mechanism: interface + chain
+ shared helpers) and `enrich_valuefrom.go` (policy: the two production
rules) means a future rule can be added as a third file
(`enrich_<something>.go`) plus one line in `DefaultEnrichers()` —
satisfying FR-007 ("composable units, no edits to existing rules").

## Complexity Tracking

> The constitution's Principle IV asks for ≥3 concrete usages before
> introducing an abstraction. This plan introduces one abstraction with
> 2 concrete usages at landing; the table below records the justification.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| `Enricher` interface + `ChainEnrich` composer with 2 concrete rules at landing (vs. constitution's "≥3 usages" rule). | FR-007 explicitly mandates composable inference units so a future rule (e.g., a vault-aware rule, or a future `valueFrom` prefix) can be added without modifying existing rules. The shared `applyEnrichmentRule` helper also delivers immediate DRY benefit across the two production rules (Principle VII). | Inlining both rules' loops in `Plan()` would duplicate the iterate-apps × sort × per-env-scan boilerplate verbatim and force any future rule to either modify `Plan()` (violating FR-007) or duplicate the loop again. The cost of the interface (one method, one file) is small relative to the maintenance friction of in-line duplication. |
