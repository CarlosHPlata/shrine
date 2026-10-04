# Data Model: Teardown Headers That Print and Teardown Event Names That Match the Rest of the Log

**Feature**: `028-fix-teardown-event-names` | **Date**: 2026-10-03

No persisted schema changes: `state.Deployment`, `planner.PlannedStep`, `engine.Event`, and the log line grammar are all unchanged. This document captures the in-memory concepts the fix touches and the rules that govern them.

## Entities

### Deployment kind (existing — unchanged)

| Where | Value | Used for |
|-------|-------|----------|
| `manifest.ApplicationKind` / `manifest.ResourceKind` | `"Application"` / `"Resource"` | The recorded form |
| `state.Deployment.Kind` | recorded form | Written by the container backend after a successful create |
| `planner.PlannedStep.Kind` | recorded form | Teardown order (applications, then resources); route-removal gate |

The recorded form stays capitalised everywhere. Existing state needs no migration.

### Teardown event prefix (new — a local in `engine.teardownKind`)

| Name | Derivation | Used for |
|------|------------|----------|
| `eventPrefix` | `strings.ToLower(kind)` | The first segment of the three teardown event names — and nothing else |

### Teardown events (existing `engine.Event` values — names change)

| Emitted when | Status | Fields | Name before | Name after |
|--------------|--------|--------|-------------|------------|
| A deployment's teardown starts, before its container is removed | `started` | `team`, `name` | `Application.teardown` / `Resource.teardown` | `application.teardown` / `resource.teardown` |
| Container removal fails | `error` | `team`, `name`, `error` | `Application.remove` / `Resource.remove` | `application.remove` / `resource.remove` |
| Route removal fails (applications only, routing backend present) | `error` | `team`, `name`, `error` | `Application.routing_remove` | `application.routing_remove` |

Statuses, field sets, field values, and emission points are unchanged.

## What the kind feeds, before and after

| Consumer of the kind in `teardownKind` | Value used before | Value used after |
|----------------------------------------|-------------------|------------------|
| Event names (`.teardown`, `.remove`, `.routing_remove`) | recorded (`Application`) | **lowercased (`application`)** |
| Route-removal gate `step.Kind == manifest.ApplicationKind` | recorded | recorded (unchanged) |
| Error text `<Kind> "<name>": <cause>` / `<Kind> "<name>" routing: <cause>` | recorded | recorded (unchanged) |

## Teardown event sequence

For team `t` with planned steps `[{Application, "web"}, {Resource, "db"}]` (the planner's order):

| # | Engine action | Event emitted | Terminal line |
|---|---------------|---------------|---------------|
| 1 | — | `application.teardown` started `{team: t, name: web}` | `🗑️  Tearing down Application: web (team: t)` |
| 2 | `Container.RemoveContainer({t, web})` | (backend's own `container.remove` events) | removal progress lines |
| 3 | `Routing.RemoveRoute(t, web)` — only if a routing backend is present | — | — |
| 4 | — | `resource.teardown` started `{team: t, name: db}` | `🗑️  Tearing down Resource: db (team: t)` |
| 5 | `Container.RemoveContainer({t, db})` | (backend's own `container.remove` events) | removal progress lines |
| 6 | `finalizeRouting`, then `Container.RemoveNetwork(t)` | `routing.finalize`, `network.remove` (unchanged) | unchanged |

Failure at step 2 or 5 → `<prefix>.remove` error, run stops. Failure at step 3 → `application.routing_remove` error, run stops. Later steps emit nothing.

## Invariants

1. **Lowercase names**: every event name emitted during a teardown equals its own lowercase form (FR-005).
2. **One name per event**: each teardown event is emitted once, under the lowercase name only (FR-006).
3. **Announce, then act**: a deployment's `started` event is emitted before the backend is asked to remove its container (FR-001).
4. **One header per step**: the number of `*.teardown` started events equals the number of planned steps reached; zero steps → zero such events (FR-002).
5. **Recorded kind is read-only here**: the fix never writes, rewrites, or compares a lowercased kind (FR-008).
6. **Deploy untouched**: no deploy-path event name, field, or line changes (FR-009).

## State transitions

None. Teardown's effect on Docker and on state is identical before and after.
