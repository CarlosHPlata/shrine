# Phase 0 Research: Official Shrine CLI Documentation Site

This document records the design decisions for the docs site, the alternatives considered, and the rationale for each choice. It resolves every open question raised by [spec.md](./spec.md) so Phase 1 can begin with no `NEEDS CLARIFICATION` markers.

---

## Decision 1 — Hosting platform

**Decision**: GitHub Pages.

**Rationale**:
- The spec requires a per-page "copy as Markdown" action (FR-005, FR-006). That requires custom HTML/JS, which GitHub Pages permits but GitHub Wiki does not.
- GitHub Pages allows a polished landing page, mobile-friendly theming, custom navigation, and a build pipeline that can run `shrine docs gen` before site assembly.
- Both options satisfy "deployed on GitHub" and "easy to modify".

**Alternatives considered**:
- **GitHub Wiki** — zero build, in-browser editing, but rendered pages do not allow custom JavaScript or custom layouts. The copy-as-Markdown feature cannot be cleanly implemented. The Home page cannot be styled. Rejected.
- **Read the Docs** — would introduce a third-party hosting dependency; outside of "deployed on GitHub". Rejected.
- **Self-hosted (e.g., on a VM)** — adds operational burden for no user benefit. Rejected.

---

## Decision 2 — Static site generator

**Decision**: Hugo (extended edition).

**Rationale**:
- Shrine is a Go CLI; Hugo is also written in Go. Contributors only need a Go toolchain to author, build, and preview docs locally (`go install github.com/gohugoio/hugo@latest`).
- The user explicitly stated this preference: keep the toolchain to one language so that doc contributors do not need to install Python, Node, or Ruby. (Recorded as a feedback memory.)
- Hugo's "custom output formats" feature lets us publish each page as both `.html` and the verbatim `.md` source — the cleanest possible implementation of the copy-as-Markdown feature.
- Hugo modules are themselves Go modules, so theme management uses tooling Shrine contributors already understand.

**Alternatives considered**:
- **MkDocs Material** — strongest "default" choice for CLI/tool docs (FastAPI, Pydantic, Trivy). Excellent search, mobile, theming. Rejected because it requires a Python toolchain.
- **Docusaurus** — React-based, very polished, great for product docs with versioning. Rejected: requires Node.js, heavier than v1 needs, versioning is out of scope.
- **VitePress** — Vue-based, modern, fast. Rejected: requires Node.js.
- **Jekyll** — GitHub Pages' default, no extra build needed. Rejected: requires Ruby; theming for the copy-as-MD partial is awkward; less actively maintained than Hugo.

---

## Decision 3 — Hugo theme

**Decision**: [Hextra](https://github.com/imfing/hextra).

**Rationale**:
- Modern look out of the box (Tailwind-based, dark mode, mobile-friendly).
- Built-in Pagefind search — no third-party search SaaS, no separate JS framework, satisfies FR-008 with no extra configuration.
- Built-in code-block "copy" buttons; we extend the same pattern for the page-level "copy as Markdown" button via a `partials/copy-as-markdown.html` partial that overrides Hextra's default page header.
- Distributed as a Hugo module → installed with `hugo mod get github.com/imfing/hextra`. No vendored theme directory to maintain.
- Actively maintained, with sites in production for similar tooling docs.

**Alternatives considered**:
- **Hugo Book** — minimal, classic, well-suited for CLI docs. Rejected: less batteries-included (search, dark mode require extra wiring); landing page is plain.
- **Geekdoc** — technical-doc oriented, well-themed for reference docs. Rejected: smaller community than Hextra, less modern look-and-feel for the home page.
- **Doks** — feature-rich. Rejected: depends on Hyas (a wrapper toolchain) and pulls in npm assets, contradicting the single-toolchain decision.

---

## Decision 4 — Site search

**Decision**: Pagefind (bundled by Hextra).

**Rationale**:
- Builds a static search index at build time; no runtime server, no third-party SaaS, no per-search API cost.
- Index is generated as part of `hugo` build, so deploys are atomic.
- Already integrated by the Hextra theme — zero extra configuration to satisfy FR-008.

**Alternatives considered**:
- **Algolia DocSearch** — high quality, but introduces a third-party dependency and an external service contract. Rejected for v1.
- **Lunr.js** — purely client-side, but Hextra/Pagefind already covers this niche better, and we avoid adopting two search stacks. Rejected.
- **No search** — fails FR-008. Rejected.

---

## Decision 5 — Copy-as-Markdown implementation strategy

**Decision**: Hugo custom output format + `_default/single.md` template that renders `{{ .RawContent }}`.

**Behavior**:
- Each content page produces *two* outputs: `/<path>/index.html` and `/<path>/index.md`.
- The `.md` artifact is the **verbatim source `.md`** (the same bytes a contributor wrote, minus Hugo front-matter), with the page title prepended as an H1 if it is not already present.
- The "copy as Markdown" button on each HTML page issues `fetch('./index.md')`, then writes the response body to the clipboard via the standard Clipboard API.

**Rationale**:
- Lossless. The Markdown an AI agent receives is identical to the source — code blocks keep their language fences, tables stay as Markdown tables, no HTML noise sneaks in (FR-006).
- Free of JavaScript transformation steps that could subtly mangle content.
- Trivial to test: a CI smoke step verifies that `index.md` exists for every page Hugo generated.
- Works equally well for the auto-generated CLI reference pages, since they are also `.md` files at build time.

**Alternatives considered**:
- **Embed source in `<script type="text/markdown">` tag** — same end state, but inflates HTML payload and complicates incremental rebuilds. Rejected.
- **HTML → Markdown conversion in JavaScript (turndown.js)** — lossy: loses code-block language hints and emoji shortcodes; introduces a runtime dependency. Rejected.
- **Generate a single combined "all docs as MD" archive** — useful as a future enhancement, but the spec asks for per-page copy. Out of scope for v1.

---

## Decision 6 — CLI reference generation

**Decision**: Add `shrine docs gen <output-dir>` (Cobra) backed by `cobra/doc.GenMarkdownTree`. CI invokes it before `hugo`.

**Rationale**:
- Cobra ships first-party Markdown generation. Output is one file per command, with sections for usage, flags, and subcommands — the right shape for a reference page.
- Running it as part of CI means the reference cannot drift: a renamed flag or removed subcommand is reflected on the next docs build.
- Locally, contributors run `shrine docs gen ./docs/content/cli` to regenerate before previewing.
- Satisfies SC-002 (100% subcommand coverage) by construction — no human curation step that can miss a command.
- The new command is *thin*: `cmd/docs.go` parses flags, `internal/handler/docsgen/` does the work (per Constitution Principle II).

**Alternatives considered**:
- **Hand-write the CLI reference** — fastest to start, guaranteed to drift. Rejected.
- **Custom `--help` parser** — reinvents Cobra's built-in support. Rejected.
- **Generate at Hugo build time via a Hugo data source / shell hook** — couples the docs build to the Go toolchain in a less explicit way; also works only if Hugo's runner has the Shrine binary on PATH. The current proposal already arranges that, but invoking a dedicated Cobra subcommand is cleaner and explicitly supported by the Constitution's "thin dispatcher" pattern.

---

## Decision 7 — Source location, branch, deploy pipeline

**Decision**:
- All docs source lives at `docs/` in `main`.
- A GitHub Actions workflow at `.github/workflows/docs.yml` runs on push to `main` (and on PR for preview/lint), and:
  1. Sets up Go.
  2. Builds the Shrine binary.
  3. Runs `shrine docs gen ./docs/content/cli`.
  4. Installs Hugo extended (via `go install` to keep the toolchain Go-only, or via `peaceiris/actions-hugo` if a pinned binary is preferred — final choice in Phase 2).
  5. Builds the site (`hugo --gc --minify --strict`).
  6. Uploads the `public/` artifact and deploys to Pages via `actions/deploy-pages`.

**Rationale**:
- Single canonical source of truth in `main`; no `gh-pages` branch to babysit (the official deploy-pages action manages the artifact).
- Build-time CLI ref generation guarantees the published reference matches the binary in `main`.
- `--strict` flag in Hugo treats broken internal links as errors — satisfies the FR-014 / edge-case "broken internal link" requirement.

**Alternatives considered**:
- **Maintain a `gh-pages` branch manually** — older pattern, more error-prone, no benefit. Rejected.
- **Build docs from a release tag** — would require a versioning UX that v1 explicitly excludes. Rejected for now.
- **Deploy from a separate docs repo** — fragments the workflow; contradicts FR-011 (docs flow through the same review process). Rejected.

---

## Decision 8 — Versioning, localization, analytics, comments (v1 scope)

**Decision**: Single "latest" line of docs (mirrors `main`); English only; no analytics; no comment system; report-an-issue link goes to the existing GitHub issue tracker pre-filled with the page path.

**Rationale**:
- Matches the spec's Assumptions section explicitly.
- Minimizes v1 scope without foreclosing future expansion (Hugo + Hextra both support versioned docs and i18n; we just don't enable them yet).

**Alternatives considered**:
- **Multi-version docs from day one** — out of scope per spec.
- **Disqus / Giscus comments** — adds runtime dependency and content-moderation surface. Rejected for v1.
- **Plausible / GoatCounter analytics** — privacy-friendly options exist; can be added later without spec changes. Deferred.

---

## Open items deferred to Phase 2 (`/speckit-tasks`)

These are planning details, not unresolved decisions:

- Exact Hugo version pin in CI (`go install` `@v0.135.0` vs floating).
- Naming and ordering of the curated guide pages.
- Whether the home page links to the CLI reference index or to `getting-started/quick-start.md` first.
- Initial content for `troubleshooting/_index.md` — list of known issues at launch.

These are documented in `tasks.md` (Phase 2 output) but do not block design.
