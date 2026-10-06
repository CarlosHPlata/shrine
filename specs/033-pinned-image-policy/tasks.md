# Tasks: The Pinned Policy: Resolve Once, Keep the Exact Version

**Input**: Design documents from `/specs/033-pinned-image-policy/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: INCLUDED. The constitution mandates TDD (Principle V: integration test files are created before the implementation code) and the delivery plan requires the integration scenarios to be written before the implementation, compiled under the integration tag, and never run locally. Unit tests touch no filesystem and use in-memory file fakes, the existing `dockerAPI` fake, and a fake pin store. Iterate with `go test ./...`; compile the suite with `go vet -tags integration ./tests/integration/...`; CI is the integration gate.

**Organization**: Phase 2 holds the registry helper, the integration scenarios (written first), and the shared seams every story needs: the policy constant and reference helpers, the pin store, the backend result fields, and the planner normalisation. Phases 3 to 8 follow the spec's six user stories in priority order. The ticket ships as one pull request; the story phases are commit boundaries, not releases.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label (US1 to US6); Setup, Foundational, and Polish tasks carry none

## Phase 1: Setup

**Purpose**: Confirm a green baseline and create the one on-disk fixture the scenarios reference

- [x] T001 Verify green baseline on branch `033-pinned-image-policy`: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/...` pass with no file changes; confirm `specs/epics/pinned-image-versions/` is present
- [x] T002 [P] Create fixture `tests/testdata/pinned/fixed-version/` for owner `shrine-deploy-test` with three manifests: `app-fixed.yml` (Application `whoami-fixed`, image `127.0.0.1:5000/shrine/whoami:v1.10.2`, port 80, `imagePullPolicy: Pinned`), `res-fixed.yml` (Resource `cache-fixed`, type `cache`, version `"16"`, `imagePullPolicy: Pinned`), `res-unknown.yml` (Resource `cache-unknown`, type `cache`, version `"1"`, image `traefik/whoami`, `imagePullPolicy: Sometimes`). The registry host in `app-fixed.yml` never has to answer: validation fails before resolution

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The registry helper and integration scenarios, written first; then the shared seams, each with its unit tests ahead of the code, so every story builds on a compiling, green tree

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Integration scenarios (write FIRST; compile only, never run locally)

- [x] T003 Create `tests/integration/testutils/registry.go` (`//go:build integration`, package `testutils`) per research R12: `type LocalRegistry struct{ Host string; containerID string }`; `StartLocalRegistry(tc *TestCase) *LocalRegistry` pulls `registry:2` when `ImageInspect` reports not-found, creates a container named `shrine-test-registry-<random suffix>` with `5000/tcp` bound to `127.0.0.1` port `0`, starts it, reads the mapped port from `ContainerInspect(...).NetworkSettings.Ports`, polls `GET http://127.0.0.1:<port>/v2/` until it answers (deadline 30 s), sets `Host`, and registers `ContainerRemove(Force: true)` with `tc.t.Cleanup`; `(r *LocalRegistry) PushAs(tc *TestCase, source, repoTag string) string` pulls `source` when absent, `ImageTag`s it as `r.Host + "/" + repoTag`, `ImagePush`es with `RegistryAuth` = base64 of `{}` and drains the stream, then returns the `sha256:…` digest found in `ImageInspect(r.Host+"/"+repoTag).RepoDigests` for that repository; `(tc *TestCase) ImageIDOf(ref string) string` (`ImageInspect(...).ID`, fatal on error); `(tc *TestCase) RemoveImage(ref string)` (`ImageRemove` with `Force: true, PruneChildren: true`, fatal on error); `WritePinnedFixture(tc *TestCase, dir, host, envValue string)` writes `app.yml` (Application `whoami-pinned`, owner `shrine-deploy-test`, image `<host>/shrine/whoami`, port 80, `imagePullPolicy: Pinned`, env `ROUND` = `envValue`) and `resource.yml` (Resource `cache-pinned`, owner `shrine-deploy-test`, type `cache`, no `version`, image `<host>/shrine/whoami`, `imagePullPolicy: Pinned`) into `dir`
- [x] T004 Create `tests/integration/pinned_image_policy_test.go` (`//go:build integration`, `NewDockerSuite(t, testTeam)`; BeforeEach sets `tc.StateDir`, seeds subnet state, applies the `team` fixture, starts the registry, pushes `traefik/whoami:v1.10.1` as `shrine/whoami:latest` and keeps the returned digest and `ImageIDOf("traefik/whoami:v1.10.1")`, writes the pinned fixture into `tc.TempDir()`; helpers `pinnedDeploy(tc, dir, flags...)`, `pinsPath(tc)` = `<StateDir>/shrine-deploy-test/pins.txt`, `readFile(tc, path)`) with these scenarios, each cited by spec id: (US2) `deploy --dry-run --path tests/testdata/pinned/fixed-version` fails, stderr contains `application "whoami-fixed": spec.image "127.0.0.1:5000/shrine/whoami:v1.10.2" names a fixed version but the image pull policy is Pinned`, `resource "cache-fixed": spec.version "16" names a fixed version but the image pull policy is Pinned`, and `spec.imagePullPolicy must be one of Always, IfNotPresent, Pinned`; `pins.txt` does not exist; (US1, SC-001) first deploy succeeds, output contains `📌 Pinned shrine-deploy-test.whoami-pinned at latest@` and the same for `cache-pinned`, `pins.txt` has exactly two lines each containing `<host>/shrine/whoami@<digest>`, `AssertContainerImage` of both containers equals `<host>/shrine/whoami@<digest>`; then `PushAs("traefik/whoami:v1.10.2", "shrine/whoami:latest")` and a loop of ten cycles over `[redeploy, recreate (rewrite fixture with ROUND=2 then 3…), teardown+deploy, RemoveImage(pinned ref)+deploy, redeploy, recreate, teardown+deploy, RemoveImage+deploy, redeploy, redeploy]`, asserting after every cycle `AssertContainerImage(... <host>/shrine/whoami@<digest>)` and `ContainerInspect(...).Image == imageID of v1.10.1`, that plain redeploys print `📌 Using pinned` and no `Pulling image`, that the wiped-cache cycles print `Pulling image <host>/shrine/whoami@<digest>`, and that the container id is unchanged across a plain redeploy and changed across a recreate; (US3, SC-006) after teardown, `delete application whoami-pinned --dry-run` prints `[dry-run] would release image pin <host>/shrine/whoami@<digest> for shrine-deploy-test/whoami-pinned` and `pins.txt` is unchanged; `delete application whoami-pinned` prints `Released image pin for shrine-deploy-test/whoami-pinned.` and the application line is gone; deploy prints `📌 Pinned` for the application with `v1.10.2`'s digest and `📌 Using pinned` for the resource; after teardown, rewriting `app.yml` to image `<host>/shrine/whoami:v1.10.2` with no policy and deploying prints `🔎 Resolved shrine-deploy-test.whoami-pinned` and removes the line; restoring `Pinned` and deploying prints `📌 Pinned` again; after teardown, `delete team shrine-deploy-test` prints `Released 2 image pin(s) for team "shrine-deploy-test".` and `pins.txt` has no lines (reapply the team fixture afterwards); (US4, SC-004) with no pins, `deploy --dry-run` output contains `policy=Pinned -> would resolve newest and pin` for both artifacts; after a deploy, it contains `-> pinned <host>/shrine/whoami@<digest> (latest, ` and running it twice leaves `pins.txt` and `deployments.txt` byte-identical and prints no `📌`; (US5, SC-005) after teardown, rewrite the application's `pins.txt` line so the fourth field is `<host>/shrine/whoami@sha256:` + 64 zeros, deploy fails, stderr contains `is no longer served by the registry` and `application "whoami-pinned"` and `deploy the application under Always or IfNotPresent`, `AssertContainerNotExists` for both containers and `AssertNetworkNotExists("shrine.shrine-deploy-test.private")`; (FR-015) the `resources` fixture deploys with output identical in shape to T2's and `pins.txt` absent
- [x] T005 Compile the suite: `go vet -tags integration ./tests/integration/...` is green (the new files use only the Docker SDK and `testutils` helpers; unused-import and signature errors fixed here, never by running the suite)

### Shared seams (tests first, then code; the tree is green again before any story)

- [x] T006 [P] Unit tests in `internal/manifest/types_test.go`: `TagOf` table (`nginx:1.27` → `1.27`, `nginx` → ``, `127.0.0.1:5000/app` → ``, `127.0.0.1:5000/app:v1` → `v1`, `repo@sha256:abc` → ``, `ghcr.io/me/app:latest` → `latest`); `IsDigestReference` (`repo@sha256:abc` true, `repo:1.2` false); `IsKnownPullPolicy` (three true, `` false, `Sometimes` false); `IsManifestOwnedPolicy` (`Always`, `IfNotPresent` true; `Pinned`, `` false); `EffectivePullPolicy("127.0.0.1:5000/app", "")` → `Always` (the derived-rule fix of research R6) and the existing cases unchanged; `EffectivePullPolicyWithDefault` precedence (declared wins, then default, then derived)
- [x] T007 Add to `internal/manifest/types.go`: `ImagePullPolicyPinned = "Pinned"`, `IsKnownPullPolicy`, `IsManifestOwnedPolicy`, `TagOf` (a colon counts as the tag separator only after the last slash; a `@` suffix is cut first), `IsDigestReference`, `EffectivePullPolicyWithDefault(image, declared, dflt)`; `EffectivePullPolicy(image, declared)` delegates with an empty default and uses `TagOf` (T006 passes)
- [x] T008 [P] Unit tests in `internal/manifest/parser_test.go`: a Resource with `type: postgres` and no `version` parses with `Spec.Image == "postgres"`; with `version: "16"` still `postgres:16`; with an explicit `image` the override is kept in both cases. Unit tests in `internal/manifest/validate_test.go`: a Resource without `version` passes `Validate` (the check moved to the planner); the existing resource and application validation cases are unchanged
- [x] T009 In `internal/manifest/parser.go` default the Resource image to `<type>` when `version` is empty and `type` is set; in `internal/manifest/validate.go` remove the `spec.version is required` check from `validateResourceSpec` (T008 passes). The enum check is US2's.
- [x] T010 [P] Unit tests in new `internal/state/local/pins_test.go` over an in-memory file fake modelled on `fakeHostPortFile` (no filesystem): missing file is an empty team; a file with a `#` comment, a blank line, a four-field line, a line whose date is not RFC 3339, a line whose fourth field lacks `@sha256:`, and two valid lines loads exactly the two; `Put` inserts, replaces by name, writes the five-field sorted format byte-exact with the `2006-01-02T15:04:05Z` date; `Get` returns `ErrImagePinNotFound` when absent; `Release` of an absent name writes nothing (write count unchanged) and of a present name removes the line; `ReleaseTeam` empties the team and is idempotent; `List` is sorted by name; `ListAll` is keyed `team/name` across two teams; a write error leaves the in-memory state unchanged
- [x] T011 Create `internal/state/pins.go` (`ImagePin`, `ImagePinStore`, `ErrImagePinNotFound`, `ImagePinKey`) per data-model section 2, and `internal/state/local/pins.go` implementing it per contracts/state-and-docs.md: `NewImagePinStore(baseDir)` and `newImagePinStoreWithFileOps(baseDir, read, write)`, `sync.Mutex`, `os.ReadFile` and the existing `writeTeamFile`, per-team load on demand (`<baseDir>/<team>/pins.txt`), forgiving reader, sorted writer; add `ImagePins ImagePinStore` to `state.Store` in `internal/state/state.go` and construct it in `internal/state/local/local.go` (T010 passes)
- [x] T012 Add to `internal/engine/backends.go`: `ResolvedImage.Requested string` and `ResolvedImage.PinnedAt time.Time`; `ImageSourceResolved = "resolved"` and `ImageSourcePinned = "pinned"` beside `ImageSourceManifest` (contract: contracts/backend-contract.md). Existing fakes need no change
- [x] T013 [P] Unit tests in new `internal/planner/policy_test.go`: `applyEffectivePullPolicy` leaves a declared policy alone, fills `Pinned` from a `Pinned` default, and fills the derived rule (`Always` for `web:latest` and `web`, `IfNotPresent` for `web:1.2`) when both are empty, for Applications and Resources alike; `Plan` with a `Pinned` application and `""` default leaves `Spec.ImagePullPolicy == "Pinned"` on the set in `PlanResult.ManifestSet`
- [x] T014 Thread the default policy: `Plan(set, store, registries, ports, filter, defaultPullPolicy string)` in `internal/planner/plan.go` calls `applyEffectivePullPolicy(set, defaultPullPolicy)` before `Resolve`; `Resolve(set, store, registries, defaultPullPolicy string)` in `internal/planner/resolve.go` forwards it to `validateImagePolicies` (added as a stub returning nil in this task, filled in US2); create `internal/planner/policy.go` with `applyEffectivePullPolicy`; update every call site to pass `""`: `internal/handler/deploy.go` (both), `internal/handler/apply.go`, `internal/planner/plan_test.go`, `internal/planner/plan_hostports_test.go`, `internal/planner/resolve_test.go` (T013 passes)
- [x] T015 Green checkpoint: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/... && gofmt -l .` all clean; commit

**Checkpoint**: `Pinned` parses and reaches the engine as the effective policy; the pin store exists and is wired; nothing pins yet

---

## Phase 3: User Story 1 - Freeze what you got (Priority: P1) 🎯 MVP

**Goal**: the Docker backend's `ResolveImage` pins on the first `Pinned` deploy and reuses the pin on every later one, pulling by digest only when the image is absent locally

**Independent Test**: the first-deploy and ten-cycle scenarios of T004 (CI); locally, the backend unit tests over the scripted Docker fake and fake pin store

### Tests for User Story 1 (write FIRST; must fail before T017)

- [x] T016 [P] [US1] Unit tests in `internal/engine/local/dockercontainer/docker_image_test.go` over a scripted `dockerAPI` fake (per-reference `ImageInspect` results and errors, `ImagePull` results, call recording; extend the existing `recordingDockerAPI`) and a fake `state.ImagePinStore` recording `Put`/`Release` calls, with `backend.now` fixed to `2026-10-06T10:42:17Z`: (a) `Pinned`, no pin, pull of `ghcr.io/me/app:latest` then inspect yielding `RepoDigests: ["ghcr.io/me/app@sha256:<64a>"]` → `Put` called once with `{Kind, Name, Requested: "ghcr.io/me/app:latest", Pinned: "ghcr.io/me/app@sha256:<64a>", PinnedAt: now}`, result `Ref == Pinned`, `Digest == "sha256:<64a>"`, `Source == ImageSourceResolved`, `Requested == "ghcr.io/me/app:latest"`, finished event fields `source=resolved`, `requested`; `ImageList` never called even though the tag is present locally; (b) `Pinned`, pin on record, `ImageInspect(pin.Pinned)` succeeds → no `ImagePull`, no `Put`, result `Ref == pin.Pinned`, `Source == ImageSourcePinned`, `PinnedAt == pin.PinnedAt`, finished event has `pinned_at=2026-10-06`; (c) `Pinned`, pin on record, inspect not-found then pull of `pin.Pinned` succeeds and second inspect succeeds → `ImagePull` called with the digest reference, `image.pull` started/finished carry it, `Source == pinned`; (d) `Pinned`, no pin, inspect yields no matching digest → error text `image "ghcr.io/me/app:latest" carries no registry digest and cannot be pinned`, no `Put`; (e) `Pinned`, pin on record whose `Requested` is `ghcr.io/other/app:latest` while the manifest names `ghcr.io/me/app` → behaves as (a) and `Put` replaces it; (f) a `reg:myregistry/app` image under `Pinned` records `Requested` expanded; (g) `Always` and `IfNotPresent` behave exactly as the existing T2 tests assert (unchanged)
- [x] T017 [US1] Implement the pinned branches in `internal/engine/local/dockercontainer/docker_image.go` per contracts/backend-contract.md: `ResolveImage` dispatches after the started event on `manifest.IsManifestOwnedPolicy(op.ImagePullPolicy)` to `resolveManifestOwned` (today's body, release added in US3) or to the pinned path: `usablePin(op, ref)` (`Get`, `ErrImagePinNotFound` → absent, `sameRepository(pin.Requested, ref)`), `reusePin(ctx, op, pin)` (`inspectImage(pin.Pinned)`; `errdefs.IsNotFound` → `pullImage(pin.Pinned)` then inspect; other inspect errors returned as today), `pinNewest(ctx, op, ref)` (`pullImage`, `inspectImage`, `pickRepoDigest`, empty → the no-digest error, `Put` with `backend.now()`, error wrap `recording image pin for <team>/<name>: %w`), `sameRepository(a, b)` over `repositoryOf` and `normalizeRepository`, `pinnedReference(ref, digest)` = `repositoryOf(ref) + "@" + digest`; add `now func() time.Time` to `DockerBackend` in `docker_backend.go`, defaulting to `func() time.Time { return time.Now().UTC() }` in `NewDockerBackend`; finished event gains `requested` and, for `pinned`, `pinned_at` (T016 passes; the "no longer served" wording and backend error events are US5's, so (c)'s failure path may return the raw pull error for now)
- [x] T018 [US1] `go test ./...` green; `gofmt -l .` clean; commit

**Checkpoint**: a `Pinned` manifest pins on first deploy and every later deploy runs the pinned digest; the terminal prints nothing yet for the two new sources

---

## Phase 4: User Story 2 - A manifest can ask for the pinned policy, and cannot contradict it (Priority: P2)

**Goal**: the enum check rejects unknown values at parse time; the planner rejects a fixed version under `Pinned` and still requires a Resource version under the other two, with the manifest's other errors

**Independent Test**: the validation scenario of T004 (CI); locally, quickstart step 1 against the `fixed-version` fixture

### Tests for User Story 2 (write FIRST; must fail before T021 and T022)

- [x] T019 [P] [US2] Unit tests in `internal/manifest/validate_test.go`: an Application and a Resource with `imagePullPolicy: Sometimes` each fail `Validate` with `spec.imagePullPolicy must be one of Always, IfNotPresent, Pinned`; each of the three values passes; an empty value passes
- [x] T020 [P] [US2] Unit tests in `internal/planner/policy_test.go` for `validateImagePolicies(set, "")` byte-exact per contracts/operator-output.md: `Pinned` application `repo:1.2` → `application "x": spec.image "repo:1.2" names a fixed version but the image pull policy is Pinned; use "repo" or "repo:latest"`; `Pinned` application `repo@sha256:…` → same shape with `use "repo" or "repo:latest"`; `Pinned` application `repo` and `repo:latest` → no error; `Pinned` resource version `16` → `resource "db": spec.version "16" names a fixed version but the image pull policy is Pinned; omit it or use "latest"`; `Pinned` resource with version `latest` or empty → no error; `Pinned` resource with image override `postgres:16` → `resource "db": spec.image "postgres:16" names a fixed version but the image pull policy is Pinned; use "postgres" or "postgres:latest"`; `IfNotPresent` resource with empty version → `resource "db": spec.version is required`; a set with two violations reports both; `Resolve` on a `Pinned` application with a fixed tag and an undefined `reg:` alias reports both errors in one slice
- [x] T021 [US2] Add `validatePullPolicy(policy string) []string` to `internal/manifest/validate.go` and call it from `validateApplicationSpec` and `validateResourceSpec` (T019 passes)
- [x] T022 [US2] Implement `validateImagePolicies(set, defaultPullPolicy) []error` in `internal/planner/policy.go` per data-model section 1.2 with named helpers (`fixedVersionImageError(kind, name, image)`, `fixedVersionResourceVersionError(name, version)`, `repositoryWithoutVersion(image)` using `manifest.TagOf` and `manifest.IsDigestReference`); `defaultPullPolicy` is accepted and unused (one WHY comment: T4 needs it to tell a configuration-sourced policy apart); replace the stub from T014 (T020 passes)
- [x] T023 [US2] Build the binary into the scratchpad, apply the `team` fixture into a scratch state dir, run quickstart step 1 (`deploy --dry-run --path tests/testdata/pinned/fixed-version`) and confirm the three error lines of contracts/operator-output.md and a non-zero exit; run `deploy --dry-run --path tests/testdata/deploy/resources` and confirm the output is unchanged from T2's (FR-015)
- [x] T024 [US2] `go test ./...` green; `gofmt -l .` clean; commit

**Checkpoint**: a `Pinned` manifest can never also name a version; untouched manifests validate as before

---

## Phase 5: User Story 3 - Pins outlive containers and are released only on purpose (Priority: P3)

**Goal**: a manifest-owned resolution releases the pin; `delete application` and `delete team` release pins and preview the release under `--dry-run`

**Independent Test**: the release-path scenarios of T004 (CI); locally, the backend and handler unit tests

### Tests for User Story 3 (write FIRST; must fail before T027 and T028)

- [ ] T025 [P] [US3] Unit tests in `internal/engine/local/dockercontainer/docker_image_test.go`: `Always` with a pin on record → `Release(team, name)` called once after the image is located, result `Source == manifest`; `IfNotPresent` with no pin → `Release` called (idempotent, no error); a `Release` error → `ResolveImage` returns `releasing image pin for <team>/<name>: <cause>`; a nil `state.ImagePins` → no panic, no release attempted
- [ ] T026 [P] [US3] Unit tests in `internal/handler/deployments_test.go` and `internal/handler/teams_test.go` over the handler's stub store extended with an in-memory `ImagePinStore`: `DeleteApplication` with a pin and no container prints `Released image pin for team/name.` and the pin is gone; with `DryRun` prints `[dry-run] would release image pin <pinned> for team/name` and the pin stays; with neither port, record, nor pin prints the existing "nothing to delete" line; with a pin only, does not print "nothing to delete"; `DeleteTeam` with two pins prints `Released 2 image pin(s) for team "name".` and `ListAll` has no `name/` keys; with none prints no pin line; a nil `ImagePins` on the store is tolerated by both
- [ ] T027 [US3] In `internal/engine/local/dockercontainer/docker_image.go` add `releasePin(op) error` (nil store → nil; `Release`; wrap as `releasing image pin for %s/%s: %w`) and call it at the end of `resolveManifestOwned` (one WHY comment citing TD-6) (T025 passes)
- [ ] T028 [US3] In `internal/handler/deployments.go` `DeleteApplication`: read `store.ImagePins.Get(team, name)` (nil store or `ErrImagePinNotFound` → no pin) after the host port lookup; add the dry-run line and the release (`Release`, wrap `releasing image pin for %s: %w`) between the host-port and deployment-record blocks; include the pin in the "nothing to delete" condition. In `internal/handler/teams.go` add `releaseTeamImagePins(store, team) (int, error)` modelled on `releaseTeamHostPorts` (count `ListAll` keys with the `team/` prefix, then `ReleaseTeam`) and call it in `DeleteTeam` after host ports, printing `Released %d image pin(s) for team %q.` when positive. Update `cmd/delete.go` `deleteApplicationCmd` `Short`/`Long` to mention the image pin (T026 passes)
- [ ] T029 [US3] `go test ./...` green; `gofmt -l .` clean; commit

**Checkpoint**: every release path of the spec exists; redeploy, recreation, and teardown never touch `pins.txt`

---

## Phase 6: User Story 4 - The operator can see what each deploy decided, before and after it runs (Priority: P4)

**Goal**: the terminal renders the pinned-now and reused lines; dry run previews would-pin and reuse from a read-only snapshot and writes nothing

**Independent Test**: the dry-run scenarios of T004 (CI); locally, the renderer and dry-run unit tests, then quickstart step 2

### Tests for User Story 4 (write FIRST; must fail before T032 and T033)

- [ ] T030 [P] [US4] Unit tests in `internal/ui/terminal_logger_test.go`: finished `image.resolve` with `source=resolved team=team-a name=web ref=ghcr.io/me/web@sha256:3f2a9c1b4d7e… digest=sha256:3f2a9c1b4d7e… requested=ghcr.io/me/web:latest` renders `  📌 Pinned team-a.web at latest@3f2a9c1b4d7e\n`; `source=pinned` with `pinned_at=2026-10-06` and `requested=postgres:17` renders `  📌 Using pinned team-a.db 17@9c1b4d7e3f2a (since 2026-10-06)\n`; `requested=postgres` (untagged) renders `latest@…`; `requested=postgres@sha256:…` renders the twelve hex alone (`  📌 Using pinned team-a.db 9c1b4d7e3f2a (since …)`); `source=manifest` is unchanged; remove `source=pinned` from the silent-kinds test; `readableVersion` table
- [ ] T031 [P] [US4] Unit tests in `internal/engine/dryrun/dry_run_container_test.go`: `ResolveImage` under `Pinned` with no entry in `Pins` prints `[DOCKER] ImageResolve: name=demo.web image=ghcr.io/me/web policy=Pinned -> would resolve newest and pin\n`; with `Pins["demo/web"] = {Requested: "ghcr.io/me/web:latest", Pinned: "ghcr.io/me/web@sha256:<64>", PinnedAt: 2026-10-06T10:42:17Z}` prints `… policy=Pinned -> pinned ghcr.io/me/web@sha256:<64> (latest, 2026-10-06)\n`; manifest-owned prints T2's line unchanged; the returned `ResolvedImage` has `Ref == op.Image` and `ImageID == ""` in all three cases; `NewDryRunEngine(out, nil, pins)` sets `Pins` on the container backend
- [ ] T032 [US4] In `internal/ui/terminal_logger.go` add the `resolved` and `pinned` arms under `case "image.resolve"` and `readableVersion(requested, digest string) string` (`shortDigest(digest)` alone when `manifest.IsDigestReference(requested)`; else `tag@short` with `latest` for an empty `manifest.TagOf`) (T030 passes)
- [ ] T033 [US4] In `internal/engine/dryrun/dry_run_container.go` add `Pins map[string]state.ImagePin` and the two pinned lines in `ResolveImage` (`readable` computed as in T032, duplicated as a two-line private helper rather than importing `ui`); change `NewDryRunEngine(out io.Writer, hostPorts state.HostPortMap, pins map[string]state.ImagePin)` in `dry_run_engine.go`; in `internal/handler/deploy.go` `DryRun` build the snapshot with a small `imagePinSnapshot(store)` helper (nil store → empty map; `ListAll` error returned) and pass it (T031 passes)
- [ ] T034 [US4] Build the binary into the scratchpad and run quickstart step 2: `deploy --dry-run` of a `Pinned` manifest prints `-> would resolve newest and pin` before `CreatePlatformNetwork`, creates no `pins.txt`, and two runs leave the state dir byte-identical; run `deploy --dry-run --path tests/testdata/deploy/resources` and confirm the output is unchanged from T2's
- [ ] T035 [US4] `go test ./...` green; `gofmt -l .` clean; commit

**Checkpoint**: a real deploy says pinned, reused, or manifest-owned per artifact; dry run previews without writing

---

## Phase 7: User Story 5 - A pin the registry no longer serves stops the deploy before anything changes (Priority: P5)

**Goal**: the pull-by-digest failure carries the "no longer served" message and the pinned failures emit a backend `image.resolve` error event

**Independent Test**: the no-longer-served scenario of T004 (CI); locally, the backend unit tests

### Tests for User Story 5 (write FIRST; must fail before T037)

- [ ] T036 [P] [US5] Unit tests in `internal/engine/local/dockercontainer/docker_image_test.go`: `Pinned`, pin on record, inspect not-found, pull of `pin.Pinned` fails with `manifest unknown` → error text equals `pinned exact version "ghcr.io/me/app@sha256:<64>" for team-a/web is no longer served by the registry; deploy the application under Always or IfNotPresent to release the pin, then return to Pinned: pulling image "ghcr.io/me/app@sha256:<64>": manifest unknown` (kind lower-cased from `op.Kind`), `errors.Is` the pull cause, an `image.resolve` error event was emitted with `team`, `name`, `ref == pin.Pinned`, `error`, and no finished event; the no-digest failure of T016 (d) also emits an `image.resolve` error event with `ref` = the tag reference; no `image.resolve` error event is emitted for manifest-owned failures (T2 behaviour kept)
- [ ] T037 [US5] In `internal/engine/local/dockercontainer/docker_image.go` add `notServedError(op, pin, cause) error` producing the message of contracts/operator-output.md and emit it through `emitErr("image.resolve", {team, name, ref: pin.Pinned}, …)` in `reusePin`; emit the no-digest error through `emitErr("image.resolve", {team, name, ref})` in `pinNewest` (T036 passes)
- [ ] T038 [US5] `go test ./...` green; `gofmt -l .` clean; commit

**Checkpoint**: an unservable pin fails in the pre-pass with a message that names the artifact, the exact version, and the way out

---

## Phase 8: User Story 6 - The documentation explains the policy and the pin (Priority: P6)

**Goal**: the manifest reference and the contributor reference describe the three values, the no-fixed-version rule, the pin lifecycle, and the pin record

**Independent Test**: read both documents against spec SC-009

- [ ] T039 [P] [US6] Update `docs/content/reference/manifest-schema.md` per contracts/state-and-docs.md: both YAML blocks read `imagePullPolicy: <Always|IfNotPresent|Pinned>`; the Resource block's `version` comment becomes `# required unless imagePullPolicy is Pinned`; the Resource `spec.version` row's Required cell becomes `yes, unless `imagePullPolicy` is `Pinned``; both `spec.imagePullPolicy` rows describe the three values and link to `#image-pull-policy`; add `### Image pull policy` after `### spec.env[]` with the three modes and the derived default, the `Pinned` rule for Application image, Resource version, and Resource image override with the two error shapes quoted, the pin lifecycle (created on first deploy; reused across redeploy, recreation, teardown, and a wiped image cache; released only by `shrine delete application`, `shrine delete team`, or a deploy under `Always` or `IfNotPresent`), and the pre-deploy failure when the registry no longer serves the exact version. Run `make docs-check` if it exists in the Makefile, else skip
- [ ] T040 [P] [US6] Update `AGENTS.md`: under `<team>/` in State Directory Layout add `│   ├── pins.txt                 # image pins (<kind> <name> <requested> <pinned> <pinned-at>); survive teardown, released by delete and by a manifest-owned deploy`; in `### shrine delete application <name>` add that it releases the application's image pin and that `shrine delete team <name>` releases every pin the team held; in the paragraph after the Deploy Pipeline diagram add one sentence: under `Pinned`, `ResolveImage` reuses the recorded exact version (pulling it by digest only when absent locally) or resolves the newest and records it in `pins.txt`, and a manifest-owned resolution releases any pin

**Checkpoint**: an operator can answer "how do I pin, what releases it" from the manifest reference alone

---

## Phase 9: Polish & Cross-Cutting Concerns

- [ ] T041 Record the ten deviations of contracts/state-and-docs.md in `specs/epics/pinned-image-versions/design.md`: an "*Amended by T3 (spec 033)*" note in section 1 (enum check absent on `main`; `EffectivePullPolicy` tag parsing), section 3.5 (untagged readable is `latest`), section 4.1 (`Requested`, `PinnedAt`), section 4.2 (repository-mismatch re-pin; the T3 message wording; both pinned failures emit a backend error event), section 4.4 (full digest reference on the dry-run line), section 4.5 (manifest-sourced Resource wording; helpers in `policy.go`), and section 5 (whoami tags; helper names; run-time fixture)
- [ ] T042 Add the entry for 033 to `specs/progress.md` in the form of the 031 and 032 entries: title, spec link, issue #54, epic and ticket, what changed, acceptance SC-001 to SC-009 mapping, gate `TestPinnedImagePolicy` (CI executes)
- [ ] T043 Run `graphify update .` and commit the refreshed `graphify-out/`
- [ ] T044 Final verification: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/... && gofmt -l .` clean; `git diff --stat main -- tests/integration/` shows only `pinned_image_policy_test.go` and `testutils/registry.go` added, no existing suite edited; every task above checked; `git status` clean after commit
- [ ] T045 Rebase onto `origin/main` if it moved (expected conflicts only in `specs/progress.md`, `.specify/feature.json`, and `CLAUDE.md`; keep both entries), push `033-pinned-image-policy`, open the pull request with `/speckit-git-pr` following `.github/pull_request_template.md`: `Closes #54` in Why, the definition-of-done list of the issue, the `TagOf` behaviour change named explicitly, the deviations list, and the hand-off notes of research.md R16
- [ ] T046 Run `/shrine-pr-review` on the pull request, fix every real finding, push again, confirm CI is green (the integration job runs the registry-backed suite)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: depends on Phase 1; T003 before T004; T004 before T005 (scenarios first); T006 before T007; T008 before T009; T010 before T011; T012 any time; T013 before T014; T007 before T009, T013, T014 (they use the new helpers); T011 before T014 (handlers reference `store.ImagePins` only in US3, so T011 may also run in parallel with T013); T015 closes the phase
- **US1 (Phase 3)**: depends on Phase 2; T016 before T017
- **US2 (Phase 4)**: depends on Phase 2; T019 and T020 before T021 and T022; T023 after T022
- **US3 (Phase 5)**: depends on Phase 3 (the `resolveManifestOwned` split exists); T025 and T026 before T027 and T028
- **US4 (Phase 6)**: depends on Phase 3 (the events carry `requested` and `pinned_at`); T030 and T031 before T032 and T033; T034 after T033
- **US5 (Phase 7)**: depends on Phase 3 (`reusePin` and `pinNewest` exist); T036 before T037
- **US6 (Phase 8)**: no code dependency; may run any time after Phase 2 fixes the names
- **Polish (Phase 9)**: depends on everything above; T041 to T043 before T044; T044 before T045; T045 before T046

### Parallel Opportunities

- T006, T008, T010, T012, T013 (different files) after T003 to T005
- T016 is the only US1 test; US2's T019 and T020 can be written while US1 is implemented
- T025 and T026; T030 and T031; T039 and T040
- US2, US6 can proceed while US1 is in progress; US3, US4, US5 wait for US1 and are then independent of each other

---

## Implementation Strategy

### MVP First (User Story 1)

1. Phase 1, then Phase 2: the registry helper and the integration suite first, then the policy constant and reference helpers, the parser and validator changes, the pin store, the backend result fields, and the planner normalisation.
2. Phase 3: the pinned branches in `ResolveImage`. At this point a `Pinned` manifest pins and reuses, which is goals G1 and G5 end to end for a healthy registry; the suite's first-deploy and ten-cycle scenarios are satisfiable.

### Incremental Delivery

- US2 makes the rule enforceable; US3 adds the three release paths; US4 makes the decisions visible in deploy output and dry run; US5 fixes the failure wording and events; US6 documents. Each lands as its own commit on the same branch; the pull request carries all of them, per the ticket.

## Notes

- Integration scenarios are compiled (`go vet -tags integration`) and never run locally; another agent shares the daemon. The suite starts its own registry; the daemon on CI needs no configuration for `127.0.0.1`.
- Unit tests touch no filesystem: no `TempDir`, `MkdirAll`, or file writes; the pin store is tested through `newImagePinStoreWithFileOps`.
- Decisions TD-1 to TD-13 are settled; a disagreement goes to issue #54, not into the code.
- Every commit message ends with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.
- Never commit to or push `main`.
