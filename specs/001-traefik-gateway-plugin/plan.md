# Implementation Plan: Traefik Gateway Plugin

**Branch**: `001-traefik-gateway-plugin` | **Date**: 2026-04-29 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `specs/001-traefik-gateway-plugin/spec.md`

## Summary

Add a self-contained Traefik gateway plugin to shrine that activates when `plugins.gateway.traefik` is present and non-empty in the shrine config file. When active, the plugin generates Traefik static + dynamic configuration files from deployed app routing definitions (apps with both `Routing.Domain` set and `ExposeToPlatform: true`), mounts the config directory into a Traefik container attached to the platform network, and implements the existing `RoutingBackend` interface so the engine's routing call-site needs no structural changes. The plugin lives entirely under `internal/plugins/gateway/traefik/` to support future extraction.

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: Docker SDK (`docker/docker`), Cobra, `gopkg.in/yaml.v3`
**Storage**: Filesystem — `routing-dir` (default `{specsDir}/traefik/`) for generated Traefik YAML files; shrine state store for Traefik container deployment record
**Testing**: `go test ./...` (unit); `go test -tags integration ./tests/integration/...` with `NewDockerSuite` (integration gate)
**Target Platform**: Linux server with live Docker daemon
**Project Type**: CLI plugin extension — no new top-level commands; config-driven activation
**Performance Goals**: Plugin deploy step completes in under the time a standard container pull+start takes; no throughput requirements on config generation
**Constraints**: Plugin package MUST be self-contained under `internal/plugins/gateway/traefik/` with no reverse imports from shrine core; `CreateContainerOp` requires two new optional fields (RestartPolicy, PortBindings) to support Traefik's needs

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | New capability exposed via `plugins.gateway.traefik` config section (YAML), not CLI flags | Pass |
| II. Kubectl-Style CLI | No new CLI commands; `shrine deploy --dry-run` must skip plugin deploy step | Pass |
| III. Pluggable Backend | Traefik implements `engine.RoutingBackend`; plugin lives in `internal/plugins/` not `internal/engine/local/` — justified in Complexity Tracking | Justified Deviation |
| IV. Simplicity & YAGNI | Plugin struct has exactly the operations it needs; no premature extension points | Pass |
| V. Integration-Test Gate | Integration test file created before implementation code (TDD per constitution) | Pass |
| VI. Docker-Authoritative State | Traefik container recorded in `DeploymentStore` only after `ContainerStart` succeeds | Pass |
| VII. Clean Code & Readability | Boolean helpers: `isActive()`, `hasDashboard()`, `hasCredentials()`; no WHAT comments | Pass |

## Project Structure

### Documentation (this feature)

```text
specs/001-traefik-gateway-plugin/
├── plan.md              ← this file
├── research.md          ← Phase 0 output
├── data-model.md        ← Phase 1 output
├── quickstart.md        ← Phase 1 output
└── tasks.md             ← Phase 2 output (/speckit-tasks)
```

### Source Code

```text
internal/
├── config/
│   └── config.go                 (extend Config: add Plugins, PluginsConfig, GatewayPluginsConfig,
│                                  TraefikPluginConfig, TraefikDashboardConfig)
├── engine/
│   └── backends.go               (extend CreateContainerOp: add RestartPolicy string,
│                                  BindMounts []BindMount, PortBindings []PortBinding;
│                                  add BindMount + PortBinding value types)
├── engine/local/dockercontainer/
│   └── docker_container.go       (handle RestartPolicy, BindMounts, PortBindings in
│                                  createFreshContainer / buildMounts)
├── engine/
│   └── engine.go                 (gate routing call on ExposeToPlatform: line ~164)
├── plugins/
│   └── gateway/
│       └── traefik/
│           ├── plugin.go         (Plugin struct; isActive, Validate, Deploy, RoutingBackend)
│           ├── config_gen.go     (generates traefik.yml static config + dynamic route files)
│           └── routing.go        (implements engine.RoutingBackend; WriteRoute, RemoveRoute)
└── handler/
    └── deploy.go                 (wire plugin: validate, create RoutingBackend, post-deploy Deploy)

tests/
└── integration/
    └── traefik_plugin_test.go    (TDD: written before implementation)

tests/testdata/deploy/
└── traefik/                      (fixture manifests for integration tests)
```

### Handler Integration Flow (deploy.go)

```
Deploy(opts)
  ├── planner.Plan(...)
  ├── validate plugin config  ← fail fast before any container work
  ├── create containerBackend (dockercontainer.NewDockerBackend)
  ├── create plugin (traefik.New(cfg.Plugins.Gateway.Traefik, containerBackend, specsDir))
  ├── if plugin.isActive(): routingBackend = plugin.RoutingBackend()
  ├── create engine (Engine{Container, Routing: routingBackend, ...})
  ├── engine.ExecuteDeploy(steps, set)   ← writes route files via RoutingBackend.WriteRoute
  └── if plugin.isActive(): plugin.Deploy()  ← deploy Traefik container
```

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|--------------------------------------|
| Principle III: RoutingBackend impl lives in `internal/plugins/` not `internal/engine/local/` | FR-007 requires plugin to be self-contained for future extraction to a separate repo | Moving to `internal/engine/local/traefik/` would split the plugin across packages, making extraction require changes to shrine core |
