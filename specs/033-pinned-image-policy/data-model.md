# Data Model: The Pinned Policy: Resolve Once, Keep the Exact Version

**Feature**: 033-pinned-image-policy | **Date**: 2026-10-06

One new recorded entity (the pin), one new value for an existing field (the policy), one in-memory extension of the T2 backend result, and the projections that carry them.

## 1. Manifest (`internal/manifest`)

### 1.1 Policy value — MODIFIED

```go
const (
	ImagePullPolicyAlways       = "Always"
	ImagePullPolicyIfNotPresent = "IfNotPresent"
	ImagePullPolicyPinned       = "Pinned" // NEW
)

func IsKnownPullPolicy(policy string) bool          // NEW: one of the three
func IsManifestOwnedPolicy(policy string) bool      // NEW: Always or IfNotPresent
func EffectivePullPolicy(image, declared string) string                 // unchanged signature; uses TagOf
func EffectivePullPolicyWithDefault(image, declared, dflt string) string // NEW: declared, else dflt, else derived
func TagOf(ref string) string                       // NEW: "" when untagged; a colon counts only after the last slash
func IsDigestReference(ref string) bool             // NEW: contains "@"
```

Validation (`validate.go`): both kinds reject a non-empty policy that is not one of the three with `spec.imagePullPolicy must be one of Always, IfNotPresent, Pinned`. `validateResourceSpec` no longer requires `spec.version`.

Parsing (`parser.go`), Resource case: `spec.image` defaults to `<type>:<version>` when a version is set and to `<type>` when it is not.

### 1.2 Fixed-version rule (planner, after normalisation)

| Kind | Field | Under `Pinned`, invalid when | Error |
|---|---|---|---|
| Application | `spec.image` | `TagOf(image)` is neither `""` nor `latest`, or `IsDigestReference(image)` | `application "x": spec.image "<image>" names a fixed version but the image pull policy is Pinned; use "<repo>" or "<repo>:latest"` |
| Resource | `spec.version` | not `""` and not `latest` | `resource "db": spec.version "<v>" names a fixed version but the image pull policy is Pinned; omit it or use "latest"` |
| Resource | `spec.image` (override) | same rule as Application | `resource "db": spec.image "<image>" names a fixed version but the image pull policy is Pinned; use "<repo>" or "<repo>:latest"` |
| Resource | `spec.version` | policy not `Pinned` and version `""` | `resource "db": spec.version is required` |

`<repo>` is the image with its tag or digest removed.

## 2. Pin record (`internal/state/pins.go`) — NEW

```go
var ErrImagePinNotFound = errors.New("image pin not found")

type ImagePin struct {
	Kind      string    // manifest.ApplicationKind | manifest.ResourceKind
	Name      string
	Requested string    // the expanded tag reference the pin was resolved from: 127.0.0.1:5000/shrine/whoami:latest, postgres
	Pinned    string    // the pullable exact version: <repository>@sha256:<64 hex>
	PinnedAt  time.Time // UTC, second precision
}

func ImagePinKey(team, name string) string // "team/name"

type ImagePinStore interface {
	Get(team, name string) (ImagePin, error)   // ErrImagePinNotFound when absent
	Put(team string, pin ImagePin) error        // insert or replace by name
	Release(team, name string) error            // idempotent
	ReleaseTeam(team string) error              // idempotent
	List(team string) ([]ImagePin, error)       // sorted by name
	ListAll() (map[string]ImagePin, error)      // keyed ImagePinKey; the dry-run snapshot
}
```

Invariants: `Requested` and `Pinned` contain no spaces and share a repository; `Pinned` always carries `@sha256:`; `PinnedAt` is never zero in a stored record.

### 2.1 File (`internal/state/local/pins.go`) — NEW

`<state-dir>/<team>/pins.txt`, one line per artifact, five space-separated fields, sorted by name, written with `writeTeamFile` (creates the team directory, atomic temp-and-rename):

```text
Application whoami-pinned 127.0.0.1:5000/shrine/whoami:latest 127.0.0.1:5000/shrine/whoami@sha256:3f2a9c1b… 2026-10-06T10:42:17Z
Resource    cache-pinned  127.0.0.1:5000/shrine/whoami        127.0.0.1:5000/shrine/whoami@sha256:3f2a9c1b… 2026-10-06T10:42:17Z
```

Reader rule: split on whitespace; skip blank lines, lines after `#`, lines with fewer than five fields, lines whose fifth field is not RFC 3339, and lines whose fourth field lacks `@sha256:`. A missing file is an empty team. `ImagePinStore` is constructed by `NewImagePinStore(baseDir)` and, for unit tests, `newImagePinStoreWithFileOps(baseDir, read, write)`.

### 2.2 Aggregate

`state.Store` gains `ImagePins ImagePinStore`; `local.NewLocalStore` constructs it. Handlers nil-check it as they nil-check `HostPorts`.

### 2.3 Lifecycle

| Transition | Actor | Trigger |
|---|---|---|
| absent → present | Docker backend `ResolveImage` | first `Pinned` resolution, or a pin whose repository no longer matches the manifest |
| present → present (same) | none | redeploy, container recreation, teardown, dry run |
| present → absent | Docker backend `ResolveImage` | the artifact resolves under `Always` or `IfNotPresent` |
| present → absent | `handler.DeleteApplication` | `shrine delete application` (not under `--dry-run`) |
| present → absent (all of team) | `handler.DeleteTeam` | `shrine delete team` |

## 3. Backend contract (`internal/engine/backends.go`) — MODIFIED

```go
type ResolvedImage struct {
	Ref       string    // the container is created from this: expanded tag reference (manifest) or <repo>@sha256:… (pinned)
	Digest    string    // sha256:…; never empty for a pinned source
	ImageID   string
	Source    string    // ImageSourceManifest | ImageSourceResolved | ImageSourcePinned
	Requested string    // NEW: the expanded tag reference a pin was resolved from; empty for manifest
	PinnedAt  time.Time // NEW: zero unless Source is pinned
}

const (
	ImageSourceManifest = "manifest"
	ImageSourceResolved = "resolved" // NEW: pinned on this deploy
	ImageSourcePinned   = "pinned"   // NEW: reused from an existing pin
)
```

`ResolveImageOp` is unchanged (`Repin` is T6's). `ContainerBackend` is unchanged. The engine keeps projecting `Ref` to `op.ResolvedRef` and `ImageID` to `op.ImageID`; under `Pinned`, `op.ImagePullPolicy` already reads `Pinned` after plan-time normalisation and lands in the deployment record's `Policy`.

## 4. Docker backend (`internal/engine/local/dockercontainer`)

| Function | Signature | Behaviour |
|---|---|---|
| `ResolveImage` | unchanged | after alias expansion and the started event, branches: `resolveManifestOwned`, `reusePin`, `pinNewest` |
| `resolveManifestOwned` | `(ctx, op, ref) (ResolvedImage, error)` | today's path, then `releasePin(op)`; `Source: manifest` |
| `releasePin` | `(op) error` | `state.ImagePins.Release(op.Team, op.Name)`; nil store is a no-op |
| `usablePin` | `(op, ref) (state.ImagePin, bool, error)` | `Get`; found and `sameRepository(pin.Requested, ref)` |
| `reusePin` | `(ctx, op, pin) (ResolvedImage, error)` | `inspectImage(pin.Pinned)`; not-found → `pullImage(pin.Pinned)` wrapped by `notServedError` on failure, then inspect; `Source: pinned`, `Requested: pin.Requested`, `PinnedAt` |
| `pinNewest` | `(ctx, op, ref) (ResolvedImage, error)` | `pullImage(ref)`, `inspectImage(ref)`, `pickRepoDigest`; empty digest → error; `Put` with `now()`; `Ref: <repo>@<digest>`, `Source: resolved`, `Requested: ref` |
| `notServedError` | `(op, pin, cause) error` | the R8 message; emitted as an `image.resolve` error event |
| `now` | field `func() time.Time` | `time.Now().UTC` in production; injected in tests |

`repositoryOf` and `normalizeRepository` stay where they are; `sameRepository(a, b)` is a one-line helper over them.

## 5. Planner (`internal/planner`)

| Function | Signature | Behaviour |
|---|---|---|
| `Plan` | `(set, store, registries, ports, filter, defaultPullPolicy string) PlanResult` | calls `applyEffectivePullPolicy` first; passes `defaultPullPolicy` to `Resolve` |
| `Resolve` | `(set, store, registries, defaultPullPolicy string) []error` | appends `validateImagePolicies(set, defaultPullPolicy)` after `validateRegistryImages` |
| `applyEffectivePullPolicy` | `(set, defaultPullPolicy)` | writes `EffectivePullPolicyWithDefault(image, declared, dflt)` into every manifest's `Spec.ImagePullPolicy` |
| `validateImagePolicies` | `(set, defaultPullPolicy) []error` | the table of section 1.2; `defaultPullPolicy` is received but unused in this ticket, so T4 can tell a configuration-sourced policy apart (design section 4.5) without a signature change |

Handlers (`Deploy`, `DryRun`, `ApplySingle`) pass `""`; T4 passes the configuration value.

## 6. Dry run (`internal/engine/dryrun`)

`DryRunContainerBackend` gains `Pins map[string]state.ImagePin`; `NewDryRunEngine(out, hostPorts, pins)` fills it; `handler.DryRun` passes `ListAll()` or an empty map. `ResolveImage` returns `ResolvedImage{Ref: op.Image, Source: ...}` after printing the line of the operator-output contract.

## 7. Events (`engine.Event`)

| Name | Status | Emitter | Fields |
|---|---|---|---|
| `image.resolve` | finished, `source=resolved` | Docker backend | `team`, `name`, `ref` (digest reference), `digest`, `requested`, `source` |
| `image.resolve` | finished, `source=pinned` | Docker backend | as above plus `pinned_at` (`2006-01-02`) |
| `image.resolve` | finished, `source=manifest` | Docker backend | unchanged |
| `image.resolve` | error | Docker backend, "no longer served" and "no registry digest" only | `team`, `name`, `ref`, `error` |
| `image.pull` | started / finished | unchanged; `ref` is the digest reference when pulling a pin | `ref` |

## 8. Terminal (`internal/ui/terminal_logger.go`)

Two arms under `case "image.resolve"`, finished: `source=resolved` and `source=pinned`, both using `readableVersion(requested, digest)` and `shortDigest(digest)`. `readableVersion` returns `TagOf(requested) + "@" + shortDigest(digest)`, with `latest` as the tag when the request carries none, and `shortDigest(digest)` alone when `requested` is itself a digest reference (design section 3.5; T6's digest bump produces such a request).

## 9. Handlers

- `DeleteApplication`: reads `ImagePins.Get`; dry run adds the would-release line; real path releases after the host port and before the deployment record; the "nothing to delete" condition includes the pin.
- `DeleteTeam`: `releaseTeamImagePins(store, name)` after host ports, printing the count when positive.
