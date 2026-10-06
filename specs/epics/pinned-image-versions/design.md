# Technical design: Pinned Image Versions

**Status**: Draft for review
**Source**: [prd.md](prd.md) for the what and why, [tickets.md](tickets.md) for the cut into tickets
**Created**: 2026-10-06

This document binds the PRD's requirements to the code: which seams change, what the new state looks like on disk, which interfaces grow, and what each ticket's spec may take as settled. It names files and functions as they exist on `main` at `6364b88`. A ticket's spec refines the design within its section; it does not reopen a decision recorded in section 2 without amending this document.

## 1. The seams as they are today

Read with [AGENTS.md](../../../AGENTS.md); only what this epic touches is listed.

| Concern | Where | What it does now |
|---|---|---|
| Pull policy values and the derived rule | `internal/manifest/types.go`: `ImagePullPolicyAlways`, `ImagePullPolicyIfNotPresent`, `EffectivePullPolicy(image, declared)` | `latest` or no tag yields `Always`; any other tag yields `IfNotPresent`. *Amended by T3 (spec 033):* the tag was taken from the last colon of the whole reference, so an untagged image on a registry with a port (`127.0.0.1:5000/app`) derived `IfNotPresent`; `TagOf` now counts a colon only after the last slash and the derived rule follows the documented wording. |
| Resource image defaulting | `internal/manifest/parser.go`, `parseManifest`, Resource case | When `spec.image` is empty, sets it to `<type>:<version>`, but only when `version` is non-empty. |
| Parse-time validation | `internal/manifest/validate.go`: `validateApplicationSpec`, `validateResourceSpec` | `spec.image` required on Application; `spec.version` required on Resource. *Amended by T3 (spec 033):* no enum check on `spec.imagePullPolicy` existed; an unknown value silently behaved as `IfNotPresent`. T3 adds `validatePullPolicy` for both kinds. |
| Plan-time, config-aware validation | `internal/planner/resolve.go`: `Resolve`, `validateRegistryImages` | Validates `reg:<alias>` prefixes against config registries. The precedent for validation that needs config. |
| Planning entry point | `internal/planner/plan.go`: `Plan(set, teams, registries, ports, filter)` | Called from `handler.Deploy`, `handler.DryRun`, and `handler.ApplySingle`. |
| Container ops | `internal/engine/engine.go`: `ExecuteDeploy`, `deployApplication`, `deployResource` | Builds a `CreateContainerOp` per step with `Image` and `ImagePullPolicy` taken from the manifest. |
| Backend contract | `internal/engine/backends.go`: `ContainerBackend`, `CreateContainerOp`, `ContainerInfo` | Six methods; `ContainerInfo` carries `Running`, `Status`, `ImageID`. |
| Image resolution | `internal/engine/local/dockercontainer/docker_image.go`: `resolveImage(ctx, ref, policy)` | Runs inside `CreateContainer`. Returns the Docker image id from `ImageInspect(...).ID`, which is the local config digest and is not pullable from a registry. |
| Container reconcile | `internal/engine/local/dockercontainer/docker_container.go`: `CreateContainer`, `configHash`, `recordDeployment`, `RemoveContainer` | Expands `reg:` aliases, resolves the image, hashes image id plus env, volumes, ports, platform flag; recreates when the hash differs; records after start; `RemoveContainer` drops the record. |
| Deployment record | `internal/state/deployments.go`: `Deployment{Kind, Name, ContainerID, ConfigHash}`; `internal/state/local/deployments.go` | One line per artifact in `<state-dir>/<team>/deployments.txt`, four space-separated fields, removed on teardown. |
| Allocation-type state | `internal/state/hostports.go`, `internal/state/local/hostports.go` | Outlives teardown; released only by delete; atomic temp-and-rename writes; injectable file ops for unit tests. The pattern the pin store copies. |
| Store aggregate | `internal/state/state.go`: `Store`; `internal/state/local/local.go`: `NewLocalStore` | Where a new store is added and constructed. |
| Dry run | `internal/engine/dryrun/dry_run_container.go`, `dry_run_engine.go` | Print-only backend; receives a read-only host-port snapshot to preview held ports. |
| Queries | `internal/handler/deployments.go`: `printDeploymentsTable`, `printDeploymentDetail`, `DeleteApplication`; `internal/handler/status.go`; `internal/handler/teams.go`: `DeleteTeam` | State-only tables; status inspects Docker; delete releases host ports. |
| Composition root | `internal/app/app.go`: `BuildDeployBundle`, `BuildApplyBundle`, `BuildTeardownBundle`; `internal/app/components.go`: `NewQueryContainerBackend` | Where a new command's dependencies are assembled. |
| Terminal rendering | `internal/ui/terminal_logger.go` | One `case` per event name; unknown names render generically. |
| Config | `internal/config/config.go`: `Config`, `Load` | Top-level keys `registries`, `specsDir`, `teamsDir`, `plugins`. |
| Generate templates | `internal/handler/apps.go`, `internal/handler/resources.go`, `cmd/generate.go` | App image defaults to `<name>:latest`; resource version defaults to `16`. | *Amended by T4 (spec 034):* both defaults are decided in the handler from the configuration default, and the `--version` flag default is empty so an explicit `16` can be told from an omitted flag.

## 2. Decisions

Numbered so tickets can cite them. D1 to D10 in the PRD are product decisions; these are their technical consequences.

- **TD-1. The pin is the repository digest, stored as a pullable reference.** After a pull, `ImageInspect(ref).RepoDigests` lists `<repository>@sha256:<hex>` entries; the one whose repository equals the pulled reference's repository is the pin. It is stored whole, for example `ghcr.io/me/hello-api@sha256:3f2a…`, so a later deploy can pull it as is. The local image id from `ImageInspect(...).ID` is not pullable and is not the pin.
- **TD-2. The config hash keeps using the local image id.** `configHash` is unchanged in inputs. A pinned artifact resolves to the same image id every time, so its hash is stable; a bump changes the image id, so the hash changes and the container is recreated on the next deploy. Nothing is recreated merely because Shrine was upgraded.
- **TD-3. A new store, `ImagePinStore`, one file per team.** `<state-dir>/<team>/pins.txt`, beside `deployments.txt` and `secrets.env`. Per team because a pin is per artifact and needs no cross-team uniqueness, unlike host ports. Lifecycle copied from the host-port store: survives teardown; written atomically; released by delete and by a deploy under a manifest-owned policy (TD-6).
- **TD-4. The deployment record gains two fields: the image reference and the policy the container was deployed under.** They are appended to the `deployments.txt` line; legacy four-field lines still parse. This is how `get` shows a version without Docker and how later tickets know whether an artifact is pinned.
- **TD-5. Image resolution becomes a backend method and an engine pre-pass.** `ContainerBackend` gains `ResolveImage`. `ExecuteDeploy` calls it for every step before `CreatePlatformNetwork`, so a failure leaves networks and containers untouched. `CreateContainer` no longer resolves when the op carries a resolved image. The dry-run backend prints the step. The engine stays free of Docker and pin logic, per the constitution.
- **TD-6. A pin belongs to the pinned policy.** When an artifact deploys under `Always` or `IfNotPresent`, its pin is released during resolution. Returning to the pinned policy is therefore a first deploy. This replaces the PRD's "kept, inert" wording of R-12 and sharpens R-11; the amendment is listed in section 6.
- **TD-7. Effective policy is computed once, at plan time, and written back into the manifest set.** A normalisation step at the start of `Plan` fills every empty `Spec.ImagePullPolicy` with the effective value: the manifest field, else the configuration default, else the derived rule. Everything downstream, including the engine's existing `EffectivePullPolicy` calls, then sees the effective value. The no-fixed-version rule and the version-required rule are validated after normalisation, in the planner, next to `validateRegistryImages`; the version-required check moves out of parse-time validation for that reason.
- **TD-8. The backend owns every pin write; handlers own pin reads and releases.** First-deploy pinning and bump both go through `ResolveImage`; `get`, `describe`, and dry run read; `delete` releases. One writer keeps the record shape in one place.
- **TD-9. The third value is `Pinned`.** Constant `manifest.ImagePullPolicyPinned`. Final per OD-1 unless the PRD is amended.
- **TD-10. The configuration key is `imagePullPolicy`, top level.** Final per OD-2 unless the PRD is amended.
- **TD-11. Short exact versions are twelve hex characters.** Per OD-4, matching `shortContainerID`.
- **TD-12. Bump needs a manifest directory and a container backend.** It loads manifests the way deploy does to read the policy and the repository, and it pulls through the Docker backend to resolve. It never calls the engine.
- **TD-13. T3's integration gate runs a local registry.** The only way to prove "latest moved and the pin did not" is a registry the test can push to. See section 5.

## 3. Data

### 3.1 Manifest

```yaml
spec:
  imagePullPolicy: Pinned     # Always | IfNotPresent | Pinned
```

Under `Pinned`: an Application `spec.image` carries no tag or the tag `latest`; a Resource omits `spec.version` or sets `latest`, and a Resource `spec.image`, when present, follows the Application rule. A digest reference in a manifest is a fixed version and is rejected under `Pinned`.

### 3.2 Configuration

```yaml
imagePullPolicy: Pinned       # optional; absent means the derived rule
```

Validated in `config.Load`: empty or one of the three values; anything else fails with `imagePullPolicy: must be one of Always, IfNotPresent, Pinned`.

### 3.3 Deployment record, extended

`internal/state/deployments.go`:

```go
type Deployment struct {
    Kind        string
    Name        string
    ContainerID string
    ConfigHash  string
    Image       string // the reference the manifest named, as written, e.g. reg:lab/hello-api:1.2.0
    Policy      string // effective image pull policy at deploy time
}
```

`<state-dir>/<team>/deployments.txt`, one line per artifact, fields separated by single spaces, appended in this order:

```text
Application hello-api 9f1c…e2 3a7b…c9 reg:lab/hello-api:latest Pinned
Resource    hello-db  5d0a…11 b2e4…77 postgres:16              IfNotPresent
```

Reader rule: split on spaces; fields five and six are optional and read as empty when absent. Image references never contain spaces. The writer always writes six fields. An empty optional value is written as `-` and read back as empty, so later fields keep their position (T1 review, #60).

### 3.4 Pin record, new

`internal/state/pins.go`:

```go
var ErrImagePinNotFound = errors.New("image pin not found")

type ImagePin struct {
    Kind      string
    Name      string
    Requested string    // the reference that was resolved, expanded: ghcr.io/me/hello-api:latest, postgres:17, or repo@sha256:… after a digest bump
    Pinned    string    // the pullable exact version: ghcr.io/me/hello-api@sha256:3f2a…
    PinnedAt  time.Time // UTC
}

type ImagePinStore interface {
    Get(team, name string) (ImagePin, error)      // ErrImagePinNotFound when absent
    Put(team string, pin ImagePin) error           // insert or replace
    Release(team, name string) error               // idempotent
    ReleaseTeam(team string) error                 // idempotent
    List(team string) ([]ImagePin, error)
    ListAll() (map[string]ImagePin, error)         // keyed "team/name"; the dry-run snapshot
}
```

`<state-dir>/<team>/pins.txt`, one line per artifact, five space-separated fields, sorted by name:

```text
Application hello-api ghcr.io/me/hello-api:latest ghcr.io/me/hello-api@sha256:3f2a9c1b4d7e…  2026-10-06T10:42:17Z
Resource    hello-db  postgres:17                 postgres@sha256:9c1b4d7e3f2a…               2026-10-06T10:40:03Z
```

`Requested` is stored expanded, so a `reg:` alias never has to be re-expanded to compare with `Pinned`. Both strings are free of spaces by construction. The implementation in `internal/state/local/pins.go` copies `hostports.go`: mutex, atomic temp-and-rename, `#` comments and malformed lines skipped on read, injectable `readFile`/`writeFile` so unit tests stay off the filesystem. `state.Store` gains `ImagePins`; `NewLocalStore` constructs it.

### 3.5 Readable form

Wherever a pin is shown in a table: `<readable>@<twelve hex>`, where readable is the tag of `Requested`, or the short digest alone when `Requested` is itself a digest reference. Examples: `latest@3f2a9c1b4d7e`, `17@9c1b4d7e3f2a`. `describe` shows the full `Pinned` string and the date. *Amended by T3 (spec 033):* an untagged `Requested` (`postgres`) reads as `latest`.

## 4. Interfaces and flow

### 4.1 Backend contract

`internal/engine/backends.go`:

```go
type ResolveImageOp struct {
    Team, Name, Kind string
    Image            string // as the manifest names it, alias not yet expanded
    ImagePullPolicy  string // effective policy
    Repin            string // bump only: a reference to resolve and record in place of the current pin
}

type ResolvedImage struct {
    Ref       string    // what the container is created from: the expanded tag reference, or the pinned digest reference
    Digest    string    // sha256:… from the registry; empty only when the image has no repository digest
    ImageID   string    // local image id, the config-hash input
    Source    string    // "manifest" | "pinned" | "resolved" | "repinned"
    Requested string    // T3 (spec 033): the expanded tag reference a pin was resolved from; Ref is a digest under Pinned, so the readable tag lives here
    PinnedAt  time.Time // T3 (spec 033): zero unless Source is "pinned"
}

type ContainerBackend interface {
    // existing six methods, plus:
    ResolveImage(op ResolveImageOp) (ResolvedImage, error)
}
```

`CreateContainerOp` gains `ImageID string`: when set, `CreateContainer` uses it as the hash input and does not call `resolveImage`. The Traefik plugin, which calls `CreateContainer` directly, leaves it empty and keeps today's path. *Amended by T2 (spec 032):* `CreateContainerOp` also gains `ResolvedRef string`, the pullable reference the container is created from, set by the engine beside `ImageID`; `Image` is never overwritten and stays the reference as the manifest wrote it, which is what the deployment record of section 3.3 stores. On the direct path `CreateContainer` expands the alias into `ResolvedRef` and leaves `Image` untouched. `ContainerInfo` gains `Image string`, the reference the container was created from, read from `ContainerInspect(...).Config.Image`.

### 4.2 Docker backend behaviour of `ResolveImage`

In `internal/engine/local/dockercontainer/docker_image.go`, replacing the private `resolveImage` as the primary entry:

1. Expand the `reg:` alias once, as `CreateContainer` does today.
2. `Always` or `IfNotPresent`: today's behaviour, then `ImageInspect` for the image id and the repository digest. If the artifact has a pin, release it (TD-6). `Source` is `manifest`.
3. `Pinned` with `Repin` empty:
   - pin exists: `ImageInspect(pin.Pinned)`; when absent locally, pull `pin.Pinned` with the registry credentials of its host; a pull failure returns `pinned exact version <pin.Pinned> for <team>/<name> is no longer served; run "shrine bump <kind> <name>" to choose another` (T3 wording says "edit the manifest" until T6 lands). `Ref` is `pin.Pinned`, `Source` is `pinned`. *Amended by T3 (spec 033), as shipped:* a pin is usable only when its `Requested` repository equals the expanded reference's repository; otherwise it is treated as absent and replaced by the no-pin branch, because a pin for an image the manifest no longer names is meaningless. The T3 message reads `pinned exact version "<pin.Pinned>" for <team>/<name> is no longer served by the registry; deploy the <kind> under Always or IfNotPresent to release the pin, then return to Pinned: <pull cause>`, since "edit the manifest" is misleading under `Pinned`, where no version can be named; T6 replaces the second clause with the bump wording. Both pinned failures (no longer served, no registry digest) are emitted by the backend as `image.resolve` error events so the cause renders under the resolving line before the engine's own error, as a pull failure does.
   - no pin: pull the expanded manifest reference, pick the repository digest, `Put` the pin with `PinnedAt` now, `Ref` is the digest reference, `Source` is `resolved`.
4. `Pinned` with `Repin` set: pull `Repin`, pick the digest, `Put` the pin, `Source` is `repinned`.
5. Events: `image.resolve` started with `team`, `name`, `ref`; finished with `team`, `name`, `ref`, `digest`, `source`; error with the message. `image.pull` events stay as they are. *Amended by T3 (spec 033):* finished also carries `requested` for the pinned sources and `pinned_at` (`YYYY-MM-DD`) for `pinned`.

Picking the digest: `pickRepoDigest(inspect.RepoDigests, repository)` returns the entry whose part before `@` equals the repository of the pulled reference. An empty result is an error for `Pinned` and a warning-free empty `Digest` for manifest-owned artifacts.

*Amended by T2 (spec 032), as shipped:* `pickRepoDigest` returns the `sha256:…` part after `@` of the matching entry, which is what `ResolvedImage.Digest` holds; the pullable pin of TD-1 is therefore `repositoryOf(ref) + "@" + digest`. Repositories are compared after both sides drop a leading `docker.io/` and then a leading `library/` (`normalizeRepository`), because the daemon records Docker Hub repositories without them while an expanded alias carries them. The Docker backend emits no `image.resolve` error event of its own: the failing operation's event (`registry.alias`, `image.list`, `registry.auth`, `image.pull`, `image.inspect`) and the engine's `image.resolve` error carry the message. T3 may emit an `image.resolve` error from the backend for the "pin no longer served" case.

### 4.3 Engine pre-pass

`internal/engine/engine.go`, at the top of `ExecuteDeploy`, before `CreatePlatformNetwork`:

```go
resolved, err := engine.resolveImages(set, steps)   // map[kind+"/"+name]ResolvedImage
```

One `ResolveImageOp` per step, in step order, from the step's manifest. The first error aborts the deploy; nothing else has run. `deployApplication` and `deployResource` then set `op.ResolvedRef = resolved.Ref` and `op.ImageID = resolved.ImageID`; `op.Image` stays the manifest reference (*amended by T2, spec 032*; the original text read `op.Image = resolved.Ref`). Only artifacts that will be deployed are resolved, so `deploy team` resolves one team's images.

### 4.4 Dry run

`DryRunContainerBackend` gains `Pins map[string]state.ImagePin` beside `HostPorts`, filled by `NewDryRunEngine(out, hostPorts, pins)` from `ImagePins.ListAll()` in `handler.DryRun`. `ResolveImage` prints one line and returns `ResolvedImage{Ref: op.Image, Source: ...}`:

```text
[DOCKER] ImageResolve: name=team.hello-api image=ghcr.io/me/hello-api:latest policy=Pinned -> would resolve newest and pin
[DOCKER] ImageResolve: name=team.hello-db image=postgres policy=Pinned -> pinned postgres@sha256:9c1b4d7e3f2a… (17, 2026-10-06)
[DOCKER] ImageResolve: name=team.web image=nginx:1.27 policy=IfNotPresent -> manifest-owned
```

*Amended by T3 (spec 033):* the pinned line prints the full `Pinned` reference, not the twelve-character form, because the preview is where an operator copies the exact version from; the preview shows the pin on record as is, since it expands no alias and cannot apply the repository-match rule of section 4.2.

### 4.5 Plan-time normalisation and validation

`internal/planner/plan.go`: `Plan` gains a `defaultPullPolicy string` parameter, empty meaning the derived rule, and calls `applyEffectivePullPolicy(set, defaultPullPolicy)` before `Resolve`. `internal/planner/resolve.go`: `Resolve` gains `validateImagePolicies(set, defaultPullPolicy)` after `validateRegistryImages`:

- `Pinned` and a tag other than `latest`, or a digest reference: `application "x": spec.image "repo:1.2" names a fixed version but the image pull policy is Pinned; use "repo" or "repo:latest"`.
- `Pinned` Resource with `spec.version` other than empty or `latest`: `resource "db": spec.version "16" names a fixed version but the image pull policy is Pinned (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default`. The parenthetical appears only when the policy came from the configuration default. *Amended by T3 (spec 033):* when the policy came from the manifest the message ends `; omit it or use "latest"`, because the design's second clause only makes sense for a configuration-sourced policy; T4 adds the configuration wording. A Resource `spec.image` override naming a fixed version gets the Application-shaped message; the `<type>:<version>` image the parser derives is not treated as an override, so one mistake is reported once. The two helpers live in a new `internal/planner/policy.go` beside `resolve.go`.
- Not `Pinned` and Resource `spec.version` empty: `resource "db": spec.version is required`, the text parse-time validation uses today.

*Amended by T4 (spec 034):* `applyEffectivePullPolicy` records on the `ManifestSet` which precedence layer supplied each artifact's policy, and `validateImagePolicies(set)` reads that record to choose the message ending; the `defaultPullPolicy` parameter is removed from `validateImagePolicies` and `Resolve`, since `Plan` is the only place the default enters. The configuration-sourced Application message ends `(from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default`, like the Resource message; the manifest-sourced endings are T3's.

`internal/manifest/parser.go`: the Resource image default becomes `<type>:<version>` when a version is set and `<type>` when it is not. `internal/manifest/validate.go`: the `spec.version is required` check is removed from `validateResourceSpec`; the enum check for `spec.imagePullPolicy` accepts the third value.

### 4.6 Bump

`cmd/bump.go`: `bump` with subcommands `application` (alias `app`) and `resource` (alias `res`); flags `-v/--version`, `-t/--team`, `--dry-run`, `-p/--path`. Thin dispatcher to `handler.Bump(bundle, handler.BumpOptions{Kind, Name, Team, Version, DryRun})`.

`internal/app/app.go`: `BuildBumpBundle(cfg, store, paths, manifestDir, out)` resolves the specs directory, builds the terminal observer, and a container backend through `newContainerBackend`; no engine, no vault, no routing.

`internal/handler/bump.go`:

1. `planner.LoadDir`, then `applyEffectivePullPolicy` with `cfg.ImagePullPolicy`, then find the manifest by kind and name, using `--team` against `metadata.owner` when several match.
2. Refuse when the effective policy is not `Pinned`: `application "x": its version is manifest-owned (imagePullPolicy IfNotPresent); edit the manifest to change it`.
3. Build the target from the manifest image with its tag removed: `-v 17` gives `postgres:17`; `-v sha256:…` gives `postgres@sha256:…`; no `-v` gives the manifest reference itself. Validate `-v` as a tag (`[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}`) or `sha256:` plus 64 hex characters.
4. Read the current pin for the "previous" line, then `backend.ResolveImage(ResolveImageOp{..., ImagePullPolicy: Pinned, Repin: target})`.
5. Print `Bumped team/db: 16@aaaa… -> 17@bbbb…; run "shrine deploy" to apply` or, with no previous pin, `Pinned team/db at 17@bbbb…`. With `--dry-run`, print `[dry-run] would resolve postgres:17 and pin team/db` and call nothing.

### 4.7 Queries

- `internal/handler/deployments.go`: `printDeploymentsTable` adds a VERSION column after KIND. T1 fills it with `Deployment.Image`, or `-` when empty. T5 changes the rule: when `Deployment.Policy` is `Pinned`, read the pin and print the readable form of section 3.5; otherwise `Deployment.Image`.
- `printDeploymentDetail` adds `Image:` and `Pull policy:` in T1; T5 adds `Pinned:` with the full digest reference and the date, and `Running image:` from `ContainerInfo.Image`. `describe` gains a container backend through `NewQueryContainerBackend`; when Docker is unreachable the running image prints as unavailable and the command still succeeds.
- `internal/handler/status.go`: `containerStatusRow` gains `Image`; the table gains an IMAGE column; IMAGE ID stays.

### 4.8 Delete

- `DeleteApplication` releases the pin after the host port. `DeleteTeam` calls `ReleaseTeam` after releasing host ports. Both in T3.
- `DeleteResource` in T7 is `DeleteApplication` generalised over the kind: no host port, same refusal while the container exists, record and pin released, `--team` and `--dry-run`.

### 4.9 Events and rendering

New event names, each with a `case` in `internal/ui/terminal_logger.go` and nothing new in the file logger:

| Event | Status | Rendered as |
|---|---|---|
| `image.resolve` | started | `🔎 Resolving image for team.name (ref)` |
| `image.resolve` | finished, source `resolved` | `📌 Pinned team.name at latest@3f2a9c1b4d7e` |
| `image.resolve` | finished, source `pinned` | `📌 Using pinned team.name 17@9c1b4d7e3f2a (since 2026-10-06)` |
| `image.resolve` | finished, source `repinned` | `📌 Bumped team.name to 17@9c1b4d7e3f2a` |
| `image.resolve` | finished, source `manifest` | `🔎 Resolved team.name nginx:1.27@a1b2c3d4e5f6` |
| `image.resolve` | error | the generic `❌ Error [image.resolve]` line |

Exact strings are fixed by each ticket's contract; the table fixes the shape.

### 4.10 Generate

`handler.AppOptions` and `handler.ResourceOptions` gain `PullPolicy string`, the effective default from `cfg.ImagePullPolicy`. Under `Pinned` the app image defaults to `<name>` and the resource skeleton omits the `version:` line; otherwise the templates are unchanged. No `imagePullPolicy:` line is written, so the manifest keeps following the configuration default. *Amended by T4 (spec 034):* the image default moves from `cmd/generate.go` into `handler.GenerateApp`, and the `--version` flag default becomes empty with `handler.GenerateResource` filling `16` unless the default is `Pinned`; an explicit `--image` or `--version` is written verbatim under every default. Rendering is split into `renderAppSkeleton` and `renderResourceSkeleton` so unit tests stay off the filesystem.

### 4.11 Documentation touch points

- `docs/content/reference/manifest-schema.md`: the two `spec.imagePullPolicy` rows and both YAML blocks list three values; the Resource `spec.version` row becomes "yes, unless the policy is Pinned"; a new subsection "Image pull policy" states the rule and the pin lifecycle.
- Configuration: there is no configuration reference page today; the guides embed fragments of `config.yml`. T4 adds the key to the page where `specsDir` is introduced for operators and the ticket spec names it.
- CLI pages: generated by `make docs-gen-cli` into `docs/content/cli/`, so `bump_application.md`, `bump_resource.md`, and `delete_resource.md` come from the Cobra definitions.
- `AGENTS.md`: the deploy pipeline diagram gains `Container.ResolveImage()` as a pre-pass before `CreatePlatformNetwork()`; the state layout gains `pins.txt` and the two new `deployments.txt` fields; the config layout gains `imagePullPolicy`; the CLI reference gains bump and delete resource.

## 5. Integration test fixture for the pinned policy

The core scenario is: deploy, move the registry's `latest` to a different image, redeploy, assert the same digest. Public registries cannot be moved, so T3's suite starts a registry container:

- `testutils.StartLocalRegistry(tc)` runs `registry:2` bound to `127.0.0.1:0`, records the port, and removes the container in `AfterEach`. Docker treats `127.0.0.1` registries as insecure by default, so the daemon needs no configuration and the config file needs no `registries` entry.
- `testutils.PushAs(tc, source, ref)` tags a small public image already present on the runner, `alpine:3.19` and `alpine:3.20` serve, as `127.0.0.1:<port>/shrine/app:latest` and pushes it. Moving `latest` is a second `PushAs` with the other source. *Amended by T3 (spec 033), as shipped:* the sources are `traefik/whoami:v1.10.1` and `v1.10.2`, pushed as `shrine/whoami:latest`, because an alpine container exits at once and fails the running assertions; `PushAs` is a method on the `LocalRegistry` that `StartLocalRegistry` returns and it returns the pushed digest; the pinned manifests are written at run time by `WritePinnedFixture` (the port is only known then) and only the validation fixtures live under `tests/testdata/pinned/`.
- `testutils.AssertContainerImageDigest(name, digest)` inspects the container's image and compares `RepoDigests`. *Amended by T3 (spec 033):* not needed; every pinned container is created from the digest reference, so the existing `AssertContainerImage` on `Config.Image` plus an image-id comparison prove the same thing.
- The "wiped cache" cycle is `docker image rm` of the pinned digest on the runner between deploys.

The fixtures under `tests/testdata/pinned/` are manifests with `imagePullPolicy: Pinned` whose image is `127.0.0.1:<port>/shrine/app`; the port is substituted at test time the way other suites substitute the state directory.

## 6. PRD amendments this design asks for

- **R-11 and R-12.** Replace "kept, inert" with: a deploy under a manifest-owned policy releases the artifact's pin; returning to the pinned policy is a first deploy. Releases therefore happen on delete and on a manifest-owned deploy, never on redeploy, recreation, or teardown. Reason: TD-6, a stale pin has no reader and would surprise on return.
- **R-17 and T1.** The deployment record stores the policy as well as the image, so T1's record format changes once, not twice.
- **M2.** As already noted in the ticket breakdown: existing output lines unchanged; new lines may be added.

## 7. Open technical points to settle inside T3's spec

- The repository form Docker writes into `RepoDigests` for official Docker Hub images, `postgres@sha256:…` versus `docker.io/library/postgres@sha256:…`, decides how `pickRepoDigest` compares repositories. Verify against the daemon in the suite. *Settled by T2:* both forms match, because `normalizeRepository` strips `docker.io/` and `library/` from both sides before comparing; T2's integration suite asserts a digest on Docker Hub images.
- Whether `RepoDigests` after a tag pull of a multi-architecture image carries the index digest or the platform manifest digest. Either pins correctly on the same host; the PRD's architecture-move note depends on the former.
- Whether `ImageInspect` accepts a digest reference for the local presence check on every daemon version the project supports, or whether `ImageList` with a `reference` filter is needed.

---

## Per-ticket requirements with their technical binding

Each ticket's spec starts from these. Identifiers are `T<n>-<nn>`; the PRD requirement each serves is in brackets.

### T1. Deployed version in get and describe

- **T1-01** [R-17] Deploy records the manifest's image reference and the effective pull policy with each deployment. Where: `state.Deployment` gains `Image` and `Policy` (section 3.3); `recordDeployment` in `docker_container.go` fills them from the op; `CreateContainer` keeps the unexpanded reference in a local before `op.Image = expanded`, because the record stores the reference as written. *After T2 (spec 032)* `op.Image` is never overwritten on either path, so the record reads it directly; the expanded form lives in `op.ResolvedRef`.
- **T1-02** [R-17] `deployments.txt` carries the two new fields and legacy lines still load. Where: `loadTeam` and `saveTeam` in `internal/state/local/deployments.go`; the reader rule of section 3.3.
- **T1-03** [R-17] `get deployed`, `get applications`, `get resources` print a VERSION column, state only. Where: `printDeploymentsTable`; the `cmd/get.go` commands are untouched.
- **T1-04** [R-18] `describe app` and `describe resource` print `Image:` and `Pull policy:`. Where: `printDeploymentDetail`.
- **T1-05** [R-17] A record without the fields shows `-` and the next deploy fills it. Where: the reader rule plus `recordDeployment` on every successful deploy, which already runs on the up-to-date path through `ensureRunning`.

### T2. Resolve every image before touching any container

- **T2-01** [R-14] The backend contract gains `ResolveImage`. Where: `internal/engine/backends.go`, section 4.1; implemented in `docker_image.go` for `Always` and `IfNotPresent` only, returning `Ref`, `Digest`, `ImageID`, `Source: manifest`.
- **T2-02** [R-14] The engine resolves every step's image before `CreatePlatformNetwork` and aborts on the first failure. Where: `resolveImages` in `engine.go`, section 4.3.
- **T2-03** [R-14] `CreateContainer` skips resolution when the op carries `ImageID`. Where: `CreateContainerOp.ImageID`; `CreateContainer` in `docker_container.go`; the Traefik plugin path is unchanged.
- **T2-04** [R-15] Deploy output prints the exact version per artifact. Where: `image.resolve` events in the backend; two new `case` arms in `terminal_logger.go` for started and finished with source `manifest`.
- **T2-05** [R-16] Dry run prints the resolution step. Where: `DryRunContainerBackend.ResolveImage`, section 4.4, the `manifest-owned` line only.
- **T2-06** [R-32] The deploy pipeline diagram in `AGENTS.md` shows the pre-pass.
- **T2-07** [M2] Pull semantics are unchanged: `Always` pulls, `IfNotPresent` reuses a local image. Where: the moved code keeps today's `ImageList` lookup.

### T3. The pinned policy

- **T3-01** [R-01] The manifest accepts `Pinned`. Where: `manifest.ImagePullPolicyPinned`; the enum check in `validate.go`.
- **T3-02** [R-02, R-03] Plan-time normalisation and the no-fixed-version and version-required rules. Where: `applyEffectivePullPolicy` and `validateImagePolicies` in the planner, section 4.5; `Plan` gains the `defaultPullPolicy` parameter, threaded as empty from `handler.Deploy`, `handler.DryRun`, `handler.ApplySingle`; `parseManifest` defaults the Resource image to `<type>` when no version is set; `validateResourceSpec` drops the version check.
- **T3-03** [R-09, R-10, R-13] First deploy pins; later deploys use the pin; local presence skips the registry; absence pulls by digest. Where: `ResolveImage` cases 3 in section 4.2; `pickRepoDigest`.
- **T3-04** [R-11, R-12] The pin store, released by `delete application`, `delete team`, and a manifest-owned deploy. Where: `internal/state/pins.go`, `internal/state/local/pins.go`, `state.Store.ImagePins`, `NewLocalStore`; `DeleteApplication`, `DeleteTeam`; `ResolveImage` case 2.
- **T3-05** [R-14] A pin the registry no longer serves fails in the pre-pass with the message of section 4.2, wording "edit the manifest" until T6.
- **T3-06** [R-15] Output distinguishes pinned now, reused, manifest-owned. Where: `image.resolve` finished with `source`; the `resolved` and `pinned` arms in `terminal_logger.go`.
- **T3-07** [R-16] Dry run previews pins without writing. Where: `Pins` snapshot on the dry-run backend, `ListAll`, `NewDryRunEngine` signature, `handler.DryRun`.
- **T3-08** [TD-2] The config hash input stays the image id, so a reused pin never recreates a container and a bump always does.
- **T3-09** [R-28, R-32] Manifest reference and `AGENTS.md` state layout. Where: section 4.11.
- **T3-10** [TD-13] The local registry fixture and helpers of section 5.

### T4. Configuration default and generated manifests

- **T4-01** [R-06] `config.Config.ImagePullPolicy`, validated in `Load`. Where: section 3.2.
- **T4-02** [R-04] Precedence manifest, config, derived. Where: `cfg.ImagePullPolicy` passed as `defaultPullPolicy` from the three handlers into `Plan`; `applyEffectivePullPolicy` already implements the order.
- **T4-03** [R-07] The configuration-sourced message. Where: `validateImagePolicies` knows whether the policy came from the default; message in section 4.5.
- **T4-04** [R-08] Generate follows the default. Where: section 4.10.
- **T4-05** [R-29] Configuration documentation and `AGENTS.md` config layout.

### T5. Pinned versions in get, describe, and status

- **T5-01** [R-17] VERSION shows the readable pin for `Policy == Pinned`. Where: `printDeploymentsTable` reads `ImagePins.Get`; section 3.5.
- **T5-02** [R-18] `describe` shows policy, pin with date, and running image. Where: `printDeploymentDetail`; `cmd/describe.go` builds a query backend; `ContainerInfo.Image` from `docker_status.go`; the unavailable-Docker rule of section 4.7.
- **T5-03** [R-19] Nothing is shown for artifacts without a deployment record. Where: the tables are driven by records, so no code is needed beyond not listing pins on their own.
- **T5-04** [R-20] `status` gains IMAGE. Where: `containerStatusRow`, `printStatusTable`, `inspectDeployments`.

### T6. shrine bump

- **T6-01** [R-21] The command and its flags. Where: `cmd/bump.go`; `BuildBumpBundle`; section 4.6.
- **T6-02** [R-05, R-24] Refusal rules and lookup by kind, name, team in the loaded set.
- **T6-03** [R-22] Target construction and `-v` validation.
- **T6-04** [R-23] Immediate resolution through `ResolveImage` with `Repin`; no container touched; previous and new printed. Where: `ResolveImage` case 4; the `repinned` arm in `terminal_logger.go`.
- **T6-05** [R-25] Dry run prints and calls no backend.
- **T6-06** [R-14] The T3 message now points at bump.
- **T6-07** [R-30] CLI pages regenerated; `AGENTS.md` CLI reference.

### T7. shrine delete resource and pin release on every delete

- **T7-01** [R-27] `DeleteResource` as the kind-generalised `DeleteApplication`. Where: `internal/handler/deployments.go`; `cmd/delete.go`.
- **T7-02** [R-11] Every delete verb releases pins, asserted end to end.
- **T7-03** [R-30] CLI page; `AGENTS.md` CLI reference.

### T8. Operator guide

- **T8-01** [R-31] `docs/content/guides/image-versions.md` from captured output, linked from the guides index, the manifest reference, and troubleshooting.
- **T8-02** [R-28, R-29] Vocabulary pass over the pages T3 to T7 wrote, against section 5 of the PRD.
