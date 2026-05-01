# Phase 1 Contract — Per-App Routing File Log Events

**Date**: 2026-05-01

This contract defines the four log events emitted by the traefik routing backend's `WriteRoute` / `RemoveRoute` methods after this feature ships. The names follow the existing `gateway.config.*` convention from spec 004 (so a future log scraper can pattern-match the entire gateway-file family with one regex), and the field shapes mirror the existing events in the same package.

## Event Catalog

### `gateway.route.generated`

**Emitted by**: `WriteRoute` after a fresh write succeeds (the file was Absent before the call and `os.WriteFile` returned nil).

**Status**: `engine.StatusInfo`.

**Fields**:
| Field | Type | Source |
|---|---|---|
| `team` | string | `op.Team` |
| `name` | string | `op.ServiceName` |
| `path` | string | `filepath.Join(r.dynamicDir(), routeFileName(op.Team, op.ServiceName))` |

**Operator semantic**: "I just wrote this file from your current manifest. From now on it is yours."

### `gateway.route.preserved`

**Emitted by**: `WriteRoute` when the file already exists at the target path and the write was skipped.

**Status**: `engine.StatusInfo`.

**Fields**: identical to `gateway.route.generated` — `team`, `name`, `path`.

**Operator semantic**: "I left your file alone. If you recently changed the manifest for this app and expected something new, delete this file and redeploy."

**Compatibility note**: this event mirrors `gateway.config.preserved` (spec 004) byte-for-byte in field shape. Existing log scrapers that key on `gateway.config.preserved` will need a parallel key for `gateway.route.preserved` if they want to track per-app preserves; no existing key is renamed.

### `gateway.route.stat_error`

**Emitted by**: `WriteRoute` when `isPathPresent` returns a non-nil error (i.e., `Lstat` returned something other than success or `IsNotExist`).

**Status**: `engine.StatusWarning`.

**Fields**:
| Field | Type | Source |
|---|---|---|
| `team` | string | `op.Team` |
| `name` | string | `op.ServiceName` |
| `path` | string | per-app file path |
| `error` | string | the underlying `error.Error()` from the stat |

**Operator semantic**: "I couldn't tell whether your file exists. I treated it as present (did not write) so I wouldn't accidentally clobber operator state. The deploy continues — investigate the error."

**Critical contract point**: `WriteRoute` returns **nil** to the engine in this case (FR-007 / FR-011). The deploy is not aborted by this event.

### `gateway.route.orphan`

**Emitted by**: `RemoveRoute` when the per-app file is present at teardown time.

**Status**: `engine.StatusWarning`.

**Fields**:
| Field | Type | Source |
|---|---|---|
| `team` | string | `team` arg |
| `name` | string | `host` arg (which is the application name; see Decision 3 in `research.md`) |
| `path` | string | per-app file path |

**Operator semantic**: "You just tore down this app, but its routing file is still on disk and may keep routing traffic. Delete this path by hand to fully complete the teardown: `<path>`."

**Critical contract point**: `RemoveRoute` returns **nil** to the engine in this case (FR-009 / FR-011). The teardown is not aborted, and Shrine does not attempt the `os.Remove` call.

## Compatibility Guarantees

1. **No existing event is renamed or has its field set narrowed.** Specifically:
   - `gateway.config.preserved` (spec 004) and `gateway.config.generated` (spec 004) are unchanged.
   - `routing.configure` (engine.go:175, fields `domain`, `port`, `aliases`) is unchanged.

2. **Field shapes are stable.** Every event in this family has at minimum `team`, `name`, `path`. Warning events add `error` only when the error is the source of the warning (`gateway.route.stat_error` only; `gateway.route.orphan` is not error-driven).

3. **Status strings come from the existing `engine.Status*` constants.** No new status level is introduced.

## Worked Examples

### Example 1 — Repeat deploy of an unchanged manifest

```text
gateway.route.preserved  team=marketing  name=blog  path=/var/lib/shrine/gateway/dynamic/marketing-blog.yml
gateway.route.preserved  team=ops        name=api   path=/var/lib/shrine/gateway/dynamic/ops-api.yml
```

### Example 2 — First deploy of a new app on a host with one existing app

```text
gateway.route.preserved  team=ops        name=api   path=/var/lib/shrine/gateway/dynamic/ops-api.yml
gateway.route.generated  team=marketing  name=blog  path=/var/lib/shrine/gateway/dynamic/marketing-blog.yml
```

### Example 3 — Stat error on one app

```text
gateway.route.stat_error team=ops name=api   path=/var/lib/shrine/gateway/dynamic/ops-api.yml error=permission denied
gateway.route.generated  team=marketing  name=blog  path=/var/lib/shrine/gateway/dynamic/marketing-blog.yml
```

The deploy completes; exit code reflects the underlying Docker outcomes only (FR-011 / SC-007).

### Example 4 — Teardown of an app whose file the operator edited

```text
application.teardown     team=marketing  name=blog
container.remove         team=marketing  name=blog
gateway.route.orphan     team=marketing  name=blog  path=/var/lib/shrine/gateway/dynamic/marketing-blog.yml
```

The teardown succeeds (exit 0). Operator must `rm /var/lib/shrine/gateway/dynamic/marketing-blog.yml` to fully tear down the route.
