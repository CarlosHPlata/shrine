# Contract: command wiring, bundle, handler, backend, and shared helpers

**Feature**: 036-bump-command

## `cmd/bump.go` (NEW)

```go
var bumpCmd = &cobra.Command{Use: "bump", Short: "Move a pinned artifact to another version", Long: …}
var bumpApplicationCmd = &cobra.Command{Use: "application [name]", Aliases: []string{"app"}, Args: cobra.ExactArgs(1), RunE: runBump(manifest.ApplicationKind)}
var bumpResourceCmd    = &cobra.Command{Use: "resource [name]",    Aliases: []string{"res"}, Args: cobra.ExactArgs(1), RunE: runBump(manifest.ResourceKind)}

var (bumpVersion, bumpTeam, bumpPath string; bumpDryRun bool)   // persistent flags on bumpCmd: -v/--version, -t/--team, -p/--path, --dry-run

func runBump(kind string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		dir, err := cfg.ResolveSpecsDir(bumpPath)
		if err != nil { return err }
		opts := handler.BumpOptions{Kind: kind, Name: args[0], Team: bumpTeam, Version: bumpVersion}
		if bumpDryRun {
			return handler.BumpDryRun(cmd.OutOrStdout(), cmd.ErrOrStderr(), dir, store, cfg, opts)
		}
		bundle, cleanup, err := app.BuildBumpBundle(cfg, store, paths, dir, cmd.OutOrStdout(), cmd.ErrOrStderr())
		if err != nil { return err }
		defer cleanup()
		return handler.Bump(bundle, opts)
	}
}
```

Thin dispatcher: no lookup, no validation, no printing beyond Cobra's. `cmd/bump_test.go` pins the arg-count error for both subcommands and that `app` and `res` resolve to them, in-process through `cmd.SetArgs` with a `t.TempDir()` state dir, in the shape of `TestDeployTeam_RequiresArg`.

## `internal/app` (bundle)

```go
type BumpBundle struct { Out, ErrOut io.Writer; Cfg *config.Config; Store *state.Store; Paths *config.Paths; SpecsDir string; Observer engine.Observer; ContainerBackend engine.ContainerBackend }
func BuildBumpBundle(cfg *config.Config, store *state.Store, paths *config.Paths, manifestDir string, out, errOut io.Writer) (*BumpBundle, func() error, error)
```

Sequence: `cfg.ValidateRegistries()` (`validating registries: %w`) → `cfg.ResolveSpecsDir(manifestDir)` (error as is) → `newObserverPair(out, paths)` (`observer: %w`) → `newContainerBackend(store, cfg.Registries, observer)` (`container backend: %w`, file logger closed first). Returns `joinCleanup(closeObserver)`. Unit tests in `internal/app/app_test.go` over the swapped constructors: happy shape (observer pair, container backend, no vault/plugin/engine constructed), each failure prefix with the cause preserved and the log writer closed iff opened, the unresolvable-specsDir fail-fast before the logger opens.

## `internal/handler/bump.go` (NEW)

```go
type BumpOptions struct { Kind, Name, Team, Version string }

func Bump(b *app.BumpBundle, opts BumpOptions) error
func BumpDryRun(out, errOut io.Writer, manifestDir string, store *state.Store, cfg *config.Config, opts BumpOptions) error

// unexported; unit-tested without the filesystem
func validateBumpVersion(version string) error
func prepareBump(errOut io.Writer, manifestDir string, set *planner.ManifestSet, store *state.Store, cfg *config.Config, opts BumpOptions) (bumpTarget, error)
func findBumpArtifact(set *planner.ManifestSet, opts BumpOptions, manifestDir string) (meta manifest.Metadata, image string, err error)
func bumpFilter(kind, name string) planner.Filter
func effectivePolicyOf(set *planner.ManifestSet, kind, name string) string
func buildBumpTarget(image, version string) string
func repinOp(t bumpTarget) engine.ResolveImageOp
func previousPin(store *state.Store, team, name string) (state.ImagePin, bool, error)
func bumpResolved(out io.Writer, store *state.Store, backend engine.ContainerBackend, t bumpTarget) error
func formatBumpResult(t bumpTarget, previous state.ImagePin, hasPrevious bool, resolved engine.ResolvedImage) string
func formatBumpDryRun(t bumpTarget) string
```

Rules the unit tests pin:

- `validateBumpVersion`: `""`, `17`, `v1.4.0`, `latest`, `a_b.c-d`, 128 characters accepted; 129 characters, leading `.`, `-`, a space, `postgres:17`, `repo@sha256:…`, `sha256:` with 63 or 65 hex or uppercase hex rejected with the exact message.
- `findBumpArtifact`: found by kind; the other kind's map is not searched (`bump resource` of an application name is unknown); unknown names the directory; `--team` equal to the owner passes; different names both; empty `--team` never fails on owner.
- `buildBumpTarget`: `(postgres, 17)` → `postgres:17`; `(postgres, sha256:<64>)` → `postgres@sha256:<64>`; `(postgres, "")` → `postgres`; `(127.0.0.1:5000/shrine/whoami, v2)` → `127.0.0.1:5000/shrine/whoami:v2`; `(reg:lab/api, 1.2)` → `reg:lab/api:1.2`; `(postgres:latest, 17)` → `postgres:17` (the manifest's `latest` is dropped).
- `prepareBump` over an in-memory set planned with `memTeamStore`: declared `IfNotPresent` refused naming `IfNotPresent`; no field with `cfg.ImagePullPolicy = "Always"` refused naming `Always`; no field, no default, tagged image refused naming `IfNotPresent` (derived rule); declared `Pinned` passes with the right `bumpTarget`; a `Pinned` manifest naming a fixed version fails with the validation line on `errOut` and `Spec validation errors`.
- `bumpResolved` with a recording backend stub: the op is `{Team, Name, Kind, Image: ManifestImage, ImagePullPolicy: Pinned, Repin: Target}`; the previous pin is read before `ResolveImage` is called; output line with a previous pin, without one, for a digest previous; a backend error is returned as `<kind> "<name>": <err>` with `errors.Is` intact and nothing printed to `out`; a nil `store.ImagePins` means no previous.
- `formatBumpDryRun`: the exact line.

`Bump`: `validateBumpVersion(opts.Version)` → `set, err := planner.LoadDir(b.SpecsDir)` → `prepareBump(b.ErrOut, b.SpecsDir, set, b.Store, b.Cfg, opts)` → `bumpResolved(b.Out, b.Store, b.ContainerBackend, target)`. `BumpDryRun`: `validateBumpVersion` → `LoadDir` → `prepareBump` → `fmt.Fprintln(out, formatBumpDryRun(target))`. The version is validated before the directory is read, so an invalid `-v` is reported even for a directory that would fail to load; `prepareBump` works on a loaded set so its unit tests build one in memory.

## `internal/handler/deploy.go` (EXTRACTED)

```go
func planManifestSet(errOut io.Writer, set *planner.ManifestSet, store *state.Store, cfg *config.Config, filter planner.Filter) (planner.PlanResult, error)
```

Body: `buildPortContext(store, cfg)` → `planner.Plan(set, store.Teams, cfg.Registries, ports, filter, cfg.ImagePullPolicy)` → `result.Error` returned as is → validation errors printed as today and `fmt.Errorf("Spec validation errors")`. `Deploy` and `DryRun` call it after `LoadDir` and keep `No steps generated.` and the rest. No behaviour change; `TestDeployDryRun` in `cmd/cmd_test.go` keeps passing.

## `internal/engine` and the Docker backend

```go
type ResolveImageOp struct { …; Repin string }   // NEW field
const ImageSourceRepinned = "repinned"            // NEW

// docker_image.go
func (backend *DockerBackend) ResolveImage(op engine.ResolveImageOp) (engine.ResolvedImage, error)   // dispatch gains the repin branch
func (backend *DockerBackend) repin(ctx context.Context, op engine.ResolveImageOp) (engine.ResolvedImage, error)         // NEW: expand op.Repin, started event with the target, pinReference(…, ImageSourceRepinned)
func (backend *DockerBackend) pinReference(ctx context.Context, op engine.ResolveImageOp, ref, source string) (engine.ResolvedImage, error) // RENAMED from pinNewest, source parameterised
func (backend *DockerBackend) notServedError(op engine.ResolveImageOp, pin state.ImagePin, cause error) error              // REWORDED (research R7)
```

Dispatch order in `ResolveImage`: expand `op.Image` (alias error as today) → if `op.Repin != ""`: policy must be `Pinned` else `fmt.Errorf("repin of %s/%s requires the Pinned policy, got %s", …)` returned before any event; expand `op.Repin`; emit started with `ref` = the expanded target; `pinReference(ctx, op, target, engine.ImageSourceRepinned)`; emit finished through `resolvedImageFields` (which already carries `requested`) → else the existing manifest-owned / pinned dispatch with its started event. `repositoryOf` is deleted; callers use `manifest.RepositoryOf`.

Rules the backend unit tests pin (over `scriptedDockerAPI`, `fakePinStore`, `fixedNow`):

- `Pinned` + `Repin: ghcr.io/me/app:17`: calls `ImagePull(ghcr.io/me/app:17)` then `ImageInspect`; `Put` with `{Kind, Name, Requested: ghcr.io/me/app:17, Pinned: ghcr.io/me/app@<digest>, PinnedAt: fixedNow}`; result `{Ref: pinned, Digest, ImageID, Source: repinned, Requested: ghcr.io/me/app:17}`; started event `ref` = the target; finished event `source=repinned`, `requested` = the target, no `pinned_at`.
- `Repin` in alias form (`reg:lab/app:17` with `testRegistries`) is expanded before the pull and recorded expanded.
- `Repin: ghcr.io/me/app@sha256:<64>`: pulled by digest, `Requested` = the digest reference, `Pinned` equal to it.
- pull failure: error wraps the pull cause, `image.pull` error event emitted, no `Put`, no finished event, an existing pin untouched.
- no registry digest: the existing no-digest error and `image.resolve` error event; no `Put`.
- `Always` + `Repin`: the refusal text; no event, no call, no `Put`.
- `Repin` empty: every existing test unchanged (`pinNewest` behaviour through `pinReference`).
- `notServedError`: the reworded string, byte-exact, with `application` lower-cased; `errors.Is` the cause; event fields unchanged.

## `internal/ui/terminal_logger.go`

```go
case engine.ImageSourceRepinned:
	fmt.Fprintf(t.out, "  📌 Bumped %s to %s\n", artifact, manifest.ReadableVersion(e.Fields["requested"], e.Fields["digest"]))
```

One byte-exact case added to the rendered-kinds table in `terminal_logger_test.go`, plus the digest-request form (`… to 9c1b4d7e3f2a`).

## `internal/manifest`

```go
func RepositoryOf(ref string) string   // NEW; table in data-model section 3
```

`internal/planner/policy.go` (`repositoryWithoutVersion`) and `internal/engine/local/dockercontainer/docker_image.go` (`repositoryOf`) call it; their private copies are removed; their existing tests keep passing.

## State stores

Reads: `store.Teams` (through `Plan`), `store.HostPorts.ListHostPorts` (through `buildPortContext`), `store.ImagePins.Get` (previous). The only write is `store.ImagePins.Put` inside `DockerBackend.pinReference`, after the pull and inspect succeed (TD-8, constitution VI). No deployment record, host port, secret, container, or network is touched.
