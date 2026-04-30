# Phase 1 Data Model: Preserve Operator-Edited traefik.yml

This feature does not introduce a new manifest kind, store record, or persistent entity. The "data model" is the implicit state of the gateway routing directory on disk and the observer event emitted to describe what Shrine did with it.

## Entities

### `traefik.yml` (filesystem entry)

Not a Go struct — a path on disk. Captured here for completeness because the entire feature is decided by its presence/absence.

| Property | Type | Source | Notes |
|----------|------|--------|-------|
| `path` | string | `filepath.Join(routingDir, "traefik.yml")` | `routingDir` resolved by `Plugin.resolvedRoutingDir()` |
| `present` | bool | `os.Lstat(path)` returns no error | `os.Lstat`, not `os.Stat` — symlinks (broken or otherwise) and non-regular files all count as present |
| `statErr` | error | non-`IsNotExist` failure of `os.Lstat` | If present, deploy fails per FR-007 |

**Lifecycle**:
- **Absent** → Shrine writes the default-generated content via `os.WriteFile(..., 0o644)`, then `present=true` for all subsequent deploys.
- **Present** → Shrine performs no write. Mode, owner, mtime, and content are unchanged.
- **Stat error (other than NotExist)** → Shrine fails the deploy with a wrapped error naming the path and the cause.

No state transitions are performed by Shrine other than the absent→present flip on first deploy. Operators perform the present→absent transition by deleting the file; that is User Story 3, and it falls out of the same logic.

### `engine.Event` (observer signal — existing struct, new event names)

```go
type Event struct {
    Name   string                  // new: "gateway.config.preserved" | "gateway.config.generated"
    Status EventStatus             // engine.StatusInfo for both
    Fields map[string]string       // {"path": "<absolute path to traefik.yml>"}
}
```

| Field | Value | Why |
|-------|-------|-----|
| `Name` | `"gateway.config.preserved"` or `"gateway.config.generated"` | Two discrete event names so the terminal logger can render distinct messages and the file logger can be grepped per-policy |
| `Status` | `engine.StatusInfo` | Matches the constitution wording ("info level or equivalent" in FR-006); not a started/finished pair (the action is atomic) |
| `Fields["path"]` | absolute path to `traefik.yml` | Lets operators verify *which* file was preserved without scanning the whole deploy log; mirrors the `name` field used by container events |

No `Started`/`Finished` pairing — both events represent atomic, point-in-time observations after the existence probe.

### `Plugin` (existing struct — one new field)

```go
type Plugin struct {
    cfg      *config.TraefikPluginConfig
    backend  engine.ContainerBackend
    specsDir string
    observer engine.Observer        // NEW: receives info events; never nil (NoopObserver default)
}
```

**Validation**: `New(...)` accepts a `nil` observer at the type level but normalizes it to `engine.NoopObserver{}` at construction time so `Deploy()` never has to nil-check. Same pattern used elsewhere in the codebase.

## Relationships

```
Plugin.Deploy()
  └── (computes routingDir)
       └── isStaticConfigPresent(routingDir)
              ├── present?true  → emit gateway.config.preserved → skip write
              ├── present?false → generateStaticConfig (writes file) → emit gateway.config.generated
              └── stat error    → return wrapped error (deploy fails)
```

No cross-package data flow changes. The `RoutingBackend` (per-route `dynamic/` writer) is unchanged. The `containerBackend` injection path is unchanged. The only new wire is the `observer` parameter on `traefik.New(...)`.
