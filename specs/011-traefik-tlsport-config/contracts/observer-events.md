# Contract: Observer Events for `tlsPort` Publishing

**Feature**: Traefik Plugin `tlsPort` Config Option
**Audience**: Downstream consumers of `engine.Observer` events emitted by the Traefik plugin (terminal logger, structured deploy logs, future UI surfaces).

This is the only operator-visible programmatic contract this feature introduces. There is no public Go API change beyond the additive struct field on `config.TraefikPluginConfig` (covered in `data-model.md`), no manifest schema change, no CLI flag change. The feature surfaces one new event name on the existing `engine.Observer` channel and keeps every existing event name and field set unchanged.

## Event namespace

Events live under the existing `gateway.` namespace already used by the plugin. No new namespace is introduced.

## Event introduced

### `gateway.config.tls_port_no_websecure`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | yes | Absolute filesystem path of the operator-preserved `traefik.yml` that lacks a `websecure` entrypoint. |
| `hint` | string | yes | One-line, human-readable cleanup instruction (e.g., `"tlsPort=<N> publishes host port <N>→443/tcp on the Traefik container, but this preserved traefik.yml has no entryPoints.websecure listening on :443. Add the entrypoint, or delete the file so Shrine regenerates it."`). |

| Status | Emitted when |
|--------|-------------|
| `engine.StatusWarning` | `cfg.TLSPort > 0` AND the static `traefik.yml` is preserved (not Shrine-generated this deploy) AND probing the file shows no `entryPoints.websecure` key. Emitted on every deploy where the mismatch persists; emitted in addition to (not instead of) the corresponding `gateway.config.preserved` event. |

## Compatibility expectations

- **Consumers MUST treat unknown event names as ignorable.** The terminal logger already does this; new consumers added to the codebase MUST follow the same rule.
- **No existing event name is renamed or removed.** `gateway.config.generated`, `gateway.config.preserved`, `gateway.config.legacy_http_block`, `gateway.config.legacy_probe_error`, `gateway.dashboard.generated`, `gateway.dashboard.preserved`, `gateway.route.*` all continue to behave exactly as today.
- **Field set is additive-only.** Future feature work on the plugin MAY add new fields to existing events; consumers MUST tolerate unknown fields.
- **No new event fields are added to existing events.** The new event is the entire surface delta.

## Non-events (intentionally not emitted)

- There is no `gateway.config.tls_port_published` or similar success event. The existing `gateway.config.generated` event signals successful regeneration of `traefik.yml` (which now includes the `websecure` entrypoint when `tlsPort` is set); no separate "TLS port now published" event is needed because the container-create event (`container.created` from the engine) already signals the host port mapping was applied.
- There is no `gateway.config.tls_port_removed` event. Per Decision 7 in `research.md`, removal flows through the existing regeneration path; the standard `gateway.config.generated` event is sufficient signal that a fresh `traefik.yml` was written.
- There is no probe-error event for the `hasWebsecureEntrypoint` helper. If YAML unmarshalling of the preserved `traefik.yml` fails, the helper returns the error to the caller, which surfaces it through the existing `gateway.config.legacy_probe_error` event (status `StatusWarning`, fields `path`/`error`) — same pattern already used for the legacy-http-block probe in `config_gen.go`. This avoids inventing yet another error event for the same probe failure mode.

## Terminal logger rendering

`internal/ui/terminal_logger.go` adds one new `case` clause:

```go
case "gateway.config.tls_port_no_websecure":
    fmt.Fprintf(t.out, "  ⚠️  tlsPort set but traefik.yml is missing websecure entrypoint at %s — %s\n", e.Fields["path"], e.Fields["hint"])
```

The `⚠️` glyph and indentation match the sibling `gateway.config.legacy_http_block` rendering.

## Verification

The new event name and its emission conditions are asserted by the integration scenario `should warn but not modify preserved traefik.yml lacking websecure entrypoint when tlsPort is set` in `tests/integration/traefik_plugin_test.go` (Phase 2 / `/speckit-tasks`). The unit-level probe behavior is asserted in `internal/plugins/gateway/traefik/config_gen_test.go` against in-memory YAML fixtures (no filesystem touches; uses the existing `readFileFn` injection point).
