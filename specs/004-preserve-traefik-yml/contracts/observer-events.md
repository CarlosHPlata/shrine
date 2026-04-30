# Contract: Gateway Config Observer Events

This feature exposes one new contract to the rest of the codebase: two new event names on the existing `engine.Observer` interface. The interface itself does not change.

## Event: `gateway.config.preserved`

**Emitted when**: `Plugin.Deploy()` finds an existing entry at the `traefik.yml` path (regular file, symlink — broken or otherwise — directory, or any other file type) and skips the default-write.

**Schema**:

| Field | Type | Required | Value |
|-------|------|----------|-------|
| `Name` | `string` | yes | `"gateway.config.preserved"` |
| `Status` | `engine.EventStatus` | yes | `engine.StatusInfo` |
| `Fields["path"]` | `string` | yes | Absolute path to the preserved file |

**Producers**: `internal/plugins/gateway/traefik` (sole producer — the helper that gates `generateStaticConfig`).

**Consumers** (in this feature):
- `internal/ui/terminal_logger.go` — renders `  📄 Preserving operator-owned traefik.yml: <path>` (or equivalent emoji-prefixed line consistent with neighboring deploy steps).
- `internal/ui/file_logger.go` — persists the event verbatim; no code change needed (FileLogger logs every event by name).

**Frequency**: exactly once per `shrine deploy` invocation against a host where the file already exists.

## Event: `gateway.config.generated`

**Emitted when**: `Plugin.Deploy()` finds the `traefik.yml` path absent and writes the default static configuration.

**Schema**:

| Field | Type | Required | Value |
|-------|------|----------|-------|
| `Name` | `string` | yes | `"gateway.config.generated"` |
| `Status` | `engine.EventStatus` | yes | `engine.StatusInfo` |
| `Fields["path"]` | `string` | yes | Absolute path to the freshly generated file |

**Producers**: same as above.

**Consumers** (in this feature):
- `internal/ui/terminal_logger.go` — renders `  📝 Generated default traefik.yml: <path>`.
- `internal/ui/file_logger.go` — same passthrough behavior.

**Frequency**: exactly once per `shrine deploy` invocation against a host where the file does not yet exist (first deploy after install, or after the operator deletes the file per User Story 3).

## Mutual exclusivity

`gateway.config.preserved` and `gateway.config.generated` MUST NOT both fire in the same deploy. The existence probe is single-shot and decides exactly one branch.

## Failure mode (no event emitted)

If `os.Lstat` returns an error and `os.IsNotExist(err)` is false, `Plugin.Deploy()` returns `fmt.Errorf("traefik plugin: checking traefik.yml at %q: %w", path, err)` *before* either event is emitted. The deploy fails. No partial-state event is logged for this branch — the wrapped error itself surfaces to stderr through the standard error path.

## Backwards compatibility

Adding new event names is additive — existing observers (`TerminalObserver`, `FileLogger`) ignore unknown names by default. No existing event contract is modified. No struct field is renamed.

## Constructor change (related, not strictly an "event contract")

`func traefik.New(cfg *config.TraefikPluginConfig, backend engine.ContainerBackend, specsDir string) (*Plugin, error)` gains a fourth parameter:

```go
func New(
    cfg *config.TraefikPluginConfig,
    backend engine.ContainerBackend,
    specsDir string,
    observer engine.Observer,         // NEW — may be nil; normalized to NoopObserver
) (*Plugin, error)
```

A nil `observer` is normalized to `engine.NoopObserver{}` inside `New` so the Deploy path never branches on nil. Callers in production: `internal/handler/deploy.go` (passes the existing `engine.MultiObserver` already constructed at line 73 of that file).
