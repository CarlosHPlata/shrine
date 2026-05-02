# Implementation Plan: Per-Alias TLS Opt-In for Routing Aliases

**Branch**: `012-tls-alias-routers` | **Date**: 2026-05-02 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/012-tls-alias-routers/spec.md`

## Summary

Add an optional boolean `tls` field to each entry of `routing.aliases` in the application manifest so an operator can declare per-alias HTTPS publishing inline instead of hand-editing the generated dynamic config (which became operator-owned in spec 009 — turning the workaround durable but invisible to the manifest). When `tls: true`, the generated alias router attaches to both `web` and `websecure` entrypoints and carries an empty `tls: {}` block; when omitted or `false`, the alias router shape is byte-identical to today (FR-009 regression guard). Per-application TLS certificate provisioning, ACME, redirects, and `websecure`-entrypoint wiring stay entirely operator-owned (FR-012, mirroring 011 FR-010); this feature only opens the routing/entrypoint side at the per-alias granularity.

The implementation is four narrow changes:

1. **`internal/manifest/types.go`**: add `TLS bool \`yaml:"tls,omitempty"\`` to `RoutingAlias`. Field is a plain bool, not a pointer — `tls: false` and an omitted field both produce `false`, which is the desired semantic per the spec's "tls: false behaves identically to omission" edge case. (No three-state ambiguity, unlike `StripPrefix` whose default depends on whether `pathPrefix` is set.)
2. **`internal/engine/backends.go` + `internal/engine/engine.go`**: extend `engine.AliasRoute` with a `TLS bool` field; populate it inside `resolveAliasRoutes`; extend `formatAliasesForLog` to append a `" (tls)"` suffix on TLS-enabled aliases so FR-010's per-alias log marker is honored without inventing a new event.
3. **`internal/plugins/gateway/traefik/spec.go` + `routing.go`**: extend the `router` struct with an optional `TLS *tlsBlock \`yaml:"tls,omitempty"\`` pointer (where `tlsBlock struct{}` marshals to `tls: {}`); inside `WriteRoute`, when `ar.TLS == true`, set `EntryPoints: []string{"web", "websecure"}` and `TLS: &tlsBlock{}` on the alias router (primary-domain router shape unchanged); add a `staticConfigPath` field to `RoutingBackend` so it can call the existing `hasWebsecureEntrypoint(path)` probe and emit a new `gateway.alias.tls_no_websecure` warning per FR-007 when an alias declares TLS but the active static config lacks the `websecure` entrypoint.
4. **`internal/manifest/validate.go`**: no new validation logic for the boolean shape itself (YAML unmarshal already rejects non-boolean values into a `bool` field with a clear error per FR-004 — this is verified by a unit test rather than reimplemented). The existing `validateRoutingAliases` collision check (spec 006) is unchanged: it still keys on `(host, normalizedPathPrefix)` and ignores the new `tls` field, satisfying FR-006 ("TLS flag is a routing decoration, not a uniqueness key"). FR-005's "tls is only valid inside an alias entry" guarantee falls out of the schema for free — `tls` is declared only on `RoutingAlias`, never on `Routing`, so YAML unmarshal will reject a top-level `routing.tls` with `field tls not found in type manifest.Routing`.

A new testutils helper is **not** needed — the existing `tc.AssertFileContains` and `tc.AssertFileDoesNotContain` cover all router-shape assertions byte-by-byte. Three integration scenarios in `tests/integration/traefik_plugin_test.go` cover the user-story acceptance criteria. Unit tests in `internal/plugins/gateway/traefik/routing_test.go` are extended to cover the new router shape, the `tls: false` regression guard, and the warning emission path. A new integration scenario for the warning case lives alongside the existing alias scenarios.

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: `gopkg.in/yaml.v3` (already used by the plugin and by the manifest parser); `internal/engine.Observer` for the new warning event; the existing `hasWebsecureEntrypoint` probe shipped by spec 011 — reused verbatim.
**Storage**: Filesystem only — `<routing-dir>/dynamic/<team>-<service>.yml` per-app file already managed by `RoutingBackend.WriteRoute`. No new files; no new state-store entries; no new state-package signature change.
**Testing**: `go test ./...` for unit tests (no filesystem touches per project convention; uses the existing `writeFileFn`/`mkdirAllFn`/`readFileFn` injection points). `make test-integration` (build tag `integration`) for the end-to-end gate using `NewDockerSuite` against a live Docker daemon.
**Target Platform**: Linux server (homelab operator host running Docker).
**Project Type**: CLI (single Go module under `cmd/`, internal packages under `internal/`).
**Performance Goals**: N/A — this feature adds at most one extra YAML probe (`hasWebsecureEntrypoint`) per `WriteRoute` call when at least one alias has `tls: true`; the probe is `os.ReadFile` on a small static config and is short-circuited when no alias requests TLS. Cost is bounded by the same single deploy-cycle cost as the existing 011 probe.
**Constraints**: Must not regress any existing alias test (006/008 contracts unchanged); must produce byte-identical generated dynamic config when no alias sets `tls` (FR-009 regression guard verified by extending the existing `TestWriteRoute_NoAliases` and `TestWriteRoute_OneAlias_Strip` table-driven coverage); must not modify operator-preserved per-app dynamic config files (spec 009 regime — FR-008 inherits unchanged); `entryPoints` list order MUST be `[web, websecure]` to match the issue's exemplar and keep the byte-comparison-friendly assertion in tests.
**Scale/Scope**: One struct-field addition on the manifest side, one struct-field addition on the engine side, one struct-field addition on the traefik routing struct, one new YAML helper struct (`tlsBlock`), one extension of `formatAliasesForLog`, one new observer event (`gateway.alias.tls_no_websecure`) plus its terminal-logger renderer, one new field on `RoutingBackend`. Estimated total delta: ~80 LOC of production code and ~200 LOC of test code across 6 production files and 4 test files.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] Pass — exposed as a single new YAML field `spec.routing.aliases[].tls` in the application manifest. No new CLI flags. The new field is field-level and validated structurally by YAML unmarshal (rejects non-boolean values with a clear error naming the offending path), satisfying the constitution's "field-level constraints MUST be enforced at parse/validate time" rule. |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] N/A — no new commands. Existing `shrine deploy --dry-run` continues to work because `tls` flows through the same `WriteRouteOp` path that the dry-run routing backend already prints; the new `EntryPoints: [web, websecure]` and `TLS: &tlsBlock{}` fields appear in the dry-run output verbatim. |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass — Traefik-specific logic (router shape with `tls: {}`, `web`+`websecure` entrypoints, websecure-probe warning) lives entirely inside `internal/plugins/gateway/traefik/`. The engine-side changes are limited to (a) carrying the new `TLS bool` through `engine.AliasRoute` and `WriteRouteOp` so plugin code can read it, and (b) extending the human-facing log formatter — neither contains routing-specific decisions. The collision check in `internal/manifest/validate.go` is plugin-agnostic and remains keyed only on host+path; the new `tls` field is invisible to it. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — no new abstractions, interfaces, factories, or types beyond the unavoidable: the manifest `RoutingAlias.TLS` field, the engine `AliasRoute.TLS` field, the traefik `router.TLS` pointer field, and the empty `tlsBlock struct{}` whose only job is to marshal to `tls: {}` (which an `interface{}` or `map[string]any` would do less safely). The new `gateway.alias.tls_no_websecure` event reuses the existing `Observer.OnEvent` channel and `engine.Event` shape — no new event-type abstraction. The `RoutingBackend` gains one field (`staticConfigPath string`), not a new dependency injection surface. |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass — three new scenarios in `tests/integration/traefik_plugin_test.go` cover all three user stories' acceptance criteria, plus one scenario for the FR-007 warning path. Listed in research.md Decision 7 and detailed in the Project Structure section below. The integration suite continues to use `NewDockerSuite` with `BeforeEach`/`AfterEach` cleanup per the harness convention. |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] N/A — no state-record or container-recreation changes. This feature touches the dynamic-config file writer only; the Traefik container is unaffected (its static config gained `websecure` in spec 011 and its port bindings gained `tlsPort` in spec 011 — this feature consumes both as preconditions). |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | [x] Pass — `tlsBlock` is a pure type; the new branch in `WriteRoute` is six lines and reads as a single sentence (`if ar.TLS { r.EntryPoints = []string{"web", "websecure"}; r.TLS = &tlsBlock{} }`), no comment needed. The new `emitAliasTLSNoWebsecureSignal` helper mirrors the naming of the existing `emitTLSPortNoWebsecureSignal` from spec 011 — same `emit{Cause}{Effect}` shape. The `formatAliasesForLog` extension adds one `if` branch matching the existing "(no strip)" precedent. |

> Violations MUST be documented in the Complexity Tracking table below.

**Result**: All seven principles pass with no violations. The feature is a pure additive extension of the spec-006 alias surface and the spec-011 TLS surface; no engine core touched, no state schema changes, no new directories.

## Project Structure

### Documentation (this feature)

```text
specs/012-tls-alias-routers/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — 7 design decisions resolved
├── data-model.md        # Phase 1 output — schema delta + router-shape projection table + edge-case truth table
├── quickstart.md        # Phase 1 output — 6-step manual operator validation script
├── contracts/
│   └── observer-events.md  # Phase 1 output — single new event `gateway.alias.tls_no_websecure`
├── checklists/
│   └── requirements.md  # Created by /speckit-specify
└── tasks.md             # Phase 2 output (created by /speckit-tasks)
```

### Source Code (repository root)

```text
internal/manifest/
├── types.go                         # MODIFIED — add `TLS bool \`yaml:"tls,omitempty"\`` to RoutingAlias struct
├── parser_test.go                   # MODIFIED — add cases asserting tls round-trips for true/false/omitted
├── validate.go                      # UNCHANGED — collision key remains (host, normalizedPathPrefix); tls is not a uniqueness input (FR-006)
└── validate_test.go                 # MODIFIED — add a parser-level case asserting non-boolean tls is rejected by yaml.Unmarshal with a path-bearing error message (FR-004)

internal/engine/
├── backends.go                      # MODIFIED — add `TLS bool` to engine.AliasRoute (no change to WriteRouteOp shape; AliasRoute is the per-alias struct)
├── engine.go                        # MODIFIED — populate AliasRoute.TLS in resolveAliasRoutes; extend formatAliasesForLog to append " (tls)" when r.TLS is true
└── engine_aliases_test.go           # MODIFIED — extend resolveAliasRoutes coverage and formatAliasesForLog coverage with tls cases

internal/plugins/gateway/traefik/
├── spec.go                          # MODIFIED — add `TLS *tlsBlock \`yaml:"tls,omitempty"\`` to router struct; add new `tlsBlock struct{}` type
├── routing.go                       # MODIFIED — when ar.TLS, set router.EntryPoints=[web, websecure] and router.TLS=&tlsBlock{}; add emitAliasTLSNoWebsecureSignal called once when at least one alias has TLS=true and probe returns false
├── plugin.go                        # MODIFIED — RoutingBackend constructor now passes staticConfigPath = filepath.Join(routingDir, "traefik.yml") into the new RoutingBackend.staticConfigPath field
├── routing_test.go                  # MODIFIED — extend existing TestWriteRoute_* with tls cases; add new TestWriteRoute_AliasWithTLS_AddsWebsecureAndTLSBlock; add TestWriteRoute_AliasTLS_NoEffect_OnPrimaryRouter (regression guard); add TestWriteRoute_AliasTLS_EmitsWarning_WhenStaticConfigLacksWebsecure
└── routing.go                       # see above (single file edited twice)

internal/ui/
└── terminal_logger.go               # MODIFIED — add `case "gateway.alias.tls_no_websecure"` clause; renders alongside the existing gateway.config.* warnings in the same ⚠️ style

tests/integration/
└── traefik_plugin_test.go           # MODIFIED — three new scenarios (one alias with tls; mixed TLS-on/TLS-off aliases; FR-009 regression — manifest with no tls field produces byte-identical config); one new scenario for FR-007 warning when static config lacks websecure

tests/integration/fixtures/
├── traefik-alias-tls/               # NEW — fixture: one alias with tls: true
│   ├── application.yml
│   └── team.yml
└── traefik-alias-tls-mixed/         # NEW — fixture: two aliases, first without tls, second with tls: true
    ├── application.yml
    └── team.yml

AGENTS.md                            # MODIFIED — add the tls field to the routing.aliases example block; one-line note that tls only opens the entrypoint side and does not configure certs (mirrors spec 011's analogous note)
```

**Structure Decision**: The feature lives in three layers — manifest schema (`internal/manifest/`), engine-side alias projection (`internal/engine/`), and Traefik-specific router generation (`internal/plugins/gateway/traefik/`). The manifest layer adds a single bool; the engine layer threads it through to plugins via `engine.AliasRoute`; the plugin layer renders it as the YAML shape Traefik expects. No new top-level packages, no new directories, no new state files. The probe used for the FR-007 warning (`hasWebsecureEntrypoint`) is reused from spec 011 — no duplicate implementation. Test layout mirrors the existing two-tier split (unit tests beside production code with the project's no-filesystem-in-unit-tests convention; integration tests under `tests/integration/` with the `integration` build tag and the `NewDockerSuite` harness). The new observer event extends the existing `gateway.` namespace under a new `gateway.alias.*` sub-namespace, parallel to the established `gateway.config.*` and `gateway.route.*` sub-namespaces, keeping the contract for downstream UI/log consumers stable.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

The Constitution Check has no violations. The single design decision worth surfacing is the choice to introduce a new sub-namespace `gateway.alias.*` for the warning event rather than reusing `gateway.config.tls_port_no_websecure`:

| Item | Why included in this feature's scope | Simpler alternative rejected because |
|------|--------------------------------------|--------------------------------------|
| New event name `gateway.alias.tls_no_websecure` (parallel to spec 011's `gateway.config.tls_port_no_websecure`) | The two events fire on different surfaces (per-app dynamic config vs. cluster-wide static config), carry different fields (per-alias context vs. file-level context), and have different operator remediations (turn off `tls: true` per alias vs. set `tlsPort` cluster-wide). Reusing the 011 event name would conflate the two and force a conditional in the renderer to figure out which kind of mismatch the operator hit. | (a) Reusing `gateway.config.tls_port_no_websecure` would require carrying per-alias context in fields the existing renderer doesn't print, producing a confusing operator log line. (b) Suppressing the warning entirely would silently break FR-007's contract — operators who set `tls: true` without wiring `tlsPort` would see "TLS routes published" in the log but no inbound TLS traffic, exactly the failure mode this feature is meant to surface. The new event is one renderer case in `terminal_logger.go` and one Decision in research.md; the cost is a single line of contract surface. |
