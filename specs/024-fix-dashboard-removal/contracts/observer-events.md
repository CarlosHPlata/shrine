# Contract: Observer Event for Dashboard Removal

**Feature**: Remove Stale Dashboard Config on Dashboard Removal (`024-fix-dashboard-removal`)
**Audience**: Downstream consumers of `engine.Observer` events emitted by the Traefik plugin (terminal logger, file logger, future UI surfaces).

This is the only operator-visible contract this feature introduces. No public Go API change, no manifest schema change, no CLI change. It fulfils the reservation made in spec 010's observer-events contract ("There is no `gateway.dashboard.removed` event … if added later, the natural name is reserved here").

## Event introduced

### `gateway.dashboard.removed`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | yes | Absolute filesystem path of the dashboard dynamic file that was just deleted. |

| Status | Emitted when |
|--------|-------------|
| `engine.StatusInfo` | The gateway plugin is active, the configuration no longer calls for a dashboard, a previously generated `__shrine-dashboard.yml` existed, and its deletion succeeded. Emitted at most once per deploy, after the deletion. |

## Non-events (intentionally not emitted)

- **No event when there is nothing to remove.** A deploy without a dashboard and without a stale file is silent (spec FR-004/SC-006).
- **No `gateway.dashboard.remove_error` event.** A failed stat or deletion fails the deploy with a wrapped error naming the path and cause (`traefik plugin: removing stale dashboard dynamic file at <path>: <cause>`); it does not degrade to a warning event. See research.md Decision 3.
- **No dry-run variant.** Dry-run deploys use the print-only routing backend; the removal is subsumed by the existing `[ROUTE]  Finalize` line, matching how dashboard generation is (not) itemized in dry-run today.

## Compatibility expectations

- Consumers MUST treat unknown event names as ignorable; the terminal logger's generic fall-through renders this event without a code change.
- No existing event is renamed, removed, or re-shaped: `gateway.dashboard.generated`, `gateway.dashboard.preserved`, `gateway.config.*`, and `gateway.route.*` behave exactly as today.
- Field set is additive-only for future revisions; consumers MUST tolerate unknown fields.

## Verification

- Unit: `recordingObserver` assertions in `internal/plugins/gateway/traefik/routing_test.go` (event name, `StatusInfo`, `path` field; zero events on the nothing-to-remove path).
- Integration: the remove-dashboard-redeploy scenario in `tests/integration/traefik_plugin_test.go` asserts the file is gone after the second deploy and the deploy output reports the removal.
