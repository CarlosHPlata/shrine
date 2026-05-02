# Research: Traefik Plugin `tlsPort` Config Option

**Feature**: 011-traefik-tlsport-config
**Phase**: 0 (Outline & Research)
**Date**: 2026-05-01

This document resolves the open design questions for the `tlsPort` config option before Phase 1 design begins. There were no `NEEDS CLARIFICATION` markers in the Technical Context; the items below are decisions that emerged from reading the existing Traefik plugin (`internal/plugins/gateway/traefik/`), the engine container backend (`internal/engine/local/dockercontainer/`), and the integration-test harness (`tests/integration/`).

## Decision 1 — Where to add the `tlsPort` field

**Decision**: Add `TLSPort int \`yaml:"tlsPort,omitempty"\`` to `config.TraefikPluginConfig` in `internal/config/plugin_traefik.go`, sibling to the existing `Port` field.

**Rationale**: `TraefikPluginConfig` is the single canonical YAML schema for the plugin. The existing `Port` field uses `yaml:"port,omitempty"`; mirroring that pattern with `tlsPort` keeps the operator-facing YAML camelCase and the Go field exported as `TLSPort` (Go's MixedCaps convention for the acronym). `omitempty` so unset configs round-trip identically (US 2 / FR-004).

**Alternatives considered**:
- Nest under a new `tls:` block (e.g., `traefik.tls.port`). Rejected — the issue's proposed YAML shape is a flat top-level `tlsPort`, and nesting would need additional fields to justify it (we have none — see FR-010 / Clarification §1).
- Reuse `Dashboard`-style `*TraefikTLSConfig` pointer struct. Rejected — same YAGNI reason; one optional integer doesn't need a pointer struct.

## Decision 2 — How to publish host port → container `443/tcp`

**Decision**: Extend `Plugin.portBindings()` in `plugin.go` to append `{HostPort: tlsPort, ContainerPort: "443", Protocol: "tcp"}` when `cfg.TLSPort > 0`. Container port `443` is hardcoded on the container side.

**Rationale**: This mirrors exactly how `port` works today (`{HostPort: <port>, ContainerPort: <port>, Protocol: "tcp"}` for the HTTP entrypoint, with HostPort and ContainerPort coincidentally equal because port 80 is what Traefik historically listened on by default). For HTTPS, Traefik's canonical entrypoint port is 443; only the host side is operator-configurable. The `engine.PortBinding{HostPort, ContainerPort, Protocol}` shape already supports asymmetric host/container ports — `buildPortBindings()` in `dockercontainer/docker_container.go` does not assume they are equal.

**Alternatives considered**:
- Make container port operator-configurable (e.g., `tlsContainerPort`). Rejected — out of scope per spec assumption "container port `443/tcp` is the canonical Traefik HTTPS entrypoint port and is hardcoded on the container side".
- Bind `udp` for HTTP/3. Rejected — the issue only mentions TCP/HTTPS; HTTP/3 is a future feature.

## Decision 3 — Container recreation on `tlsPort` drift (FR-006)

**Decision**: Extend `state.ConfigHash()` in `internal/state/deployments.go` to take port-binding specs alongside its existing inputs (image digest, env, volumes, ExposeToPlatform). Update its sole caller `dockercontainer.configHash()` to feed sorted port-binding specs into the hash. With this in place, Shrine's existing reconciliation path (`isContainerUpToDate` → `removeStaleContainer` → `createFreshContainer`) recreates the Traefik container automatically when `tlsPort` (or any other published port) changes between deploys.

**Rationale**: Today `configHash` deliberately omits port bindings; this is a latent correctness bug — changing `port` or `dashboard.port` between deploys also fails to trigger recreation today, leading to the same class of stuck-binding confusion as the issue describes. Extending the hash fixes the bug at the engine layer rather than special-casing Traefik. The change is small (one new helper arg, threaded through one caller). Plugin-local drift detection was the alternative; rejected because it would duplicate inspection logic the engine already owns.

**Operational consequence**: On the first `shrine deploy` after this release, every existing container's stored `ConfigHash` will mismatch the recomputed hash, triggering a one-time recreate of all running containers. This is acceptable for the homelab target audience: containers re-create idempotently, named volumes persist, bind mounts persist, networks persist. Documented in the quickstart and called out in the release note this feature ships under.

**Alternatives considered**:
- Plugin-local drift detection inside `Plugin.Deploy()`. Rejected — would require either extending `engine.ContainerInfo` with port bindings (cross-package surface area) or having the plugin keep its own sidecar state file (new state surface). Both are larger than the engine-level fix.
- Always force-recreate the Traefik container on every `Deploy()`. Rejected — heavy-handed; kills container start-up grace and breaks parity with how every other container is reconciled.
- Leave FR-006 unhonored, document as a known limitation. Rejected — directly contradicts a P1 user-story requirement.

## Decision 4 — `websecure` entrypoint shape in generated `traefik.yml`

**Decision**: When `cfg.TLSPort > 0` and `traefik.yml` is being Shrine-generated (not preserved), `generateStaticConfig` adds `spec.EntryPoints["websecure"] = entryPoint{Address: ":443"}`. No `tls`, `http.tls`, `certResolver`, `forwardedHeaders`, or any other key is set on the entrypoint.

**Rationale**: Per FR-010 / Clarification §1, Shrine's responsibility is exactly the entrypoint declaration and the host port binding — nothing more. The existing `entryPoint` struct in `spec.go` already has only an `Address` field, so this falls out automatically; no struct change is needed and there is no risk of accidentally injecting TLS-config keys via Shrine.

**Alternatives considered**:
- Set `tls: {}` on the entrypoint to make it TLS-by-default. Rejected per FR-010 — TLS termination is operator-owned.
- Add `http.redirections.entryPoint` for automatic HTTP→HTTPS redirect. Rejected per FR-010.

## Decision 5 — Validation rules for `tlsPort`

**Decision**: Extend `Plugin.validate()` in `plugin.go` to enforce, when `cfg.TLSPort != 0`:
1. `1 ≤ cfg.TLSPort ≤ 65535` — out-of-range is rejected with `"traefik plugin: tlsPort %d is out of range (1-65535)"`.
2. `cfg.TLSPort != cfg.Port` (using `resolvedPort()` so a default-80 vs. explicit-80 is treated as a collision) — rejected with `"traefik plugin: tlsPort %d collides with port %d"`.
3. `cfg.Dashboard == nil || cfg.TLSPort != cfg.Dashboard.Port` — rejected with `"traefik plugin: tlsPort %d collides with dashboard.port %d"`.

Validation runs at `New()` construction time (existing convention — `New` returns `(plugin, error)` so callers don't have a separate `Validate` step). All three errors include the offending field name to satisfy FR-005 ("Validation errors MUST identify the offending field by name").

**Rationale**: Co-locating validation with the existing dashboard-credentials check keeps it under one helper. Multi-error reporting (Constitution Principle I — "Validation MUST produce multi-error reports") is preserved by the existing manifest-validation pipeline; the plugin's `validate()` is fail-fast on the first port issue, but only because the three checks are mutually exclusive on a single integer field — no value of `tlsPort` can simultaneously be out-of-range AND collide.

**Alternatives considered**:
- Validate at YAML-parse time inside `internal/config/`. Rejected — the existing convention is for `Port`/`Dashboard` validation to live in `Plugin.validate()`, not in the config package.
- Treat default port (80) as exempt from collision (i.e., let `tlsPort: 80` work if `port` is unset). Rejected — confusing for operators; the resolved port is what the container will actually publish, so collision detection uses the resolved value.

## Decision 6 — Warning when preserved `traefik.yml` lacks `websecure` entrypoint (FR-008)

**Decision**: Add a probe `hasWebsecureEntrypoint(path string) (bool, error)` modelled on the existing `hasLegacyDashboardHTTPBlock` helper in `config_gen.go`. It unmarshals the static config into a minimal probe struct and returns whether `entryPoints.websecure.address` is `":443"` (or any address, since operators may use a non-default container-side port — but FR-002 fixes container side at 443, so the probe checks for any `entryPoints.websecure` key). When `cfg.TLSPort > 0` AND the static file is preserved AND the probe returns false, emit a new event `gateway.config.tls_port_no_websecure` (status `StatusWarning`) with `path` and `hint` fields.

The warning is emitted on every deploy where the mismatch persists (idempotent advisory, mirroring `gateway.config.legacy_http_block`). The deploy still succeeds — the host port mapping is still applied per FR-007.

**Rationale**: Mirrors the existing `gateway.config.legacy_http_block` precedent: Shrine never modifies operator-preserved files, but it surfaces a clear cleanup nudge so the operator doesn't see a confusing partial-success (port bound on host, but Traefik rejects the connection because no entrypoint exists at `:443`). Idempotent emission means the warning isn't lost if the operator misses the first deploy's output.

**Alternatives considered**:
- Block the deploy when the mismatch is detected. Rejected — operators may have legitimate transitional states, and blocking is hostile when the warning carries the same diagnostic value.
- Emit only on first detection (track in state). Rejected — adds state surface; idempotent emission is simpler and matches `legacy_http_block`.
- Probe by string-matching `"websecure"` in the file. Rejected — fragile against YAML formatting; structured unmarshal into a probe struct is the established pattern.

## Decision 7 — Removal behavior when `tlsPort` is unset between deploys

**Decision**: When `cfg.TLSPort == 0` and Shrine is generating `traefik.yml` (not preserving), the generated file simply omits the `websecure` entrypoint — no special "remove" logic needed. When the file is operator-preserved, Shrine leaves it alone (FR-007). The container is recreated automatically by Decision 3's hash extension because the port-binding set differs.

**Rationale**: Static-config generation is regeneration-from-config-each-time, not stateful diffing. Removing `tlsPort` from the operator's config naturally produces a generated file without the entrypoint; no separate removal code path is needed. This mirrors how dropping the dashboard configuration produces a generated file without the `traefik` entrypoint today.

**Alternatives considered**:
- Track `tlsPort` history and emit a `gateway.config.websecure_removed` event. Rejected — YAGNI; the existing `gateway.config.generated` event is sufficient signal to operators inspecting deploy output.

## Decision 8 — Observer event names

**Decision**: One new event name introduced: `gateway.config.tls_port_no_websecure` (warning, fields: `path`, `hint`). No existing event name is renamed or removed. The terminal logger gets a new `case` clause to render the warning with the `⚠️` prefix already used by sibling warnings.

**Rationale**: Tight contract surface — operators and downstream tooling get exactly one new event to know about. The name follows the existing `gateway.config.<noun>_<verb-or-state>` convention (`legacy_http_block`, `legacy_probe_error`).

**Alternatives considered**:
- Reuse `gateway.config.legacy_http_block`. Rejected — different semantic ("legacy" implies obsolete artefact; the missing-websecure case is forward-looking).
- Introduce a `gateway.tls.*` namespace. Rejected — premature; one event doesn't justify a namespace.

## Decision 9 — Test strategy split

**Decision**:
- **Unit tests** (no filesystem touches per project convention; lstat/readFile via injected test fakes): three new files of additions to `config_gen_test.go` and `plugin_test.go` (latter created if missing for `New`/`validate`/`portBindings`). Coverage: `tlsPort` set/unset → entrypoint shape; validation collisions (3 cases); `portBindings()` output shape; `hasWebsecureEntrypoint` probe true/false/parse-error.
- **Integration tests** (`tests/integration/traefik_plugin_test.go`, build tag `integration`): six scenarios using `NewDockerSuite` against a real binary and a live Docker daemon — clean deploy with `tlsPort`, deploy without `tlsPort` (regression guard), preserved `traefik.yml` + `tlsPort` warning, validation rejection on collision, `tlsPort` change between deploys recreates container, container port-binding inspection asserts `443/tcp` is mapped.

A new testutils helper `AssertContainerPublishesPort(name, hostPort, containerPort, proto)` (in `tests/integration/testutils/assert_docker.go`) is added because no equivalent exists today; it inspects the container via the existing `tc.DockerClient` and reads `HostConfig.PortBindings`. The helper is reusable for any future feature touching port publishing.

**Rationale**: Constitution Principle V mandates the integration-test gate; Principle VII mandates test-helper extraction over inline duplication. The user-memory feedback (`feedback_unit_tests_no_filesystem.md`) is honored by reusing the existing `lstatFn`/`readFileFn` injection points. The `feedback_integration_test_isolation.md` memory is honored by NOT reusing internal-package types in the integration tests (assertions go through `tc.DockerClient.ContainerInspect`).

## Decision 10 — Documentation surface (FR-009)

**Decision**: Update `AGENTS.md` (the project's canonical operator/AI quick-reference) to add `tlsPort` to the Traefik plugin config example. Add an entry to `quickstart.md` (this feature's quickstart). No changes to README/docs are required — there is no top-level `docs/` directory in this project today.

**Rationale**: AGENTS.md is the operator-visible config reference per the constitution; keeping it synchronized with CLI/config changes is mandated. The quickstart is the per-feature operator validation script.

**Alternatives considered**:
- Add a new top-level `docs/plugins/traefik.md`. Rejected — premature; AGENTS.md is the established surface.

---

**Output**: All decisions resolved. No outstanding `NEEDS CLARIFICATION`. Phase 1 design proceeds with the entity sketch in `data-model.md`, the observer-events contract in `contracts/observer-events.md`, and the operator validation script in `quickstart.md`.
