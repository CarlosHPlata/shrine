# Tasks: Traefik Gateway Plugin

**Input**: Design documents from `specs/001-traefik-gateway-plugin/`
**Prerequisites**: plan.md ✅ spec.md ✅ research.md ✅ data-model.md ✅ quickstart.md ✅

**Tests**: Integration tests are **MANDATORY** per Constitution Principle V (TDD enforced — write and verify tests FAIL before writing implementation code).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- Integration tests MUST be written and confirmed to FAIL/not-compile before implementation

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the plugin package skeleton and integration test scaffolding before any implementation begins.

- [X] T001 Create `internal/plugins/gateway/traefik/` package directory with three stub files: `plugin.go`, `routing.go`, `config_gen.go` (each with `package traefik` only)
- [X] T002 [P] Create `tests/integration/traefik_plugin_test.go` with `//go:build integration` tag, package declaration, and import of `testutils` — no test functions yet (scaffold only)
- [X] T003 [P] Create test fixture manifests at `tests/testdata/deploy/traefik/` — team manifest (`team.yaml`) and two app manifests: one with `ExposeToPlatform: true` + `routing.domain` set, one with `ExposeToPlatform: false`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core type extensions and engine fixes that ALL user story phases depend on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T004 Extend `internal/config/config.go` — add `PluginsConfig`, `GatewayPluginsConfig`, `TraefikPluginConfig`, and `TraefikDashboardConfig` structs (see data-model.md §1); add `Plugins PluginsConfig` field to `Config`
- [X] T005 [P] Extend `internal/engine/backends.go` — add `BindMount{Source, Target string}` and `PortBinding{HostPort, ContainerPort, Protocol string}` value types; add `RestartPolicy string`, `BindMounts []BindMount`, and `PortBindings []PortBinding` fields to `CreateContainerOp`
- [X] T006 Implement `RestartPolicy`, `BindMounts`, and `PortBindings` handling in `internal/engine/local/dockercontainer/docker_container.go` — extend `createFreshContainer` to set `HostConfig.RestartPolicy`, append bind mounts in `buildMounts`, populate `HostConfig.PortBindings` and `ContainerConfig.ExposedPorts` from `op.PortBindings` (depends on T005)
- [X] T007 [P] Fix routing gate in `internal/engine/engine.go` — change line `if application.Spec.Routing.Domain != "" && engine.Routing != nil` to also require `application.Spec.Networking.ExposeToPlatform` (see research.md Decision 6)
- [X] T008 [P] Implement `WriteRoute` and `RemoveRoute` stubs in `internal/engine/dryrun/dry_run_routing.go` — print route details to the dry-run writer instead of writing files; satisfies dry-run requirement for the routing backend interface

**Checkpoint**: Foundation ready — all user story phases can now begin.

---

## Phase 3: User Story 1 — Configure Traefik Gateway (Priority: P1) 🎯 MVP

**Goal**: Plugin reads config, validates it, generates static Traefik config, and deploys the Traefik container on the platform network when the plugin block is active.

**Independent Test**: Add `plugins.gateway.traefik: {}` (empty → inactive) and a populated block (→ active) to shrine config; run `shrine deploy` against the fixture in `tests/testdata/deploy/traefik/`; assert container `shrine.platform.traefik` exists iff plugin is active.

### Integration Tests for User Story 1 — write FIRST, confirm they FAIL

- [X] T009 [US1] Write integration test in `tests/integration/traefik_plugin_test.go`: `TestTraefikPlugin_DeploysContainerWhenActive` — shrine config with populated `plugins.gateway.traefik`, run deploy, assert container `shrine.platform.traefik` is running on platform network
- [X] T010 [P] [US1] Write integration test in `tests/integration/traefik_plugin_test.go`: `TestTraefikPlugin_SkipsDeployWhenAbsent` — shrine config with no plugin section, run deploy, assert no container named `shrine.platform.traefik` exists
- [X] T011 [P] [US1] Write integration test in `tests/integration/traefik_plugin_test.go`: `TestTraefikPlugin_FailsWhenDashboardPortMissingCredentials` — shrine config with `dashboard.port` but no username/password, run deploy, assert non-zero exit before any container starts

### Implementation for User Story 1

- [X] T012 [US1] Implement `Plugin` struct, `New()`, `isActive()`, `Validate()`, `hasDashboard()`, `hasCredentials()`, `resolvedImage()`, `resolvedPort()` in `internal/plugins/gateway/traefik/plugin.go` — `isActive()` returns true iff cfg is non-nil and at least one field is non-zero; `Validate()` returns error if `hasDashboard() && !hasCredentials()`
- [X] T013 [P] [US1] Implement `generateStaticConfig(cfg *TraefikPluginConfig, routingDir string) error` in `internal/plugins/gateway/traefik/config_gen.go` — writes `traefik.yml` to `routingDir` with entryPoints web section and file provider pointing to `routingDir/dynamic`; when `cfg.Dashboard != nil`, adds dashboard entrypoint, enables API, and generates a basicAuth middleware using an htpasswd-formatted hash of `cfg.Dashboard.Password` (SHA1 used to keep stdlib-only)
- [X] T014 [US1] Implement `Deploy() error` on `Plugin` in `internal/plugins/gateway/traefik/plugin.go` — calls `os.MkdirAll` on resolvedRoutingDir, calls `generateStaticConfig`, then calls `backend.CreateContainer` with `RestartPolicy: "always"`, `BindMounts` mounting resolvedRoutingDir to `/etc/traefik`, and `PortBindings` for the routing port (depends on T012, T013)
- [X] T015 [US1] Wire plugin into `internal/handler/deploy.go` — in `Deploy()`: call `plugin.Validate()` before planning; after `ExecuteDeploy`, call `plugin.Deploy()` if active; in `DryRun()`: call `plugin.Validate()` only (skip Deploy and config generation)

**Checkpoint**: `shrine deploy` with active plugin config starts `shrine.platform.traefik` on the platform network; absent/empty plugin config is a no-op.

---

## Phase 4: User Story 2 — Custom Routing Directory (Priority: P2)

**Goal**: When `routing-dir` is specified, that path is created if absent and bind-mounted into Traefik; when absent, `{specsDir}/traefik/` is used.

**Independent Test**: Run deploy with `routing-dir: /tmp/shrine-traefik-test`; confirm that directory is created and that the Traefik container is started with `/tmp/shrine-traefik-test` bind-mounted to `/etc/traefik`.

### Integration Tests for User Story 2 — write FIRST, confirm they FAIL

- [X] T016 [US2] Write integration test in `tests/integration/traefik_plugin_test.go`: `TestTraefikPlugin_UsesCustomRoutingDir` — set `routing-dir` to a temp path that does not exist, run deploy, assert directory is created and Traefik container bind-mount source matches the custom path; also assert `traefik.yml` is present in that directory

### Implementation for User Story 2

- [X] T017 [US2] Implement `resolvedRoutingDir() string` on `Plugin` in `internal/plugins/gateway/traefik/plugin.go` — returns `cfg.RoutingDir` if non-empty, else `filepath.Join(p.specsDir, "traefik")`; does NOT create the directory (creation is `Deploy()`'s responsibility)
- [X] T018 [US2] Verify `Deploy()` already calls `os.MkdirAll(p.resolvedRoutingDir(), 0755)` before writing config (from T014); add `os.MkdirAll` for `p.resolvedRoutingDir()+"/dynamic"` subdirectory as well in `internal/plugins/gateway/traefik/plugin.go`

**Checkpoint**: Custom `routing-dir` is created on first deploy and mounted into Traefik; omitting `routing-dir` falls back to `{specsDir}/traefik/`.

---

## Phase 5: User Story 3 — Config Generation Gated by Plugin State (Priority: P3)

**Goal**: Routing config files are generated only for eligible apps (Domain + ExposeToPlatform) when the plugin is active; operator files in `routing-dir` are never deleted; absent plugin leaves no generated files.

**Independent Test**: Deploy with two apps (one eligible, one not); confirm only the eligible app's `dynamic/{team}-{name}.yml` exists; add an operator file to `dynamic/`; redeploy; confirm operator file survives.

### Integration Tests for User Story 3 — write FIRST, confirm they FAIL

- [X] T019 [US3] Write integration test in `tests/integration/traefik_plugin_test.go`: `TestTraefikPlugin_GeneratesRouteForEligibleApp` — deploy with two apps; assert `dynamic/{team}-{eligible-app}.yml` exists and contains correct host rule; assert no file for the non-eligible app
- [X] T020 [P] [US3] Write integration test in `tests/integration/traefik_plugin_test.go`: `TestTraefikPlugin_PreservesOperatorFiles` — create an operator YAML in `routing-dir/dynamic/operator-custom.yml` before deploy; run deploy; assert file still exists with original content
- [X] T021 [P] [US3] Write integration test in `tests/integration/traefik_plugin_test.go`: `TestTraefikPlugin_NoFilesWhenInactive` — shrine config with absent plugin section; run deploy; assert no `traefik.yml` and no `dynamic/` directory created

### Implementation for User Story 3

- [X] T022 [US3] Implement `RoutingBackend` struct, `routeFileName(team, name string) string`, `WriteRoute(op engine.WriteRouteOp) error`, and `RemoveRoute(team, host string) error` in `internal/plugins/gateway/traefik/routing.go` — `WriteRoute` writes `{routingDir}/dynamic/{team}-{name}.yml` with Traefik HTTP router + service YAML (see research.md Decision 2); `RemoveRoute` removes the file if it exists; `routeFileName` produces `{team}-{name}.yml`
- [X] T023 [US3] Add `RoutingBackend() engine.RoutingBackend` method to `Plugin` in `internal/plugins/gateway/traefik/plugin.go` — returns a `&RoutingBackend{routingDir: p.resolvedRoutingDir()}` instance
- [X] T024 [US3] Wire routing backend into `internal/handler/deploy.go` — when plugin is active, create `engine.Engine` with `Routing: plugin.RoutingBackend()`; when inactive, keep `Routing: nil` (existing behaviour); for DryRun, pass the dry-run routing backend from `dryrun` package

**Checkpoint**: Route files appear only for eligible apps; operator files are untouched; no plugin = no generated files.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, validation sweep, and integration gate sign-off.

- [X] T025 Write integration test in `tests/integration/traefik_plugin_test.go`: `TestTraefikPlugin_DryRunProducesNoSideEffects` — run `shrine deploy --dry-run` with active plugin config; assert no container `shrine.platform.traefik` exists, no files written to `routing-dir`, and stdout contains route description output (Constitution Principle II gate)
- [X] T026 Update `AGENTS.md` to document the `plugins.gateway.traefik` config schema (including `routing-dir`), the `internal/plugins/gateway/traefik/` package, and the updated `CreateContainerOp` fields
- [X] T027 [P] Run `go test ./...` and fix all unit-level test failures introduced by foundational changes (config types, CreateContainerOp extensions, engine routing gate fix)
- [X] T028 Run integration test suite `go test -tags integration ./tests/integration/...` — all tests including new traefik plugin tests must pass; this is the Phase V constitution gate
- [X] T029 [P] Validate `quickstart.md` example end-to-end — covered by `TestTraefikPlugin_DeploysContainerWhenActive` and `TestTraefikPlugin_UsesCustomRoutingDir`, which exercise the quickstart's minimal config plus custom routing-dir variant

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately; T002 and T003 are parallel with T001
- **Foundational (Phase 2)**: Depends on Phase 1 — BLOCKS all user story phases; T005, T007, T008 are parallel after T004 lands; T006 depends on T005
- **US1 Phase (Phase 3)**: Depends on Phase 2 — TDD: T009/T010/T011 first (parallel), then T012 → T013 (parallel with T012) → T014 → T015
- **US2 Phase (Phase 4)**: Depends on Phase 3 checkpoint — T016 first, then T017 → T018
- **US3 Phase (Phase 5)**: Depends on Phase 4 checkpoint — T019/T020/T021 first (parallel), then T022 → T023 → T024
- **Polish (Phase 6)**: Depends on all user story phases complete; T027 and T029 are parallel with T026

### User Story Dependencies

- **US1 (P1)**: No dependency on US2 or US3 — independently testable after Phase 2
- **US2 (P2)**: Builds on US1 (Deploy() scaffolding) — start after US1 checkpoint
- **US3 (P3)**: Builds on US2 (routingDir resolved) and US1 (plugin wiring in handler) — start after US2 checkpoint

### Within Each User Story

1. Write integration test(s) → confirm they FAIL (TDD gate)
2. Implement feature → run integration tests → confirm they PASS
3. Commit before advancing to next story

---

## Parallel Opportunities

### Phase 2 (after T004)
```
T005 (backends.go)     T007 (engine.go)      T008 (dry_run_routing.go)
     ↓
T006 (docker_container.go)
```

### Phase 3 (US1 tests — parallel, then sequential impl)
```
T009   T010   T011        ← write all three tests in parallel
  ↓ confirm all FAIL ↓
T012 → T013              ← T012 and T013 can overlap (different functions)
           ↓
         T014 → T015
```

### Phase 5 (US3 tests — parallel)
```
T019   T020   T021        ← write all three in parallel
  ↓ confirm all FAIL ↓
T022 → T023 → T024
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (Setup)
2. Complete Phase 2 (Foundational — critical blocker)
3. Write US1 integration tests (T009–T011), confirm they FAIL
4. Complete Phase 3 (US1 implementation)
5. **STOP**: Run `shrine deploy` with plugin config — confirm Traefik container starts
6. Run integration tests — confirm T009–T011 now PASS

### Incremental Delivery

1. Setup + Foundational → foundation ready
2. US1 → Traefik deploys when config active (MVP!)
3. US2 → custom routing-dir works + created if missing
4. US3 → routing rules generated for eligible apps only
5. Polish → docs + integration gate sign-off

---

## Notes

- **TDD is mandatory** (Constitution Principle V): integration tests at `tests/integration/traefik_plugin_test.go` MUST be written before the corresponding implementation and MUST fail before implementation makes them pass, also unit test must follow each functionality in code level.
- [P] tasks operate on different files — no merge conflicts when run concurrently
- [Story] label maps each task to a user story for traceability
- The `internal/plugins/gateway/traefik/` package has no imports from shrine handler or cmd — it only imports `internal/config`, `internal/engine`, and stdlib
- Commit after each checkpoint to keep history clean and reversible
- The integration gate (T028) is the final sign-off required before this feature branch can merge
