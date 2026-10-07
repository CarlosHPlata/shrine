# Data Model: Pinned Versions in `get`, `describe`, and `status`

**Feature**: 035-pinned-version-queries | **Date**: 2026-10-07

No recorded state changes shape. One backend result type gains a field, one handler row type gains a field, three string helpers move to the package that owns image references, and the two detail and table renderers take the pin and the running image as inputs. Every store is read; none is written.

## 1. Image reference helpers (`internal/manifest`) — MOVED and NEW

```go
// ReadableVersion is a pin as a person reads it: the tag it was resolved
// from and the short exact version, or the short exact version alone when
// the request was itself a digest (design section 3.5). Moved from internal/ui.
func ReadableVersion(requested, digest string) string

// ShortDigest keeps twelve hex characters (design TD-11). Moved from internal/ui.
func ShortDigest(digest string) string

// DigestOf returns the sha256:… part of a digest reference, or "" when
// the reference has none. NEW.
func DigestOf(ref string) string
```

| `requested` | `digest` | `ReadableVersion` |
|---|---|---|
| `127.0.0.1:5000/shrine/whoami:latest` | `sha256:3f2a9c1b4d7e…` | `latest@3f2a9c1b4d7e` |
| `postgres:17` | `sha256:9c1b4d7e3f2a…` | `17@9c1b4d7e3f2a` |
| `postgres` (no tag) | `sha256:9c1b…` | `latest@9c1b4d7e3f2a` |
| `postgres@sha256:9c1b…` (digest request) | `sha256:9c1b…` | `9c1b4d7e3f2a` |
| any | shorter than twelve hex | the hex as is, never padded |

`internal/ui/terminal_logger.go` keeps `exactVersion` and calls the exported helpers; its rendered strings do not change.

## 2. Backend result (`internal/engine`) — MODIFIED

```go
type ContainerInfo struct {
	Running bool
	Status  string
	ImageID string
	Image   string // NEW: the reference the container was created from (ContainerInspect(...).Config.Image)
}
```

| Backend | `Image` |
|---|---|
| `DockerBackend.InspectContainer` | `resp.Config.Image`; `""` when `Config` is nil |
| `DryRunContainerBackend.InspectContainer` | `""` (unchanged zero value) |
| handler test stubs | whatever the test sets |

For a pinned container `Image` is the digest reference T3 created it from (`repo@sha256:…`); for a manifest-owned container it is the expanded tag reference T2's `ResolvedRef` carried.

## 3. Deployment listing (`internal/handler/deployments.go`) — MODIFIED

```go
func loadImagePins(store *state.Store) (map[string]state.ImagePin, error)      // NEW: {} when store.ImagePins is nil; ListAll() otherwise
func formatDeploymentsTable(deployments []teamedDeployment, pins map[string]state.ImagePin) string // signature gains pins
func versionCell(team string, d state.Deployment, pins map[string]state.ImagePin) string          // NEW
func pinFor(team string, d state.Deployment, pins map[string]state.ImagePin) (state.ImagePin, bool) // NEW: key team/name, Kind must match
```

VERSION cell rule, in order:

| Record `Policy` | Pin for `team/name` with the record's `Kind` | VERSION |
|---|---|---|
| `Pinned` | present | `ReadableVersion(pin.Requested, DigestOf(pin.Pinned))` |
| `Pinned` | absent | `valueOrUnknown(d.Image)` (the recorded reference) |
| anything else, including `""` | ignored | `valueOrUnknown(d.Image)` (T1, unchanged) |

Column order and widths are unchanged: `TEAM NAME KIND VERSION CONTAINER ID`, `deploymentRowFormat`.

## 4. Deployment detail (`internal/handler/deployments.go`) — MODIFIED

```go
func DescribeApplication(team, name string, store *state.Store, backend engine.ContainerBackend) error // gains backend
func DescribeResource(team, name string, store *state.Store, backend engine.ContainerBackend) error    // gains backend
func describeDeployment(team, name, kind string, store *state.Store, backend engine.ContainerBackend) error

type deploymentDetail struct {               // NEW: everything the renderer needs, gathered by describeDeployment
	Team         string
	Deployment   state.Deployment
	Pin          state.ImagePin
	HasPin       bool
	RunningImage string                      // already rendered: a reference, "-", or "unavailable (…)"
}

func formatDeploymentDetail(d deploymentDetail) string   // pure; replaces formatDeploymentDetail(team, d)
func runningImage(backend engine.ContainerBackend, containerID string) string // NEW
func pinLine(d deploymentDetail) string                  // NEW: "" unless Policy is Pinned
```

Lines, in order (label column twelve characters wide, as today):

| Line | Shown when | Value |
|---|---|---|
| `Name:`, `Team:`, `Kind:` | always | unchanged |
| `Image:` | always | `valueOrUnknown(d.Image)` (T1) |
| `Pull policy:` | always | `valueOrUnknown(d.Policy)` (T1) |
| `Pinned:` | `Policy == Pinned` | `<pin.Pinned> (<ReadableVersion>, <PinnedAt UTC YYYY-MM-DD>)`, or `-` when `HasPin` is false |
| `Running image:` | always | see `runningImage` |
| `Container ID:`, `Config Hash:` | always | unchanged |

`runningImage` rule:

| Condition | Value |
|---|---|
| `backend == nil` | `unavailable (no container runtime)` |
| `InspectContainer` returns an error | `unavailable (<err.Error()>)` |
| `info.Image == ""` | `-` |
| otherwise | `info.Image`, in full |

The pin is looked up with `store.ImagePins.Get(team, name)` only when `Policy == Pinned`; `ErrImagePinNotFound` and a nil `ImagePins` both mean `HasPin == false`; any other error fails the command. A pin whose `Kind` differs from the record's is treated as absent.

## 5. Status table (`internal/handler/status.go`) — MODIFIED

```go
type containerStatusRow struct {
	Name    string
	Kind    string
	Running bool
	Status  string
	Image   string // NEW: shortImageReference(info.Image)
	ImageID string
}

func formatStatusTable(rows []containerStatusRow) string // NEW, pure; printStatusTable prints it
func shortImageReference(ref string) string               // NEW
```

Columns: `NAME(25) KIND(15) RUNNING(10) STATUS(12) IMAGE(40) IMAGE ID(19)`; the separator is as long as the header.

`shortImageReference` rule:

| `ref` | Cell |
|---|---|
| `""` | `-` |
| `repo@sha256:<64 hex>` | `repo@<first twelve hex>` |
| `repo:tag` or `repo` | as is |

## 6. Pin (`internal/state`) — UNCHANGED, read only

`ImagePin{Kind, Name, Requested, Pinned, PinnedAt}`; `ImagePinKey(team, name)`; `ImagePinStore.Get` and `ListAll`. Nothing in this feature calls `Put`, `Release`, or `ReleaseTeam`.

## 7. Deployment record (`internal/state`) — UNCHANGED, read only

`Deployment{Kind, Name, ContainerID, ConfigHash, Image, Policy}` as T1 left it. `Policy == "Pinned"` is the switch for the pinned rendering; any other value, including the empty legacy value, takes the T1 path.
