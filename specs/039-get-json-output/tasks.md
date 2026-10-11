# Tasks: Machine-Readable Output for `shrine get`

**Input**: Design documents from `/specs/039-get-json-output/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/cli-json-output.md, quickstart.md

**Tests**: INCLUDED. The constitution mandates TDD (Principle V: integration test files are created before the implementation code). Integration scenarios are written first, compiled under the integration tag, and never run locally (CI is the gate). Unit tests touch no filesystem and capture no stdout: the formatters return bytes and the tests decode them.

**Organization**: Phase 1 confirms the baseline. Phase 2 holds the integration suite (written first) and the seams every story needs: the helpers extracted from the table and `describe` paths (behaviour-preserving) and the JSON envelope. Phases 3 to 5 follow the spec's three user stories in priority order; each adds its unit tests first, then the handler, then the command wiring and help text. Phase 6 is docs, progress, the graph, the gates, and the pull request.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label (US1 to US3); Setup, Foundational, and Polish tasks carry none

## Phase 1: Setup

**Purpose**: Confirm a green baseline and the seams the plan binds to

- [X] T001 Verify green baseline on branch `039-get-json-output`: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/...` pass with no file changes. Confirm the seams exist: in `internal/handler/deployments.go` `teamedDeployment`, `deploymentsForTeamOrAll`, `filterByKind`, `loadImagePins`, `pinFor`, `readablePin`, `versionCell`, `valueOrUnknown`, `runningImage`, `ListApplications`, `ListResources`, `ListDeployed`; in `internal/handler/teams.go` `ListTeams`; in `internal/handler/deployments_output_test.go` the `imageBackend` fake; in `internal/app/components.go` `NewQueryContainerBackend`; in `tests/integration/` `SeedDeploymentRecord` (`testutils/seed_state.go`), `newPinnedSuite`, `pinnedDeploy`, `writeManifestOwnedSibling`, `shortHex`, `pinnedDateOf`, `pinnedOwnedApp`, `pinnedSourceOld`, `testTeam`, `fixturesPath`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The integration suite, written first; the shared helpers; the JSON envelope

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Integration scenarios (write FIRST; compile only, never run locally)

- [X] T002 In `tests/integration/get_test.go` add local helpers (same package, `encoding/json` import): `decodeListing(tc *TestCase) []map[string]any` (unmarshals `tc.RunResult().Stdout` into `struct{ Data []map[string]any \`json:"data"\` }`, `tc.Fatalf` on a parse error or when `Data` is nil, so `"data": null` fails); `entryNamed(tc, entries, name string) map[string]any` (fatal when absent); `namesOf(entries) []string`; `tableNames(tc) []string` (the NAME field, second whitespace column, of every stdout line after the `---` separator line). Then extend `TestGetNoDocker` with these scenarios, per [contracts/cli-json-output.md](contracts/cli-json-output.md):
  (1) "get teams --json lists each team with numeric quotas" (US3): `tc.Run("get", "teams", "--json", "--state-dir", tc.StateDir).AssertSuccess()`; `entryNamed(tc, decodeListing(tc), testTeam)` has `displayName == "Shrine Test"`, `maxApps == float64(10)`, `maxResources == float64(10)` (asserting `float64` proves they are JSON numbers), and exactly the four keys `name`, `displayName`, `maxApps`, `maxResources`
  (2) "get deployed --json with nothing deployed prints an empty data array" (US1, FR-008): success; `decodeListing` returns zero entries; `AssertOutputNotContains("No deployments found")`; same for `get apps --json` (`No applications deployed`) and `get resources --json` (`No resources deployed`)
  (3) "get deployed --json without a container runtime still succeeds with a null running image" (US1, FR-003a, SC-002a): `SeedDeploymentRecord(tc, testTeam, "Application whoami 0123456789abcdef 3a7b - -")`; `tc.Setenv("DOCKER_HOST", "tcp://127.0.0.1:1")`; `get deployed --json` succeeds, stderr empty; the `whoami` entry has `team == testTeam`, `kind == "Application"`, `image == ""`, `pullPolicy == ""`, `version == ""` (the table's `-`), `pin == nil`, `runningImage == nil`, `containerId == "0123456789abcdef"`, `configHash == "3a7b"`, and has all ten keys of data-model "Workload entry"
  (4) "get config-dir and get state-dir reject --json" (FR-010): each `.AssertFailure().AssertStderrContains("unknown flag: --json")`
  (5) "get deployed without --json is still the table" (FR-011): after the seed of (3), `get deployed` succeeds and `assertColumnsInOrder(tc, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")`, and stdout does not contain `"data"`
- [X] T003 In `tests/integration/get_test.go` extend `TestGetDocker` (fixtures already deploy `whoami-res` Application and `test-cache` Resource):
  (6) "get deployed --json matches the table row for row" (US1, FR-005, SC-002): run `get deployed`, keep `tableNames(tc)`; run `get deployed --json`; `namesOf(decodeListing(tc))` equals the table names in order; for each entry: `runningImage` is a non-nil string containing `traefik/whoami`, `containerId` has length 64, `configHash` is non-empty and does not end with `...`, `pin == nil`, `image` contains `traefik/whoami`, `version == image`
  (7) "get deployed --json honours --team" (US1, FR-006): with `--team testTeam` both entries are present; with `--team nobody` the `data` array is empty and the command succeeds
  (8) "get apps and get resources --json list only their kind" (US2): `get apps --json` → names `[whoami-res]`, every `kind == "Application"`; `get applications --json --team testTeam` → same; `get resources --json` → names `[test-cache]`, every `kind == "Resource"`; `get res --json` → same; each list's names equal the corresponding table's `tableNames`
- [X] T004 In `tests/integration/pinned_version_queries_test.go` add to `TestPinnedVersionQueries` the scenario "get --json carries the pin describe shows" (US1, FR-003, FR-003b): `w := worlds[tc]`; `writeManifestOwnedSibling(tc, w.specsDir)`; `pinnedDeploy(tc, w.specsDir).AssertSuccess()`; `readable := "latest@" + shortHex(w.oldDigest)`; `tc.Run("get", "deployed", "--json", "--state-dir", tc.StateDir).AssertSuccess()`; decode with `decodeListing` (same package); for `whoami-pinned` and `cache-pinned`: `pullPolicy == "Pinned"`, `version == readable`, `pin.reference == w.pinnedRef()`, `pin.version == readable`, `pin.pinnedAt` has the prefix `pinnedDateOf(tc, name)` and the suffix `Z`, `runningImage == w.pinnedRef()`; for `pinnedOwnedApp`: `pin == nil`, `version == pinnedSourceOld`, `runningImage` contains `pinnedSourceOld`
- [X] T005 Compile the suite: `go vet -tags integration ./tests/integration/...` is green (fix unused imports and helper signatures here, never by running the suite). `git diff --stat main -- tests/integration/` shows only `get_test.go` and `pinned_version_queries_test.go`; no existing assertion changed

### Shared helpers (behaviour-preserving)

- [X] T006 In `internal/handler/deployments.go` add `func listDeployments(team, kind string, store *state.Store) ([]teamedDeployment, error)` (calls `deploymentsForTeamOrAll`; filters with `filterByKind` only when `kind != ""`) and rewrite `ListApplications`, `ListResources` and `ListDeployed` to call it. Their empty-listing sentences and table output stay byte for byte. `go test ./internal/handler/` green with no assertion changed
- [X] T007 In `internal/handler/deployments.go` add `func pinOf(team string, d state.Deployment, pins map[string]state.ImagePin) (state.ImagePin, bool)` (returns `pinFor` only when `d.Policy == manifest.ImagePullPolicyPinned`) and `func versionOf(team string, d state.Deployment, pins map[string]state.ImagePin) string` (`readablePin(pin)` when `pinOf` finds one, else `d.Image`, raw, possibly empty); `versionCell` becomes `return valueOrUnknown(versionOf(team, d, pins))` and keeps its WHY comment. `TestVersionCell` and every `TestFormatDeploymentsTable_*` pass unchanged
- [X] T008 In `internal/handler/deployments.go` add `var errNoContainerRuntime = errors.New("no container runtime")` and `func inspectRunningImage(backend engine.ContainerBackend, containerID string) (string, error)`: returns `errNoContainerRuntime` for a nil backend, the `InspectContainer` error when it fails, else `info.Image`. Rewrite `runningImage` as: `image, err := inspectRunningImage(backend, containerID); if err != nil { return fmt.Sprintf("unavailable (%v)", err) }; return valueOrUnknown(image)` — byte-identical output for all four existing cases. `TestRunningImage` passes unchanged
- [X] T009 [P] Create `internal/handler/json_listing.go`: `type jsonListing[T any] struct { Data []T \`json:"data"\` }`; `func encodeListing[T any](entries []T) ([]byte, error)` that replaces a nil slice with `[]T{}`, encodes `jsonListing[T]{Data: entries}` with `json.NewEncoder` into a `bytes.Buffer`, `SetIndent("", "  ")`, `SetEscapeHTML(false)`, and returns the bytes (the encoder adds the trailing newline); `func printListing(doc []byte)` writing to stdout with `os.Stdout.Write`. One WHY comment: encoding completes before anything is written so a failure never leaves half a document on stdout (FR-009). Unit tests in `internal/handler/json_listing_test.go`: nil and empty slices both encode to `{\n  "data": []\n}\n`; a string with `&<>"\` and `é` round-trips through `json.Unmarshal` unchanged and the raw bytes contain `&<>` literally

**Checkpoint**: Foundation ready; the suite compiles against a binary that has no `--json` flag yet; table and `describe` output unchanged

---

## Phase 3: User Story 1 - Script against deployed workloads (Priority: P1) 🎯 MVP

**Goal**: `shrine get deployed --json` prints one `{"data": [...]}` document of workload entries with the `describe` fields, the table's version, a nested pin, and a live running image that is `null` when unreadable; `--team` filters; empty is `{"data": []}`

**Independent Test**: scenarios (2), (3), (5), (6), (7) and the pinned scenario of T004; quickstart steps 3, 5 and 6

### Handler (test first)

- [X] T010 [US1] Create `internal/handler/deployments_json_test.go` (fails to compile until T011). Over in-memory `[]teamedDeployment`, a `map[string]state.ImagePin` keyed by `state.ImagePinKey`, and the existing `imageBackend` fake, decode the bytes of `formatDeploymentsJSON(deployments, pins, backend)` into `struct{ Data []map[string]any }` and assert:
  `TestFormatDeploymentsJSON_KeysInOrder` (the raw bytes list `team`, `name`, `kind`, `image`, `pullPolicy`, `version`, `pin`, `runningImage`, `containerId`, `configHash` in that order, checked with `bytes.Index` increasing);
  `TestFormatDeploymentsJSON_PinnedRecordCarriesThePin` (Pinned record + matching pin with `PinnedAt` `2026-10-11T14:03:22.123Z` → `pin` = `{reference: pin.Pinned, version: readablePin(pin), pinnedAt: "2026-10-11T14:03:22Z"}` and `version == readablePin(pin)`);
  `TestFormatDeploymentsJSON_PinIsNullWhenNotPinned` (table-driven: manifest-owned record with a pin on record; Pinned record with no pin; Pinned record whose pin is of the other kind → `pin == nil`; `version` equals `d.Image`);
  `TestFormatDeploymentsJSON_UnknownValuesAreEmptyStrings` (legacy record, empty `Image`, `Policy`, `ConfigHash`, `ContainerID` → each `""`, never `"-"`);
  `TestFormatDeploymentsJSON_FullContainerIDAndConfigHash` (64-char ID and hash come back whole);
  `TestFormatDeploymentsJSON_RunningImage` (table-driven: nil backend → `nil`; `imageBackend{err: …}` → `nil`; empty `ContainerID` with a backend whose `InspectContainer` would succeed → `nil`; `imageBackend{info: {Image: "docker.io/traefik/whoami:latest"}}` → that string; `imageBackend{}` → `""`);
  `TestFormatDeploymentsJSON_EmptyListing` (nil slice → bytes equal `{\n  "data": []\n}\n`);
  `TestFormatDeploymentsJSON_KeepsListingOrder` (three records across two teams come back in input order)
- [X] T011 [US1] Create `internal/handler/deployments_json.go`: `type pinEntry struct { Reference string \`json:"reference"\`; Version string \`json:"version"\`; PinnedAt string \`json:"pinnedAt"\` }`; `type deploymentEntry struct` with the ten fields of data-model "Workload entry" (`Pin *pinEntry`, `RunningImage *string`, no `omitempty`, JSON tags as named there, field order as listed); `func toDeploymentEntry(td teamedDeployment, pins map[string]state.ImagePin, backend engine.ContainerBackend) deploymentEntry` using `versionOf`, `pinOf` (pin → `&pinEntry{pin.Pinned, readablePin(pin), pin.PinnedAt.UTC().Format(time.RFC3339)}`), `runningImageOrNull(backend, containerID) *string` (a private helper in this file: `nil` when `containerID == ""` without calling the backend, `nil` when `inspectRunningImage` errors, else `&image`); `func formatDeploymentsJSON(deployments []teamedDeployment, pins map[string]state.ImagePin, backend engine.ContainerBackend) ([]byte, error)` mapping in order and calling `encodeListing`; `func printDeploymentsJSON(deployments []teamedDeployment, store *state.Store, backend engine.ContainerBackend) error` (`loadImagePins` once, format, `printListing`); `func ListDeployedJSON(team string, store *state.Store, backend engine.ContainerBackend) error` (`listDeployments(team, "", store)` then `printDeploymentsJSON`; no empty-listing sentence). `go test ./internal/handler/` green

### Command

- [X] T012 [US1] In `cmd/get.go` add `var getJSONFlag bool`; a helper `func jsonQueryBackend() engine.ContainerBackend` that returns `app.NewQueryContainerBackend(cfg, store)` or `nil` when it errors (one WHY comment: runtime trouble never fails a listing, FR-003a); `getDeployedCmd.RunE` calls `handler.ListDeployedJSON(getTeamFlag, store, jsonQueryBackend())` when `getJSONFlag`, else `handler.ListDeployed` as today; in `init` register `getDeployedCmd.Flags().BoolVar(&getJSONFlag, "json", false, "Print the listing as JSON instead of a table")`. Append to `getDeployedCmd.Long` the paragraph: "With --json, the listing is printed as one JSON document, {\"data\": [...]}, with one entry per row carrying what describe shows: team, name, kind, image, pull policy, version, the pin (or null), the running image read from the container runtime (or null when it cannot be read), the full container ID and the full config hash. Without --json the table needs no container runtime." `go build ./... && go test ./cmd/` green; `go run . get deployed --help` shows `--json`; `go run . get config-dir --json` fails with `unknown flag: --json`

**Checkpoint**: `shrine get deployed --json` works end to end; quickstart steps 3, 5 and 6 pass for `deployed`

---

## Phase 4: User Story 2 - JSON for applications and resources separately (Priority: P2)

**Goal**: `get applications --json` (`apps`) and `get resources --json` (`res`) print the same document shape filtered to their kind, `--team` included

**Independent Test**: scenario (8) of T003 and the `get apps`/`get resources` halves of scenario (2); quickstart step 4

- [X] T013 [US2] In `internal/handler/deployments_json_test.go` add `TestListByKindJSON_FiltersKind` over `deleteTestStore([]string{"demo"}, state.HostPortMap{}, map[string][]state.Deployment{"demo": {appRecord, resourceRecord}})` calling `listDeployments("", manifest.ApplicationKind, store)` and `listDeployments("demo", manifest.ResourceKind, store)` and asserting each returns only its kind (this pins the filter both JSON variants share; the print path is covered by T010). Passes against T006; it pins the filter before the JSON wrappers rely on it
- [X] T014 [US2] In `internal/handler/deployments_json.go` add `ListApplicationsJSON(team string, store *state.Store, backend engine.ContainerBackend) error` and `ListResourcesJSON(…)`, each `listDeployments(team, manifest.ApplicationKind|ResourceKind, store)` then `printDeploymentsJSON`. `go test ./internal/handler/` green
- [X] T015 [US2] In `cmd/get.go` branch `getApplicationsCmd.RunE` and `getResourcesCmd.RunE` on `getJSONFlag` to `handler.ListApplicationsJSON`/`ListResourcesJSON(getTeamFlag, store, jsonQueryBackend())`; register `--json` on both with the same usage string as T012; append the T012 `Long` paragraph to both commands' `Long`, extracting it into one `const getJSONHelp` used by all three workload commands (Principle VII). `go build ./... && go test ./cmd/` green; `go run . get apps --help` and `go run . get res --help` show `--json`

**Checkpoint**: all three workload listings support `--json`

---

## Phase 5: User Story 3 - JSON for teams (Priority: P3)

**Goal**: `get teams --json` (`team`) prints `{"data": [...]}` with `name`, `displayName`, `maxApps`, `maxResources`, the quotas as numbers

**Independent Test**: scenario (1) of T002; quickstart step 2

- [X] T016 [US3] Create `internal/handler/teams_json_test.go` (fails to compile until T017): `TestFormatTeamsJSON_FieldsAndNumbers` builds two `*manifest.TeamManifest` in memory (one with `DisplayName: "Media & Co"`, `MaxApps: 5`, `MaxResources: 3`; one with no display name and no quotas), decodes `formatTeamsJSON(teams)` and asserts names in input order, `displayName` `"Media & Co"` and `""`, `maxApps` `float64(5)` and `float64(0)`, exactly four keys per entry, and that the raw bytes contain `Media & Co` literally; `TestFormatTeamsJSON_EmptyListing` → bytes equal `{\n  "data": []\n}\n`
- [X] T017 [US3] In `internal/handler/teams.go` add `type teamEntry struct { Name string \`json:"name"\`; DisplayName string \`json:"displayName"\`; MaxApps int \`json:"maxApps"\`; MaxResources int \`json:"maxResources"\` }`, `func formatTeamsJSON(teams []*manifest.TeamManifest) ([]byte, error)` (map in order, `encodeListing`), and `func ListTeamsJSON(store state.TeamStore) error` (`store.ListTeams()` wrapped as `ListTeams` wraps its error, format, `printListing`; no "Run 'shrine apply teams'" hint). `go test ./internal/handler/` green
- [X] T018 [US3] In `cmd/get.go` branch `getTeamsCmd.RunE` on `getJSONFlag` to `handler.ListTeamsJSON(store.Teams)`; register `--json` on `getTeamsCmd` with the T012 usage string; append to its `Long`: "With --json, the teams are printed as one JSON document, {\"data\": [...]}, with name, displayName, maxApps and maxResources for each team." `go build ./... && go test ./cmd/` green; `go run . get teams --help` shows `--json`

**Checkpoint**: all four listings support `--json`; every integration scenario of T002 to T004 would pass in CI

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: docs, contributor reference, progress entry, the graph, the final gates, and the pull request

- [X] T019 Regenerate the CLI reference: `make docs-gen-cli`; confirm `git diff --stat docs/content/cli/` shows exactly `get_deployed.md`, `get_applications.md`, `get_resources.md`, `get_teams.md` (FR-012)
- [X] T020 [P] In `docs/content/guides/image-versions.md`, section "See which version runs", after the `--team` example add one short paragraph: scripts can pass `--json` to any of the three commands to get the same rows as JSON, with the full pin (`pin.reference`, `pin.version`, `pin.pinnedAt`) and the running image, e.g. `shrine get deployed --json | jq -r '.data[] | [.name, .version] | @tsv'`; link [`shrine get deployed`](/cli/get_deployed/)
- [X] T021 [P] In `AGENTS.md` under `## CLI Reference`, after `### shrine status app/resource <name>`, add `### shrine get deployed/applications/resources/teams` stating: tables are read from state and need no container runtime; `--json` prints `{"data": [...]}`; workload entries carry `team`, `name`, `kind`, `image`, `pullPolicy`, `version`, `pin` (`reference`, `version`, `pinnedAt`, or `null`), `runningImage` (live; `null` when unreadable, command still succeeds), full `containerId`, full `configHash`; team entries carry `name`, `displayName`, `maxApps`, `maxResources`; empty listing is `{"data": []}`; `get config-dir`/`state-dir` reject `--json`
- [X] T022 [P] In `specs/progress.md` insert one `- [x]` entry above the 038 entry, in the form of the 031 to 038 entries: `specs/039-get-json-output/`; the behaviour (flag on four commands, document shape, the describe-level fields and the clarifications: nullable `runningImage`, nested `pin` plus top-level `version`, teams unchanged); the helpers extracted (`listDeployments`, `pinOf`, `versionOf`, `inspectRunningImage`, `encodeListing`); acceptance SC-001 to SC-005 including SC-002a restated; gates `TestGetNoDocker`, `TestGetDocker`, `TestPinnedVersionQueries` — CI executes
- [X] T023 Run `graphify update .` and include the regenerated `graphify-out/` in the change
- [X] T024 Final gates: `gofmt -l . | grep -v '^docs/tools'` empty; `go vet ./...`; `go build ./... && go test ./... && go vet -tags integration ./tests/integration/...` green; `make docs-check` green; `git diff --stat main -- internal/ cmd/ tests/` shows only the files the plan's Source Code tree names; walk quickstart steps 2, 5 and 6 locally (no daemon needed for 5 and 6) and confirm the output; mark every task in this file `[X]`
- [ ] T025 Pull request through `/speckit-git-pr`: Why section (scripts need machine-readable listings), the one-line Constitution Check (touches `cmd/`), the three clarification answers; then a `/shrine-pr-review` pass with no open finding and green CI (which runs the three extended suites)

---

## Dependencies & Execution Order

- **Phase 1 → Phase 2**: T001 before everything.
- **Phase 2 internal**: T002 → T003 → T004 → T005 (same suite, compile last). T006 → T007 → T008 (same file, sequential). T009 [P] is independent of T006 to T008 (different file). The suite (T002 to T005) is committed before any handler change.
- **Phase 3 (US1)**: needs T006 to T009. T010 → T011 → T012.
- **Phase 4 (US2)**: needs T011 (`printDeploymentsJSON`) and T012 (`jsonQueryBackend`, usage string). T013 → T014 → T015.
- **Phase 5 (US3)**: needs only T009 (the envelope); independent of Phases 3 and 4 in the handler (different file), but T018 edits `cmd/get.go` after T012/T015 to avoid conflicting edits.
- **Phase 6**: T019 after T012, T015, T018 (all help text final); T020, T021, T022 [P] after T019; T023 after all code; T024 after T023; T025 last.

## Parallel Execution Examples

- Phase 2: T009 (`json_listing.go`) alongside T006 to T008 (`deployments.go`).
- After T009 and T008: T010/T011 (US1 handler, `deployments_json*.go`) alongside T016/T017 (US3 handler, `teams*.go`); then T012, T015, T018 in sequence in `cmd/get.go`.
- After T019: T020 (guide), T021 (`AGENTS.md`), T022 (`progress.md`) together.

## Implementation Strategy

**MVP (Phases 1 to 3)**: the suite, the shared helpers, the envelope, and `get deployed --json`. At that point scenarios (2), (3), (5), (6), (7) and the pinned scenario would pass in CI, and scripts can read every workload through the superset listing.

**Increment 2 (Phase 4)**: the two kind-filtered listings, a thin layer over US1's print path.

**Increment 3 (Phase 5)**: teams, independent of the workload path.

**Increment 4 (Phase 6)**: docs, contributor reference, progress, graph, gates, pull request.

Total: 25 tasks. Per story: US1 3 (T010 to T012), US2 3 (T013 to T015), US3 3 (T016 to T018); Setup 1, Foundational 8 (T002 to T009), Polish 7 (T019 to T025).
