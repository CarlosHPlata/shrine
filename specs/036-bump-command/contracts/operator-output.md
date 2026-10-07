# Contract: operator-visible output of `shrine bump`

**Feature**: 036-bump-command

Every string below is exact unless marked as a shape. `<team>.<name>` is the dotted artifact form the terminal observer uses; `<team>/<name>` is the slashed form the handlers print. `<readable>` is `manifest.ReadableVersion` (`v2@9c1b4d7e3f2a`, `latest@3f2a9c1b4d7e`, or `9c1b4d7e3f2a` alone for a digest request).

## Command surface

```text
shrine bump application <name> [-v <version>] [-t <team>] [-p <dir>] [--dry-run]   (alias: app)
shrine bump resource    <name> [-v <version>] [-t <team>] [-p <dir>] [--dry-run]   (alias: res)
```

Flags are persistent on `bump`: `-v, --version string` ("Readable version (tag) or exact version (sha256:…) to pin; newest when omitted"), `-t, --team string` ("Team owning the artifact (verified when given)"), `--dry-run` ("Print what would be resolved and pinned without changing state"), `-p, --path string` ("Directory containing manifest files (overrides specsDir in config.yml)"). Exactly one positional argument; Cobra's `accepts 1 arg(s)` error otherwise.

`Short` for `bump`: `Move a pinned artifact to another version`. `Short` for the subcommands: `Pin an application to a chosen or the newest version` / `Pin a resource to a chosen or the newest version`.

`Long` (shape, both subcommands, kind word substituted):

```text
Resolve the chosen version of a Pinned application in the registry and record
it as the application's new pin. Nothing is started, stopped, or recreated:
the next "shrine deploy" applies the pin.

-v names the version to pin: a readable version (a tag such as 17 or v1.4.0)
or an exact version (sha256:…). The repository always comes from the manifest,
so a bump can never point an application at a different image. Without -v the
newest version of the manifest's image is resolved and pinned. Rolling back is
a bump to an earlier version.

Only applications whose effective image pull policy is Pinned can be bumped;
under Always or IfNotPresent the version is manifest-owned and the bump
refuses. The application need not be deployed: a pin recorded before the first
deploy is the version that deploy runs. The manifest is looked up in the
specs directory (or --path); --team verifies the manifest's owner.

The output states the previous and the new version. --dry-run prints what
would be resolved and pinned and writes nothing.
```

## Success, previous pin on record (stdout)

```text
🔎 Resolving image for shrine-deploy-test.whoami-pinned (127.0.0.1:5000/shrine/whoami:v2)
    📥 Pulling image 127.0.0.1:5000/shrine/whoami:v2...     (indicator; ends as ✅ Pulled image …)
  📌 Bumped shrine-deploy-test.whoami-pinned to v2@9c1b4d7e3f2a
Bumped shrine-deploy-test/whoami-pinned: latest@3f2a9c1b4d7e -> v2@9c1b4d7e3f2a; run "shrine deploy" to apply
```

The first three lines are the observer's (the `repinned` arm is the new one); the last is the handler's. Exit 0.

## Success, no previous pin (stdout)

```text
🔎 Resolving image for shrine-deploy-test.whoami-pinned (127.0.0.1:5000/shrine/whoami:v2)
    📥 Pulling image 127.0.0.1:5000/shrine/whoami:v2...
  📌 Bumped shrine-deploy-test.whoami-pinned to v2@9c1b4d7e3f2a
Pinned shrine-deploy-test/whoami-pinned at v2@9c1b4d7e3f2a; run "shrine deploy" to apply
```

## Success, bump to an exact version

Same shape; the resolving line shows `(…/whoami@sha256:<64 hex>)` and both readable forms show the twelve hex characters alone: `… to 9c1b4d7e3f2a` and `-> 9c1b4d7e3f2a`.

## Success, no `-v` (newest)

Same shape; the resolving line shows the manifest reference (`(127.0.0.1:5000/shrine/whoami)`), the readable forms read `latest@<12 hex>`.

## Dry run (stdout, exit 0)

```text
[dry-run] would resolve 127.0.0.1:5000/shrine/whoami:v2 and pin shrine-deploy-test/whoami-pinned
```

No other line: no resolving line, no pull, no `📌`. Without `-v`, the reference printed is the manifest's own. The refusals below apply unchanged under `--dry-run`.

## Refusals (stderr via the returned error, exit 1; nothing recorded, no registry contact)

| Case | Text |
|---|---|
| invalid `-v` | `invalid version "not valid!": use a tag (a letter, digit, or underscore, then up to 127 letters, digits, underscores, dots, or dashes) or an exact version "sha256:<64 hex>"` |
| unknown artifact | `application "nope": no manifest found in /abs/path/to/specs` |
| wrong team | `application "whoami-pinned" not found in team "other" (its manifest in /abs/path/to/specs is owned by "shrine-deploy-test")` |
| manifest-owned | `application "whoami-pinned": its version is manifest-owned (imagePullPolicy IfNotPresent); edit the manifest to change it` |
| set fails validation | `Validation errors:` then one line per error on stderr, then `Error: Spec validation errors` (deploy's exact behaviour) |

The kind word is `application` or `resource`. The directory is the resolved one (`--path`, else `specsDir`).

## Resolution failure (exit 1; pin untouched; container untouched)

```text
🔎 Resolving image for shrine-deploy-test.whoami-pinned (127.0.0.1:5000/shrine/whoami:v9)
    📥 Pulling image 127.0.0.1:5000/shrine/whoami:v9...
  ❌ Error [image.pull]: pulling image "127.0.0.1:5000/shrine/whoami:v9": …manifest unknown…
Error: application "whoami-pinned": pulling image "127.0.0.1:5000/shrine/whoami:v9": …
```

The handler wraps the backend's error as `<kind> "<name>": %w`, the engine's pre-pass shape. A no-digest image fails as `application "x": image "…" carries no registry digest and cannot be pinned` after the backend's `❌ Error [image.resolve]` line.

## `deploy` when a pinned exact version has vanished (changed text, T6-06)

```text
  ❌ Error [image.resolve]: pinned exact version "127.0.0.1:5000/shrine/whoami@sha256:0000…" for shrine-deploy-test/whoami-pinned is no longer served by the registry; run "shrine bump application whoami-pinned" to choose another version: pulling image "…": …
Error: application "whoami-pinned": pinned exact version "…" for shrine-deploy-test/whoami-pinned is no longer served by the registry; run "shrine bump application whoami-pinned" to choose another version: …
```

Only the clause after the first `;` changes from T3. Zero containers and networks are created, as before.

## `pins.txt` after a bump (state, not output; design section 3.4)

```text
Application whoami-pinned 127.0.0.1:5000/shrine/whoami:v2 127.0.0.1:5000/shrine/whoami@sha256:9c1b… 2026-10-07T10:42:17Z
Resource    cache-pinned  127.0.0.1:5000/shrine/whoami    127.0.0.1:5000/shrine/whoami@sha256:3f2a… 2026-10-07T10:40:03Z
```

`describe` (T5) then shows the new pin on `Pinned:` and the old reference on `Running image:` until the next deploy.
