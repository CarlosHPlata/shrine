# Tasks: Pinned Versions in `get`, `describe`, and `status`

**Input**: Design documents from `/specs/035-pinned-version-queries/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: INCLUDED. The constitution mandates TDD (Principle V: integration test files are created before the implementation code) and the ticket's definition of done requires the integration scenarios to be written before the implementation, compiled under the integration tag, and never run locally. Unit tests touch no filesystem: the renderers are pure functions over structs, the describe flow runs on the in-memory stores and stub backend already in `internal/handler/deployments_test.go`, and the moved helpers are tested on strings.

**Organization**: Phase 1 confirms the baseline. Phase 2 holds the integration scenarios (written first) and the two seams every story reads: the readable-form helpers in `internal/manifest` and `ContainerInfo.Image`. Phases 3 to 7 follow the spec's five user stories in priority order. The ticket is delivered as one pull request, so stories land as commits on the same branch. Nothing in this feature writes state.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label (US1 to US5); Setup, Foundational, and Polish tasks carry none

## Phase 1: Setup

**Purpose**: Confirm a green baseline; no fixture directory is needed (the pinned world is built at run time by `newPinnedSuite`, the manifest-owned rows use `basic` and `resources`)

- [x] T001 Verify green baseline on branch `035-pinned-version-queries`: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/...` pass with no file changes; confirm `specs/epics/pinned-image-versions/` is present and `main` holds #63 (`git log --oneline -1 origin/main`); confirm `grep -n 'ImagePins' internal/state/local/local.go` shows the pin store is wired into `NewLocalStore`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The integration scenarios, written first; then the shared readable form and the backend field that every story reads

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Integration scenarios (write FIRST; compile only, never run locally)

- [ ] T002 Create `tests/integration/pinned_version_queries_test.go` (`//go:build integration`, package `integration_test`) with `TestPinnedVersionQueries` on `s, worlds := newPinnedSuite(t)` (the loopback registry world of `pinned_image_policy_test.go`; reuse `pinnedDeploy`, `pinnedTeardown`, `pinsPath`, `readFileOrEmpty`, `nonEmptyLines`, `pinnedApp`, `pinnedResource`, `pinnedSourceNew`, `pinnedRepoTag`, `w.pinnedRef()`, `w.oldDigest`, and `assertColumnsInOrder` from `get_test.go`). Add one helper `replacePinDigest(tc *TestCase, old, new string)` that reads `pinsPath(tc)`, replaces every `old` with `new`, and writes the file back (0o644). Add `shortHex(digest string) string` returning the first twelve characters after `sha256:`. Scenarios, each starting with `w := worlds[tc]` and `pinnedDeploy(tc, w.specsDir).AssertSuccess()`: (1) "get shows the readable form for pinned rows and the manifest reference for others" (FR-001, FR-002, SC-001): run `get deployed --state-dir <state>`, assert `AssertOutputLineContains("whoami-pinned", "latest@"+shortHex(w.oldDigest))` and the same for `cache-pinned`, `AssertOutputNotContains(w.oldDigest)` (the full digest never appears in the table), `assertColumnsInOrder(tc, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")`; repeat for `get apps --team shrine-deploy-test` (whoami-pinned row) and `get resources` (cache-pinned row); then deploy `fixturesPath("basic")` on top and assert `get deployed` shows `AssertOutputLineContains("whoami ", "traefik/whoami")` for the manifest-owned app beside the pinned rows. (2) "describe shows the pin and the running image in agreement" (FR-004, FR-005, SC-002): run `describe app whoami-pinned --state-dir <state>` and assert success, `AssertOutputLineContains("Pull policy:", "Pinned")`, `AssertOutputLineContains("Pinned:", w.pinnedRef())`, `AssertOutputLineContains("Pinned:", "latest@"+shortHex(w.oldDigest))`, `AssertOutputLineContains("Pinned:", time.Now().UTC().Format("2006-01-02"))`, `AssertOutputLineContains("Running image:", w.pinnedRef())`; repeat for `describe resource cache-pinned --team shrine-deploy-test`. (3) "a pin replaced without a deploy shows as a difference" (FR-005, SC-002): `newDigest := w.registry.PushAs(tc, pinnedSourceNew, pinnedRepoTag)`, `replacePinDigest(tc, w.oldDigest, newDigest)`, run `describe app whoami-pinned` and assert `AssertOutputLineContains("Pinned:", newDigest)`, `AssertOutputLineContains("Running image:", w.oldDigest)`, and that the `Pinned:` line does not contain `w.oldDigest` (read `tc.RunResult().Stdout`, find the line, `strings.Contains` check); then run `get deployed` and assert the whoami-pinned row shows `latest@`+shortHex(newDigest) (the table follows the pin, not the container). (4) "status shows the running image in an IMAGE column" (FR-007, FR-008, SC-004): run `status shrine-deploy-test --state-dir <state>` and assert success, `assertColumnsInOrder(tc, "NAME", "KIND", "RUNNING", "STATUS", "IMAGE", "IMAGE ID")`, `AssertOutputLineContains("whoami-pinned", w.registry.Host+"/shrine/whoami@"+shortHex(w.oldDigest))`, `AssertOutputNotContains(w.oldDigest)`; run `status app whoami-pinned --team shrine-deploy-test` and assert the same cell. (5) "after teardown nothing shows the artifact or its pin" (FR-009, FR-010, SC-005): `before := readFileOrEmpty(tc, pinsPath(tc))`, `pinnedTeardown(tc)`, run `get deployed` and assert `AssertOutputNotContains("whoami-pinned")` and `AssertOutputNotContains("latest@")`; run `describe app whoami-pinned` and `AssertFailure().AssertStderrContains("not found").AssertOutputNotContains("Pinned:")`; run `status shrine-deploy-test` and `AssertSuccess().AssertOutputNotContains("whoami-pinned")`; assert `readFileOrEmpty(tc, pinsPath(tc)) == before` (no write); then `pinnedDeploy(tc, w.specsDir).AssertSuccess().AssertOutputContains("📌 Using pinned "+pinnedApp)` and `get deployed` shows `latest@`+shortHex(w.oldDigest) again with the same date on the `Pinned:` line of a fresh `describe`
- [ ] T003 [P] Append scenarios to the existing suites without editing any existing assertion: in `tests/integration/describe_test.go`, `TestDescribeDocker` gains "should show the image the running container was started from for a manifest-owned app" running `describe app whoami --team <testTeam> --state-dir <state>` and asserting `AssertOutputLineContains("Running image:", "traefik/whoami")` and `AssertOutputNotContains("Pinned:")`; `TestDescribeNoDocker` gains "should still describe a seeded record when the container cannot be inspected" calling `SeedLegacyDeploymentRecords(tc, testTeam)` then `describe app whoami --team <testTeam> --state-dir <state>` asserting `AssertSuccess()`, `AssertOutputLineContains("Image:", "-")`, `AssertOutputLineContains("Running image:", "unavailable")`, `AssertOutputNotContains("Pinned:")`. In `tests/integration/status_test.go`, `TestStatusDocker` gains "should show the IMAGE column beside the running state" running `status <testTeam> --state-dir <state>` and asserting `assertColumnsInOrder(tc, "NAME", "KIND", "RUNNING", "STATUS", "IMAGE", "IMAGE ID")` and `AssertOutputLineContains("test-cache", "traefik/whoami")`
- [ ] T004 Compile the suite: `go vet -tags integration ./tests/integration/...` is green (unused imports and helper signatures fixed here, never by running the suite). Confirm `git diff --stat main -- tests/integration/` shows the new file plus additions only in `describe_test.go` and `status_test.go`, and `git diff main -- tests/integration/describe_test.go tests/integration/status_test.go | grep '^-' | grep -v '^---'` prints nothing (no line removed)

### The shared readable form (test first, then code)

- [ ] T005 [P] Unit tests in `internal/manifest/types_test.go`: `TestReadableVersion` table per data-model section 1 (`127.0.0.1:5000/shrine/whoami:latest` + `sha256:3f2a9c1b4d7e` followed by 52 more hex → `latest@3f2a9c1b4d7e`; `postgres:17` → `17@…`; `postgres` → `latest@…`; `postgres@sha256:…` as requested → the twelve hex alone; a digest shorter than twelve hex → as is); `TestShortDigest` (`sha256:` prefix stripped, twelve kept, short input unchanged, empty → empty); `TestDigestOf` (`repo@sha256:abc` → `sha256:abc`, `repo:tag` → `""`, `""` → `""`). Move the two tables at `internal/ui/terminal_logger_test.go` lines ~248 and ~317 (`shortDigest`, `readableVersion`) into these tests and delete them from `ui`
- [ ] T006 In `internal/manifest/types.go` add, after `IsDigestReference`: `func ReadableVersion(requested, digest string) string`, `func ShortDigest(digest string) string`, `func DigestOf(ref string) string` (`_, digest, _ := strings.Cut(ref, "@"); return digest`), with the bodies of `readableVersion` and `shortDigest` from `internal/ui/terminal_logger.go` and their WHY comments (design section 3.5, TD-11). In `internal/ui/terminal_logger.go` delete `readableVersion` and `shortDigest`, call `manifest.ReadableVersion` in `renderImageResolved` and `manifest.ShortDigest` in `exactVersion`; `go build ./... && go test ./internal/manifest/... ./internal/ui/...` green, with the rendered `📌` lines in `terminal_logger_test.go` unchanged

### The running image from the backend

- [ ] T007 [P] In `internal/engine/backends.go` add `Image string` to `ContainerInfo` after `ImageID` with a one-line comment (the reference the container was created from); in `internal/engine/local/dockercontainer/docker_status.go` set `Image: image` where `image` is `resp.Config.Image` when `resp.Config != nil` and `""` otherwise. The dry-run backend and the handler stubs compile unchanged (struct literals with named fields). `go build ./... && go test ./internal/engine/...` green
- [ ] T008 Green checkpoint: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/... && gofmt -l .` all clean; commit

**Checkpoint**: one readable form in `manifest`; `ContainerInfo` carries the creation reference; nothing shows either yet

---

## Phase 3: User Story 1 - Read the pinned version in the deployment listing (Priority: P1) 🎯 MVP

**Goal**: the three listing commands load the pins once and print the readable form for `Pinned` records; every other row and the no-Docker behaviour are unchanged

**Independent Test**: scenario (1) of T002 (CI); locally, the renderer unit tests and quickstart step 2

### Tests for User Story 1 (write FIRST; must fail before T010)

- [ ] T009 [P] [US1] Unit tests in `internal/handler/deployments_output_test.go`: update the existing `formatDeploymentsTable(...)` calls to pass `nil` pins and keep their assertions; add `TestFormatDeploymentsTable_PinnedRowShowsTheReadableForm`: a `Pinned` record (`Team: "lab"`, `Name: "api"`, `Kind: "Application"`, `Image: "ghcr.io/me/api"`) with `pins = map[string]state.ImagePin{state.ImagePinKey("lab", "api"): {Kind: "Application", Name: "api", Requested: "ghcr.io/me/api:latest", Pinned: "ghcr.io/me/api@sha256:3f2a9c1b4d7e" + 52 more hex}}` renders `latest@3f2a9c1b4d7e` in the row, not `ghcr.io/me/api` and not the full digest; `…_PinnedRowWithoutAPinShowsTheRecordedReference` (same record, empty pins → `ghcr.io/me/api`); `…_ManifestOwnedRowIgnoresAPin` (`Policy: "Always"` with the same pin present → `ghcr.io/me/api`); `…_PinOfTheOtherKindIsIgnored` (`Pinned` Resource record, pin with `Kind: "Application"` → the recorded reference); `TestVersionCell` as a table over the same four cases plus a legacy record (`Image: ""`, `Policy: ""` → `-`)
- [ ] T010 [US1] Implement in `internal/handler/deployments.go` per data-model section 3: `loadImagePins(store *state.Store) (map[string]state.ImagePin, error)` (empty map when `store.ImagePins == nil`; else `ListAll()`, wrapping an error as `listing image pins: %w`); `pinFor(team string, d state.Deployment, pins map[string]state.ImagePin) (state.ImagePin, bool)` (key `state.ImagePinKey(team, d.Name)`, `Kind` must equal `d.Kind`); `versionCell(team string, d state.Deployment, pins map[string]state.ImagePin) string` (`d.Policy == manifest.ImagePullPolicyPinned` and a pin → `manifest.ReadableVersion(pin.Requested, manifest.DigestOf(pin.Pinned))`; else `valueOrUnknown(d.Image)`); `formatDeploymentsTable(deployments []teamedDeployment, pins map[string]state.ImagePin) string` uses `versionCell`; `printDeploymentsTable(deployments, pins)`; `ListApplications`, `ListResources`, `ListDeployed` call `loadImagePins` once after the empty check and pass the map (T009 passes). No Docker call is added; `cmd/get.go` is untouched
- [ ] T011 [US1] Build the binary into the scratchpad (`go build -o /tmp/claude-0/-root-projects-shrine/3f31e799-c78b-4ad6-8666-764ac630b7c2/scratchpad/shrine .`), run quickstart step 2 (hand-written `deployments.txt` and `pins.txt`) and confirm `get deployed` prints `latest@3f2a9c1b4d7e`, falls back to `ghcr.io/me/api` once `pins.txt` is removed, and quickstart step 1's legacy row still prints `-`
- [ ] T012 [US1] `go test ./...` green; `gofmt -l .` clean; commit

**Checkpoint**: one command answers "which version?" for every pinned artifact without Docker (PRD M3, journey J3 listing half)

---

## Phase 4: User Story 2 - See the pin, its date, and what is actually running for one artifact (Priority: P2)

**Goal**: `describe` gains the read-only backend, prints `Pinned:` for `Pinned` records and `Running image:` for every record, and never fails because Docker cannot be asked

**Independent Test**: scenarios (2) and (3) of T002 and the two appended `describe` scenarios of T003 (CI); locally, the renderer and flow unit tests and quickstart steps 1, 2, and 5

### Tests for User Story 2 (write FIRST; must fail before T015)

- [ ] T013 [P] [US2] Unit tests in `internal/handler/deployments_output_test.go`: update the existing `formatDeploymentDetail` test(s) to build a `deploymentDetail{Team, Deployment, RunningImage: "-"}` and keep their assertions; add `TestFormatDeploymentDetail_PinnedRecordShowsThePinAndTheRunningImage`: `Policy: "Pinned"`, `HasPin: true`, `Pin{Requested: "ghcr.io/me/api:latest", Pinned: "ghcr.io/me/api@sha256:3f2a9c1b4d7e"+52 hex, PinnedAt: time.Date(2026, 10, 7, 10, 42, 17, 0, time.UTC)}`, `RunningImage: "ghcr.io/me/api@sha256:3f2a9c1b4d7e"+52 hex` → output contains exactly `Pinned:       ghcr.io/me/api@sha256:3f2a9c1b4d7e…<full> (latest@3f2a9c1b4d7e, 2026-10-07)\n` and `Running image: ghcr.io/me/api@sha256:…\n`, and the lines appear in the order `Pull policy:`, `Pinned:`, `Running image:`, `Container ID:` (use `assertInOrder` on the whole output); `…_PinnedRecordWithoutAPinShowsADash` (`HasPin: false` → `Pinned:       -\n`); `…_ManifestOwnedRecordHasNoPinnedLine` (`Policy: "Always"` → no `Pinned:` substring, `Running image:` present); `…_LegacyRecordHasNoPinnedLine` (`Policy: ""` → `Image:        -`, `Pull policy:  -`, no `Pinned:`, `Running image:` present). Add `TestRunningImage` as a table: nil backend → `unavailable (no container runtime)`; a stub whose `InspectContainer` returns `errors.New("no such container")` → `unavailable (no such container)`; a stub returning `ContainerInfo{}` → `-`; a stub returning `ContainerInfo{Image: "docker.io/traefik/whoami:latest"}` → that string (extend `stubContainerBackend` in `deployments_test.go` with an `images map[string]string` consulted by `InspectContainer`, or add a tiny `imageBackend` stub in the output test file)
- [ ] T014 [P] [US2] Unit tests in `internal/handler/deployments_test.go` on the in-memory stores (`deleteTestStore`, `memImagePinStore`): `TestDescribeApplication_SucceedsWhenTheBackendCannotInspect` (one `Pinned` record, a pin, `stubContainerBackend{}` whose inspect errors → `DescribeApplication("demo", "api", store, backend)` returns nil); `TestDescribeApplication_ToleratesAStoreWithoutPins` (`store.ImagePins = nil`, `Pinned` record → nil error); `TestDescribeApplication_ToleratesANilBackend` (nil backend → nil error); `TestDescribeApplication_NotFoundAndAmbiguousAreUnchanged` (no record → error contains `not found`; the same name in two teams → error contains `ambiguous`); `TestDescribeApplication_APinWithoutARecordIsNotFound` (pin present, no deployment record → error contains `not found`, FR-009). Capture stdout only if an existing helper does; otherwise assert on the returned error alone
- [ ] T015 [US2] Implement in `internal/handler/deployments.go` per data-model section 4: `type deploymentDetail struct{ Team string; Deployment state.Deployment; Pin state.ImagePin; HasPin bool; RunningImage string }`; `runningImage(backend engine.ContainerBackend, containerID string) string` per the rule table; `pinLine(d deploymentDetail) string` (`""` unless `Policy == manifest.ImagePullPolicyPinned`; `fmt.Sprintf("Pinned:       %s (%s, %s)\n", d.Pin.Pinned, manifest.ReadableVersion(d.Pin.Requested, manifest.DigestOf(d.Pin.Pinned)), d.Pin.PinnedAt.UTC().Format(time.DateOnly))` or `Pinned:       -\n`); `formatDeploymentDetail(d deploymentDetail) string` writing `Name:`, `Team:`, `Kind:`, `Image:`, `Pull policy:`, `pinLine`, `Running image: %s\n`, `Container ID:`, `Config Hash:`; `lookupPin(store, team string, d state.Deployment) (state.ImagePin, bool, error)` (only when `Pinned`; nil `ImagePins` or `ErrImagePinNotFound` → false; kind mismatch → false; other error wrapped `reading image pin for %s/%s: %w`); `describeDeployment(team, name, kind string, store *state.Store, backend engine.ContainerBackend) error` builds the detail after the existing record lookup in both branches and prints it; `DescribeApplication` and `DescribeResource` gain the `backend engine.ContainerBackend` parameter (T013, T014 pass)
- [ ] T016 [US2] In `cmd/describe.go`: import `github.com/CarlosHPlata/shrine/internal/app`; in `describeAppCmd` and `describeResourceCmd` build `backend, err := app.NewQueryContainerBackend(cfg, store)`, return `err` if non-nil, pass `backend` to the handler; append to both `Long` texts the paragraph of contracts/query-output.md ("The record shows the image the manifest named and the effective pull policy. Under the Pinned policy it also shows the pin …"). `describe team` untouched. `go build ./...`; fix the `cmd` unit tests if any call the two handlers directly (`grep -rn 'DescribeApplication\|DescribeResource' cmd internal`)
- [ ] T017 [US2] Run quickstart steps 1 and 2 with the scratchpad binary: the legacy record describes with exit zero, `Running image: unavailable (…)`, no `Pinned:`; the hand-written `Pinned` record prints the `Pinned:` line with the full reference, `latest@3f2a9c1b4d7e`, and `2026-10-07`; removing `pins.txt` gives `Pinned:       -`. If a Docker daemon is reachable on this host, do not run steps 3 to 7 (another agent shares it); CI runs them
- [ ] T018 [US2] `go test ./...` green; `gofmt -l .` clean; commit

**Checkpoint**: an operator can read what is pinned, when, and whether what runs differs from it (journey J3 complete)

---

## Phase 5: User Story 3 - See the running image in the live status view (Priority: P3)

**Goal**: every status table prints an IMAGE column between STATUS and IMAGE ID, with a digest reference shortened to twelve hex characters

**Independent Test**: scenario (4) of T002 and the appended `status` scenario of T003 (CI); locally, the renderer unit tests

### Tests for User Story 3 (write FIRST; must fail before T020)

- [ ] T019 [P] [US3] Unit tests in `internal/handler/status_test.go`: give `mockBackend.InspectContainer` an `Image: "docker.io/traefik/whoami:latest"`; add `TestFormatStatusTable_AddsImageBetweenStatusAndImageID` (one row with `Image: "127.0.0.1:5000/shrine/whoami@3f2a9c1b4d7e"`, `ImageID: "sha256:3f2a9c1b4d7e"`; header in order `NAME`, `KIND`, `RUNNING`, `STATUS`, `IMAGE`, `IMAGE ID`; separator dashes only and as long as the header; row contains the image cell before the id cell; reuse `assertInOrder` and `tableLines` from `deployments_output_test.go`); `TestShortImageReference` table: `""` → `-`; `repo@sha256:` + 64 hex → `repo@` + first twelve hex; `127.0.0.1:5000/shrine/whoami@sha256:…` → `127.0.0.1:5000/shrine/whoami@<12 hex>`; `docker.io/traefik/whoami:latest` → unchanged; `postgres` → unchanged; `TestInspectDeployments_FillsTheImageCell` using `mockBackend` → `rows[0].Image == "docker.io/traefik/whoami:latest"`
- [ ] T020 [US3] Implement in `internal/handler/status.go` per data-model section 5: `Image string` on `containerStatusRow` before `ImageID`; `shortImageReference(ref string) string` (`""` → `-`; `manifest.IsDigestReference(ref)` → `repo + "@" + manifest.ShortDigest(digest)` using `strings.Cut(ref, "@")`; else `ref`); `inspectDeployments` sets `Image: shortImageReference(info.Image)`; `const statusRowFormat = "%-25s %-15s %-10v %-12s %-40s %-19s\n"` and a header variant with `%-10s`; `formatStatusTable(rows []containerStatusRow) string` writing the header, a dash line as long as the header (as `formatDeploymentsTable` does, replacing the hardcoded 84), and the rows; `printStatusTable` prints `formatStatusTable(rows)` (T019 passes)
- [ ] T021 [US3] In `cmd/status.go` append to the `Long` of `statusCmd`, `statusAppCmd`, and `statusResourceCmd` the sentence of contracts/query-output.md ("The IMAGE column shows the image each container was started from: the exact version for a Pinned artifact, the tag reference otherwise."); no flag changes; `go build ./...`
- [ ] T022 [US3] `go test ./...` green; `gofmt -l .` clean; commit

**Checkpoint**: drift after a bump is visible in the only live view without opening `describe` (PRD OD-3)

---

## Phase 6: User Story 4 - Pins of artifacts that are not deployed stay invisible (Priority: P4)

**Goal**: prove, rather than build, that no command shows a pin without a deployment record and that nothing in the diff writes state

**Independent Test**: scenario (5) of T002 (CI); locally, the flow unit test of T014 and a review of the diff

- [ ] T023 [US4] Review the diff for the guarantee: `git diff main -- internal/handler cmd | grep -E 'ImagePins\.(Put|Release|ReleaseTeam)|Deployments\.(Record|Remove)|HostPorts\.'` prints nothing (FR-010); `git diff main -- internal/handler | grep -n 'ImagePins'` shows only `Get` and `ListAll`; `describeDeployment` reads `Deployments.List` before any pin lookup in both branches and never widens the team search to pins (FR-009; the pin-aware team search in `findApplicationTeam` belongs to `delete` and is untouched). Record the result as a one-line note in the commit message of T024
- [ ] T024 [US4] Confirm `TestDescribeApplication_APinWithoutARecordIsNotFound` (T014) and quickstart step 7's expectations are consistent with scenario (5) of T002 (`not found` on stderr, no `Pinned:` on stdout, `pins.txt` byte-identical); `go test ./...` green; commit

**Checkpoint**: torn-down pins exist for Shrine only (PRD R-19, non-goal "showing versions of artifacts that are torn down")

---

## Phase 7: User Story 5 - The documentation shows how to read a pinned version (Priority: P5)

**Goal**: the manifest reference, the five command pages, and the contributor reference describe the readable form, the `Pinned:` and `Running image:` lines, the IMAGE column, and what a difference means

**Independent Test**: read the three documents against spec SC-007

- [ ] T025 [P] [US5] Update `docs/content/reference/manifest-schema.md` per contracts/docs-and-deviations.md: after the **The pin lifecycle.** paragraph add a bold **Reading what is pinned.** paragraph (the `get` readable form with `latest@3f2a9c1b4d7e` as example and the manifest reference for manifest-owned rows; `describe`'s `Pinned:` with full exact version, readable form, and date, and `Running image:`; a `Pinned:` that differs from `Running image:` is recorded but not yet deployed; `status`'s IMAGE column; `get` needs no runtime, `describe` prints `Running image: unavailable` without one; pins of undeployed artifacts are never shown) followed by a short ```text``` block with one `get` row and the two `describe` lines in the shapes of contracts/query-output.md
- [ ] T026 [US5] Regenerate the CLI pages: `make docs-gen-cli`; confirm `git status` shows exactly `docs/content/cli/describe_app.md`, `describe_resource.md`, `status.md`, `status_application.md`, `status_resource.md` changed under `docs/content/cli/`, each carrying the new `Long` text; run `make docs-check` if Hugo is installed, otherwise `cd docs/tools/docsgen && go test ./...` (depends on T016 and T021)
- [ ] T027 [P] [US5] Update `AGENTS.md`: under `### shrine status app/resource <name>` add the sentence on the IMAGE column (the image each container was started from: the exact version for a `Pinned` artifact, the tag reference otherwise, beside IMAGE ID); under `### shrine describe app/resource <name>` add the sentence on `Image:`, `Pull policy:`, the `Pinned:` line (full exact version, readable form, date) and `Running image:` read from Docker (`unavailable` when the daemon cannot be reached, the command still succeeds), and that a `Pinned:` differing from `Running image:` is a recorded, not yet deployed, pin
- [ ] T028 [US5] Run `bash scripts/lint-docs-frontmatter.sh docs/content` and, if `docs/public` exists after `make docs-build`, `bash scripts/check-md-shape.sh docs/public`; read the manifest reference and the five pages and confirm a reader can explain `latest@3f2a9c1b4d7e`, say where the full exact version and date are shown, and tell a waiting pin from a deployed one (spec SC-007); commit

**Checkpoint**: the documentation this ticket owns is updated in the same pull request (definition of done)

---

## Phase 8: Polish & Cross-Cutting Concerns

- [ ] T029 Record the six refinements of contracts/docs-and-deviations.md in `specs/epics/pinned-image-versions/design.md` as "*Amended by T5 (spec 035)*" notes: section 3.5 (helpers exported from `internal/manifest`, `DigestOf` added), section 4.7 (describe handlers take the backend and tolerate nil; constructor failure fails the command, inspect failure degrades the line; `ListAll()` once and the pinless `Pinned` record shows the recorded reference; the one-line `Pinned:` layout; the status IMAGE shortening), section 5 (the pin-differs scenario edits `pins.txt` until T6)
- [ ] T030 Add the entry for 035 to `specs/progress.md` in the form of the 031 to 034 entries: title, spec link, issue #56, epic and ticket T5, what changed (readable form in VERSION for `Pinned` records read once from `pins.txt`; `Pinned:` and `Running image:` in `describe` with the unavailable rule and the new `ContainerInfo.Image`; IMAGE in `status` with twelve-hex digests; nothing shown without a record; helpers moved into `manifest`), acceptance SC-001 to SC-007 mapping, gates `TestPinnedVersionQueries` plus the appended `TestDescribeDocker`, `TestDescribeNoDocker`, and `TestStatusDocker` scenarios (CI executes)
- [ ] T031 Run `graphify update .` and commit the refreshed `graphify-out/`
- [ ] T032 Final verification: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/... && gofmt -l .` clean; `make docs-gen-cli` produces no further diff; `git diff main -- tests/integration/ | grep '^-' | grep -v '^---'` prints nothing (no existing assertion removed); `grep -rn '"github.com/CarlosHPlata/shrine/internal' tests/integration/` still finds nothing (integration isolation); `grep -rn 'TempDir\|MkdirAll\|WriteFile' internal/handler/*_test.go internal/manifest/*_test.go` shows no new filesystem use beyond what `main` already had
- [ ] T033 Rebase onto `origin/main` if it moved (expected conflicts only in `specs/progress.md`, `.specify/feature.json`, `CLAUDE.md`, and `graphify-out/`; keep both entries), push `035-pinned-version-queries`, open the pull request with `/speckit-git-pr` following `.github/pull_request_template.md`: `Closes #56` in Why, the definition-of-done list of the issue with each item ticked, the six design refinements named, and the note that the pin-differs scenario edits `pins.txt` until T6 lands
- [ ] T034 Run `/shrine-pr-review` on the pull request, fix every real finding, push again, confirm CI is green (the integration job runs `TestPinnedVersionQueries` and the appended scenarios; the docs job runs the CLI drift check)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: depends on Phase 1; T002 and T003 in parallel (different files); T004 after both; T005 before T006; T007 in parallel with T005/T006; T004, T006, T007 before T008
- **US1 (Phase 3)**: depends on Phase 2 (uses `manifest.ReadableVersion`); T009 before T010; T011 after T010
- **US2 (Phase 4)**: depends on Phase 2 (`ContainerInfo.Image`, helpers) and shares `deployments.go` with US1, so follows Phase 3; T013 and T014 in parallel, both before T015; T015 before T016; T017 after T016
- **US3 (Phase 5)**: depends on Phase 2 only; can run in parallel with Phases 3 and 4 (different files: `status.go`, `status_test.go`, `cmd/status.go`); T019 before T020; T020 before T021
- **US4 (Phase 6)**: depends on Phases 3 to 5 being on the branch (it reviews the diff); T023 before T024
- **US5 (Phase 7)**: T025 and T027 depend on nothing in code and can start after Phase 2; T026 depends on T016 and T021; T028 last
- **Polish (Phase 8)**: depends on everything above; T029 to T031 before T032; T032 before T033; T033 before T034

### Parallel Opportunities

- T002 and T003 (two scenario files); T005 and T007 (manifest versus engine)
- T009, T013, T014 are all handler test files and can be written together once Phase 2 is in; T019 beside them
- US3 (status) can proceed in parallel with US1 and US2 (listing and describe) because they touch different files
- T025 and T027 (two documents) in parallel with any code phase after Phase 2

---

## Implementation Strategy

### MVP First (User Story 1)

1. Phase 1, then Phase 2: the integration scenarios first, then the shared readable form and the backend field.
2. Phase 3: the listing shows the readable form. At this point "which version is deployed?" is answered for every pinned artifact without Docker (PRD M3); scenario (1) of the suite is satisfiable.

### Incremental Delivery

- US2 completes journey J3 with the pin, its date, and the running image; US3 adds the live view; US4 proves the invisibility guarantee; US5 documents. Each lands as its own commit on the same branch; the pull request carries all of them, per the ticket.

## Notes

- Integration scenarios are compiled (`go vet -tags integration`) and never run locally; another agent shares the daemon. Quickstart steps 1 and 2 need no daemon and are the local smoke test.
- Unit tests touch no filesystem: renderers over structs, the describe flow over the in-memory stores of `deployments_test.go`, the helpers over strings.
- No task writes state: the only store calls added are `ImagePins.Get`, `ImagePins.ListAll`, and the existing `Deployments.List` and `Teams.ListTeams`.
- Decisions TD-1 to TD-13 are settled; a disagreement goes to issue #56, not into the code.
- Every commit message ends with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.
- Never commit to or push `main`.
