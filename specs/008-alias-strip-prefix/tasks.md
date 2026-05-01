---

description: "Task list for feature 008-alias-strip-prefix (per-alias opt-out of path prefix stripping)"
---

# Tasks: Per-Alias Opt-Out of Path Prefix Stripping

**Input**: Design documents from `/specs/008-alias-strip-prefix/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/log-format.md, quickstart.md

**Tests**: Constitution Principle V mandates a passing integration round-trip. Per spec clarification Q4, no new HTTP-level integration scenario is added — the bug class is structural (middleware-list correctness, already covered). The only new test is a unit-level extension of `TestFormatAliasesForLog` for FR-010, written FIRST per TDD before the log-marker implementation lands.

**Organization**: Tasks are grouped by user story. The spec has one user story (P1 — Next.js basePath redirect-loop fix); the audit phase is the foundational prerequisite that determines which FRs are Gaps versus Verified.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1 = P1 in spec.md)
- File paths in descriptions are absolute or repo-relative as appropriate

## Path Conventions

Repository-root layout per `plan.md`:

- Source: `internal/engine/`, `internal/manifest/`, `internal/plugins/gateway/traefik/`
- Tests: same package as source for unit tests; `tests/integration/` for the suite
- Docs: `specs/006-routing-aliases/contracts/manifest-schema.md`, `AGENTS.md`, `CLAUDE.md`
- Spec artifacts: `specs/008-alias-strip-prefix/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: None required. The Go module, Cobra CLI scaffold, manifest pipeline, engine package, and Traefik plugin all exist and shipped under earlier features. No initialization tasks are needed for this feature.

(Phase intentionally empty — proceed to Phase 2.)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Run the audit gate that confirms the verified FRs in `research.md` still pass on `main` before this feature's branch lands new code. If any Verified row regresses here, the diff for this feature MUST grow to fix it; otherwise it stays as documented.

**⚠️ CRITICAL**: No user story work begins until this phase is green.

- [X] T001 Run `go test ./internal/manifest/... ./internal/engine/... ./internal/plugins/gateway/traefik/...` from repo root and confirm all FR-001/FR-002/FR-003/FR-004/FR-005/FR-006/FR-009 unit cases pass green. Specifically verify these named cases are present and passing: `TestParse_ApplicationManifest_MultiAlias` (parser_test.go:111), `TestResolveAliasRoutes` all 5 subcases (engine_aliases_test.go:11-68), `TestWriteRoute_OneAlias_Strip`, `TestWriteRoute_OneAlias_NoStrip`, `TestWriteRoute_HostOnlyAlias`, `TestWriteRoute_ThreeAliases_SparseStrip` (routing_test.go:188-290), `validate_test.go` "valid alias with explicit stripPrefix false" (line 307). FR-007 is NOT directly unit-tested; its evidence is structural — `WriteRoute` (`internal/plugins/gateway/traefik/routing.go:37-95`) regenerates the entire dynamic-config YAML from `op.AdditionalRoutes` on every call, so stale strip-middleware entries cannot persist across re-deploys; T010's full integration-suite re-run catches any structural regression. Record any regression in research.md and expand this feature's scope to fix; otherwise proceed.

- [X] T002 Run `go test -tags integration ./tests/integration/... -run TestTraefikPlugin/should_publish_multiple_alias_routers_with_sparse_strip_indexing` and confirm the structural assertion `alias-2 has stripPrefix:false — strip-2 must not be emitted` (traefik_plugin_test.go:619) still passes. This is the integration evidence for FR-003 / FR-005 against a real Docker daemon (Constitution V). If integration suite is unavailable on the dev host, document the local-vs-CI gap and defer to CI gate.

**Checkpoint**: Audit green. The Verified rows in `research.md` are still valid. User story implementation can begin.

---

## Phase 3: User Story 1 — Publish a Next.js app under a Tailscale alias without redirect loops (Priority: P1) 🎯 MVP

**Goal**: An operator with a basePath-aware backend (Next.js, Grafana, JupyterLab, etc.) can publish that backend under a `routing.aliases[]` entry that forwards the prefix unchanged. The fix is discoverable from two operator-facing doc homes (manifest schema contract and `AGENTS.md`), and the deploy log signals which aliases run with `stripPrefix: false` so operators can confirm intent without inspecting generated config files.

**Independent Test** (per spec.md): Deploy an application with a single alias declaring `host: gateway.example`, `pathPrefix: /finances`, `stripPrefix: false`. Verify the generated dynamic-config YAML for the alias router has no strip middleware. Verify the deploy log for that deploy contains `gateway.example+/finances (no strip)` in the `aliases` field of the `routing.configure` event. Then change to `stripPrefix: true` (or remove the field) and re-deploy; verify the YAML now attaches the strip middleware AND the log no longer carries the `(no strip)` marker for that alias.

### TDD test for User Story 1 (FR-010 unit-level coverage) ⚠️

> **NOTE: Write the test FIRST and confirm it FAILS before T004 lands the implementation.**

- [X] T003 [US1] Extend `TestFormatAliasesForLog` in `/root/projects/shrine/internal/engine/engine_aliases_test.go` (current cases at lines 70-105) with three new subtests, additive only — do NOT modify the three existing cases:
  - `single alias with no-strip marker` — input `[]AliasRoute{{Host: "gateway.x.y", PathPrefix: "/finances", StripPrefix: false}}`, expected output `gateway.x.y+/finances (no strip)`. Pins FR-010 single-alias behavior (Example 3 of contracts/log-format.md).
  - `mixed strip across three aliases` — input matching contracts/log-format.md Example 4 (host-only, default-strip, no-strip), expected output `gateway.tail9a6ddb.ts.net+/notes,gateway.tail9a6ddb.ts.net+/notes-raw (no strip),lan.home.lab`. Pins per-entry marker placement and sort ordering.
  - `host-only alias with stripPrefix=false is no-op` — input `[]AliasRoute{{Host: "gateway.x.y", PathPrefix: "", StripPrefix: false}}`, expected output `gateway.x.y` (no marker). Pins FR-004 / Example 5 — the marker MUST be gated on `PathPrefix != ""`, not on `!StripPrefix` alone.
  Run `go test ./internal/engine/ -run TestFormatAliasesForLog -v` and confirm all three new subtests FAIL with `formatAliasesForLog` in its current (pre-marker) form. Confirm the original three subtests still PASS (no regression on FR-009 byte-stability).

### Implementation for User Story 1

- [X] T004 [US1] Modify `formatAliasesForLog` in `/root/projects/shrine/internal/engine/engine.go` (lines 324-335) to append `" (no strip)"` to per-entry strings whose alias has `r.PathPrefix != "" && !r.StripPrefix`. Keep the function as a single named function (no extraction — see data-model.md Decision 3 / Constitution IV). The post-modification body builds each entry as `r.Host`, then conditionally appends `"+"+r.PathPrefix` when prefix is non-empty, then conditionally appends `" (no strip)"` when prefix is non-empty AND `!r.StripPrefix`. Do not reorder or rename anything else; the sort and join lines stay untouched. Run `go test ./internal/engine/ -run TestFormatAliasesForLog -v` and confirm all six subtests (three existing + three new from T003) PASS.

- [X] T005 [P] [US1] Extend the `## Field reference` section for `spec.routing.aliases[].stripPrefix` in `/root/projects/shrine/specs/006-routing-aliases/contracts/manifest-schema.md` (lines 59-66) with a new prose paragraph describing WHEN to set `stripPrefix: false`. Anchor it concretely: "Set to `false` when the backend has a basePath / sub-path / context-path configured internally (Next.js `basePath`, Grafana `[server] root_url`, JupyterLab `--ServerApp.base_url`, etc.) — these backends generate redirects and asset URLs that include the prefix and will 404 / loop if Shrine strips it before forwarding." Keep the existing default rules unchanged. (FR-008 home A.)

- [X] T006 [P] [US1] Add a new `## Symptom → fix` subsection to `/root/projects/shrine/specs/006-routing-aliases/contracts/manifest-schema.md` (place it before `## Compatibility guarantees`, after `## Validation errors (operator-facing)`). Document the issue-#9 failure mode in operator-facing language: the redirect-loop / asset-404 symptom, the diagnostic ("server logs show requests with the prefix already stripped"), the fix (`stripPrefix: false` on the alias), and a one-liner cross-reference to `specs/008-alias-strip-prefix/quickstart.md` for the full walkthrough. Match the existing prose register of the file (no headings deeper than h3, no code blocks beyond the inline YAML one-liners already used). (FR-008 home A continued.)

- [X] T007 [P] [US1] Update the `### Application` example block in `/root/projects/shrine/AGENTS.md` (lines 57-60) so the alias example annotates the basePath case explicitly. Replace the single-line annotation `# default true when pathPrefix is set; set false to forward unchanged` with a small block that calls out the symptom: keep the YAML example shape, but append a one-paragraph note immediately after the closing ``` of that block (before line 86's `Each env entry uses ...`) saying: "If the backend handles the path prefix itself (Next.js with `basePath`, Grafana with `root_url`, JupyterLab with `base_url`), set `stripPrefix: false` on the alias — otherwise Shrine strips the prefix before the request reaches the backend, causing redirect loops and asset 404s. See `specs/008-alias-strip-prefix/quickstart.md` for the full diagnosis-and-fix walkthrough." Do NOT alter any unrelated lines of `AGENTS.md`. (FR-008 home B.)

- [X] T008 [US1] Run `go test ./...` from repo root and confirm the entire unit suite is green (all engine, manifest, traefik plugin, and other unit tests pass). This is the FR-009 byte-stability gate at the unit level — no test that was green before this feature should now be red.

**Checkpoint**: User Story 1 is fully functional. The deploy log marker shipped (T004), unit tests pass (T008), and the operator-facing docs in both homes (T005, T006, T007) describe the redirect-loop symptom and the `stripPrefix: false` fix. An operator hitting issue #9 can now self-serve from the docs and confirm the fix from the deploy log.

---

## Phase 4: Polish & Cross-Cutting Concerns

**Purpose**: Final validation gates and quickstart walk-through as integration-level proof.

- [X] T009 [P] Walk through `/root/projects/shrine/specs/008-alias-strip-prefix/quickstart.md` end-to-end against a local deploy: deploy a manifest with `pathPrefix: /finances`, `stripPrefix: false`, confirm the deploy log emits `routing.configure` with `aliases=...+/finances (no strip)`, confirm the dynamic-config YAML has no `middlewares:` section attached to that alias router. Record any divergence between quickstart prose and observed output as a quickstart edit, not a code change — quickstart is the operator-facing UX integration test for SC-001.

- [X] T010 [P] Run the full integration suite `go test -tags integration ./tests/integration/...` (or `make test-integration`) and confirm every existing scenario remains green. Per spec clarification Q4, no new integration scenario is added under this feature; this run is the Constitution V end-to-end gate confirming this feature did not regress any existing alias scenario, especially the FR-003 evidence at `traefik_plugin_test.go:619` (alias-2 stripPrefix:false → strip-2 absent). Memory note: this run takes ~10 minutes — schedule accordingly.

- [X] T011 Verify the `<!-- SPECKIT START -->` block in `/root/projects/shrine/CLAUDE.md` points at `specs/008-alias-strip-prefix/plan.md` (already updated during /speckit-plan). No code change here — this task is the final consistency check before merge.

- [X] T012 Mark `specs/008-alias-strip-prefix/checklists/requirements.md` items complete and confirm spec/plan/tasks artifacts cross-reference correctly: `spec.md` references issue #9 in the Input header; `plan.md` references `spec.md` in its header; `tasks.md` references `plan.md` and `spec.md` in its header; `research.md` cites the file:line evidence for each Verified FR. This is the bookkeeping pass before opening the PR.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: empty — skip.
- **Phase 2 (Foundational / Audit)**: T001 must complete before any User Story 1 task. T002 may run in parallel with T001 if Docker is available; otherwise sequence T002 after T001 (or document the gap and defer to CI).
- **Phase 3 (User Story 1)**: starts after T001 (and T002, when available) green.
- **Phase 4 (Polish)**: starts after the User Story 1 checkpoint (T008) is green.

### Within User Story 1

- **T003 (test)** strictly precedes **T004 (impl)** — TDD: confirm the new subtests FAIL on the current `formatAliasesForLog`, then make them PASS.
- **T005, T006, T007** are documentation tasks on three different files (`manifest-schema.md`, `manifest-schema.md`, `AGENTS.md`); T005 and T006 touch the same file (`manifest-schema.md`) and MUST sequence (T005 → T006). T007 is a different file and can run in parallel with either.
- **T008 (full unit suite)** depends on T004 landing and the doc tasks not having broken anything (defensive — docs shouldn't affect tests, but the repo runs Go tests on every push).

### Parallel Opportunities

- T005 ↔ T007 (different files in different packages — schema contract vs AGENTS.md).
- T009 ↔ T010 (quickstart walkthrough does not depend on integration suite output).
- T003 (writing the failing test) can be authored in parallel with T005, T006, T007 (docs); only the *running* of T003 must precede T004.

---

## Parallel Example: User Story 1 — docs and test in parallel

```bash
# After T001/T002 audit completes, three contributors can proceed:
# Contributor A: write the failing unit test
T003 — extend TestFormatAliasesForLog in internal/engine/engine_aliases_test.go

# Contributor B: extend the schema contract docs
T005 → T006 — sequence on specs/006-routing-aliases/contracts/manifest-schema.md

# Contributor C: extend AGENTS.md
T007 — edit AGENTS.md

# Then sequence:
T004 — implement formatAliasesForLog marker (after T003 confirms RED)
T008 — run go test ./... as the integration gate for User Story 1
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

This feature has only one user story; the MVP and the full feature are the same. Suggested ordering for a single contributor:

1. **Phase 2 audit (T001, T002)** — confirms the work surface is exactly what `research.md` says it is. ~5 minutes (T001) + ~10 minutes (T002 integration suite). If T001 surfaces a regression, expand the feature's scope before continuing.
2. **TDD (T003 → T004)** — writes the failing tests, confirms RED, lands the marker, confirms GREEN. ~15 minutes.
3. **Docs (T005, T006, T007)** — three small edits, no code. ~20 minutes.
4. **Polish (T008 → T009 → T010 → T011 → T012)** — full unit suite, quickstart walk-through, integration-suite re-run, consistency check, bookkeeping. ~30 minutes plus the integration suite's own ~10-minute runtime.

Total: ~80 minutes of contributor work + ~20 minutes of integration-suite wall-clock time.

### Why no Phase 1 / no new integration scenario

The audit-then-fix-gaps framing (spec clarification Q1) means no project-initialization or new-package work is on the critical path; the spec's mandate is to verify and close gaps. Per Q4, the bug class is structural (middleware-list correctness, already covered by `routing_test.go` and `traefik_plugin_test.go`), so the integration scenario list under this feature is empty — Phase 4 only re-runs the existing suite as a regression gate.

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks
- [Story] label maps task to a specific user story for traceability ([US1] = P1 in spec.md)
- T003 must FAIL before T004 lands the implementation — TDD is non-negotiable here per Constitution V's TDD rule
- Commit after each task or logical group; suggested groups: {T001,T002}, {T003,T004}, {T005,T006,T007}, {T008}, {T009,T010,T011,T012}
- If T010 surfaces a regression in the existing integration suite, that is a blocker — fix before merging
- The Constitution V documented exception (no new HTTP-level integration scenario) is captured in `plan.md` Constitution Check; do not relitigate it during PR review
