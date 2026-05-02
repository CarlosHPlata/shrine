# Contract: Per-page raw-Markdown URL

This contract defines the public URL surface that backs the "copy as Markdown" button on every documentation page. It is the integration boundary between the docs site and AI agents (or other tools) that consume Shrine documentation.

## URL shape

For every documentation page reachable at:

```text
https://<docs-host>/<path>/
```

a corresponding raw-Markdown representation MUST also be reachable at:

```text
https://<docs-host>/<path>/index.md
```

Examples:

| HTML URL | Raw-Markdown URL |
|----------|------------------|
| `/getting-started/install/` | `/getting-started/install/index.md` |
| `/cli/apply/` | `/cli/apply/index.md` |
| `/` (home) | `/index.md` |

## Response

```text
HTTP/1.1 200 OK
Content-Type: text/markdown; charset=utf-8
Content-Length: <bytes>
Cache-Control: public, max-age=300

# <Page title>

<page body as authored, verbatim>
```

### Required guarantees

1. **First non-blank line is an H1** matching the page's `title` front-matter field. If the source body did not start with `# <title>`, the output template prepends it.
2. **Body is verbatim source Markdown.** Hugo front-matter is stripped; everything else is byte-identical to the `.md` file in the repository.
3. **No site chrome.** No header navigation, no sidebar, no footer, no breadcrumbs, no "Edit this page" link, no inline JavaScript.
4. **Code blocks preserve language hints.** A source block fenced as <code>```yaml</code> remains <code>```yaml</code> in the response.
5. **Tables remain Markdown tables.** They are not converted to HTML.
6. **Embedded HTML in source is preserved as-is.** Authors who wrote raw HTML in their `.md` see exactly that HTML in the response (this is rare in our content).
7. **Encoding is UTF-8.** No BOM.

### Disallowed in the response body

- HTML wrapper elements (`<html>`, `<body>`, etc.).
- Hugo template directives (`{{ ... }}`).
- Front-matter delimiters (`---`, `+++`).
- Generated banner text added by `shrine docs gen` is **kept** in the CLI reference pages — this is intentional, since the banner is part of the source `.md` and is useful context for anyone (human or agent) reading the file.

## Caching

- `Cache-Control: public, max-age=300` (5 minutes) for both HTML and raw-MD responses.
- GitHub Pages serves over its global CDN; revalidation is automatic.
- Long-tail caching is not required: docs change infrequently, and the build pipeline produces a fresh artifact per merge to `main`.

## Discoverability

The button on each HTML page is the canonical UI for triggering this action. The URL pattern itself is also stable, public, and documented (in `getting-started/quick-start.md`'s "Using docs with AI agents" section), so agents can fetch raw Markdown directly without scraping HTML.

A future enhancement (out of v1 scope) might add a sitemap-style listing at `/all.md` for bulk ingest. This contract does **not** guarantee that endpoint exists.

## Test coverage

- **Site smoke (CI)**: after `hugo` build, a script walks `public/` and asserts that for every `index.html` there is a sibling `index.md`. Build fails otherwise.
- **Content fidelity (CI, sample)**: pick 3–5 representative pages (a CLI ref page, a guide with a code block + table, a short page with only a title), render their raw-MD response through a CommonMark renderer, and diff structurally against the source. Diff must be empty.
- **Manual / launch-time review**: spot check at least one page from each section in a real browser to confirm the clipboard write succeeds and that pasted output renders correctly in a downstream Markdown viewer.
