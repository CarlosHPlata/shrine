# Research: Per-Alias TLS Opt-In for Routing Aliases

**Feature**: 012-tls-alias-routers
**Phase**: 0 (Outline & Research)
**Date**: 2026-05-02

This document resolves the open design questions for the `tls` per-alias field before Phase 1 design begins. There were no `NEEDS CLARIFICATION` markers in the Technical Context; the items below are decisions that emerged from reading the existing alias surface (`internal/manifest/`, `internal/engine/`, `internal/plugins/gateway/traefik/routing.go`), the spec-006 collision regime, the spec-008 per-alias log marker, the spec-009 per-app file preservation, and the spec-011 `tlsPort` warning machinery.

## Decision 1 — Where to add the `tls` field on the manifest

**Decision**: Add `TLS bool \`yaml:"tls,omitempty"\`` to `manifest.RoutingAlias` in `internal/manifest/types.go`. Plain bool, not `*bool`.

**Rationale**: The alias spec already uses a flat struct with `Host`, `PathPrefix`, and `StripPrefix *bool`. The `*bool` on `StripPrefix` is necessary because its default depends on whether `pathPrefix` is set (true when set, false when unset) — i.e., it has three meaningful states. The new `tls` field has only two states (`true` / not-true), with `omitted` and `false` collapsing to the same semantic per the spec's edge-case "tls: false behaves identically to omission". A plain `bool` captures this exactly: YAML unmarshal produces `false` for both omission and explicit `false`, which is what we want. `omitempty` so unset alias entries round-trip to byte-identical YAML for FR-009.

**Alternatives considered**:
- `*bool` to distinguish unset from `false`. Rejected — no semantic difference between the two (per spec); the pointer would just leak a meaningless distinction into every consumer.
- A new sibling field `tls.entrypoint: websecure` enum. Rejected — over-engineered; the issue's example pins `tls: true` as the operator vocabulary, and "which entrypoint" is a Traefik configuration knob, not a manifest concern.

## Decision 2 — Where to add the `TLS` field on the engine-side alias projection

**Decision**: Extend `engine.AliasRoute` in `internal/engine/backends.go` with `TLS bool`, populated by `resolveAliasRoutes` in `internal/engine/engine.go`. Do not change `WriteRouteOp` (its `AdditionalRoutes []AliasRoute` already carries the new field for free).

**Rationale**: `AliasRoute` is the canonical engine-side projection of a manifest alias entry. The existing fields (`Host`, `PathPrefix`, `StripPrefix`) are passed by value to plugins via `WriteRouteOp.AdditionalRoutes`. Adding `TLS bool` to the same struct is the smallest surface change and keeps every plugin's view of an alias consistent — DNS plugins, future gateway plugins, etc. all see the same record. `resolveAliasRoutes` is already the single point of conversion from `manifest.RoutingAlias` to `engine.AliasRoute`, so populating the new field is a one-line addition.

**Alternatives considered**:
- Add `TLS bool` only to a Traefik-private struct, not to `engine.AliasRoute`. Rejected — would force the plugin to re-read the manifest, breaking the engine-as-projection contract; the engine is supposed to hand the plugin a fully-resolved view.
- Add a parallel `WriteRouteTLSOp` carrying the TLS info separately. Rejected — splits one record into two for no benefit; future fields on aliases (port mappings, middleware refs) would compound the split.

## Decision 3 — Router shape for `tls: true` aliases

**Decision**: Extend the `traefik.router` struct in `spec.go` with `TLS *tlsBlock \`yaml:"tls,omitempty"\``, where `tlsBlock` is a new empty struct `type tlsBlock struct{}`. When `ar.TLS == true`, `WriteRoute` sets `EntryPoints: []string{"web", "websecure"}` and `TLS: &tlsBlock{}` on the alias router; otherwise both fields are unset (and the YAML `omitempty` keeps the output byte-identical to today). The primary-domain router shape is **never** affected by any alias's `tls` flag.

**Rationale**: The issue's exemplar pins this exact YAML output:

```yaml
entryPoints:
  - web
  - websecure
tls: {}
```

The `entryPoints` list order matches the issue's exemplar and Traefik treats the list as a set, so order is not semantically meaningful at runtime — but it IS meaningful for byte-identical-comparison tests, where pinning the order makes assertions stable across YAML library updates. The empty `tlsBlock struct{}` marshals to `{}` in YAML 1.2 (verified against `gopkg.in/yaml.v3`), which is the literal token Traefik expects to mean "terminate TLS on this route using the default cert resolver / static-config TLS settings". An `interface{}` or `map[string]any` would also marshal to `{}`, but the typed empty struct (a) makes future extension explicit (if Traefik ever needs a per-route `certResolver` we'd add the field with explicit consideration) and (b) prevents accidental nil-vs-empty confusion in tests.

**Alternatives considered**:
- Use `map[string]any{}` for the TLS block. Rejected — typed struct is safer and self-documenting; the empty-struct pattern matches the YAML-marshal-friendly convention used elsewhere in the codebase (e.g., the existing `apiConfig`).
- Embed a `tls: true` boolean shorthand in the router and let the renderer expand to the YAML block. Rejected — Traefik's schema is `tls: <object>`, not `tls: <bool>`; matching Traefik's vocabulary literally minimizes surprise for operators reading the generated file.
- Reorder `entryPoints` alphabetically (`[web, websecure]` happens to be alphabetical, but the rule should be explicit). Rejected — pinning the literal order matches the issue exemplar; we don't need a sort rule for a 2-element list.

## Decision 4 — Where to emit the FR-007 warning ("alias has tls but websecure not declared")

**Decision**: Emit from inside `RoutingBackend.WriteRoute` in `internal/plugins/gateway/traefik/routing.go`, once per `WriteRoute` call (i.e., per application), when (a) at least one alias in `op.AdditionalRoutes` has `TLS == true` AND (b) `hasWebsecureEntrypoint(staticConfigPath)` returns false. The new event name is `gateway.alias.tls_no_websecure`. Emission is idempotent — every deploy re-emits while the mismatch persists (mirrors `gateway.config.legacy_http_block`'s precedent and satisfies FR-007's "MUST be emitted on every deploy where the mismatch is still present").

`RoutingBackend` gains one new field — `staticConfigPath string` — passed in by `Plugin.RoutingBackend()` as `filepath.Join(routingDir, "traefik.yml")`. This keeps `WriteRoute` self-contained; it does not need to reach back into `Plugin.cfg` or run a second `New()` round-trip.

**Rationale**: FR-007 names per-application context (alias index/host+path), so the warning must be emitted from a code path that has access to the application's alias list. `WriteRoute` is the only place inside the plugin where that is true. The `hasWebsecureEntrypoint` probe already exists (shipped by spec 011); reusing it is one line. Per-app emission rather than per-alias emission keeps the deploy log readable when an application has many aliases — operators see "this application's TLS aliases need a websecure entrypoint" once, not three times in a row. The fields included in the event name the application (team + service) plus the count of TLS-requesting aliases, which gives operators everything they need to triage without overflowing the renderer.

**Alternatives considered**:
- Emit from `Plugin.Deploy()` instead of `WriteRoute`. Rejected — `Deploy()` runs once at start of deploy and does not have access to per-application alias data (it runs before the engine iterates applications); moving alias info to `Deploy()` would require new plumbing.
- Emit per alias rather than per application. Rejected — for an app with many TLS aliases, this floods the log; the per-app event already carries enough context to find the misconfigured alias.
- Reuse `gateway.config.tls_port_no_websecure` from spec 011. Rejected per Complexity Tracking — different cause, different remediation, different fields; conflating them would force a conditional renderer that printed the wrong text half the time.
- Block the deploy when the mismatch is detected. Rejected — operators may legitimately stage the manifest change first and the `tlsPort` change second (or the reverse); the alias router is still written so the deploy is safe to land before the static config catches up.

## Decision 5 — Validation surface for the new field

**Decision**: Rely entirely on YAML unmarshal for the structural validation of the `tls` field — non-boolean values are rejected with a path-bearing error from `gopkg.in/yaml.v3` automatically (verified against the existing parser tests, which assert error-message shapes for similar `*bool`/`bool` fields). Add no new code in `validate.go`. The "tls is only valid inside an alias entry" guarantee (FR-005) is enforced by the schema for free — the field is declared only on `RoutingAlias`, never on `Routing`, so a top-level `routing.tls` is rejected by `yaml.Unmarshal` with `"field tls not found in type manifest.Routing"`.

**Rationale**: Adding redundant string-checking logic in `validate.go` would duplicate what the YAML decoder already does, and would push us toward multi-error reporting at the wrong layer (the `Validate` function is for semantic rules; structural type errors belong in the parse step). The test that asserts "non-boolean tls is rejected" lives in `parser_test.go` so the contract is captured next to the parser, not next to the validator.

The `validateRoutingAliases` collision check is unchanged: it keys on `(host, normalizedPathPrefix)`, which is the right uniqueness rule per FR-006 ("TLS flag is a routing decoration, not a uniqueness key"). Two aliases with the same host+path collide regardless of `tls` flags; aliases that differ only by `tls` are NOT a collision (one's HTTP, one's HTTPS — but they'd produce identical Traefik routes because Traefik routers are keyed by rule, not by entrypoint set, so this would be a real collision and the existing check correctly flags it).

**Alternatives considered**:
- Add an explicit "tls must be a boolean" check in `validateRoutingAliases`. Rejected — duplicates the parser's structural check; the parser-level error is already clear and includes the YAML path.
- Add a positive-allow-list "field tls is only valid here" check. Rejected — Go's struct-tag-driven `yaml.Unmarshal` already enforces this at zero cost; an explicit allow-list would be redundant.

## Decision 6 — Per-alias log marker for FR-010

**Decision**: Extend `formatAliasesForLog` in `engine.go` to append `" (tls)"` to an alias's log string when `r.TLS == true`. The marker composes with the existing `" (no strip)"` marker, so an alias with `pathPrefix: /foo, stripPrefix: false, tls: true` logs as `"alias.example.com+/foo (no strip) (tls)"`. The marker is appended in a fixed order — `(no strip)` first (existing position) then `(tls)` — so the log line is stable under sort.

**Rationale**: The spec's FR-010 ("operators MUST be able to confirm at a glance which aliases were published with HTTPS") is satisfied by extending the existing per-alias log marker introduced by spec 008. Adding one `if r.TLS` branch to `formatAliasesForLog` is the smallest possible change; no new event needed (the existing `routing.configure` event already prints the formatted alias list per app). The marker shape `" (tls)"` matches the existing `" (no strip)"` precedent literally — same parenthesized lowercase keyword, same leading space.

**Alternatives considered**:
- Emit a separate `routing.alias.tls` event per TLS-enabled alias. Rejected — duplicates the info already in `routing.configure` and floods the log.
- Use a `🔒` emoji marker in the existing log line. Rejected — terminal logger conventions in this codebase use literal text in marker fields; emoji are reserved for the `t.out` renderer side, not for the field strings.

## Decision 7 — Integration test scenarios

**Decision**: Add four new scenarios to `tests/integration/traefik_plugin_test.go`:

1. **`should publish alias router with tls block when alias sets tls: true`** — covers User Story 1's acceptance scenarios 1 and 3. Deploys a manifest with one alias `tls: true`, asserts the generated dynamic config contains `entryPoints: [web, websecure]` and `tls: {}` on the alias router, and `entryPoints: [web]` (no `tls`) on the primary-domain router.
2. **`should produce mixed TLS-on / TLS-off router shapes when only some aliases set tls`** — covers User Story 2. Deploys a manifest with two aliases (first without `tls`, second with `tls: true`), asserts the generated dynamic config contains exactly two alias routers with the expected differentiated shapes.
3. **`should produce byte-identical dynamic config when no alias sets tls`** — covers User Story 3 / FR-009. Deploys a manifest with one alias and no `tls` field, captures the generated dynamic config, then re-deploys the same manifest and asserts the file is unchanged. Also asserts the file does NOT contain the strings `"websecure"` or `"tls:"`.
4. **`should warn when alias sets tls: true but static config lacks websecure entrypoint`** — covers FR-007. Deploys a manifest with `tls: true` on an alias against a Traefik plugin configured WITHOUT `tlsPort` (so the generated `traefik.yml` has no `websecure`), asserts (a) the deploy succeeds, (b) the alias router is still written with the TLS block, and (c) the deploy log contains the `gateway.alias.tls_no_websecure` event with the expected fields.

**Rationale**: Each scenario maps 1:1 to an FR or acceptance scenario in the spec. The `NewDockerSuite` harness builds the real `shrine` binary and runs it as a subprocess against a live Docker daemon, so each assertion is end-to-end including the YAML emission of the new `tls: {}` block. Fixtures are stored under `tests/integration/fixtures/traefik-alias-tls*/` mirroring the existing `traefik-alias-host-only/` and `traefik-alias-prefix/` precedents.

**Alternatives considered**:
- One mega-scenario covering all three router shapes plus the warning. Rejected — `NewDockerSuite` cleanup is per-test-case, and the constitution favors separate scenarios per acceptance criterion for clear regression reporting.
- A scenario that actually issues an HTTPS request and validates the response body. Rejected as out-of-scope — per FR-012, Shrine does not configure TLS termination; an HTTPS-from-the-client integration would require a wired-up cert resolver and a trusted-cert chain, which is outside this feature's surface. A future end-to-end TLS test belongs in a separate spec that owns the cert flow.
