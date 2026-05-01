# Contract: Observer Events for Dashboard Generation and Legacy-Block Warning

**Feature**: Fix Traefik Dashboard Generated in Static Config
**Audience**: Downstream consumers of `engine.Observer` events emitted by the Traefik plugin (terminal logger, structured deploy logs, future UI surfaces).

This is the only operator-visible contract this feature introduces. There is no public Go API change, no manifest schema change, no CLI flag change. The fix surfaces three new event names on the existing `engine.Observer` channel and depends on consumers not breaking when they are emitted.

## Event namespace

All events live under the existing `gateway.` namespace already used by the plugin. No new namespace is created.

## Events introduced

### `gateway.dashboard.generated`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | yes | Absolute filesystem path of the dashboard dynamic file that was just written. |

| Status | Emitted when |
|--------|-------------|
| `engine.StatusInfo` | The dashboard dynamic file was written for the first time on this deploy (file did not exist; configuration calls for a dashboard). |

### `gateway.dashboard.preserved`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | yes | Absolute filesystem path of the existing dashboard dynamic file that was left untouched. |

| Status | Emitted when |
|--------|-------------|
| `engine.StatusInfo` | The dashboard dynamic file already existed; it was preserved unchanged regardless of whether the dashboard-related Shrine config has changed since the previous deploy. |

### `gateway.config.legacy_http_block`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | yes | Absolute filesystem path of the pre-existing static config file (`traefik.yml`) containing the legacy `http:` block. |
| `hint` | string | yes | One-line, human-readable cleanup instruction (e.g., "Remove the top-level `http:` block from this file; the dashboard now lives in `<dynamic-dir>/__shrine-dashboard.yml`."). |

| Status | Emitted when |
|--------|-------------|
| `engine.StatusWarning` | The static `traefik.yml` is present and contains a top-level `http:` key (the artefact of an earlier buggy Shrine version). Emitted on every deploy where the block remains; emitted in addition to (not instead of) the relevant `gateway.config.generated` or `gateway.config.preserved` event. |

## Compatibility expectations

- **Consumers MUST treat unknown event names as ignorable**. The terminal logger already does this; new consumers added to the codebase MUST follow the same rule.
- **No existing event name is renamed or removed**. `gateway.config.generated`, `gateway.config.preserved`, `gateway.route.generated`, `gateway.route.preserved`, `gateway.route.orphan`, and `gateway.route.stat_error` continue to behave exactly as today.
- **Field set is additive-only**. Future feature work on the dashboard generator MAY add new fields to these events; consumers MUST tolerate unknown fields.

## Non-events (intentionally not emitted)

- There is no `gateway.dashboard.removed` event (FR-008 in the spec describes removal-on-config-removal but is out of scope for this fix's first iteration; if added later, the natural name is reserved here).
- There is no `gateway.dashboard.password_rotated` or similar event. Per Decision 3 in research.md, the dashboard dynamic file is preserved unconditionally; password rotation is operator-driven (delete-and-redeploy or hand-edit). The `gateway.dashboard.preserved` event is sufficient signal that the file was left as-is.

## Verification

Each event name above is asserted in the integration scenarios documented in plan.md (§ Integration tests). The terminal logger (`internal/ui/terminal_logger.go`) does not need a code change to render these events — it has a generic fall-through path for unknown event names that prints the status, name, and salient fields. A cosmetic per-event renderer can be added in a follow-up if desired but is not required by this contract.
