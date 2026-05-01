# Implementation Plan: Preserve Operator-Edited Per-App Routing Files

**Branch**: `009-preserve-app-configs` | **Date**: 2026-05-01 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/009-preserve-app-configs/spec.md`

## Summary

Extend the spec 004 "preserve operator-edited file" pattern from the gateway-wide `traefik.yml` static config down to per-app routing files in `dynamic/<team>-<service>.yml`. After this change ships, the traefik plugin's `WriteRoute` only writes a per-app file when the path is absent (mirrors `generateStaticConfig`'s present-check + skip), and the never-called `RemoveRoute` is wired into the engine's teardown path so it can emit a warning when an operator-owned per-app file is left behind by an app removal. Stat errors and orphan-file states surface as warning-level events that do **not** abort the deploy run — gateway-file outcomes do not gate `shrine deploy` success once the first per-app template has shipped (FR-011 / SC-007). Tests are dual-layered: unit-level coverage at `routing_test.go` (existence check, observer events, warning paths) and an integration scenario at `tests/integration/traefik_plugin_test.go` that proves the byte-identical guarantee end-to-end against a real shrine binary.

The diff is intentionally small. Two source files carry the behavior change: `internal/plugins/gateway/traefik/routing.go` (add existence pre-check, observer-emitted events on the `RoutingBackend` struct) and `internal/engine/engine.go` (call `RemoveRoute` from the teardown path when an application had routing). One existing helper is generalized: `config_gen.go`'s `isStaticConfigPresent` becomes `isPathPresent` and is reused by both call sites — exactly the third concrete usage that triggers Constitution IV's extract-rule.

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: existing — no new modules. Touches `internal/plugins/gateway/traefik`, `internal/engine`. The `gopkg.in/yaml.v3` and `os`/`io/fs` packages already in use cover the new code paths.
**Storage**: N/A. The feature changes the *write* policy for per-app files but introduces no new state files. Shrine never reads back per-app file content under this feature.
**Testing**: `go test ./internal/...` for unit-level coverage in `internal/plugins/gateway/traefik/routing_test.go` (preserve-on-exists, observer events for preserved/generated/stat-error/orphan, symlink/non-regular-file handling). Integration coverage extended in `tests/integration/traefik_plugin_test.go` with one new scenario (`TestTraefikPlugin_PerAppFile_PreservedAcrossRedeploys`) that uses `NewDockerSuite`, the real shrine binary, and a real Docker daemon to assert the byte-identical guarantee from SC-001/005. No mock Docker is introduced (Constitution V).
**Target Platform**: Linux server (homelab); single-binary CLI deployed by `shrine deploy`.
**Project Type**: CLI tool with embedded execution engine and pluggable backends.
**Performance Goals**: N/A. The added stat call runs once per app per deploy; the teardown-path stat runs once per torn-down app. Both are O(apps).
**Constraints**:
- SC-001/SC-005 (byte-identical content on every repeat deploy) — the existence check MUST gate the `os.WriteFile` call entirely; we do not compute content and discard it. This means the YAML marshal step in `WriteRoute` is also skipped when the file is preserved (as a side benefit, this is fewer allocations on repeat deploys).
- SC-003 (zero new knobs) — no new manifest fields, no new flags, no new env vars. The policy is always-on.
- SC-007 / FR-011 (deploy success governed by Docker outcomes only) — the `WriteRoute` and `RemoveRoute` code paths MUST internally distinguish "stat-error / orphan-warn" (return nil after emitting a warning) from "true write/IO failure on a fresh-write" (still propagate as a real error). The error-vs-warning split is the load-bearing implementation choice for FR-007 and FR-011.
**Scale/Scope**: ≤100 apps × 1 per-app file per app on a single homelab host. Same scale as spec 006/008.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] N/A — no new capability is configurable; FR-005 explicitly forbids any new flag/env/manifest field. The preserve policy is the single, default behavior. |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] N/A — no new commands. The dryrun routing backend (`internal/engine/dryrun/dry_run_routing.go`) prints whatever WriteRoute/RemoveRoute it is asked to do; the preserve policy lives inside the *real* traefik backend, so dry-run output is unaffected. |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass — the existence-check, the YAML-marshal-skip, and the warning emission all live inside the traefik `RoutingBackend` (`internal/plugins/gateway/traefik/routing.go`). The `engine.RoutingBackend` interface in `internal/engine/backends.go` does not change; the engine treats the backend as a black box. The one engine.go change — wiring `RemoveRoute` into `teardownKind` — is interface-level orchestration, not backend-specific logic. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass with one pre-cleared extraction. `isStaticConfigPresent` (1 caller in spec 004) + the new per-app existence check inside `WriteRoute` (caller 2) + the new orphan-detect inside `RemoveRoute` (caller 3) = exactly 3. The extraction is mandated by Constitution VII's DRY rule, not speculative. The new helper signature is `isPathPresent(path string) (bool, error)` (was: `isStaticConfigPresent(routingDir string) (bool, error)`). No other abstractions are added. |
| V. Integration-Test Gate | Does this phase map to an integration test scenario in `tests/integration/` using `NewDockerSuite` against a real binary? | [x] Pass — one new scenario in `tests/integration/traefik_plugin_test.go`: `TestTraefikPlugin_PerAppFile_PreservedAcrossRedeploys`. The scenario deploys an app, mutates the resulting per-app file on disk (adds a sentinel comment line), redeploys, and asserts the file's bytes match the mutated baseline. Cleanup uses the existing `NewDockerSuite` `BeforeEach`/`AfterEach`. The user's [memory: integration tests are slow] guidance is respected — only one new integration test is added; all other coverage is unit-level. |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] N/A — this feature does not write state files. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | [x] Pass — the existence-check is extracted into `isPathPresent`; the per-app preserve check is named `isRoutePresent` (a thin wrapper that names the call site's intent — it stat-checks the per-app file path); event-emission helpers use intention-revealing names (`emitRouteGenerated`, `emitRoutePreserved`, `emitRouteStatError`, `emitOrphanRouteWarning`). The boolean-method-naming rule (Constitution VII) is satisfied. No WHAT comments; only WHY-comments where a reader might otherwise relitigate (e.g., one-liner above the symlink case in `isPathPresent` explaining why we use `Lstat` not `Stat`). |

> No violations to track. Complexity Tracking table omitted.

## Project Structure

### Documentation (this feature)

```text
specs/009-preserve-app-configs/
├── plan.md              # This file
├── spec.md              # Already created (with Clarifications session 2026-05-01)
├── research.md          # Phase 0 output — design decisions: observer plumbing, helper extraction, error-vs-warning split
├── data-model.md        # Phase 1 output — short: there is no new data model. Documents the per-app file lifecycle states and the in-memory `RoutingBackend` struct extension.
├── quickstart.md        # Phase 1 output — operator walkthrough: deploy → edit → redeploy → see preserved log → optional reset via rm-and-redeploy
├── contracts/
│   └── log-events.md        # Phase 1 output — names, statuses, and field shapes for the four new/extended events: gateway.route.generated, gateway.route.preserved, gateway.route.stat_error, gateway.route.orphan
├── checklists/
│   └── requirements.md      # Already created by /speckit-specify
└── tasks.md             # Phase 2 output (NOT created here; produced by /speckit-tasks)
```

### Source Code (repository root)

```text
internal/
├── engine/
│   ├── engine.go                                              # MODIFY: teardownKind for ApplicationKind calls engine.Routing.RemoveRoute(team, name) AFTER container removal succeeds. The call is gated on `engine.Routing != nil` (mirrors the WriteRoute call at engine.go:165). RemoveRoute returning a non-nil error still aborts (preserves Constitution VI's after-Docker ordering). Stat-error / orphan-warn paths return nil from the backend, so they don't abort.
│   └── backends.go                                            # NO CHANGE — RoutingBackend interface stays {WriteRoute, RemoveRoute}. The behavior change lives entirely in the traefik implementation.
└── plugins/gateway/traefik/
    ├── routing.go                                             # MODIFY: WriteRoute pre-checks isPathPresent → if true, emit gateway.route.preserved and return nil; on stat error, emit gateway.route.stat_error and return nil. RemoveRoute changes from os.Remove → isPathPresent + emit gateway.route.orphan (warning) when present; absent = silent no-op (still return nil). Both methods use a new `observer` field on the RoutingBackend struct (added in this file).
    ├── config_gen.go                                          # MODIFY: extract isStaticConfigPresent body into isPathPresent(path string); isStaticConfigPresent becomes a one-line wrapper. The lstatFn injection point is preserved (used by config_gen_test.go).
    ├── plugin.go                                              # MODIFY: Plugin.RoutingBackend() passes p.observer into the new RoutingBackend{routingDir, observer} constructor.
    ├── routing_test.go                                        # EXTEND: add cases — TestWriteRoute_FilePresent_Preserves, TestWriteRoute_StatError_EmitsWarningAndContinues, TestWriteRoute_FreshWrite_EmitsGenerated (rename existing happy-path tests to surface the new event), TestRemoveRoute_FilePresent_EmitsOrphanWarning, TestRemoveRoute_FileAbsent_IsNoOp. Use a recordingObserver mirroring the pattern at config_gen_test.go:16-19.
    └── config_gen_test.go                                     # NO CHANGE expected — the helper rename is internal; existing assertions on gateway.config.preserved and gateway.config.generated stand.

tests/integration/
└── traefik_plugin_test.go                                     # EXTEND: one new test — TestTraefikPlugin_PerAppFile_PreservedAcrossRedeploys. Uses NewDockerSuite, deploys a single app via the real binary, captures the bytes of the resulting <team>-<service>.yml, appends a sentinel comment line, runs deploy a second time, asserts the file's content equals (original + sentinel). [memory: integration suite is ~10 min; only one scenario added.]

# UNCHANGED but worth listing for the auditor:
internal/engine/dryrun/dry_run_routing.go                       # NO CHANGE — DryRun backend prints intent only; preserve policy lives in the real backend.
internal/handler/deploy.go                                      # NO CHANGE — the routing backend is constructed via plugin.RoutingBackend() which now wires the observer; the call site is unchanged.
internal/handler/teardown.go                                    # NO CHANGE — engine.ExecuteTeardown is the entry point; the new RemoveRoute call lives inside engine.teardownKind.
```

**Structure Decision**: Single project. The diff is two source files modified for behavior, two for plumbing (`plugin.go`, `config_gen.go`-as-helper-extraction), one test file extended at the unit level, one at the integration level. No new directories, no new packages, no new dependencies. The `engine.RoutingBackend` interface is intentionally untouched — adding `Observer` to the interface would force the dryrun backend to grow a no-op and would couple every future routing backend to the gateway-warning event taxonomy. Keeping the observer inside the traefik implementation is the simpler, more localized choice.

## Phase 0: Outline & Research

The Phase 0 deliverable is `research.md`, which captures three design decisions where a wrong choice would force rework later:

1. **Where does the observer live?** Decision: on the traefik `RoutingBackend` struct (set in `Plugin.RoutingBackend()` from `p.observer`). Alternatives considered: (a) extend the `engine.RoutingBackend` interface to take an Observer per call — rejected because it ripples to every routing-backend implementation including dryrun; (b) return a structured `WriteRouteResult` with events for the engine to emit — rejected because every existing call site at `engine.go:189` would need to be rewritten and the engine would grow a translation layer it doesn't need; (c) emit events through a package-level singleton — rejected on Constitution IV grounds (singletons are abstractions without ≥3 usages here). The chosen design preserves the interface, keeps the observer in exactly one place per backend instance, and matches the established pattern in `config_gen.go:36` where `generateStaticConfig` already accepts an Observer.

2. **What's the error-vs-warning contract for `WriteRoute` / `RemoveRoute` after this change?** Decision: a non-nil error from these methods continues to mean "abort this app's deploy step" (preserving today's engine behavior at engine.go:189). The new "stat-error" and "orphan-warn" cases internally emit a warning event and return nil — they are NOT errors from the engine's perspective. The only remaining error returns are: (a) the dynamic dir cannot be created, and (b) the YAML marshal or `os.WriteFile` fails on a fresh write. Alternatives: (a) a new `WarningError` type — rejected (Constitution IV; no other backend needs it); (b) a callback-style "emit-then-fail" — rejected (Constitution VII; uglier than two return paths). The chosen split makes FR-011 / SC-007 implementable without any new types.

3. **How does `RemoveRoute` get the observer event surface, given it's currently never called?** Decision: wire `engine.teardownKind` to call `engine.Routing.RemoveRoute(team, step.Name)` for `ApplicationKind` steps after `RemoveContainer` succeeds, gated on `engine.Routing != nil`. The host argument is the application name (mirrors the route file name `<team>-<name>.yml`, derived in `routeFileName`). Alternatives: (a) add a new "audit orphan files" CLI command — rejected (FR-005: no new commands/flags); (b) only emit the orphan warning on next deploy when an unreferenced per-app file is found — rejected because "next deploy" may never happen, and the warning fires too late to be useful. The chosen wiring fires the warning exactly when the operator runs `shrine teardown`, which is the discoverable moment for the action.

The `research.md` doc also records:
- The audit confirmation that `RemoveRoute` is currently uncalled (verified by `grep -rn 'RemoveRoute\b' internal/ cmd/` — only definition sites, no callers).
- The reuse opportunity: `isStaticConfigPresent` already implements exactly the existence semantics FR-008 requires (use `Lstat`, treat any entry as "present", surface non-`IsNotExist` errors). Generalizing it is a 4-line refactor.
- Confirmation that the `lstatFn` test seam in `config_gen.go:16` carries over to the generalized helper without test rewrites.

## Phase 1: Design & Contracts

**Prerequisites**: `research.md` complete (three design decisions recorded; reuse opportunity verified).

Outputs:

- **`data-model.md`**: Short. There is no new data model. The doc records:
  - The per-app routing file lifecycle states: `Absent → Generated → Operator-Owned (preserved on every subsequent deploy) → Orphan (after teardown, if not deleted by operator)`. The transitions are: deploy when absent → Generated; deploy when present → Operator-Owned (no-op write); teardown when present → Orphan (no-op remove + warning); operator `rm` → Absent.
  - The in-memory extension of `RoutingBackend` struct: adds `observer engine.Observer`. No persisted state.
  - A note that the orphan state is an *operator-action* state, not a Shrine-managed state — Shrine flags it but does not transition out of it.

- **`contracts/log-events.md`**: The four event names and their fields. Names follow the existing `gateway.config.*` convention (so future log scrapers can pattern-match):
  - `gateway.route.generated` — Status: Info. Fields: `team`, `name`, `path`. Emitted by `WriteRoute` after a fresh write succeeds.
  - `gateway.route.preserved` — Status: Info. Fields: `team`, `name`, `path`. Emitted by `WriteRoute` when the file is present and the write was skipped. (Mirrors `gateway.config.preserved`'s shape from spec 004.)
  - `gateway.route.stat_error` — Status: Warning. Fields: `team`, `name`, `path`, `error`. Emitted by `WriteRoute` when `Lstat` returns a non-`IsNotExist` error. The deploy continues.
  - `gateway.route.orphan` — Status: Warning. Fields: `team`, `name`, `path`. Emitted by `RemoveRoute` when the file is present at teardown. Operator-actionable: the message names the path and instructs `rm` to fully tear the route down.
  The doc also records the compatibility guarantee: aliases for the existing `gateway.config.preserved` / `gateway.config.generated` (spec 004) are unchanged byte-for-byte; no existing field is renamed.

- **`quickstart.md`**: A 5-minute walkthrough framed as an operator session. Operator deploys an app, sees `gateway.route.generated` in the deploy log, hand-edits the per-app file to add a custom middleware, redeploys, sees `gateway.route.preserved` and confirms the middleware is intact, removes the app from the manifest, runs teardown, sees `gateway.route.orphan` with the file path, runs `rm` against the named path. The walkthrough is the operator-facing UX integration test for SC-006 (single-step recovery is discoverable).

- **Agent context update**: Replace the existing line in `CLAUDE.md` (lines 2-4, between `<!-- SPECKIT START -->` and `<!-- SPECKIT END -->`) so the plan pointer reads `specs/009-preserve-app-configs/plan.md`. No prose change beyond that — `AGENTS.md` is not touched by this feature (the operator-facing narrative lives in `quickstart.md`, which is the right home for a walkthrough; `AGENTS.md` continues to describe stable conventions).

## Re-Check (Post-Design)

After Phase 1 artifacts are written, re-evaluate Constitution Check.

Expected outcome: still all-Pass.

- Principle III: the design Phase 1 reaffirms that the `engine.RoutingBackend` interface is unchanged. The dryrun backend (`dry_run_routing.go`) needs no edits. Verified.
- Principle IV: the only new abstraction is the rename `isStaticConfigPresent → isPathPresent`. With three call sites (static config, per-app write, per-app teardown), the extract-rule is satisfied exactly. No premature helpers (`emitRouteGenerated`/`emitRoutePreserved`/etc. are inline 2-line emissions, not helper functions, unless code review at task time finds the inline form repeats verbatim 3+ times — in which case extraction is mandatory).
- Principle V: the integration test at `traefik_plugin_test.go` is a real-binary, real-Docker scenario. It does not require a Next.js or other application image — the standard `nginx`-style placeholder used elsewhere in the suite is sufficient because the assertion is filesystem-level (file bytes), not HTTP-level. The [memory: integration tests are slow ~10 min] guidance is respected: exactly one scenario is added.
- Principle VII: the helper extraction (`isPathPresent`) and the named emit-helpers (if extracted) keep WHAT-comments out of the codebase. The one WHY-comment in scope is "use Lstat not Stat — symlinks count as present per FR-008" above the helper's `lstatFn` call.

The Complexity Tracking table remains empty.
