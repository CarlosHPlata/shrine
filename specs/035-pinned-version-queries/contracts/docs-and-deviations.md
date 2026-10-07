# Contract: documentation, progress entry, and design refinements

**Feature**: 035-pinned-version-queries

## `docs/content/reference/manifest-schema.md`, subsection Image pull policy

After the paragraph that begins **The pin lifecycle.**, a new paragraph titled in bold **Reading what is pinned.** stating: `shrine get deployed` (and `get applications`, `get resources`) shows a pinned artifact's version as the readable version it was resolved from, `@`, and the first twelve characters of the exact version, for example `latest@3f2a9c1b4d7e`, while manifest-owned artifacts show the reference their manifest named; `shrine describe app <name>` and `describe resource <name>` show the full exact version with the readable form and the date it was pinned on a `Pinned:` line, and the image the running container was started from on a `Running image:` line, so a `Pinned:` that differs from `Running image:` is a pin that has been recorded but not yet deployed; `shrine status` shows the running image in an IMAGE column; `get` needs no container runtime, and `describe` prints `Running image: unavailable` when the runtime cannot be reached; pins of artifacts that are not deployed are never shown. A short example block with one `get` row and the two `describe` lines.

## `cmd/describe.go` and `cmd/status.go` help text, then `make docs-gen-cli`

The `Long` texts of `describe app`, `describe resource`, `status`, `status application`, and `status resource` gain the sentences given in [query-output.md](query-output.md). Regenerated pages: `docs/content/cli/describe_app.md`, `describe_resource.md`, `status.md`, `status_application.md`, `status_resource.md`. The docs workflow's drift check requires the regenerated pages to be committed. No other page changes.

## `AGENTS.md`

- Section `### shrine status app/resource <name>`: add one sentence: the table's IMAGE column shows the image each container was started from (the exact version for a `Pinned` artifact, the tag reference otherwise) beside IMAGE ID.
- Section `### shrine describe app/resource <name>`: add one sentence: shows `Image:`, `Pull policy:`, and under `Pinned` a `Pinned:` line (full exact version, readable form, date) and a `Running image:` line read from Docker (`unavailable` when the daemon cannot be reached, the command still succeeds); a `Pinned:` that differs from `Running image:` is a recorded, not yet deployed, pin.
- The quick-reference block at the top: `shrine get deployed` line, if present, unchanged; no new command.

## `specs/progress.md`

One `- [x]` entry in the form of the 031 to 034 entries, naming the spec directory, issue #56, the epic and ticket T5, the behaviour (readable form in VERSION for `Pinned` records read from `pins.txt`, `Pinned:` and `Running image:` in `describe` with the unavailable rule, IMAGE in `status`, nothing shown without a record, the helper move into `manifest`, `ContainerInfo.Image`), the acceptance criteria by SC id, and the gates `TestPinnedVersionQueries` (loopback registry) plus the appended `TestDescribeDocker`, `TestDescribeNoDocker`, and `TestStatusDocker` scenarios (CI executes).

## `graphify update .`

Run after the code changes, per the repository rule; the regenerated `graphify-out/` is committed with the pull request as the previous tickets did.

## `design.md` refinements to record with the pull request

1. Section 3.5 and 4.9: the readable-form helpers (`ReadableVersion`, `ShortDigest`) live in `internal/manifest` beside `TagOf` and `IsDigestReference`, exported, with `DigestOf` added; `internal/ui` calls them (research R1).
2. Section 4.7: `describe` handlers take the container backend as a parameter and tolerate nil; a failed inspection or absent runtime renders `Running image: unavailable (<reason>)` and the command succeeds; a failed backend construction (malformed Docker environment) fails the command as it does for `status` and `delete` (research R3).
3. Section 4.7: the listing reads pins once through `ListAll()` rather than `Get` per row, and a `Pinned` record without a pin shows the recorded reference (research R2).
4. Section 4.7: the `Pinned:` line is `<pinned> (<readable>, <YYYY-MM-DD>)`, with the readable form exactly as the VERSION column prints it (research R5).
5. Section 4.7, `status`: the IMAGE column shortens a digest reference to `<repository>@<twelve hex>` per TD-11; a tag reference is printed as is (research R6).
6. Section 5, fixtures: the "pin differs from the running image" scenario edits `pins.txt` until T6's `bump` exists (research R7).
