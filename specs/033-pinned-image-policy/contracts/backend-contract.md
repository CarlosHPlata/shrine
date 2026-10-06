# Contract: `ContainerBackend.ResolveImage` under the three policies

**Feature**: 033-pinned-image-policy

## Signature

Unchanged from T2: `ResolveImage(op engine.ResolveImageOp) (engine.ResolvedImage, error)`. `ResolvedImage` gains `Requested` and `PinnedAt`; `Source` gains `resolved` and `pinned`.

## Docker backend

Steps 1 and 2 (alias expansion, started event) are T2's. Step 3 branches on `op.ImagePullPolicy`, which the planner has already normalised to one of the three values.

### `Always` / `IfNotPresent` (manifest-owned)

T2's behaviour, then release the artifact's pin when one is on record (idempotent). A failed release is an error (`releasing image pin for <team>/<name>: <cause>`). `Source` is `manifest`.

### `Pinned`, a usable pin on record

A pin is usable when its `Requested` repository equals the expanded reference's repository (both normalised as `pickRepoDigest` normalises).

1. Inspect `pin.Pinned` locally. Found: no registry call.
2. Not found: pull `pin.Pinned` with the credentials of its registry host (`image.pull` started and finished with `ref` = the digest reference), then inspect. A pull failure is the error
   `pinned exact version "<pin.Pinned>" for <team>/<name> is no longer served by the registry; deploy the <kind> under Always or IfNotPresent to release the pin, then return to Pinned: <cause>`
   emitted as an `image.resolve` error event (`team`, `name`, `ref` = `pin.Pinned`) and returned.
3. Return `ResolvedImage{Ref: pin.Pinned, Digest: <after @>, ImageID: <inspect>, Source: "pinned", Requested: pin.Requested, PinnedAt: pin.PinnedAt}`; finished event with `source=pinned`, `requested`, `pinned_at`.

No pin write happens on this path.

### `Pinned`, no usable pin

1. Pull the expanded reference (never a local lookup, even if the tag is present), inspect, pick the repository digest.
2. Empty digest: error `image "<ref>" carries no registry digest and cannot be pinned`, emitted as an `image.resolve` error event.
3. `Put` `ImagePin{Kind, Name, Requested: <expanded ref>, Pinned: <repo>@<digest>, PinnedAt: now}`; a store failure is `recording image pin for <team>/<name>: <cause>`.
4. Return `ResolvedImage{Ref: <repo>@<digest>, Digest, ImageID, Source: "resolved", Requested: <expanded ref>}`; finished event with `source=resolved`, `requested`.

A pin on record whose repository does not match is overwritten by step 3.

### Ordering guarantee

`ResolveImage` is still called by the engine's pre-pass for every step before `CreatePlatformNetwork`. A pin written for an earlier artifact stays written when a later artifact fails; no container or network exists in either case.

## Dry-run backend

Prints one line per artifact and returns `ResolvedImage{Ref: op.Image, Source: ...}` with no Docker, registry, or store call:

| Policy | Pin in snapshot | Line |
|---|---|---|
| `Always` / `IfNotPresent` | any | `[DOCKER] ImageResolve: name=<team>.<name> image=<as written> policy=<policy> -> manifest-owned` (T2, unchanged) |
| `Pinned` | none | `[DOCKER] ImageResolve: name=<team>.<name> image=<as written> policy=Pinned -> would resolve newest and pin` |
| `Pinned` | present | `[DOCKER] ImageResolve: name=<team>.<name> image=<as written> policy=Pinned -> pinned <pin.Pinned> (<readable>, <YYYY-MM-DD>)` |

The snapshot is `ImagePins.ListAll()` taken once by `handler.DryRun`. The preview shows the pin on record even when its repository no longer matches the manifest; the real deploy replaces such a pin.

## `CreateContainer`

Unchanged. Under `Pinned` the op arrives with `ResolvedRef = <repo>@<digest>` and `ImageID` set, so the container's `Config.Image` is the digest reference and the config hash input is the local image id; an unchanged manifest with a reused pin is up to date and is not recreated.

## Test fakes

Existing fakes need no change: they return `Source: manifest`. New dedicated fakes in the Docker package script `ImageInspect` and `ImagePull` per reference and record calls; a fake `ImagePinStore` records `Put` and `Release`.
