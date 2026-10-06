# Data Model: Resolve Every Image Before Touching Any Container

**Feature**: 032-preflight-image-resolve | **Date**: 2026-10-06

No recorded state changes. This feature adds in-memory types on the backend contract, one field on an existing op, two events, and one projection in the engine.

## 1. Backend contract (`internal/engine/backends.go`)

### 1.1 `ResolveImageOp` — NEW

```go
type ResolveImageOp struct {
	Team            string
	Name            string
	Kind            string // manifest.ApplicationKind | manifest.ResourceKind
	Image           string // as the manifest names it; a reg:<alias> form is not yet expanded
	ImagePullPolicy string // effective policy: Always | IfNotPresent
}
```

Built by the engine from the step's manifest; `ImagePullPolicy` is `manifest.EffectivePullPolicy(spec.Image, spec.ImagePullPolicy)`. The `Repin` field of design section 4.1 is added by T6.

### 1.2 `ResolvedImage` — NEW

```go
type ResolvedImage struct {
	Ref     string // the container is created from this: the expanded reference
	Digest  string // sha256:<64 hex> picked from the local image's repository digests; empty when none matches
	ImageID string // local image id (sha256:…), the config-hash input
	Source  string // ImageSourceManifest
}

const ImageSourceManifest = "manifest"
```

Invariants: `Ref` never carries the `reg:` prefix; `ImageID` is non-empty on success from the Docker backend and empty from the dry-run backend; `Digest` may be empty.

### 1.3 `ContainerBackend` — MODIFIED

```go
type ContainerBackend interface {
	CreateNetwork(name string) error
	RemoveNetwork(name string) error
	CreateContainer(op CreateContainerOp) error
	RemoveContainer(op RemoveContainerOp) error
	CreatePlatformNetwork() error
	InspectContainer(containerID string) (ContainerInfo, error)
	ResolveImage(op ResolveImageOp) (ResolvedImage, error) // NEW
}
```

Implementations: `dockercontainer.DockerBackend`, `dryrun.DryRunContainerBackend`, and the test fakes in `internal/engine`, `internal/handler`, and `internal/plugins/gateway/traefik`.

### 1.4 `CreateContainerOp` — MODIFIED

```go
type CreateContainerOp struct {
	// existing fields unchanged
	ImageID string // NEW: when set, CreateContainer skips resolution and hashes this id
}
```

Set by the engine from `ResolvedImage.ImageID` together with `Image = ResolvedImage.Ref`. Left empty by the Traefik plugin.

## 2. Docker backend internals (`internal/engine/local/dockercontainer/docker_image.go`)

### 2.1 `localImage` — NEW, private

```go
type localImage struct {
	ID          string
	RepoDigests []string
}
```

Produced by `findLocalImage` (from `image.Summary`) and `inspectImage` (from `image.InspectResponse`), so `locateImage` returns one shape regardless of which path found the image.

### 2.2 Helpers

| Function | Signature | Behaviour |
|---|---|---|
| `locateImage` | `(ctx, ref, policy string) (localImage, error)` | policy other than `Always`: `findLocalImage`, return on hit; then `pullImage` and `inspectImage` |
| `findLocalImage` | `(ctx, ref string) (localImage, bool, error)` | `ImageList` with `reference` filter; first summary when present |
| `pullImage` | `(ctx, ref string) error` | credentials via `registryAuthFor`, `image.pull` started/finished events, drains the stream |
| `inspectImage` | `(ctx, ref string) (localImage, error)` | `ImageInspect`, `image.inspect` error event on failure |
| `pickRepoDigest` | `(repoDigests []string, repository string) string` | digest part of the entry whose normalised repository equals the normalised `repository`; `""` when none |
| `repositoryOf` | `(ref string) string` | strips `@digest`, then a tag after the last `/` |
| `normalizeRepository` | `(repository string) string` | trims a leading `docker.io/`, then a leading `library/` |
| `resolveContainerImage` | `(ctx, op *engine.CreateContainerOp) (string, error)` (in `docker_container.go`) | `op.ImageID` when set; else expand alias into `op.Image` and `locateImage(...).ID` |

## 3. Engine projection (`internal/engine/engine.go`)

| Function | Signature | Behaviour |
|---|---|---|
| `resolveImages` | `(set *planner.ManifestSet, steps []planner.PlannedStep) (map[string]ResolvedImage, error)` | one `ResolveImage` per step in order; first error aborts with the `image.resolve` error event; map keyed `resolvedImageKey(kind, name)` = `kind + "/" + name` |
| `resolveImageOpFor` | `(set, step) ResolveImageOp` | reads owner, name, image, and effective policy from the step's manifest |
| `deployApplication` / `deployResource` | gain the map as a parameter | set `op.Image = resolved.Ref`, `op.ImageID = resolved.ImageID` |

`ExecuteDeploy` order after this feature: observer default → `resolveImages` → `CreatePlatformNetwork` → built-ins → `resolveResources` → step loop → `finalizeRouting`.

## 4. Events (`engine.Event`)

| Name | Status | Emitter | Fields |
|---|---|---|---|
| `image.resolve` | started | Docker backend, after alias expansion | `team`, `name`, `ref` (expanded) |
| `image.resolve` | finished | Docker backend, after a successful resolution | `team`, `name`, `ref` (expanded), `digest` (`sha256:…` or empty), `source` (`manifest`) |
| `image.resolve` | error | engine, on the first failed step | `team`, `name`, `ref` (as the manifest wrote it), `error` |
| `image.pull` | started / finished / error | unchanged | `ref` |
| `image.list`, `image.inspect`, `registry.alias`, `registry.auth` | error | unchanged names; two messages now name the reference | `ref`, `error` |

The file logger needs nothing new: it prints every event generically.

## 5. Dry run (`internal/engine/dryrun/dry_run_container.go`)

`ResolveImage` prints one line and returns `ResolvedImage{Ref: op.Image, Source: ImageSourceManifest}`; `ImageID` stays empty, so the dry-run `CreateContainer` line keeps printing `image=<op.Image>` as today. The `Pins` snapshot of design section 4.4 is T3's.

## 6. Terminal (`internal/ui/terminal_logger.go`)

Two new arms under one `case "image.resolve"`: started prints the resolving line; finished prints the resolved line only when `source == manifest`. `shortDigest(digest)` strips `sha256:` and keeps twelve characters (TD-11). Other finished sources render nothing in this ticket.
