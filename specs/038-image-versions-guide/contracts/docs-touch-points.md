# Contract: the other pages this ticket changes

Every change outside the new guide. Spec FR-012, FR-013, and FR-015 to FR-017 bind them.

## Troubleshooting entry (FR-013)

In `docs/content/troubleshooting/_index.md`, a new section placed before "See also":

- Heading: `## A deploy stops because a pinned version is no longer served`. The anchor is `#a-deploy-stops-because-a-pinned-version-is-no-longer-served`.
- The symptom, as the `Error:` line the deploy prints (assembled, research R2), with the cause the daemon appends.
- What it means: neither the registry nor the host still has the exact version the pin names, often because the registry deleted an untagged image. Nothing was created, changed, or removed.
- The fix: run the `shrine bump` command the message names. With `-v` it moves to a chosen version; without `-v` it moves to the newest. Then deploy.
- When the appended cause is an authentication or connection error rather than `manifest unknown`, the registry is unreachable with the configured credentials: fix the `registries` entry or the network first, and do not bump.
- A link to the guide.

## Manifest reference (FR-012, FR-015, research R7 V4)

In `docs/content/reference/manifest-schema.md`, section "Image pull policy":

- One sentence linking the guide, at the start of the section's lifecycle text.
- The bump example becomes `shrine bump resource shop-db -v 17` with the output line `Bumped shop/shop-db: 16@2d6f0b8e4a1c -> 17@9c1b4d7e3f2a; run "shrine deploy" to apply`. This line's format string is in `formatBumpResult`.
- The lifecycle prose's sample lines keep their shape but use the guide's artifact: `📌 Pinned shop.api at latest@3f2a9c1b4d7e` and `📌 Using pinned shop.api latest@3f2a9c1b4d7e (since 2026-10-01)`.
- The "Reading what is pinned" example becomes the captured `get resources` table (header, separator line, rows), followed by the captured `Pinned:` line and the assembled `Running image:` line for `shop-db` after the bump to 17 and before the deploy, so the two lines differ. Exact versions are written in full.
- No rule, value, or behaviour statement in the section changes.

## Command help (FR-015 to FR-017, research R7 V1 to V3)

The edits are made in the Cobra `Long` texts and regenerated with `make docs-gen-cli`. No `Use`, flag, or `RunE` changes.

- `cmd/get.go`, the `deployed`, `applications`, and `resources` subcommands each gain one paragraph:

  > The VERSION column shows the version each artifact was deployed with. For a Pinned artifact it is the readable version the pin was resolved from and the first twelve characters of its exact version, such as latest@3f2a9c1b4d7e. For any other artifact it is the image reference its manifest named. The table is read from state, so it needs no container runtime.

- `cmd/describe.go`, `app` and `resource`: the paragraph about the record becomes:

  > The record shows the image the manifest named and the effective pull policy. Under the Pinned policy it also shows the pin: the exact version, the readable version it was resolved from, and the date. Every record shows the image the running container was started from, or "unavailable" with the reason when the container runtime cannot be reached. A pin that differs from the running image has been recorded but not yet deployed.

- `cmd/status.go`, the root `status` command and the `application` and `resource` subcommands, wherever the IMAGE sentence appears:

  > The IMAGE column shows the image each container was started from: for a Pinned artifact, its repository and the first twelve characters of its exact version; for any other artifact, the image reference it was created from.

## Design document (FR-019, research R10)

`specs/epics/pinned-image-versions/design.md`, under "T8. Operator guide", after T8-01: one *Amended by T8 (spec 038)* sentence recording the capture deviation.

## Not changed

- The README configuration section: no finding (research R7 V6).
- `AGENTS.md`: the design names no `AGENTS.md` line for T8. The CLI reference lines already describe bump, delete, describe, and status.
- Code behaviour, tests, and fixtures.
