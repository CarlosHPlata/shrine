# Tasks: `shrine bump`

**Input**: Design documents from `/specs/036-bump-command/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: INCLUDED. The constitution mandates TDD (Principle V: integration test files are created before the implementation code) and the ticket's definition of done requires the integration scenarios to be written before the implementation, compiled under the integration tag, and never run locally. Unit tests touch no filesystem: the handler helpers run over an in-memory `planner.ManifestSet`, the existing `memTeamStore`, a new `memImagePinStore`, and a recording backend stub; the backend's repin branch runs over `scriptedDockerAPI` and `fakePinStore`; the bundle shape runs over the swapped constructors in `internal/app`.

**Organization**: Phase 1 confirms the baseline. Phase 2 holds the integration suite (written first) and the four seams every story needs: `manifest.RepositoryOf`, `Repin` on the op with the `repinned` source, the backend's `pinReference`, and the `planManifestSet` extraction. Phases 3 to 9 follow the spec's seven user stories in priority order; US1 carries the command, bundle, handler, backend branch, and terminal arm, and US2 to US5 are mostly the unit cases and checks that prove each variant through the same code. The ticket is delivered as one pull request, so stories land as commits on the same branch.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label (US1 to US7); Setup, Foundational, and Polish tasks carry none

## Phase 1: Setup

**Purpose**: Confirm a green baseline; no fixture directory is needed (the pinned world is built at run time by `newPinnedSuite`)

- [X] T001 Verify green baseline on branch `036-bump-command`: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/...` pass with no file changes; confirm `main` holds #64 (`git log --oneline -1 origin/main` shows `f9e7008`); confirm the seams the plan binds to exist: `grep -n 'func (backend \*DockerBackend) pinNewest' internal/engine/local/dockercontainer/docker_image.go`, `grep -n 'func repositoryOf' internal/engine/local/dockercontainer/docker_image.go`, `grep -n 'func repositoryWithoutVersion' internal/planner/policy.go`, `grep -n 'ImageSourcePinned' internal/engine/backends.go`, `grep -n 'func newPinnedSuite' tests/integration/pinned_image_policy_test.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The integration suite, written first; then the shared helpers and the backend field every story reads

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Integration scenarios (write FIRST; compile only, never run locally)

- [X] T002 Create `tests/integration/bump_test.go` (`//go:build integration`, package `integration_test`) with `TestBump` on `s, worlds := newPinnedSuite(t)` (reuse `pinnedDeploy`, `pinnedTeardown`, `pinsPath`, `readFileOrEmpty`, `nonEmptyLines`, `pinnedContainerID`, `pinnedApp`, `pinnedResource`, `pinnedSourceNew`, `pinnedRepoTag`, `testTeam`, `w.pinnedRef()`, `w.oldDigest`, `w.registry.Host`). Add helpers: `bump(tc *TestCase, specsDir string, args ...string) *TestCase` running `append([]string{"bump"}, args...)` plus `--path specsDir --state-dir tc.StateDir`; `pushVersion(tc, w *pinnedWorld, tag string) string` returning `w.registry.PushAs(tc, pinnedSourceNew, "shrine/whoami:"+tag)`; `pinLineFor(tc, name string) string` returning the line of `pinsPath(tc)` containing ` name ` or `""`; `shortHex(digest string) string` = first twelve characters after `sha256:`. Scenarios, each starting with `w := worlds[tc]`:
  (1) "bump to a chosen version records it, deploy applies it, and bump back rolls it back" (US1, US2, SC-001, SC-003): `pinnedDeploy(tc, w.specsDir).AssertSuccess()`; `newDigest := pushVersion(tc, w, "v2")`; `before := pinnedContainerID(tc, pinnedApp)`; `resBefore := pinLineFor(tc, "cache-pinned")`; `bump(tc, w.specsDir, "app", "whoami-pinned", "-v", "v2").AssertSuccess().AssertOutputContains("🔎 Resolving image for "+pinnedApp+" ("+w.registry.Host+"/shrine/whoami:v2)").AssertOutputContains("📌 Bumped "+pinnedApp+" to v2@"+shortHex(newDigest)).AssertOutputContains("Bumped "+testTeam+"/whoami-pinned: latest@"+shortHex(w.oldDigest)+" -> v2@"+shortHex(newDigest)+`; run "shrine deploy" to apply`)`; assert `pinLineFor(tc, "whoami-pinned")` contains both `w.registry.Host+"/shrine/whoami:v2"` and `w.registry.Host+"/shrine/whoami@"+newDigest`; assert `pinLineFor(tc, "cache-pinned") == resBefore`; assert `pinnedContainerID(tc, pinnedApp) == before` and `tc.AssertContainerImage(pinnedApp, w.pinnedRef())`; `pinnedDeploy(tc, w.specsDir).AssertSuccess().AssertOutputContains("📌 Using pinned "+pinnedApp+" v2@"+shortHex(newDigest))`; assert `pinnedContainerID(tc, pinnedApp) != before` and `tc.AssertContainerImage(pinnedApp, w.registry.Host+"/shrine/whoami@"+newDigest)`; then `bump(tc, w.specsDir, "application", "whoami-pinned", "-v", w.oldDigest).AssertSuccess().AssertOutputContains("Bumped "+testTeam+"/whoami-pinned: v2@"+shortHex(newDigest)+" -> "+shortHex(w.oldDigest)+`; run "shrine deploy" to apply`)`; `pinnedDeploy(tc, w.specsDir).AssertSuccess()`; `tc.AssertContainerImage(pinnedApp, w.pinnedRef())`.
  (2) "bump to a version the registry does not serve fails and records nothing" (US1 scenario 4 and 7, SC-002): deploy; `pinsBefore := readFileOrEmpty(tc, pinsPath(tc))`; `before := pinnedContainerID(tc, pinnedApp)`; `bump(tc, w.specsDir, "app", "whoami-pinned", "-v", "v9").AssertFailure().AssertStderrContains(`application "whoami-pinned"`).AssertStderrContains(`pulling image "`+w.registry.Host+`/shrine/whoami:v9"`)`; assert `readFileOrEmpty(tc, pinsPath(tc)) == pinsBefore` and the container id unchanged; `bump(tc, w.specsDir, "app", "whoami-pinned", "-v", "not valid!").AssertFailure().AssertStderrContains(`invalid version "not valid!"`).AssertOutputNotContains("Resolving image")`; pins still byte-identical.
  (3) "bump without a version records the current newest" (US3, SC-004): deploy; `newDigest := pushVersion(tc, w, "latest")`; `before := pinnedContainerID(tc, pinnedResource)`; `bump(tc, w.specsDir, "res", "cache-pinned").AssertSuccess().AssertOutputContains("Bumped "+testTeam+"/cache-pinned: latest@"+shortHex(w.oldDigest)+" -> latest@"+shortHex(newDigest))`; `pinLineFor(tc, "cache-pinned")` contains `w.registry.Host+"/shrine/whoami@"+newDigest`; container id unchanged and `tc.AssertContainerImage(pinnedResource, w.pinnedRef())`; `pinnedDeploy(tc, w.specsDir).AssertSuccess()`; `tc.AssertContainerImage(pinnedResource, w.registry.Host+"/shrine/whoami@"+newDigest)`.
  (4) "manifest-owned, unknown, and wrong-team artifacts are refused and nothing is recorded" (US4, SC-005): `WriteManifestOwnedFixture(tc, w.specsDir, w.registry.Host, "latest")` (the app now has a fixed tag and no policy); for `flags := [][]string{{}, {"--dry-run"}}`: `bump(tc, w.specsDir, append([]string{"app", "whoami-pinned", "-v", "v2"}, flags...)...).AssertFailure().AssertStderrContains(`application "whoami-pinned": its version is manifest-owned (imagePullPolicy IfNotPresent); edit the manifest to change it`)`; `bump(tc, w.specsDir, "app", "nope", "-v", "v2").AssertFailure().AssertStderrContains(`application "nope": no manifest found in `+w.specsDir)`; `bump(tc, w.specsDir, "res", "whoami-pinned", "-v", "v2").AssertFailure().AssertStderrContains(`resource "whoami-pinned": no manifest found in `)`; `bump(tc, w.specsDir, "res", "cache-pinned", "--team", "other", "-v", "v2").AssertFailure().AssertStderrContains(`resource "cache-pinned" not found in team "other"`)`; `tc.AssertFileNotExists(pinsPath(tc))`.
  (5) "bump before the first deploy chooses the version that deploy runs" (US4 scenario 3, SC-006): no deploy; `newDigest := pushVersion(tc, w, "v2")`; `bump(tc, w.specsDir, "app", "whoami-pinned", "-v", "v2").AssertSuccess().AssertOutputContains("Pinned "+testTeam+"/whoami-pinned at v2@"+shortHex(newDigest)+`; run "shrine deploy" to apply`).AssertOutputNotContains("->")`; assert `len(nonEmptyLines(readFileOrEmpty(tc, pinsPath(tc)))) == 1`; `tc.AssertContainerNotExists(pinnedApp)`; `pinnedDeploy(tc, w.specsDir).AssertSuccess().AssertOutputContains("📌 Using pinned "+pinnedApp+" v2@").AssertOutputContains("📌 Pinned "+pinnedResource+" at latest@")`; `tc.AssertContainerImage(pinnedApp, w.registry.Host+"/shrine/whoami@"+newDigest)`.
  (6) "dry run prints what it would resolve and writes nothing" (US5, SC-007): deploy; `pushVersion(tc, w, "v2")`; `pinsBefore := readFileOrEmpty(tc, pinsPath(tc))`; `bump(tc, w.specsDir, "app", "whoami-pinned", "-v", "v2", "--dry-run").AssertSuccess().AssertOutputContains("[dry-run] would resolve "+w.registry.Host+"/shrine/whoami:v2 and pin "+testTeam+"/whoami-pinned").AssertOutputNotContains("Resolving image").AssertOutputNotContains("📌")`; `bump(tc, w.specsDir, "res", "cache-pinned", "--dry-run").AssertSuccess().AssertOutputContains("[dry-run] would resolve "+w.registry.Host+"/shrine/whoami and pin "+testTeam+"/cache-pinned")`; assert `readFileOrEmpty(tc, pinsPath(tc)) == pinsBefore`
- [X] T003 [P] In `tests/integration/pinned_image_policy_test.go`, scenario "a pin the registry no longer serves stops the deploy before any change": replace `AssertStderrContains("deploy the ")` with `AssertStderrContains(`run "shrine bump application whoami-pinned" to choose another version`)` (US6, SC-008). No other existing assertion changes
- [X] T004 Compile the suite: `go vet -tags integration ./tests/integration/...` is green (unused imports and helper signatures fixed here, never by running the suite). Confirm `git diff --stat main -- tests/integration/` shows the new file plus the one-line change in `pinned_image_policy_test.go`, and `git diff main -- tests/integration/pinned_image_policy_test.go | grep -c '^-[^-]'` prints `1`

### Shared helpers and the backend field (test first, then code)

- [X] T005 [P] Unit test in `internal/manifest/types_test.go`: `TestRepositoryOf` table per data-model section 3 (`postgres:17` → `postgres`; `postgres` → `postgres`; `127.0.0.1:5000/shrine/whoami:v2` → `127.0.0.1:5000/shrine/whoami`; `127.0.0.1:5000/shrine/whoami` unchanged; `ghcr.io/me/app@sha256:` + 64 hex → `ghcr.io/me/app`; `reg:lab/hello-api:1.2` → `reg:lab/hello-api`; `""` → `""`)
- [X] T006 In `internal/manifest/types.go` add `func RepositoryOf(ref string) string` after `DigestOf` (cut `@` suffix, then the tag after the last slash as `TagOf` finds it; WHY comment: a registry port is never mistaken for a tag). In `internal/planner/policy.go` delete `repositoryWithoutVersion` and call `manifest.RepositoryOf` in `fixedVersionImageError`; in `internal/engine/local/dockercontainer/docker_image.go` delete `repositoryOf` and call `manifest.RepositoryOf` in `resolveManifestOwned`, `pinNewest`, `pinnedReference`, and `sameRepository`. `go build ./... && go test ./internal/manifest/... ./internal/planner/... ./internal/engine/...` green with no test edited
- [X] T007 [P] In `internal/engine/backends.go` add `Repin string` to `ResolveImageOp` (comment: the target to pin instead of `Image`; empty on every deploy path) and `ImageSourceRepinned = "repinned"` to the source constants. `go build ./...` green (the dry-run backend and the engine compile unchanged)
- [X] T008 In `internal/engine/local/dockercontainer/docker_image.go` rename `pinNewest(ctx, op, ref)` to `pinReference(ctx context.Context, op engine.ResolveImageOp, ref, source string)` with `Source: source` in the returned `ResolvedImage`; `resolvePinned` calls `pinReference(ctx, op, ref, engine.ImageSourceResolved)`. Keep the WHY comment (first deploy pulls the newest; a bump pulls the chosen reference). `go test ./internal/engine/local/dockercontainer/...` green with no test edited
- [X] T009 Extract the planning helper in `internal/handler/deploy.go`: `func planManifestSet(errOut io.Writer, set *planner.ManifestSet, store *state.Store, cfg *config.Config, filter planner.Filter) (planner.PlanResult, error)` holding `buildPortContext`, `planner.Plan(set, store.Teams, cfg.Registries, ports, filter, cfg.ImagePullPolicy)`, the `result.Error` return, and the `Validation errors:` printing followed by `fmt.Errorf("Spec validation errors")`; `Deploy` and `DryRun` call `planner.LoadDir` then `planManifestSet` and keep their `No steps generated.` and engine calls. `go test ./cmd/... ./internal/handler/...` green (`TestDeployDryRun` output unchanged)

**Checkpoint**: Foundation ready; the suite compiles against a binary that has no `bump` command yet

---

## Phase 3: User Story 1 - Upgrade one artifact to a chosen version (Priority: P1) 🎯 MVP

**Goal**: `shrine bump application|resource <name> -v <version>` resolves the target in the registry, records the new pin through the backend, prints previous and new, and touches no container; the next deploy applies it; a missing or invalid version fails with nothing recorded

**Independent Test**: scenarios (1) and (2) of `TestBump`; quickstart steps 3, 4, and 6

### Backend branch (test first)

- [X] T010 [P] [US1] Unit tests in `internal/engine/local/dockercontainer/docker_image_test.go` over `newScriptedDockerAPI`, `newFakePinStore`, `pinnedBackend`, `fixedNow` (script `inspected[target]` with `RepoDigests: []string{"ghcr.io/me/app@"+newDigest}` for each target): `TestResolveImage_RepinPullsTheTargetAndRecordsIt` (`Pinned`, `Repin: "ghcr.io/me/app:17"`, pin on record `existingPin()`): calls are `ImagePull` then `ImageInspect` with `pulledRefs == ["ghcr.io/me/app:17"]`; `pins.puts` has one pin `{Kind: "Application", Name: "web", Requested: "ghcr.io/me/app:17", Pinned: "ghcr.io/me/app@"+newDigest, PinnedAt: fixedNow}`; result `{Ref: "ghcr.io/me/app@"+newDigest, Digest: newDigest, ImageID, Source: "repinned", Requested: "ghcr.io/me/app:17"}`; started event `ref == "ghcr.io/me/app:17"`; finished event fields `source=repinned`, `requested=ghcr.io/me/app:17`, no `pinned_at`. `TestResolveImage_RepinExpandsAnAlias` (`Repin: "reg:lab/app:17"` with `testRegistries`): pulled and recorded expanded. `TestResolveImage_RepinByDigestRecordsTheDigestReference` (`Repin: "ghcr.io/me/app@"+newDigest`): pulled by that reference, `Requested` and `Pinned` both equal it, readable form later shows the digest alone. `TestResolveImage_RepinPullFailureLeavesThePin` (`pullErrs[target] = cause`): error wraps `cause`, `image.pull` error event, no `image.resolve` finished, `pins.puts` empty, `pins.Get` still returns `existingPin()`. `TestResolveImage_RepinWithoutADigestEmitsAResolveError` (inspect returns no `RepoDigests`): the existing no-digest message with the target, `image.resolve` error event, no `Put`. `TestResolveImage_RepinRequiresThePinnedPolicy` (`Always` + `Repin`): error `repin of team-a/web requires the Pinned policy, got Always`, `api.calls` empty, no event, no `Put`
- [X] T011 [US1] In `internal/engine/local/dockercontainer/docker_image.go` add the fourth branch per contracts/handler-and-backend.md: in `ResolveImage`, after `expandRegistryAlias(op.Image, …)`, `if op.Repin != "" { return backend.repin(ctx, op) }` placed before the started event; `func (backend *DockerBackend) repin(ctx context.Context, op engine.ResolveImageOp) (engine.ResolvedImage, error)`: policy not `Pinned` → `fmt.Errorf("repin of %s/%s requires the Pinned policy, got %s", op.Team, op.Name, op.ImagePullPolicy)`; `target, err := expandRegistryAlias(op.Repin, backend.registries)` (alias error emitted as the `registry.alias` event with `ref: op.Repin`); `emitStarted("image.resolve", {team, name, ref: target})`; `resolved, err := backend.pinReference(ctx, op, target, engine.ImageSourceRepinned)`; on success `emitFinished("image.resolve", resolvedImageFields(op, resolved))`. WHY comment on `repin`: bump is the only caller; the backend stays the one pin writer (TD-8). `go test ./internal/engine/...` green including T010

### Terminal arm (test first)

- [X] T012 [P] [US1] In `internal/ui/terminal_logger_test.go` add to the rendered-kinds table: `ev("image.resolve", engine.StatusFinished, "team", "team-a", "name", "db", "ref", "postgres@sha256:9c1b…", "digest", "sha256:9c1b4d7e3f2a…", "requested", "postgres:17", "source", "repinned")` → `"  📌 Bumped team-a.db to 17@9c1b4d7e3f2a\n"`; and a digest-request case (`requested` = `postgres@sha256:…`) → `"  📌 Bumped team-a.db to 9c1b4d7e3f2a\n"`
- [X] T013 [US1] In `internal/ui/terminal_logger.go` `renderImageResolved` add `case engine.ImageSourceRepinned: fmt.Fprintf(t.out, "  📌 Bumped %s to %s\n", artifact, manifest.ReadableVersion(e.Fields["requested"], e.Fields["digest"]))`. `go test ./internal/ui/...` green

### Bundle (test first)

- [X] T014 [P] [US1] In `internal/app/app_test.go` add `TestBuildBumpBundle_*` in the shape of the deploy bundle tests over `pinHermeticEnv`, `useInMemoryFileLogger`, and the swapped `newContainerBackend`: happy path returns a bundle with `Out`, `ErrOut`, `Cfg`, `Store`, `Paths`, `SpecsDir` (resolved from `manifestDir`), `Observer` (a `MultiObserver` carrying the fake file logger), and `ContainerBackend`; `newVault`, `newTraefikPlugin`, and `newLocalEngine` are never called (swap them to fail the test); `validating registries:` prefix on a bad registry config with no logger opened; `observer:` prefix with the cause; `container backend:` prefix with the cause and the logger closed once; unresolvable `specsDir` (`t.Setenv("HOME", "")`, `~/manifests`) fails with `resolving specsDir` before the logger opens; cleanup closes the logger and reports its error
- [X] T015 [US1] In `internal/app/app.go` add `BumpBundle` and `BuildBumpBundle(cfg, store, paths, manifestDir, out, errOut)` per data-model section 4: `cfg.ValidateRegistries()` → `cfg.ResolveSpecsDir(manifestDir)` → `newObserverPair(out, paths)` → `newContainerBackend(store, cfg.Registries, observer)` with `closeObserver` on failure → return the bundle and `joinCleanup(closeObserver)`. Doc comment names what it does not build (no plugin, vault, routing, engine; TD-12). `go test ./internal/app/...` green

### Handler (test first)

- [X] T016 [P] [US1] Create `internal/handler/bump_test.go` (package `handler`): add `memImagePinStore` (map keyed `state.ImagePinKey`, `Get` returning `state.ErrImagePinNotFound` when absent, `Put` recording into the map and a `puts` slice, `Release`/`ReleaseTeam`/`List`/`ListAll` minimal) and `recordingContainerBackend` (embeds a zero `engine.ContainerBackend` via a struct that implements the interface with no-ops; `ResolveImage` records the op, returns `resolved engine.ResolvedImage` or `err`). Helper `pinnedSet(t, kind, name, owner, image, policy string) *planner.ManifestSet` building a set with `planner.NewManifestSet()` and `MergeManifest` of a hand-built `*manifest.Manifest` (look at `planner/loader_test.go` for the manifest shape; for a resource set `Spec.Type` and `Spec.Image`). Tests: `TestValidateBumpVersion` table (`""`, `17`, `v1.4.0`, `latest`, `a_b.c-d`, 128 characters, `sha256:`+64 lowercase hex accepted; 129 characters, leading `.`, leading `-`, `v 9`, `postgres:17`, `repo@sha256:…`, `sha256:` with 63 hex, with 65 hex, with uppercase hex rejected with the exact message of contracts/operator-output.md). `TestBuildBumpTarget` table (data-model and contract cases including `(postgres:latest, 17)` → `postgres:17` and `(reg:lab/api, 1.2)` → `reg:lab/api:1.2`). `TestFindBumpArtifact`: found by kind returns metadata and image; `bump resource` of an application name is unknown; unknown names the directory exactly `application "nope": no manifest found in /specs`; `--team` equal to the owner passes; `--team other` fails with `application "web" not found in team "other" (its manifest in /specs is owned by "team-a")`. `TestPrepareBump` over `planner.Plan`-ready sets with `&state.Store{Teams: &memTeamStore{}, HostPorts: &memHostPortStore{ports: state.HostPortMap{}}}` and `cfg := &config.Config{}`: declared `IfNotPresent` refused naming `IfNotPresent`; no field with `cfg.ImagePullPolicy = "Always"` refused naming `Always`; no field, no default, image `nginx:1.27` refused naming `IfNotPresent`; declared `Pinned` with `-v 17` returns `bumpTarget{Team: "team-a", Kind, Name, ManifestImage: "nginx", Target: "nginx:17"}`; a `Pinned` manifest naming `nginx:1.27` fails with `Spec validation errors` and `errOut` containing `names a fixed version`. `TestBumpResolved`: with a previous pin `{Requested: "nginx:16", Pinned: "nginx@sha256:aaaa…"}` and the stub returning `{Ref: "nginx@sha256:bbbb…", Digest: "sha256:bbbb…", Source: "repinned", Requested: "nginx:17"}`, the recorded op is `{Team: "team-a", Name: "web", Kind: "Application", Image: "nginx", ImagePullPolicy: "Pinned", Repin: "nginx:17"}`, the previous pin was read before `ResolveImage` (record call order), and `out` is exactly `Bumped team-a/web: 16@aaaaaaaaaaaa -> 17@bbbbbbbbbbbb; run "shrine deploy" to apply\n`; without a previous pin `out` is `Pinned team-a/web at 17@bbbbbbbbbbbb; run "shrine deploy" to apply\n`; a digest previous (`Requested: "nginx@sha256:aaaa…"`) prints `aaaaaaaaaaaa -> …`; a stub error `errBoom` returns `application "web": boom` with `errors.Is(err, errBoom)` and `out` empty; a nil `store.ImagePins` means no previous. `TestFormatBumpDryRun`: `[dry-run] would resolve nginx:17 and pin team-a/web`
- [X] T017 [US1] Create `internal/handler/bump.go` per contracts/handler-and-backend.md: `BumpOptions`, `bumpTarget`, `validateBumpVersion` (two compiled `regexp` package vars, the exact message), `findBumpArtifact(set, opts, manifestDir)` (kind switch over `set.Applications`/`set.Resources`; `strings.ToLower(kind)` in messages), `bumpFilter(kind, name)` (`planner.ByApp`/`planner.ByResource`), `effectivePolicyOf(set, kind, name)` (reads `Spec.ImagePullPolicy` from the planned set), `buildBumpTarget(image, version)` (`manifest.RepositoryOf(image)` + `":"`/`"@"` + version; the image itself when version is empty), `prepareBump(errOut, manifestDir, set, store, cfg, opts)` in the data-model order (find → team → `planManifestSet` → policy refusal `%s %q: its version is manifest-owned (imagePullPolicy %s); edit the manifest to change it` → target), `repinOp(t)`, `previousPin(store, team, name)` (nil `ImagePins` or `ErrImagePinNotFound` → absent; other errors returned as `reading image pin for %s/%s: %w`), `bumpResolved(out, store, backend, t)` (previous, `backend.ResolveImage(repinOp(t))` wrapped `%s %q: %w` on error, `fmt.Fprintln(out, formatBumpResult(...))`), `formatBumpResult` over `manifest.ReadableVersion(previous.Requested, manifest.DigestOf(previous.Pinned))` and `manifest.ReadableVersion(resolved.Requested, resolved.Digest)`, `formatBumpDryRun(t)`, and the entry points `Bump(b *app.BumpBundle, opts)` (`validateBumpVersion` → `planner.LoadDir(b.SpecsDir)` → `prepareBump` → `bumpResolved`) and `BumpDryRun(out, errOut, manifestDir, store, cfg, opts)` (`validateBumpVersion` → `LoadDir` → `prepareBump` → print). `go test ./internal/handler/...` green including T016

### Command (test first)

- [X] T018 [P] [US1] Create `cmd/bump_test.go` (package `cmd_test`) in the shape of `TestDeployTeam_RequiresArg`: `bump application` and `bump resource` without a name fail with `accepts 1 arg`; `bump app` and `bump res` without a name fail the same way (proving the aliases resolve); `bump` alone prints help containing `application` and `resource` (use `cmd.SetOutput`, `cmd.SetArgs([]string{"bump", "--state-dir", t.TempDir()})`)
- [X] T019 [US1] Create `cmd/bump.go` per contracts/handler-and-backend.md and the help text of contracts/operator-output.md: `bumpCmd` (`Use: "bump"`, `Short`, `Long`), `bumpApplicationCmd` (`Use: "application [name]"`, `Aliases: []string{"app"}`, `Args: cobra.ExactArgs(1)`, `RunE: runBump(manifest.ApplicationKind)`), `bumpResourceCmd` (`resource [name]`, alias `res`, `manifest.ResourceKind`); persistent flags on `bumpCmd`: `StringVarP(&bumpVersion, "version", "v", "", …)`, `StringVarP(&bumpTeam, "team", "t", "", …)`, `StringVarP(&bumpPath, "path", "p", "", …)`, `BoolVar(&bumpDryRun, "dry-run", false, …)` with the flag help strings of the contract; `runBump(kind)` resolves `cfg.ResolveSpecsDir(bumpPath)`, builds `handler.BumpOptions`, dispatches to `handler.BumpDryRun` or builds `app.BuildBumpBundle` with `defer cleanup()` and calls `handler.Bump`; `init()` registers `bumpCmd` on `rootCmd` and the two subcommands. `go build ./... && go test ./cmd/...` green including T018

**Checkpoint**: `go test ./...` green; `go vet -tags integration ./tests/integration/...` green; quickstart steps 3, 4, and 6 pass on a private daemon if one is available (not required locally)

---

## Phase 4: User Story 2 - Roll back (Priority: P2)

**Goal**: a bump to the earlier readable version or to the exact version a previous bump printed records it, and the next deploy runs it; there is no separate command

**Independent Test**: the second half of `TestBump` scenario (1); quickstart step 5

- [X] T020 [US2] Confirm the rollback path needs no new code and is pinned by tests: `TestBuildBumpTarget` covers the digest form (T016), `TestResolveImage_RepinByDigestRecordsTheDigestReference` covers the pull by digest (T010), the digest-previous case of `TestBumpResolved` covers the `<12 hex> -> …` output (T016), and `TestBump` scenario (1) covers deploy after the rollback (T002). Verify `grep -n 'sha256' internal/handler/bump_test.go` shows the digest target and digest-previous cases; if any is missing, add it. `go test ./internal/handler/... ./internal/engine/...` green
- [X] T021 [US2] In `cmd/bump.go` `Long` text, verify the sentence "Rolling back is a bump to an earlier version." is present for both subcommands (R-26); `go test ./cmd/...` green

**Checkpoint**: rollback is the same command with an earlier value

---

## Phase 5: User Story 3 - Take the newest again (Priority: P3)

**Goal**: `bump` without `-v` resolves the manifest's own reference (newest) and records it; previous and new are printed even when equal

**Independent Test**: `TestBump` scenario (3); quickstart step 7

- [X] T022 [US3] Confirm the empty-version path is pinned: `TestValidateBumpVersion` accepts `""` (T016), `TestBuildBumpTarget` has `(postgres, "")` → `postgres` and `(127.0.0.1:5000/shrine/whoami, "")` → unchanged (T016), and add to `TestBumpResolved` the case where the stub returns a `Requested` of `nginx` with the same digest as the previous pin, asserting `out` is `Bumped team-a/web: latest@aaaaaaaaaaaa -> latest@aaaaaaaaaaaa; run "shrine deploy" to apply\n` (previous and new may be equal; both printed). `go test ./internal/handler/...` green
- [X] T023 [US3] Add to `internal/engine/local/dockercontainer/docker_image_test.go` `TestResolveImage_RepinUntaggedTargetPinsNewest`: `Repin: "ghcr.io/me/app"` (no tag) pulls `ghcr.io/me/app`, records `Requested: "ghcr.io/me/app"`, and the finished event's `requested` is the untagged reference (the terminal then reads `latest@…` through `ReadableVersion`). `go test ./internal/engine/...` green

**Checkpoint**: "newest" is an explicit recorded act

---

## Phase 6: User Story 4 - Only pinned artifacts can be bumped, and any pinned artifact in the directory can (Priority: P4)

**Goal**: manifest-owned artifacts are refused naming the policy; unknown names are refused naming the directory; a wrong `--team` is refused; an undeployed pinned artifact can be bumped and its first deploy runs the chosen version; `--path` names the directory as deploy does

**Independent Test**: `TestBump` scenarios (4) and (5); quickstart steps 1 and 8

- [X] T024 [US4] Confirm the refusal and lookup rules are pinned by `TestFindBumpArtifact` and `TestPrepareBump` (T016) and add the two missing cases: `--team` given and equal to the owner on a `Pinned` manifest returns the target (verifies, does not disambiguate), and an empty `set` (no manifests at all) refuses with `no manifest found in /specs`. `go test ./internal/handler/...` green
- [X] T025 [US4] Add to `TestBumpResolved` (T016) the undeployed case explicitly: `memImagePinStore` empty and `memDeploymentStore` empty (bump never reads deployment records; assert the stub store's `List` was not called by giving `memDeploymentStore{listErr: errBoom}` and expecting success). `go test ./internal/handler/...` green
- [X] T026 [US4] In `cmd/bump.go`, verify the `-p/--path` flag help reads exactly as `deploy`'s (`Directory containing manifest files (overrides specsDir in config.yml)`) and that `runBump` resolves it through `cfg.ResolveSpecsDir(bumpPath)` so the unknown-artifact message names the resolved directory; add to `cmd/bump_test.go` a case running `bump app nope -v 1 --path <t.TempDir()> --state-dir <t.TempDir()>` and asserting the error contains `no manifest found in ` and the temp directory path (an empty directory loads as an empty set; no Docker needed because `LoadDir` and the lookup fail before the bundle is built). `go test ./cmd/...` green

**Checkpoint**: every refusal records nothing and contacts no registry

---

## Phase 7: User Story 5 - Preview a bump without recording it (Priority: P5)

**Goal**: `--dry-run` prints the reference that would be resolved and the artifact that would be pinned, builds no Docker client, and writes nothing; refusals apply unchanged

**Independent Test**: `TestBump` scenario (6) and the `--dry-run` refusal in scenario (4); quickstart step 2

- [X] T027 [US5] Confirm `handler.BumpDryRun` shares `prepareBump` with `Bump` and ends in `formatBumpDryRun` (T017), then add to `internal/handler/bump_test.go` `TestPrepareBump_RefusalsAreSharedByDryRun`: the manifest-owned, unknown, and wrong-team cases of `TestPrepareBump` are table-driven and the table is reused by this test, proving one refusal path serves both entry points; and `TestFormatBumpDryRun` covers a tagged target (`[dry-run] would resolve nginx:17 and pin team-a/web`), a digest target, and the manifest reference itself (`would resolve nginx and pin …`). `BumpDryRun` is not unit-tested directly because it calls `LoadDir`; `TestBump` scenario (6) covers it. `go test ./internal/handler/...` green
- [X] T028 [US5] Add to `cmd/bump_test.go` a dry-run case with `DOCKER_HOST` set to `tcp://127.0.0.1:1` via `t.Setenv` and an empty `--path` directory: `bump app nope -v 1 --dry-run` fails with `no manifest found` and not with any Docker client error, proving the dry-run path builds no bundle before the lookup. `go test ./cmd/...` green

**Checkpoint**: dry run is a print-only path

---

## Phase 8: User Story 6 - A vanished exact version points at bump (Priority: P6)

**Goal**: the deploy failure for a pinned exact version the registry no longer serves names `shrine bump <kind> <name>` as the way out

**Independent Test**: the reworded assertion in `TestPinnedImagePolicy` (T003); quickstart step 8

- [X] T029 [P] [US6] In `internal/engine/local/dockercontainer/docker_image_test.go` `TestResolveImage_PinnedNoLongerServedNamesTheWayOut` change `want` to `pinned exact version "ghcr.io/me/app@` + appDigest + `" for team-a/web is no longer served by the registry; run "shrine bump application web" to choose another version: pulling image "ghcr.io/me/app@` + appDigest + `": manifest unknown`; keep the `errors.Is`, event, and no-`Put` assertions
- [X] T030 [US6] In `internal/engine/local/dockercontainer/docker_image.go` `notServedError`: the format becomes `pinned exact version %q for %s/%s is no longer served by the registry; run \"shrine bump %s %s\" to choose another version: %w` with `pin.Pinned, op.Team, op.Name, strings.ToLower(op.Kind), op.Name, cause`; rewrite the comment (WHY: the pin cannot be honoured and only a bump chooses another exact version). Remove the now-unused `manifest.ImagePullPolicyAlways`/`IfNotPresent`/`Pinned` references if nothing else in the file uses them. `go test ./internal/engine/...` green

**Checkpoint**: the message names the exact command to run

---

## Phase 9: User Story 7 - The documentation describes bump (Priority: P7)

**Goal**: generated command pages for `bump`, `bump application`, and `bump resource`; the manifest reference names bump as the way to move a pinned version; the contributor reference lists bump

**Independent Test**: from `docs/content/cli/bump_application.md` and the manifest reference alone, answer how to move a pinned resource to 17, go back, take the newest, and what happens to the container (spec SC-010)

- [X] T031 [P] [US7] Run `make docs-gen-cli` and confirm `docs/content/cli/bump.md`, `bump_application.md`, and `bump_resource.md` exist with the `Short`, `Long`, the four flags, and the `AUTO-GENERATED` banner; `git status --short docs/content/cli/` shows only those three files added; run `make docs-check`
- [X] T032 [P] [US7] Edit `docs/content/reference/manifest-schema.md` subsection Image pull policy per contracts/docs-and-deviations.md: the **Moving a pin.** paragraph with its example block after the "Only three things release a pin" paragraph; the vanished-version sentence ends with "and the `shrine bump` command to run"; the **Reading what is pinned.** clause gains "(the result of a `bump`)"
- [X] T033 [P] [US7] Edit `AGENTS.md` per contracts/docs-and-deviations.md: the Quick Start `shrine bump app my-api -v 1.4.0` line; the `### shrine bump application/resource <name>` section after the describe section; the `bump.go` entry in the project tree; the appended clause in the image-resolution pipeline note; the `pins.txt` line's "; replaced by `shrine bump`"

**Checkpoint**: documentation and code change together

---

## Phase 10: Polish & Cross-Cutting Concerns

**Purpose**: the project's definition of done for a ticket

- [X] T034 Run `gofmt -l . ` (empty), `go vet ./...`, `go test ./...`, and `go vet -tags integration ./tests/integration/...`; confirm no test under `internal/` or `cmd/` creates a directory or writes a file other than `cmd/bump_test.go`'s `t.TempDir()` state and path directories (grep the new test files for `os.WriteFile`, `os.MkdirAll`: none)
- [X] T035 Review the diff against the contracts: `git diff main --stat` lists only the files in plan.md's structure; `git diff main -- internal/handler/deploy.go` shows the extraction and no behaviour change; `grep -rn 'repositoryOf\|repositoryWithoutVersion' internal/` prints nothing; `grep -n 'Put(' internal/handler/bump.go` prints nothing (the backend is the only writer); `grep -n 'ResolveImage' internal/engine/dryrun/` is unchanged
- [X] T036 [P] Add the `specs/progress.md` entry per contracts/docs-and-deviations.md, above the 035 entry, in the form of the 031 to 035 entries, naming issue #57, ticket T6, the behaviour, the finding (a name is unique per manifest directory, so `--team` verifies and no ambiguity error exists), SC-001 to SC-010, and the gate `TestBump` plus the reworded `TestPinnedImagePolicy` assertion (CI executes)
- [X] T037 [P] Record the seven design refinements of contracts/docs-and-deviations.md in `specs/epics/pinned-image-versions/design.md` as `*Amended by T6 (spec 036):*` notes in sections 4.2 (points 3 and 4), 4.6, and 5, in the style of the T2 to T5 amendments
- [X] T038 Run `graphify update .` and commit the regenerated `graphify-out/`
- [ ] T039 Walk `specs/036-bump-command/quickstart.md` steps 0 to 2 locally (no daemon); steps 3 to 8 only on a private daemon, otherwise rely on CI. Tick every task in this file, then open the pull request through `/speckit-git-pr` with `Closes #57` in its Why section, run `/shrine-pr-review`, and address every open finding before merge

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: none
- **Foundational (Phase 2)**: T002 to T004 first (the suite is written before any implementation); T005 to T009 after, blocking every story. Within it T005, T007 are parallel; T006 after T005; T008 after T007 and T006; T009 independent of T005 to T008
- **US1 (Phase 3)**: after Phase 2. Four test-first pairs: backend (T010 → T011), terminal (T012 → T013), bundle (T014 → T015), handler (T016 → T017); the command (T018 → T019) after T015 and T017. The four pairs are parallel with each other
- **US2 to US5 (Phases 4 to 7)**: after US1; each is a verification-and-gap-filling phase over the same code and can run in any order
- **US6 (Phase 8)**: after Phase 2 only (independent of US1); T029 → T030
- **US7 (Phase 9)**: T031 after T019 (the pages come from the command help); T032 and T033 after Phase 2 (they describe the contract, not the code)
- **Polish (Phase 10)**: after every story; T036 and T037 parallel; T038 after all code; T039 last

### Parallel Opportunities

- Phase 2: T003 with T002; T005 with T007; T009 with T005 to T008
- Phase 3: T010, T012, T014, T016 together (four test files); then T011, T013, T015, T017 together (four source files); then T018, then T019
- Phase 8 and Phase 9's T032 and T033 can run while Phase 3 is in progress
- Phase 10: T036 with T037

## Parallel Example: User Story 1

```bash
# Write the four unit-test files first, together:
Task: "T010 repin branch tests in internal/engine/local/dockercontainer/docker_image_test.go"
Task: "T012 repinned arm cases in internal/ui/terminal_logger_test.go"
Task: "T014 bump bundle tests in internal/app/app_test.go"
Task: "T016 handler helper tests in internal/handler/bump_test.go"

# Then the four implementations, together:
Task: "T011 repin branch in internal/engine/local/dockercontainer/docker_image.go"
Task: "T013 repinned arm in internal/ui/terminal_logger.go"
Task: "T015 BumpBundle in internal/app/app.go"
Task: "T017 handler in internal/handler/bump.go"
```

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1, then Phase 2 with the integration suite written and compiled before any seam changes
2. Phase 3: the command resolves, records, prints, and the next deploy applies
3. **STOP and VALIDATE**: `go test ./...`, `go vet -tags integration ./tests/integration/...`; quickstart steps 3, 4, 6 if a private daemon exists

### Incremental Delivery

Phases 4 to 7 each add the unit cases that pin one variant of the same path (rollback, newest, refusals and undeployed, dry run) and are cheap; Phase 8 is the one-line message change; Phase 9 is documentation; Phase 10 is the ticket's definition of done. All land in one pull request.

## Notes

- Never run the integration suite locally; CI is the integration gate
- Unit tests never touch the filesystem apart from `cmd/bump_test.go`'s `t.TempDir()` for the state and path directories, which the existing command tests also use
- The backend is the only writer of pins; the handler reads
- Commit after each phase or logical group; the ticket is one pull request
