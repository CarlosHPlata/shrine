# Research: Resolve Every Image Before Touching Any Container

**Feature**: 032-preflight-image-resolve | **Date**: 2026-10-06

No `NEEDS CLARIFICATION` markers existed in the Technical Context. The epic design settles every choice; the entries below record how each one lands in this codebase and the alternatives the design or this plan rejected.

## R1. Backend contract shape

**Decision**: `internal/engine/backends.go` gains

```go
type ResolveImageOp struct {
    Team, Name, Kind string
    Image            string // as the manifest names it, alias not yet expanded
    ImagePullPolicy  string // effective policy
}

type ResolvedImage struct {
    Ref     string // what the container is created from: the expanded reference
    Digest  string // sha256:… from the registry; empty when the image has no repository digest
    ImageID string // local image id, the config-hash input
    Source  string // ImageSourceManifest in this ticket
}

const ImageSourceManifest = "manifest"
```

and `ContainerBackend` gains `ResolveImage(op ResolveImageOp) (ResolvedImage, error)`. `CreateContainerOp` gains `ImageID string`.

**Rationale**: design section 4.1 verbatim, minus the `Repin` field that T6 adds. A named constant for the source keeps the renderer's comparison and the dry-run return free of string literals and gives T3 a place to add `pinned`, `resolved`, `repinned`.

**Alternatives considered**: returning only the image id (today's private contract) was rejected by TD-1: the id is not pullable and cannot be printed as a version. Adding `Repin` now was rejected by the ticket scope.

## R2. Where alias expansion lives

**Decision**: `ResolveImage` expands `reg:<alias>` exactly once, as `CreateContainer` does today, before the pull, the credential lookup, and the inspection; `ResolvedImage.Ref` is the expanded form. `CreateContainer` expands only on its own path (`ImageID` empty, the Traefik plugin); on the engine path `op.Image` already arrives expanded.

**Rationale**: issue #33's invariant: every reader of the reference sees the same fully-qualified string. Design section 4.2 step 1.

**Alternatives considered**: expanding in the engine was rejected by constitution III (registry configuration is backend knowledge); expanding in both places unconditionally is harmless for an expanded string but hides the invariant, so the container path branches on `ImageID` instead.

## R3. Picking the exact version

**Decision**: `pickRepoDigest(repoDigests []string, repository string) string` returns the `sha256:…` part of the entry whose repository (the part before `@`) equals `repository` after both are normalised by `normalizeRepository`: strip a leading `docker.io/`, then a leading `library/`. `repositoryOf(ref)` strips a `@digest` suffix and then a tag (a `:` after the last `/`). No match returns the empty string, which is allowed for manifest-owned artifacts.

**Rationale**: the daemon records Docker Hub repositories as `traefik/whoami@sha256:…` and official images as `postgres@sha256:…`, while an expanded alias reads `docker.io/traefik/whoami:latest`. Without normalisation the test fixtures (Docker Hub images) would print no digest. Design section 4.2 fixes the matching rule and section 7 flags the Hub form as the point to verify; the normalisation makes the comparison correct for both forms the daemon can write. The design describes the helper as returning the matching entry; this plan returns the digest portion because `ResolvedImage.Digest` is documented as `sha256:…` and T3 composes the pullable pin as `repositoryOf(ref) + "@" + digest`.

**Alternatives considered**: taking `RepoDigests[0]` was rejected: an image tagged under several repositories (a local mirror and a Hub tag) would pin the wrong registry. Failing on an empty result was rejected for manifest-owned artifacts by design section 4.2 ("warning-free empty Digest").

## R4. Pull semantics and the local lookup

**Decision**: `locateImage(ctx, ref, policy) (localImage, error)` keeps today's branches: for any policy other than `Always`, `ImageList` with a `reference` filter; a hit returns `localImage{ID, RepoDigests}` from the `image.Summary` without any further call. Otherwise `pullImage` (credentials via `registryAuthFor`, `image.pull` started and finished events unchanged) followed by `inspectImage`, returning the same two fields from the `image.InspectResponse`.

**Rationale**: T2-07 and spec FR-004. `image.Summary` carries `RepoDigests`, so the local hit yields the digest with no registry call and no extra daemon call; the existing `fakeDockerAPI` (whose `ImageInspect` panics) keeps driving `CreateContainer` tests unchanged.

**Alternatives considered**: always inspecting after the list was rejected: an extra daemon call per artifact and a change to the existing unit fakes for no behavioural gain.

## R5. Engine pre-pass

**Decision**: `ExecuteDeploy` calls `engine.resolveImages(set, steps)` immediately after installing the no-op observer and before `CreatePlatformNetwork`. The method builds one `ResolveImageOp` per step via `resolveImageOpFor(set, step)` (team, name, kind, manifest image, `manifest.EffectivePullPolicy`), calls `ResolveImage`, and returns `map[string]ResolvedImage` keyed `kind + "/" + name`. The first error returns `engine.emitErr("image.resolve", {team, name, ref}, fmt.Errorf("%s %q: %w", strings.ToLower(kind), name, err))`. `deployApplication` and `deployResource` receive the map and set `op.Image = resolved.Ref`, `op.ImageID = resolved.ImageID`.

**Rationale**: design section 4.3 verbatim. Lower-casing the kind in the error matches the `application "x": …` form every other engine error uses (spec 028).

**Alternatives considered**: resolving inside the step loop just before `CreateContainer` was rejected: it is today's failure mode. Resolving every artifact of the manifest set rather than the planned steps was rejected by design section 4.3 ("only artifacts that will be deployed are resolved").

## R6. CreateContainer skip rule

**Decision**: `resolveContainerImage(ctx, op *CreateContainerOp) (imageID string, err error)` returns `op.ImageID` when set; otherwise it expands the alias into `op.Image` and returns `locateImage(...).ID`. `CreateContainer` calls it where it called `resolveImage` today and feeds the id to `configHash`.

**Rationale**: T2-03; the Traefik plugin leaves `ImageID` empty and is untouched.

**Alternatives considered**: a separate `CreateContainerOp.Resolved *ResolvedImage` was rejected: the hash needs only the id, and a nil-able struct invites a second source of truth for `Image`.

## R7. Config hash unchanged

**Decision**: `configHash(op, imageID)` keeps the local image id as its first input.

**Rationale**: TD-2: a redeploy after upgrade produces the same hash for an unchanged manifest, so nothing is recreated.

**Alternatives considered**: hashing the registry digest instead was rejected by TD-2 (every container would be recreated once on upgrade, and images without a digest would hash to a different input than before).

## R8. Events and rendering

**Decision**: the Docker backend emits `image.resolve` started with `team`, `name`, `ref` (expanded) after a successful alias expansion, and finished with `team`, `name`, `ref`, `digest`, `source` after a successful resolution. Failures emit nothing new from the backend; the failing operation's own error event (`registry.alias`, `image.list`, `registry.auth`, `image.pull`, `image.inspect`) and the engine's `image.resolve` error carry the message. The terminal renders the started line as a plain line, not an indicator, because `image.pull` opens its own indicator inside the step and the single-indicator renderer cannot nest two.

**Rationale**: design section 4.2 step 5 and section 4.9; the clarification session fixed the plain-line choice and the two-error-line shape, which mirrors `container.create` today.

**Alternatives considered**: a spinner for the started line was rejected because `handleStep` would leak the running indicator when `image.pull` replaces it.

## R9. Dry-run line

**Decision**: `DryRunContainerBackend.ResolveImage` prints `[DOCKER] ImageResolve: name=<team>.<name> image=<op.Image> policy=<policy> -> manifest-owned` and returns `ResolvedImage{Ref: op.Image, Source: ImageSourceManifest}`. `op.Image` is the manifest form, so an alias stays visible exactly as the `ContainerCreate` line shows it today.

**Rationale**: design section 4.4, manifest-owned line only. The `Pins` snapshot and the `NewDryRunEngine` signature change belong to T3.

## R10. Failure text

**Decision**: the engine wraps the backend error as `<kind> "<name>": <backend error>`; every backend failure names the reference. The one message that did not, `listing images: …`, becomes `listing images matching "<ref>": …`; the credential-encoding failure becomes `registry credentials for "<ref>": …`.

**Rationale**: spec FR-002 and the clarification on which form of the reference is named: the failing operation's own form.

**Alternatives considered**: a uniform wrap `resolving image "<ref>" for <team>/<name>: …` was rejected: it names the reference twice on the common pull failure.

## R11. Test strategy

**Decision**:
- Integration, written first in `tests/integration/preflight_image_resolve_test.go` on `NewDockerSuite(t, "shrine-deploy-test")`: (a) fixture `preflight-unresolvable` with a healthy resource, a healthy application, and an application that declares dependencies on both and names `localhost:1/shrine/unresolvable:1.0.0`, so the broken step is last by construction; assert failure, stderr names the artifact and the reference, neither healthy container exists, the team network does not exist; (b) `deploy --dry-run` of the `resources` fixture: one `ImageResolve` line per artifact, every one before `CreatePlatformNetwork` and the first `ContainerCreate`, no container afterwards; (c) fixture `preflight-fixed-tag` deployed twice: the second output has no `Pulling image` line and the container id is unchanged; (d) the `basic` fixture deployed twice: both outputs pull. One scenario appended to `TestDeploy` asserts the two terminal lines per artifact on the `resources` fixture.
- Unit: engine pre-pass ordering and abort over `fakeContainerBackend` with a timeline; `pickRepoDigest` and `repositoryOf` tables; `DockerBackend.ResolveImage` over a `fakeDockerAPI` that records calls for `Always` (pull then inspect), `IfNotPresent` present (list only) and absent (list, pull, inspect), and the events emitted; `CreateContainer` with `ImageID` set never calls `ImageList`, `ImagePull`, or `ImageInspect` and hashes that id; the dry-run line; the two renderers byte-exact, including the no-digest form and silence for a non-manifest finished source.

**Rationale**: constitution V (integration written first, real binary) and the memory rules: no filesystem in unit tests, integration never run locally, `localhost:1` as the established unreachable registry (spec 029).

## R12. Documentation touch points

**Decision**: `AGENTS.md` only: the Deploy Pipeline diagram gains `Container.ResolveImage()` as a pre-pass before `CreatePlatformNetwork()`, the `CreateContainer(op)` line stops claiming the image pull, and one sentence under the diagram states that a failure during resolution leaves networks and containers untouched. `specs/progress.md` gains the entry. No manifest reference, configuration, or CLI page changes (no new surface).

## R13. Hand-off notes for the next tickets

- T1 plans to store the reference "as written" in the deployment record by reading `op.Image` inside `CreateContainer` before expansion. On the engine path, `op.Image` now arrives already expanded (`ResolvedImage.Ref`), so T1 or T3 must carry the manifest form separately (for example a field on `CreateContainerOp` set by the engine from the manifest) if the record must keep the alias form.
- `pickRepoDigest` returns the digest portion; the pullable pin TD-1 describes is `repositoryOf(ref) + "@" + digest`.
- `ResolveImage` emits no error event of its own; T3's "pin no longer served" failure should emit `image.resolve` error from the backend so the terminal prints it before the engine's line, or rely on the engine's line alone.
