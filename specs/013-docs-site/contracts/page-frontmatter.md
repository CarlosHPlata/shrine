# Contract: Hugo front-matter for content pages

This contract defines the front-matter every hand-authored documentation page must (or may) carry. It is enforced by `hugo --strict` and a small CI lint step.

The format used in this site is **YAML front-matter** (delimited by `---` lines), chosen for consistency with the rest of the Shrine repository (Kubernetes-style YAML throughout).

## Required fields

| Field | Type | Notes |
|-------|------|-------|
| `title` | string | Becomes the page H1, the navigation label, and the first line of the raw-Markdown response. Must be non-empty. Avoid Markdown formatting in this field. |

## Recommended fields

| Field | Type | Notes |
|-------|------|-------|
| `description` | string | One-line summary. Used for `<meta name="description">`, search snippets, and parent-section listings. Aim for ≤ 160 characters. |
| `weight` | int | Sibling order within the parent section. Lower numbers appear first. Use multiples of 10 (10, 20, 30) so insertions don't require renumbering. |

## Optional fields

| Field | Type | Notes |
|-------|------|-------|
| `draft` | bool | If `true`, page is excluded from production builds (`hugo --buildDrafts` to preview). |
| `aliases` | list of string | Old URLs that should redirect to this page. Use when renaming/moving a page to avoid breaking external links. |
| `cascade` | object | Hugo cascading defaults for child pages. Used on section `_index.md` files only. |
| `toc` | bool | Theme-specific (Hextra). Defaults to `true`; set `false` to suppress the per-page table of contents. |

## Disallowed fields

- `date` / `lastmod` — derived from git history at build time; do not hand-set.
- `version` — the version stamp is a site-wide footer derived from `git describe`, not per-page.
- Custom fields not consumed by either Hugo or Hextra — they pollute the front-matter and confuse contributors.

## Examples

### Standard guide page

```yaml
---
title: "Routing & aliases"
description: "Configure Traefik routes and per-app aliases for your Shrine deployments."
weight: 20
---
```

### Section index page

```yaml
---
title: "Guides"
description: "How-to articles for common Shrine operations."
weight: 30
cascade:
  type: docs
---
```

### Auto-generated CLI reference page (written by `shrine docs gen`)

```yaml
---
title: "apply"
description: "Apply one or more manifests to the cluster"
weight: 10
---
```

## Linting

A pre-commit / CI lint step (`scripts/lint-docs-frontmatter.sh` — created in Phase 2) checks every `.md` file under `docs/content/`:

- `title` is present and non-empty.
- `description` is ≤ 160 characters when present.
- No disallowed fields appear.
- YAML parses cleanly.

Failure short-circuits the docs build.
