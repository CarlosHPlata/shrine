# Implementation Plan: Official Shrine CLI Documentation Site

**Branch**: `013-docs-site` | **Date**: 2026-05-02 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/013-docs-site/spec.md`

## Summary

Ship the first official Shrine CLI documentation site as a static site built with **Hugo (extended)** using the **Hextra** theme, hosted on **GitHub Pages**, with a **per-page "copy as Markdown" button** backed by Hugo's custom output formats (each page is published as both `/foo/` and `/foo/index.md`). The CLI reference is auto-generated from Cobra via a new `shrine docs gen <dir>` command so reference content cannot drift from `--help`. All tooling stays inside the Go ecosystem (`go install` for Hugo, Cobra for content generation) — no Python or Node runtime is added to the contributor setup.

## Technical Context

**Language/Version**: Go 1.24+ (existing); Hugo extended ≥ 0.135 (installed via `go install github.com/gohugoio/hugo@latest -tags extended` or pinned in CI)
**Primary Dependencies**:
- `github.com/spf13/cobra` (already used) — `cobra/doc` package for Markdown generation
- `github.com/gohugoio/hugo` — static site builder
- `github.com/imfing/hextra` — Hugo theme module (modern docs theme, built-in search & dark mode)
- Pagefind (vendored by Hextra) — static search index
**Storage**: Plain Markdown files committed under `docs/content/`. No database, no runtime state.
**Testing**:
- Unit: `go test ./internal/handler/docsgen/...` for generation logic
- Integration: `go test -tags integration ./tests/integration/...` covering `shrine docs gen` against the real binary (per Principle V)
- Site smoke: a CI job that runs `hugo --strict` (fails on broken refs) and verifies every published page has a `.md` companion file
**Target Platform**: Static HTML hosted on GitHub Pages; build runs in `ubuntu-latest` GitHub Actions
**Project Type**: Single Go project; docs site is an additional artifact under `docs/`
**Performance Goals**: Home page first-meaningful-paint < 2s on broadband (SC-005); Hugo build < 30s for the full site
**Constraints**:
- Toolchain MUST stay Go-only for contributors (no Python/Node required to author or build docs locally)
- Copy-as-Markdown output MUST be the verbatim source `.md` (no HTML→MD round-trip), so it is lossless for AI agents
- CLI reference MUST be regenerated, not hand-edited — drift from `--help` is treated as a build break
**Scale/Scope**:
- ~25–40 content pages at launch (overview, install, quick-start, ~12 CLI subcommand pages, Traefik/routing/TLS guides, troubleshooting)
- Single "latest" line of docs (no per-version archive in v1)
- English only

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] N/A — docs site does not introduce new infrastructure capabilities; no manifest surface. |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] N/A — no new commands are added to the `shrine` binary. The CLI-reference generator is a **separate** Go module (`docs/tools/docsgen/`) with its own binary, invoked via `make docs-gen-cli` (or `go run ./cmd/docsgen`); it is not a Shrine subcommand and does not participate in the kubectl-style surface. |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] N/A — no new infrastructure backend. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — single SSG, single theme, no plugin frameworks, no premature abstractions. The copy-as-Markdown feature is implemented as a Hugo output format + small partial, not a general "exporter framework". |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] N/A for the main `shrine` binary — this feature does not add behavior to the Shrine CLI itself. The auxiliary docsgen tool is exercised by its own unit tests in `docs/tools/docsgen/` (in-memory, no filesystem) plus a CI drift-check in US3 that runs the tool end-to-end against the real Cobra tree. The static site has a separate CI smoke job (`hugo --strict` + `.md` companion check). |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] N/A — no Docker interaction. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | [x] Pass — `docs/tools/docsgen/` exposes intention-revealing helpers (`generateCommandTree`, `writeMarkdownForCommand`, `ShouldSkipCommand`, `frontMatterFor`); no inline what-comments. |

> No violations. Complexity Tracking table omitted.

## Project Structure

### Documentation (this feature)

```text
specs/013-docs-site/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — decisions for Pages vs Wiki, Hugo vs alternatives, theme, search, copy-MD strategy
├── data-model.md        # Phase 1 output — Documentation Page, Navigation Tree, Copy-as-MD Output entities
├── quickstart.md        # Phase 1 output — local build, add a page, regenerate CLI ref, deploy
├── contracts/           # Phase 1 output
│   ├── cli-docs-gen.md  # `shrine docs gen <dir>` command contract
│   ├── copy-as-md-url.md# Per-page raw-Markdown URL contract
│   └── page-frontmatter.md # Required/optional Hugo front-matter for content pages
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created here)
```

### Source Code (repository root)

```text
docs/                                  # Hugo site root (NEW)
├── hugo.yaml                          # Site config (theme module, output formats, menus)
├── go.mod                             # Hugo module file (separate from project go.mod)
├── content/
│   ├── _index.md                      # Home page
│   ├── getting-started/
│   │   ├── _index.md
│   │   ├── install.md
│   │   └── quick-start.md
│   ├── cli/                           # AUTO-GENERATED — do not hand-edit
│   │   ├── _index.md                  # (curated index page; subcommand pages overwritten by `shrine docs gen`)
│   │   ├── apply.md
│   │   ├── deploy.md
│   │   └── ... (one per Cobra subcommand)
│   ├── guides/
│   │   ├── traefik.md
│   │   ├── routing-and-aliases.md
│   │   └── tls.md
│   ├── reference/
│   │   └── manifest-schema.md
│   └── troubleshooting/
│       └── _index.md
├── layouts/
│   ├── _default/
│   │   └── single.md                  # Custom output-format template — renders `{{ .RawContent }}` for copy-as-MD
│   └── partials/
│       └── copy-as-markdown.html      # The button shown on every page
└── static/                            # Logo, favicon, screenshots

docs/tools/docsgen/                    # NEW — separate Go module; auxiliary tooling
├── go.mod                             # OWN go.mod — replaces shrine via local relative path
├── go.sum
├── docsgen.go                         # package docsgen — generation logic + WriteCommand
├── docsgen_test.go                    # Unit tests, in-memory only (no filesystem)
└── cmd/
    └── docsgen/
        └── main.go                    # package main — `docsgen -out <dir> [-clean] [-include-hidden]`

cmd/
└── root.go                            # MODIFIED — exposes RootCmd() getter for the external docsgen module

.github/
└── workflows/
    └── docs.yml                       # NEW — build & deploy site to GitHub Pages on push to main
```

**Structure Decision**: The Hugo site lives at `docs/`, a sibling of the Go source tree. The CLI-reference generator lives at `docs/tools/docsgen/` as a **separate Go module** with its own `go.mod` and its own `go.sum`. Per the project memory note "Auxiliary tooling lives in a separate Go module", this is required so that:
1. Documentation generation does not appear under `internal/` — that path is for code that ships with the `shrine` binary.
2. The main module's `go.mod` is **not** polluted with docs-generation dependencies (e.g., `github.com/spf13/cobra/doc`, `cpuguy83/go-md2man`, `russross/blackfriday`).
3. The docsgen tool is "as if it were a different project" — installable, testable, and replaceable on its own.

The docsgen module reaches into the main module via a `replace github.com/CarlosHPlata/shrine => ../../..` directive, importing only the public `cmd.RootCmd()` getter to obtain the assembled Cobra tree. Generated reference content under `docs/content/cli/` is overwritten on every docs build via `make docs-gen-cli`; contributors never edit those files by hand.

## Phase 0 — Outline & Research

See [research.md](./research.md). All open decisions resolved:

1. **Hosting platform** → GitHub Pages (Wiki rejected because it cannot host the copy-as-Markdown button).
2. **Static site generator** → Hugo extended (chosen over MkDocs Material, Docusaurus, Jekyll, VitePress to keep the contributor toolchain Go-only — see feedback memory).
3. **Theme** → Hextra (modern docs theme, built-in Pagefind search, dark mode, code-copy buttons; alternatives considered: Hugo Book, Geekdoc, Doks).
4. **Search** → Pagefind (bundled with Hextra, static post-build index — no runtime search server, no third-party SaaS).
5. **Copy-as-Markdown implementation** → Hugo custom output format that renders `{{ .RawContent }}` to `index.md` next to every `index.html`. Lossless because the published `.md` is the source `.md`.
6. **CLI reference generation** → Cobra `doc.GenMarkdownTree` invoked by a new `shrine docs gen <dir>` command, run in CI before the Hugo build.
7. **Source location** → `docs/` in `main`. Build artifact deployed via the official `actions/deploy-pages` workflow.

No `NEEDS CLARIFICATION` markers remain.

## Phase 1 — Design & Contracts

Artifacts produced:

- **[data-model.md](./data-model.md)** — entities: Documentation Page, Navigation Tree, Copy-as-Markdown Output, Generated CLI Reference Page.
- **[contracts/cli-docs-gen.md](./contracts/cli-docs-gen.md)** — `shrine docs gen <dir>` command-line contract (flags, exit codes, output layout, idempotency).
- **[contracts/copy-as-md-url.md](./contracts/copy-as-md-url.md)** — public URL contract: every `/path/to/page/` MUST also resolve at `/path/to/page/index.md` with `Content-Type: text/markdown`.
- **[contracts/page-frontmatter.md](./contracts/page-frontmatter.md)** — required and optional Hugo front-matter for hand-authored content pages.
- **[quickstart.md](./quickstart.md)** — contributor onboarding: install Hugo via `go install`, `hugo serve` for local preview, regenerate CLI reference, add a new page, deploy.

**Agent context update**: `CLAUDE.md` is updated to point to this plan (`specs/013-docs-site/plan.md`) inside the `<!-- SPECKIT START -->` / `<!-- SPECKIT END -->` block.

### Constitution re-check (post-design)

Re-evaluating after Phase 1 design:

- **II. Kubectl-Style CLI**: confirmed — `shrine docs gen <dir>` follows verb-first; the contract document explicitly states it does not mutate Docker/network state and therefore does not need `--dry-run`.
- **V. Integration-Test Gate**: confirmed — `tests/integration/docs_gen_test.go` is in scope for Phase 2 and is listed as the gating test for this feature. The static site smoke (`hugo --strict`) is additional, not a substitute.
- **VII. Clean Code & Readability**: confirmed — the `internal/handler/docsgen/` design uses named helpers (`generateCommandTree`, `writeMarkdownForCommand`, `shouldSkipCommand`); no abstraction is introduced beyond what one set of usages needs.

No new violations introduced by Phase 1 design.

## Complexity Tracking

> Empty — no Constitution violations to justify.
