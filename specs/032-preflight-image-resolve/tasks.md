# Tasks: Resolve Every Image Before Touching Any Container

**Input**: Design documents from `/specs/032-preflight-image-resolve/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: INCLUDED. The constitution mandates TDD (Principle V: integration test files are created before the implementation code) and the delivery plan requires the integration scenarios to be written before the implementation, compiled under the integration tag, and never run locally. Unit tests touch no filesystem and use the existing `dockerAPI` and `ContainerBackend` fakes. Iterate with `go test ./...`; compile the suite with `go vet -tags integration ./tests/integration/...`; CI is the integration gate.

**Organization**: Phase 2 holds the integration scenarios (written first) and the backend contract every story needs. Phases 3 to 7 follow the spec's user stories in priority order.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label (US1 to US5); Setup, Foundational, and Polish tasks carry none

## Phase 1: Setup

**Purpose**: Confirm a green baseline and create the two fixtures the scenarios reference

- [x] T001 Verify green baseline on branch `032-preflight-image-resolve`: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/...` pass with no file changes
- [x] T002 [P] Create fixture `tests/testdata/deploy/preflight-unresolvable/` with three manifests for owner `shrine-deploy-test`: `resource.yml` (Resource `cache-ok`, type `cache`, version `"1"`, image `traefik/whoami`, env `endpoint` exported as output), `app-ok.yml` (Application `web-ok`, image `traefik/whoami`, port 80), `app-broken.yml` (Application `zz-broken`, image `localhost:1/shrine/unresolvable:1.0.0`, port 80, `dependencies` on Resource `cache-ok` and Application `web-ok` so it is last in deploy order by construction)
- [x] T003 [P] Create fixture `tests/testdata/deploy/preflight-fixed-tag/app.yml`: Application `whoami-fixed`, owner `shrine-deploy-test`, image `traefik/whoami:v1.10.2`, port 80 (verify the tag exists on Docker Hub before committing)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The integration scenarios, written first, and the backend contract with both real implementations, so every story builds on a compiling, green tree

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Integration scenarios (write FIRST; compile only, never run locally)

- [x] T004 Create `tests/integration/preflight_image_resolve_test.go` (`//go:build integration`, `NewDockerSuite(t, testTeam)`, BeforeEach applies the `team` fixture and seeds subnet state like `TestDeploy`) with: (US1) deploy of `preflight-unresolvable` fails, stderr contains `zz-broken` and `localhost:1/shrine/unresolvable:1.0.0`, `AssertContainerNotExists` for `shrine-deploy-test.cache-ok` and `shrine-deploy-test.web-ok`, `AssertNetworkNotExists("shrine.shrine-deploy-test.private")`; (US3) `deploy --dry-run` of `resources` succeeds, output contains `[DOCKER] ImageResolve: name=shrine-deploy-test.test-cache image=traefik/whoami policy=Always -> manifest-owned` and the `whoami-res` line, both indices precede the index of `[DOCKER] CreatePlatformNetwork` and of the first `[DOCKER] ContainerCreate:`, no container exists afterwards; (US4) `preflight-fixed-tag` deployed twice: the second output does not contain `Pulling image traefik/whoami:v1.10.2`, contains `Resolved shrine-deploy-test.whoami-fixed traefik/whoami:v1.10.2`, and the container id is unchanged across the two runs; (US4) `basic` deployed twice: both outputs contain `Pulled image traefik/whoami`
- [x] T005 Append one scenario to `TestDeploy` in `tests/integration/deploy_test.go`: (US2) deploy `resources` succeeds and the output contains `🔎 Resolving image for shrine-deploy-test.test-cache (traefik/whoami)`, `🔎 Resolved shrine-deploy-test.test-cache traefik/whoami@`, and the same two lines for `whoami-res`; the first `Resolved` index precedes the index of `Deploying Resource: test-cache`; no existing scenario or assertion is edited
- [x] T006 Compile the suite: `go vet -tags integration ./tests/integration/...` is green (the new file uses only `testutils` helpers)

### Contract and implementations (make the tree green again before any story)

- [x] T007 Add `ResolveImageOp{Team, Name, Kind, Image, ImagePullPolicy string}`, `ResolvedImage{Ref, Digest, ImageID, Source string}`, `const ImageSourceManifest = "manifest"`, `ResolveImage(op ResolveImageOp) (ResolvedImage, error)` on `ContainerBackend`, and `ImageID string` on `CreateContainerOp` in `internal/engine/backends.go` (contract: contracts/backend-contract.md; no `Repin` field)
- [x] T008 [P] Give every `ContainerBackend` fake a `ResolveImage` returning `ResolvedImage{Ref: op.Image, Source: ImageSourceManifest}`: `fakeContainerBackend` and `teardownFailingContainerBackend` in `internal/engine/engine_test.go`, `capturingContainerBackend` in `internal/engine/engine_publish_test.go`, `stubContainerBackend` in `internal/handler/deployments_test.go`, `fakeBackend` in `internal/plugins/gateway/traefik/plugin_test.go`, `recordingContainerBackend` in `internal/plugins/gateway/traefik/routing_test.go`
- [x] T009 [P] Unit tests in new `internal/engine/local/dockercontainer/docker_image_test.go` over a call-recording `dockerAPI` fake (no filesystem): `pickRepoDigest` table (exact match; `docker.io/traefik/whoami` matches `traefik/whoami@sha256:…`; `docker.io/library/postgres` matches `postgres@sha256:…`; a different repository is skipped; no match returns `""`); `repositoryOf` table (tag, no tag, digest reference, registry with port); `ResolveImage` under `Always` calls `ImagePull` then `ImageInspect` and never `ImageList`; under `IfNotPresent` with a local hit calls `ImageList` only and takes `ID` and the digest from the summary; under `IfNotPresent` with a miss calls `ImageList`, `ImagePull`, `ImageInspect`; expands `reg:myregistry/...` once and reports the expanded `Ref`; returns `Source == ImageSourceManifest`; emits `image.resolve` started with `team`, `name`, `ref` and finished with `team`, `name`, `ref`, `digest`, `source`; a pull failure returns an error whose text contains the expanded reference and emits no `image.resolve` finished event
- [x] T010 Implement `ResolveImage` in `internal/engine/local/dockercontainer/docker_image.go` per contracts/backend-contract.md: private `localImage{ID, RepoDigests}`; `locateImage(ctx, ref, policy)` keeping today's branches; `findLocalImage` (ImageList, summary fields), `pullImage` (credentials, `image.pull` started/finished, drained stream), `inspectImage`; `pickRepoDigest`, `repositoryOf`, `normalizeRepository`; alias expansion once; `image.resolve` started and finished events; error wording `listing images matching %q: %w` and `registry credentials for %q: %w`; delete the old `resolveImage` (T009 passes)
- [x] T011 [P] Unit test in `internal/engine/dryrun/dry_run_container_test.go`: `ResolveImage` prints exactly `[DOCKER] ImageResolve: name=demo.web image=reg:lab/web:1.2 policy=IfNotPresent -> manifest-owned\n` and returns `ResolvedImage{Ref: "reg:lab/web:1.2", Source: ImageSourceManifest}` with empty `ImageID`
- [x] T012 Implement `DryRunContainerBackend.ResolveImage` in `internal/engine/dryrun/dry_run_container.go` (T011 passes)
- [x] T013 Green checkpoint: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/... && gofmt -l .` all clean; commit

**Checkpoint**: the contract exists with both real implementations; the engine does not call it yet

---

## Phase 3: User Story 1 - A bad image reference changes nothing (Priority: P1) 🎯 MVP

**Goal**: `ExecuteDeploy` resolves every planned step before `CreatePlatformNetwork` and aborts on the first failure; `CreateContainer` uses the pre-resolved image

**Independent Test**: the `preflight-unresolvable` scenario of T004 (CI); locally, the engine unit tests prove the call order and the abort

### Tests for User Story 1 (write FIRST; must fail before T016 and T017)

- [x] T014 [P] [US1] Unit tests in `internal/engine/engine_resolve_test.go` (new file beside `engine_test.go`, same package) over a timeline-recording fake: with a Resource and an Application step, `ExecuteDeploy` calls `ResolveImage` for both steps in step order before `CreatePlatformNetwork`, `CreateNetwork`, and `CreateContainer`; the op carries `Team`, `Name`, `Kind`, the manifest `Image`, and the effective policy (`Always` for an untagged image, `IfNotPresent` for a fixed tag); when `ResolveImage` fails for the second step, `ExecuteDeploy` returns an error whose text is `application "svc-b": <cause>`, the timeline holds no `CreatePlatformNetwork`, `CreateNetwork`, `CreateContainer`, or `Finalize`, and the observer recorded one `image.resolve` error event with `team`, `name`, `ref`; on success each `CreateContainerOp` carries `Image == ResolvedImage.Ref` and `ImageID == ResolvedImage.ImageID` (fake returns a distinct `Ref` and `ImageID` per op to prove the projection)
- [x] T015 [P] [US1] Unit tests in `internal/engine/local/dockercontainer/docker_container_test.go`: `CreateContainer` with `ImageID: "sha256:pre"` and `Image: "docker.io/traefik/whoami:latest"` never calls `ImageList`, `ImagePull`, or `ImageInspect` (fake panics on them), hands `docker.io/traefik/whoami:latest` to `ContainerCreate`, and the `reg:` form is not expanded a second time; existing alias tests keep passing unchanged (they leave `ImageID` empty)

### Implementation for User Story 1

- [x] T016 [US1] Implement `resolveImages(set, steps)`, `resolveImageOpFor(set, step)`, `resolvedImageKey(kind, name)` in `internal/engine/engine.go`; call `resolveImages` at the top of `ExecuteDeploy` before `CreatePlatformNetwork`; wrap the first error as `fmt.Errorf("%s %q: %w", strings.ToLower(step.Kind), step.Name, err)` through `emitErr("image.resolve", {team, name, ref})`; pass the map to `deployApplication` and `deployResource` and set `op.Image = resolved.Ref`, `op.ImageID = resolved.ImageID` (T014 passes)
- [x] T017 [US1] Add `resolveContainerImage(ctx, op *engine.CreateContainerOp) (string, error)` in `internal/engine/local/dockercontainer/docker_container.go`: return `op.ImageID` when set, else expand the alias into `op.Image` and return `locateImage(...).ID`; `CreateContainer` calls it in place of the old expansion plus `resolveImage` and feeds the id to `configHash` unchanged (T015 passes)
- [x] T018 [US1] `go test ./...` green; `gofmt -l .` clean; commit

**Checkpoint**: an unresolvable reference fails before any network or container operation; the Traefik plugin path is untouched

---

## Phase 4: User Story 2 - Deploy output states the exact version per artifact (Priority: P2)

**Goal**: the terminal renders `image.resolve` started and finished (source `manifest`) per contracts/operator-output.md

**Independent Test**: the scenario appended in T005 (CI); locally, the renderer unit tests are byte-exact

### Tests for User Story 2 (write FIRST; must fail before T021)

- [x] T019 [P] [US2] Add to `TestTerminalObserver_RendersEachKind` in `internal/ui/terminal_logger_test.go`: `image.resolve` started with `team=team-a name=web ref=nginx:1.27` renders `🔎 Resolving image for team-a.web (nginx:1.27)\n`; finished with `source=manifest digest=sha256:a1b2c3d4e5f6a7b8…` renders `  🔎 Resolved team-a.web nginx:1.27@a1b2c3d4e5f6\n`; a separate test: finished with `source=manifest` and empty `digest` renders `  🔎 Resolved team-a.web nginx:1.27\n`; add to `TestTerminalObserver_SilentKinds`: finished with `source=pinned` renders nothing; the error status renders only the generic `  ❌ Error [image.resolve]: boom\n` line and starts no indicator
- [x] T020 [P] [US2] Unit test for `shortDigest` in `internal/ui/terminal_logger_test.go`: `sha256:` prefix stripped and twelve characters kept; a value shorter than twelve returned whole; empty stays empty

### Implementation for User Story 2

- [x] T021 [US2] Add the `case "image.resolve"` arms and `shortDigest` to `internal/ui/terminal_logger.go`: started prints the resolving line as a plain line (one WHY comment: `image.pull` opens its own indicator inside the step, so this line must not be one); finished prints the resolved line only when `source == engine.ImageSourceManifest`, with `@<short>` only when `digest` is non-empty (T019, T020 pass)
- [x] T022 [US2] `go test ./...` green (the 33-kind count in `specs/027` is a historical note, not a test constraint); `gofmt -l .` clean; commit

**Checkpoint**: a real deploy prints the two lines per artifact before the first deploy header

---

## Phase 5: User Story 3 - Dry run shows the step and touches nothing (Priority: P3)

**Goal**: `deploy --dry-run` prints the resolution step before any operation line, with no Docker or state side effect

**Independent Test**: the dry-run scenario of T004 (CI); locally, build the binary and run quickstart steps 1 and 2 (dry run only; no daemon needed)

- [x] T023 [US3] Build the binary (`go build -o /tmp/claude-0/-root-projects-shrine/0ea10696-89e0-4875-9db6-31794c53e4ac/scratchpad/shrine .`), apply the `team` fixture into a scratch state dir, run `deploy --dry-run --path tests/testdata/deploy/resources` and confirm the two `ImageResolve` lines precede `CreatePlatformNetwork`; run it twice and confirm the state dir is byte-identical; run the alias dry run of quickstart step 2 and confirm `image=reg:myregistry/...` on both the `ImageResolve` and `ContainerCreate` lines
- [x] T024 [US3] Confirm no dry-run output line other than the added `ImageResolve` lines changed, by comparing the full dry-run output against the expected block in contracts/operator-output.md and the existing dry-run assertions in `tests/integration/registry_alias_test.go` and `tests/integration/deploy_team_infer_test.go`

**Checkpoint**: the preview shows the step in the order it runs

---

## Phase 6: User Story 4 - Healthy deploys behave exactly as before (Priority: P4)

**Goal**: pull semantics and the recreate decision are unchanged

**Independent Test**: the fixed-tag and `latest` scenarios of T004 and the unedited existing suites (CI); locally, the hash unit test

### Tests for User Story 4 (write FIRST; must fail before T026)

- [x] T025 [P] [US4] Unit test in `internal/engine/local/dockercontainer/docker_container_test.go`: `configHash(op, imageID)` for an op carrying `ImageID` equals the hash computed before this feature for the same env, volumes, ports, platform flag, and image id (call `state.ConfigHash` directly with the same inputs and compare), proving the hash input is the local image id (TD-2)

### Implementation for User Story 4

- [x] T026 [US4] Confirm by reading `CreateContainer` that `configHash` receives the id from `resolveContainerImage` on both paths and that no other hash input changed; adjust only if T025 fails
- [x] T027 [US4] `go test ./...` green; `git diff --stat main -- tests/integration/` shows only `preflight_image_resolve_test.go` added and `deploy_test.go` appended; commit

**Checkpoint**: nothing changes for manifests that do not opt in

---

## Phase 7: User Story 5 - Contributors see the step in the pipeline reference (Priority: P5)

**Goal**: `AGENTS.md` Deploy Pipeline diagram gains the pre-pass

**Independent Test**: read the diagram

- [x] T028 [US5] In `AGENTS.md` Deploy Pipeline: add `Container.ResolveImage()  ← pre-pass over every planned step, in step order` as the first child of `engine.ExecuteDeploy(steps, set)`, before `Container.CreatePlatformNetwork()`; change the `Container.CreateContainer(op)` note from `image pull, reconcile-by-name, multi-network attach` to `uses the pre-resolved image; reconcile-by-name, multi-network attach`; add one sentence under the diagram, before the resolver paragraph: a failure during image resolution stops the deploy before any network or container exists, so a bad reference changes nothing

**Checkpoint**: contributor reference matches the code

---

## Phase 8: Polish & Cross-Cutting Concerns

- [x] T029 Add the entry for 032 to `specs/progress.md` in the project's usual form (title, spec link, issue #53, what changed, acceptance SC-001 to SC-007 mapping, gate: `TestPreflightImageResolve` and the appended `TestDeploy` scenario, CI executes)
- [x] T030 Final verification: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/... && gofmt -l .` clean; every task above checked; `git status` clean after commit
- [x] T031 Rebase onto `origin/main` if it moved (expected conflicts only in `specs/progress.md` and `.specify/feature.json`; keep both entries), push `032-preflight-image-resolve`, open the pull request with `gh pr create` following `.github/pull_request_template.md` with `Closes #53` in Why, the definition-of-done list, and the hand-off notes of research.md R13
- [x] T032 Run `/shrine-pr-review` on the pull request, fix every real finding, push again (findings DRY1, DRY2, CC1, UT1 fixed: `newResolveImageOp` extracted, `recordingDockerAPI` embeds `fakeDockerAPI`, the `resolveContainerImage` comment trimmed, inspect-failure test added)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: depends on Phase 1; T004 to T006 before T007 (tests first); T007 before T008, T010, T012; T009 and T011 any time after T007; T013 closes the phase
- **US1 (Phase 3)**: depends on Phase 2; T014 and T015 before T016 and T017
- **US2 (Phase 4)**: depends on Phase 2 only (the backend already emits the events); T019 and T020 before T021
- **US3 (Phase 5)**: depends on Phase 3 (the engine must call the dry-run backend's `ResolveImage`)
- **US4 (Phase 6)**: depends on Phase 3
- **US5 (Phase 7)**: no code dependency; documents Phase 3
- **Polish (Phase 8)**: depends on everything above

### Parallel Opportunities

- T002 and T003 (fixtures)
- T008, T009, T011 after T007
- T014 and T015 (different test files)
- T019 and T020 (same file; parallel only as separate edits)
- US2 and US5 can proceed while US1 is in progress

---

## Implementation Strategy

### MVP First (User Story 1)

1. Phase 1, then Phase 2 (integration scenarios first, then the contract with both implementations).
2. Phase 3: the engine pre-pass and the `CreateContainer` skip. At this point an unresolvable reference already fails with zero changes, and the backend already emits the events the terminal will render.

### Incremental Delivery

- US2 adds the two renderer arms; US3 is a local dry-run check; US4 is a hash proof; US5 is the diagram. Each lands as its own commit on the same branch; the pull request carries all of them, per the ticket.

## Notes

- Integration scenarios are compiled (`go vet -tags integration`) and never run locally; another agent shares the daemon.
- Unit tests touch no filesystem: no `TempDir`, `MkdirAll`, or file writes.
- Every commit message ends with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.
- Never commit to or push `main`.
