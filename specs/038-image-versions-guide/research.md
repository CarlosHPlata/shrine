# Research: Operator guide, managing image versions

Phase 0 of [plan.md](plan.md). Every decision below is settled; nothing is left marked NEEDS CLARIFICATION.

## R1. How the guide's output is produced

- **Decision**: Use no container runtime (owner's answer, spec Clarifications). Output the binary prints without a daemon is captured from real runs of a binary built from `main` at `b19862f`, against a scratch config directory, state directory, and manifest directory, with `DOCKER_HOST=tcp://127.0.0.1:1` so Docker is never contacted. Output that needs the daemon or a registry is assembled line by line from the format strings in the code, and each line is checked against an integration-suite assertion where one exists.
- **Captured** (real binary, no daemon): the validation errors under `Pinned`; the configuration-default refusal; every `deploy --dry-run`; `bump --dry-run`; the bump refusals for an invalid `-v`, a manifest-owned artifact, an unknown name, and a wrong `--team`; `generate application` and `generate resource` under a `Pinned` default; `get deployed`, `get applications`, `get resources` and `describe` (from seeded state files written in the documented formats); `describe` with the runtime unreachable; `delete resource` with and without `--dry-run`; and `delete team`.
- **Assembled** (format strings, no daemon): the image-resolution lines of a real deploy and of a bump; the container lines of a recreate; the `Running image:` line of `describe` when the runtime is reachable; the `status` table (printed through the handler's own row format); the failure of a bump to a missing tag; and the "no longer served" deploy failure.
- **Rationale**: The owner chose not to use this host's Docker daemon. Capturing everything the binary can print without a daemon keeps most blocks exact. The assembled blocks are limited to lines the code prints from fixed format strings, so the only freedom left is the placeholder values.
- **Alternatives considered**: real deploys on this host against a throwaway `registry:2` (declined by the owner); a capture script run on the CI runner (declined; it needs a push-and-wait loop and leaves a script in the repository).

## R2. Sources for every assembled line

| Output | Format source | Corroborating assertion |
|---|---|---|
| `🔎 Resolving image for <team>.<name> (<ref>)` | `internal/ui/terminal_logger.go` `image.resolve` started | `tests/integration/deploy_test.go:255`, `bump_test.go:47` |
| `    ✅ Pulled image <ref>` | `terminal_logger.go` `image.pull` via `handleStep` (prefix four spaces; the started spinner line is erased on finish) | `pinned_image_policy_test.go:188-197` (pull absent when the image is local) |
| `  📌 Pinned <team>.<name> at <readable>` | `renderImageResolved`, source `resolved` | `pinned_image_policy_test.go:171` |
| `  📌 Using pinned <team>.<name> <readable> (since <date>)` | source `pinned` | `pinned_image_policy_test.go:192`, `bump_test.go:67` |
| `  📌 Bumped <team>.<name> to <readable>` | source `repinned` | `bump_test.go:48` |
| `  🔎 Resolved <team>.<name> <ref>@<12 hex>` | source `manifest`, `exactVersion` | `deploy_test.go:256` |
| `Bumped <team>/<name>: <before> -> <after>; run "shrine deploy" to apply` | `handler/bump.go` `formatBumpResult` | `bump_test.go:49,75,116` |
| `Pinned <team>/<name> at <after>; run "shrine deploy" to apply` | `formatBumpResult`, no previous pin | `bump_test.go:158` |
| `📦 Deploying Resource: <name> (type: <type>)`, `  🌐 Ensuring network: shrine.<team>.private`, `  🏗️  Creating container: <team>.<name>`, `    🔄 Image changed for <c>, replacing container...`, `    ✨ Creating fresh container: <c>`, `    ✅ Container <c> is running` | `terminal_logger.go`; emitted by `engine.deployResource` and `dockercontainer.CreateContainer` (`removeStaleContainer`, `createFreshContainer`) | `bump_test.go:57-70` (container recreated on the deploy after a bump) |
| `Running image: <ref>` | `handler/deployments.go` `formatDeploymentDetail`; the value is `ContainerInspect(...).Config.Image`, which is the digest reference for a pinned container | `pinned_version_queries_test.go:112-157` |
| `status` row | `handler/status.go` `statusRowFormat` `%-25s %-15s %-10v %-12s %-40s %-19s`; IMAGE through `shortImageReference` | `status_test.go:83-89`, `pinned_version_queries_test.go:161-179` |
| `Error: application "<name>": pinned exact version "<ref>" for <team>/<name> is no longer served by the registry; run "shrine bump application <name>" to choose another version: <cause>` | `dockercontainer/docker_image.go` `notServedError`, wrapped by `engine.resolveImages` | `pinned_image_policy_test.go:351-353` |
| `Error: resource "<name>": pulling image "<ref>": <cause>` | `docker_image.go` `pullImage`, wrapped by `handler.bumpResolved` | `bump_test.go:87-89` |

The `<cause>` of a pull failure is the Docker daemon's own message. For a tag or digest that a registry does not have, the daemon prints `Error response from daemon: manifest for <reference> not found: manifest unknown: manifest unknown`; the guide uses that text.

## R3. What the guide trims

- **Decision**: Deploy output is shown from its first line through the image-resolution block, then `…` on a line of its own; container lines are shown only where a journey is about them (the recreate in J4). The usage text Cobra prints after every error is trimmed after the `Error:` line. The `❌ Error [...]` lines the terminal renderer prints while a pull indicator is active are trimmed, because in a terminal they share a line with the indicator and cannot be shown faithfully as text. Trailing spaces in table rows are dropped.
- **Rationale**: These are the lines this feature added and the lines a reader acts on. A trim never changes a line that remains (spec FR-003).
- **Alternatives considered**: showing whole deploys (dozens of lines unrelated to versions, each one more to assemble by hand).

## R4. The example

- **Decision**: One team, `shop`, with three artifacts:
  - `api`, an Application on `traefik/whoami` with `imagePullPolicy: Pinned`. It is small and public, and it is the image the integration suites use.
  - `shop-db`, a Resource of type `postgres` with `imagePullPolicy: Pinned` and no `version`. Its first version is chosen in advance with `bump -v 16` (PRD R-24), it is upgraded to `17` (J4), and it is rolled back by exact version (J5).
  - `cache`, a Resource of type `redis` with `version: "7.4"` and no policy, so it is manifest-owned under the derived rule. It is the contrast in the tables, the bump refusal, and the configuration default (J7).
- **Rationale**: Every journey needs both kinds of artifact and one manifest-owned contrast. A database is where a chosen version matters most, which is why the PRD's J4 is a Postgres upgrade. The guide adds one sentence saying that Shrine moves the image, not the data, so a major database upgrade still needs the database's own procedure.
- **Alternatives considered**: the integration suites' `127.0.0.1:<port>/shrine/whoami` (the address reads as a test fixture); minor PostgreSQL versions only (they lose the PRD's journey).

## R5. Placeholder values

Every value below is used the same way in the guide, the troubleshooting entry, and the manifest reference's examples (spec FR-011). The first twelve hex characters of the exact versions the manifest reference already used (`3f2a9c1b4d7e`, `9c1b4d7e3f2a`) are kept.

| Meaning | Exact version |
|---|---|
| `traefik/whoami`, newest on the first deploy | `sha256:3f2a9c1b4d7ebacb024fcc9cc3ba71306a98135816442f1b7d6817ed226ae2e2` |
| `traefik/whoami`, newest months later (J6) | `sha256:5e8c2b7a1f04e575d034d87b11b2b1464d85f44eb790ab617a8e7e248bef8aef` |
| `postgres:16` when chosen | `sha256:2d6f0b8e4a1c5e953124e2943a0ef520833a53ec00009a6c756237ef124ab460` |
| `postgres:17` (J4) | `sha256:9c1b4d7e3f2abafaeca130ff41ae79c7f98018fd87e177d20f90c7d5b32c63f1` |
| `redis:7.4` (manifest-owned, shown only as twelve hex) | `sha256:e1c7a93f5b20…` |

Dates: first pins on `2026-10-01`, the upgrade on `2026-10-20`, the rollback on `2026-10-21`, the newest-again bump on `2027-01-15`. Container ids `4c7e19a2b8d0` (`api`), `7a3e5c9b1d2f` (`cache`), `5d0a11c3b2e4` (`shop-db`). Manifest directory `/home/me/shop/manifests`.

## R6. Where pins live, and the host rebuild

- **Decision**: J2 states that pins are recorded in Shrine's state directory (`<state-dir>/<team>/pins.txt`, by default under `~/.local/share/shrine`), so a rebuild that keeps the state directory keeps every pin, and a rebuild that loses it starts every pinned artifact fresh. The guide tells the operator to keep or back up the state directory.
- **Rationale**: PRD J2 promises that every pinned artifact comes back on its exact version after a prune or a Docker reinstall. That holds only while the state directory survives. A guide that left this out would overpromise.

## R7. Vocabulary pass findings (T8-02)

Audited pages: `docs/content/reference/manifest-schema.md`, the README configuration section, and the generated pages for `bump`, `delete`, `describe`, `get`, `status`, and `generate`.

| # | Page | Finding | Fix |
|---|---|---|---|
| V1 | `get deployed`, `get applications`, `get resources` (help in `cmd/get.go`) | The VERSION column is undocumented: neither the readable form of a pinned artifact nor the reference of a manifest-owned one is described. | Add one paragraph to the three `Long` texts. Regenerate. |
| V2 | `describe app`, `describe resource` (help in `cmd/describe.go`) | Says the running image appears "under the Pinned policy … when the container runtime can be reached". In fact `Running image:` appears for every record and reads `unavailable (<reason>)` when the runtime cannot be reached. | Reword. Regenerate. |
| V3 | `status` (help in `cmd/status.go`) | "the exact version for a Pinned artifact, the tag reference otherwise": the cell holds the repository with a short exact version, and "tag reference" is not a PRD term. | Reword to the PRD's terms. Regenerate. |
| V4 | Manifest reference, image pull policy section | Its examples use the integration suite's names (`shrine-deploy-test/whoami-pinned`, tag `v2`). The table example has no separator line. The `Pinned:` and `Running image:` examples abbreviate the exact version with `…` and add a second space that the real output does not have. | Rewrite the examples on the guide's `shop` example, matching the output exactly. Link the guide. |
| V5 | Manifest reference | "the exact version (the registry digest)" already ties digest to exact version on first use. | None. |
| V6 | `bump`, `delete`, `generate` pages and the README section | Use repository, readable version, exact version, pin, and manifest-owned with their PRD meaning. | None. |

Help-text edits change no behaviour. The CLI drift check in `make docs-check` holds the pages and the Cobra tree to each other.

## R8. Link check

- **Decision**: There is no link checker in `make docs-check`; Hugo fails only on broken `ref` shortcodes. As spec 030 did, the quickstart builds the site and greps the built HTML for every anchor and page the new links target. Links follow the existing guides' form, site-absolute without the `/shrine` base (`/reference/manifest-schema/#image-pull-policy`), which the site rewrites.
- **Alternatives considered**: adding a link checker to the toolchain (a new tool, outside this ticket's scope).

## R9. Where the guide sits

- **Decision**: `docs/content/guides/image-versions.md`, weight 45 (after "Publish to localhost" at 40), listed last in `docs/content/guides/_index.md`. The troubleshooting entry is a new `##` section before "See also" in `docs/content/troubleshooting/_index.md`, with its own anchor.
- **Rationale**: This follows design T8-01 for the path and appends to the existing guide order.

## R10. Design deviation to record

- **Decision**: Append to the T8 list in `specs/epics/pinned-image-versions/design.md`: *Amended by T8 (spec 038):* output needing a container runtime is assembled from the code's format strings and checked against the integration suites, instead of captured from a real deploy; daemon-free output is captured. Also say so in the pull request, as delivery plan section 6 step 9 requires.
