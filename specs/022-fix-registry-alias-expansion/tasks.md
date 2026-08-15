# Tasks: Expand `reg:` Registry Aliases Before the Container Is Created

**Input**: Design documents from `/specs/022-fix-registry-alias-expansion/`
**Prerequisites**: plan.md, spec.md, research.md (D1–D6), data-model.md, contracts/deploy-diagnostics.md, quickstart.md

**Tests**: REQUIRED — the spec's Verification Requirements (VR-001–VR-005) bind this
task list. Tests for FR-001, FR-002, FR-008, and FR-009 MUST be run and **seen
red** against the unfixed code before their fix task begins (VR-003). Evidence
for FR-001–FR-006 MUST be a real (non-dry-run) deploy against the real Docker
daemon (VR-001). Every test asserts an operator-observable outcome (VR-002).
The Requirement→Test Traceability Mapping at the bottom of this file satisfies
VR-004 and is a blocking gate for `/speckit-implement`.

**Organization**: Tasks are grouped by user story. US1 and US2 are both P1 and
share a single production fix (research D1); the ordering constraint this
creates is marked with a ⚠️ VR-003 GATE and spelled out in Dependencies &
Execution Order.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1 (alias app deploys), US2 (alias resource deploys), US3 (error names image), US4 (no phantom log lines)

---

## Phase 1: Setup

**Purpose**: Establish a provably green baseline so every later red run (VR-003) is attributable to the defect, not to pre-existing breakage.

- [x] T001 Record the pre-change baseline on branch `022-fix-registry-alias-expansion`: run `go build ./...` and `go test ./...` from the repo root and confirm both pass with zero changes. Do not run the integration suite here (it is slow; it runs once as the final gate in T024).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The `dockerAPI` observation seam (research D2) and the shared integration assertion helper. Every red test in Phases 3–5 depends on one or both. Pure refactor — zero behaviour change, proven by T004.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T002 Create the package-internal `dockerAPI` interface in `internal/engine/local/dockercontainer/docker_api.go` with exactly the 12 methods the backend calls today, signatures matching `*client.Client` so it satisfies the interface structurally: `ContainerCreate`, `ContainerInspect`, `ContainerRemove`, `ContainerStart`, `ImageInspect`, `ImageList`, `ImagePull`, `NetworkCreate`, `NetworkInspect`, `NetworkRemove`, `VolumeCreate`, `VolumeInspect` (research D2). Unexported; add the one-line WHY comment that this is the test seam demanded by spec VR-001/VR-002.
- [x] T003 Retype the `client` field of `DockerBackend` from `*client.Client` to `dockerAPI` in `internal/engine/local/dockercontainer/docker_backend.go`. No other edits — `NewDockerBackend` and every call site must compile unchanged (depends on T002).
- [x] T004 Prove seam neutrality: `go build ./...` and `go test ./...` pass with zero test edits and zero behavioural diffs (depends on T003). This is the plan's Implementation Flow step 1 exit criterion.
- [x] T005 [P] Add `AssertContainerImage(containerName, expectedImage string) *TestCase` to `tests/integration/testutils/assert_docker.go`, modeled on the existing `AssertContainerEnvVar` method: `ContainerInspect` the named container and require `Config.Image == expectedImage`. This reads back the exact string Docker was handed at create time (research D4).

**Checkpoint**: Seam in place, behaviour provably unchanged, assertion helper available. Red-test writing can begin in parallel across all four stories.

---

## Phase 3: User Story 1 — A new workload deploys using the `reg:` alias form (Priority: P1) 🎯 MVP

**Goal**: `shrine deploy` of an `Application` with `image: reg:<alias>/<path>:<tag>` and no existing container creates the container with the expanded reference — the bug in issue #33.

**Independent Test** (spec, restated in full per VR-005): Declare a registry alias in config, write an `Application` manifest using `image: reg:<alias>/<path>:<tag>` for a workload that has no existing container, and run `shrine deploy` — the real command, not `--dry-run`. Verify the deploy succeeds and the created container's image reference is the fully-qualified one. Compare against the same manifest with the host written out in full — both must produce the same result.

### Red tests for User Story 1 (write, run, and record the red output BEFORE T011)

- [x] T006 [P] [US1] Create `internal/engine/local/dockercontainer/docker_container_test.go` (package `dockercontainer`) with a fake `dockerAPI` and three test functions (research D3 — no filesystem, no daemon, `nil` state store, no-op observer):
  - Fake behaviour: `ImageInspect` returns a fixed digest ID (so the pull path is skipped), `ContainerInspect` returns `errdefs.ErrNotFound` (forcing the fresh-create path), `ContainerCreate` **captures the `*container.Config` and returns an error** (stopping the flow before `recordDeployment`, so no state file is ever written). The 9 unused methods may panic to prove they are unreached.
  - `TestCreateContainer_ExpandsAliasIntoContainerSpec`: registries `[{host: docker.io, alias: myregistry}]`, `op.Image = "reg:myregistry/traefik/whoami:latest"` → captured `Config.Image` MUST equal `"docker.io/traefik/whoami:latest"` and MUST NOT have the `reg:` prefix (FR-001, SC-003). **Expected RED**: capture shows the raw `reg:` string.
  - `TestCreateContainer_BareAliasExpandsToHost`: `op.Image = "reg:myregistry"` → captured `Config.Image` equals `"docker.io"` (edge case: alias with no path segment, consistent with plan-time validation).
  - `TestCreateContainer_PlainReferencePassesThroughUnchanged`: `op.Image = "nginx:latest"` → captured `Config.Image` equals `"nginx:latest"` (FR-005). Expected green today — a pin, not a red-first test.
  - Record the red run output (VR-003 evidence for T025).
- [x] T007 [P] [US1] Add `TestRegistryAliasRealDeploy_Application` to `tests/integration/registry_alias_test.go` (depends on T005): using `NewDockerSuite(t, "shrine-deploy-test")` and `testutils.Execute`, run a **real** `shrine deploy` (no `--dry-run`) of the existing fixture `tests/testdata/deploy/registry-alias/` (app `alias-app`, image `reg:myregistry/traefik/whoami:latest`). Assert: exit success, `AssertContainerRunning("shrine-deploy-test.alias-app")`, `AssertContainerImage("shrine-deploy-test.alias-app", "docker.io/traefik/whoami:latest")` (FR-001, SC-001, SC-003). Then run the same deploy a second time unchanged and assert the container ID is identical — the existing up-to-date container takes the early-return path (FR-011, the masking edge case, covered explicitly rather than assumed). Run with `go test -tags integration -run TestRegistryAliasRealDeploy_Application ./tests/integration/...`. **Expected RED**: the first deploy fails with `invalid reference format`. Record the red output. Leave the three existing dry-run tests (`TestRegistryAliasConfig`, `TestRegistryAliasAppImage`, `TestRegistryAliasResourceImage`) byte-for-byte unmodified (FR-007/SC-005).
- [x] T008 [P] [US1] Create the form-equivalence fixtures (research D4): `tests/testdata/deploy/registry-alias-eq-alias/` and `tests/testdata/deploy/registry-alias-eq-full/` — identical `config.yml` (`host: docker.io, alias: myregistry`) and identical `Application` manifests (same name, e.g. `alias-eq`, owner `shrine-deploy-test`, port 80, `exposeToPlatform: false`) differing ONLY in the image line: `reg:myregistry/traefik/whoami:latest` vs `docker.io/traefik/whoami:latest`.
- [x] T009 [US1] Add `TestRegistryAliasFormEquivalence_NoRecreate` to `tests/integration/registry_alias_test.go` (depends on T005, T008): real-deploy `registry-alias-eq-alias/` first and assert the container's image is `docker.io/traefik/whoami:latest`, capturing the container ID; then real-deploy `registry-alias-eq-full/` and assert the container ID is **unchanged** — the form swap alone never recreates (FR-004, FR-010, SC-004, SC-008; pins the digest-based hash contract, research D6). **Expected RED on the alias leg** against unfixed code (`invalid reference format`); the no-recreate property itself needs no red run (FR-010 is not in VR-003's list — D6: verification-only).
- [x] T010 [P] [US1] Add `TestRegistryAliasUnknown_RealDeployFailsBeforeCreate` to `tests/integration/registry_alias_test.go`: real (non-dry-run) deploy of the existing fixture `tests/testdata/deploy/registry-alias-unknown/`; assert exit failure, the error names the unknown alias, and no container was created for the workload (FR-006 under VR-001 — the existing unknown-alias coverage is dry-run/plan-level only). Expected green today (plan-time validation already works); this is a pin required by VR-001, not a red-first test — note this in the test's comment is NOT needed, note it only in the PR evidence.

> **⚠️ VR-003 GATE — do not start T011 yet.** The fix below also resolves User Story 2 (same code path, research D1). T014 (US2's red integration test) and T015 (mixed-manifest red test) MUST be written and **seen red first**, or US2's regression tests will never have a recorded red run. Execution order: T006, T007, T009, T010, T014, T015 all red/recorded → then T011.

### Implementation for User Story 1 (and, by shared code path, User Story 2)

- [x] T011 [US1] In `internal/engine/local/dockercontainer/docker_container.go`, expand the alias exactly once at the top of `DockerBackend.CreateContainer`, before anything reads `op.Image`: `op.Image = expandRegistryAlias(op.Image, backend.registries)` result assigned into the by-value `op`; on expansion error, fail the creation through the existing error pathway (wrapped error + `container.create` error event) before any Docker call (research D1; FR-001, FR-002, FR-003, FR-006). Blocked by: T004 and recorded red runs of T006, T007, T009, T014, T015.
- [x] T012 [US1] In `internal/engine/local/dockercontainer/docker_image.go`, remove the now-redundant expansion inside `resolveImage` and add the one-line WHY comment stating the caller owns alias expansion (the hidden invariant — Constitution VII's permitted comment kind). Signature unchanged (research D1; enforces the single-expansion-point invariant, data-model.md). Depends on T011 — land as one change unit.
- [x] T013 [US1] Green verification for US1: `go test ./...` passes (T006's red tests now green, plain-ref pin still green); targeted `go test -tags integration -run 'TestRegistryAliasRealDeploy_Application|TestRegistryAliasFormEquivalence_NoRecreate|TestRegistryAliasUnknown_RealDeployFailsBeforeCreate' ./tests/integration/...` passes. Do not run the full integration suite here (final gate is T024).

**Checkpoint (VR-005 — full restatement, no narrowing)**: Given a config declaring an alias for a registry host and an `Application` manifest using `image: reg:<alias>/<path>:<tag>` for a workload with no existing container, running **`shrine deploy`** (not `--dry-run`) creates the container successfully and its image reference is the alias expanded to the configured registry host; deploying the equivalent fully-qualified manifest produces the same observable result, and swapping a running workload's manifest between the two forms recreates nothing.

---

## Phase 4: User Story 2 — Resources using the alias form deploy too (Priority: P1)

**Goal**: `Resource` containers reach the runtime through the same creation path; alias-form resources deploy exactly like alias-form applications.

**Independent Test** (spec, restated in full per VR-005): Write a `Resource` manifest with `image: reg:<alias>/<path>:<tag>` for a resource with no existing container, deploy it with **`shrine deploy`** (not `--dry-run`), and verify the container is created with the expanded reference.

### Red tests for User Story 2 (⚠️ executed BEFORE T011 — see the VR-003 GATE in Phase 3)

- [x] T014 [P] [US2] Add `TestRegistryAliasRealDeploy_Resource` to `tests/integration/registry_alias_test.go` (depends on T005): real (non-dry-run) deploy of the existing fixture `tests/testdata/deploy/registry-alias-resource/` (resource `alias-db`, image `reg:myregistry/postgres:15`). Assert: exit success, `AssertContainerRunning("shrine-deploy-test.alias-db")`, `AssertContainerImage("shrine-deploy-test.alias-db", "docker.io/postgres:15")` (FR-002, SC-002). **Expected RED**: `invalid reference format`. Record the red output.
- [x] T015 [P] [US2] Create fixture `tests/testdata/deploy/registry-alias-mixed/` — the same `config.yml` (`docker.io` / `myregistry`) plus BOTH an alias-form `Application` (reuse the `alias-app` manifest shape, distinct name e.g. `alias-mixed-app`) and an alias-form `Resource` (reuse the `alias-db` shape, distinct name e.g. `alias-mixed-db`), owner `shrine-deploy-test` — and add `TestRegistryAliasRealDeploy_Mixed` to `tests/integration/registry_alias_test.go`: one real deploy of the whole set; assert both containers are created and neither `Config.Image` begins with `reg:` (US2 acceptance scenario 2). Existing shared fixtures are NOT modified — the dry-run tests that grep them must pass unmodified. **Expected RED**. Record the red output.
- [ ] T016 [US2] Green verification for US2 (after T011/T012): targeted `go test -tags integration -run 'TestRegistryAliasRealDeploy_Resource|TestRegistryAliasRealDeploy_Mixed' ./tests/integration/...` passes.

**Checkpoint (VR-005)**: Given a config declaring an alias and a `Resource` manifest using the alias image form for a resource with no existing container, running **`shrine deploy`** creates the container successfully with the expanded image reference; a manifest set containing both an alias-form `Application` and an alias-form `Resource` deploys with neither carrying an unexpanded `reg:` reference into the container runtime.

---

## Phase 5: User Story 3 — A rejected image reference is named in the error (Priority: P2)

**Goal**: Every container-creation failure names the image reference Docker rejected, alongside the container name, per contracts/deploy-diagnostics.md §2.

**Independent Test** (spec): Trigger a container-creation failure with a deliberately malformed image reference and verify the error output contains that reference.

### Red test for User Story 3

- [x] T017 [P] [US3] Add `TestCreateContainer_CreateFailureNamesImage` to `internal/engine/local/dockercontainer/docker_container_test.go`: same fake as T006 (`ContainerCreate` returns an error) but with a **capturing observer** recording emitted events. Deploy an alias-form op and assert on the `container.create` error event per contracts/deploy-diagnostics.md §2: `Fields["image"]` equals the **expanded** reference, `Fields["name"]` keeps the `<team>.<resource>` shape, and `Fields["error"]` matches `creating container "<name>" (image "<expanded ref>"): <cause>` — the string the ❌ terminal line prints verbatim, hence operator-observable (FR-008, SC-006, VR-002). **Expected RED**: today's message and fields carry no image. Record the red output. (VR-001 does not bind FR-008 — unit-level evidence is admissible.)

### Implementation for User Story 3

- [x] T018 [US3] In `createFreshContainer` in `internal/engine/local/dockercontainer/docker_container.go`, enrich the creation failure per research D5: wrapped error becomes `creating container %q (image %q): %w` and the error event's fields gain `"image": op.Image` (already the expanded form at that point). Blocked by T017's red run; T011 must have landed (the "expanded form" claim depends on it).
- [x] T019 [US3] Green verification for US3: `go test ./...` passes, T017 green.

**Checkpoint (VR-005)**: Given a manifest whose image reference the container runtime rejects for any reason, when the deploy fails at container creation, the error output names the rejected image reference alongside the container name — an operator can identify the offending manifest field from the deploy output alone.

---

## Phase 6: User Story 4 — The deploy log does not invent container names (Priority: P3)

**Goal**: Exactly one `🏗️  Creating container` line per creation attempt, no malformed names, successful-deploy output byte-identical to today (contracts/deploy-diagnostics.md §3).

**Independent Test** (spec): Trigger a container-creation failure and count the "creating container" lines in the output: exactly one per creation attempt, none with a malformed name.

### Red test for User Story 4

- [x] T020 [P] [US4] Create `internal/ui/terminal_logger_test.go` with `TestTerminalObserver_ContainerCreate`: construct `NewTerminalObserver(&bytes.Buffer{})` and feed it the real event sequence one failed creation produces (derive the exact statuses/fields from what `internal/engine/engine.go` and `docker_container.go`'s error emit actually send — the engine's informational progress event with `team`+`name` fields, the backend's error event whose only identity field is the full container `name`, and the engine's error re-emit). Assert: exactly one line containing `Creating container:` and it renders `<team>.<name>` with both segments; no output line contains the leading-dot artifact (`: .`); the `❌ Error [container.create]` line(s) are preserved (FR-009, SC-007). Add a success-sequence case asserting the output is exactly today's single progress line (US4 acceptance scenario 2). **Expected RED**: three `Creating container` lines, one malformed. Record the red output.

### Implementation for User Story 4

- [x] T021 [US4] In `internal/ui/terminal_logger.go`, guard the `container.create` case so the `🏗️  Creating container` progress line renders only for the engine's informational status (per research D5 / contract §3 — never as a side effect of rendering an error event). Blocked by T020's red run.
- [x] T022 [US4] Green verification for US4: `go test ./...` passes, T020 green.

**Checkpoint (VR-005)**: Given a deploy in which container creation fails, the output shows the "creating container" line exactly once for that container with both name segments present; given a deploy in which creation succeeds, the progress lines are unchanged from today's successful-deploy output.

---

## Phase 7: Polish & Final Gates

**Purpose**: The full-suite gates (run once — the integration suite is slow, ~10 min, project rule), the VR-003/SC-011 evidence ledger, and project hygiene.

- [x] T023 Full unit gate: `go test ./...` green across the repo.
- [ ] T024 Full integration gate — the Constitution Principle V gate, run ONCE as the final check: `make test-integration`. Confirms all new real-deploy tests green AND the pre-existing dry-run assertions (`TestRegistryAliasConfig`, `TestRegistryAliasAppImage`, `TestRegistryAliasResourceImage`) pass **unmodified** (FR-007, SC-005), plus zero regressions across the rest of the suite (SC-009).
- [x] T025 [P] Compile the SC-011 red-run ledger: collect the recorded red outputs from T006, T007, T009 (alias leg), T014, T015, T017, T020 into the PR description (or commit messages), one entry per test with the failing output excerpt. The count of never-seen-red regression tests introduced by this feature MUST be zero.
- [ ] T026 [P] Execute the manual verification in `specs/022-fix-registry-alias-expansion/quickstart.md`: real deploy of an alias manifest, `docker inspect --format '{{.Config.Image}}'` shows the expanded reference; dry-run still shows the alias form.
- [x] T027 Run `graphify update .` to refresh the knowledge graph after the code changes (project rule in CLAUDE.md).

---

## Requirement→Test Traceability Mapping (VR-004 — blocking gate)

Every functional requirement and acceptance scenario maps to at least one automated test. **Unmapped requirements: none — the gate to `/speckit-implement` is open.** No requirement describing live-execution behaviour is evidenced by dry-run (SC-010).

| Requirement | Evidence (test) | Task | Kind |
|---|---|---|---|
| FR-001 | `TestRegistryAliasRealDeploy_Application`; `TestCreateContainer_ExpandsAliasIntoContainerSpec` | T007, T006 | real deploy (VR-001) + unit |
| FR-002 | `TestRegistryAliasRealDeploy_Resource`; `TestRegistryAliasRealDeploy_Mixed` | T014, T015 | real deploy (VR-001) |
| FR-003 | `TestRegistryAliasRealDeploy_Application` (pull, auth, create all succeeded against one expanded ref, image asserted); `TestCreateContainer_ExpandsAliasIntoContainerSpec` | T007, T006 | real deploy (VR-001) + unit |
| FR-004 | `TestRegistryAliasFormEquivalence_NoRecreate` | T009 | real deploy (VR-001) |
| FR-005 | `TestCreateContainer_PlainReferencePassesThroughUnchanged`; existing plain-ref real deploys (`deploy_test.go`, `deploy_team_test.go`, …) re-run in T024 | T006, T024 | unit + real deploy (VR-001) |
| FR-006 | `TestRegistryAliasUnknown_RealDeployFailsBeforeCreate`; existing plan-time rejection tests, unmodified | T010, T024 | real deploy (VR-001) |
| FR-007 | Existing `TestRegistryAliasAppImage` + `TestRegistryAliasResourceImage` dry-run assertions, byte-for-byte unmodified | T024 | integration (dry-run is the requirement here) |
| FR-008 | `TestCreateContainer_CreateFailureNamesImage` (asserts the exact string the ❌ line prints) | T017 | unit (VR-001 does not bind FR-008) |
| FR-009 | `TestTerminalObserver_ContainerCreate` | T020 | renderer output |
| FR-010 | `TestRegistryAliasFormEquivalence_NoRecreate` (container ID unchanged across form swap) | T009 | real deploy |
| FR-011 | `TestRegistryAliasRealDeploy_Application` redeploy-idempotence step; existing up-to-date-container suite behaviour | T007, T024 | real deploy |

| Acceptance scenario | Evidence (test) | Task |
|---|---|---|
| US1-AS1 (fresh alias deploy succeeds, expanded image) | `TestRegistryAliasRealDeploy_Application` | T007 |
| US1-AS2 (alias vs fully-qualified: same result) | `TestRegistryAliasFormEquivalence_NoRecreate` | T009 |
| US1-AS3 (plain ref unchanged) | `TestCreateContainer_PlainReferencePassesThroughUnchanged` + existing suite | T006, T024 |
| US1-AS4 (pull/credentials/spec agree on one ref) | `TestRegistryAliasRealDeploy_Application` + `TestCreateContainer_ExpandsAliasIntoContainerSpec` | T007, T006 |
| US2-AS1 (fresh alias resource deploy) | `TestRegistryAliasRealDeploy_Resource` | T014 |
| US2-AS2 (app+resource set, neither carries `reg:`) | `TestRegistryAliasRealDeploy_Mixed` | T015 |
| US3-AS1 (error names rejected ref + container name) | `TestCreateContainer_CreateFailureNamesImage` | T017 |
| US3-AS2 (offending field identifiable from output alone) | `TestCreateContainer_CreateFailureNamesImage` (message contains manifest's resolved image field) | T017 |
| US4-AS1 (one creating-line, no malformed names) | `TestTerminalObserver_ContainerCreate` failure case | T020 |
| US4-AS2 (success output unchanged) | `TestTerminalObserver_ContainerCreate` success case | T020 |
| Edge: unknown alias fails pre-create | `TestRegistryAliasUnknown_RealDeployFailsBeforeCreate` | T010 |
| Edge: empty alias rejected | existing `registry-alias-badformat` tests, unmodified | T024 |
| Edge: bare alias (`reg:<alias>`) consistent | `TestCreateContainer_BareAliasExpandsToHost` | T006 |
| Edge: form-swap edit never recreates | `TestRegistryAliasFormEquivalence_NoRecreate` | T009 |
| Edge: existing up-to-date container unaffected | T007 redeploy step + T009 second leg | T007, T009 |
| Edge: dry-run shows alias form | existing dry-run tests, unmodified | T024 |
| Edge: plain/fully-qualified refs untouched | `TestCreateContainer_PlainReferencePassesThroughUnchanged` + full suite | T006, T024 |

**Success-criteria coverage**: SC-001→T007 · SC-002→T014 · SC-003→T006+T007/T014 (`AssertContainerImage`) · SC-004→T009 · SC-005→T024 · SC-006→T017 · SC-007→T020 · SC-008→T009 · SC-009→T024 · SC-010→this mapping (complete) · SC-011→T025 ledger.

**Red-first roster (VR-003)**: FR-001→T006, T007 · FR-002→T014, T015 · FR-008→T017 · FR-009→T020. Each MUST have a recorded red run before its fix task (T011, T018, T021 respectively); T025 audits this.

---

## Dependencies & Execution Order

### Phase dependencies

- **Setup (Phase 1)**: none.
- **Foundational (Phase 2)**: after T001. T005 is independent of T002–T004 and can run in parallel with them. **Blocks all stories.**
- **US1/US2/US3/US4 red tests**: after Phase 2; all parallelizable with each other.
- **Fix tasks**: T011 requires red runs of T006, T007, T009, T014, T015 (the ⚠️ VR-003 GATE — US2's tests run before US1's fix because the fix is shared). T018 requires T017 red AND T011 landed. T021 requires only T020 red.
- **Polish (Phase 7)**: after all stories green.

### True execution order (differs from task-ID order at one point, by design)

```text
T001 → T002 → T003 → T004
        T005 (parallel with T002–T004)
→ T006, T007, T008→T009, T010, T014, T015, T017, T020   (all red tests, parallel)
→ T011 → T012 → T013, T016                               (shared P1 fix, then both P1 stories verify green)
→ T018 → T019                                            (US3, any time after T011)
→ T021 → T022                                            (US4, independent of T011/T018)
→ T023 → T024 → T025, T026, T027
```

### Parallel opportunities

```bash
# After Phase 2, launch every red test together — five files, no shared edits
# except registry_alias_test.go (T007, T009, T010, T014, T015 append to it —
# sequence those five or write them as one editing pass):
Task: "T006 unit red tests in docker_container_test.go"
Task: "T007+T009+T010+T014+T015 integration red tests in registry_alias_test.go (one editing pass; T008 fixtures first)"
Task: "T017 error-contract red test in docker_container_test.go (after T006 lands the fake)"
Task: "T020 renderer red test in internal/ui/terminal_logger_test.go"
```

---

## Implementation Strategy

- **MVP scope**: Phases 1–4 (US1 **and** US2). The two P1 stories share one production fix, so the MVP boundary naturally includes both; shipping US1 "alone" would leave US2 fixed but unevidenced, which VR-004 forbids.
- **Incremental delivery**: MVP (fix + both P1 stories verified) → US3 (error enrichment) → US4 (renderer guard) → final gates. US3 and US4 are independent of each other and can land in either order.
- **Suite discipline** (project rule): iterate with `go test ./...` and *targeted* `-run` integration invocations; the full `make test-integration` runs exactly once, as T024.

---

## Execution Record (2026-08-15)

**Directive applied**: per the operator's instruction at implementation time,
integration tests were NOT executed locally — the branch is pushed and the
remote pipeline runs them. Consequences, recorded honestly:

- **Open tasks**: T016 and T024 (integration green runs) are delegated to the
  remote pipeline and stay unchecked until it reports. T026 (quickstart manual
  verification) needs a live host and is deferred to the same window.
- **VR-003 evidence, unit level (executed locally, then seen green)**:
  `TestCreateContainer_ExpandsAliasIntoContainerSpec` (captured spec image was
  `reg:myregistry/traefik/whoami:latest`), `TestCreateContainer_BareAliasExpandsToHost`,
  `TestCreateContainer_CreateFailureNamesImage` (no image field, message
  omitted the reference), `TestTerminalObserver_ContainerCreate` (three
  creating-lines, one `.shrine-deploy-test.alias-app`). Full red output is in
  commit `b684ed2`'s message.
- **VR-003 evidence, integration level**: the five real-deploy tests were
  committed (compiling under `-tags integration`) in `b684ed2`, one commit
  BEFORE the fix — checking out that commit reproduces the red run
  (`invalid reference format`). Red-first is preserved by commit ordering
  rather than a locally executed run.
- **T014 deviation**: the real resource deploy uses a new
  `registry-alias-resource-live/` fixture (whoami-backed `cache` resource)
  instead of the existing postgres fixture — `postgres:15` exits immediately
  without env (nothing provisions it), so `AssertContainerRunning` would flake
  and CI would pull a ~100MB image for nothing. The postgres fixture remains
  untouched as the dry-run contract (FR-007/SC-005).

## Notes

- Every red test's failing output must be captured when first run — T025 cannot reconstruct it after the fix lands.
- T007/T009/T010/T014/T015 all edit `tests/integration/registry_alias_test.go`; they are marked [P] against tasks in *other* files only — sequence them among themselves.
- The three pre-existing dry-run tests are contract, not collateral: any diff to them fails FR-007/SC-005 review.
- Commit after each task or logical group; the ⚠️ VR-003 GATE is the one place where committing "in task order" would destroy evidence — respect the true execution order above.
