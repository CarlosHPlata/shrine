# Contract: handler signatures, backend result, and shared helpers

**Feature**: 035-pinned-version-queries

## `internal/manifest` (image reference helpers)

```go
func ReadableVersion(requested, digest string) string // moved from internal/ui (readableVersion), exported, behaviour identical
func ShortDigest(digest string) string                 // moved from internal/ui (shortDigest), exported, behaviour identical
func DigestOf(ref string) string                       // new: "sha256:…" after "@", or "" when absent
```

Unit tests in `internal/manifest`: the table of data-model section 1, plus `DigestOf` on a tag reference, a digest reference, and an empty string. `internal/ui/terminal_logger_test.go` keeps asserting the rendered `📌` lines byte for byte, which proves the move changed nothing.

## `internal/engine`

```go
type ContainerInfo struct {
	Running bool
	Status  string
	ImageID string
	Image   string // new
}
```

`internal/engine/local/dockercontainer/docker_status.go` fills `Image` from `resp.Config.Image` when `resp.Config != nil`. No other backend method changes; the `ContainerBackend` interface is unchanged.

## `internal/handler`

```go
// deployments.go
func ListApplications(team string, store *state.Store) error   // unchanged signature; loads pins once
func ListResources(team string, store *state.Store) error      // unchanged signature; loads pins once
func ListDeployed(team string, store *state.Store) error       // unchanged signature; loads pins once
func DescribeApplication(team, name string, store *state.Store, backend engine.ContainerBackend) error // + backend, nil tolerated
func DescribeResource(team, name string, store *state.Store, backend engine.ContainerBackend) error    // + backend, nil tolerated

// unexported, pure, unit-tested without the filesystem
func formatDeploymentsTable(deployments []teamedDeployment, pins map[string]state.ImagePin) string
func versionCell(team string, d state.Deployment, pins map[string]state.ImagePin) string
func formatDeploymentDetail(d deploymentDetail) string
func runningImage(backend engine.ContainerBackend, containerID string) string
func loadImagePins(store *state.Store) (map[string]state.ImagePin, error)

// status.go
func formatStatusTable(rows []containerStatusRow) string
func shortImageReference(ref string) string
```

Rules the unit tests pin:

- `formatDeploymentsTable`: a `Pinned` record with a pin prints the readable form; without a pin, the recorded reference; a non-`Pinned` record ignores a pin that happens to exist under its name; a pin of the other kind is ignored; columns and separator unchanged (existing tests keep passing with `nil` pins).
- `formatDeploymentDetail`: `Pinned:` present only for a `Pinned` record, with the full reference, the readable form, and the UTC date; `-` when the pin is absent; `Running image:` always present, after `Pull policy:` (and after `Pinned:` when shown), before `Container ID:`.
- `runningImage`: nil backend, erroring backend, empty image, and a reference.
- `describeDeployment`: a nil `store.ImagePins` and `ErrImagePinNotFound` both yield `Pinned:       -`; the not-found and ambiguity errors are unchanged; the command succeeds with a backend whose `InspectContainer` fails.
- `formatStatusTable` and `shortImageReference`: the IMAGE column between STATUS and IMAGE ID, a digest reference shortened, a tag reference as is, empty as `-`, separator as long as the header.
- `StatusApplication` / `StatusResource` (existing `TestStatusAutoTeam`): unchanged expectations; the mock gains an `Image`.

## `cmd`

```go
// cmd/describe.go: app and resource subcommands
backend, err := app.NewQueryContainerBackend(cfg, store)
if err != nil {
	return err
}
return handler.DescribeApplication(team, args[0], store, backend)
```

`describe team` is untouched. `cmd/status.go` and `cmd/get.go` are untouched apart from the `Long` help text of the status commands.

## `internal/app`

`NewQueryContainerBackend` is reused as is; no new constructor.

## State stores

Read only: `store.Deployments.List`, `store.Teams.ListTeams`, `store.ImagePins.Get`, `store.ImagePins.ListAll`. No call to any `Put`, `Record`, `Release`, or `Remove` in the code this feature adds.
