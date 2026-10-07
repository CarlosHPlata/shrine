# Contract: the "Managing image versions" guide

The page `docs/content/guides/image-versions.md`, which spec FR-001 to FR-011 and FR-014 bind. This contract fixes its structure and where each output block comes from; the prose is free within it.

## Front matter

```yaml
---
title: "Managing image versions"
description: "Pin an image at its newest version, see which version runs, and move it on purpose: upgrade, roll back, take the newest."
weight: 45
---
```

## Sections, in order

| # | Heading | Journey | M7 question answered | Output blocks (C = captured, A = assembled) |
|---|---|---|---|---|
| 0 | What this guide covers | — | — | none |
| 1 | Concept: who owns the version | — | — | none; the three-policy table, the precedence, what a pin records, the vocabulary, the note that values are illustrative and how trims are marked |
| 2 | The example | — | — | the three manifests (`api`, `shop-db`, `cache`), the team, the `config.yml` with `specsDir` |
| 3 | Pin a service at its newest version | J1 | how to pin | C validation error for a fixed version; C `deploy --dry-run`; A `bump resource shop-db -v 16` before the first deploy; A first deploy; A a later redeploy that reuses the pins |
| 4 | Rebuild the host | J2 | what happens on prune | A deploy after `teardown` and `docker image prune -a`, pulling by exact version; prose on where pins live and on registry retention |
| 5 | See which version runs | J3 | how to see versions | C `get deployed`; C `get resources --team shop`; C+A `describe resource shop-db` (only the `Running image:` line assembled); A `status shop`; C `describe` with the runtime unreachable |
| 6 | Upgrade one artifact | J4 | how to upgrade | A `bump resource shop-db -v 17`; C+A `describe` showing a waiting pin; A the deploy that recreates only `shop-db`; A a bump to a missing tag; C an invalid `-v`; C `bump --dry-run`; C the refusal of a manifest-owned artifact |
| 7 | Roll back | J5 | how to roll back | A `bump resource shop-db -v sha256:…` with the earlier exact version; A the deploy |
| 8 | Take the newest again | J6 | — | C `bump app api --dry-run`; A `bump app api` with no `-v` |
| 9 | Make pinning the house rule | J7 | what the configuration default changes | the `config.yml` line; C the refusal naming `cache`; the two ways out; C `deploy --dry-run` after one; C `generate application` and the manifest it writes; prose on a manifest's own field and on removing the default |
| 10 | Retire an artifact | J8 | — | A deploy after teardown reusing the pins with their original dates; A the refusal while the container exists; C `delete resource --dry-run`; C `delete resource`; C `delete team`; the list of what releases a pin and what never does |
| 11 | When a pin cannot be honoured | — | — | none; two sentences and a link to the troubleshooting entry |
| 12 | What this does not do | — | — | none; the non-goals of FR-010 |
| 13 | Where to read more | — | — | none; links to the manifest reference, README configuration section, and the command pages of FR-014 |

## Rules every block follows

- Every block is fenced `text`, and its first line is the command, `$ shrine …`. The guide says once, in section 1, that the `$` is the prompt.
- A trim is a line holding only `…`. A trimmed block never alters a line that it keeps (research R3).
- Placeholder values are the ones in research R5, and nowhere else.
- No block asks the reader to copy a value from the guide. Where a command needs an exact version, the guide says where the reader's own value is printed (`describe`, or the previous version in a bump's output).

## Links out (spec FR-012 to FR-014)

- `/reference/manifest-schema/#image-pull-policy`
- `/troubleshooting/#a-deploy-stops-because-a-pinned-version-is-no-longer-served`
- `/cli/bump/`, `/cli/bump_application/`, `/cli/bump_resource/`, `/cli/delete_resource/`, `/cli/describe_resource/`, `/cli/get_deployed/`, `/cli/status/`, `/cli/generate_application/`
- The README configuration section, at `https://github.com/CarlosHPlata/shrine#configuration`, because no configuration page exists on the site (design 4.11).

## Links in

- `docs/content/guides/_index.md`: one list line, last, `[Managing image versions](image-versions/)` with a one-line description.
- `docs/content/reference/manifest-schema.md`, image pull policy section: a sentence linking the guide.
- `docs/content/troubleshooting/_index.md`, the new entry: a link to the guide.
