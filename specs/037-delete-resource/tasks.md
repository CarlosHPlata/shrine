# Tasks: `shrine delete resource` and pin release on every delete

**Input**: Design documents from `/specs/037-delete-resource/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: INCLUDED. The constitution mandates TDD (Principle V: integration test files are created before the implementation code) and the ticket's definition of done requires the integration scenarios to be written before the implementation, compiled under the integration tag, and never run locally; unit tests touch no filesystem.

**Organization**: Phase 1 confirms the baseline. Phase 2 holds the integration suite (written first) and the one seam every story needs: the kind-generalised `deleteArtifact` behind `DeleteApplication`, behaviour-preserving. Phases 3 to 8 follow the spec's six user stories in priority order; each adds its unit tests first, then the code. Phase 9 is the progress entry, the design refinements, the graph, and the pull request.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label (US1 to US6); Setup, Foundational, and Polish tasks carry none

## Phase 1: Setup

**Purpose**: Confirm a green baseline; no fixture directory is needed (the pinned world is built at run time by `newPinnedSuite`)

- [X] T001 Verify green baseline on branch `037-delete-resource`: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/...` pass with no file changes; confirm `main` holds #65 (`git log --oneline -1 origin/main` shows `c4785b3`); confirm the seams the plan binds to exist: `handler.DeleteApplication`, `DeleteApplicationOptions`, `resolveDeleteTeam`, `findImagePin`, `findApplicationRecord` in `internal/handler/deployments.go`; `newPinnedSuite`, `pinnedDeploy`, `pinnedTeardown`, `pinsPath`, `deploymentsPath`, `readFileOrEmpty`, `nonEmptyLines`, `pinnedApp`, `pinnedResource` in `tests/integration/pinned_image_policy_test.go`; `app.NewQueryContainerBackend` in `internal/app/components.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The integration suite, written first; then the kind-generalised delete body that every story calls

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Integration scenarios (write FIRST; compile only, never run locally)

- [X] T002 In `tests/integration/delete_test.go` add `TestDeleteResource` on `s, worlds := newPinnedSuite(t)` (same package; reuse the helpers named in T001; add a local helper `assertStateUnchanged(tc, before map[string]string)` that reads `pinsPath` and `deploymentsPath` and compares byte for byte, and `stateSnapshot(tc) map[string]string`), with the six scenarios of [contracts/handler-and-command.md](contracts/handler-and-command.md), asserting the exact strings of [contracts/operator-output.md](contracts/operator-output.md):
  (1) "delete of a torn-down resource releases the pin and the record, and the next deploy pins afresh" (US1, SC-001): `pinnedDeploy(tc, w.specsDir).AssertSuccess()`; `newDigest := w.registry.PushAs(tc, pinnedSourceNew, pinnedRepoTag)`; `pinnedTeardown(tc)`; `tc.Run("delete", "resource", "cache-pinned", "--state-dir", tc.StateDir).AssertSuccess().AssertOutputContains("Released image pin for "+testTeam+"/cache-pinned.").AssertOutputContains("Removed deployment record for "+testTeam+"/cache-pinned.")`; `pins.txt` and `deployments.txt` contain `whoami-pinned` and not `cache-pinned`; `pinnedDeploy(...).AssertSuccess().AssertOutputContains("📌 Pinned "+pinnedResource+" at latest@").AssertOutputContains("📌 Using pinned "+pinnedApp+" latest@")`; `tc.AssertContainerImage(pinnedResource, w.registry.Host+"/shrine/whoami@"+newDigest)`; `tc.AssertContainerImage(pinnedApp, w.pinnedRef())`
  (2) "delete while the container exists is refused and points at teardown" (US2, SC-002): deploy; `before := stateSnapshot(tc)`; for `flags := [][]string{{}, {"--dry-run"}}`: `tc.Run(append([]string{"delete", "resource", "cache-pinned"}, append(flags, "--state-dir", tc.StateDir)...)...).AssertFailure().AssertStderrContains("still has a container").AssertStderrContains("shrine teardown "+testTeam)`; `assertStateUnchanged(tc, before)`
  (3) "dry run prints the pin and the record and writes nothing" (US3, SC-003): deploy, teardown; `before := stateSnapshot(tc)`; `--dry-run` `.AssertSuccess().AssertOutputContains("[dry-run] would release image pin "+w.pinnedRef()+" for "+testTeam+"/cache-pinned").AssertOutputContains("[dry-run] would remove deployment record for "+testTeam+"/cache-pinned")`; `assertStateUnchanged(tc, before)`
  (4) "--team finds the resource, and so does the automatic search" (US4, SC-005): deploy, teardown; `--team other` `.AssertSuccess().AssertOutputContains("Nothing to delete for resource \"cache-pinned\" in team \"other\".")` with state unchanged; `--team testTeam` succeeds with the `Released image pin` line; deploy, teardown; no `--team` succeeds with the `Released image pin` line; `pins.txt` still names `whoami-pinned`
  (5) "a name nothing is held for is a soft success" (US1 scenario 4): no deploy; `tc.Run("delete", "resource", "ghost", "--state-dir", tc.StateDir).AssertSuccess().AssertOutputContains("Nothing to delete for resource \"ghost\".")`
  (6) "every delete verb releases the pins it deletes" (US5, SC-004): deploy, teardown; `delete application whoami-pinned` → `Released image pin for <team>/whoami-pinned.` and `pins.txt` still contains `cache-pinned`; `delete resource cache-pinned` → `Released image pin for <team>/cache-pinned.` and `nonEmptyLines(pins.txt)` is empty; `pinnedDeploy` → `📌 Pinned ` for both `pinnedApp` and `pinnedResource`; teardown; `delete team testTeam` → `Released 2 image pin(s) for team "<team>".` and `nonEmptyLines(pins.txt)` empty; `tc.Run("apply", "teams", "--path", fixturesPath("team"), "--state-dir", tc.StateDir).AssertSuccess()` so `AfterEach` finds the team
- [X] T003 Compile the suite: `go vet -tags integration ./tests/integration/...` is green (unused imports and helper signatures fixed here, never by running the suite). Confirm `git diff --stat main -- tests/integration/` shows only `delete_test.go`; `TestDeleteTeam` and `TestDeleteTeamWithDeployments` untouched

### The kind-generalised delete body (behaviour-preserving for applications)

- [X] T004 In `internal/handler/deployments.go` rename `DeleteApplicationOptions` to `DeleteOptions` (doc comment per data-model section 1) and update the call sites in `cmd/delete.go` and every `TestDeleteApplication_*` in `internal/handler/deployments_test.go`; `go build ./... && go test ./internal/handler/` green with no assertion changed
- [X] T005 In `internal/handler/deployments.go` extract the body of `DeleteApplication` into `func deleteArtifact(store *state.Store, container engine.ContainerBackend, kind string, opts DeleteOptions) error` per [contracts/handler-and-command.md](contracts/handler-and-command.md): `kindWord := strings.ToLower(kind)` in the nothing-to-delete, refusal, dry-run nothing, and in-team nothing lines; `hasHostPortStep(kind) bool` (`kind == manifest.ApplicationKind`) gates the port read, the dry-run port line, the release, and the `Released host port` line; `resolveDeleteTeam(store, kind, name, team)` consults host ports only when `hasHostPortStep(kind)` and filters records and pins by `Kind == kind`, ambiguity message `ambiguous: %s %q found in teams [%s], use --team to disambiguate` with `kindWord`; `findImagePin(store, team, kind, name)` adds the guard `pin.Kind == kind`; `findApplicationRecord` renamed `hasDeploymentRecord(store, team, kind, name)` matching `d.Kind == kind`; `DeleteApplication` becomes `return deleteArtifact(store, container, manifest.ApplicationKind, opts)`; update the WHY comments to name both verbs. `go test ./internal/handler/` green with every existing `TestDeleteApplication_*` assertion unchanged (byte-identical output for applications)

**Checkpoint**: Foundation ready; the suite compiles against a binary that has no `delete resource` subcommand yet

---

## Phase 3: User Story 1 - Retire a resource for good (Priority: P1) 🎯 MVP

**Goal**: `shrine delete resource <name>` releases a torn-down resource's pin and record, prints what it released, is a soft success when nothing is held, and never touches an application of the same name

**Independent Test**: scenarios (1) and (5) of `TestDeleteResource`; quickstart steps 1 and 3

### Handler (test first)

- [X] T006 [P] [US1] Unit tests in `internal/handler/deployments_test.go` beside the application tests, over `deleteTestStore`, `newMemImagePinStore`, and `stubContainerBackend{existing: map[string]bool{}}`, with `cachePin()` returning `state.ImagePin{Kind: manifest.ResourceKind, Name: "cache", Requested: "ghcr.io/me/cache:latest", Pinned: "ghcr.io/me/cache@sha256:abc"}`: `TestDeleteResource_ReleasesPinAndRecord` (a `Resource` record and `cachePin()` for `demo/cache`, `HostPorts: nil` to prove the resource path never consults it → `DeleteResource` returns nil, `ImagePins.Get` is `ErrImagePinNotFound`, `Deployments.List("demo")` empty); `TestDeleteResource_PinAloneIsFoundAndReleased` (pin only, no record, no `--team` → released); `TestDeleteResource_IdempotentWhenNothingHeld` (no team and `Team: "demo"` both nil); `TestDeleteResource_ToleratesAStoreWithoutPins` (nil `ImagePins`, a `Resource` record → nil); `TestDeleteResource_IgnoresAnApplicationOfTheSameName` (an `Application` record and `apiPin()` under the name `api`, no resource state → `DeleteResource{Name: "api"}` returns nil and both the record and the pin remain); `TestDeleteApplication_IgnoresAResourceOfTheSameName` (the mirror: a `Resource` record and `cachePin()` → `DeleteApplication{Name: "cache"}` returns nil, both remain). All fail to compile until T007
- [X] T007 [US1] In `internal/handler/deployments.go` add `func DeleteResource(store *state.Store, container engine.ContainerBackend, opts DeleteOptions) error { return deleteArtifact(store, container, manifest.ResourceKind, opts) }` with a doc comment mirroring `DeleteApplication`'s (no host port; Docker authoritative). `go test ./internal/handler/` green

### Command (test first)

- [X] T008 [P] [US1] Create `cmd/delete_test.go` in the shape of `cmd/bump_test.go`: `TestDeleteResource_RequiresArg` runs `delete resource` with no argument and with two arguments in-process through `rootCmd.SetArgs` with a `t.TempDir()` state dir and asserts the error contains `accepts 1 arg(s)`; fails until T009
- [X] T009 [US1] In `cmd/delete.go` add `deleteResTeam string`, `deleteResDryRun bool`, `deleteResourceCmd` (`Use: "resource [name]"`, `Short` and `Long` from [contracts/operator-output.md](contracts/operator-output.md), `Args: cobra.ExactArgs(1)`), and `func runDelete(del deleteHandler, team *string, dryRun *bool) func(*cobra.Command, []string) error` that builds `app.NewQueryContainerBackend(cfg, store)` and calls `handler.DeleteResource` for `manifest.ResourceKind`, else `handler.DeleteApplication`, with `handler.DeleteOptions{Name: args[0], Team: *team, DryRun: *dryRun}`; `deleteApplicationCmd.RunE` becomes `runDelete(manifest.ApplicationKind, &deleteAppTeam, &deleteAppDryRun)`; `init` adds `deleteResourceCmd` after `deleteApplicationCmd` and registers `--team`/`-t` ("Team owning the resource (searched automatically when omitted)") and `--dry-run` ("Print what would be released without changing state"). `go build ./... && go test ./cmd/` green; `go run . delete --help` lists `application`, `resource`, `team`

**Checkpoint**: `shrine delete resource` retires a torn-down resource; quickstart step 1 passes without a daemon

---

## Phase 4: User Story 2 - A live resource cannot be deleted (Priority: P2)

**Goal**: the delete refuses while `<team>.<name>` exists in Docker, names the teardown command, and releases nothing

**Independent Test**: scenario (2) of `TestDeleteResource`; quickstart step 4

- [X] T010 [US2] Unit test `TestDeleteResource_RefusesWhileContainerExists` in `internal/handler/deployments_test.go`: `stubContainerBackend{existing: map[string]bool{"demo.cache": true}}`, a `Resource` record and `cachePin()` → error contains `resource "demo/cache" still has a container` and `shrine teardown demo`; pin and record untouched; same result with `DryRun: true`. Passes against T005 and T007 as written; if it does not, fix the message in `deleteArtifact`, never the test

---

## Phase 5: User Story 3 - Preview what a delete would release (Priority: P3)

**Goal**: `--dry-run` prints the pin (with its exact version) and the record that would go, or nothing to delete, and writes nothing

**Independent Test**: scenario (3) of `TestDeleteResource`; quickstart step 3's first delete

- [X] T011 [US3] Unit test `TestDeleteResource_DryRunWritesNothing` in `internal/handler/deployments_test.go`: a `Resource` record and `cachePin()`, `DryRun: true` → nil; `ImagePins.Get("demo", "cache")` still succeeds; `Deployments.List("demo")` still has one record; capture stdout (as the application dry-run test does, or through `os.Pipe` if it does not) and assert `[dry-run] would release image pin ghcr.io/me/cache@sha256:abc for demo/cache` and `[dry-run] would remove deployment record for demo/cache` and no `host port` text. Passes against T005 as written

---

## Phase 6: User Story 4 - Find the resource by name, or name its team (Priority: P4)

**Goal**: without `--team` the resource is found across teams by resource records and resource pins only; several teams give the ambiguity error; `--team` narrows

**Independent Test**: scenario (4) of `TestDeleteResource` plus the unit test below; quickstart step 1's `--team` line

- [X] T012 [US4] Unit test `TestDeleteResource_AmbiguousAcrossTeams` in `internal/handler/deployments_test.go`: `Resource` records for `cache` in `demo` and `media` (no host ports) → error contains `ambiguous: resource "cache" found in teams [demo, media]`; with `Team: "demo"` returns nil and `media`'s record remains. Add `TestDeleteResource_ApplicationInAnotherTeamIsNotACandidate`: an `Application` record `api` in `demo` and a `Resource` record `api` in `media` → `DeleteResource{Name: "api"}` returns nil without ambiguity, `media`'s record is removed, `demo`'s remains (US4 scenario 5). Both pass against T005 as written

---

## Phase 7: User Story 5 - Every delete verb releases what the artifact held (Priority: P5)

**Goal**: the end-to-end assertion that `delete application`, `delete resource`, and `delete team` each release the pins of what they delete, and nothing else does

**Independent Test**: scenario (6) of `TestDeleteResource`; quickstart step 5

- [X] T013 [US5] Confirm no code change is owed: `handler.DeleteTeam` (`releaseTeamImagePins`) and the application path are T3's and untouched; `TestDeleteTeam_*` and `TestDeleteApplication_ReleasesThePin` in `internal/handler/deployments_test.go` still pass; `grep -n "delete team\|delete application" tests/integration/pinned_image_policy_test.go` shows the T3 scenarios untouched. Re-read scenario (6) in `tests/integration/delete_test.go` against [contracts/operator-output.md](contracts/operator-output.md) and `handler/teams.go`'s `Released %d image pin(s) for team %q.` line; `go vet -tags integration ./tests/integration/...` green

---

## Phase 8: User Story 6 - The documentation describes delete resource (Priority: P6)

**Goal**: a generated `delete resource` page, the `delete` parent page, the manifest reference's release sentence, and the `AGENTS.md` CLI reference and tree

**Independent Test**: quickstart step 6

- [X] T014 [P] [US6] Run `make docs-gen-cli`; confirm `docs/content/cli/delete_resource.md` is created with the `Short`, `Long`, `--dry-run`, and `-t, --team` of [contracts/operator-output.md](contracts/operator-output.md), `docs/content/cli/delete.md` gains the `shrine delete resource` SEE ALSO line, and `delete_application.md` and `delete_team.md` are unchanged (`git status --short docs/content/cli`)
- [X] T015 [P] [US6] In `docs/content/reference/manifest-schema.md` change the sentence "Only three things release a pin: `shrine delete application <name>`, `shrine delete team <name>`, and a deploy of the artifact under `Always` or `IfNotPresent`." to "Only four things release a pin: `shrine delete application <name>`, `shrine delete resource <name>`, `shrine delete team <name>`, and a deploy of the artifact under `Always` or `IfNotPresent`."; nothing else in the file changes
- [X] T016 [P] [US6] In `AGENTS.md`: rename the heading `### shrine delete application <name>` to `### shrine delete application/resource <name>` and append to its paragraph, before the `delete team` sentence, "`shrine delete resource <name>` is the same verb for a resource: it releases the image pin and drops the deployment record (a resource holds no host port), refuses while the container exists, and takes the same `--team` and `--dry-run`. Both verbs release only a pin of their own kind."; in the Project Structure `cmd/` tree add `│   ├── delete.go               # shrine delete team|application|resource <name> [--team] [--dry-run]` after the `teardown.go` line
- [X] T017 [US6] `make docs-check` (or `make docs-build` plus the two `scripts/check-md-*.sh` checks) green; from `docs/content/cli/delete_resource.md` and the `AGENTS.md` paragraph alone answer SC-007: how to retire a resource, that the container must be torn down first, how to preview, and that the pin is released

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: progress entry, design refinements, the graph, the final gates, and the pull request

- [X] T018 [P] In `specs/progress.md` insert one `- [x]` entry above the 036 entry, in the form of the 031 to 036 entries, per the `specs/progress.md` section of [contracts/docs-and-deviations.md](contracts/docs-and-deviations.md): `specs/037-delete-resource/`, issue #58, epic Pinned Image Versions, ticket T7; the behaviour; the two refinements; SC-001 to SC-007 restated; gates `TestDeleteResource` (`tests/integration/delete_test.go`, loopback registry), CI executes
- [X] T019 [P] In `specs/epics/pinned-image-versions/design.md` record the two refinements of [contracts/docs-and-deviations.md](contracts/docs-and-deviations.md): under section 4.8 the kind guard on the pin read and why `Remove` stays by name; under the T7 list (T7-02) the test level of the ambiguity case and the home of the three-verb assertion. Keep TD-1 to TD-13 untouched
- [X] T020 Run `graphify update .` and commit the regenerated `graphify-out/`
- [X] T021 Final gates: `gofmt -l . | grep -v '^docs/tools' ` empty; `go vet ./...`; `go build ./... && go test ./... && go vet -tags integration ./tests/integration/...` green; `git diff --stat main -- tests/integration/ internal/ cmd/` shows only the files the plan's Source Code tree names; mark every task in this file `[X]`; walk quickstart steps 1 and 2 (no daemon) and confirm the output lines
- [ ] T022 Pull request through `/speckit-git-pr`: Why section with `Closes #58`, the definition-of-done items listed, the two design refinements named; then a `/shrine-pr-review` pass with no open finding and green CI (which runs `TestDeleteResource`)

---

## Dependencies & Execution Order

- **Phase 1 → Phase 2**: T001 before everything.
- **Phase 2 internal**: T002 → T003 (compile); T004 → T005 (rename before extraction). T002/T003 and T004/T005 are independent of each other and may interleave, but the suite is committed first.
- **Phase 3 (US1)**: T006 and T008 [P] in parallel (different files); T007 after T006; T009 after T007 and T008.
- **Phases 4, 5, 6 (US2, US3, US4)**: each a single unit-test task against the Phase 2 body; independent of one another and of Phase 3's command task, but they share `deployments_test.go`, so run them sequentially after T007.
- **Phase 7 (US5)**: after T005 and T009 (needs the resource verb to exist for scenario 6 to be meaningful); no code.
- **Phase 8 (US6)**: T014 after T009 (the help text must exist); T015 and T016 any time after T001; T017 after T014 to T016.
- **Phase 9**: T018 and T019 [P] after Phase 8; T020 after all code; T021 after T020; T022 last.

## Parallel Execution Examples

- After T005: T006 (handler tests) and T008 (cmd test) together.
- After T009: T014, T015, T016 together, with T010, T011, T012 running in sequence in the handler test file.
- After T017: T018 and T019 together.

## Implementation Strategy

**MVP (Phases 1 to 3)**: the suite, the generalised body, `DeleteResource`, and the subcommand. At that point quickstart steps 1 and 3 work and scenarios (1) and (5) would pass in CI.

**Increment 2 (Phases 4 to 7)**: pin the refusal, dry run, team resolution, and the three-verb assertion with unit tests; no new behaviour is expected to be needed because the generalised body carries it, and any gap found is fixed in `deleteArtifact`.

**Increment 3 (Phases 8 and 9)**: documentation, progress, design, graph, pull request.

Total: 22 tasks. Per story: US1 4 (T006 to T009), US2 1, US3 1, US4 1, US5 1, US6 4; Setup 1, Foundational 4, Polish 5.
