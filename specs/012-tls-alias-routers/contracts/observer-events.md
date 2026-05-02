# Contract: Observer Events for Per-Alias TLS Publishing

**Feature**: Per-Alias TLS Opt-In for Routing Aliases
**Audience**: Downstream consumers of `engine.Observer` events emitted by the Traefik plugin (terminal logger, structured deploy logs, future UI surfaces).

This is the only operator-visible programmatic contract this feature introduces. There is no public Go API change beyond the additive struct fields documented in `data-model.md`, no new manifest kind, no new CLI flag. The feature surfaces one new event name on the existing `engine.Observer` channel — `gateway.alias.tls_no_websecure` — and extends one existing event's `aliases` field to include a per-alias `(tls)` marker. Every existing event name and field set is unchanged.

## Event namespace

Events live under the existing `gateway.` namespace already used by the plugin. This feature opens a new sub-namespace `gateway.alias.*` parallel to the established `gateway.config.*` and `gateway.route.*` sub-namespaces. The existing `routing.configure` event (emitted by the engine, not the plugin) is unchanged in name; only the rendering of one of its fields is extended.

## Events introduced

### `gateway.alias.tls_no_websecure`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `team` | string | yes | Owner team of the application whose manifest declared at least one alias with `tls: true`. |
| `name` | string | yes | Application name (the value of `metadata.name` in the manifest). |
| `path` | string | yes | Absolute filesystem path of the static Traefik config (`<routing-dir>/traefik.yml`) whose `entryPoints.websecure` key was probed and found missing. |
| `tls_aliases` | string | yes | Comma-separated list of the TLS-requesting aliases on the application, formatted as `host[+pathPrefix]` (mirrors `formatAliasesForLog`'s shape so operators can grep for the offending alias). Sorted alphabetically for stable diffing. |
| `hint` | string | yes | One-line, actionable cleanup instruction (e.g., `"Set plugins.gateway.traefik.tlsPort to publish a websecure entrypoint, or add it to the preserved traefik.yml."`). |

| Status | Emitted when |
|--------|--------------|
| `engine.StatusWarning` | At least one alias in the application's `routing.aliases` has `tls: true` AND probing the active static Traefik config (`<routing-dir>/traefik.yml`) shows no `entryPoints.websecure` key. Emitted on every deploy where the mismatch persists; emitted in addition to (not instead of) the existing `gateway.config.tls_port_no_websecure` event from spec 011 if both mismatches happen to be present at once. The deploy is **not** blocked — the alias router is still written. |

The probe error path (e.g., the static config is unreadable or contains malformed YAML) reuses spec 011's existing `gateway.config.tls_port_probe_error` event verbatim — no new probe-error event is needed because the underlying probe is the same `hasWebsecureEntrypoint` helper, and a probe failure that prevents this feature from emitting its warning is structurally the same failure that already concerns spec 011's machinery.

## Events extended (rendering only)

### `routing.configure` (existing event, unchanged in name and emission conditions)

The `aliases` field, formatted by `engine.formatAliasesForLog`, gains a new optional `(tls)` marker. The marker composes with the existing `(no strip)` marker introduced by spec 008.

| Alias shape | Old `aliases` field render | New `aliases` field render |
|-------------|---------------------------|----------------------------|
| `host=a.example.com` | `a.example.com` | (unchanged) |
| `host=a.example.com, pathPrefix=/x` (default strip true) | `a.example.com+/x` | (unchanged) |
| `host=a.example.com, pathPrefix=/x, stripPrefix=false` | `a.example.com+/x (no strip)` | (unchanged) |
| `host=a.example.com, tls=true` | n/a | `a.example.com (tls)` |
| `host=a.example.com, pathPrefix=/x, tls=true` | n/a | `a.example.com+/x (tls)` |
| `host=a.example.com, pathPrefix=/x, stripPrefix=false, tls=true` | n/a | `a.example.com+/x (no strip) (tls)` |

The marker order is fixed: `(no strip)` first (matching its existing position), `(tls)` second. Multiple aliases continue to be comma-joined and alphabetically sorted as before, so the field stays diff-stable.

## Compatibility expectations

- **Consumers MUST treat unknown event names as ignorable.** The terminal logger already does this; new consumers added to the codebase MUST follow the same rule.
- **No existing event name is renamed or removed.** `gateway.config.*`, `gateway.dashboard.*`, `gateway.route.*`, `routing.configure`, `dns.register`, `container.create`, etc. all continue to behave exactly as today.
- **Field set is additive-only.** Future feature work on the plugin MAY add new fields to existing events; consumers MUST tolerate unknown fields. The `aliases` field on `routing.configure` is a single human-formatted string by contract — its render is a stable text format, not a parser-friendly structure, so consumers who need machine-readable per-alias data should consume the future structured-log surface, not parse this string.
- **No new event fields are added to existing events.** The `routing.configure` event's `aliases` field gains content (the `(tls)` marker) but does not gain a new key.

## Non-events (intentionally not emitted)

- There is no `gateway.alias.tls_published` or similar success event. The existing `gateway.route.generated` event already signals successful regeneration of a per-app dynamic config file (which now includes the `tls: {}` block when at least one alias sets `tls: true`); no separate "alias TLS now published" event is needed.
- There is no `gateway.alias.tls_removed` event. Removal flows through the same `gateway.route.generated` path (the file is regenerated without the TLS block); no special signal is needed.
- The websecure probe reuses spec 011's existing `gateway.config.tls_port_probe_error` event for I/O failures (file unreadable, malformed YAML) rather than introducing a parallel `gateway.alias.tls_probe_error`. The error contract is identical, the underlying probe is identical, and adding a parallel event would force operators to handle two equivalent error surfaces.

## Terminal logger rendering

`internal/ui/terminal_logger.go` adds one new `case` clause:

```go
case "gateway.alias.tls_no_websecure":
    fmt.Fprintf(t.out, "  ⚠️  alias tls: true but websecure entrypoint missing in %s for %s.%s (%s) — %s\n",
        e.Fields["path"], e.Fields["team"], e.Fields["name"], e.Fields["tls_aliases"], e.Fields["hint"])
```

The `⚠️` glyph and indentation match the sibling `gateway.config.legacy_http_block` and `gateway.config.tls_port_no_websecure` renderings.

The `routing.configure` renderer is unchanged structurally — it already prints `↳ Aliases: <field>` when the `aliases` field is non-empty. The new `(tls)` marker flows through that path unmodified.

## Verification

- `gateway.alias.tls_no_websecure` emission and field shape are asserted by the integration scenario `should warn when alias sets tls: true but static config lacks websecure entrypoint` in `tests/integration/traefik_plugin_test.go`. Unit-level emission is asserted in `internal/plugins/gateway/traefik/routing_test.go` against a fake observer with the existing `readFileFn` injection point used to stub the static-config probe.
- `routing.configure` `aliases` field rendering for `(tls)` is asserted at the unit level in `internal/engine/engine_aliases_test.go` (extending the existing `formatAliasesForLog` cases).
- The `routing.configure` end-to-end render including `(tls)` is asserted in the integration scenarios alongside the dynamic-config-shape assertions, so a regression in either layer surfaces in the same test run.
