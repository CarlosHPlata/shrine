# Tasks: Publish Application Ports on Localhost

**Input**: Design documents from `/specs/023-publish-host-ports/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: INCLUDED — the constitution mandates TDD (Principle V: integration test files are created before the implementation code; unit tests are written first per phase). Unit tests never touch the filesystem (stores use injected file-op functions). Iterate with `go test ./...`; run the new integration scenarios per-story with `-run` filters; the full `make test-integration` suite (~10 min) runs once as the final gate.

**Organization**: Tasks are grouped by user story (US1–US5 from spec.md) so each story is independently implementable and testable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label (US1–US5); Setup/Foundational/Polish tasks carry none

## Phase 1: Setup

**Purpose**: Confirm a green baseline and register the feature per the development workflow

- [x] T001 Verify green baseline: `go build ./...` and `go test ./...` pass on branch `023-publish-host-ports` (no file changes)
- [x] T002 Register the feature phase with acceptance criteria in specs/progress.md (constitution workflow step 2; do NOT yet touch the superseded "host-port publishing is intentionally unsupported" note — that is T046)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Manifest type, validation, allocation store, and engine op types that every user story builds on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Foundational tests (write FIRST — these MUST fail before T006–T011 land)

- [x] T003 [P] Unit tests for `Publish` YAML unmarshaling in internal/manifest/types_publish_test.go: `publish: true` → HostPort 0; `publish: false` → nil; `publish: {hostPort: 8080}` → 8080; omission → nil; invalid node kinds/keys → error naming the field (contract: contracts/manifest-schema.md)
- [x] T004 [P] Unit tests for publish validation in internal/manifest/validate_publish_test.go: hostPort 0 and 1024–29999/32768–65535 pass; <1024, >65535, and 30000–32767 fail; errors accumulate in the multi-error report alongside other spec errors
- [x] T005 [P] Unit tests for the local host-port store in internal/state/local/hostports_test.go using injected file-op functions (no real filesystem): idempotent allocate returns existing entry; first-free scan skips reserved and persisted ports; claim upserts own key and returns `ErrHostPortTaken` for another key's port; release is idempotent soft-success; team release removes all `team/*` entries; exhaustion returns `ErrNoAvailableHostPorts` with no state change; save failure rolls back in-memory maps; file format `team/app=port` sorted with `#` comments tolerated (contract: contracts/hostport-store.md)

### Foundational implementation (only start once T003–T005 are written and failing)

- [x] T006 Add `Publish` type with custom `yaml.v3` unmarshaler, `Networking.Publish *Publish` field, and `shouldAttachToPlatform()` boolean method in internal/manifest/types.go (makes T003 pass)
- [x] T007 Add publish range/exclusion checks to `validateApplicationSpec` in internal/manifest/validate.go (makes T004 pass)
- [x] T008 [P] Define `HostPortStore` interface, `HostPortMap`, and sentinels `ErrHostPortNotFound`/`ErrNoAvailableHostPorts`/`ErrHostPortTaken` in internal/state/hostports.go (contract: contracts/hostport-store.md)
- [x] T009 Implement the local store with `hostports.txt` persistence (atomic temp+rename, `sync.Mutex`, eager load, rollback-on-save-failure, constants 30000–32767, reserved ports seeded at construction) in internal/state/local/hostports.go (makes T005 pass; depends on T008)
- [x] T010 Wire `HostPorts` into `state.Store` in internal/state/state.go, construct it in `NewLocalStore` in internal/state/local/local.go, and pass the reserved gateway ports (Traefik HTTP, TLS, dashboard from config) from the composition root in internal/app/components.go
- [x] T011 Add `PublishPort` struct, `CreateContainerOp.Publish *PublishPort`, and `PortBinding.HostIP` in internal/engine/backends.go

**Checkpoint**: `go test ./...` green — user story implementation can now begin

---

## Phase 3: User Story 1 - Reach an application on a chosen localhost port (Priority: P1) 🎯 MVP

**Goal**: `networking.publish: {hostPort: N}` deploys a container whose port answers on `localhost:N` (loopback-only), visible in dry-run, recreated on port change

**Independent Test**: Deploy one app with an explicit port; `curl localhost:<port>` succeeds; `docker inspect` shows HostIp `127.0.0.1`; a no-publish app shows zero bindings

### Tests for User Story 1 (write FIRST — must fail before T016–T018)

- [x] T012 [US1] Create tests/integration/publish_test.go (`NewDockerSuite`) with explicit-port scenarios: app reachable via HTTP on `localhost:<port>`; `docker inspect` HostConfig.PortBindings shows `{"HostIp":"127.0.0.1","HostPort":"<port>"}`; dry-run prints `publish=127.0.0.1:<port>-><containerPort>/tcp` with no state change; changing the port recreates the container on the new port and frees the old; an app without `publish` has no port bindings (spec US1 AS-1..5)
- [x] T013 [P] [US1] Unit tests for the engine projection in internal/engine/engine_publish_test.go: `op.Publish` set from `Networking.Publish` with `ContainerPort` = `spec.Port`; nil when absent; `op.ExposeToPlatform` = derived `shouldAttachToPlatform()`
- [x] T014 [P] [US1] Unit tests for the Docker backend explicit path in internal/engine/local/dockercontainer/docker_container_publish_test.go (fakeDockerAPI pattern): `resolvePublishBinding` claims the explicit port and appends `PortBinding{HostIP: "127.0.0.1"}`; `buildPortBindings` passes HostIP through to `nat.PortBinding`; `configHash` port spec includes HostIP only when non-empty (existing hash strings byte-identical — contract: research.md R9)
- [x] T015 [P] [US1] Unit tests for the dry-run publish line in internal/engine/dryrun/dry_run_container_test.go: explicit port prints `publish=127.0.0.1:<port>-><containerPort>/tcp`; no line when `op.Publish` nil (contract: contracts/operator-output.md)

### Implementation for User Story 1

- [x] T016 [US1] Project manifest → op in `deployApplication` in internal/engine/engine.go: set `op.Publish` and derive `op.ExposeToPlatform` via `shouldAttachToPlatform()` (makes T013 pass)
- [x] T017 [US1] Implement `resolvePublishBinding` (explicit → `ClaimHostPort`), HostIP passthrough in `buildPortBindings`, and the conditional-HostIP `configHash` port spec in internal/engine/local/dockercontainer/docker_container.go (makes T014 pass)
- [x] T018 [US1] Print the publish line for explicit ports in internal/engine/dryrun/dry_run_container.go (makes T015 pass)
- [x] T019 [US1] Run the US1 integration scenarios: `go test -tags integration ./tests/integration/ -run TestPublish` — all green

**Checkpoint**: MVP — an operator can publish on a fixed port and hit `localhost:<port>`

---

## Phase 4: User Story 2 - Conflicting user-set ports fail fast at dry run and deploy (Priority: P2)

**Goal**: Duplicate explicit claims, gateway-reserved ports, and ports persisted for other apps abort both dry-run and deploy before any change, all conflicts in one deterministic report

**Independent Test**: Two manifests claiming the same port fail dry-run and deploy with both apps named and zero containers created

### Tests for User Story 2 (write FIRST — must fail before T023–T025)

- [x] T020 [P] [US2] Extend tests/integration/publish_test.go with conflict scenarios: duplicate explicit port fails dry-run AND deploy naming both apps with no container created; explicit claim on a configured Traefik port fails as reserved; explicit claim on another app's persisted port fails as taken; several conflicts reported in one invocation in stable order; an app explicitly claiming its own persisted port succeeds (spec US2 AS-1..6)
- [x] T021 [P] [US2] Unit tests for `DetectHostPortCollisions` in internal/planner/hostports_test.go: all three conflict kinds; self-adoption passes; message formats and deterministic sorted joined error per contracts/planner-errors.md; empty set / no-publish set → nil
- [x] T022 [P] [US2] Unit tests in internal/planner/plan_test.go (extend if it exists, else create): `Plan` invokes the detector for FilterNone, FilterTeam, FilterApp, and FilterRes and returns `PlanResult.Error` on conflict

### Implementation for User Story 2

- [x] T023 [US2] Implement `PortContext` and `DetectHostPortCollisions` in internal/planner/hostports.go (makes T021 pass)
- [x] T024 [US2] Add the `PortContext` parameter to `Plan` and call the detector for ALL filters in internal/planner/plan.go (makes T022 pass)
- [x] T025 [US2] Assemble `PortContext` (reserved gateway ports from config + `ListHostPorts()` snapshot) in `Deploy` and `DryRun` in internal/handler/deploy.go and in the apply path in internal/handler/apply.go
- [x] T026 [US2] Run the US2 integration scenarios: `go test -tags integration ./tests/integration/ -run TestPublish` — all green

**Checkpoint**: Explicit ports are safe — conflicts cannot reach the engine

---

## Phase 5: User Story 3 - Automatic port allocation that stays stable across redeploys (Priority: P3)

**Goal**: `publish: true` assigns a free port from 30000–32767, reports it in deploy output, keeps it across redeploy/recreation/teardown, and releases it only via `delete application` / `delete team`

**Independent Test**: Deploy with `publish: true`, note the port, force container recreation and teardown+redeploy — same port each time; `shrine delete application` releases it

### Tests for User Story 3 (write FIRST — must fail before T031–T035)

- [x] T027 [US3] Extend tests/integration/publish_test.go with automatic-mode scenarios: first deploy allocates from 30000–32767, prints `published <team>/<app> on 127.0.0.1:<port>`, app reachable; port identical across plain redeploy, hash-forced recreation, and teardown+redeploy; dry-run before first deploy prints `(auto)` and leaves hostports.txt byte-identical; dry-run after deploy prints the held port; `shrine delete application` refuses while the container exists, then releases after teardown; `shrine delete team` releases remaining allocations (spec US3 AS-1..6, AS-8; quickstart steps 3–6)
- [x] T028 [P] [US3] Extend internal/engine/local/dockercontainer/docker_container_publish_test.go: automatic path calls `AllocateHostPort` and binds the returned port; allocation error (exhaustion) fails the op wrapped with team/app and no deployment record is written; published-port event emitted after container start with the resolved port
- [x] T029 [P] [US3] Unit tests for the release surfaces in internal/handler/deployments_test.go: `DeleteApplication` refuses when Docker reports the container by name, releases the port and drops the record when absent, is idempotent when nothing is held, honors `--dry-run` (no writes); `DeleteTeam` releases all `team/*` ports beside its subnet release
- [x] T030 [P] [US3] Extend internal/engine/dryrun/dry_run_container_test.go: automatic with no snapshot entry prints `(auto)`; snapshot entry prints the held port

### Implementation for User Story 3

- [x] T031 [US3] Add the automatic branch to `resolvePublishBinding` (`AllocateHostPort`) and emit the published-port event after `ContainerStart` in internal/engine/local/dockercontainer/docker_container.go, with the event type added in internal/engine/events.go (makes T028 pass)
- [x] T032 [US3] Thread a read-only `HostPortMap` snapshot through `NewDryRunEngine` in internal/engine/dryrun/dry_run_engine.go into `DryRunContainerBackend` in internal/engine/dryrun/dry_run_container.go (held-port / `(auto)` print), loading it in `DryRun` in internal/handler/deploy.go (makes T030 pass)
- [x] T033 [US3] Implement `DeleteApplication` (Docker-authoritative refusal, port release, stale-record removal, `--dry-run`, `--team` disambiguation with all-teams search) in internal/handler/deployments.go (makes T029 pass)
- [x] T034 [US3] Add the `shrine delete application <name>` subcommand (thin dispatcher, `--team`/`-t`, `--dry-run`) in cmd/delete.go
- [x] T035 [US3] Release team host ports in `DeleteTeam` in internal/handler/teams.go, with the released-count output line per contracts/operator-output.md
- [x] T036 [US3] Run the US3 integration scenarios: `go test -tags integration ./tests/integration/ -run TestPublish` — all green

**Checkpoint**: Automatic ports are assigned, discoverable, stable, and releasable

---

## Phase 6: User Story 4 - Publishing works without a second switch; platform exposure stays independent (Priority: P4)

**Goal**: Publish alone implies platform-network attachment (visible in dry-run); `exposeToPlatform` alone publishes nothing; publish grants no cross-team dependency rights

**Independent Test**: Three apps — publish-only, exposure-only, both — verified independently for network attachment, port exposure, and cross-team dependency rejection

### Tests for User Story 4 (write FIRST — must fail before T040 if any gap exists)

- [x] T037 [US4] Extend tests/integration/publish_test.go with semantics scenarios: publish-only app is attached to `shrine.platform` (docker inspect Networks) and deploys with no validation error; exposure-only app is attached but exposes zero host ports; both-set app deploys with no warning; a cross-team dependency on a publish-only app is rejected at plan time; dry-run for a publish-only app prints the platform-attachment line next to the publish line (spec US4 AS-1..5)
- [x] T038 [P] [US4] Extend internal/engine/engine_publish_test.go with the derivation matrix (the four combinations from contracts/manifest-schema.md) asserting `op.ExposeToPlatform` and, unchanged, that the routing gate still requires the RAW `ExposeToPlatform` field (publish-only + routing.domain → no route written)
- [x] T039 [P] [US4] Regression unit test in internal/planner/resolve_publish_test.go: cross-team dependency on a publish-only target fails exactly as for any non-exposed target (raw-field gate untouched)

### Implementation for User Story 4

- [x] T040 [US4] Close any gaps T037–T039 surface (derivation landed in T016; dry-run attachment line prints from the derived op field) — touch only internal/engine/engine.go / internal/engine/dryrun/dry_run_container.go if a test fails
- [x] T041 [US4] Run the US4 integration scenarios: `go test -tags integration ./tests/integration/ -run TestPublish` — all green

**Checkpoint**: The combination table's behavior holds end to end

---

## Phase 7: User Story 5 - Reference documentation (Priority: P5)

**Goal**: The manifest reference and a how-to guide let an operator learn the feature — including the combination table — without reading source or specs

**Independent Test**: SC-007's four questions answerable from the published docs alone

### Implementation for User Story 5

- [x] T042 [P] [US5] Add the `networking.publish` entry to docs/content/reference/manifest-schema.md: both YAML forms with examples, valid explicit range (1024–65535 excluding 30000–32767), automatic range, and the four-row combination table copied VERBATIM from specs/023-publish-host-ports/contracts/manifest-schema.md (FR-018; spec US5 AS-1..2)
- [x] T043 [P] [US5] Create the how-to guide docs/content/guides/publish-localhost.md: fixed vs automatic ports, port stability across redeploys/teardown, conflict behavior at dry-run and deploy, releasing via `delete application`/`delete team` (spec US5 AS-3..4)
- [x] T044 [US5] Regenerate the CLI reference for the new `delete application` subcommand: `make docs-gen-cli` (output under docs/content/cli/)
- [x] T045 [US5] SC-007 self-check: verify the four questions (fixed port how; redeploy keeps port; is exposure also needed; what stops a deploy) are answerable from docs/content/ alone; fix any gap in the pages from T042–T043

**Checkpoint**: All five user stories delivered

---

## Phase 8: Polish & Cross-Cutting Concerns

- [x] T046 [P] Amend the constitution's Technical Stack line ("host-port publishing is unsupported by design" → Traefik by default; per-application loopback-only publishing via `networking.publish`) with a MINOR bump 1.1.1 → 1.2.0 and sync-impact comment in .specify/memory/constitution.md, and update the superseded note at specs/progress.md line ~101 (research.md R11)
- [x] T047 [P] Sync AGENTS.md with the new manifest field and the `shrine delete application` command
- [x] T048 Walk specs/023-publish-host-ports/quickstart.md manually end to end and fix any drift between it and actual output
- [x] T049 Run `graphify update .` to refresh the knowledge graph after code changes
- [x] T050 Final gates: `go test ./...` green locally; the FULL integration suite is delegated to the CI pipeline per operator direction — mark the feature phase `[x]` in specs/progress.md once CI is green

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: none
- **Foundational (Phase 2)**: after Setup — BLOCKS all user stories
- **US1 (Phase 3)**: after Foundational
- **US2 (Phase 4)**: after Foundational; its integration scenarios extend the publish_test.go file created in T012
- **US3 (Phase 5)**: after Foundational; T031 extends `resolvePublishBinding` from T017, so US3 implementation follows US1
- **US4 (Phase 6)**: verification-heavy; depends on T016 (derivation) from US1
- **US5 (Phase 7)**: content depends on final behavior of US1–US4; T044 needs T034
- **Polish (Phase 8)**: after all desired stories

### Within Each User Story

- Tests written first and observed failing (constitution TDD) → implementation → story integration run
- Same-file sequencing across phases: tests/integration/publish_test.go grows in T012 → T020 → T027 → T037 (one story at a time); docker_container_publish_test.go in T014 → T028; dry_run_container_test.go in T015 → T030; engine_publish_test.go in T013 → T038

### Parallel Opportunities

- Phase 2 tests: T003, T004, T005 (three files, three authors)
- Phase 2 implementation: T006→T007 (manifest) ∥ T008→T009 (store) ∥ T011 (op types); T010 after T009
- US1 tests: T013, T014, T015 in parallel once T012's scenarios are drafted
- US2 tests: T020, T021, T022 in parallel; then T023 ∥ T025 (T024 after T023)
- US3 tests: T028, T029, T030 in parallel alongside T027; then T031 ∥ T032 ∥ T033 (T034 after T033)
- US4 tests: T037, T038, T039 in parallel
- US5: T042 ∥ T043
- Polish: T046 ∥ T047

## Parallel Example: Foundational Phase

```bash
# Three test authors in parallel (different files):
Task: "Unit tests for Publish YAML unmarshaling in internal/manifest/types_publish_test.go"
Task: "Unit tests for publish validation in internal/manifest/validate_publish_test.go"
Task: "Unit tests for the local host-port store in internal/state/local/hostports_test.go"

# Then three implementation tracks in parallel:
Track A: T006 → T007          (internal/manifest)
Track B: T008 → T009 → T010   (internal/state, then wiring)
Track C: T011                 (internal/engine/backends.go)
```

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1 + Phase 2 (foundation)
2. Phase 3 (US1) → **STOP and VALIDATE**: explicit port reachable on localhost, dry-run clean
3. Demo-able: `publish: {hostPort: 8080}` + `curl localhost:8080`

### Incremental Delivery

1. US1 → explicit ports work (MVP)
2. US2 → explicit ports are safe (conflicts fail fast)
3. US3 → automatic ports with stability and release lifecycle
4. US4 → semantics guaranteed by tests
5. US5 → documented
6. Polish → constitution amendment, AGENTS.md sync, full integration gate

Each story leaves `go test ./...` green and its `-run TestPublish` integration scenarios passing; the full ~10-minute integration suite runs only at T050.

## Notes

- Unit tests must not touch the filesystem — the store tests inject file-op functions (project test policy)
- Commit after each task or logical group; every phase ends with `go test ./...` green
- Stop at any checkpoint to validate the story independently
