# Implementation Plan: Per-Alias Opt-Out of Path Prefix Stripping

**Branch**: `008-alias-strip-prefix` | **Date**: 2026-05-01 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/008-alias-strip-prefix/spec.md`

## Summary

Issue #9 reports a redirect-loop failure mode for basePath-aware backends (Next.js, Grafana, JupyterLab, etc.) when published behind a `routing.aliases[]` entry that strips its `pathPrefix`. The remediation — an opt-out boolean on each alias entry — was already specced in 006 and shipped in PR #7 (commit `1a31dac`). Per the spec's clarifications (2026-05-01), this feature is therefore an **audit-then-fix-gaps** pass: planning starts by verifying each FR in the spec against the current `main`; only un-satisfied FRs receive new implementation work.

The expected gaps on entry (validated in Phase 0) are:

1. **FR-008 (operator-facing docs)** — the manifest schema contract at `specs/006-routing-aliases/contracts/manifest-schema.md` already describes `stripPrefix` and shows a no-strip example, but does not anchor that example to the Next.js basePath / redirect-loop / asset-404 symptom that drove issue #9. `AGENTS.md` mentions `stripPrefix` only as a one-line annotation in the alias example. Operators hitting the bug have no narrative breadcrumb to the fix in either home.
2. **FR-010 (deploy-log marker)** — `formatAliasesForLog` (`internal/engine/engine.go:324`) emits `host` or `host+pathPrefix` per alias and does not differentiate stripping vs forwarding. There is no log signal for `stripPrefix: false`; this FR is unimplemented.

FR-001 through FR-007 and FR-009 are expected to verify clean (covered by `engine_aliases_test.go`, `routing_test.go`, `validate_test.go`, `parser_test.go`, and structural cases in `tests/integration/traefik_plugin_test.go`); the audit produces a verification matrix in `research.md` rather than new code. Per Q4, no HTTP-level integration test is added — structural coverage of middleware emission is the bug-class evidence that matters.

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: existing — no new modules. Touches `internal/engine`, `specs/006-routing-aliases/contracts/`, `AGENTS.md`, `CLAUDE.md`.
**Storage**: N/A — this feature does not write state. The existing dynamic-config writer (`internal/plugins/gateway/traefik/routing.go`) already emits the strip middleware correctly per `stripPrefix`; no behavior change there.
**Testing**: `go test ./internal/...` for the unit-level FR-010 log-marker change and the audit-driven verification matrix; integration suite is NOT extended under this feature (per Q4 — bug class is "wrong middleware composition," covered structurally by existing `routing_test.go` and `traefik_plugin_test.go` cases).
**Target Platform**: Linux server (homelab); single-binary CLI deployed by `shrine deploy`.
**Project Type**: CLI tool with embedded execution engine and pluggable backends.
**Performance Goals**: N/A — log formatting runs once per app per deploy, O(aliases).
**Constraints**: SC-004 (byte-identical generated config for unaffected manifests) — the FR-010 log marker MUST appear only when an alias has `pathPrefix` set AND `stripPrefix: false`; the existing log shape for stripping aliases MUST be preserved character-for-character so existing log scrapers (if any) and existing `TestFormatAliasesForLog` cases keep passing.
**Scale/Scope**: Same as spec 006 — ≤100 apps × ≤10 aliases on a single homelab host.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] N/A — manifest field already exists (added under spec 006). No new field, no new flag. The work is verification + log-marker + docs. |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] N/A — no new commands. |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] N/A — no new backend logic. The FR-010 marker is added to the engine's existing log-formatting helper, which is plugin-agnostic and already lives at the engine layer (`formatAliasesForLog`). |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — no new types, no new helper functions beyond a possible boolean check inside `formatAliasesForLog`. The log-marker change is two lines of code; no abstraction is introduced. |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass with documented exception — per Clarifications Q4, the bug class is "wrong middleware composition" and is already covered structurally by `tests/integration/traefik_plugin_test.go:611` (alias-2 has `stripPrefix:false` → strip-2 must not be emitted). The audit verifies this assertion exists and passes; no new HTTP-level integration scenario is added because spinning up a Next.js backend would not strengthen the proof beyond what the YAML-level assertion already gives, and the integration suite runs ~10 min per pass (memory note). The FR-010 log-marker change is unit-level (`engine_aliases_test.go`) — log formatting is engine-internal with no Docker dependency. |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] N/A — this feature does not write state. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | [x] Pass — the FR-010 marker logic stays inside `formatAliasesForLog` (single call site). If a boolean check is needed, it goes inline as `r.PathPrefix != "" && !r.StripPrefix` — naming is self-evident from the field names; no comment needed. Documentation prose for FR-008 is operator-facing copy, not code, so the WHAT-comment rule does not apply. |

> No violations to track. Complexity Tracking table omitted.

## Project Structure

### Documentation (this feature)

```text
specs/008-alias-strip-prefix/
├── plan.md              # This file
├── research.md          # Phase 0 output — the audit verification matrix
├── data-model.md        # Phase 1 output — extension notes only (struct already exists in spec 006)
├── quickstart.md        # Phase 1 output — operator walkthrough: redirect-loop → fix → verify
├── contracts/
│   └── log-format.md        # Phase 1 output — the FR-010 deploy-log line shape
├── checklists/
│   └── requirements.md      # Already created by /speckit-specify
└── tasks.md             # Phase 2 output (NOT created here)
```

### Source Code (repository root)

```text
internal/
└── engine/
    ├── engine.go                  # MODIFY: formatAliasesForLog appends "(no strip)" when alias has pathPrefix and stripPrefix=false (FR-010)
    └── engine_aliases_test.go     # EXTEND: TestFormatAliasesForLog gains a no-strip case asserting "(no strip)" marker; existing cases unchanged

specs/006-routing-aliases/
└── contracts/
    └── manifest-schema.md         # MODIFY: extend the "Multiple aliases, mixed strip" example (already present) with a Next.js basePath narrative; add a new "Symptom → fix" subsection cross-referencing issue #9 (FR-008 home A)

AGENTS.md                          # MODIFY: extend the existing aliases block (lines 57-60) with a note describing the basePath / redirect-loop symptom and pointing to stripPrefix:false as the fix (FR-008 home B)
CLAUDE.md                          # MODIFY: SPECKIT block re-pointed to this plan (specs/008-alias-strip-prefix/plan.md); the FR-008 operator-context narrative lives in AGENTS.md (the canonical agent context per Constitution governance), CLAUDE.md only updates its plan reference

# VERIFY-ONLY (no code change expected; audit confirms):
internal/manifest/types.go                                  # FR-001: RoutingAlias.StripPrefix *bool
internal/manifest/validate.go                               # FR-006: stripPrefix accepted without error
internal/manifest/validate_test.go                          # FR-006 evidence
internal/manifest/parser_test.go                            # FR-001 evidence (nil/true/false unmarshal)
internal/engine/engine.go                                   # FR-002/FR-003 default-resolution in resolveAliasRoutes
internal/engine/engine_aliases_test.go                      # FR-002/FR-003/FR-004 evidence
internal/plugins/gateway/traefik/routing.go                 # FR-002/FR-003/FR-005 emit / omit strip middleware
internal/plugins/gateway/traefik/routing_test.go            # FR-002/FR-003/FR-005 evidence
tests/integration/traefik_plugin_test.go                    # FR-009 (byte-identical default), structural FR-003 evidence
```

**Structure Decision**: Single project. The diff under this feature is intentionally tiny — two source files modified (`internal/engine/engine.go` and `internal/engine/engine_aliases_test.go`), three documentation files modified (`specs/006-routing-aliases/contracts/manifest-schema.md`, `AGENTS.md`, `CLAUDE.md`), four spec artifacts created (`research.md`, `data-model.md`, `quickstart.md`, `contracts/log-format.md`). No new directories, no new packages, no new dependencies.

## Phase 0: Outline & Research

The Phase 0 deliverable is the **audit verification matrix**: one row per FR in the spec, mapping it to the file and (where applicable) test that proves it on the current `main` — or recording the gap that this feature must close.

Open questions distilled from Technical Context and the spec's Assumptions:

1. **Does PR #7's `routing_test.go` actually assert that NO strip middleware appears for `stripPrefix: false` (FR-003), or does it only assert the prefix value when stripping?** A pre-grep saw `routing_test.go:232` ("expected no middlewares section when StripPrefix=false") and `routing_test.go:288` ("expected no strip-2 middleware (StripPrefix=false at index 2)"); the audit confirms the assertion shape is "no middleware emitted" rather than "middleware list does not contain X."
2. **Does `parser_test.go` cover the `stripPrefix` YAML unmarshal cases (nil, true, false)?** A pre-grep saw assertions at lines 106-156 covering all three cases. The audit captures the exact case names so the matrix is auditable.
3. **Does `validate_test.go` confirm `stripPrefix: false` is accepted without error (FR-006)?** A pre-grep saw `validate_test.go:307` ("valid alias with explicit stripPrefix false"). The audit captures the case name.
4. **Does the existing `tests/integration/traefik_plugin_test.go` produce a YAML-level assertion that `strip-2` is absent for `stripPrefix:false`?** A pre-grep saw `traefik_plugin_test.go:611-620` doing exactly this. The audit confirms the assertion shape and run-side (subprocess against real Docker daemon, per Constitution V).
5. **Is there an existing operator-facing doc home for the FR-008 narrative beyond `manifest-schema.md` and `AGENTS.md`?** README, `docs/`, or other? Pre-survey found `AGENTS.md` is the canonical agent reference per Constitution governance ("guidance file ... runtime development reference"), and `README.md` does not exist as a routing-pattern home. The two-home choice from Q2 stands; the audit confirms no additional home is required to satisfy FR-008.
6. **What is the exact current shape of the per-alias log line so FR-010's marker is additive?** Read of `formatAliasesForLog` (`internal/engine/engine.go:324-335`) confirms the shape is comma-separated entries, each `host` or `host+pathPrefix`, sorted ascending. The marker design must preserve this shape for stripping aliases (SC-004) — appending `(no strip)` only on aliases that opt out.

Findings consolidate in `research.md` (next file). The matrix uses three statuses per FR: **Verified** (existing code/test proves it), **Gap** (this feature must close it), **N/A** (no work expected; documentary only).

## Phase 1: Design & Contracts

**Prerequisites**: `research.md` complete (audit matrix written; FR-008 and FR-010 confirmed as the only Gaps).

Outputs (each generated next):

- **`data-model.md`**: short — there is no new data model, only a reaffirmation of `RoutingAlias.StripPrefix *bool` from spec 006 and the engine-side projection `AliasRoute{Host, PathPrefix, StripPrefix bool}`. The doc records the default-resolution rule (`StripPrefix: nil → true if pathPrefix set, false otherwise`) so future readers do not relitigate it. State transitions: stateless (Traefik file watcher reconciles).
- **`contracts/log-format.md`**: the FR-010 log-line contract. Defines:
  - The exact format of the `aliases` event field (e.g., `host[+pathPrefix][ (no strip)]`, comma-joined, sorted).
  - The marker placement (suffix on the entry, not on the comma-joined string).
  - The compatibility guarantee: aliases without `(no strip)` keep their existing shape character-for-character.
  - Worked examples mapping representative alias-list inputs to expected log strings.
- **`quickstart.md`**: a 5-minute walkthrough framed as a debugging session — operator deploys a Next.js app with `pathPrefix: /finances`, hits the asset-404 / redirect-loop symptom, opens `AGENTS.md` and finds the `stripPrefix: false` recommendation, edits the manifest, redeploys, verifies in the deploy log that the alias is now annotated `(no strip)`, and confirms the app loads. The walkthrough is the operator-facing UX integration test for SC-001.
- **Agent context update**: replace the existing line in `CLAUDE.md` (lines 1-5, between `<!-- SPECKIT START -->` and `<!-- SPECKIT END -->`) so it points at `specs/008-alias-strip-prefix/plan.md`. The substantive operator narrative (the redirect-loop symptom and `stripPrefix: false` fix) belongs in `AGENTS.md` per Constitution governance ("`AGENTS.md` is the runtime development reference"); `CLAUDE.md` only carries the plan pointer.

## Re-Check (Post-Design)

After Phase 1 artifacts are written, re-evaluate Constitution Check.

Expected outcome: still all-Pass.

- Principle V's documented exception (no new HTTP-level integration test) is justified by the bug class (middleware-list correctness, already structurally tested) and the test-strategy clarification (Q4) recorded in the spec. The audit step itself acts as the integration-gate equivalent for the verification half of the work — running the existing suite to confirm Verified rows still pass is part of the Phase 2 task list.
- Principle VII applies to the `formatAliasesForLog` change: the no-strip branch stays inline; if its readability degrades when added, the function gets refactored into two named helpers (`formatOneAliasEntry` + `joinAliasEntries`) before the change lands. That decision is captured in `data-model.md` so the implementer does not have to relitigate it.
- The Complexity Tracking table remains empty.
