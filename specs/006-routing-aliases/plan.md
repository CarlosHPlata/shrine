# Implementation Plan: Routing Aliases for Application Manifests

**Branch**: `006-routing-aliases` | **Date**: 2026-04-30 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/006-routing-aliases/spec.md`

## Summary

Add an optional `routing.aliases` list to the `Application` manifest schema. Each alias declares a `host`, an optional `pathPrefix`, and an optional `stripPrefix` flag (default `true` when `pathPrefix` is set). The Traefik gateway plugin generates one extra router per alias in the application's dynamic config file, all pointing to the same backend service as the primary `routing.domain` router. When the Traefik plugin is inactive, aliases are parsed but silently ignored. A new pre-execution validation pass detects cross-application host+path collisions and fails the deploy with a multi-error report before any gateway config is written.

The work spans three layers — manifest schema/validation, engine routing op, Traefik dynamic-config generator — plus a planner-level cross-app collision check. The change is additive to the existing primary-domain code path; existing manifests continue to produce byte-identical config (SC-005).

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: `gopkg.in/yaml.v3` (manifest parse + Traefik dynamic-config marshal); existing `github.com/CarlosHPlata/shrine/internal/{manifest,engine,plugins/gateway/traefik}` packages
**Storage**: Files under the resolved Traefik routing directory (`<routing>/dynamic/<team>-<service>.yml`); no database
**Testing**: `go test ./...` for unit tests (no filesystem touch — see memory note); `go test -tags integration ./tests/integration/...` for the `NewDockerSuite`-backed end-to-end gate (Constitution V)
**Target Platform**: Linux server (homelab); single-binary CLI deployed by `shrine deploy`
**Project Type**: CLI tool with embedded execution engine and pluggable backends
**Performance Goals**: N/A — alias generation runs once per deploy and is bounded by manifest size (typical: ≤10 aliases per app, ≤100 apps per fleet). Generation is O(routes) and writes a single YAML file per app.
**Constraints**: Generated dynamic config must remain byte-stable across re-deploys when the manifest is unchanged (Traefik's file watcher reloads on every write — flapping config triggers needless reloads). Alias router names must be deterministic and collision-free within a file.
**Scale/Scope**: Single-host homelab fleet; ≤100 apps × ≤10 aliases ≈ ≤1k alias routers across all dynamic config files.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] Pass — adds `spec.routing.aliases[]` to the Application manifest; no new CLI flag, no env var. Validation produces multi-error reports per the existing `validateApplicationSpec` pattern. |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] N/A — no new commands. `shrine apply` and `shrine deploy` already support `--dry-run`; the new field flows through the existing dispatchers unchanged. |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass — alias-to-router translation lives in `internal/plugins/gateway/traefik/`. The engine's `RoutingBackend` interface widens by one field on `WriteRouteOp` (a list of additional routes); engine core remains backend-agnostic and nil-backend-safe. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — no new types beyond `RoutingAlias` (the data the user already declares); no new interface, no factory, no service locator. Cross-app collision detection is a single helper function in the planner, not a new validator subsystem. |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass — adds scenarios to `tests/integration/traefik_plugin_test.go` covering: deploy app with one alias (US1), deploy app with `pathPrefix` strip and no-strip variants, deploy two colliding apps (FR-008a), deploy alias-bearing manifest with Traefik plugin disabled (US2). Tests run the real binary as a subprocess against a live Docker daemon, per Principle V. |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] N/A — this feature does not write to `DeploymentStore` or any state file. Alias config is gateway-side YAML, watched and reconciled by Traefik itself. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | [x] Pass — alias-rule rendering, strip-middleware naming, and collision detection are extracted to named helpers (`buildRouterRule`, `stripMiddlewareName`, `findRoutingCollisions`); validation funcs follow existing `validateApplicationSpec` style; no narrative comments planned. |

> No violations to track. Complexity Tracking table omitted.

## Project Structure

### Documentation (this feature)

```text
specs/006-routing-aliases/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── manifest-schema.md   # Manifest YAML contract for routing.aliases
└── tasks.md             # Phase 2 output (NOT created here)
```

### Source Code (repository root)

```text
internal/
├── manifest/
│   ├── types.go              # ADD: RoutingAlias struct; Routing.Aliases field
│   ├── validate.go           # ADD: validateRoutingAliases() called from validateApplicationSpec
│   └── parser_test.go        # EXTEND: test parsing aliases from YAML
├── engine/
│   ├── backends.go           # MODIFY: WriteRouteOp gains AdditionalRoutes []AliasRoute
│   └── engine.go             # MODIFY: deployApplication translates app.Spec.Routing.Aliases into WriteRouteOp.AdditionalRoutes
├── planner/
│   └── loader.go             # ADD: detectRoutingCollisions() called once before ExecuteDeploy
└── plugins/gateway/traefik/
    ├── routing.go            # MODIFY: WriteRoute renders N routers + 1 service + N? strip-middlewares per app
    ├── spec.go               # MODIFY: middleware{} struct gains stripPrefix field for serialization
    └── routing_test.go       # NEW: unit tests for multi-router YAML emission and strip-middleware shape

tests/
├── integration/
│   └── traefik_plugin_test.go   # EXTEND: scenarios for US1, US2, FR-008a, strip vs no-strip
└── testdata/
    ├── hello-api-aliased.yml    # NEW: fixture manifest with one alias (used by integration scenarios)
    └── hello-api-collision.yml  # NEW: fixture manifest that collides with hello-api.yml on host+path
```

**Structure Decision**: Single project, real paths above. The change is purely additive to four existing packages plus one new entry point in the planner. No new directories, no new modules.

## Phase 0: Outline & Research

Open questions distilled from Technical Context and the spec's Assumptions:

1. **How does Traefik render `StripPrefix` in file-provider dynamic config?** Need the exact YAML shape for a middleware that strips one path prefix and the way to attach it to a router.
2. **Where should cross-app collision detection live — planner, validate phase, or engine pre-flight?** The engine currently iterates steps independently; a pre-existing `ManifestSet` exists in the planner.
3. **Is `WriteRouteOp` the right surface to widen, or should we introduce a `WriteAppRoutesOp` that takes a list?** The existing op assumes one router + one service per call.
4. **Are `stripPrefix` and `pathPrefix` semantics on the primary domain in scope?** The spec adds `stripPrefix` only to alias entries; the existing `Routing.PathPrefix` on the primary domain has no strip flag today.
5. **How should the alias-listing log signal (FR-012) plug into the existing observer events?**

Findings will be consolidated in `research.md` (next file).

## Phase 1: Design & Contracts

Outputs (each generated next):

- **`data-model.md`**: schema and validation rules for `RoutingAlias`, `Routing.Aliases`, and the engine-side `AliasRoute` projection. State transitions: stateless — alias entries are reconciled by Traefik's file watcher whenever the dynamic config file is rewritten or removed.
- **`contracts/manifest-schema.md`**: the YAML contract operators write — field names, types, defaults, validation errors. This is the externally-visible interface (Constitution I: capabilities exposed via manifest).
- **`quickstart.md`**: a 5-minute walk-through of adding one alias to an existing manifest, deploying, and verifying both addresses resolve.
- **Agent context update**: the `<!-- SPECKIT START -->` block in `CLAUDE.md` is updated to reference this plan path.

## Re-Check (Post-Design)

After Phase 1 artifacts are written, re-evaluate Constitution Check. Expected outcome: still all-Pass — the design adds one struct, one slice field, one validation helper, one engine projection, one planner pre-flight check, and one Traefik-side rendering pass. No abstractions are introduced for hypothetical future gateway plugins; the manifest field is plugin-agnostic only because it's plain data, not because of any new interface.
