# Tasks: Deployed Version in get and describe

**Input**: Design documents from `/specs/031-deployed-version-columns/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: INCLUDED — the constitution mandates TDD (Principle V: integration test files are written before the implementation; unit tests are written first per phase). Unit tests never touch the filesystem (the store is tested through injected file-op functions; the backend through the fake Docker API; the handler through string-returning formatters). Iterate with `go test ./...`; compile the integration suite with `go vet -tags integration ./tests/integration/...`; the integration scenarios run in CI only, never locally.

**Organization**: Tasks are grouped by user story (US1–US3 from spec.md) so each story is independently implementable and testable. The integration scenarios for all three stories are written first, in the Foundational phase, because the ticket names them as the gate and they share two helpers.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label (US1–US3); Setup/Foundational/Polish tasks carry none

## Phase 1: Setup

**Purpose**: Confirm a green baseline on the feature branch

- [x] T001 Verify green baseline on branch `031-deployed-version-columns`: `go build ./...`, `go test ./...`, and `go vet -tags integration ./tests/integration/...` pass with no file changes

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The integration scenarios (written first, per the ticket), and the record shape every story reads

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Integration scenarios (write FIRST — they compile now and go red in CI until the stories land)

- [x] T002 [P] Extend `TestGetDocker` in tests/integration/get_test.go: `get deployed` prints a `VERSION` header and the `whoami-res` and `test-cache` rows each carry `traefik/whoami`; `get deployed --team shrine-deploy-test` does the same; `get applications` and `get resources` carry it on their rows; after `SeedLegacyDeploymentRecords(tc, testTeam)` the two rows show `-` and not `traefik/whoami`; after a second `deploy` of the same fixture the rows show `traefik/whoami` again (spec US1 AS-1..3, AS-6; US3 AS-1, AS-3, AS-4)
- [x] T003 [P] Extend `TestDescribeDocker` in tests/integration/describe_test.go: `describe app whoami` prints an `Image:` line with `traefik/whoami` and a `Pull policy:` line with `Always`, with and without `--team`; after deploying the `resources` fixture in the test, `describe resource test-cache` prints the same two lines; after `SeedLegacyDeploymentRecords(tc, testTeam)`, `describe app whoami` succeeds and shows `-` on both lines (spec US2 AS-1..4; US3 AS-2)
- [x] T004 Add the two helpers the scenarios use: `AssertOutputLineContains(anchor, want string)` in tests/integration/testutils/assert_general.go (fails naming the anchor when no stdout line contains it, or when that line lacks `want`) and `SeedLegacyDeploymentRecords(tc *TestCase, team string)` in tests/integration/testutils/assert_state.go (rewrites `<StateDir>/<team>/deployments.txt` keeping the first four whitespace-separated fields of every line); then `go vet -tags integration ./tests/integration/...` compiles

### Foundational unit tests (write FIRST — must fail before T006–T007)

- [x] T005 Rewrite internal/state/local/deployments_test.go on an in-memory file map injected through `newDeploymentStoreWithFileOps` (no `t.TempDir`, no `os.MkdirAll`, no file writes): keep every case the file pins today (comment and blank-line tolerance, three- and four-field lines, persistence across store instances, update, remove, remove of unknown, empty team, team isolation) and add: a four-field line loads with empty `Image` and `Policy`; a six-field line loads fully; `Record` of a fully populated deployment writes `Kind Name ContainerID ConfigHash Image Policy`; a legacy line present when another record is written still loads afterwards (contract: contracts/deployment-record.md)

### Foundational implementation (only start once T005 is written and failing)

- [x] T006 Add `Image string` and `Policy string` to `Deployment` in internal/state/deployments.go with the two field comments from design section 3.3
- [x] T007 Implement the store changes in internal/state/local/deployments.go: `readFile`/`writeFile` fields using the `readFileFn`/`writeFileFn` types from hostports.go; unexported `newDeploymentStoreWithFileOps(baseDir, read, write)`; `NewDeploymentStore` wires `os.ReadFile` and a writer that creates the team directory (0700) then calls `atomicWriteFile`; `loadTeam` splits with `strings.Fields`, skips lines with fewer than three fields, and reads fields four to six through a `fieldAt(fields, i)` helper; `saveTeam` writes six fields per line, sorted by name. Derive the temp-file prefix in `atomicWriteFile` (internal/state/local/hostports.go) from the target's base name so it serves both stores (makes T005 pass)

**Checkpoint**: `go test ./...` green; `go vet -tags integration ./tests/integration/...` compiles — user story implementation can now begin

---

## Phase 3: User Story 1 - See which version each artifact was deployed with (Priority: P1) 🎯 MVP

**Goal**: Deploy records the manifest's reference and effective policy; `get deployed` / `get applications` / `get resources` print a VERSION column after KIND, from state alone

**Independent Test**: Deploy the standard fixtures; the three listing commands, with and without `--team`, show `traefik/whoami` on every row in a VERSION column between KIND and CONTAINER ID (CI: T002)

### Tests for User Story 1 (write FIRST — must fail before T010–T011)

- [x] T008 [P] [US1] Create internal/handler/deployments_output_test.go with tests for `formatDeploymentsTable`: the header lists `TEAM`, `NAME`, `KIND`, `VERSION`, `CONTAINER ID` in that order (assert by index); a row with `Image: "reg:lab/hello-api:1.2.0"` prints it verbatim between the kind and the short container id; a row with an empty `Image` prints `-`; the separator line is as long as the header (contract: contracts/operator-output.md)
- [x] T009 [P] [US1] Create internal/engine/local/dockercontainer/docker_container_record_test.go: with the `startCapableFakeDockerAPI`, `fakeDeploymentStore`, and `testRegistries` already defined in the package, `CreateContainer` for `Image: "reg:myregistry/traefik/whoami:latest"`, `ImagePullPolicy: "IfNotPresent"` records `Image` equal to the unexpanded `reg:myregistry/traefik/whoami:latest` (not `docker.io/...`), `Policy` equal to `IfNotPresent`, and the kind, name, container id, and a non-empty config hash

### Implementation for User Story 1

- [x] T010 [US1] In internal/handler/deployments.go add `valueOrUnknown(value string) string` (returns `-` for empty) and `formatDeploymentsTable(deployments []teamedDeployment) string` with row format `%-20s %-30s %-15s %-40s %-15s`, VERSION after KIND filled from `valueOrUnknown(td.Deployment.Image)`, and a separator computed from the header length; make `printDeploymentsTable` print the formatted string (makes T008 pass)
- [x] T011 [US1] In internal/engine/local/dockercontainer/docker_container.go add `newDeploymentRecord(op) state.Deployment` (Kind, Name, Image, Policy from the op) called at the top of `CreateContainer` before `op.Image = expanded`, with a one-line WHY comment; set `record.ConfigHash = configHash(op, digest)`; replace the `wantHash string` parameter of `ensureRunning` and `createFreshContainer` with `record state.Deployment`; change `recordDeployment` to `(team string, record state.Deployment, containerID string)` setting `ContainerID` then calling `Record` (makes T009 pass)
- [x] T012 [US1] `go build ./... && go test ./...` green; build the binary and run `go run . deploy --path test/smock/ --dry-run` to confirm the dry-run output is unchanged and no state is written

**Checkpoint**: MVP — a deploy records the version and the listing shows it

---

## Phase 4: User Story 2 - See the image and the pull policy of one artifact (Priority: P2)

**Goal**: `describe app` and `describe resource` print `Image:` and `Pull policy:` after `Kind:`

**Independent Test**: Deploy the standard fixtures; `describe app whoami` and `describe resource test-cache` show `Image:        traefik/whoami` and `Pull policy:  Always` (CI: T003)

### Tests for User Story 2 (write FIRST — must fail before T014)

- [x] T013 [US2] Extend internal/handler/deployments_output_test.go with tests for `formatDeploymentDetail`: the lines appear in the order `Name:`, `Team:`, `Kind:`, `Image:`, `Pull policy:`, `Container ID:`, `Config Hash:`; `Image:` and `Pull policy:` carry the record's values padded to the fourteen-character label column; both print `-` when the record's fields are empty; the config-hash preview rule (sixteen characters plus `...`) is unchanged

### Implementation for User Story 2

- [x] T014 [US2] In internal/handler/deployments.go add `formatDeploymentDetail(team string, d state.Deployment) string` printing the two new lines after `Kind:` through `valueOrUnknown`, and make `printDeploymentDetail` print it (makes T013 pass); `go test ./...` green

**Checkpoint**: describe shows the image and the policy

---

## Phase 5: User Story 3 - Records from a previous release keep working and heal themselves (Priority: P3)

**Goal**: Legacy four- and three-field records list and describe with `-`; the next deploy, including one that only finds the container up to date, rewrites the record with the version

**Independent Test**: Rewrite a team's `deployments.txt` to four fields, list and describe (`-`), redeploy unchanged manifests, list again (`traefik/whoami`) (CI: T002 legacy and redeploy scenarios, T003 legacy scenario)

### Tests for User Story 3 (write FIRST — must fail before T016)

- [x] T015 [US3] Extend internal/engine/local/dockercontainer/docker_container_record_test.go with the up-to-date path: run `CreateContainer` once on the fresh path to learn the recorded config hash; replace the store's record with a legacy copy (same hash, empty `Image` and `Policy`); swap in a fake whose `ContainerInspect` returns an existing running container; run `CreateContainer` again and assert no `ContainerCreate` or `ContainerRemove` happened and the last record carries `Image` and `Policy` (spec FR-007, T1-05)

### Implementation for User Story 3

- [x] T016 [US3] Confirm T015 passes with the T011 implementation (the up-to-date path already calls `recordDeployment`); if it does not, fix the threading in internal/engine/local/dockercontainer/docker_container.go without touching the config-hash inputs
- [x] T017 [US3] Confirm the store's legacy cases from T005 pass and `go vet -tags integration ./tests/integration/...` still compiles the legacy scenarios of T002 and T003

**Checkpoint**: All three user stories delivered in code; CI will exercise the integration scenarios

---

## Phase 6: Polish & Cross-Cutting Concerns

- [x] T018 [P] Update the `deployments.txt` line in the State Directory Layout block of AGENTS.md to `<kind> <name> <container-id> <config-hash> <image> <pull-policy>`
- [x] T019 [P] Add the spec 031 entry to specs/progress.md in the project's usual form (title, `see specs/031-deployed-version-columns/` and issue #52, what changed, acceptance by SC, gate naming `TestGetDocker` and `TestDescribeDocker` — CI executes), placed before the spec 030 entry
- [x] T020 Final local gates: `gofmt -l .` prints nothing; `go vet ./...`; `go build ./... && go test ./...` green; `go vet -tags integration ./tests/integration/...` green; every task above checked
- [x] T021 Rebase onto `origin/main` if it moved (keep both entries in specs/progress.md and .specify/feature.json), push `031-deployed-version-columns`, open the pull request against main from `.github/pull_request_template.md` with `Closes #52` in Why and the one-line Constitution Check for the `internal/engine/local/` change, run `/shrine-pr-review`, fix every real finding, push again

Note: `graphify update .` runs on main after merge by the orchestrator, not on this branch.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: none
- **Foundational (Phase 2)**: after Setup — BLOCKS all user stories. T002 and T003 depend on T004's helpers to compile; T005 before T006–T007
- **US1 (Phase 3)**: after Foundational
- **US2 (Phase 4)**: after Foundational; shares internal/handler/deployments.go and its test file with US1, so follows US1
- **US3 (Phase 5)**: after US1 (its backend test extends T009's file and relies on T011's threading)
- **Polish (Phase 6)**: after all stories

### Within Each User Story

- Tests written first and observed failing (constitution TDD) → implementation → `go test ./...`
- Same-file sequencing: internal/handler/deployments_output_test.go grows T008 → T013; docker_container_record_test.go grows T009 → T015; internal/handler/deployments.go changes in T010 then T014

### Parallel Opportunities

- Phase 2: T002 ∥ T003 (different files), then T004; T005 can be written alongside them
- US1 tests: T008 ∥ T009; US1 implementation: T010 ∥ T011
- Polish: T018 ∥ T019

## Parallel Example: Foundational Phase

```bash
# Two integration scenario authors in parallel (different files):
Task: "Extend TestGetDocker in tests/integration/get_test.go"
Task: "Extend TestDescribeDocker in tests/integration/describe_test.go"

# Then the helpers they share, and the store test alongside:
Task: "Add AssertOutputLineContains and SeedLegacyDeploymentRecords in tests/integration/testutils/"
Task: "Rewrite internal/state/local/deployments_test.go on injected file ops"
```

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1 + Phase 2 (integration scenarios, record, store)
2. Phase 3 (US1) → **STOP and VALIDATE**: `go test ./...` green, dry run unchanged
3. Demo-able: deploy, then `shrine get deployed` shows the VERSION column

### Incremental Delivery

1. US1 → the record carries the version and the listing shows it (MVP)
2. US2 → describe shows image and policy
3. US3 → legacy records tolerated and healed, pinned by the backend's up-to-date path test
4. Polish → AGENTS.md, progress entry, gates, pull request, review

Each story leaves `go test ./...` green and the integration suite compiling; CI runs the suites once the pull request is open.

## Notes

- Unit tests must not touch the filesystem — the store tests inject file-op functions (project test policy)
- Integration tests are isolated from internal packages; the two helpers live in testutils and T5 reuses the line assertion
- Commit after each logical group with the attribution line; every phase ends with `go test ./...` green
- Do not use the local Docker daemon; `--dry-run` is the only local run of the binary
