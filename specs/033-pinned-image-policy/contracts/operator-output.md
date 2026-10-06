# Contract: operator-visible output and errors

**Feature**: 033-pinned-image-policy

## Validation errors (planning, before any change)

Printed under `Validation errors:` on stderr with the manifest set's other errors, exit non-zero:

```text
application "whoami-fixed": spec.image "127.0.0.1:5000/shrine/whoami:v1.10.2" names a fixed version but the image pull policy is Pinned; use "127.0.0.1:5000/shrine/whoami" or "127.0.0.1:5000/shrine/whoami:latest"
resource "cache-fixed": spec.version "16" names a fixed version but the image pull policy is Pinned; omit it or use "latest"
resource "cache-override": spec.image "postgres:16" names a fixed version but the image pull policy is Pinned; use "postgres" or "postgres:latest"
resource "cache-noversion": spec.version is required
```

Parse-time (`manifest.Validate`), unchanged mechanism, new message:

```text
spec.imagePullPolicy must be one of Always, IfNotPresent, Pinned
```

## Deploy output (terminal observer)

Per artifact, in the resolution block T2 introduced, before the first deploy header. The started line is unchanged.

| Decision | Lines |
|---|---|
| pinned on this deploy | `🔎 Resolving image for <team>.<name> (<ref>)` · `    📥 Pulling image <ref>...` / `    ✅ Pulled image <ref>` · `  📌 Pinned <team>.<name> at <readable>@<12 hex>` |
| reused, image present | `🔎 Resolving image for <team>.<name> (<ref>)` · `  📌 Using pinned <team>.<name> <readable>@<12 hex> (since <YYYY-MM-DD>)` |
| reused, image absent | as above with `    📥 Pulling image <repo>@sha256:…...` / `    ✅ Pulled image <repo>@sha256:…` between the two lines |
| manifest-owned | T2's lines, unchanged |

`<ref>` on the started line is the expanded tag reference. `<readable>` is the tag of the reference the pin was resolved from, `latest` when it had none; when that reference is itself a digest reference (not produced by this ticket), `<readable>@<12 hex>` collapses to `<12 hex>` alone. `<12 hex>` is the digest without `sha256:`, truncated to twelve characters.

Example, first deploy then redeploy:

```text
🔎 Resolving image for shrine-deploy-test.whoami-pinned (127.0.0.1:5000/shrine/whoami:latest)
    ✅ Pulled image 127.0.0.1:5000/shrine/whoami:latest
  📌 Pinned shrine-deploy-test.whoami-pinned at latest@3f2a9c1b4d7e
🔎 Resolving image for shrine-deploy-test.cache-pinned (127.0.0.1:5000/shrine/whoami)
    ✅ Pulled image 127.0.0.1:5000/shrine/whoami
  📌 Pinned shrine-deploy-test.cache-pinned at latest@3f2a9c1b4d7e
📦 Deploying Resource: cache-pinned (type: cache)
…
```

```text
🔎 Resolving image for shrine-deploy-test.whoami-pinned (127.0.0.1:5000/shrine/whoami:latest)
  📌 Using pinned shrine-deploy-test.whoami-pinned latest@3f2a9c1b4d7e (since 2026-10-06)
```

## Failure: pin no longer served

```text
🔎 Resolving image for shrine-deploy-test.whoami-pinned (127.0.0.1:5000/shrine/whoami:latest)
    📥 Pulling image 127.0.0.1:5000/shrine/whoami@sha256:0000…...
  ❌ Error [image.pull]: pulling image "127.0.0.1:5000/shrine/whoami@sha256:0000…": Error response from daemon: manifest for … not found
  ❌ Error [image.resolve]: pinned exact version "127.0.0.1:5000/shrine/whoami@sha256:0000…" for shrine-deploy-test/whoami-pinned is no longer served by the registry; deploy the application under Always or IfNotPresent to release the pin, then return to Pinned: pulling image "…": …
  ❌ Error [image.resolve]: application "whoami-pinned": pinned exact version "…" for shrine-deploy-test/whoami-pinned is no longer served by the registry; …
Error: application "whoami-pinned": pinned exact version "…" … is no longer served by the registry; …
```

Nothing else is printed: no network, container, or routing line follows. The last line is the command's error on stderr.

## Failure: no registry digest

```text
  ❌ Error [image.resolve]: image "<ref>" carries no registry digest and cannot be pinned
  ❌ Error [image.resolve]: application "<name>": image "<ref>" carries no registry digest and cannot be pinned
```

## Dry run

```text
[DOCKER] ImageResolve: name=shrine-deploy-test.whoami-pinned image=127.0.0.1:5000/shrine/whoami policy=Pinned -> would resolve newest and pin
[DOCKER] ImageResolve: name=shrine-deploy-test.cache-pinned image=127.0.0.1:5000/shrine/whoami policy=Pinned -> pinned 127.0.0.1:5000/shrine/whoami@sha256:3f2a9c1b… (latest, 2026-10-06)
[DOCKER] ImageResolve: name=shrine-deploy-test.web image=nginx:1.27 policy=IfNotPresent -> manifest-owned
[DOCKER] CreatePlatformNetwork name=shrine.platform
```

The pinned line prints the full `pin.Pinned` reference, not the twelve-character form, because the dry run is where an operator copies the exact version from. Repeated dry runs change no file under the state directory.

## Delete

`shrine delete application <name>`:

```text
Released host port 30000 for team/name.          (when held, unchanged)
Released image pin for team/name.                (NEW, when on record)
Removed deployment record for team/name.         (unchanged)
Nothing to delete for application "name" in team "team".   (unchanged; now also requires no pin)
```

`--dry-run` adds `[dry-run] would release image pin <pin.Pinned> for team/name` between the host-port and record lines.

`shrine delete team <name>`:

```text
Released 2 host port(s) for team "name".   (unchanged, when > 0)
Released 2 image pin(s) for team "name".   (NEW, when > 0)
Deleted team "name" from state.
```

## Log file (`shrine.log`)

Generic lines; the new fields appear in alphabetical order as the file logger prints them:

```text
2026-10-06T10:42:19Z [finished] image.resolve digest=sha256:… name=whoami-pinned ref=127.0.0.1:5000/shrine/whoami@sha256:… requested=127.0.0.1:5000/shrine/whoami:latest source=resolved team=shrine-deploy-test
2026-10-06T11:03:02Z [finished] image.resolve digest=sha256:… name=whoami-pinned pinned_at=2026-10-06 ref=… requested=… source=pinned team=shrine-deploy-test
```

## Unchanged

Every existing line keeps its text and position. Manifests that do not name `Pinned` produce exactly T2's output.
