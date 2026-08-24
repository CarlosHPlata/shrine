# Data Model: Remove Stale Dashboard Config on Dashboard Removal

**Feature**: `024-fix-dashboard-removal` | **Date**: 2026-08-23

This feature introduces no new persisted state, no manifest schema changes, and no Go API changes outside the `traefik` package. The "data" is the lifecycle of one generated file and the observer events that narrate it.

## Entity: Dashboard dynamic config file

| Attribute | Value |
|-----------|-------|
| Path | `<routingDir>/dynamic/__shrine-dashboard.yml` (`dashboardDynamicFileName()`, `config_gen.go:208`) |
| Owner | Shrine (reserved `__shrine-` prefix; never operator-authored) |
| Written when | Gateway plugin active AND `hasDashboard()` (dashboard block present with `port > 0`) AND file absent at Finalize |
| Preserved when | `hasDashboard()` AND file present at Finalize |
| **Removed when (NEW)** | Gateway plugin active AND NOT `hasDashboard()` AND file present at Finalize |
| Contents | basicAuth middleware (`dashboard-auth`) + router to `api@internal` on the `traefik` entrypoint |

## Lifecycle state machine (evaluated once per deploy, in `RoutingBackend.Finalize`)

State = (dashboard configured?, file present?). Transitions:

| # | Dashboard configured | File present | Action | Resulting state | Event |
|---|---------------------|--------------|--------|-----------------|-------|
| 1 | yes | no | generate file | present | `gateway.dashboard.generated` |
| 2 | yes | yes | preserve unchanged | present | `gateway.dashboard.preserved` |
| 3 | **no** | **yes** | **remove file (NEW)** | **absent** | **`gateway.dashboard.removed`** |
| 4 | no | no | nothing | absent | none |
| — | any | stat error | fail deploy | unchanged | none (wrapped error) |
| — | no | yes, remove fails | fail deploy | unchanged (file survives, loudly) | none (wrapped error) |

Transitions 1, 2, and the stat-error row are existing behavior (spec 010). Transition 3 and the remove-fails row are this feature. Transition 4 today is implicit (the `hasDashboard` guard skips generation); it becomes an explicit probe-then-no-op with no event (spec FR-004).

Row 3 also affects the container: `RoutingBackend.portBindings()` no longer includes the dashboard port, so the recreated Traefik container stops publishing it. The preserved static `traefik.yml` may still declare the `traefik` entrypoint (static-config preservation is spec 004's contract and is out of scope here); with the router file gone and the host port unpublished, the dashboard is unreachable regardless.

## Validation rules

Unchanged. `traefik.New` still rejects `dashboard.port` without credentials, so "dashboard configured" is always a fully-credentialed dashboard by the time Finalize runs.

## Removed code (negative delta)

| Symbol | Location | Fate |
|--------|----------|------|
| `Plugin.portBindings()` | `plugin.go:128-142` | deleted — dead duplicate of `RoutingBackend.portBindings()` |
| `TestPlugin_PortBindings_*` | `plugin_test.go:117,142` | repointed to `RoutingBackend` in `routing_test.go` |
| `removeFileFn` | `routing.go:20` | kept — becomes live (sole production call site: dashboard removal) |
