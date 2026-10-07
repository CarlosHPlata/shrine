# Implementation Plan: Operator guide, managing image versions

**Branch**: `038-image-versions-guide` | **Date**: 2026-10-07 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/038-image-versions-guide/spec.md`

## Summary

This plan writes the operator guide "Managing image versions" (`docs/content/guides/image-versions.md`). The guide walks PRD journeys J1 to J8 on one example team, `shop`: a pinned application, a pinned database, and a manifest-owned cache. The plan also adds a troubleshooting entry for a pin the registry no longer serves, links the guide from the guides index and the manifest reference, and makes a vocabulary pass over the pages tickets T3 to T7 wrote. The pass rewrites the manifest reference's examples to match the real output and corrects three command help texts (`get`, `describe`, `status`) at their Cobra source, then regenerates their pages.

The owner chose not to use a container runtime. Every line the binary prints without a daemon is captured from real runs against scratch state. The remaining lines (real deploys, bumps, the running image, `status`) are assembled from the format strings in the code and checked against the integration suites' assertions ([research.md](research.md) R1 and R2).

## Technical Context

**Language/Version**: Markdown for the Hugo site (Hextra theme), plus Go 1.24 for three Cobra `Long` strings only.
**Primary Dependencies**: Hugo at the version pinned in the `Makefile`, and the `docs/tools/docsgen` generator (a separate Go module).
**Storage**: N/A.
**Testing**: `make docs-check` (front-matter lint, docsgen tests, CLI drift check, Hugo build, Markdown companion and shape checks); a grep-based link check over `docs/public` ([quickstart.md](quickstart.md) step 3); `go test ./...` and `go vet -tags integration ./tests/integration/...` unchanged.
**Target Platform**: The documentation site on GitHub Pages under `/shrine/`.
**Project Type**: CLI tool documentation.
**Performance Goals**: N/A.
**Constraints**: No container runtime (spec Clarifications). Generated CLI pages are never hand-edited. No product behaviour changes (FR-017).
**Scale/Scope**: One new page of about 400 lines, one troubleshooting section, one reference section rewritten in its examples, and three help texts.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | N/A: no new capability; documentation of existing fields. |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | N/A: no new command. Help text stays self-documenting via `--help`, and `AGENTS.md` needs no change (contracts/docs-touch-points.md). |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | N/A. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | Pass: no abstraction. One page and edits to existing pages; no configuration page or link-checker tool is added (research R8, design 4.11). |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | Pass by exemption: the ticket names no integration scenario (tickets.md T8) because nothing executable changes. The gate is `make docs-check` plus the link check, as for spec 030. Existing suites are untouched and must stay green in CI. |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | N/A. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | N/A for code. The help-text edits are string changes only. |

No violations, so Complexity Tracking stays empty.

**Post-design re-check**: unchanged. The contracts add no code path; the only Go edits are three `Long` strings.

## Project Structure

### Documentation (this feature)

```text
specs/038-image-versions-guide/
├── plan.md                       # this file
├── research.md                   # capture method, sources, example, placeholders, vocabulary findings
├── data-model.md                 # output blocks, placeholder set, pages touched
├── quickstart.md                 # verification steps
├── capture.sh                    # the daemon-free capture, reproducible by a reviewer
├── contracts/
│   ├── guide-outline.md          # the guide's sections and the provenance of each block
│   └── docs-touch-points.md      # troubleshooting, manifest reference, help texts, design amendment
├── checklists/requirements.md
└── tasks.md                      # /speckit-tasks
```

### Source (repository root)

```text
docs/content/
├── guides/
│   ├── _index.md                 # + one list line
│   └── image-versions.md         # new
├── troubleshooting/_index.md     # + "A deploy stops because a pinned version is no longer served"
├── reference/manifest-schema.md  # image pull policy section: link and faithful examples
└── cli/                          # regenerated: get_*.md, describe_app.md, describe_resource.md, status*.md
cmd/
├── get.go                        # Long: VERSION column
├── describe.go                   # Long: running image on every record
└── status.go                     # Long: IMAGE column wording
specs/epics/pinned-image-versions/design.md   # T8 amendment
specs/progress.md                             # entry for 038
```

**Structure Decision**: This is a documentation-only change to the existing Hugo site, plus three Cobra help strings whose pages are generated from them.

## Phase 0: Research

See [research.md](research.md). It covers the capture method (R1), the source of every assembled line (R2), the trim rules (R3), the example and its placeholders (R4, R5), where pins live during a host rebuild (R6), the vocabulary findings V1 to V6 (R7), the link check (R8), the page's place in the site (R9), and the design deviation (R10).

## Phase 1: Design

- [contracts/guide-outline.md](contracts/guide-outline.md): the guide's 14 sections and, for each output block, whether it is captured or assembled.
- [contracts/docs-touch-points.md](contracts/docs-touch-points.md): the troubleshooting entry, the manifest reference edits, the exact replacement help text, and the design amendment.
- [data-model.md](data-model.md): the rules output blocks follow and the pages they live in.
- [quickstart.md](quickstart.md): unit and integration compile, regeneration, `make docs-check`, the link check, the re-capture, and the reader check.

## Risks

- **An assembled line drifts from the binary.** Mitigation: each assembled line has a named format source and, where one exists, an integration assertion (research R2). The review pass reads the guide against those sources.
- **The daemon's error text differs by registry.** The `manifest unknown` text is the Docker daemon's message for Docker Hub. The troubleshooting entry tells the reader to read the appended cause, and does not rely on matching it.
- **Help-text edits are caught by the drift check only when pages are regenerated.** Mitigation: quickstart step 1 runs before step 2.

## Complexity Tracking

None.
