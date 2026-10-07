# Data Model: `shrine bump`

**Feature**: 036-bump-command | **Date**: 2026-10-07

No recorded state changes shape. The resolution op gains one field and one source value, the composition root gains a fourth bundle, the handler gains an options struct and a small target value, and `internal/manifest` gains one reference helper that two private copies collapse into. The pin record is written through the existing store by the existing backend method.

## 1. Resolution op and result (`internal/engine`) — MODIFIED

```go
type ResolveImageOp struct {
	Team            string
	Name            string
	Kind            string
	Image           string // the manifest reference, as written (alias form allowed)
	ImagePullPolicy string
	Repin           string // NEW: the target to pin instead of op.Image; empty on every deploy path
}

const (
	ImageSourceManifest = "manifest"
	ImageSourceResolved = "resolved"
	ImageSourcePinned   = "pinned"
	ImageSourceRepinned = "repinned" // NEW
)
```

`ResolvedImage` is unchanged. For a repin: `Ref` = the pullable exact version, `Digest` = `sha256:…`, `ImageID` = the local image id, `Source` = `repinned`, `Requested` = the expanded target.

Branches of `DockerBackend.ResolveImage`, in order:

| `ImagePullPolicy` | `Repin` | Branch | Pin write |
|---|---|---|---|
| `Always` / `IfNotPresent` | empty | `resolveManifestOwned` (T2), releases a pin (T3) | release |
| `Always` / `IfNotPresent` | set | error `repin of <team>/<name> requires the Pinned policy, got <policy>` | none |
| `Pinned` | empty | `resolvePinned`: reuse (T3) or `pinReference(…, ImageSourceResolved)` | first deploy only |
| `Pinned` | set | `pinReference(ctx, op, expand(Repin), ImageSourceRepinned)` | always, after the pull and inspect succeed |

## 2. Pin record (`internal/state`) — UNCHANGED shape, new writer path

`ImagePin{Kind, Name, Requested, Pinned, PinnedAt}`; the file format of design section 3.4 is unchanged. After a bump:

| Bump | `Requested` | `Pinned` |
|---|---|---|
| `-v v2` | `<host>/shrine/whoami:v2` | `<host>/shrine/whoami@sha256:<64 hex>` |
| `-v sha256:<64 hex>` | `<host>/shrine/whoami@sha256:<64 hex>` | the same reference |
| no `-v` | `<host>/shrine/whoami` (the manifest reference, expanded) | `<host>/shrine/whoami@sha256:<64 hex>` |

`Requested` is stored expanded, as T3 does; the readable form follows design section 3.5: `v2@<12 hex>`, `<12 hex>` alone for a digest request, `latest@<12 hex>` for an untagged request.

## 3. Reference helper (`internal/manifest`) — NEW

```go
// RepositoryOf returns the reference without its version: a digest suffix
// is cut first, then a tag after the last slash. NEW; replaces
// dockercontainer.repositoryOf and planner.repositoryWithoutVersion.
func RepositoryOf(ref string) string
```

| `ref` | `RepositoryOf` |
|---|---|
| `postgres:17` | `postgres` |
| `postgres` | `postgres` |
| `127.0.0.1:5000/shrine/whoami:v2` | `127.0.0.1:5000/shrine/whoami` |
| `127.0.0.1:5000/shrine/whoami` | `127.0.0.1:5000/shrine/whoami` |
| `ghcr.io/me/app@sha256:…` | `ghcr.io/me/app` |
| `reg:lab/hello-api:1.2` | `reg:lab/hello-api` |

## 4. Bump bundle (`internal/app`) — NEW

```go
type BumpBundle struct {
	Out              io.Writer
	ErrOut           io.Writer
	Cfg              *config.Config
	Store            *state.Store
	Paths            *config.Paths
	SpecsDir         string
	Observer         engine.Observer
	ContainerBackend engine.ContainerBackend
}

func BuildBumpBundle(cfg *config.Config, store *state.Store, paths *config.Paths, manifestDir string, out, errOut io.Writer) (*BumpBundle, func() error, error)
```

Construction order and failure prefixes: `validating registries: …`, the specs-dir resolution error as is, `observer: …`, `container backend: …` (the file logger is closed on this failure). No vault, no Traefik plugin, no routing, no engine. Cleanup closes the file logger.

## 5. Handler values (`internal/handler/bump.go`) — NEW

```go
type BumpOptions struct {
	Kind    string // manifest.ApplicationKind or manifest.ResourceKind
	Name    string
	Team    string // optional; verified against metadata.owner
	Version string // "" means the manifest reference (newest)
}

// bumpTarget is everything known once the artifact is found, the policy
// checked, and the target built; both the real and the dry-run path end here.
type bumpTarget struct {
	Team          string
	Kind          string
	Name          string
	ManifestImage string // the manifest reference, as written
	Target        string // the reference to resolve: repo:tag, repo@sha256:…, or ManifestImage
}
```

`prepareBump` state machine, in order; every step before the last records nothing and contacts no registry:

| Step | Input | Failure |
|---|---|---|
| validate `-v` | `opts.Version` | `invalid version "<v>": use a tag (…) or an exact version "sha256:<64 hex>"` |
| load | `manifestDir` | `LoadDir`'s parse or validation error, as deploy |
| find | `set`, kind, name | `<kind> "<name>": no manifest found in <dir>` |
| team | `metadata.owner`, `opts.Team` | `<kind> "<name>" not found in team "<t>" (its manifest in <dir> is owned by "<owner>")` |
| plan | `planManifestSet(errOut, set, store, cfg, ByApp/ByResource(name))` | `Validation errors:` on `errOut`, then `Spec validation errors`; or the planner's `Error` |
| policy | effective `Spec.ImagePullPolicy` | `<kind> "<name>": its version is manifest-owned (imagePullPolicy <policy>); edit the manifest to change it` |
| target | image, version | none (already validated) |

Then, real path only: `previousPin` (read), `ContainerBackend.ResolveImage(repinOp(target))` (the only write), `formatBumpResult`.

## 6. Deploy planning helper (`internal/handler/deploy.go`) — EXTRACTED

```go
// planManifestSet plans a loaded set the way every manifest-driven command
// does: port context, Plan, planner error, validation errors to errOut.
func planManifestSet(errOut io.Writer, set *planner.ManifestSet, store *state.Store, cfg *config.Config, filter planner.Filter) (planner.PlanResult, error)
```

`Deploy` and `DryRun` keep their observable behaviour (same lines on `errOut`, same `Spec validation errors` error, `No steps generated.` still printed by the callers).

## 7. Terminal rendering (`internal/ui`) — MODIFIED

| Event | Status | Fields used | Rendered |
|---|---|---|---|
| `image.resolve` | finished, `source=repinned` | `team`, `name`, `requested`, `digest` | `  📌 Bumped <team>.<name> to <ReadableVersion(requested, digest)>` |

The three existing arms are unchanged. The file logger needs nothing new.
