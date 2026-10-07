# Contract: handler generalisation, command wiring, and tests

**Feature**: 037-delete-resource

## `internal/handler/deployments.go`

```go
type DeleteOptions struct{ Name, Team string; DryRun bool }   // renamed from DeleteApplicationOptions

func DeleteApplication(store *state.Store, container engine.ContainerBackend, opts DeleteOptions) error {
	return deleteArtifact(store, container, manifest.ApplicationKind, opts)
}
func DeleteResource(store *state.Store, container engine.ContainerBackend, opts DeleteOptions) error {
	return deleteArtifact(store, container, manifest.ResourceKind, opts)
}

func deleteArtifact(store *state.Store, container engine.ContainerBackend, kind string, opts DeleteOptions) error
func resolveDeleteTeam(store *state.Store, kind, name, team string) (string, error)
func findImagePin(store *state.Store, team, kind, name string) (state.ImagePin, bool)
func hasDeploymentRecord(store *state.Store, team, kind, name string) bool   // renamed from findApplicationRecord
func hasHostPortStep(kind string) bool                                         // kind == manifest.ApplicationKind
```

Order inside `deleteArtifact`, unchanged from today's `DeleteApplication` with the kind threaded through:

1. `resolveDeleteTeam`; empty team → `Nothing to delete for <kindWord> %q.` and return nil.
2. `container != nil` and `InspectContainer(team + "." + name)` succeeds → refusal error `<kindWord> %q still has a container; run "shrine teardown %s" first` (ref form `team/name`).
3. Read held state: host port only when `hasHostPortStep(kind)`; pin through `findImagePin` with the kind guard; record through `hasDeploymentRecord`.
4. `opts.DryRun` → the `[dry-run]` lines, return nil.
5. Release host port (applications), release pin, remove record, each printing its line; `nothingHeld` prints the in-team line.

Rules the unit tests pin (in `deployments_test.go`, beside the application tests, over `deleteTestStore`, `newMemImagePinStore`, and `stubContainerBackend`):

- `TestDeleteResource_RefusesWhileContainerExists`: `existing["demo.cache"]` → error contains `teardown`; pin and record untouched.
- `TestDeleteResource_ReleasesPinAndRecord`: a `Resource` record and a `Resource` pin for `demo/cache` → both gone; no host-port call is made (the `memHostPortStore` is left empty and is not consulted: assert `ports` map still empty, or use a nil `HostPorts` to prove the resource path never touches it).
- `TestDeleteResource_IdempotentWhenNothingHeld`: no team and explicit team both return nil.
- `TestDeleteResource_DryRunWritesNothing`: record and pin stay.
- `TestDeleteResource_AmbiguousAcrossTeams`: `Resource` records for `cache` in `demo` and `media` → error names both; with `Team: "demo"` only demo's record goes.
- `TestDeleteResource_PinAloneIsFoundAndReleased`: a `Resource` pin, no record → team found, pin released.
- `TestDeleteResource_IgnoresAnApplicationOfTheSameName`: an `Application` record and an `Application` pin for `demo/cache`, no resource state → `DeleteResource` returns nil, prints nothing-to-delete, and both the application record and pin remain (FR-006; the kind guard on `findImagePin` and on the candidate search).
- `TestDeleteApplication_IgnoresAResourceOfTheSameName`: the mirror image.
- `TestDeleteResource_ToleratesAStoreWithoutPins`: nil `ImagePins`.
- Existing `TestDeleteApplication_*` tests: call sites updated to `DeleteOptions`; assertions unchanged.

Unit tests touch no filesystem.

## `cmd/delete.go`

```go
var (deleteAppTeam, deleteResTeam string; deleteAppDryRun, deleteResDryRun bool)

var deleteApplicationCmd = &cobra.Command{Use: "application [name]", Short: …unchanged…, Long: …unchanged…, Args: cobra.ExactArgs(1),
	RunE: runDelete(manifest.ApplicationKind, &deleteAppTeam, &deleteAppDryRun)}
var deleteResourceCmd = &cobra.Command{Use: "resource [name]", Short: "Delete a resource from state and release its image pin", Long: …operator-output.md…, Args: cobra.ExactArgs(1),
	RunE: runDelete(manifest.ResourceKind, &deleteResTeam, &deleteResDryRun)}

func runDelete(del deleteHandler, team *string, dryRun *bool) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		backend, err := app.NewQueryContainerBackend(cfg, store)
		if err != nil { return err }
		opts := handler.DeleteOptions{Name: args[0], Team: *team, DryRun: *dryRun}
		if kind == manifest.ResourceKind { return handler.DeleteResource(store, backend, opts) }
		return handler.DeleteApplication(store, backend, opts)
	}
}
```

`init` adds `deleteResourceCmd` to `deleteCmd` after `deleteApplicationCmd` and registers its two flags with the resource wording. `delete team` untouched.

`cmd/delete_test.go` (NEW, in the shape of `cmd/bump_test.go`): `TestDeleteResource_RequiresArg` (no arg and two args both fail with `accepts 1 arg(s)`), run in-process with `cmd.SetArgs` and a `t.TempDir()` state dir.

## `tests/integration/delete_test.go` — written first

```go
func TestDeleteResource(t *testing.T) {
	s, worlds := newPinnedSuite(t)
	s.Test("delete of a torn-down resource releases the pin and the record, and the next deploy pins afresh", …)
	s.Test("delete while the container exists is refused and points at teardown", …)
	s.Test("dry run prints the pin and the record and writes nothing", …)
	s.Test("--team finds the resource, and so does the automatic search", …)
	s.Test("a name nothing is held for is a soft success", …)
	s.Test("every delete verb releases the pins it deletes", …)
}
```

Scenario facts to assert (strings from [operator-output.md](operator-output.md)):

1. Retire: `pinnedDeploy`; `registry.PushAs(pinnedSourceNew, pinnedRepoTag)` → `newDigest`; `pinnedTeardown` (which already drops both deployment records, so only the pin is held); `delete resource cache-pinned` succeeds with the `Released image pin` line and no `Removed deployment record` line; `pins.txt` has no `cache-pinned` line and still names `whoami-pinned`; `pinnedDeploy` prints `📌 Pinned shrine-deploy-test.cache-pinned at latest@` and `📌 Using pinned shrine-deploy-test.whoami-pinned latest@`; `cache-pinned` runs `host/shrine/whoami@newDigest`, `whoami-pinned` runs `w.pinnedRef()`.
2. Refusal: `pinnedDeploy`; `delete resource cache-pinned` fails, stderr contains `still has a container` and `shrine teardown shrine-deploy-test`; `pins.txt` and `deployments.txt` byte-identical before and after; same with `--dry-run`.
3. Dry run: `pinnedDeploy`, `pinnedTeardown`; `--dry-run` succeeds with `[dry-run] would release image pin <w.pinnedRef()> for shrine-deploy-test/cache-pinned` and no `would remove deployment record` line (the record went with the teardown; the record lines are pinned by the unit tests over a store that still holds one); both files byte-identical.
4. Team: `pinnedDeploy`, `pinnedTeardown`; `delete resource cache-pinned --team shrine-deploy-test` succeeds and releases; redeploy and teardown; `delete resource cache-pinned` without `--team` succeeds and releases; a wrong team (`--team other`) prints `Nothing to delete for resource "cache-pinned" in team "other".` and leaves state untouched.
5. Soft success: `delete resource ghost` prints `Nothing to delete for resource "ghost".` and exits 0, with no deploy.
6. Three verbs: `pinnedDeploy`, `pinnedTeardown`; `delete application whoami-pinned` → `Released image pin for shrine-deploy-test/whoami-pinned.`, `pins.txt` keeps `cache-pinned`; `delete resource cache-pinned` → `Released image pin for shrine-deploy-test/cache-pinned.`, `pins.txt` has no non-empty lines; `pinnedDeploy` prints `📌 Pinned` for both; `pinnedTeardown`; `delete team shrine-deploy-test` → `Released 2 image pin(s) for team "shrine-deploy-test".`, no pin lines; `apply teams` again so `AfterEach` cleanup finds the team.

Existing `TestDeleteTeam` and `TestDeleteTeamWithDeployments` untouched. Compile check: `go vet -tags integration ./tests/integration/...`; CI runs the suite.
