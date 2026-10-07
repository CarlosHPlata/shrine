# Data model: Operator guide, managing image versions

This ticket changes documentation only, so there is no product data. The entities below are what the pages are made of. The checks in [quickstart.md](quickstart.md) verify them.

## Output block

A fenced `text` block in a page that shows what a command prints.

| Attribute | Rule |
|---|---|
| Command line | First line, `$ shrine <args>` exactly as the reader would type it. |
| Provenance | Captured (the real binary with no container runtime) or assembled (format strings in the code). Research R1 lists which commands are which, and R2 gives the source of every assembled line. |
| Variable parts | Exact versions, dates, container ids, config hashes, the manifest directory. Drawn only from research R5. |
| Trim | A line holding only `…`. It may replace lines; it never changes a kept line (research R3). |

## Placeholder set

Research R5 fixes one set of values for an exact version, a date, and an id per artifact. The guide, the troubleshooting entry, and the manifest reference use the same set. Readable forms derive from it, never the other way round: `latest@3f2a9c1b4d7e` is `ReadableVersion("traefik/whoami", "sha256:3f2a9c1b4d7e…")`.

## Example state

The state files the captures were taken from, written in the formats `internal/state/local` reads, under a scratch state directory:

- `shop/pins.txt`: `<kind> <name> <requested> <pinned> <pinned-at RFC 3339>`, one line for each pinned artifact.
- `shop/deployments.txt`: `<kind> <name> <container-id> <config-hash> <image> <policy>`, one line for each deployed artifact.

They exist only in the scratchpad and are never committed.

## Pages

| Page | Change | Spec |
|---|---|---|
| `docs/content/guides/image-versions.md` | new | FR-001 to FR-011, FR-014 |
| `docs/content/guides/_index.md` | one list line | FR-001 |
| `docs/content/troubleshooting/_index.md` | one section | FR-013 |
| `docs/content/reference/manifest-schema.md` | link and examples | FR-012, FR-015 |
| `cmd/get.go`, `cmd/describe.go`, `cmd/status.go` | `Long` help text | FR-015, FR-016 |
| `docs/content/cli/get_*.md`, `describe_*.md`, `status*.md` | regenerated | FR-016 |
| `specs/epics/pinned-image-versions/design.md` | one amendment line | FR-019 |
