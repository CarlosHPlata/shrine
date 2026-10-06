# Contract: `ContainerBackend.ResolveImage`

**Feature**: 032-preflight-image-resolve

## Signature

```go
ResolveImage(op engine.ResolveImageOp) (engine.ResolvedImage, error)
```

## Docker backend (`dockercontainer.DockerBackend`)

Given `op.Image` and `op.ImagePullPolicy`:

1. Expand a `reg:<alias>` prefix exactly once. Failure: `registry.alias` error event with `ref` = the manifest form; error text `image "<ref>": alias "<alias>" is not defined in config registries` (unchanged wording).
2. Emit `image.resolve` started with `team`, `name`, `ref` (expanded).
3. Locate the image:
   - `ImagePullPolicy != "Always"`: list local images matching the expanded reference. A hit ends the search with no registry call. Failure: `image.list` error event; text `listing images matching "<ref>": <cause>`.
   - Otherwise, or on a miss: pull with the registry credentials for the reference's host (`image.pull` started and finished events, unchanged), then inspect. Failures: `registry.auth` (`registry credentials for "<ref>": <cause>`), `image.pull` (`pulling image "<ref>": <cause>` or `reading image stream for "<ref>": <cause>`), `image.inspect` (`inspecting image "<ref>": <cause>`), each an error event with `ref`.
4. Pick the digest: the `sha256:…` part of the repository digest whose repository equals the reference's repository after both drop a leading `docker.io/` and then a leading `library/`. No match yields an empty digest and no error.
5. Emit `image.resolve` finished with `team`, `name`, `ref`, `digest`, `source` = `manifest`.
6. Return `ResolvedImage{Ref: <expanded>, Digest: <picked or empty>, ImageID: <local id>, Source: "manifest"}`.

On any failure the method returns the error of the failing step without emitting an `image.resolve` event of its own; the engine emits the `image.resolve` error.

Pull semantics are exactly today's: `Always` always pulls; any other policy pulls only when no local image matches.

## Dry-run backend (`dryrun.DryRunContainerBackend`)

Prints `[DOCKER] ImageResolve: name=<team>.<name> image=<op.Image> policy=<op.ImagePullPolicy> -> manifest-owned` and returns `ResolvedImage{Ref: op.Image, Source: "manifest"}`. No Docker call, no registry call, no state write. The reference is printed as the manifest wrote it.

## Engine use (`engine.Engine.ExecuteDeploy`)

- Called once per planned step, in step order, before `CreatePlatformNetwork`, with `Team`, `Name`, `Kind` from the manifest, `Image` as written, and `ImagePullPolicy` = `manifest.EffectivePullPolicy(image, declared)`.
- The first error aborts `ExecuteDeploy`. The engine emits `image.resolve` error with `team`, `name`, `ref` (as written), `error`, and returns `fmt.Errorf("%s %q: %w", lower(kind), name, err)`: `application "web": pulling image "ghcr.io/x/web:1.2": <cause>`.
- On success the step's `CreateContainerOp` carries `Image = Ref` and `ImageID = ImageID`.

## `CreateContainer` with a resolved image

- `op.ImageID != ""`: no alias expansion, no image list, no pull, no inspect; `op.Image` is used as given for the container spec and `op.ImageID` as the config-hash input.
- `op.ImageID == ""`: today's path, unchanged (the Traefik plugin).

## Test fakes

Every `ContainerBackend` fake gains `ResolveImage`. The engine fakes return `ResolvedImage{Ref: op.Image, Source: "manifest"}` so existing projection tests see the same `Image` on the op; dedicated fakes record calls and return a configured error to drive the abort test.
