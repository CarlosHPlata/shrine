# Tasks: Publish Application Ports on Localhost

**Input**: Design documents from `/specs/023-publish-host-ports/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/ (4 files), quickstart.md

**Tests**: INCLUDED — Constitution Principle V (Integration-Test Gate) and research R13 mandate TDD: the integration scenarios in `tests/integration/publish_test.go` are written **first** in each story phase and must fail before implementation begins. Unit tests ship inside their implementation tasks and must never touch the filesystem (injected file-op functions, per project policy). The full integration suite (~10 min) runs only as the final gate; iterate with `go test ./...` and targeted `go test ./tests/integration -run <TestName>` runs.

**Organization**: Tasks are grouped by user story so each story is an independently implementable, independently testable increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1–US5)
- Every task names its exact file path(s)

---

## Phase 1: Setup (Governance Reconciliation)

**Purpose**: Amend the constitution before behavior and constitution diverge (research R11). No project scaffolding is needed — the Go module, test harness, and directory layout already exist.

- [ ] T001 Amend the constitution's Technical Stack line from "External access: Traefik only; host-port publishing is unsupported by design" to "External access: Traefik by default; per-application loopback-only host publishing via `networking.publish`" in `.specify/memory/constitution.md`, with a MINOR version bump 1.1.1 → 1.2.0 and amendment rationale per the governance section
- [ ] T002 [P] Update the superseded design note ("Host-port publishing is intentionally unsupported…") at `specs/progress.md:101` to state that feature 023 ships `networking.publish` with a `HostPortStore` allocator, as that note anticipated

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The manifest type, allocation store, and engine op vocabulary that every user story builds on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [ ] T003 Add `Publish` struct (`HostPort int`, 0 = automatic) with a custom `yaml.v3` unmarshaler (scalar `true` → automatic; scalar `false` → nil/omitted; mapping `{hostPort: N}` → explicit; any other node kind or key → parse error naming the field) plus `Publish *Publish` field and `shouldAttachToPlatform()` method (`ExposeToPlatform || Publish != nil`) on `Networking` in `internal/manifest/types.go`; unit tests for all four YAML forms in `internal/manifest/parser_test.go` (data-model.md §1.1–1.2)
- [ ] T004 Add explicit-port checks to `validateApplicationSpec` in `internal/manifest/validate.go`: `publish.hostPort` must be 0 or within 1024–65535 and must not fall inside 30000–32767 (automatic range); errors append to the existing per-manifest multi-error report, never fail fast individually; unit tests in `internal/manifest/validate_test.go` (contracts/manifest-schema.md validation table) — depends on T003
- [ ] T005 [P] Define `HostPortStore` interface (`AllocateHostPort`, `ClaimHostPort`, `GetHostPort`, `ReleaseHostPort`, `ReleaseTeamHostPorts`, `ListHostPorts`), `HostPortMap` (`map[string]int`, key `team/app`), and sentinel errors `ErrHostPortNotFound`, `ErrNoAvailableHostPorts`, `ErrHostPortTaken` in `internal/state/hostports.go` (contracts/hostport-store.md)
- [ ] T006 Implement the local store in `internal/state/local/hostports.go`, mirroring `internal/state/local/subnets.go`: `hostports.txt` with sorted `team/app=port` lines, forgiving parser (skip `#` and malformed lines), atomic temp+rename saves with in-memory rollback on failure, `sync.Mutex`, reserved ports seeding the taken set at construction (never persisted), automatic range constants 30000–32767, idempotent `AllocateHostPort` (existing entry returned unchanged; lowest free slot otherwise; `ErrNoAvailableHostPorts` on exhaustion with no state change) and `ClaimHostPort` (idempotent upsert for own key, overwrite releases previous value, `ErrHostPortTaken` when another key holds the port); unit tests with injected file-op functions — no real filesystem — in `internal/state/local/hostports_test.go` — depends on T005
- [ ] T007 Wire the store: add `HostPorts HostPortStore` to `Store` in `internal/state/state.go`; construct `NewHostPortStore(baseDir, reserved)` in `internal/state/local/local.go`; supply the Traefik gateway's reserved host ports (HTTP, TLS, dashboard from config) from the composition root in `internal/app` — depends on T006
- [ ] T008 [P] Add engine op vocabulary in `internal/engine/backends.go`: `PublishPort` struct (`HostPort int` 0 = automatic, `ContainerPort int`), `Publish *PublishPort` field on `CreateContainerOp`, and `HostIP string` field on `PortBinding` ("" keeps today's 0.0.0.0 behavior) (data-model.md §3)

**Checkpoint**: `go test ./...` green; no user-visible behavior change yet.

---

## Phase 3: User Story 1 - Reach an application on a chosen localhost port (Priority: P1) 🎯 MVP

**Goal**: An application manifest declaring `publish: {hostPort: N}` answers on `localhost:N` after deploy, bound to loopback only; dry-run previews the mapping; non-publishing apps are byte-for-byte unchanged.

**Independent Test**: Deploy one app with an explicit port; `curl localhost:<port>` succeeds from the host; `docker inspect` shows `HostIp: 127.0.0.1`; a manifest without `publish` produces no host port.

### Tests for User Story 1 (write FIRST — must fail before implementation)

- [ ] T009 [US1] Add explicit-publish scenarios to a new `tests/integration/publish_test.go` using `NewDockerSuite`: (a) app with `publish: {hostPort: N}` answers over HTTP on `localhost:N`; (b) `docker inspect` shows `HostConfig.PortBindings` with `HostIp` `127.0.0.1` (the 011-tlsPort precedent, spec AS-5 loopback proxy); (c) changing the manifest to another port and redeploying moves the binding (old port dead, new port live); (d) an app without `publish` exposes no host port; (e) `deploy --dry-run` prints the `publish=127.0.0.1:<port>-><containerPort>/tcp` mapping and creates/changes nothing

### Implementation for User Story 1

- [ ] T010 [P] [US1] Project manifest → op in `deployApplication` in `internal/engine/engine.go`: set `op.Publish = &PublishPort{HostPort: spec.Networking.Publish.HostPort, ContainerPort: spec.Port}` when `Publish != nil`, and derive `op.ExposeToPlatform = spec.Networking.shouldAttachToPlatform()`; the routing gate (`engine.go:187`) and planner cross-team gates keep reading the raw `ExposeToPlatform` field; unit tests in `internal/engine/engine_test.go` (data-model.md §4.1, research R8)
- [ ] T011 [P] [US1] Resolve the explicit binding in `internal/engine/local/dockercontainer/docker_container.go`: new `resolvePublishBinding` helper calling `ClaimHostPort(team, name, port)` and appending `PortBinding{HostIP: "127.0.0.1", HostPort: <port>, ContainerPort: op.Publish.ContainerPort, Protocol: "tcp"}`, invoked from both `createFreshContainer` and `isContainerUpToDate` before `buildPortBindings` and `configHash` (idempotent store calls make double invocation safe); pass `HostIP` through `buildPortBindings` to `nat.PortBinding.HostIP`; include the HostIP segment in the config-hash port spec **only when non-empty** (`127.0.0.1:8080:3000/tcp` vs unchanged `8080:3000/tcp`) so existing Traefik hashes stay byte-identical (research R9/SC-006); plumb the `HostPortStore` into the backend where it is constructed (`docker_backend.go`); unit tests via the existing `fakeDockerAPI` pattern including a hash-stability case in `internal/engine/local/dockercontainer/docker_container_test.go`
- [ ] T012 [P] [US1] Print the explicit publish detail line `publish=127.0.0.1:<hostPort>-><containerPort>/tcp` under the `[DOCKER] ContainerCreate:` entry in `internal/engine/dryrun/dry_run_container.go` (contracts/operator-output.md); unit test for the line format

**Checkpoint**: US1 integration scenarios pass (`go test ./tests/integration -run <PublishTestName>` against a real Docker daemon); `go test ./...` green. MVP deliverable.

---

## Phase 4: User Story 2 - Conflicting user-set ports fail fast at dry run and deploy (Priority: P2)

**Goal**: Duplicate explicit claims, claims on gateway-reserved ports, and claims on another app's persisted allocation abort both dry-run and deploy before any change, reporting every conflict in one deterministic invocation.

**Independent Test**: Two manifests claiming the same explicit port make both dry-run and deploy exit non-zero naming the port and both apps, with zero containers created.

### Tests for User Story 2 (write FIRST — must fail before implementation)

- [ ] T013 [US2] Add conflict scenarios to `tests/integration/publish_test.go`: (a) two manifests claiming the same explicit port fail dry-run AND deploy with `host port collision: port N declared by "team/a" and "team/b"` and no container is created; (b) a claim on a Traefik-reserved gateway port fails with the `host port reserved` message; (c) a claim on a port persisted for a different application fails with the `host port taken` message; (d) an app explicitly claiming its own persisted port deploys successfully (self-adoption, FR-010); (e) several distinct conflicts are all reported in one invocation in stable order (contracts/planner-errors.md)

### Implementation for User Story 2

- [ ] T014 [US2] Implement `PortContext` (`Reserved []int`, `Persisted state.HostPortMap`) and `DetectHostPortCollisions(set *ManifestSet, ports PortContext) error` in a new `internal/planner/hostports.go`, following the `DetectRoutingCollisions` discipline in `internal/planner/collisions.go`: deterministic sorted walk, all three conflict kinds accumulated, sorted joined single error with the exact message formats of contracts/planner-errors.md, self-adoption passes silently; unit tests covering duplicate/reserved/taken/self-adoption/determinism/multi-conflict in a new `internal/planner/hostports_test.go`
- [ ] T015 [US2] Add the `PortContext` parameter to `planner.Plan` in `internal/planner/plan.go` and invoke `DetectHostPortCollisions` for **all** filters, including single-manifest apply (unlike routing collisions — reserved and persisted ports exist outside the set); update all existing callers and `internal/planner/plan_test.go` — depends on T014
- [ ] T016 [US2] Assemble `PortContext` from config gateway ports + `HostPorts.ListHostPorts()` in the deploy and dry-run paths of `internal/handler/deploy.go` and the apply path of `internal/handler/apply.go`, passing it to `planner.Plan` so a returned error aborts before any engine operation — depends on T015

**Checkpoint**: US2 integration scenarios pass; US1 scenarios still pass; conflicts detected identically for full-set, team, and single-file deploys.

---

## Phase 5: User Story 3 - Automatic port allocation that stays stable across redeploys (Priority: P3)

**Goal**: `publish: true` assigns a free port from 30000–32767, reports it in deploy output, keeps it identical across redeploy/recreation/teardown, and releases it only via `shrine delete application` or `shrine delete team`; dry-run previews without side effects.

**Independent Test**: Deploy with `publish: true`, note the reported port, force container recreation and a teardown+redeploy — port unchanged all three times; `shrine delete application` then frees it.

### Tests for User Story 3 (write FIRST — must fail before implementation)

- [ ] T017 [US3] Add automatic-allocation scenarios to `tests/integration/publish_test.go`: (a) `publish: true` deploys with a port from 30000–32767, prints `published <team>/<app> on 127.0.0.1:<port> -> <cport>/tcp`, and answers on `localhost:<port>`; (b) the port is identical across a plain redeploy, a config-change-forced recreation, and a teardown+redeploy (SC-003); (c) a dry-run before first deploy prints `publish=127.0.0.1:(auto)->…` and leaves `hostports.txt` byte-identical (SC-005); (d) a dry-run after deploy shows the held port; (e) switching the manifest to a different explicit port releases the old automatic allocation (FR-010); (f) `shrine delete application` refuses while the container exists, then releases after teardown; (g) `shrine delete team` releases every port the team held

### Implementation for User Story 3

- [ ] T018 [P] [US3] Add the automatic path to `resolvePublishBinding` in `internal/engine/local/dockercontainer/docker_container.go`: `HostPort == 0` → `AllocateHostPort(team, name)` (idempotent lookup-or-allocate); wrap `ErrNoAvailableHostPorts` with the team/app reference so exhaustion fails that app's deploy with a clear message and no allocation recorded (FR-016); unit tests in `internal/engine/local/dockercontainer/docker_container_test.go`
- [ ] T019 [US3] Emit the publish deploy event `published <team>/<app> on 127.0.0.1:<hostPort> -> <containerPort>/tcp` after container start through the engine's existing observer/event stream (`internal/engine/events.go`, emission point beside container start in `internal/engine/local/dockercontainer/docker_container.go`), carrying the backend-resolved port so operators discover automatic assignments from deploy output alone (FR-014, contracts/operator-output.md) — depends on T018
- [ ] T020 [P] [US3] Dry-run preview of held ports: `handler.DryRun` in `internal/handler/deploy.go` loads a read-only `ListHostPorts()` snapshot and passes it to `dryrun.NewDryRunEngine` in `internal/engine/dryrun/dry_run_engine.go`; the dry-run container backend in `internal/engine/dryrun/dry_run_container.go` prints the persisted port when the app holds one and `publish=127.0.0.1:(auto)-><containerPort>/tcp` otherwise — never allocating, persisting, or releasing (FR-011, research R12); unit tests for both print paths
- [ ] T021 [P] [US3] `DeleteApplication` handler in `internal/handler/deployments.go`: locate the app across all teams when `--team` is omitted and error listing candidates on ambiguity (kubectl-style); query Docker by container name and refuse while the container exists (`application "team/app" still has a running container; run "shrine teardown <team>" first` — Docker-authoritative, Principle VI); otherwise release the host port via `ReleaseHostPort`, drop the stale deployment record, print the released-port and removed-record lines of contracts/operator-output.md; soft success when nothing is held; `--dry-run` prints what would be released and writes nothing; unit tests
- [ ] T022 [US3] Add the `shrine delete application <name>` subcommand to `cmd/delete.go` following the existing `delete team` pattern: verb-first, `--team`/`-t` optional, `--dry-run` supported, wired to the T021 handler — depends on T021
- [ ] T023 [P] [US3] Release every team allocation in `DeleteTeam` in `internal/handler/teams.go` via `ReleaseTeamHostPorts`, beside the existing subnet release, printing `released <N> host ports for team <name>` when N > 0; unit tests

**Checkpoint**: US3 scenarios pass; US1–US2 still pass; repeated dry-runs leave `hostports.txt` byte-for-byte unchanged.

---

## Phase 6: User Story 4 - Publishing works without a second switch, and platform exposure stays independent (Priority: P4)

**Goal**: `publish` alone implies platform-network attachment (visible in dry-run); `exposeToPlatform` alone publishes nothing; both-set is valid and redundant; publishing never grants cross-team dependency rights.

**Independent Test**: Deploy publish-only, expose-only, and both-set apps; verify network attachment, host-port exposure, and cross-team dependency rejection independently for each.

### Tests for User Story 4 (write FIRST — must fail before implementation)

- [ ] T024 [US4] Add exposure-semantics scenarios to `tests/integration/publish_test.go`: (a) a publish-only app (no `exposeToPlatform`) deploys with no validation error, is attached to the platform network (`docker inspect` network list), and answers on its host port; (b) an expose-only app is attached to the platform network with zero host ports; (c) both-set deploys with no warning; (d) another team's `valueFrom`/dependency on a publish-only app is rejected exactly as for any non-exposed app (FR-013); (e) dry-run for a publish-only app prints the `attach to platform network=…` line alongside the publish line (AS-5)

### Implementation for User Story 4

- [ ] T025 [P] [US4] Print the `attach to platform network=<name>` detail line in `internal/engine/dryrun/dry_run_container.go` whenever the **derived** attachment on the op is true — including publish-only manifests — so the implied attachment is visible (contracts/operator-output.md); unit test
- [ ] T026 [P] [US4] Lock the semantics with unit tests (no production change expected — the derivation shipped in T003/T010): `shouldAttachToPlatform` truth table in `internal/manifest/parser_test.go`; engine projection publish-only → platform-attach op and expose-only → no `op.Publish` in `internal/engine/engine_test.go`; planner cross-team gates (`internal/planner/resolve.go:164,194`) still read the raw `ExposeToPlatform` field in `internal/planner/resolve_test.go`

**Checkpoint**: All four US1–US4 story suites pass independently; the combination table's four rows are all covered by tests.

---

## Phase 7: User Story 5 - Operators can read how publishing works in the reference documentation (Priority: P5)

**Goal**: The manifest reference and a how-to guide document both publish modes, ranges, conflict rules, the allocation lifecycle, and the four-row publish × exposeToPlatform combination table (FR-018, SC-007).

**Independent Test**: From the docs alone, answer: how to publish on a fixed port, what happens to a dynamic port on redeploy, whether platform exposure is also required, and which conflicts stop a deploy.

### Implementation for User Story 5

- [ ] T027 [P] [US5] Add the `networking.publish` field to `docs/content/reference/manifest-schema.md`: both YAML forms with an example each, explicit range 1024–65535 excluding 30000–32767, the automatic range 30000–32767, the three conflict rules with fail-fast at dry-run and deploy, the allocation lifecycle (stable across redeploy/recreation/teardown; released only by `delete application` / `delete team`), and the four-row combination table copied **verbatim** from `specs/023-publish-host-ports/contracts/manifest-schema.md`
- [ ] T028 [P] [US5] Write the how-to guide `docs/content/guides/publish-localhost.md` (new, following the style of the existing guides in that directory): fixed vs automatic ports, discovering an automatic port from deploy output, port stability across redeploys, conflict behavior, and releasing ports via the delete commands — answering SC-007's four questions

**Checkpoint**: `make docs-serve` renders both pages under `/shrine/`; an operator can answer all four SC-007 questions from the pages alone.

---

## Phase 8: Polish & Final Gate

**Purpose**: Full verification and post-change housekeeping.

- [ ] T029 Run `go test ./...` across the repository and fix any regression (unit sweep — fast, run freely)
- [ ] T030 Run the full integration suite `make test-integration` (~10 min, real binary + real Docker) as the constitution's phase gate — all pre-existing suites plus `tests/integration/publish_test.go` must pass — depends on T029
- [ ] T031 Execute the manual round-trip in `specs/023-publish-host-ports/quickstart.md` end to end (explicit port, conflict, automatic allocation, stability, dry-run purity, release) — depends on T030
- [ ] T032 [P] Run `graphify update .` to refresh the knowledge graph after the code changes (AST-only, no API cost)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately; T001 must land before merge (governance)
- **Foundational (Phase 2)**: No dependency on Phase 1 — BLOCKS all user stories
- **US1 (Phase 3)**: Depends on Foundational — delivers the MVP
- **US2 (Phase 4)**: Depends on Foundational; layers onto US1's deploy path but is independently testable (conflicts fire from planning, before any engine work)
- **US3 (Phase 5)**: Depends on Foundational; T018 extends US1's `resolvePublishBinding` (T011), T020 extends US1's dry-run line (T012)
- **US4 (Phase 6)**: Mostly verification — the derivation ships in T003/T010; only T025 adds output
- **US5 (Phase 7)**: Content depends on decisions frozen in Foundational (ranges, semantics); can be written any time after Phase 2
- **Polish (Phase 8)**: After all desired stories

### Key Task Dependencies

- T003 → T004 (validation reads the new type)
- T005 → T006 → T007 (interface → impl → wiring)
- T003, T007, T008 → T010, T011 (projection and backend need type, store, ops)
- T014 → T015 → T016 (detector → Plan signature → handlers)
- T011 → T018 → T019 (explicit path → automatic path → event with resolved port)
- T021 → T022 (handler → CLI subcommand)
- T029 → T030 → T031 (unit sweep → integration gate → manual quickstart)
- Each story's integration-test task (T009, T013, T017, T024) precedes that story's implementation tasks (TDD)

### Parallel Opportunities

- Phase 1: T001 ∥ T002
- Phase 2: T005 ∥ T008 ∥ T003 (three independent files); T004 after T003; T006–T007 chain after T005
- US1: T010 ∥ T011 ∥ T012 after T009 (engine.go / docker_container.go / dry_run_container.go)
- US3: T018 ∥ T020 ∥ T021 ∥ T023 after T017 (four different files)
- US4: T025 ∥ T026 after T024
- US5: T027 ∥ T028 — and the whole phase can run in parallel with Phases 3–6 once Phase 2 is done
- Different developers can take different stories after Phase 2

---

## Parallel Example: User Story 1

```bash
# After T009 (integration scenarios written and failing):
Task: "T010 Project manifest → op (Publish + derived attachment) in internal/engine/engine.go"
Task: "T011 resolvePublishBinding explicit path + HostIP pass-through + conditional hash in internal/engine/local/dockercontainer/docker_container.go"
Task: "T012 Explicit publish print line in internal/engine/dryrun/dry_run_container.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 2: Foundational (T003–T008) — Phase 1 can land in the same PR at any point before merge
2. Phase 3: T009 tests first (fail) → T010–T012 in parallel → US1 scenarios green
3. **STOP and VALIDATE**: quickstart §1 by hand — `curl localhost:8080` and the `docker inspect` loopback check
4. This alone is a shippable increment: explicit ports, loopback-only, dry-run preview, non-publishing apps untouched

### Incremental Delivery

1. Foundational → US1 (MVP: explicit ports work end to end)
2. - US2 → explicit ports become safe (fail-fast conflict detection at plan time)
3. - US3 → automatic allocation with stability and the delete/release surface
4. - US4 → exposure-semantics guarantees locked by tests and visible in dry-run
5. - US5 → the behavior contract becomes public documentation
6. Each story leaves all previous stories' integration scenarios green

---

## Notes

- Unit tests never touch the filesystem — the store tests use injected file-op functions (project policy)
- The integration suite is the final gate, not an iteration loop (~10 min per run); use targeted `-run` filters while developing a story
- Commit after each task or logical group; the constitution amendment (T001) must be in the branch before merge
- Total: 32 tasks — Setup 2 · Foundational 6 · US1 4 · US2 4 · US3 7 · US4 3 · US5 2 · Polish 4
