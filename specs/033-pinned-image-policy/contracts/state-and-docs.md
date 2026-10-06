# Contract: recorded state and documentation

**Feature**: 033-pinned-image-policy

## `pins.txt`

Path: `<state-dir>/<team>/pins.txt`. Created on the first `Put` for the team (the team directory is created if absent, mode 0700, as for `deployments.txt`). Written atomically; rewritten whole on every `Put`, `Release`, and `ReleaseTeam` that changes it; an idempotent release writes nothing. Lines sorted by artifact name.

```text
<kind> <name> <requested> <pinned> <pinned-at>
```

| Field | Form |
|---|---|
| kind | `Application` or `Resource` |
| name | the artifact name |
| requested | the expanded tag reference the pin was resolved from (no `reg:` prefix) |
| pinned | `<repository>@sha256:<64 hex>` |
| pinned-at | RFC 3339 UTC, `2026-10-06T10:42:17Z` |

Reader: whitespace-split; `#` starts a comment; lines with fewer than five fields, a non-RFC-3339 date, or a `pinned` without `@sha256:` are skipped. Absent file means no pins. Nothing else under the state directory changes shape; `deployments.txt` records `Pinned` in its policy field for pinned artifacts through the existing writer.

## Dry run invariant

Two consecutive `shrine deploy --dry-run` runs, with or without pins on record, leave every file under the state directory byte-identical.

## `docs/content/reference/manifest-schema.md`

- Resource YAML block: `version: <string> # required unless imagePullPolicy is Pinned` and `imagePullPolicy: <Always|IfNotPresent|Pinned>`.
- Application YAML block: `imagePullPolicy: <Always|IfNotPresent|Pinned>`.
- Resource table: `spec.version` Required becomes `yes, unless `imagePullPolicy` is `Pinned``; `spec.imagePullPolicy` Description becomes `Image pull policy: `Always`, `IfNotPresent`, or `Pinned`. See [Image pull policy](#image-pull-policy).`
- Application table: the same `spec.imagePullPolicy` description.
- New subsection `### Image pull policy` after `### spec.env[]`, stating: the three values and the derived default; what `Always` and `IfNotPresent` do (unchanged); that under `Pinned` the manifest names only the repository (an Application image with no tag or `latest`; a Resource with no `version` or `latest`, and an image override following the Application rule), with the two error shapes quoted; the pin lifecycle: first deploy resolves the newest version and records the exact version, every later deploy runs it across redeploy, recreation, teardown, and a wiped image cache, and the pin is released only by `shrine delete application`, `shrine delete team`, or a deploy under `Always` or `IfNotPresent`; and that a pinned exact version the registry no longer serves fails the deploy before any change.

## `AGENTS.md`

- State Directory Layout: add `│   ├── pins.txt                 # image pins (<kind> <name> <requested> <pinned> <pinned-at>); survive teardown, released by delete and by a manifest-owned deploy` under `<team>/`.
- `### shrine delete application <name>`: add that it releases the application's image pin, and that `shrine delete team <name>` releases every pin the team held.
- The paragraph after the Deploy Pipeline diagram: one sentence stating that under `Pinned` the pre-pass reuses the recorded exact version or resolves the newest and records it, and that a manifest-owned resolution releases any pin.

## `specs/progress.md`

One `- [x]` entry in the form of the 031 and 032 entries, naming the spec directory, issue #54, the epic and ticket, the behaviour, the acceptance criteria by SC id, and the gate `TestPinnedImagePolicy`.

## `design.md` deviations to record with the pull request

1. The enum check on `spec.imagePullPolicy` did not exist; this ticket adds it (research R7).
2. `EffectivePullPolicy` parsed the tag from the last colon of the whole reference; it now uses `TagOf`, so an untagged image on a registry with a port derives `Always` as documented (research R6).
3. The integration fixture pushes `traefik/whoami:v1.10.1` and `v1.10.2` instead of two alpine tags (research R12).
4. `ResolvedImage` gains `Requested` and `PinnedAt` so the terminal can print the readable tag and the date (research R5).
5. A pin whose repository no longer matches the manifest is replaced on the next `Pinned` deploy (spec FR-011, research R3).
6. The "no longer served" message quotes the reference, says "by the registry", names the concrete manifest edit as the way out, and appends the pull cause; the "no registry digest" failure is also a backend `image.resolve` error event (research R8).
7. The manifest-sourced Resource message under `Pinned` reads `omit it or use "latest"`; the design's wording is kept for the configuration-sourced case in T4 (research R6).
8. The dry-run pinned line prints the full digest reference (research R9).
9. The integration helpers are `LocalRegistry.PushAs` returning the digest, `ImageIDOf`, `RemoveImage`, and `WritePinnedFixture`; there is no `AssertContainerImageDigest`, and pinned manifests are written at run time (research R12).
10. The planner helpers live in a new `internal/planner/policy.go` rather than in `plan.go` and `resolve.go` (research R6).
