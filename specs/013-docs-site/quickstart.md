# Quickstart: contributing to the Shrine docs site

Audience: anyone who wants to add a page, fix a typo, or build the site locally to preview a change.

This page is itself a candidate piece of content for the live site — once feature 013-docs-site lands, expect a polished version under `docs/content/getting-started/contributing-to-docs.md`.

---

## Prerequisites

You only need a working Go toolchain. The site builds with **Hugo extended**, which we install from source via `go install` so contributors don't need a separate package manager.

```bash
go install github.com/gohugoio/hugo@latest
hugo version  # confirm "extended" appears in the output
```

> If your `hugo version` output does not say `extended`, reinstall with build tags:
> `CGO_ENABLED=1 go install -tags extended github.com/gohugoio/hugo@latest`

---

## Layout

All docs source lives at `docs/` in the repo root:

```text
docs/
├── hugo.yaml             # Site config
├── go.mod                # Hugo module file (separate from project go.mod)
├── content/              # Markdown pages, organized by section
├── layouts/              # Custom partials (copy-as-MD button, raw-MD output template)
└── static/               # Logos, screenshots
```

Generated CLI reference pages live in `docs/content/cli/` and **MUST NOT** be hand-edited — they are overwritten on every docs build by `shrine docs gen`.

---

## Build & preview locally

From the repo root:

```bash
cd docs/
hugo serve --buildDrafts --navigateToChanged
```

Open <http://localhost:1313/>. The page hot-reloads as you save Markdown files.

---

## Add a new page

1. Pick the right section under `docs/content/`. Most new content goes under `guides/` or `troubleshooting/`.
2. Create a new `.md` file with this minimal front-matter:

   ```yaml
   ---
   title: "My new guide"
   description: "Short one-liner."
   weight: 30
   ---

   # My new guide

   …content…
   ```

3. Cross-link to it from a parent `_index.md` if you want a curated entry beyond the auto-generated section listing.
4. Save; `hugo serve` will pick it up immediately. Verify the "Copy as Markdown" button on your page produces the source text you wrote.

See `specs/013-docs-site/contracts/page-frontmatter.md` for the full front-matter contract.

---

## Regenerate the CLI reference

Whenever you add, rename, or reflow a Cobra subcommand under `cmd/`, regenerate the reference pages:

```bash
go build -o ./shrine .
./shrine docs gen ./docs/content/cli --clean
```

The drift-check in CI fails if you forget — running this locally before pushing will save a round trip.

See `specs/013-docs-site/contracts/cli-docs-gen.md` for the full command contract.

---

## Verify "Copy as Markdown" works on your page

After `hugo serve`:

1. Open your page in the browser.
2. Click the "Copy as Markdown" button (top of the page in the Hextra header partial).
3. Paste the clipboard contents into any Markdown viewer (or feed it to your AI assistant).
4. Check that the structure is preserved: title as H1, code blocks intact with language hints, tables remain tables, no site chrome.

Equivalent without the button:

```bash
curl -s http://localhost:1313/path/to/your/page/index.md
```

The two outputs MUST be byte-identical.

---

## Open a PR

1. Create a feature branch from `main` (per the repo's existing workflow).
2. Commit your `.md` changes (and any layout/asset changes if you are working on the site itself).
3. Open a PR. CI will run:
   - `hugo --strict` (broken internal link → fail)
   - Front-matter lint
   - `shrine docs gen` drift check (CLI reference pages must match the current `cmd/` tree)
   - Per-page raw-Markdown smoke (every published HTML page must have a sibling `index.md`)
4. Once merged to `main`, the deploy job pushes the site to GitHub Pages within ~10 minutes (SC-004).

---

## Using the docs with an AI agent

For consumers (not contributors): every page exposes a one-click "Copy as Markdown" button. The button writes the page's clean Markdown source to your clipboard so you can paste it directly into a coding assistant.

If you want to fetch programmatically, append `index.md` to any page URL:

```text
https://<docs-host>/cli/apply/index.md
```

The response is `text/markdown`, UTF-8, with no site chrome — see `specs/013-docs-site/contracts/copy-as-md-url.md` for the full contract.

---

## Troubleshooting (contributor)

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| `hugo serve` exits with "no theme found" | Hugo modules not initialized | `cd docs && hugo mod tidy` |
| New page does not appear in the navigation | Missing `title` in front-matter, or `draft: true` | Set `title`; remove or set `draft: false` to publish |
| CLI reference drift check fails in CI | Forgot to regenerate after a Cobra change | Run `./shrine docs gen ./docs/content/cli --clean` and commit the result |
| Copy-as-Markdown returns 404 | The `markdown` output format is not enabled for this section | Check `hugo.yaml` `outputs` for the affected page kind |
