# Spec: Decoupled Logging & UI (Observer Pattern)

## Status
Done

Shipped as designed below, with one naming difference: the terminal renderer is `TerminalObserver` in `internal/ui/terminal_logger.go`, not `TerminalUI`. Acceptance criteria verified against the code on 2026-10-05 (spec 030). Later features that touched the event stream and its renderers: `specs/027-app-ui-unit-coverage/`, `specs/028-fix-teardown-event-names/`.

## Goal

Decouple the user interface (CLI output, emojis, spinners) from the core deployment logic in `Engine` and `DockerBackend`. The engine remains platform-agnostic and testable without stdout side effects.

## Context

Currently `Engine` and `DockerBackend` call `fmt.Printf` directly. This ties UI rendering to business logic, makes the engine hard to test, and makes it impossible to swap rendering strategies (plain text, JSON, silent) without changing core code.

## Acceptance Criteria

- [x] `Engine` and `DockerBackend` emit no `fmt.Printf` calls — all output goes through an `Observer`
- [x] A `TerminalUI` implementation brings back the existing rich CLI output (emojis, spinner)
- [x] Spinner state is managed by `TerminalUI`, not by `DockerBackend`
- [x] Swapping `Observer` implementations (e.g. a silent no-op for tests) requires no changes to `Engine` or `DockerBackend`
- [x] All existing unit tests still pass

## Design

### Event stream (`internal/engine/events.go`)

```go
type Event struct {
    Name   string            // e.g. "image.pull", "container.create"
    Status string            // "started", "finished", "info", "error"
    Fields map[string]string // contextual data: "ref", "name", "owner", etc.
}

type Observer interface {
    OnEvent(e Event)
}
```

A data-driven event stream rather than a method-heavy interface keeps the engine decoupled from any rendering concern. The Observer decides how to render — the engine just emits named events with structured fields.

### `TerminalUI` (`internal/ui/terminal_logger.go`)

Responsibilities:
- Map event names to emojis (🚀, 🌐, 🏗️, …)
- Manage the stateful spinner: start on `"started"`, stop on `"finished"` or `"error"`
- Format and write to stdout/stderr

### Engine integration

- `Engine` and `DockerBackend` accept an `Observer` in their constructors (or via a setter)
- All `fmt.Printf` calls replaced with `obs.OnEvent(...)`
- Spinner logic currently inside `docker_backend.go` moves entirely to `TerminalUI`

## Implementation Order

1. **Scaffold** — create `internal/engine/events.go` with `Event` and `Observer` types
2. **Engine refactor** — update `engine.go` to emit high-level events (deploy started, step started/finished, error)
3. **Backend refactor** — update `docker_backend.go` to emit low-level events (image pull, container create/restart/recreate) and remove the inline spinner
4. **UI implementation** — create `TerminalUI` in `internal/ui/terminal_logger.go` that restores the rich aesthetics
5. **Wiring** — connect `TerminalUI` as the `Observer` in `cmd/deploy.go`

## Event Name Conventions

Use dot-separated namespaces: `<subsystem>.<operation>` (e.g. `image.pull`, `container.create`, `network.create`, `route.write`). The `Status` field carries the lifecycle: `started` → `finished` | `error`.

Event names are always lowercase. A name derived from a manifest kind uses the kind in lowercase (`application.teardown`, `resource.remove`), never the recorded form (`Application`): observers match names exactly, so a capitalised name is silently dropped by the terminal and stands out in the log.

Fields should use lowercase snake_case keys: `"image_ref"`, `"container_name"`, `"team"`.
