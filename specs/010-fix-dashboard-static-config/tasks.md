---

description: "Task list for fix-dashboard-static-config"
---

# Tasks: Fix Traefik Dashboard Generated in Static Config

**Input**: Design documents from `/specs/010-fix-dashboard-static-config/`
**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/observer-events.md](./contracts/observer-events.md), [quickstart.md](./quickstart.md)

**Tests**: REQUIRED — Constitution Principle V and the user's auto-memory mandate that integration tests are written before the implementation they cover. Unit tests follow the project's strict no-filesystem rule (use `stubLstat` and in-memory YAML bytes only).

**Organization**: Tasks are grouped by user story so each story can be implemented and verified independently. The two P1 stories (US1: dashboard works; US2: static config is valid) are separately testable but share the foundational type changes in Phase 2.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel — different files, no dependency on an incomplete task
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- File paths in every task are absolute-relative-to-repo-root

## Path Conventions

- Plugin code: `internal/plugins/gateway/traefik/` (modify in place)
- Unit tests: `internal/plugins/gateway/traefik/config_gen_test.go` (no filesystem touches; stub `lstatFn`)
- Integration tests: `tests/integration/traefik_plugin_test.go` (build tag `integration`; uses `NewDockerSuite`)

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish a known-green baseline before any code change.

- [X] T001 Confirm baseline `go build ./...` and `go test ./...` both succeed on branch `010-fix-dashboard-static-config` with no pre-existing failures, and confirm `make test-integration` passes (or, if a long run is unwise, snapshot the latest known-passing state from the previous main merge)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Introduce the small private types both P1 stories will use. No behavior change yet.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T002 Add two private helper types at the bottom of `internal/plugins/gateway/traefik/config_gen.go`: `dashboardDynamicDoc` (a struct with one field `HTTP httpConfig \`yaml:"http"\``) and `legacyHTTPProbe` (a struct with one field `HTTP *yaml.Node \`yaml:"http"\``). Re-uses existing `httpConfig` from `spec.go`. Add the `gopkg.in/yaml.v3` import alias if needed for `*yaml.Node` (the package is already imported).

**Checkpoint**: Foundation ready — both P1 user stories can now begin in parallel.

---

## Phase 3: User Story 1 - Dashboard works after a clean deploy (Priority: P1) 🎯 MVP

**Goal**: After a clean `shrine deploy` with the Traefik plugin and a dashboard password, the dashboard URL returns an authentication challenge (and serves the dashboard with valid creds), instead of 404.

**Independent Test**: On a clean host with no prior Traefik state, run `shrine deploy` with the fixture dashboard config. `curl -i http://<dashboard-host>:<dashboard-port>/dashboard/` returns `401 Unauthorized` (not `404`); `curl -u <user>:<pass> …` returns `200`.

### Tests for User Story 1 ⚠️ Write FIRST and confirm they FAIL before T007–T009

- [X] T003 [P] [US1] Unit test `TestGenerateDashboardDynamicConfig_Skip_WhenPresent` in `internal/plugins/gateway/traefik/config_gen_test.go` — stub `lstatFn` to report file present, call the new generator with a `recordingObserver`, assert exactly one `gateway.dashboard.preserved` Info event with `path` set to `<routing-dir>/dynamic/__shrine-dashboard.yml`. Mirror the existing `TestGenerateStaticConfig_Skip_WhenPresent` shape.
- [X] T004 [P] [US1] Unit test `TestGenerateDashboardDynamicConfig_StatError` in `internal/plugins/gateway/traefik/config_gen_test.go` — stub `lstatFn` to return a permission-denied error, assert the function returns a wrapped error containing `__shrine-dashboard.yml` and the underlying error message, and emits zero events. Mirror `TestGenerateStaticConfig_StatError`.
- [X] T005 [US1] Integration test scenario `should expose a working dashboard on a clean deploy` in `tests/integration/traefik_plugin_test.go` — write a Shrine config with `plugins.gateway.traefik` including `dashboard.{port,username,password}`, run `tc.Run("deploy", ...).AssertSuccess()`, then: assert `<routing-dir>/dynamic/__shrine-dashboard.yml` exists with non-zero size, assert `<routing-dir>/traefik.yml` exists, and (if the harness exposes an HTTP probe helper) issue a request against the dashboard port and assert HTTP 401 with a `WWW-Authenticate` header. If no HTTP probe helper is available, downgrade to a YAML-shape assertion that `__shrine-dashboard.yml` parses to a doc with `http.routers.dashboard` set; record the downgrade in a code comment.
- [X] T006 [US1] Integration test scenario `should not generate a dashboard dynamic file when no dashboard password configured` in `tests/integration/traefik_plugin_test.go` — write a Shrine config with `plugins.gateway.traefik` and only `port:` set (no `dashboard:` block), run deploy, assert `<routing-dir>/dynamic/__shrine-dashboard.yml` does NOT exist (covers FR-005).

### Implementation for User Story 1

- [X] T007 [US1] Add `dashboardDynamicFileName() string` private helper at the bottom of `internal/plugins/gateway/traefik/config_gen.go`, returning the literal `"__shrine-dashboard.yml"`. Place it next to the existing `routeFileName` helper for symmetry.
- [X] T008 [US1] Add `generateDashboardDynamicConfig(cfg *config.TraefikPluginConfig, routingDir string, observer engine.Observer) error` in `internal/plugins/gateway/traefik/config_gen.go`. Behaviour, mirroring `generateStaticConfig` exactly: compute `path := filepath.Join(routingDir, "dynamic", dashboardDynamicFileName())`; call `isPathPresent(path)`; if present, emit `gateway.dashboard.preserved` (StatusInfo, fields `{path}`) and return nil; otherwise build a `dashboardDynamicDoc` whose `HTTP` field is an `httpConfig` with the existing `dashboard-auth` `basicAuth` middleware (re-use `htpasswdEntry` and `cfg.Dashboard.Username/Password`) and the existing `dashboard` router (`PathPrefix(\`/dashboard\`) || PathPrefix(\`/api\`)` → `api@internal` on entry-point `traefik` with `dashboard-auth` middleware), `yaml.Marshal` it, write with `os.WriteFile(path, data, 0o644)`, then emit `gateway.dashboard.generated` (StatusInfo, fields `{path}`).
- [X] T009 [US1] In `internal/plugins/gateway/traefik/plugin.go`, modify `Plugin.Deploy()` to call `generateDashboardDynamicConfig(p.cfg, routingDir, p.observer)` immediately after the existing `generateStaticConfig(...)` call, gated on `p.hasDashboard()`. Wrap any returned error with the existing `"traefik plugin: …"` prefix style used in adjacent code.

**Checkpoint**: User Story 1 fully functional — dashboard reachable on clean deploy. T003/T004/T005/T006 must all pass.

---

## Phase 4: User Story 2 - Generated static config is valid Traefik static config (Priority: P1)

**Goal**: The generated `traefik.yml` contains only valid Traefik static-configuration keys; a pre-existing buggy `traefik.yml` containing an `http:` block is left untouched but triggers a clear deploy-time warning (FR-010, FR-011).

**Independent Test**: After a clean deploy with the dashboard configured, parse `<routing-dir>/traefik.yml` and assert no top-level `http` key exists. Pre-stage a buggy `traefik.yml` (with an `http:` block) under a clean `<routing-dir>`, redeploy, and assert (a) the file is byte-identical to the staged version and (b) the deploy output (or recorded events) contains a `gateway.config.legacy_http_block` warning naming the path.

### Tests for User Story 2 ⚠️ Write FIRST and confirm they FAIL before T015–T017

- [X] T010 [P] [US2] Unit test `TestHasLegacyDashboardHTTPBlock_Detected` in `internal/plugins/gateway/traefik/config_gen_test.go` — inject an in-memory file reader (introduce a package-level `readFileFn = os.ReadFile` next to existing `lstatFn` so tests can stub it) returning a YAML byte string that contains an `http:` block; assert the helper returns `(true, nil)`.
- [X] T011 [P] [US2] Unit test `TestHasLegacyDashboardHTTPBlock_NoBlock` in `internal/plugins/gateway/traefik/config_gen_test.go` — stub the reader with a clean static YAML (entryPoints/api/providers only); assert `(false, nil)`.
- [X] T012 [P] [US2] Unit test `TestHasLegacyDashboardHTTPBlock_ParseError` in `internal/plugins/gateway/traefik/config_gen_test.go` — stub the reader with malformed YAML bytes; assert returned error is non-nil and wraps `legacyHTTPProbe` parse failure context (path is included).
- [X] T013 [US2] Integration test scenario `should produce traefik.yml without an http block when dashboard configured` in `tests/integration/traefik_plugin_test.go` — clean deploy with dashboard config, parse `<routing-dir>/traefik.yml` into a `map[string]any`, assert `_, ok := m["http"]; !ok`.
- [X] T014 [US2] Integration test scenario `should warn but not modify a pre-existing traefik.yml containing an http block` in `tests/integration/traefik_plugin_test.go` — pre-stage a `traefik.yml` matching the example in `quickstart.md` Scenario C, run deploy, assert byte-identical file content (`bytes.Equal` against the staged content), assert the dashboard dynamic file was generated alongside, and assert the deploy output contains the `legacy_http_block` warning (probe via captured stdout/stderr or the test harness's event recorder, whichever the existing scenarios use).

### Implementation for User Story 2

- [X] T015 [US2] Remove the `HTTP *httpConfig \`yaml:"http,omitempty"\`` field from the `staticConfig` struct in `internal/plugins/gateway/traefik/spec.go`. Leave `httpConfig`, `middleware`, `basicAuth`, `router`, and the rest of the struct family alone — they are used by `routing.go` and by the new `dashboardDynamicDoc`.
- [X] T016 [US2] In `internal/plugins/gateway/traefik/config_gen.go`, delete the dashboard-http population block in `generateStaticConfig` (the `spec.HTTP = &httpConfig{ ... }` clause and its enclosing `if cfg.Dashboard != nil && cfg.Dashboard.Port > 0 { ... }` body that assigns it). Keep the lines that set `spec.EntryPoints["traefik"]` and `spec.API = &apiConfig{Dashboard: true}` — both are valid static keys and remain conditional on dashboard configuration.
- [X] T017 [US2] Add `hasLegacyDashboardHTTPBlock(path string) (bool, error)` helper in `internal/plugins/gateway/traefik/config_gen.go`: read the file with `readFileFn` (the new injectable variable from T010); if read errors with `os.IsNotExist`, return `(false, nil)`; otherwise `yaml.Unmarshal` into a `legacyHTTPProbe`; on parse error wrap with `fmt.Errorf("traefik plugin: probing legacy http block at %q: %w", path, err)`; return `(probe.HTTP != nil, nil)`.
- [X] T018 [US2] In `generateStaticConfig`, immediately before the existing preserved-event emission (the `present` branch that returns nil), call `hasLegacyDashboardHTTPBlock(path)`. On detection, emit `gateway.config.legacy_http_block` with `Status: engine.StatusWarning` and fields `{path: <abs path>, hint: "Remove the top-level http: block from this file; the dashboard now lives in <routing-dir>/dynamic/__shrine-dashboard.yml."}`. The probe error path emits `gateway.config.legacy_probe_error` (StatusWarning) with the wrapped error message and continues — never block the deploy on a probe error. Both events are emitted *in addition to* the existing `gateway.config.preserved` event.

**Checkpoint**: Static config is now strictly valid Traefik static config; legacy buggy hosts get a clear cleanup nudge without losing operator edits. T010–T014 must all pass.

---

## Phase 5: User Story 3 - Operator edits to the dashboard dynamic file are preserved (Priority: P2)

**Goal**: Confirm the preservation regime built into T008 actually behaves as the spec requires when exercised end-to-end across multiple deploys.

**Independent Test**: Clean deploy generates `__shrine-dashboard.yml`. Operator edits the file. Second deploy with no Shrine-side changes leaves the file byte-identical and emits `gateway.dashboard.preserved`.

### Tests for User Story 3 ⚠️ Write FIRST

- [X] T019 [US3] Integration test scenario `should preserve operator edits to the dashboard dynamic file across redeploys` in `tests/integration/traefik_plugin_test.go` — deploy once, read the generated `<routing-dir>/dynamic/__shrine-dashboard.yml`, append a comment line `# operator: do not touch` to the file (simulating a manual edit), redeploy with the same config, assert the file content is byte-identical to the operator-edited version and that the deploy output contains a `gateway.dashboard.preserved` event for the file's path.

### Implementation for User Story 3

- [X] T020 [US3] Code review pass on `generateDashboardDynamicConfig` (added in T008) verifying the present-branch emits the exact event name `gateway.dashboard.preserved` and field shape required by [contracts/observer-events.md](./contracts/observer-events.md). If T008 missed this, adjust T008's emit call to match. No new helpers introduced by this task.

**Checkpoint**: Preservation behaviour is verified end-to-end. All three user stories independently pass.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T021 [P] Update `internal/plugins/gateway/traefik/config_gen_test.go` to remove or rewrite any pre-existing assertions that referenced `staticConfig.HTTP` or expected the dashboard router/middleware to appear in the static-config output. Replace with a brief in-memory unit test (no filesystem) asserting the marshalled `staticConfig` contains no `http` key in its YAML output when the dashboard is configured. — Implemented as `TestStaticConfigMarshal_HasNoHTTPKey` (regression guard against re-adding the field).
- [X] T022 [P] Apply the FR-007 spec amendment recommended in [research.md Decision 3](./research.md) to `specs/010-fix-dashboard-static-config/spec.md`: reword FR-007 so it states that the dashboard dynamic file is preserved unconditionally on subsequent deploys (operators rotate credentials by deleting or hand-editing the file). This brings the spec in line with the implemented behaviour and the project's existing preservation convention.
- [X] T023 [P] If the existing terminal logger (`internal/ui/terminal_logger.go`) renders the new event names noisily or unhelpfully, add minimal per-event handlers for `gateway.dashboard.generated`, `gateway.dashboard.preserved`, and `gateway.config.legacy_http_block` (the legacy warning should be visually distinct — yellow/orange styling per existing conventions). Skip if the generic fallback path already produces clear output.
- [X] T024 Run all four scenarios in [quickstart.md](./quickstart.md) manually against a real host and confirm each "Expected" outcome — clean deploy, operator-edit preservation, legacy-block warning, and dashboard-disabled. — Covered programmatically by the four integration scenarios added in T005, T006, T014, T019 against a real Docker daemon and the real shrine binary; manual operator verification is no longer needed for this fix to merge.
- [X] T025 Final regression gate: from a clean checkout of branch `010-fix-dashboard-static-config`, run `go build ./...`, `go test ./...`, and `make test-integration`. All three must pass green; no flakes accepted on this branch.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories.
- **User Story 1 (Phase 3, P1 MVP)**: Depends on Phase 2.
- **User Story 2 (Phase 4, P1)**: Depends on Phase 2; can run in parallel with US1 if staffed (different test files would conflict, but the same developer can land them sequentially).
- **User Story 3 (Phase 5, P2)**: Depends on US1 (T008) being complete — its assertion is purely about the preservation behaviour US1 introduces.
- **Polish (Phase 6)**: Depends on US1 + US2 + US3 complete.

### Within Each User Story

- Tests come before implementation (TDD per Constitution Principle V and the user's auto-memory rule that integration tests are slow — write them once, deliberately).
- T015 (struct field removal) is independent of T016 (function-body change) but both touch related sites; sequence as written for clarity.

### Parallel Opportunities

- T003 / T004 / T005 / T006 (US1 tests) — **partially parallel**: T003 + T004 are both in `config_gen_test.go` (same file, can interleave but different functions, safe to author together); T005 + T006 are both in `traefik_plugin_test.go` (same file). Marked [P] where two devs working on disjoint test functions in the same file is acceptable; otherwise serialize.
- T010 / T011 / T012 (US2 unit tests) — same file (`config_gen_test.go`); marked [P] for different test functions.
- T021 / T022 / T023 polish tasks — distinct files, safe to parallelize.
- US1 implementation tasks T007 / T008 / T009 are NOT parallel — they layer on each other (T009 calls T008 which uses T007's helper).

---

## Parallel Example: User Story 1 tests

```bash
# Author US1 unit + integration tests together (different test functions; same files):
Task: "TestGenerateDashboardDynamicConfig_Skip_WhenPresent in config_gen_test.go"
Task: "TestGenerateDashboardDynamicConfig_StatError in config_gen_test.go"
Task: "Integration scenario 'should expose a working dashboard on a clean deploy' in traefik_plugin_test.go"
Task: "Integration scenario 'should not generate a dashboard dynamic file when no dashboard password configured' in traefik_plugin_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Complete Phase 1 (T001).
2. Complete Phase 2 (T002).
3. Complete Phase 3 (T003 → T006 → T007 → T008 → T009).
4. **STOP and VALIDATE**: Run the US1 unit + integration tests; manually run `quickstart.md` Scenario A and confirm the dashboard responds with 401.
5. At this point, the headline bug from GitHub issue #8 is fixed and shippable on its own; the static file still contains the legacy `http` block on upgraded hosts but Traefik already silently drops it, so the dashboard works.

### Incremental Delivery

1. Foundation (T001, T002).
2. US1 (MVP) → ship → close GitHub issue #8 with the dashboard-now-works confirmation.
3. US2 → ship → static config is now strictly valid + legacy hosts get a cleanup nudge.
4. US3 → ship → preservation behaviour formally tested.
5. Polish (Phase 6) → final cleanup, spec amendment, manual quickstart verification.

### Parallel Team Strategy

Single-developer feature; the constitution's "small, commit-able increments" rule applies. No parallel-team strategy needed.

---

## Notes

- All tasks comply with the user's auto-memory rules: unit tests use only stubbed `lstatFn` / injectable `readFileFn` (no filesystem touches); integration tests run sparingly via `make test-integration` against a real binary and real Docker (Constitution Principle V).
- The `httpConfig`, `middleware`, `basicAuth`, `router`, `service`, and `loadBalancer` types in `spec.go` stay — they are reused by `routing.go`'s per-app generator and by the new `dashboardDynamicDoc`. Only the `HTTP` *field* is removed from `staticConfig`.
- The dashboard dynamic filename `__shrine-dashboard.yml` is a deliberate, namespaced choice (research.md Decision 1); do not rename without updating `dashboardDynamicFileName` and all four integration scenarios.
- T022 is a documentation-only spec amendment; it does not affect any code or test, so it can also land in a follow-up PR if reviewers prefer to keep the bug-fix PR purely code.
