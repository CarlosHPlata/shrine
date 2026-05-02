# Phase 1 Data Model: Documentation Site

This site is content-driven, not data-driven — there is no database, no runtime persistence, and no user accounts. The "entities" below describe the **content model** that `/speckit-plan` extracted from the spec, expressed in terms of Hugo's content structures so that Phase 2 task generation has a concrete target.

---

## Entity 1 — Documentation Page

A single readable unit on the docs site. Each page is one Markdown file under `docs/content/`.

**Fields** (Hugo front-matter + body):

| Field | Source | Required | Notes |
|-------|--------|----------|-------|
| `title` | front-matter | yes | Becomes the page H1 and the navigation label. |
| `description` | front-matter | recommended | Used for `<meta name="description">` and search snippets. |
| `weight` | front-matter | optional | Controls sibling order in the navigation tree. Lower = earlier. |
| `draft` | front-matter | optional | If `true`, page is excluded from production builds. |
| `cascade` | front-matter | optional | Hugo cascading defaults for child pages (used on section `_index.md` files). |
| `body` | Markdown after front-matter | yes | The page content. |
| `source path` | derived | yes | Recorded in build metadata; surfaced as the "Edit this page" link target (FR-007). |
| `version stamp` | derived from `git describe` at build time | yes | Surfaced in the page footer (FR-009). |
| `last modified` | derived from git log | optional | Displayed in the footer if present. |

**Validation rules** (enforced by `hugo --strict` and a CI step):

- Every page MUST have a non-empty `title`.
- Every internal link MUST resolve to another page or asset that exists at build time.
- Pages under `docs/content/cli/` MUST NOT be hand-edited (a CI check verifies the directory matches the output of `shrine docs gen` on the current binary).
- Draft pages MUST NOT be published to production builds.

**State transitions**:

```text
authored (Markdown file in branch)
   │  PR opened
   ▼
under review (visible only via local `hugo serve`)
   │  PR merged to main
   ▼
published (visible at the public URL within ~10 minutes — SC-004)
   │  file deleted in a later PR
   ▼
removed (404 page served; old URL is not redirected unless an explicit redirect is added)
```

---

## Entity 2 — Navigation Tree

The ordered, grouped list of Documentation Pages presented in the site's left/top navigation.

**Structure**:

```text
Home (content/_index.md)
├── Getting Started
│   ├── Install         (content/getting-started/install.md)
│   └── Quick Start     (content/getting-started/quick-start.md)
├── CLI Reference         (content/cli/_index.md — curated)
│   ├── apply           (content/cli/apply.md — generated)
│   ├── deploy          (content/cli/deploy.md — generated)
│   └── ...             (one per Cobra subcommand)
├── Guides
│   ├── Traefik gateway     (content/guides/traefik.md)
│   ├── Routing & aliases   (content/guides/routing-and-aliases.md)
│   └── TLS / HTTPS         (content/guides/tls.md)
├── Reference
│   └── Manifest schema     (content/reference/manifest-schema.md)
└── Troubleshooting        (content/troubleshooting/_index.md)
```

**Derivation**: produced by Hugo from the directory tree under `content/` plus `weight` ordering in front-matter. There is no separate "navigation config file" — the tree is the source of truth, satisfying FR-003 (≤ 3 clicks from Home to any page).

**Validation rules**:

- Every top-level section MUST have an `_index.md` with a `title`.
- The CLI Reference section's `_index.md` is hand-maintained (intro + table of contents); its individual subcommand pages are auto-generated and overwritten on each build.

---

## Entity 3 — Copy-as-Markdown Output

The Markdown document produced when a visitor triggers the per-page "copy as Markdown" action (FR-005, FR-006).

**Fields**:

| Field | Required | Notes |
|-------|----------|-------|
| H1 (page title) | yes | Pre-pended by the output template if the source body does not already start with `# <title>`. |
| Body content | yes | Verbatim from the source `.md` file: paragraphs, headings (H2+), lists, fenced code blocks (with language hints preserved), inline code, tables, blockquotes. |
| Site chrome | excluded | No header nav, no sidebar, no footer, no breadcrumbs, no "Edit this page" link, no inline JS, no HTML tags beyond what the source `.md` itself contains. |
| Front-matter | excluded | Hugo front-matter (YAML/TOML) is stripped before output. |

**Source**: `layouts/_default/single.md` (Hugo template) for hand-authored pages; the auto-generated CLI reference pages are already pure Markdown, so the same template applies unchanged.

**Validation rules**:

- For every published HTML page at `/<path>/`, the corresponding raw-Markdown URL `/<path>/index.md` MUST return a `200 OK` response with `Content-Type: text/markdown; charset=utf-8`.
- The first non-blank line of the response MUST be a Markdown H1 matching the page's `title` field.
- A round-trip check (render the response with a CommonMark renderer; compare to the source body) MUST be visually equivalent for ≥ 95% of pages at launch (SC-003), with all CLI reference pages passing (they are pure-Markdown by construction).

---

## Entity 4 — Generated CLI Reference Page

A specialization of Documentation Page that is produced by `shrine docs gen` rather than authored by hand.

**Fields**: same as Documentation Page, with these constraints:

| Field | Source | Notes |
|-------|--------|-------|
| `title` | Cobra command's `Use` (first token) | E.g. `apply`, `deploy`. |
| `description` | Cobra command's `Short` | One-line description used in front-matter and listing pages. |
| `body` | Generated via `cobra/doc.GenMarkdownTree` | Standard sections: Synopsis, Examples, Options, Inherited Options, See Also. |
| `weight` | Sorted alphabetically by command name | Override only if a curated ordering is required. |

**Validation rules**:

- The set of files in `docs/content/cli/` (excluding `_index.md`) MUST equal the set of non-hidden Cobra commands at build time.
- A CI step asserts this set equality and fails the build if they diverge.
- Generated files MUST include a banner comment indicating they are auto-generated and edits will be overwritten.

**State transitions**:

```text
Cobra command exists in cmd/
   │  CI runs `shrine docs gen ./docs/content/cli`
   ▼
Markdown file written/updated in docs/content/cli/<name>.md
   │  hugo build picks it up
   ▼
Page published at /cli/<name>/ with raw-MD companion at /cli/<name>/index.md
```
