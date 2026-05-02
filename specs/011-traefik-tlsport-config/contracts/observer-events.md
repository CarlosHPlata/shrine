# Contract: Observer Events for `tlsPort` Publishing

**Feature**: Traefik Plugin `tlsPort` Config Option
**Audience**: Downstream consumers of `engine.Observer` events emitted by the Traefik plugin (terminal logger, structured deploy logs, future UI surfaces).

This is the only operator-visible programmatic contract this feature introduces. There is no public Go API change beyond the additive struct field on `config.TraefikPluginConfig` (covered in `data-model.md`), no manifest schema change, no CLI flag change. The feature surfaces two new event names on the existing `engine.Observer` channel — one for the success case (`tls_port_no_websecure` advisory warning) and one for the probe failure case (`tls_port_probe_error`) — and keeps every existing event name and field set unchanged.

## Event namespace

Events live under the existing `gateway.` namespace already used by the plugin. No new namespace is introduced.

## Events introduced

### `gateway.config.tls_port_no_websecure`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | yes | Absolute filesystem path of the operator-preserved `traefik.yml` that lacks a `websecure` entrypoint. |
| `hint` | string | yes | One-line, actionable cleanup instruction (e.g., `"Add an entryPoints.websecure listening on :443, or delete the file so Shrine regenerates it."`). The diagnostic context (which file, which mismatch) lives in the renderer preamble and the `path` field — the hint carries only the fix. |

| Status | Emitted when |
|--------|-------------|
| `engine.StatusWarning` | `cfg.TLSPort > 0` AND the static `traefik.yml` is preserved (not Shrine-generated this deploy) AND probing the file shows no `entryPoints.websecure` key. Emitted on every deploy where the mismatch persists; emitted in addition to (not instead of) the corresponding `gateway.config.preserved` event. |

### `gateway.config.tls_port_probe_error`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string | yes | Absolute filesystem path of the operator-preserved `traefik.yml` whose websecure-entrypoint probe failed. |
| `error` | string | yes | The wrapped error message from the probe (e.g., a YAML parse failure or a non-`ErrNotExist` read error). Includes the path so an operator can locate the file and the underlying cause without cross-referencing the `path` field. |

| Status | Emitted when |
|--------|-------------|
| `engine.StatusWarning` | `cfg.TLSPort > 0` AND the static `traefik.yml` is preserved AND `hasWebsecureEntrypoint` returned a non-nil error (e.g., the file is unreadable or contains malformed YAML). The deploy is **not** blocked — the probe is advisory; emission is once per deploy where the failure occurs. |

## Compatibility expectations

- **Consumers MUST treat unknown event names as ignorable.** The terminal logger already does this; new consumers added to the codebase MUST follow the same rule.
- **No existing event name is renamed or removed.** `gateway.config.generated`, `gateway.config.preserved`, `gateway.config.legacy_http_block`, `gateway.config.legacy_probe_error`, `gateway.dashboard.generated`, `gateway.dashboard.preserved`, `gateway.route.*` all continue to behave exactly as today.
- **Field set is additive-only.** Future feature work on the plugin MAY add new fields to existing events; consumers MUST tolerate unknown fields.
- **No new event fields are added to existing events.** The new event is the entire surface delta.

## Non-events (intentionally not emitted)

- There is no `gateway.config.tls_port_published` or similar success event. The existing `gateway.config.generated` event signals successful regeneration of `traefik.yml` (which now includes the `websecure` entrypoint when `tlsPort` is set); no separate "TLS port now published" event is needed because the container-create event (`container.created` from the engine) already signals the host port mapping was applied.
- There is no `gateway.config.tls_port_removed` event. Per Decision 7 in `research.md`, removal flows through the existing regeneration path; the standard `gateway.config.generated` event is sufficient signal that a fresh `traefik.yml` was written.
- The websecure probe has its **own** dedicated probe-error event (`gateway.config.tls_port_probe_error` above) rather than reusing `gateway.config.legacy_probe_error`. This is a deliberate departure from research.md Decision 8's original plan: reuse would cause the existing terminal-logger renderer (which hard-codes the text "Could not probe traefik.yml for legacy http block") to mis-attribute websecure probe failures to the unrelated legacy-block detection path, producing confusing operator output. Two events × two renderers cleanly separate cause and effect at the cost of one extra event name in the contract surface.

## Terminal logger rendering

`internal/ui/terminal_logger.go` adds two new `case` clauses:

```go
case "gateway.config.tls_port_no_websecure":
    fmt.Fprintf(t.out, "  ⚠️  tlsPort set but traefik.yml is missing websecure entrypoint at %s — %s\n", e.Fields["path"], e.Fields["hint"])

case "gateway.config.tls_port_probe_error":
    fmt.Fprintf(t.out, "  ⚠️  Could not probe traefik.yml for websecure entrypoint (deploy continues): %s (%s)\n", e.Fields["path"], e.Fields["error"])
```

The `⚠️` glyph and indentation match the sibling `gateway.config.legacy_http_block` and `gateway.config.legacy_probe_error` renderings.

## Verification

The new event names and their emission conditions are asserted by the integration scenarios `should warn but not modify preserved traefik.yml lacking websecure entrypoint when tlsPort is set` and `should emit tls_port_probe_error when preserved traefik.yml is malformed` in `tests/integration/traefik_plugin_test.go`. The unit-level probe behavior is asserted in `internal/plugins/gateway/traefik/config_gen_test.go` against in-memory YAML fixtures (no filesystem touches; uses the existing `readFileFn` injection point), including a dedicated `TestEmitTLSPortNoWebsecureSignal_EmitsProbeError_WhenReadFails` for the new probe-error path.
