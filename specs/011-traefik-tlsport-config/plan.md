# Implementation Plan: Traefik Plugin `tlsPort` Config Option

**Branch**: `011-traefik-tlsport-config` | **Date**: 2026-05-01 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/011-traefik-tlsport-config/spec.md`

## Summary

Add an optional `tlsPort` integer field to the Traefik plugin configuration so a single `tlsPort: 443` line in `~/.config/shrine/config.yml` (a) publishes the chosen host port to container `443/tcp` on the platform Traefik container, and (b) declares a bare `websecure` entrypoint at `:443` in the generated `traefik.yml`. TLS-termination concerns (certificates, ACME, redirects, mTLS) are explicitly out of scope per FR-010 — the feature only opens the network path.

The implementation is three narrow changes:

1. **`internal/config/plugin_traefik.go`**: add the `TLSPort int \`yaml:"tlsPort,omitempty"\`` field to `TraefikPluginConfig`.
2. **`internal/plugins/gateway/traefik/`**: extend `Plugin.validate()` with the three port-collision/range checks (FR-005); extend `Plugin.portBindings()` to append `<tlsPort>:443/tcp` (FR-002); extend `generateStaticConfig` to emit the `websecure` entrypoint when generating fresh (FR-003); add a `hasWebsecureEntrypoint` probe + `gateway.config.tls_port_no_websecure` warning event for the preserved-file mismatch case (FR-008).
3. **`internal/state/deployments.go` + `internal/engine/local/dockercontainer/docker_container.go`**: extend `state.ConfigHash` to include port-binding specs so any port change (including `tlsPort` drift) triggers container recreation through the existing reconciliation path (FR-006).

A new testutils helper `AssertContainerPublishesPort` is added in `tests/integration/testutils/assert_docker.go` because no port-binding assertion exists today; six integration scenarios in `tests/integration/traefik_plugin_test.go` cover the user-story acceptance criteria. Unit tests in `internal/plugins/gateway/traefik/` are extended to cover the new entrypoint shape, validation rejections, and the `hasWebsecureEntrypoint` probe.

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: `gopkg.in/yaml.v3` (already used by the plugin); `github.com/docker/docker/api/types/container` + `github.com/docker/go-connections/nat` (already used by `dockercontainer.buildPortBindings`); `internal/engine.Observer` for the new warning event.
**Storage**: Filesystem only — `<routing-dir>/traefik.yml` (static) is bind-mounted into the Traefik container at `/etc/traefik`. No new files; no new state-store entries.
**Testing**: `go test ./...` for unit tests (no filesystem touches per project convention; reuses existing `lstatFn`/`readFileFn` injection points). `make test-integration` (build tag `integration`) for the end-to-end gate using `NewDockerSuite` against a live Docker daemon.
**Target Platform**: Linux server (homelab operator host running Docker).
**Project Type**: CLI (single Go module under `cmd/`, internal packages under `internal/`).
**Performance Goals**: N/A — config generation runs once per `shrine deploy` and is bounded by a single small YAML write plus one `ContainerInspect` call already in the engine path.
**Constraints**: Must remain idempotent across redeploys; must not modify operator-edited `traefik.yml` (FR-007); must not regress any existing Traefik-plugin test; the `ConfigHash` extension MUST keep the existing hash inputs in the same order so future regressions can be diagnosed by replaying old `Deployment.ConfigHash` values.
**Scale/Scope**: Two struct-field additions, four small helper additions / extensions, one new observer event name, one new testutils assertion helper, six integration scenarios, six unit-test additions. Estimated total delta: ~150 LOC of production code and ~250 LOC of test code across 6 production files and 4 test files.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] Pass — exposed as a single new YAML field `plugins.gateway.traefik.tlsPort` in the operator config (`~/.config/shrine/config.yml`). No new CLI flags. Validation rules are field-level and emit errors that name the offending field, consistent with the constitution's "field-level constraints MUST be enforced at parse/validate time" rule. |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] N/A — no new commands. Existing `shrine deploy --dry-run` continues to work because `tlsPort` flows through the same `Plugin.Deploy()` path that already short-circuits when the dry-run container backend is used (the dry-run backend prints the `CreateContainerOp` rather than executing it; the new port binding appears in the printed op). |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass — Traefik-specific logic (entrypoint generation, validation, host-port projection, websecure-probe warning) lives entirely inside `internal/plugins/gateway/traefik/`. The `state.ConfigHash` extension is engine-wide and explicitly is **not** Traefik-specific — it's a fix to the engine's reconciliation contract that happens to be required by FR-006; documented in research.md Decision 3 and the Complexity Tracking entry below. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — no new abstractions, interfaces, factories, or types. Re-uses existing `entryPoint` struct, existing `engine.PortBinding` shape, existing `Observer.OnEvent` channel, existing `lstatFn`/`readFileFn` injection points. The new `hasWebsecureEntrypoint` helper and `AssertContainerPublishesPort` testutils helper are direct analogues of existing siblings (`hasLegacyDashboardHTTPBlock` and `AssertContainerHasBindMount` respectively). |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass — six new scenarios in `tests/integration/traefik_plugin_test.go` cover all three user stories' acceptance scenarios. Listed in research.md Decision 9 and detailed in the Project Structure section below. |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] N/A — no new state-record fields. The existing `recordDeployment` call is unchanged in ordering and still runs after `ContainerCreate`/`ContainerStart` succeed; the only state-package change is the `ConfigHash` signature, which is a pure function. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | [x] Pass — `hasWebsecureEntrypoint` mirrors `hasLegacyDashboardHTTPBlock`'s `has*` boolean naming; new validation helpers (`isTLSPortOutOfRange`, `isTLSPortColliding`) follow the project's `is/has/should` convention. The new event-rendering branch in `terminal_logger.go` is one `case` clause, no comment needed. The `ConfigHash` signature extension is the only change to a widely-used helper; its existing doc comment is updated to mention `portSpecs`. |

> Violations MUST be documented in the Complexity Tracking table below.

**Result**: Six principles pass with no violations. One borderline scope decision (`ConfigHash` extension being engine-wide rather than plugin-local) is documented in Complexity Tracking below to make the trade-off legible — it is **not** a constitution violation but a deliberate scope inclusion worth surfacing.

## Project Structure

### Documentation (this feature)

```text
specs/011-traefik-tlsport-config/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output — 10 design decisions resolved
├── data-model.md        # Phase 1 output — schema delta + projection tables + state transitions
├── quickstart.md        # Phase 1 output — 8-step manual operator validation script
├── contracts/
│   └── observer-events.md  # Phase 1 output — single new event `gateway.config.tls_port_no_websecure`
├── checklists/
│   └── requirements.md  # Created by /speckit-specify
└── tasks.md             # Phase 2 output (created by /speckit-tasks)
```

### Source Code (repository root)

```text
internal/config/
├── plugin_traefik.go             # MODIFIED — add `TLSPort int \`yaml:"tlsPort,omitempty"\`` field
└── plugin_traefik_test.go        # UNCHANGED — existing tests cover ResolveRoutingDir only

internal/plugins/gateway/traefik/
├── plugin.go                     # MODIFIED — extend validate() with tlsPort range/collision rules; extend portBindings() to append <tlsPort>:443/tcp; thread tlsPort through Deploy() → generateStaticConfig
├── plugin_test.go                # NEW (or extend existing helpers_test.go) — unit tests for validate() rejections and portBindings() shape
├── config_gen.go                 # MODIFIED — add websecure entrypoint when cfg.TLSPort > 0; add hasWebsecureEntrypoint probe; emit gateway.config.tls_port_no_websecure when preserved + tlsPort + no websecure
├── config_gen_test.go            # MODIFIED — assertions for websecure entrypoint shape (only address; no tls keys); hasWebsecureEntrypoint probe; warning emission against in-memory YAML
├── spec.go                       # UNCHANGED — entryPoint struct already has only an Address field, satisfying the "no Shrine-injected tls keys" guarantee structurally
└── routing.go                    # UNCHANGED — per-app routing files don't change; they continue to mount on the existing `web` entrypoint by default

internal/state/
├── deployments.go                # MODIFIED — ConfigHash takes a new portSpecs []string arg, sorted internally
└── deployments_test.go           # MODIFIED — add coverage for portSpecs being part of the hash

internal/engine/local/dockercontainer/
├── docker_container.go           # MODIFIED — configHash() projects op.PortBindings into "<host>:<container>/<proto>" strings and forwards to state.ConfigHash
└── docker_container_test.go      # MODIFIED — assert configHash differs when only PortBindings changes

internal/ui/
└── terminal_logger.go            # MODIFIED — add `case "gateway.config.tls_port_no_websecure"` clause

tests/integration/
├── traefik_plugin_test.go        # MODIFIED — six new scenarios (clean tlsPort deploy, no-tlsPort regression, preserved file warning, collision rejection, drift recreate, removal)
└── testutils/
    └── assert_docker.go          # MODIFIED — new AssertContainerPublishesPort(name, hostPort, containerPort, proto) helper

AGENTS.md                          # MODIFIED — add tlsPort to the Traefik plugin config example block (FR-009)
```

**Structure Decision**: The feature lives in three layers — operator-facing config schema (`internal/config/`), plugin behavior (`internal/plugins/gateway/traefik/`), and engine-wide reconciliation (`internal/state/` + `internal/engine/local/dockercontainer/`). The plugin layer is where most of the change concentrates; the engine-wide change is a single helper-signature extension necessary to honor FR-006 (port-binding drift triggers recreation). No new top-level packages, no new directories. Test layout mirrors the existing two-tier split (unit tests beside production code with the project's no-filesystem-in-unit-tests convention; integration tests under `tests/integration/` with the `integration` build tag and the `NewDockerSuite` harness). Observer event names extend the existing `gateway.config.*` namespace rather than inventing a new one, keeping the contract for downstream UI/log consumers stable.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

The Constitution Check has no violations. The single borderline scope decision is documented here for transparency, not because it violates a principle:

| Item | Why included in this feature's scope | Simpler alternative rejected because |
|------|--------------------------------------|--------------------------------------|
| `state.ConfigHash` signature extension to include port-binding specs | FR-006 mandates that changing `tlsPort` between deploys recreates the container. The existing reconciliation path keys on `state.ConfigHash`, which today omits port bindings — meaning ANY port change (not just `tlsPort`) silently fails to trigger recreation. Fixing this at the engine layer in one place honors FR-006 correctly and incidentally fixes a latent bug for `port` and `dashboard.port` drift. | (a) Plugin-local drift detection inside `Plugin.Deploy()` would either need to extend `engine.ContainerInfo` with port bindings (cross-package surface area to add a single read path) or maintain a sidecar state file (new state surface, duplicates what `Deployment.ConfigHash` already exists for). (b) Always-recreate the Traefik container kills start-up grace and breaks parity with how every other container is reconciled. (c) Leaving FR-006 unhonored directly contradicts a P1 acceptance scenario. The one-time recreate-on-upgrade for all containers is acceptable for the homelab target audience because the recreate path is idempotent across bind mounts, named volumes, and networks. |
