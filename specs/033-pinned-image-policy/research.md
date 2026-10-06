# Research: The Pinned Policy: Resolve Once, Keep the Exact Version

**Feature**: 033-pinned-image-policy | **Date**: 2026-10-06

No `NEEDS CLARIFICATION` markers existed in the Technical Context. The epic design settles every choice of substance; the entries below record how each one lands in this codebase as it is on `main` after T2 (#61), the three places where the design's assumptions did not survive contact with the code, and the alternatives rejected.

## R1. The pin store

**Decision**: `internal/state/pins.go` declares `ImagePin{Kind, Name, Requested, Pinned, PinnedAt}`, `ErrImagePinNotFound`, `ImagePinKey(team, name)` (`team + "/" + name`), and the `ImagePinStore` interface of design section 3.4 (`Get`, `Put`, `Release`, `ReleaseTeam`, `List`, `ListAll`). `internal/state/local/pins.go` implements it per team in `<state-dir>/<team>/pins.txt`, copying `hostports.go` and `deployments.go`: a `sync.Mutex`, `readFile`/`writeFile` function fields, `os.ReadFile` and the existing `writeTeamFile` (which creates the team directory with 0700 and writes atomically) in production, `newImagePinStoreWithFileOps` for unit tests, `#` comments and malformed lines skipped on read, lines sorted by name on write. `state.Store` gains `ImagePins ImagePinStore`; `NewLocalStore` constructs it.

**Rationale**: design TD-3 and section 3.4. Per team because a pin needs no cross-team uniqueness. Reusing `writeTeamFile` keeps one directory-creation rule for team files.

**Alternatives considered**: a pin column on `deployments.txt` was rejected by TD-3: the deployment record is dropped on teardown and the pin must outlive it. One `pins.txt` at the state root keyed `team/name` (the host-port layout) was rejected because `delete team` and the per-team `List` would filter by prefix for no gain, and the team directory is where the team's other files live.

## R2. Line format

**Decision**: five space-separated fields, `<kind> <name> <requested> <pinned> <pinned-at>`, for example `Application hello-api 127.0.0.1:5000/shrine/whoami:latest 127.0.0.1:5000/shrine/whoami@sha256:3f2a… 2026-10-06T10:42:17Z`. `PinnedAt` is RFC 3339 in UTC, second precision. A line with fewer than five fields, an unparsable date, or a `pinned` field without `@sha256:` is skipped on read.

**Rationale**: design section 3.4. Both references are free of spaces by construction (registry references cannot contain them). The UTC timestamp sorts and diffs cleanly and prints its date part in the reused line.

**Alternatives considered**: `-` placeholders as in `deployments.txt` were not needed because no field is optional.

## R3. `ResolveImage` under `Pinned`

**Decision**: `DockerBackend.ResolveImage` branches on the effective policy after alias expansion and the started event:

- manifest-owned (`Always`, `IfNotPresent`): today's `locateImage`, then `releasePin(op)` which calls `state.ImagePins.Release(team, name)` (a no-op when absent); a failed release is an error because a stale pin would surprise on return (TD-6). `Source` stays `manifest`.
- `Pinned`, usable pin on record: `reusePin(ctx, op, pin)`: `inspectImage(ctx, pin.Pinned)`; on not-found, `pullImage(ctx, pin.Pinned)` then inspect; the pull failure is wrapped as the "no longer served" message of R8. Returns `Ref: pin.Pinned`, `Digest` = the part after `@`, `ImageID` from the inspect, `Source: pinned`, plus `PinnedAt` in the finished event.
- `Pinned`, no usable pin: `pinNewest(ctx, op, ref)`: `pullImage(ctx, ref)`, `inspectImage`, `pickRepoDigest`; an empty digest is the error `image "<ref>" carries no registry digest and cannot be pinned`; otherwise `Put` the pin `{Requested: ref, Pinned: repositoryOf(ref) + "@" + digest, PinnedAt: now}` and return `Ref` = that pinned reference, `Source: resolved`.

A pin is usable when `normalizeRepository(repositoryOf(pin.Requested)) == normalizeRepository(repositoryOf(ref))`; otherwise it is treated as absent and overwritten by `Put` (spec FR-011).

**Rationale**: design section 4.2 cases 2 and 3; T2 research R3 for the digest composition. The repository comparison closes the gap the spec flagged: a pin for an image the manifest no longer names is meaningless.

**Alternatives considered**: checking local presence with `ImageList` filtered on the digest reference was rejected: Docker's `reference` filter matches `repo:tag` patterns and is unreliable for `repo@digest`; `ImageInspect` resolves a digest reference through the daemon's reference store, which registers `repo@sha256:…` after any tag pull and after a digest pull, and reports not-found cleanly through `errdefs.IsNotFound`. Pulling the tag when the pin is absent locally was rejected by R-13 (never the tag).

## R4. Where the clock comes from

**Decision**: `DockerBackend` gains a `now func() time.Time` field, defaulting to `time.Now().UTC` in `NewDockerBackend`, so unit tests pin the date without touching the system clock.

**Rationale**: the same seam the resolver uses for generated secrets; one injectable function, no clock interface (constitution IV).

## R5. Events and rendering

**Decision**: the finished `image.resolve` event gains `source` values `resolved` and `pinned`, and for `pinned` a `pinned_at` field (`2006-01-02`). `terminal_logger.go` gains two arms under the existing `case "image.resolve"`: `resolved` prints `  📌 Pinned <team>.<name> at <readable>@<12 hex>`, `pinned` prints `  📌 Using pinned <team>.<name> <readable>@<12 hex> (since <date>)`. `readable` is `TagOf(requested)`, `latest` when the request carries no tag, and the short digest alone when `requested` is itself a digest reference, per design section 3.5; T3 never produces such a request, T6's digest bump does, and the helper is written for both so T6 inherits it. The event therefore carries `requested` (the tag reference the pin was resolved from) beside `ref` (the digest reference the container is created from). The "no longer served" failure is emitted by the backend as an `image.resolve` error event before the engine's own, so the terminal shows the cause under the resolving line as it does for a pull failure.

**Rationale**: design section 4.9; T2 hand-off note. `requested` is needed because `ref` is a digest reference under `Pinned` and the readable tag would otherwise be lost. Short digests reuse `shortDigest`.

**Alternatives considered**: printing the full digest reference on the pinned lines was rejected by section 3.5: tables and lines use the readable form; `describe` (T5) shows the full string.

## R6. Plan-time normalisation and validation

**Decision**: `planner.Plan` gains a trailing `defaultPullPolicy string` parameter; the three handlers (`Deploy`, `DryRun`, `ApplySingle`) pass `""`. `applyEffectivePullPolicy(set, defaultPullPolicy)` runs first in `Plan`, filling every empty `Spec.ImagePullPolicy` with `manifest.EffectivePullPolicyWithDefault(image, "", defaultPullPolicy)` (manifest field, else default, else derived rule); `EffectivePullPolicy` stays as the two-argument form and delegates. `Resolve` gains `validateImagePolicies(set, defaultPullPolicy)` after `validateRegistryImages`; the default is received but unused until T4, so T4 adds only the message clause: under `Pinned`, an Application image or Resource image override with a tag other than `latest` or with `@` is `application "x": spec.image "repo:1.2" names a fixed version but the image pull policy is Pinned; use "repo" or "repo:latest"`; a Resource `spec.version` other than empty or `latest` is `resource "db": spec.version "16" names a fixed version but the image pull policy is Pinned; omit it or use "latest"`; not `Pinned` and Resource `spec.version` empty is `resource "db": spec.version is required`. The configuration-sourced parenthetical of design section 4.5 is T4's; the manifest-sourced Resource wording (`omit it or use "latest"`) is this ticket's, accepted by the owner, because the design's second clause (`set spec.imagePullPolicy on the manifest or change the default`) only makes sense when the policy came from the configuration. `validateResourceSpec` drops the version check; `parseManifest` defaults the Resource image to `<type>` when no version is set.

`manifest.TagOf(ref)` and `manifest.IsDigestReference(ref)` are added beside `EffectivePullPolicy`, which switches to `TagOf`. `TagOf` treats a colon as the tag separator only when it comes after the last slash, as `repositoryOf` in the Docker package already does.

**Found while planning**: `EffectivePullPolicy` takes the last colon of the whole reference as the tag separator, so an untagged image on a registry with a port (`127.0.0.1:5000/app`) derives `IfNotPresent` instead of the documented `Always`. Switching to `TagOf` fixes it. This changes the derived policy of such manifests from reuse-local to pull-every-deploy, which is what the manifest reference has always documented; it is recorded as a deviation in the plan and named in the pull request.

**Rationale**: design TD-7 and section 4.5. Normalising first means the engine's `EffectivePullPolicy(image, declared)` calls see the effective value with no further change, and the deployment record stores `Pinned`, which T5 reads.

**Alternatives considered**: validating at parse time was rejected because the rule depends on the effective policy, which T4 will take from the configuration; the planner is where config-aware validation already lives (`validateRegistryImages`).

## R7. The enum check that does not exist

**Decision**: `validate.go` gains `validatePullPolicy(spec.ImagePullPolicy)` for both kinds: empty or one of the three, else `spec.imagePullPolicy must be one of Always, IfNotPresent, Pinned`. `manifest.ImagePullPolicyPinned` and `manifest.IsKnownPullPolicy` are the new names.

**Rationale**: design section 4.5 says "the enum check for `spec.imagePullPolicy` accepts the third value", but no such check exists on `main`: an unknown value is passed through `EffectivePullPolicy` and behaves as `IfNotPresent` in `locateImage`. Constitution I requires field constraints at validate time, and under three values a typo must not silently float. Recorded as a design deviation to note in `design.md` section 1 with the pull request.

## R8. The "no longer served" message

**Decision**: `pinned exact version "<pin.Pinned>" for <team>/<name> is no longer served by the registry; deploy the <kind> under Always or IfNotPresent to release the pin, then return to Pinned` wrapping the pull error as `%w`. Emitted as an `image.resolve` error event by the backend with `team`, `name`, `ref` = `pin.Pinned`. The engine then wraps it as `<kind> "<name>": …` and emits its own error, as for every resolution failure.

**Rationale**: spec FR-012 and ticket T3 ("the message names the manifest as the way out until T6 lands"). T6 replaces the second clause with `run "shrine bump <kind> <name>" to choose another`.

**Deviation from design section 4.2, accepted by the owner**: the reference is quoted, "by the registry" is added, the way out is the concrete manifest edit (deploy under `Always` or `IfNotPresent`) rather than "edit the manifest", which is misleading under `Pinned` where no version can be named, and the pull cause is appended. The "no registry digest" failure is also emitted as a backend `image.resolve` error event, so both pinned failures render their cause under the resolving line before the engine's own error line, as a pull failure does today.

## R9. Dry run

**Decision**: `DryRunContainerBackend` gains `Pins map[string]state.ImagePin` keyed `team/name`; `NewDryRunEngine(out, hostPorts, pins)` fills it; `handler.DryRun` passes `store.ImagePins.ListAll()` (an empty map when the store is nil, so handler unit tests with partial stores keep working). `ResolveImage` prints, under `Pinned`, `-> pinned <pin.Pinned> (<readable>, <date>)` when a pin is on record and `-> would resolve newest and pin` otherwise; manifest-owned keeps T2's line. The preview shows the pin on record as is: it does not expand aliases and so cannot apply the repository-match rule of R3; a pin whose repository no longer matches the manifest is replaced on the real deploy, and the contract says so.

**Rationale**: design section 4.4. The dry-run backend has no registry configuration, by design (constitution III), so it reports recorded state rather than predicting the resolution.

**Alternatives considered**: giving the dry-run backend the registries so it could expand aliases was rejected as a second expansion site (#33 invariant) for a preview edge case.

## R10. Delete releases

**Decision**: `DeleteApplication` reads the pin with `store.ImagePins.Get` after the host port; dry run prints `[dry-run] would release image pin <pinned> for <team>/<name>`; the real path calls `Release` and prints `Released image pin for <team>/<name>.`; "nothing to delete" now also considers the pin. `DeleteTeam` calls `releaseTeamImagePins(store, name)` after the host ports, printing `Released <n> image pin(s) for team "<name>".` when `n > 0`. Both nil-check `store.ImagePins` the way `releaseTeamHostPorts` nil-checks `store.HostPorts`.

**Rationale**: design section 4.8 and TD-8 (handlers own releases). The nil check keeps the handler unit tests' partial stores valid.

## R11. Config hash and recreation

**Decision**: no change to `configHash`. Under `Pinned` the engine sets `op.ImageID` from the pin's inspect, so a reused pin hashes to the same id and `isContainerUpToDate` keeps the container. `op.ResolvedRef` is the digest reference, so `Config.Image` of every pinned container reads `repo@sha256:…`, which is what the integration suite asserts.

**Rationale**: design TD-2 and T3-08.

## R12. The integration fixture

**Decision**: `tests/integration/testutils/registry.go` (integration build tag) adds:

- `StartLocalRegistry(tc) *LocalRegistry`: pulls `registry:2` if absent, creates `shrine-test-registry` from it with `127.0.0.1:0->5000/tcp`, starts it, reads the mapped port from `ContainerInspect`, polls `http://127.0.0.1:<port>/v2/` until it answers, and registers removal (force) in a `tc` cleanup. `LocalRegistry.Host` is `127.0.0.1:<port>`.
- `(*LocalRegistry).PushAs(tc, source, repoTag string) string`: ensures `source` is present locally (pull if absent), tags it as `<host>/<repoTag>`, pushes with an empty base64 `{}` registry auth, and returns the pushed digest read back from `ImageInspect(<host>/<repoTag>).RepoDigests`.
- `(*TestCase).ImageIDOf(ref) string` and `(*TestCase).RemoveImage(ref)` (force) for the wiped-cache cycle.
- `WritePinnedFixture(tc, dir, host, envValue string)`: writes the team-owned Application manifest (`image: <host>/shrine/whoami`, `imagePullPolicy: Pinned`, one env var whose value the recreate cycle changes) and the Resource manifest (`type: cache`, no `version`, `image: <host>/shrine/whoami`, `imagePullPolicy: Pinned`) into a temp directory, because the port is only known at run time.

Source images are `traefik/whoami:v1.10.1` and `traefik/whoami:v1.10.2`, pushed as `<host>/shrine/whoami:latest` in that order.

The helper names and shapes differ from design section 5, accepted by the owner: `PushAs` is a method on `LocalRegistry` and returns the pushed digest; there is no `AssertContainerImageDigest`, because every pinned container is created from the digest reference and the existing `AssertContainerImage` on `Config.Image` plus an image-id comparison prove the same thing; the pinned manifests are written at run time rather than substituted into on-disk fixtures, and only the validation fixture `fixed-version/` lives under `tests/testdata/pinned/`.

**Rationale**: design section 5 and TD-13. Docker treats `127.0.0.0/8` registries as insecure, so the daemon needs no configuration and `config.yml` no `registries` entry; an empty auth header satisfies the SDK's push. Two whoami tags instead of the design's two alpine tags: alpine has no long-running entrypoint and would exit at once, failing `AssertContainerRunning`; whoami serves until stopped and `v1.10.2` is already cached on the runner by the T2 suite. Writing the manifests at run time is how the vault suite injects run-time values into its config.

**Alternatives considered**: a fixed host port for the registry was rejected because parallel runs on one host would collide. Docker Hub as the registry was rejected by section 5: its `latest` cannot be moved.

## R13. Test strategy

**Decision**:
- Integration, written first in `tests/integration/pinned_image_policy_test.go` on `NewDockerSuite(t, testTeam)` with the registry started in `BeforeEach`: (a) validation: a fixture directory `tests/testdata/pinned/fixed-version/` with a `Pinned` application naming a tag, a `Pinned` resource naming a version, and a resource with an unknown policy value, deployed with `--dry-run`, fails and stderr names each artifact and field; (b) first deploy pins: output has `📌 Pinned`, `pins.txt` holds one line per artifact with the digest `PushAs` returned, `Config.Image` of both containers is `<host>/shrine/whoami@sha256:…`; (c) the ten-cycle loop after moving `latest` to `v1.10.2`: plain redeploy (`📌 Using pinned`, container id unchanged, no `Pulling image` line), env change (container id changes, image unchanged), teardown and deploy, `RemoveImage` of the pinned reference then deploy (`Pulling image <digest ref>` appears), and repeats; every cycle asserts `Config.Image` equals the first digest reference and `.Image` equals the id of `v1.10.1`; (d) release paths: teardown, `delete application`, deploy pins `v1.10.2`'s digest; `delete team` after teardown releases the resource's pin too; editing the application to `image: <host>/shrine/whoami:v1.10.2` under no policy and deploying prints `🔎 Resolved` and removes the line from `pins.txt`, then restoring `Pinned` pins afresh; (e) `delete application --dry-run` prints the would-release line and leaves `pins.txt` unchanged; (f) dry run: before any pin, `-> would resolve newest and pin`; after a deploy, `-> pinned <digest ref> (latest, <date>)`; two consecutive dry runs leave the state directory byte-identical (compare `pins.txt` and `deployments.txt` contents); (g) no longer served: after a teardown, rewrite the application's `pins.txt` line so `pinned` names the same repository at a digest the registry has never held (`sha256:` followed by 64 zeros) and make sure no local image carries it; deploy fails, stderr contains `is no longer served by the registry` and `application "<name>"`, no container of the team exists, and the team network does not exist. The existing suites are untouched.
- Unit: `ImagePinStore` over an in-memory file fake (load forgiving, put sorts and persists, release idempotent, release team, list all keyed, missing file is empty); `validateImagePolicies` and `applyEffectivePullPolicy` tables; `validatePullPolicy`; `parseManifest` resource image default without version; `DockerBackend.ResolveImage` over `fakeDockerAPI` extended with scripted `ImageInspect` and `ImagePull` results and a fake pin store: first pin writes the expected record with the injected clock, reuse inspects and never pulls, reuse pulls by digest on not-found, "no longer served" wording and event, no-digest error, manifest-owned releases, repository mismatch re-pins; dry-run lines for the three decisions; the two terminal arms byte-exact; `DeleteApplication` and `DeleteTeam` release and dry-run lines over the handler's stub store; `NewDryRunEngine` snapshot wiring.

**Rationale**: constitution V and the memory rules (integration written first and never run locally; unit tests off the filesystem).

## R14. Documentation touch points

**Decision**: `docs/content/reference/manifest-schema.md`: both `imagePullPolicy: <Always|IfNotPresent|Pinned>` YAML lines; the Resource `spec.version` row becomes `yes, unless `imagePullPolicy` is `Pinned``; both `spec.imagePullPolicy` rows list the three values and link to a new `### Image pull policy` subsection after `spec.env[]` that states the three modes, the no-fixed-version rule with the two error shapes, and the pin lifecycle. `AGENTS.md`: the state layout gains `pins.txt`; the `delete application` entry says it releases the image pin and that `delete team` releases the team's pins; the pipeline note after the diagram gains one sentence on the pinned branch. `specs/progress.md` gains the entry. No CLI page changes.

## R15. Open technical points from design section 7

- **Multi-architecture digest**: not verified by this ticket. The daemon records in `RepoDigests` the digest of what the registry served for the tag, which is believed to be the index for a multi-architecture tag, but no code in this ticket depends on which it is: either digest pins correctly on one host, and the fixture pushes a single-platform image. The PRD's architecture-move note stays an unverified expectation until someone checks it against a daemon.
- **Local presence check**: `ImageInspect` with a digest reference, see R3. Verified by the wiped-cache cycle of the integration suite (inspect must report not-found, then the pull by digest must succeed and a second inspect must find it).

## R16. Hand-off notes for the next tickets

- T4 passes `cfg.ImagePullPolicy` where this ticket passes `""` to `Plan`, and adds the configuration-sourced parenthetical to the two messages in `validateImagePolicies`, which already receives the default and can tell whether the policy came from it.
- T5 reads `state.Deployment.Policy == Pinned` and `ImagePins.Get` for the VERSION column; the readable form is `readableVersion(requested, digest)`, which this ticket keeps private to the terminal package; T5 should move `readableVersion` and `shortDigest` to a shared place when it needs them.
- T6 adds `Repin` to `ResolveImageOp` and the `repinned` branch beside `reusePin` and `pinNewest`, and replaces the second clause of R8's message.
- T7 generalises `DeleteApplication`; the pin release code written here takes the kind from the record, not a constant, so the generalisation is a parameter change.
