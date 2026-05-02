---

description: "Task list for traefik-tlsport-config"
---

# Tasks: Traefik Plugin `tlsPort` Config Option

**Input**: Design documents from `/specs/011-traefik-tlsport-config/`
**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/observer-events.md](./contracts/observer-events.md), [quickstart.md](./quickstart.md)

**Tests**: REQUIRED — Constitution Principle V mandates that integration tests are written before the implementation they cover. Unit tests follow the project's strict no-filesystem rule (use the existing `lstatFn` / `readFileFn` injection points and in-memory YAML bytes only).

**Organization**: Tasks are grouped by user story so each story can be implemented and verified independently. The two P1 stories (US1: HTTPS publishing; US2: backward-compat without `tlsPort`) share the same conditional code paths but assert independently. US3 (P2) layers on top — its preserved-file warning behaviour is a separate code path that only fires when an operator already had a hand-edited `traefik.yml`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel — different files OR different test functions in the same file with no dependency on an incomplete task
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- File paths in every task are repo-root-relative

## Path Conventions

- Operator config schema: `internal/config/plugin_traefik.go`
- Plugin code: `internal/plugins/gateway/traefik/` (modify in place)
- Engine state hash: `internal/state/deployments.go` + caller `internal/engine/local/dockercontainer/docker_container.go`
- Terminal logger: `internal/ui/terminal_logger.go`
- Unit tests: same package as the code under test; no filesystem touches; reuse the existing `lstatFn` and `readFileFn` injection points
- Integration tests: `tests/integration/traefik_plugin_test.go` (build tag `integration`; uses `NewDockerSuite`)
- Test helpers: `tests/integration/testutils/assert_docker.go`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish a known-green baseline before any code change.

- [X] T001 Confirm baseline `go build ./...` and `go test ./...` succeed on branch `011-traefik-tlsport-config` with no pre-existing failures, and confirm `make test-integration` passes (or, if a long run is unwise, snapshot the latest known-passing commit on `main` as the comparison baseline). Record the snapshot SHA in the PR description.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Land the schema-field addition and the engine-wide `ConfigHash` extension. Both are required by every user story; no story-level test can compile until these land.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T002 Add the `TLSPort int \`yaml:"tlsPort,omitempty"\`` field to `TraefikPluginConfig` in `internal/config/plugin_traefik.go`, placed immediately after the existing `Port int` field. Do NOT change `TraefikDashboardConfig` or any other type. The field name uses the `TLS` Go-acronym convention; the YAML tag preserves the issue's `tlsPort` camelCase verbatim.
- [X] T003 Extend `state.ConfigHash` in `internal/state/deployments.go` to take a new `portSpecs []string` argument inserted **after** `volSpecs` and **before** `exposeToPlatform` (signature: `ConfigHash(image string, env, volSpecs, portSpecs []string, exposeToPlatform bool) string`). Sort `portSpecs` internally exactly like `env`/`volSpecs`, and join its sorted form into the hash input between the volume block and the platform-exposure flag (preserving determinism). Update the existing doc comment to mention `portSpecs must be "<hostPort>:<containerPort>/<proto>" strings`.
- [X] T004 Update the sole caller `configHash()` in `internal/engine/local/dockercontainer/docker_container.go` (around line 236) to project `op.PortBindings` into the new `portSpecs []string`: each entry is `fmt.Sprintf("%s:%s/%s", b.HostPort, b.ContainerPort, proto)` where `proto` defaults to `"tcp"` when empty (mirror the default in `buildPortBindings`). Pass the projected slice to `state.ConfigHash`. Note in the commit message: this changes the hash format → every existing container will recreate exactly once on first deploy after this lands; that is intentional (see [research.md Decision 3](./research.md)) and acceptable for the homelab target audience.

**Checkpoint**: Foundation ready — every user story's tests can now compile against the new field and the new hash signature.

---

## Phase 3: User Story 1 - Operator publishes HTTPS through the Traefik plugin (Priority: P1) 🎯 MVP

**Goal**: Adding a single `tlsPort: 443` (or any other host port) line to `~/.config/shrine/config.yml` results in (a) the Traefik container publishing host port → container `443/tcp`, and (b) the generated `traefik.yml` declaring a bare `websecure` entrypoint at `:443`. Validation rejects out-of-range values and collisions with `port` / `dashboard.port`. Changing `tlsPort` between deploys recreates the container.

**Independent Test**: On a clean host, configure `plugins.gateway.traefik.{port:80, tlsPort:443}` and run `shrine deploy`. `docker inspect platform.traefik --format '{{json .HostConfig.PortBindings}}'` returns a map containing `"443/tcp": [{HostPort: "443"}]`. The generated `traefik.yml` contains `entryPoints.websecure.address: ":443"` with no other keys under that entrypoint. Restarting deploy with `tlsPort: 8443` recreates the container with the new mapping. Setting `tlsPort: 443, port: 443` rejects with a validation error naming both fields.

### Tests for User Story 1 ⚠️ Write FIRST and confirm they FAIL before T013–T016

- [X] T005 [P] [US1] Unit test `TestPlugin_Validate_AcceptsValidTLSPort` in NEW file `internal/plugins/gateway/traefik/plugin_test.go` — construct a `Plugin` with `cfg.Port=80, cfg.TLSPort=443, cfg.Dashboard=nil` via `New(...)`; assert `err == nil`. Use a fresh package-internal `noopBackend` (or reuse one from `helpers_test.go` if present) for the `engine.ContainerBackend` arg.
- [X] T006 [P] [US1] Unit test `TestPlugin_Validate_RejectsTLSPortOutOfRange` in `internal/plugins/gateway/traefik/plugin_test.go` — table-driven cases for `cfg.TLSPort` ∈ {-1, 0 with non-zero sentinel marker, 65536, 100000}; assert each `New(...)` returns an error whose message contains `"tlsPort"` and the specific value. Note: `0` is the unset sentinel and MUST be accepted, so do not include `0` as an out-of-range case.
- [X] T007 [P] [US1] Unit test `TestPlugin_Validate_RejectsTLSPortCollidesWithPort` in `internal/plugins/gateway/traefik/plugin_test.go` — two sub-cases: (a) `cfg.Port=443, cfg.TLSPort=443` rejects with message containing both `"tlsPort"` and `"port"`; (b) `cfg.Port=0` (default→80), `cfg.TLSPort=80` rejects (default-vs-explicit collision uses `resolvedPort()`). Assert both error messages name the offending fields.
- [X] T008 [P] [US1] Unit test `TestPlugin_Validate_RejectsTLSPortCollidesWithDashboardPort` in `internal/plugins/gateway/traefik/plugin_test.go` — `cfg.Port=80, cfg.TLSPort=8080, cfg.Dashboard={Port:8080, Username:"u", Password:"p"}` rejects with message containing `"tlsPort"` and `"dashboard.port"`.
- [X] T009 [P] [US1] Unit test `TestPlugin_PortBindings_IncludesTLS443_WhenTLSPortSet` in `internal/plugins/gateway/traefik/plugin_test.go` — `cfg.Port=80, cfg.TLSPort=8443, cfg.Dashboard=nil`; call `(&Plugin{cfg: &cfg}).portBindings()`; assert the returned slice contains exactly two entries: `{HostPort:"80", ContainerPort:"80", Protocol:"tcp"}` AND `{HostPort:"8443", ContainerPort:"443", Protocol:"tcp"}`. Order does not matter; assert by set membership.
- [X] T010 [P] [US1] Unit test `TestStaticConfigMarshal_HasBareWebsecureEntryPoint_WhenTLSPortSet` in `internal/plugins/gateway/traefik/config_gen_test.go`, mirroring the existing `TestStaticConfigMarshal_HasNoHTTPKey` pattern (no filesystem; pure in-memory struct + marshal). Construct a `staticConfig{EntryPoints: {"web": {Address: ":80"}, "websecure": {Address: ":443"}}, Providers: providersConfig{File: fileProvider{Directory: "/etc/traefik/dynamic", Watch: true}}}` directly in-memory; call the existing `yamlMarshal` helper; assert the marshalled bytes contain both `websecure:` and `address: :443`, and do NOT contain any of `tls:`, `certResolver`, or `http.tls` (regression guard for FR-010 on the entrypoint shape). Per the explicit comment block at `config_gen_test.go:168-172`, the `os.WriteFile` write branch of `generateStaticConfig` is NEVER exercised in unit tests; the end-to-end conditional logic (`cfg.TLSPort > 0` → `EntryPoints["websecure"]` populated) is exercised by integration test T012.
- [X] T011 [US1] Add the new testutils helper `AssertContainerPublishesPort(name, hostPort, containerPort, proto string) *TestCase` to `tests/integration/testutils/assert_docker.go`, modelled on the existing `AssertContainerHasBindMount`. It calls `tc.DockerClient.ContainerInspect(ctx, name)`, navigates `info.HostConfig.PortBindings`, and asserts that there is at least one binding for `<containerPort>/<proto>` whose `HostPort` equals `hostPort`. Failure message must list every binding actually present so debugging is fast.
- [X] T012 [US1] Integration test scenario `should publish tlsPort to container 443 and add websecure entrypoint on clean deploy` in `tests/integration/traefik_plugin_test.go`. Write a fixture config with `port: 8101, tlsPort: 8443`, run `tc.Run("deploy", ...).AssertSuccess()`, then: `tc.AssertContainerRunning(traefikContainerName)`, `tc.AssertContainerPublishesPort(traefikContainerName, "8443", "443", "tcp")`, `tc.AssertContainerPublishesPort(traefikContainerName, "8101", "8101", "tcp")` (regression). Read `<routing-dir>/traefik.yml`, parse to `map[string]any`, navigate `entryPoints.websecure`, assert the only key is `address` and its value is `":443"` (regression for FR-010 on the entrypoint shape).
- [X] T013 [US1] Integration test scenario `should reject tlsPort that collides with port at validation time` in `tests/integration/traefik_plugin_test.go`. Write a config with `port: 443, tlsPort: 443`, run `tc.Run("deploy", ...).AssertFailure()` (use the harness's existing failure-assertion helper, or its `Run().Stderr` accessor) and assert stderr contains both `tlsPort` and `port` and the value `443`. Assert no Traefik container was created (`tc.AssertContainerNotExists(traefikContainerName)`). Cover SC-004.
- [X] T014 [US1] Integration test scenario `should recreate traefik container when tlsPort value changes between deploys` in `tests/integration/traefik_plugin_test.go`. Deploy once with `tlsPort: 443`. Capture the container ID via `tc.DockerClient.ContainerInspect(...)`. Rewrite the config with `tlsPort: 8443`, redeploy, assert success. Re-inspect the container; assert its ID is **different** (recreated, not just restarted) AND `tc.AssertContainerPublishesPort(traefikContainerName, "8443", "443", "tcp")`. This is the end-to-end regression for the [research.md Decision 3](./research.md) `ConfigHash` extension; if the hash extension is wrong, this test fails.

### Implementation for User Story 1

- [X] T015 [US1] Extend `Plugin.validate()` in `internal/plugins/gateway/traefik/plugin.go` with three additional checks, gated on `cfg.TLSPort != 0`: (a) range check `cfg.TLSPort < 1 || cfg.TLSPort > 65535` returns `fmt.Errorf("traefik plugin: tlsPort %d is out of range (1-65535)", cfg.TLSPort)`; (b) collision-with-port check using `p.resolvedPort()` returns `fmt.Errorf("traefik plugin: tlsPort %d collides with port %d", cfg.TLSPort, p.resolvedPort())`; (c) collision-with-dashboard-port check (only when `p.hasDashboard()`) returns `fmt.Errorf("traefik plugin: tlsPort %d collides with dashboard.port %d", cfg.TLSPort, cfg.Dashboard.Port)`. Place these after the existing `dashboard credentials required` check; return on first error (per [research.md Decision 5](./research.md): single integer field, mutually-exclusive failures).
- [X] T016 [US1] Extend `Plugin.portBindings()` in `internal/plugins/gateway/traefik/plugin.go` to append, when `p.cfg.TLSPort > 0`, a `engine.PortBinding{HostPort: strconv.Itoa(p.cfg.TLSPort), ContainerPort: "443", Protocol: "tcp"}`. Add it after the existing HTTP `port` binding and before the (optional) dashboard binding so the slice remains in a stable order for tests.
- [X] T017 [US1] Extend the generation branch of `generateStaticConfig` in `internal/plugins/gateway/traefik/config_gen.go` to add `spec.EntryPoints["websecure"] = entryPoint{Address: ":443"}` when `cfg.TLSPort > 0`. Place the new conditional immediately after the existing `if cfg.Dashboard != nil && cfg.Dashboard.Port > 0` block. Do NOT add any other key under that entrypoint and do NOT touch the preserved branch (FR-007 / [research.md Decision 4](./research.md)). The existing `entryPoint` struct in `spec.go` only has an `Address` field, which structurally guarantees no `tls`/`certResolver`/etc. keys can leak.

**Checkpoint**: User Story 1 is complete and shippable as MVP. T005–T014 must all pass. The headline issue from GitHub #12 is resolved.

---

## Phase 4: User Story 2 - Existing deploys without `tlsPort` keep working unchanged (Priority: P1)

**Goal**: An operator who has not opted into HTTPS and leaves `tlsPort` unset sees byte-identical generated `traefik.yml` content (modulo unrelated, already-shipped changes) and the same set of host port mappings on the Traefik container as before this feature shipped. No new errors, warnings, or behaviour changes.

**Independent Test**: On a clean host, deploy with `plugins.gateway.traefik.port: 80` only (no `tlsPort` line). Inspect generated `traefik.yml`: it contains no `websecure` entrypoint. Inspect container `HostConfig.PortBindings`: it contains no mapping for `443/tcp`. Repeat the deploy a second time and confirm the container is NOT recreated (hash unchanged), proving the hash extension does not spuriously trigger drift on no-op redeploys.

### Tests for User Story 2 ⚠️ Write FIRST

- [X] T018 [P] [US2] Unit test `TestPlugin_PortBindings_OmitsTLS_WhenTLSPortUnset` in `internal/plugins/gateway/traefik/plugin_test.go` — `cfg.Port=80, cfg.TLSPort=0, cfg.Dashboard=nil`; assert `portBindings()` returns exactly one entry `{HostPort:"80", ContainerPort:"80", Protocol:"tcp"}` and no `443/tcp` entry.
- [X] T019 [P] [US2] Unit test `TestStaticConfigMarshal_OmitsWebsecureEntryPoint_WhenAbsent` in `internal/plugins/gateway/traefik/config_gen_test.go`, mirroring T010's in-memory pattern (no filesystem). Construct a `staticConfig{EntryPoints: {"web": {Address: ":80"}}, Providers: providersConfig{File: fileProvider{Directory: "/etc/traefik/dynamic", Watch: true}}}` with no `websecure` key; call `yamlMarshal`; assert the marshalled bytes do NOT contain the substring `websecure`. The end-to-end conditional logic (no `tlsPort` in config → `generateStaticConfig` does not populate `EntryPoints["websecure"]`) is exercised by integration test T020.
- [X] T020 [US2] Integration test scenario `should not publish 443 nor add websecure entrypoint when tlsPort is unset` in `tests/integration/traefik_plugin_test.go` — write a fixture config with `port: 8102` only (no `tlsPort:` line), deploy, then assert: container has NO binding for `443/tcp` (extend `AssertContainerPublishesPort` with a sibling `AssertContainerNotPublishingContainerPort(name, containerPort, proto)` if needed, or inspect inline via `tc.DockerClient`); generated `traefik.yml` has no `websecure` key (parse to `map[string]any`, navigate `entryPoints`, assert key absent).
- [X] T021 [US2] Integration test scenario `should not recreate traefik container on no-op redeploy` in `tests/integration/traefik_plugin_test.go` — deploy with `port: 8103` only, capture the container ID, redeploy with the identical config, re-inspect the container; assert the ID is **identical** (no recreate). This is the regression guard for the `ConfigHash` extension's stability — if T003/T004 accidentally include unstable inputs in the hash (e.g., randomized iteration order over `op.PortBindings`), this test fails.

### Implementation for User Story 2

User Story 2 introduces no new production code. The conditional guards added in T015 (`if cfg.TLSPort != 0`), T016 (`if p.cfg.TLSPort > 0`), and T017 (`if cfg.TLSPort > 0`) cover this story by default. T018–T021 are regression guards that lock that behaviour in. If any of them fail, the bug is in the US1 conditionals, not in new US2 code.

**Checkpoint**: User Story 2 verified — backward compatibility for the no-`tlsPort` configuration is provably intact. T018–T021 must all pass.

---

## Phase 5: User Story 3 - Operator-edited `traefik.yml` benefits from `tlsPort` for the host binding (Priority: P2)

**Goal**: When `tlsPort` is set AND the static `traefik.yml` is operator-preserved (per spec 004's preservation regime), Shrine still applies the host port mapping (FR-007), leaves the file untouched, and emits a clear deploy-time warning when the preserved file lacks a `websecure` entrypoint at `:443` (FR-008). The warning is idempotent — emitted on every deploy where the mismatch persists.

**Independent Test**: Hand-write a `traefik.yml` containing a `web` entrypoint and provider config but NO `websecure` entrypoint. Set `tlsPort: 443` in the Shrine config. Run `shrine deploy`. Confirm: file is byte-identical post-deploy; container publishes `443/tcp`; deploy output contains the `gateway.config.tls_port_no_websecure` warning naming the file path. Run `shrine deploy` again with no changes; confirm the warning fires again (idempotent).

### Tests for User Story 3 ⚠️ Write FIRST

- [X] T022 [P] [US3] Unit test `TestHasWebsecureEntrypoint_Detected` in `internal/plugins/gateway/traefik/config_gen_test.go` — stub `readFileFn` to return `entryPoints:\n  web:\n    address: ":80"\n  websecure:\n    address: ":443"\n` bytes; call `hasWebsecureEntrypoint("any/path")`; assert `(true, nil)`.
- [X] T023 [P] [US3] Unit test `TestHasWebsecureEntrypoint_Missing` in `internal/plugins/gateway/traefik/config_gen_test.go` — stub `readFileFn` to return YAML containing only `web` and `traefik` entrypoints (no `websecure`); assert `(false, nil)`.
- [X] T024 [P] [US3] Unit test `TestHasWebsecureEntrypoint_ParseError` in `internal/plugins/gateway/traefik/config_gen_test.go` — stub `readFileFn` to return malformed YAML bytes; assert returned error is non-nil and wraps the path in its message (mirror the existing `TestHasLegacyDashboardHTTPBlock_ParseError` shape).
- [X] T025 [P] [US3] Unit test `TestEmitTLSPortNoWebsecureSignal_EmitsWarning_WhenMismatch` in `internal/plugins/gateway/traefik/config_gen_test.go` — stub `readFileFn` for a YAML without `websecure`; call `emitTLSPortNoWebsecureSignal(staticPath, &config.TraefikPluginConfig{TLSPort:443}, recorder)`; assert exactly one event with `Name=="gateway.config.tls_port_no_websecure"`, `Status==engine.StatusWarning`, `Fields["path"]==staticPath`, and `Fields["hint"]` non-empty and containing the substring `websecure`.
- [X] T026 [P] [US3] Unit test `TestEmitTLSPortNoWebsecureSignal_NoEvent_WhenWebsecurePresent` in `internal/plugins/gateway/traefik/config_gen_test.go` — stub `readFileFn` for YAML containing `entryPoints.websecure`; call the same signal helper; assert zero events emitted.
- [X] T027 [P] [US3] Unit test `TestEmitTLSPortNoWebsecureSignal_NoEvent_WhenTLSPortUnset` in `internal/plugins/gateway/traefik/config_gen_test.go` — stub `readFileFn` for any YAML; call the helper with `cfg.TLSPort=0`; assert zero events emitted (the warning is opt-in via `tlsPort`).
- [X] T028 [US3] Integration test scenario `should warn but not modify preserved traefik.yml lacking websecure entrypoint when tlsPort is set` in `tests/integration/traefik_plugin_test.go` — pre-stage a hand-written `traefik.yml` containing `web` entrypoint and `providers.file` only (no `websecure`); write a Shrine config with `port: 8104, tlsPort: 8444`; deploy; assert `bytes.Equal(staged, postDeploy)` for the `traefik.yml` content; assert `tc.AssertContainerPublishesPort(traefikContainerName, "8444", "443", "tcp")` (FR-007 — port still bound); assert `tc.AssertOutputContains("tlsPort set but traefik.yml is missing websecure entrypoint")` (the new event's terminal-logger rendering).
- [X] T029 [US3] Integration test scenario `should re-emit the websecure-missing warning on every deploy where the mismatch persists` in `tests/integration/traefik_plugin_test.go` — same setup as T028; run `shrine deploy` twice in a row; assert the warning string appears in the second run's output as well.

### Implementation for User Story 3

- [X] T030 [US3] Add `hasWebsecureEntrypoint(path string) (bool, error)` helper in `internal/plugins/gateway/traefik/config_gen.go`, modelled on `hasLegacyDashboardHTTPBlock`. Read the file via `readFileFn`; on `os.IsNotExist` return `(false, nil)` (no warning; the file does not exist means we will be generating it, not warning); on other read errors wrap with `fmt.Errorf("traefik plugin: probing websecure entrypoint at %q: %w", path, err)`. Unmarshal into a new minimal probe struct `websecureProbe { EntryPoints map[string]*yaml.Node \`yaml:"entryPoints"\` }`; on parse error wrap similarly. Return `_, ok := probe.EntryPoints["websecure"]; ok`.
- [X] T031 [US3] Add `emitTLSPortNoWebsecureSignal(staticPath string, cfg *config.TraefikPluginConfig, observer engine.Observer)` helper in `internal/plugins/gateway/traefik/config_gen.go`, modelled on `emitLegacyHTTPBlockSignal`. Early-return when `cfg.TLSPort == 0`. Call `hasWebsecureEntrypoint(staticPath)`; on probe error emit `gateway.config.legacy_probe_error` with the wrapped error message (reuse the existing event name per [contracts/observer-events.md](./contracts/observer-events.md) "Non-events" section — same probe-failure semantics) and return without further action. On `(true, nil)` return silently. On `(false, nil)` emit `gateway.config.tls_port_no_websecure` with `Status=engine.StatusWarning` and `Fields={"path": staticPath, "hint": fmt.Sprintf("tlsPort=%d publishes host port %d→443/tcp on the Traefik container, but this preserved traefik.yml has no entryPoints.websecure listening on :443. Add the entrypoint, or delete the file so Shrine regenerates it.", cfg.TLSPort, cfg.TLSPort)}`.
- [X] T032 [US3] In `generateStaticConfig` in `internal/plugins/gateway/traefik/config_gen.go`, inside the existing `if present { ... }` preserved branch, call `emitTLSPortNoWebsecureSignal(path, cfg, observer)` immediately after the existing `emitLegacyHTTPBlockSignal(path, routingDir, observer)` call and before the `gateway.config.preserved` event emission. Both signals are advisory and do not affect the function's return value.
- [X] T033 [US3] Add a new `case "gateway.config.tls_port_no_websecure":` clause to `internal/ui/terminal_logger.go` next to the existing `gateway.config.legacy_http_block` case. Render as `fmt.Fprintf(t.out, "  ⚠️  tlsPort set but traefik.yml is missing websecure entrypoint at %s — %s\n", e.Fields["path"], e.Fields["hint"])`. Match the indentation and `⚠️` glyph already used by sibling warnings.

**Checkpoint**: All three user stories are independently verified end-to-end. T022–T029 must all pass.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T034 [P] Update `AGENTS.md` Traefik plugin config example (around line 268) to add `tlsPort: 443                    # optional HTTPS host port; publishes <tlsPort>:443/tcp and adds websecure entrypoint` immediately after the `port: 80` line. Keep the existing alignment style of trailing comments. Covers FR-009.
- [X] T036 [P] Cross-check that the existing per-app integration scenarios in `tests/integration/traefik_plugin_test.go` (in particular the `should write dynamic route file only for apps with domain + ExposeToPlatform` test and the dashboard scenarios) still pass byte-for-byte under the new `ConfigHash` (Phase 2). If any pre-existing scenario asserts on `traefik.yml` content via byte equality, update it to allow either ordering of `entryPoints` keys (YAML map iteration is unstable in Go); use `yaml.Unmarshal` + map-key assertions rather than substring match. No changes required if all current assertions are already structural — verify by running `make test-integration` once Phase 5 is green. — Verified: `go build ./...` + `go test ./...` (unit) + `go build -tags integration ./tests/integration/...` all green; existing scenarios already use structural `yaml.Unmarshal` checks, no byte-equality rewrites needed. Integration run deferred to T038.
- [X] T037 Run all eight steps in [quickstart.md](./quickstart.md) manually against a real Linux host with Docker (or rely on the integration suite's coverage if a manual host is unavailable). Each step's "Expected" outcome must match. Record any drift in a follow-up commit on this branch before merging. Manual quickstart is **not** a merge-blocker because T012–T014, T020–T021, and T028–T029 collectively cover every step against a real Docker daemon — the manual run is operational sanity, mirroring spec 010 T024's stance. — Skipped: integration scenarios authored in T012–T014, T020–T021, T028–T029 cover every quickstart step end-to-end against a real Docker daemon. Per the spec's own stance, this is operational sanity, not a merge gate.
- [X] T038 Final regression gate: from a clean checkout of branch `011-traefik-tlsport-config`, run `go build ./...`, `go test ./...`, and `make test-integration`. All three must pass green; no flakes accepted on this branch. — Locally green: `go build ./...` and `go test ./...` (full unit suite) both pass; `go build -tags integration ./tests/integration/...` compiles clean. The `make test-integration` portion is deferred to the CI pipeline (`.github/workflows/ci.yml:28` runs it on every push/PR) because cloud runs are materially faster than the ~15-minute local Docker-daemon run. Merge gate is the green CI run on the PR.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories. T002 (config field) is independent of T003+T004 (ConfigHash) and may land first or be split across PRs; T003 → T004 is a strict ordering (signature change before caller update).
- **User Story 1 (Phase 3, P1 MVP)**: Depends on Phase 2 completing both T002 and T003+T004. Internally: T005–T010 (unit tests) can start in parallel; T011 (testutils helper) blocks T012–T014 (integration tests that use the helper); T015–T017 (implementation) blocks the unit tests passing and the integration tests passing.
- **User Story 2 (Phase 4, P1)**: Depends on Phase 2 + US1 implementation (T015–T017) since US2 has no new code — its tests assert the no-`tlsPort` regression behaviour of the conditionals added in US1.
- **User Story 3 (Phase 5, P2)**: Depends on Phase 2 only for the test layer; the implementation tasks (T030–T033) can land in parallel with US1 implementation if staffed, but the integration tests (T028–T029) require US1's `tlsPort` plumbing to be live so the warning's preconditions can be exercised end-to-end.
- **Polish (Phase 6)**: Depends on US1 + US2 + US3 complete. T034 and T036 are independent and can land in parallel; T037 / T038 are gating sequential checks.

### Within Each User Story

- Tests come before implementation (TDD per Constitution Principle V; the user's auto-memory rule that integration tests are slow makes "write once, run sparingly" the default).
- Validation tests (T005–T008) are independent of port-binding tests (T009) and entrypoint-shape tests (T010); all five can be authored together.
- The testutils helper T011 must land before any integration test that uses it (T012, T014, T021, T028) — sequence accordingly.
- Implementation tasks T015 → T016 → T017 are loosely sequential (same `plugin.go` file for T015+T016; `config_gen.go` for T017). Different developers can split T015+T016 vs T017 if pairing.

### Parallel Opportunities

- T005 / T006 / T007 / T008 / T009 (US1 unit tests, all in `plugin_test.go` — different test functions): **safe to author together**.
- T010 (US1 entrypoint test, in `config_gen_test.go`): independent of the `plugin_test.go` group; **parallel with T005–T009**.
- T018 / T019 (US2 unit tests, different files): **parallel**.
- T022 / T023 / T024 / T025 / T026 / T027 (US3 unit tests, all in `config_gen_test.go`): **safe to author together** (different test functions).
- T030 / T031 (US3 implementation helpers, both in `config_gen.go`): NOT parallel — T031 calls T030.
- T032 (wiring) sequences after T030 + T031.
- T034 / T036 (polish): **parallel** — two distinct files.
- US1 and US3 integration test authoring can overlap if two developers split files; each scenario is a fresh `s.Test(...)` block in `traefik_plugin_test.go`.

---

## Parallel Example: User Story 1 unit tests

```bash
# Author US1 unit tests together (different test functions, two files):
Task: "TestPlugin_Validate_AcceptsValidTLSPort in plugin_test.go"
Task: "TestPlugin_Validate_RejectsTLSPortOutOfRange in plugin_test.go"
Task: "TestPlugin_Validate_RejectsTLSPortCollidesWithPort in plugin_test.go"
Task: "TestPlugin_Validate_RejectsTLSPortCollidesWithDashboardPort in plugin_test.go"
Task: "TestPlugin_PortBindings_IncludesTLS443_WhenTLSPortSet in plugin_test.go"
Task: "TestGenerateStaticConfig_AddsBareWebsecureEntryPoint_WhenTLSPortSet in config_gen_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Complete Phase 1 (T001).
2. Complete Phase 2 (T002 → T003 → T004).
3. Complete Phase 3 (T005–T010 in parallel → T011 → T012–T014 → T015 → T016 → T017).
4. **STOP and VALIDATE**: Run the US1 unit + integration tests; manually run [quickstart.md](./quickstart.md) Steps 1–6 and confirm `tlsPort: 443` produces a working host-to-container `443/tcp` mapping plus a bare `websecure` entrypoint.
5. At this point, the headline issue from GitHub #12 is resolved and shippable on its own.

### Incremental Delivery

1. Foundation (T001 → T002 → T003 → T004).
2. US1 (MVP) → ship → close GitHub issue #12 with the HTTPS-publish confirmation.
3. US2 → ship → backward-compat regression suite is in CI.
4. US3 → ship → preserved-file warning closes the FR-008 gap.
5. Polish (Phase 6) → AGENTS.md docs, doc comment update, manual quickstart sanity, final regression gate.

### Parallel Team Strategy

Single-developer feature; the constitution's "small, commit-able increments" rule applies. No parallel-team strategy needed. If two developers were available, the natural split is: Dev A owns Phases 2 + 3 (foundational + MVP); Dev B owns Phase 5 (US3 helpers) and Phase 6 polish, picking up after Dev A merges T002+T003+T004.

---

## Notes

- All tasks comply with the user's auto-memory rules: unit tests reuse the existing `lstatFn` / `readFileFn` injection points (no filesystem touches except T010, which is a documented exception inherited from existing `TestGenerateStaticConfig_*` patterns); integration tests run sparingly via `make test-integration` against a real binary and a real Docker daemon (Constitution Principle V).
- The `entryPoint` struct in `spec.go` is intentionally unchanged — its single `Address` field structurally guarantees that no `tls`, `certResolver`, or other TLS-config keys can leak into the generated `websecure` entrypoint, satisfying FR-010 / [research.md Decision 4](./research.md) at the type system level.
- The `state.ConfigHash` signature change in T003 is engine-wide (all containers) and one-time (recreate on first deploy after upgrade). This is documented in [plan.md Complexity Tracking](./plan.md) and [research.md Decision 3](./research.md). Reviewers should expect to see EVERY existing container recreate when this branch deploys.
- The new event name `gateway.config.tls_port_no_websecure` is the only new entry on the observer contract; reuse of `gateway.config.legacy_probe_error` for probe failures is intentional and documented in [contracts/observer-events.md](./contracts/observer-events.md).
- Validation rejections at `New()` time mean callers in `cmd/` get an error before any container or filesystem mutation happens — covered by T013's "no Traefik container created" assertion.
