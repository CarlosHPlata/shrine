---
title: "Contributing to docs"
description: "Add a page, fix a typo, or build the Shrine docs site locally."
weight: 30
---

The Shrine docs site is built with [Hugo](https://gohugo.io/) and the [Hextra](https://imfing.github.io/hextra/) theme. All source content lives in this repository under `docs/content/`, and a GitHub Actions workflow publishes the site on every push to `main`.

## Prerequisites

Only a Go toolchain. Hugo is installable as a Go binary:

```bash
make docs-tools
```

This installs the version pinned in the project `Makefile` (`HUGO_VERSION`) into `$(go env GOPATH)/bin/hugo`. Run it once per machine.

## Layout

```text
docs/
├── hugo.yaml               # site config
├── content/                # Markdown source
│   ├── _index.md           # home
│   ├── getting-started/    # install, quick start, this page
│   ├── cli/                # auto-generated from `make docs-gen-cli`
│   ├── guides/             # task-focused walkthroughs
│   ├── reference/          # schema reference
│   └── troubleshooting/
├── layouts/                # custom partials, raw-Markdown templates, 404
├── static/                 # logo, favicon
└── tools/docsgen/          # separate Go module — CLI reference generator
```

The pages under `docs/content/cli/` are **regenerated on every docs build**; do not hand-edit them. To add or change a CLI command's reference, edit the Cobra command's `Use`/`Short`/`Long`/`Examples` in `cmd/<name>.go`.

## Build and preview locally

```bash
make docs-serve
```

Open `http://<host>:1313/shrine/` (note the `/shrine/` subpath — it mirrors the production GitHub Pages base URL). The server hot-reloads on file changes.

To produce a one-shot build:

```bash
make docs-build
ls docs/public/
```

## Add a new page

1. Pick a section under `docs/content/`. Most new content goes under `guides/` or `troubleshooting/`.
2. Create a new `.md` file with this minimal front-matter:

   ```yaml
   ---
   title: "My new guide"
   description: "Short one-liner (≤ 160 chars)."
   weight: 30
   ---

   # My new guide

   …content…
   ```

3. Cross-link from the parent `_index.md` if you want it featured beyond the auto-generated section listing.
4. `make docs-serve` will pick it up immediately. Verify the "Copy as Markdown" button on your page produces clean source.

The full front-matter contract lives at `specs/013-docs-site/contracts/page-frontmatter.md`. The PR-time linter (`scripts/lint-docs-frontmatter.sh`) enforces it.

## Regenerate the CLI reference

Whenever you add, rename, or refactor a Cobra subcommand under `cmd/`, regenerate the reference pages:

```bash
make docs-gen-cli
```

The CI drift-check fails any PR that ships out-of-sync reference pages — running this locally first saves a round trip. The full command contract is at `specs/013-docs-site/contracts/cli-docs-gen.md`.

## Verify the "Copy as Markdown" button

After `make docs-serve`:

1. Open your page in the browser.
2. Click the "Copy as Markdown" button at the top of the content area.
3. Paste into any Markdown viewer (or feed it to your AI assistant).
4. Confirm the output preserves headings, code fences with language hints, and tables.

Equivalent without the button — every page exposes a sibling raw-Markdown URL:

```bash
curl -s http://<host>:1313/shrine/cli/apply/index.md
```

The response is `text/markdown`, UTF-8, with no site chrome. The full URL contract lives at `specs/013-docs-site/contracts/copy-as-md-url.md`.

## Open a PR

1. Branch from `main`.
2. Commit your `.md` (and any layout/asset changes).
3. Open the PR. CI will run:
   - **Front-matter lint** — broken/missing front-matter fails fast.
   - **CLI drift check** — `docs/content/cli/*.md` must match the current Cobra tree.
   - **Hugo build** — broken `{{</* ref */>}}` shortcodes fail the build.
   - **Markdown companion check** — every published HTML page must have an `index.md` sibling.
   - **Markdown shape check** — every `index.md` starts with an H1 and contains no site chrome.
4. After merge to `main`, the site updates within ~10 minutes with no further action.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `make docs-serve` says "no theme found" | `cd docs && hugo mod tidy` |
| New page does not appear in navigation | Check `title` is non-empty and `draft` is not `true` |
| CI drift check fails | Run `make docs-gen-cli` locally and commit the result |
| Copy-as-Markdown returns 404 | The raw-MD output format isn't enabled for that page kind — check `outputs:` in `hugo.yaml` |
