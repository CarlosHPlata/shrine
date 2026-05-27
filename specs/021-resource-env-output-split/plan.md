# Implementation Plan: Split Resource `env` and `output` (SRP)

**Branch**: `021-resource-env-output-split` | **Date**: 2026-05-26 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/021-resource-env-output-split/spec.md`

## Summary

Split the overloaded Resource `output` block into two single-responsibility
blocks:

- **`spec.env`** — the resource container's runtime configuration, modelled on
  `ApplicationSpec.Env` (`value` / `valueFrom` / `template`) **plus**
  `generated: true` for auto-minted secrets. This is what now feeds the
  container's environment.
- **`spec.output`** — a pure export allowlist. Each entry declares a `name`
  (re-exporting an env var or the built-in `host`/`port`) and an optional
  `template`; no `value`, `valueFrom`, or `generated`.

Other manifests may consume only keys present in a resource's `output`
(strict allowlist). Per clarification, **resources become first-class
consumers**: a resource's `env` `valueFrom` may reference other manifests'
exported outputs in addition to vault refs, subject to the same access /
reachability / deploy-order / inference rules that apply to Application
consumers today. Old manifests that set `value`/`valueFrom`/`generated` on an
output are rejected with an actionable validation error (breaking change, no
auto-migration).

Technical approach: add `Env []EnvVar` to `ResourceSpec`; keep the existing
`Output` value-bearing fields only so old manifests still unmarshal and can be
*rejected* by validation. Resolution gains an explicit env/exports split via a
new `resolver.ResolvedResource{Env, Exports}` return type — the container gets
`Env`, consumers get `Exports`. Because resources can now depend on other
manifests, `planner.Order` stops treating resources as leaves and reads
`res.Spec.Dependencies`, the feature-020 enrichment loop generalizes to scan
resource consumers, and the engine resolves resources in topological order so a
resource can read an earlier resource's exports.

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: existing `internal/manifest`, `internal/planner`,
`internal/resolver`, `internal/engine`, `internal/handler`; no new third-party
dependencies. Templating remains Go `text/template` with the existing
topological renderer.
**Storage**: Secret store (`state.SecretStore`) for `generated` env values via
`GetOrGenerate`; vault plugin for `valueFrom: vault:` refs. No new storage.
Resolution and validation are otherwise in-memory.
**Testing**: `go test ./...` for unit gates
(`internal/manifest/...`, `internal/planner/...`, `internal/resolver/...`,
`internal/engine/...`); `go test -tags integration ./tests/integration/...`
(`make test-integration`) for the Principle V gate using `NewDockerSuite`
against the real shrine binary.
**Target Platform**: Linux server (homelab Docker host); same as today.
**Project Type**: Single-binary Go CLI (`cmd/shrine`).
**Performance Goals**: resolution stays O(manifests × env vars) with map
lookups; the added topological resource-resolution pass reuses the existing
`Order` output. No new hot path.
**Constraints**:
- Multi-error validation (Principle I): collect all env/output errors, no
  fail-fast on first.
- Enrichment must keep the input `ManifestSet` observably unmodified (FR-014
  reuses feature-020's copy-on-write).
- Strict allowlist must be enforced at BOTH validate time (FR-009) and
  resolve time (an un-exported key is simply absent from `Exports`).
- Determinism: resource resolution order follows the deterministic `Order`
  output.
**Scale/Scope**: tens of manifests per team; low single-digit teams. Cost is
dominated by Docker operations elsewhere.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] Pass — adds `spec.env` and reshapes `spec.output` as manifest fields; validation stays multi-error and at parse/validate time (FR-001–FR-011). |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] N/A — no new subcommands. Behaviour rides existing `shrine deploy` / `--dry-run`; the dry-run path is updated to reflect the env/output split (FR-013). |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] N/A — no new backend. The `resolver.Resolver` interface (not a backend) changes shape; engine core gains ordered resolution but no backend-specific logic. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — `ResolvedResource{Env, Exports}` is the minimal honest representation of the split (used by Live + DryRun resolvers and the engine). Resource-as-consumer reuses (generalizes) the existing `applyEnrichmentRule` rather than adding a parallel path. One justified ripple noted in Complexity Tracking. |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass — new `tests/integration/deploy_resource_env_output_test.go` covers US1 (env→container, curated exports), US2 (private secret not consumable), US3 (old-manifest rejection), and the resource-as-consumer + ordering case (FR-014). |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] N/A — env/output resolution and secret generation run entirely before backend calls, exactly as resource resolution does today; no change to state-record timing. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | [x] Pass — shared helpers (`resolveEnvValue`, `computeExports`, `cloneResourceWithDeps`); boolean helpers use `is`/`has` (`isReservedBuiltin`, `hasExplicitDependency`). The env-resolution loop is shared between Application and Resource paths. |

> Violations MUST be documented in the Complexity Tracking table below.

## Project Structure

### Documentation (this feature)

```text
specs/021-resource-env-output-split/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── resource-env-output.md   # manifest schema + resolver contract
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/
├── manifest/
│   ├── types.go        # MODIFIED — add `Env []EnvVar` to ResourceSpec; Output keeps
│   │                   #            value/generated/valueFrom fields ONLY so old manifests
│   │                   #            unmarshal and can be rejected; Output's valid surface is name+template
│   ├── validate.go     # MODIFIED — validateResourceSpec: validate env (app-env rules + generated,
│   │                   #            reserved built-in names); validate output as export list
│   │                   #            (name+optional template; reject value/valueFrom/generated;
│   │                   #            non-template name must match an env var or host/port)
│   └── validate_test.go
├── planner/
│   ├── templates.go    # MODIFIED — output templates reference declared ENV names + built-ins
│   │                   #            (host/port/team/name); add resource env-template validation
│   ├── resolve.go      # MODIFIED — allowlist check (referenced key ∈ target.output);
│   │                   #            validate resource env valueFrom incl. cross-manifest refs +
│   │                   #            access/reachability for resource consumers (replaces
│   │                   #            validateResourceVaultOutputs)
│   ├── order.go        # MODIFIED — resources read Spec.Dependencies (no longer hardcoded leaves)
│   ├── enrich_valuefrom.go # MODIFIED — applyEnrichmentRule generalized to scan resource consumers;
│   │                   #            cloneResourceWithDeps helper
│   ├── enrich.go       # MODIFIED — DefaultEnrichers / chaining unchanged in shape; resource edges
│   └── *_test.go
├── resolver/
│   ├── resolver.go     # MODIFIED — ResolveResource(res, deps) (ResolvedResource, error);
│   │                   #            new ResolvedResource{Env, Exports}; resolve env then compute exports
│   ├── dry_run_resolver.go # MODIFIED — same signature; placeholder env + exports
│   └── *_test.go
├── engine/
│   └── engine.go       # MODIFIED — resolve resources in topo (steps) order, pass deps,
│                       #            deps.Resources[name]=Exports; deployResource uses resolved Env
└── handler/
    ├── resources.go    # MODIFIED — generate-resource skeleton shows env + output
    ├── dryrun.go        # MODIFIED — dry-run renders env (container) vs output (interface)
    └── deploy_plan_format.go # MODIFIED — summary reflects split if it lists resource values

tests/
└── integration/
    └── deploy_resource_env_output_test.go  # NEW — US1/US2/US3 + resource-as-consumer + ordering

docs/                   # MODIFIED — resource manifest reference (env/output split); AGENTS.md sync
```

**Structure Decision**: Single-binary Go CLI. All changes live in existing
`internal/*` packages — no new package or module. The split is expressed as a
new manifest field plus a new resolver return type; everything else is an
extension of paths that already exist for Applications.

## Complexity Tracking

> Recorded per Principle IV: the changes below add real cost; each is justified
> by a user-confirmed requirement (Q1 = full symmetry; Q2 = template access).

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| `resolver.Resolver.ResolveResource` signature change (now takes `deps` and returns `ResolvedResource{Env, Exports}`), rippling to `LiveResolver`, `DryRunResolver`, the engine, and their tests. | The SRP split *is* an env/exports separation, and FR-014 lets resource env reference other manifests' exports, which requires `deps`. A single `map[string]string` return can no longer represent both the container env and the (different) export allowlist. | Returning one merged map and post-filtering in the engine would reintroduce the exact env/output conflation this feature removes, and would hide the allowlist behind engine logic instead of the resolver contract. |
| Engine resolves resources in topological order (driven by `Order` output) instead of arbitrary map order, and `planner.Order` reads `res.Spec.Dependencies`. | FR-014 allows resource→resource/app `valueFrom`; a resource that reads another resource's export must be resolved after it. Treating resources as leaves (today's behaviour) would make such references unresolvable. | Keeping the unordered pre-resolve loop cannot satisfy resource→resource references; a one-off special case for "resource depends on resource" would duplicate the topo logic that `Order` already provides. |
