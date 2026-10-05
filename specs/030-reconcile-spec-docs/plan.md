# Implementation Plan: Reconcile Specs, Tracking Metadata, and Docs with Shipped Behaviour

**Branch**: `030-reconcile-spec-docs` | **Date**: 2026-10-05 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/030-reconcile-spec-docs/spec.md` (GitHub issue #39)

## Summary

The spec set, the published docs, and the project's tracking files disagree with the shipped product in six areas the issue names. This feature corrects the text and changes no product code. Research re-verified every claim against `main` @ `64c93bc` by reading the code and running the real binary in `--dry-run`, and found the same defects in a handful of adjacent places ([research.md](research.md) Part 3), which are included.

The approach, in priority order:

1. **One canonical statement** of the generated-gateway-file lifecycle goes into spec 009 as a linkable section. Specs 004, 006, 008, 011, and 012 are amended to agree with it and link to it, each amendment keeping its identifier and quoting the original wording.
2. **The same fact for operators**: a "Known limitations" section in the Traefik gateway guide, linked from the routing guide, the TLS guide, and a new troubleshooting entry; two false sentences in the TLS guide corrected.
3. **Specs say what shipped**: spec 015 moved to the env/outputs shape spec 021 delivered; spec 009's orphan warning placed on teardown; two requirements (and one scenario found in research) marked descoped with entries in the known-gaps list; two task entries corrected.
4. **A wiring guide** built around one worked example whose manifests and output were verified against the binary.
5. **Tracking metadata** brought up to date: progress file, specs README, three legacy feature statuses, two lines of the project reference.

The exact text of the canonical statement and the known-limitations section, the required meaning of every other amendment, and the guide's verified manifests are fixed in [contracts/](contracts/).

## Technical Context

**Language/Version**: Markdown (GitHub-flavoured for `specs/` and `AGENTS.md`; Hugo/Goldmark for `docs/content/`). Go 1.25 is used only to build the binary that verifies the guide's manifests; no Go file changes
**Primary Dependencies**: None new. Docs site: Hugo v0.161.1 extended + Hextra theme, both already pinned in the `Makefile`
**Storage**: N/A
**Testing**: Local gates in [quickstart.md](quickstart.md) — scope check, front-matter lint, `make docs-build`, Markdown-companion checks, link and anchor check over the built site, identifier-stability diff, and a preview run of the guide's manifests. CI: the existing docs workflow. No Go tests are added or run; the integration suite is untouched and not run locally
**Target Platform**: GitHub (spec and reference rendering) and GitHub Pages (docs site)
**Project Type**: Documentation-only change to a single Go CLI repository
**Performance Goals**: N/A
**Constraints**: No file under `cmd/`, `internal/`, `tests/`, or build configuration changes (FR-029). No requirement identifier changes (FR-007). Clarification records and earlier features' design artifacts are not edited (research D4, D9). Every quoted output line must be one the product prints (research F3, F7; FR-027). The docs CI does not check plain links, so Gate 3 is the only protection against a broken anchor (research F10)
**Scale/Scope**: 24 tracked files — 1 new docs page, 7 edited docs pages, 8 edited specs, 2 edited task lists, 3 legacy feature files, and 3 tracking files (`specs/progress.md`, `specs/README.md`, `AGENTS.md`) — plus the `CLAUDE.md` plan pointer, and 2 untracked empty directories removed locally. Roughly 65 individual edits, enumerated per file in [data-model.md](data-model.md) §2

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | N/A — no capability added. The new guide documents existing manifest fields and its examples were validated against the shipped schema |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | N/A — no command added. The feature corrects three places that overstated what `--dry-run` prints |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | N/A — no product code |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | Pass — the limitation is a section in an existing guide, not a new page (D7); the README describes the directory convention instead of indexing 29 specs (D13); no docs tooling or link checker is added (D16); earlier features' design artifacts are left alone (D9) |
| V. Integration-Test Gate | Does this phase map to an integration test phase in `specs/features/integration-tests.md` using `NewDockerSuite` against a real binary? | N/A — no behaviour changes, so there is nothing for the integration gate to exercise. The feature's own gate is quickstart Gates 1–5; Gate 5 runs the real binary against the guide's manifests |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | N/A — no state code |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | N/A for code. The prose equivalent holds: the policy is stated once and linked, not restated in six specs (D1) |

**Governance clause**: "`AGENTS.md` … MUST be kept consistent with this Constitution." This feature moves `AGENTS.md` toward consistency (two stale lines corrected) and changes nothing the constitution governs.

**Noted, not a violation of this feature**: the constitution's Development Workflow section still says new features need a spec in `specs/features/<name>.md` and locates integration tests at `test/integration/`. Practice since spec 001 is numbered directories and `tests/integration/`. The refreshed specs README documents the practice as it is. Bringing the constitution's wording in line needs an amendment and version bump through `/speckit-constitution` and is outside this feature (research Part 4).

**Post-Phase-1 re-check**: unchanged — no gate applies beyond IV, which passes. No Complexity Tracking entries required.

## Scope Adjustments from Research

Seven places where research found the issue's defects outside the lines it cites. Each is included and the spec was updated to match; details and reasons in [research.md](research.md) Part 3.

| # | Finding | Effect on the spec |
|---|---|---|
| E1 | Spec 004 FR-008 still promises per-route files are "written and removed by Shrine" | FR-006 now covers spec 004 |
| E2 | Spec 011 SC-003 promises HTTPS on an existing deployment in one deploy | Within FR-006 |
| E3 | Seven further contradicting statements in specs 006, 008, 012 (two of them, 006 SC-001 and 012 SC-001, found by the T010 sweep during implementation) | Within FR-006 |
| E4 | More inputs fail to propagate than the spec listed (gateway `port`, `dashboard.port`, `spec.port`, primary `pathPrefix`, alias `host`) | FR-004 broadened |
| E5 | Spec 015 US3 scenario 1 and the secrets vault guide claim the preview prints env placeholders; it prints no env at all | FR-013 sharpened; FR-032 added |
| E6 | `AGENTS.md` describes per-app route files as "managed" | FR-021 broadened |
| E7 | `progress.md` reports Go 1.24.4 (`go.mod`: 1.25.0) | Within FR-018 |

Two findings changed how something must be worded rather than what is in scope:

- **The orphan warning fires once, at `shrine teardown <team>`** — not on every deploy until the file is removed, as spec 009's rationale says, and not from `shrine delete application` (research F5).
- **A regenerated `traefik.yml` does not take effect until the gateway container restarts**, and a deploy restarts it only when the container's own configuration changed. The operator-facing section says so (research F2).

Observed and deliberately left out: research Part 4.

## Project Structure

### Documentation (this feature)

```text
specs/030-reconcile-spec-docs/
├── plan.md              # This file
├── research.md          # Phase 0 — 12 current-state findings, 18 decisions, scope adjustments, out-of-scope observations
├── data-model.md        # Phase 1 — document entities, complete edit inventory, consistency rules
├── quickstart.md        # Phase 1 — six verification gates
├── contracts/
│   ├── spec-amendments.md   # Canonical statement (verbatim), note formats, required meaning of every spec amendment, known-gaps entries (verbatim)
│   ├── docs-pages.md        # Known-limitations section (verbatim), link points, guide corrections, front matter
│   └── wiring-guide.md      # Guide outline, verified manifests, verified output and error messages
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks — NOT created by /speckit-plan)
```

### Repository files changed

```text
specs/
├── 004-preserve-traefik-yml/spec.md            # MODIFIED: Amendments; FR-008
├── 006-routing-aliases/spec.md                 # MODIFIED: Amendments; edge case, FR-009, SC-004
├── 008-alias-strip-prefix/spec.md              # MODIFIED: Amendments; US1 test, edge case, FR-007, SC-001
├── 009-preserve-app-configs/spec.md            # MODIFIED: Amendments; NEW canonical section; 8 orphan-warning statements
├── 011-traefik-tlsport-config/spec.md          # MODIFIED: Amendments + terminology; edge case, FR-003, SC-003
├── 012-tls-alias-routers/
│   ├── spec.md                                 # MODIFIED: Amendments + terminology; US2-AS2, SC-005; FR-004 partly descoped; SC-004
│   └── tasks.md                                # MODIFIED: T006 description
├── 015-infisical-secrets-vault/spec.md         # MODIFIED: Amendments; 7 old-shape statements; US3-AS1 descoped
├── 021-resource-env-output-split/
│   ├── spec.md                                 # MODIFIED: Amendments; FR-013 descoped; US1 Independent Test
│   └── tasks.md                                # MODIFIED: T029 descoped
├── features/
│   ├── routing.md                              # MODIFIED: status → superseded, pointer, differences
│   ├── logging-observer.md                     # MODIFIED: status → done, criteria checked
│   └── integration-tests.md                    # MODIFIED: status and per-phase markers
├── progress.md                                 # MODIFIED: phases 9/11/12/13, Current State, 2 Known Gaps, entry for 030
├── README.md                                   # MODIFIED: layout, ritual, legacy table, unused numbers
├── 005-traefik-entrypoints/                    # REMOVED locally (untracked, empty)
└── 007-fix-traefik-dynamic-dashboard/          # REMOVED locally (untracked, empty)

docs/content/
├── guides/
│   ├── wiring-env-and-outputs.md               # NEW
│   ├── _index.md                               # MODIFIED: one list entry
│   ├── traefik.md                              # MODIFIED: NEW "Known limitations" section; 4 link edits
│   ├── routing-and-aliases.md                  # MODIFIED: NEW section "Changing routing after the first deploy"
│   ├── tls.md                                  # MODIFIED: 2 corrections, 1 link sentence
│   └── secrets-vault.md                        # MODIFIED: "Dry-run behaviour" section
├── reference/manifest-schema.md                # MODIFIED: one link sentence
└── troubleshooting/_index.md                   # MODIFIED: one new entry

AGENTS.md                                       # MODIFIED: line 185 (planner entry points), line 304 (generated files)
CLAUDE.md                                       # MODIFIED: plan pointer (done by /speckit-plan)

cmd/, internal/, tests/, main.go, Makefile, .github/, .goreleaser.yml, docs/content/cli/   # UNCHANGED
```

**Structure Decision**: No new directories and no new tooling. One new page in the existing guides section; everything else is an edit in place. Spec amendments follow the note formats in [contracts/spec-amendments.md](contracts/spec-amendments.md) §2; docs edits follow the existing guide conventions (root-relative Markdown links, the "What this guide covers / Concept / … / Common pitfalls / See also" section pattern, YAML front matter with `title`, `description`, `weight`).

## Implementation Outline

Five work packages, one per user story, in priority order. Each is independently reviewable and is one commit (research D17). Within a package, steps that touch the same file are sequential; [data-model.md](data-model.md) §2 lists the files shared between packages (`009/spec.md`, `012/spec.md`, `progress.md`).

**WP1 — Canonical preserve policy (US1, P1)**

1. Spec 009: add the Amendments section and the canonical section verbatim (spec-amendments §1).
2. Specs 004, 006, 008: Amendments section, then each statement in spec-amendments §4.
3. Specs 011, 012: Amendments section with the terminology note, then each statement in §4.
4. Gate 4: canonical link counts, single canonical heading, identifier-stability diff.

**WP2 — Operator-facing limitation (US2, P1)**

5. Traefik guide: insert the "Known limitations" section verbatim (docs-pages §1), then the four link edits (§2).
6. Routing guide: new section. TLS guide: two corrections and one link sentence (§2, §3). Troubleshooting: new entry.
7. Gates 2 and 3. Read the canonical section and the known-limitations section side by side (consistency rule 2).

**WP3 — Specs describe what shipped (US3, P2)**

8. Spec 009: the eight orphan-warning statements (spec-amendments §5); extend the Amendments entry from WP1.
9. Spec 015: Amendments section, seven old-shape statements, US3-AS1 descope (§6).
10. Spec 021: FR-013 descope, US1 Independent Test, task T029. Spec 012: FR-004, SC-004, task T006; extend the Amendments entry from WP1.
11. `progress.md`: the two Known Gaps entries verbatim (§3).
12. Secrets vault guide: correct "Dry-run behaviour" (docs-pages §4).
13. Gate 4: the 015, 009, and descope greps; identifier-stability diff again.

**WP4 — Wiring guide (US4, P2)**

14. Write `wiring-env-and-outputs.md` from the outline and verified material in the wiring-guide contract.
15. Add the guides-index entry and the manifest-reference link (docs-pages §5).
16. Gate 5: build the binary, copy the manifests out of the page as written, preview the same-team set and each cross-team stage; compare with the page.
17. Gates 2 and 3 again.

**WP5 — Tracking metadata (US5, P3)**

18. `progress.md`: phase lines, Current State, entry for this feature (research D12).
19. `specs/README.md`: rewrite layout, ritual, legacy table, unused numbers, status vocabulary (D13).
20. Legacy feature files: `routing.md` and `logging-observer.md` statuses; for `integration-tests.md`, check each pending phase's listed scenarios against the sub-test names in `tests/integration/` before marking it complete (D14).
21. `AGENTS.md`: the two lines (D15).
22. Remove the two empty directories from the working tree.
23. Gate 4: tracking-metadata greps.

**Close-out**

24. Run every gate once more on the whole change, including Gate 1 (scope) and the Gate 6 reading checks.
25. `graphify update .`
26. Open the pull request; the description lists the scope adjustments (E1–E7) and the out-of-scope observations from research Part 4 so the maintainer can decide on follow-ups. CI's docs workflow is the remote gate.

**Divergence protocol** (FR-031): if any statement being written turns out not to match the product, stop, follow the product, and record the divergence in the pull request description and in the affected research finding and contract.

## Complexity Tracking

No violations.
