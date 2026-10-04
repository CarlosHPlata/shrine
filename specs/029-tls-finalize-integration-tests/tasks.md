# Tasks: Integration Coverage for TLS Alias Routers and the Routing Finalize Phase

**Input**: Design documents from `/specs/029-tls-finalize-integration-tests/`
**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/operator-observables.md](contracts/operator-observables.md), [quickstart.md](quickstart.md)

**Tests**: The feature *is* tests. Every story task authors an integration scenario; there are no production-code tasks.

**Organization**: One phase per user story. All scenarios live in the same file, so story phases are sequential with respect to that file; only fixture tasks are parallel.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: User story from spec.md (US1–US5)

## Ground rules for every task

- **Do not modify** anything under `cmd/`, `internal/`, `main.go`, `Makefile`, or `.github/`.
- **Do not edit** any existing scenario, helper, `BeforeEach`, or `AfterEach` in `tests/integration/traefik_plugin_test.go`. Add only.
- **Do not run** the integration suite locally. Compile-check with `go vet -tags integration ./tests/integration/...`; CI executes.
- New scenarios go at the **end** of `TestTraefikPlugin` (after `should remove stale dashboard dynamic file when dashboard config is removed`, before the function's closing brace), in S1→S7 order.
- Every alias scenario (S1–S4) starts by registering the alias team, exactly as existing alias scenarios do:
  `tc.Run("apply", "teams", "--path", aliasFixturePath("<variant>"), "--state-dir", tc.StateDir).AssertSuccess()`
- Every scenario writes its config with `writeConfig(t, configDir, …)` using `configDir := tc.Path("config")` and `routingDir := tc.Path("traefik")`, and deploys with `--config-dir configDir --state-dir tc.StateDir --path <fixture>`.
- Comments only for WHY (Constitution VII), one line each.
- If a scenario's expectation contradicts shipped behaviour, stop and report (FR-016); do not weaken the assertion or change product code.

---

## Phase 1: Setup

**Purpose**: Confirm the starting point is clean so any later failure is attributable to this feature.

- [x] T001 On branch `029-tls-finalize-integration-tests`, run `go build ./...`, `go test ./...`, and `go vet -tags integration ./tests/integration/...` from the repo root and confirm all three pass before any edit

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Shared helpers used by US1, US2, and US3.

- [x] T002 In `tests/integration/traefik_plugin_test.go`, add `"strings"` to the import block and add three file-level helpers directly after `aliasFixturePath`: (a) `readDynamicRouters(tc *TestCase, path string) map[string]map[string]any` — reads the file with `os.ReadFile`, `yaml.Unmarshal`s into `map[string]any`, walks `http` → `routers`, and returns each router as `map[string]any`; calls `tc.Fatalf` with the file content on any read/parse/shape failure. (b) `assertPlainRouter(tc *TestCase, routers map[string]map[string]any, name string)` — fails via `tc.Fatalf` unless router `name` exists, its `entryPoints` is exactly `[web]`, and it has no `tls` key. (c) `assertTLSRouter(tc *TestCase, routers map[string]map[string]any, name string)` — fails unless router `name` exists, its `entryPoints` is exactly `[web, websecure]`, and it has a `tls` key whose value is an empty map (`yaml.v3` decodes `tls: {}` as `map[string]any{}`). Extract a private `routerEntryPoints(router map[string]any) []string` if needed to avoid duplicating the `[]any` → `[]string` conversion between (b) and (c)
- [x] T003 Run `go vet -tags integration ./tests/integration/...`; unused-helper warnings are not errors for file-level funcs, so this must pass before any scenario is added

**Checkpoint**: Helpers compile; no existing code changed.

---

## Phase 3: User Story 1 — `tls: true` on an alias produces an HTTPS-capable route (Priority: P1) 🎯 MVP

**Goal**: A real deploy of a single TLS alias yields a TLS alias router, a plain primary router, and the `(tls)` marker on stdout. Delivers 012 T016/T017.

**Independent Test**: CI sub-test `TestTraefikPlugin/should_publish_alias_router_with_tls_block_when_alias_sets_tls:_true` passes.

- [x] T004 [P] [US1] Create `tests/testdata/deploy/traefik-alias-tls/team.yaml` as an exact copy of `tests/testdata/deploy/traefik-alias-prefix/team.yaml`
- [x] T005 [P] [US1] Create `tests/testdata/deploy/traefik-alias-tls/app.yaml` modelled on `tests/testdata/deploy/traefik-alias-prefix/app.yaml`: `metadata.name: whoami-tls`, `metadata.owner: shrine-alias-test`, `spec.image: traefik/whoami`, `spec.port: 80`, `spec.replicas: 1`, `spec.networking.exposeToPlatform: true`, `spec.routing.domain: whoami-tls.shrine.lab`, `spec.routing.aliases: [{host: alias.shrine.lab, pathPrefix: /tls, stripPrefix: false, tls: true}]`
- [x] T006 [US1] Append scenario S1 `should publish alias router with tls block when alias sets tls: true` to `TestTraefikPlugin` in `tests/integration/traefik_plugin_test.go`: apply teams from `aliasFixturePath("tls")`; config `port: 8119`, `tlsPort: 8447`; deploy `aliasFixturePath("tls")` and `AssertSuccess`; `routers := readDynamicRouters(tc, filepath.Join(routingDir, "dynamic", "shrine-alias-test-whoami-tls.yml"))`; `assertTLSRouter(tc, routers, "shrine-alias-test-whoami-tls-alias-0")`; `assertPlainRouter(tc, routers, "shrine-alias-test-whoami-tls")`; `tc.AssertOutputContains("(tls)")`; `tc.AssertOutputNotContains("alias tls: true but websecure entrypoint missing")`
- [x] T007 [US1] Run `go vet -tags integration ./tests/integration/...` and confirm it passes

**Checkpoint**: US1 authored and compile-clean.

---

## Phase 4: User Story 2 — Per-alias TLS is independent and follows the manifest (Priority: P1)

**Goal**: Mixed aliases get distinct router shapes sharing one service; removing `tls: true` reverts the router once the per-app file is removed. Delivers 012 T023/T024, US2, SC-005.

**Independent Test**: CI sub-tests `…/should_give_only_the_opted-in_alias_a_tls_router_when_aliases_are_mixed` and `…/should_revert_alias_router_to_plain_when_tls_is_removed_and_re-deployed` pass.

- [x] T008 [P] [US2] Create `tests/testdata/deploy/traefik-alias-tls-mixed/team.yaml` (copy of `traefik-alias-prefix/team.yaml`) and `tests/testdata/deploy/traefik-alias-tls-mixed/app.yaml`: same shape as T005 with `metadata.name: whoami-tls-mixed`, `spec.routing.domain: whoami-tls-mixed.shrine.lab`, and two aliases in this order — `{host: lan.shrine.lab, pathPrefix: /lan}` then `{host: ext.shrine.lab, pathPrefix: /ext, stripPrefix: false, tls: true}`
- [x] T009 [P] [US2] Create `tests/testdata/deploy/traefik-alias-tls-removed/team.yaml` (copy of `traefik-alias-prefix/team.yaml`) and `tests/testdata/deploy/traefik-alias-tls-removed/app.yaml`: identical to `traefik-alias-tls/app.yaml` (same `metadata.name: whoami-tls`, same domain, same alias host/pathPrefix/stripPrefix) but with the `tls` line removed
- [x] T010 [US2] Append scenario S2 `should give only the opted-in alias a tls router when aliases are mixed` to `tests/integration/traefik_plugin_test.go`: apply teams from `aliasFixturePath("tls-mixed")`; config `port: 8120`, `tlsPort: 8448`; deploy and `AssertSuccess`; read routers from `dynamic/shrine-alias-test-whoami-tls-mixed.yml`; `assertPlainRouter` for `shrine-alias-test-whoami-tls-mixed` and `…-alias-0`; `assertTLSRouter` for `…-alias-1`; loop over the three router names and `tc.Fatalf` unless each `service` equals `"shrine-alias-test-whoami-tls-mixed"`
- [x] T011 [US2] Append scenario S3 `should revert alias router to plain when tls is removed and re-deployed` to `tests/integration/traefik_plugin_test.go`: apply teams from `aliasFixturePath("tls")`; config `port: 8121`, `tlsPort: 8449`; deploy `aliasFixturePath("tls")`, `AssertSuccess`, `assertTLSRouter` on `shrine-alias-test-whoami-tls-alias-0`; `os.Remove` the per-app file `dynamic/shrine-alias-test-whoami-tls.yml` with a one-line WHY comment (spec 009 preserve policy: a manifest change lands only when the file is absent) and `t.Fatalf` on error; deploy `aliasFixturePath("tls-removed")`, `AssertSuccess`; re-read routers; `assertPlainRouter` on `…-alias-0` and on the primary; `tc.AssertOutputNotContains("(tls)")`
- [x] T012 [US2] Run `go vet -tags integration ./tests/integration/...` and confirm it passes

**Checkpoint**: US1 + US2 authored; the two zero-coverage outcomes of spec 012 have scenarios.

---

## Phase 5: User Story 3 — Non-TLS alias deploys stay stable and quiet (Priority: P2)

**Goal**: A non-TLS manifest generates byte-identical routing across an unchanged redeploy and across a regenerate, with no TLS content or output. Delivers 012 T026.

**Independent Test**: CI sub-test `…/should_keep_non-tls_alias_routing_byte-stable_and_silent_about_tls` passes.

- [x] T013 [US3] Append scenario S4 `should keep non-tls alias routing byte-stable and silent about tls` to `tests/integration/traefik_plugin_test.go`: apply teams from `aliasFixturePath("prefix")`; config `port: 8122` with **no** `tlsPort`; define a local closure `deployAndRead := func() []byte` that deploys `aliasFixturePath("prefix")`, `AssertSuccess`, asserts `tc.AssertOutputNotContains("(tls)")` and `tc.AssertOutputNotContains("alias tls: true but websecure entrypoint missing")`, then returns `os.ReadFile` of `dynamic/shrine-alias-test-whoami-prefix.yml` (fatal on error); call it for `first`; assert `first` contains neither `websecure` nor `tls:` via `bytes.Contains`; call it again for `preserved` and require `bytes.Equal(first, preserved)`; `os.Remove` the file (one-line WHY comment: the preserved redeploy never rewrites, so regeneration is what proves deterministic output); call it again for `regenerated` and require `bytes.Equal(first, regenerated)`; failure messages print both byte slices
- [x] T014 [US3] Run `go vet -tags integration ./tests/integration/...` and confirm it passes

**Checkpoint**: All spec 012 deferred tasks have scenarios.

---

## Phase 6: User Story 4 — A finalize failure fails the deploy and is attributable (Priority: P2)

**Goal**: With the gateway image unpullable, the deploy exits non-zero, the per-app step is shown to have run, and the error is attributed to `routing.finalize` on stdout and in the log. Delivers 018 T012.

**Independent Test**: CI sub-test `…/should_exit_non-zero_and_attribute_the_error_to_routing_finalize_when_the_gateway_cannot_start` passes.

- [x] T015 [US4] Append scenario S5 `should exit non-zero and attribute the error to routing finalize when the gateway cannot start` to `tests/integration/traefik_plugin_test.go`: config with `routing-dir`, `port: 8123`, and `image: localhost:1/shrine-test/unpullable-gateway:0` under `plugins.gateway.traefik`, preceded by a one-line WHY comment (the gateway image is resolved only inside Finalize, and nothing listens on port 1, so app steps succeed and Finalize fails deterministically without network); deploy `traefikFixturePath()` and `AssertFailure()`; `tc.AssertOutputContains("Deploying Application: hello-eligible")`; `tc.AssertOutputContains("Error [routing.finalize]")`; `tc.AssertFileContains(filepath.Join(tc.StateDir, "logs", "shrine.log"), "[error] routing.finalize")`; `tc.AssertFileExists(filepath.Join(routingDir, "dynamic", traefikTestTeam+"-hello-eligible.yml"))`; `tc.AssertContainerRunning(traefikTestTeam + ".hello-eligible")`; `tc.AssertContainerNotExists(traefikContainerName)`
- [x] T016 [US4] Run `go vet -tags integration ./tests/integration/...` and confirm it passes

**Checkpoint**: Finalize failure path is pinned end-to-end.

---

## Phase 7: User Story 5 — Finalize appears in dry-run and produces the gateway (Priority: P3)

**Goal**: Dry-run previews the finalize operation after per-app route operations with no side effects; a real deploy leaves static config and the gateway container in place and records finalize in the log. Delivers 018 T014/T018/T019.

**Independent Test**: CI sub-tests `…/should_print_the_finalize_route_operation_last_among_route_operations_on_dry-run` and `…/should_leave_static_config_and_gateway_container_in_place_after_the_finalize_phase` pass.

- [x] T017 [US5] Append scenario S6 `should print the finalize route operation last among route operations on dry-run` to `tests/integration/traefik_plugin_test.go`: config `port: 8124`; run `deploy --dry-run` with `traefikFixturePath()` and `AssertSuccess`; `stdout := tc.RunResult().Stdout`; require `strings.Count(stdout, "[ROUTE]  Finalize") == 1` (two spaces after `[ROUTE]`); require `strings.LastIndex(stdout, "[ROUTE]  WriteRoute") >= 0` and that it is less than `strings.Index(stdout, "[ROUTE]  Finalize")`; `tc.Fatalf` with stdout on violation; then `tc.AssertContainerNotExists(traefikContainerName)`, `tc.AssertFileNotExists(filepath.Join(routingDir, "traefik.yml"))`, `tc.AssertFileNotExists(filepath.Join(routingDir, "dynamic"))`
- [x] T018 [US5] Append scenario S7 `should leave static config and gateway container in place after the finalize phase` to `tests/integration/traefik_plugin_test.go`: config `port: 8125`; deploy `traefikFixturePath()` and `AssertSuccess`; `tc.AssertFileExists(filepath.Join(routingDir, "traefik.yml"))`; `tc.AssertContainerRunning(traefikContainerName)`; `logFile := filepath.Join(tc.StateDir, "logs", "shrine.log")`; `tc.AssertFileContains(logFile, "[started] routing.finalize")`; `tc.AssertFileContains(logFile, "[info] routing.finalize")`
- [x] T019 [US5] Run `go vet -tags integration ./tests/integration/...` and confirm it passes

**Checkpoint**: All nine deferred tasks have scenarios.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Regression gates, traceability, and the CI gate.

- [x] T020 Run the local gates from `specs/029-tls-finalize-integration-tests/quickstart.md`: `go build ./...`, `go test ./...`, `go vet -tags integration ./tests/integration/...`, `gofmt -l tests/integration/` (must print nothing)
- [x] T021 Verify scope: `git diff --stat main -- . ':!tests' ':!specs' ':!graphify-out' ':!CLAUDE.md' ':!.specify'` prints nothing, and `git diff main -- tests/integration/traefik_plugin_test.go` shows only added lines (no `-` lines other than the diff header)
- [x] T022 Verify uniqueness in `tests/integration/traefik_plugin_test.go`: each of `port: 8119`…`port: 8125` and `tlsPort: 8447`…`tlsPort: 8449` appears exactly once, and each new scenario name appears exactly once
- [x] T023 [P] In `specs/012-tls-alias-routers/tasks.md`, change T016, T017, T023, T024, T026 from `[~]` to `[x]` and append to each `— delivered by specs/029-tls-finalize-integration-tests (S<n>)` using the mapping in `specs/029-tls-finalize-integration-tests/data-model.md`; on T016/T023 note the corrected fixture path `tests/testdata/deploy/`; on T024 note the preserve-policy correction
- [x] T024 [P] In `specs/018-routing-backend-finalize/tasks.md`, change T012, T014, T018, T019 from `[~]` to `[x]` and append `— delivered by specs/029-tls-finalize-integration-tests (S<n>)`; on T012 note the unpullable-image trigger; on T019 note the effects-based log assertion
- [x] T025 [P] In `specs/features/integration-tests.md`, add the three new fixtures and seven new `TestTraefikPlugin` scenarios, following the document's existing format for the Traefik suite
- [x] T026 [P] In `specs/progress.md`, add a `[x]` entry for 029 above the 028 entry, in the same style: what was deferred, what now covers it (S1–S7), the two findings (finalize success is log-only; preserve policy makes unchanged redeploys trivially identical), and the gate (`TestTraefikPlugin`, CI executes)
- [x] T027 Run `graphify update .` from the repo root
- [x] T030 Fix found by S1 in CI (maintainer decision: same PR, separate commit): in `internal/plugins/gateway/traefik/`, add `willHaveWebsecureEntrypoint` to `config_gen.go`, pass the plugin config to `emitAliasTLSNoWebsecureSignal` in `routing.go`, and add the two absent-static-config unit tests to `routing_test.go` (see plan.md Amendment)
- [ ] T028 Push the branch, open a PR referencing issue #38, and from the CI run confirm: the seven new sub-tests `--- PASS`; every pre-existing sub-test has the same result as on `main`; the `Integration Test` step took under 4 minutes (if not, raise `-timeout` in the Makefile `test-integration` target in a follow-up commit and say so in the PR)
- [ ] T029 Re-run the CI workflow twice more and confirm three consecutive green runs (SC-005)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (T001)** → **Foundational (T002–T003)** → story phases → **Polish (T020–T029)**.
- US1, US2, US3 depend on T002 (helpers). US4 and US5 do not use the helpers and depend only on T001.

### User Story Dependencies

- **US1 (P1)**: T002. Independent of other stories.
- **US2 (P1)**: T002; S3 (T011) deploys the `traefik-alias-tls` fixture created in US1 (T004, T005).
- **US3 (P2)**: T002 only for file placement; reuses the existing `traefik-alias-prefix` fixture.
- **US4 (P2)**, **US5 (P3)**: independent; reuse the existing `traefik` fixture.

### Within the shared test file

T006, T010, T011, T013, T015, T017, T018 all append to `tests/integration/traefik_plugin_test.go` and must be done one at a time, in that order, so the scenarios land S1→S7.

### Parallel Opportunities

- Fixture tasks T004, T005, T008, T009 touch different files and can all run together, before or alongside T002.
- Doc tasks T023–T026 touch different files and can run together.

## Parallel Example: fixtures

```text
Task: "Create tests/testdata/deploy/traefik-alias-tls/team.yaml"            (T004)
Task: "Create tests/testdata/deploy/traefik-alias-tls/app.yaml"             (T005)
Task: "Create tests/testdata/deploy/traefik-alias-tls-mixed/{team,app}.yaml"   (T008)
Task: "Create tests/testdata/deploy/traefik-alias-tls-removed/{team,app}.yaml" (T009)
```

## Implementation Strategy

### MVP First (User Story 1)

T001 → T002–T003 → T004–T007. One scenario that pins the headline behaviour of spec 012; shippable alone.

### Incremental Delivery

Each story phase ends with a compile check, so the branch can be pushed after any phase and CI will run whatever scenarios exist. Issue #38 asks for one PR, so the expected path is all phases, then Phase 8, then a single PR.

## Notes

- 29 tasks: Setup 1, Foundational 2, US1 4, US2 5, US3 2, US4 2, US5 3, Polish 10.
- Asserted strings and values come from [contracts/operator-observables.md](contracts/operator-observables.md); do not invent alternatives.
- The app container name in T015 follows the `<team>.<name>` convention (`shrine-traefik-test.hello-eligible`).
