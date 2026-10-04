# Contract: Teardown Observer Events

**Feature**: Teardown Headers That Print and Teardown Event Names That Match the Rest of the Log (`028-fix-teardown-event-names`)
**Audience**: Consumers of `engine.Observer` events emitted by the engine during `shrine teardown <team>` — the terminal logger, the file logger, and anyone reading or filtering `<state>/logs/shrine.log`.

This is the only operator-visible contract this feature changes. No public Go API change, no manifest schema change, no CLI flag or help-text change, no state format change.

## Events renamed

| Old name (no longer emitted) | New name | Status | Fields |
|------------------------------|----------|--------|--------|
| `Application.teardown` | `application.teardown` | `started` | `team`, `name` |
| `Resource.teardown` | `resource.teardown` | `started` | `team`, `name` |
| `Application.remove` | `application.remove` | `error` | `team`, `name`, `error` |
| `Resource.remove` | `resource.remove` | `error` | `team`, `name`, `error` |
| `Application.routing_remove` | `application.routing_remove` | `error` | `team`, `name`, `error` |

Only the name changes. Status, fields, field values, emission order, and the number of events are identical.

### `application.teardown` / `resource.teardown`

| Status | Emitted when |
|--------|-------------|
| `engine.StatusStarted` | Once per deployment recorded for the team, immediately before the engine asks the container backend to remove that deployment's container. Applications first (name-sorted), then resources (name-sorted). Never emitted for a team with no recorded deployments. |

### `application.remove` / `resource.remove`

| Status | Emitted when |
|--------|-------------|
| `engine.StatusError` | The container backend returned an error removing the deployment's container. `error` holds `<Kind> "<name>": <cause>` with the kind in its recorded, capitalised form (unchanged). The teardown stops; the command exits non-zero. |

### `application.routing_remove`

| Status | Emitted when |
|--------|-------------|
| `engine.StatusError` | The application's container was removed but the routing backend returned an error removing its route. `error` holds `Application "<name>" routing: <cause>` (unchanged). The teardown stops; the command exits non-zero. There is no `resource.routing_remove`: resources have no routes. |

## Operator-visible surface

### Terminal (stdout)

| Event | Line |
|-------|------|
| `application.teardown` started | `🗑️  Tearing down Application: <name> (team: <team>)` — **newly visible** |
| `resource.teardown` started | `🗑️  Tearing down Resource: <name> (team: <team>)` — **newly visible** |
| `application.remove` error | `  ❌ Error [application.remove]: Application "<name>": <cause>` — was `[Application.remove]` |
| `resource.remove` error | `  ❌ Error [resource.remove]: Resource "<name>": <cause>` — was `[Resource.remove]` |
| `application.routing_remove` error | `  ❌ Error [application.routing_remove]: Application "<name>" routing: <cause>` — was `[Application.routing_remove]` |

The header text itself is not new: the terminal logger has always rendered it for the lowercase names. It is the engine that now emits those names.

### Log file (`<state>/logs/shrine.log`)

```text
<timestamp> [started] application.teardown name="whoami" team="shrine-deploy-test"
<timestamp> [started] resource.teardown name="shared-cache" team="shrine-teardown-a"
<timestamp> [error] application.remove error="Application \"whoami\": <cause>" name="whoami" team="shrine-deploy-test"
```

Line grammar and field ordering are unchanged (spec 027).

## Non-changes (intentionally)

- **No alias.** The capitalised names are not emitted alongside the lowercase ones, and there is no compatibility switch.
- **No log rewrite.** The log is append-only; entries written by earlier versions keep their capitalised names.
- **No change to backend events.** `container.remove` (`started` / `finished` / `info reason="not found"` / `error`), `network.remove`, and `routing.finalize` are emitted exactly as before, including the backend's own `container.remove` error line that precedes the engine's `<kind>.remove` line on a failed removal.
- **No change to deploy events.** `application.deploy`, `resource.deploy`, and every other deploy-path name were already lowercase literals.
- **No change to error prose or exit codes.** The `error` field and the error returned to the command keep the capitalised kind.
- **No change to state.** `Deployment.Kind` is still recorded as `Application` / `Resource`.

## Compatibility expectations

- A consumer that filters the log on `Application.teardown`, `Resource.teardown`, `Application.remove`, `Resource.remove`, or `Application.routing_remove` MUST switch to the lowercase names for entries written after this change. No such consumer exists in this repository.
- A consumer that already filters on the lowercase names (the documented convention since spec 009) starts matching teardown entries it previously missed.
- Event names are lowercase, dot-separated `<subsystem>.<operation>`; a name derived from a manifest kind uses the kind in lowercase. Future events MUST follow the same rule.

## Verification

- Unit — emitter: `TestEngine_ExecuteTeardown_AnnouncesEachDeploymentBeforeRemovingIt` and `TestEngine_ExecuteTeardown_NamesFailureEventsInLowercase` in `internal/engine/engine_test.go` (exact interleaved timeline of events and backend calls; the three failure names; unchanged error text).
- Unit — handler wiring: `TestTeardown_RemovesPlannedDeploymentsThenNetwork` and `TestTeardown_StopsAtFirstRemovalFailure` in `internal/handler/teardown_test.go` assert the lowercase names for deployments recorded with the capitalised kind.
- Unit — renderer (existing, unchanged): the terminal catalogue in `internal/ui/terminal_logger_test.go` pins the header lines for the lowercase names.
- Integration (CI executes): `TestTeardown` asserts `Tearing down Application: whoami (team: shrine-deploy-test)` on stdout; `TestTeardownMultiTeam` asserts `Tearing down Resource: shared-cache (team: shrine-teardown-a)`; `TestFileLogger` asserts `[started] application.teardown name="whoami" team="shrine-deploy-test"` in the log.
