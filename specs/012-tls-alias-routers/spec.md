# Feature Specification: Per-Alias TLS Opt-In for Routing Aliases

**Feature Branch**: `012-tls-alias-routers`
**Created**: 2026-05-02
**Status**: Draft
**Input**: GitHub issue [#13](https://github.com/CarlosHPlata/shrine/issues/13) — "App manifest should support websecure entrypoint on alias routers"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator publishes a single alias over HTTPS via the manifest (Priority: P1)

An operator runs an application — say, a personal-finance app — that they reach over HTTP at `finances.home.lab` from inside their LAN. They also want to reach the same app from outside the LAN over their Tailscale tailnet at `https://gateway.tail9a6ddb.ts.net/finances`. Today, the alias generated for the Tailscale host attaches only to the `web` entrypoint, so reaching it over HTTPS requires the operator to hand-edit the generated dynamic config to add `websecure` and `tls: {}` on every alias they want served over TLS — a workaround that survives across redeploys (per spec 009) but means the manifest no longer reflects the full routing intent. With per-alias TLS opt-in, the operator declares the intent inline:

```yaml
routing:
  domain: finances.home.lab
  aliases:
    - host: gateway.tail9a6ddb.ts.net
      pathPrefix: /finances
      stripPrefix: false
      tls: true
```

After `shrine deploy`, the alias router published in the application's dynamic config attaches to both the `web` and `websecure` entrypoints and carries an empty TLS block (`tls: {}`), causing Traefik to terminate TLS on that route using its existing TLS configuration. The primary `routing.domain` and any aliases without `tls: true` keep behaving exactly as they do today.

**Why this priority**: This is the entire point of the feature. Without it, the manifest cannot express "publish this alias over HTTPS," so multi-network operators (LAN + Tailscale, internal + public) must keep hand-editing generated config to recover routing intent that ought to live in the manifest. P1 because nothing else in this feature delivers value on its own.

**Independent Test**: Author a manifest with `routing.domain` plus one alias that sets `tls: true`. With the Traefik gateway plugin active and a `websecure` entrypoint already configured (per spec 011's `tlsPort` flow or via operator-edited `traefik.yml`), run `shrine deploy`. Inspect the generated dynamic config file for the application: the alias router declares `entryPoints: [web, websecure]` and contains `tls: {}`; the primary-domain router still declares only `entryPoints: [web]` and has no `tls` block. Send an HTTPS request to the alias address; it reaches the running container.

**Acceptance Scenarios**:

1. **Given** a manifest with `routing.domain` and one alias entry setting `tls: true`, **When** `shrine deploy` runs with the Traefik plugin active, **Then** the generated dynamic config's alias router declares both `web` and `websecure` entrypoints and contains `tls: {}`, while the primary-domain router declares only `web` and has no `tls` block.
2. **Given** the same manifest, **When** an HTTPS request is sent to the alias host (and Traefik has a `websecure` entrypoint listening), **Then** the request is routed to the same backend service as the primary-domain HTTP request, producing identical response bodies.
3. **Given** a manifest with `routing.domain` and one alias entry that omits `tls` (or sets `tls: false`), **When** `shrine deploy` runs, **Then** the generated alias router declares only the `web` entrypoint and contains no `tls` block — byte-identical to the pre-feature output for the same input.

---

### User Story 2 - Mixed TLS-on / TLS-off aliases on the same application (Priority: P1)

An operator runs the same app with two aliases: one internal (HTTP-only on the LAN) and one external (must be HTTPS over Tailscale). They declare both aliases in the same `routing.aliases` list, set `tls: true` on the external one only, and leave the internal one untouched. After `shrine deploy`, the application's dynamic config contains one alias router that attaches to `web` only, and a second alias router (the external one) that attaches to both `web` and `websecure` and carries `tls: {}`. The primary domain remains HTTP-only.

**Why this priority**: This is the realistic deployment shape the issue points at — at least one alias keeps HTTP, at least one needs HTTPS, and the manifest must express both without ambiguity. P1 because the feature is incomplete if `tls: true` cannot be applied per-alias-entry; uniform-on-all and uniform-off-all are not viable substitutes for the multi-network operator.

**Independent Test**: Author a manifest with `routing.domain` and two aliases — first alias has no `tls` field (or `tls: false`), second alias has `tls: true`. Run `shrine deploy`. Inspect the generated dynamic config: the first alias router has `entryPoints: [web]` and no TLS block; the second alias router has `entryPoints: [web, websecure]` and `tls: {}`. Both aliases route to the same backend service as the primary domain.

**Acceptance Scenarios**:

1. **Given** a manifest with two alias entries where the first omits `tls` and the second sets `tls: true`, **When** `shrine deploy` completes, **Then** the generated dynamic config contains exactly two alias routers, one HTTP-only and one HTTPS-enabled, and both point to the same backend service.
2. **Given** the same manifest, **When** the operator removes `tls: true` from the second alias and re-deploys, **Then** the second alias router is regenerated without `tls: {}` and attaches only to `web` — the dynamic config returns to a fully HTTP-only state.

---

### User Story 3 - Existing manifests without `tls` keep working unchanged (Priority: P1)

An operator who has not opted into per-alias TLS leaves all alias entries unchanged (no `tls` field present). After `shrine deploy`, the generated dynamic config is byte-identical (modulo unrelated, already-shipped changes) to the file produced by the previous Shrine release for the same input. No new entrypoints, no `tls` blocks, no log noise.

**Why this priority**: Backward compatibility with the existing fleet of Shrine deployments is non-negotiable. Adding a new optional alias field MUST NOT alter behavior for operators who do not adopt it. P1 because a regression here would break every existing deployment on upgrade — including the operator-preserved files protected by spec 009.

**Independent Test**: On a host with a previously-deployed application whose manifest declares aliases without `tls`, capture the current dynamic config file. Upgrade to the release containing this feature. Run `shrine deploy` with the unchanged manifest. Compare the dynamic config: it must be byte-identical to the captured baseline. (When spec 009 has marked the file as operator-owned, `shrine deploy` MUST NOT rewrite it; the assertion on byte-equality applies to the file Shrine would have generated, not necessarily to a regenerated file.)

**Acceptance Scenarios**:

1. **Given** a manifest with one or more alias entries, none of which set `tls`, **When** `shrine deploy` completes after upgrading to this release, **Then** the dynamic config Shrine would generate for that manifest is byte-identical to the file generated by the previous release for the same input.
2. **Given** a manifest with no `routing.aliases` field at all, **When** `shrine deploy` completes, **Then** behavior is identical to today and no `tls`-related artefact appears anywhere in the generated config.

---

### Edge Cases

- **`tls: true` is set but the active Traefik static configuration has no `websecure` entrypoint**: Shrine writes the alias router with `web` + `websecure` entrypoints and `tls: {}` regardless. Traefik will start successfully but routes attached to the missing `websecure` entrypoint will not receive HTTPS traffic until the operator wires the entrypoint (via spec 011's `tlsPort` config, or by editing a preserved `traefik.yml`). Shrine SHOULD emit a deploy-time warning that names the application, the alias index, and the missing `websecure` entrypoint, instructing the operator to set `tlsPort` or add the entrypoint manually. (This mirrors the warning pattern established by spec 011 for the static-config side.)
- **`tls` is omitted entirely**: Treated as `tls: false`. The alias router is generated with only the `web` entrypoint and no `tls` block, exactly as today.
- **`tls: false` is set explicitly**: Behaves identically to omission. No error, no warning. The field is accepted as harmless because operators may flip the field on/off across deploys and authoring `tls: false` explicitly is a legitimate signal of intent.
- **`tls: true` on an alias that has only `host` (no `pathPrefix`)**: The alias router matches the host on every path, attaches to `web` + `websecure`, and carries `tls: {}`. No interaction with `stripPrefix` (which is a no-op when there is no prefix; see spec 006).
- **`tls: true` combined with `stripPrefix: false` (or `stripPrefix: true`)**: The two fields are independent. `stripPrefix` continues to control whether the matched prefix is removed before the request reaches the backend; `tls` controls whether `websecure` is published and `tls: {}` is emitted. Both effects compose without conflict.
- **`tls: true` on the primary `routing.domain`**: Out of scope for this feature. The `tls` field is only valid inside `routing.aliases` entries; declaring `tls: true` at the `routing.*` top level (or anywhere outside an alias entry) MUST fail manifest validation with a clear error naming the application and the offending field path. Operators who need HTTPS on the primary domain do so via spec 011's `tlsPort` plus their own Traefik TLS configuration, exactly as before. (Adding per-primary-domain TLS opt-in is a separate, larger conversation about migration; this feature deliberately does not pre-empt it.)
- **`tls` set to a non-boolean value** (e.g., a string `"yes"`, an integer, an object): The manifest is invalid and the deploy fails with a clear YAML-shape error naming the application, the alias index, and the offending value. Validation does not coerce truthy strings into booleans.
- **Operator preserves the dynamic config file (per spec 009) and then later flips `tls: true` on an alias**: Shrine MUST NOT rewrite the preserved file. The flip in the manifest has no effect until the operator either deletes the file (so Shrine regenerates it including the new `tls: true` semantics) or hand-edits the preserved file to mirror the new intent. This matches the regime established by spec 009 — manifest fields are advisory once a file is operator-owned. Shrine SHOULD emit the same `gateway.route.preserved` info-level signal it emits today, so the operator can spot the divergence in the deploy log without a separate warning.
- **Primary-domain HTTPS via the issue's old workaround**: Some operators may have edited their preserved dynamic config to put `tls: {}` on the primary-domain router as well. Per the previous bullet, that file remains preserved; this feature does not regenerate it. The new manifest field affects only newly-generated dynamic config files, never preserved ones.
- **`routing.aliases` is empty or omitted**: Behavior is identical to today. The `tls` field has no surface area when there are no alias entries.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The application manifest schema MUST accept an optional boolean `tls` field on each entry of `routing.aliases`. The field defaults to `false` when omitted.
- **FR-002**: When the Traefik gateway plugin is active and an alias entry sets `tls: true`, Shrine MUST generate the alias router in the application's dynamic config file with the `entryPoints` list `[web, websecure]` (in that order) and MUST emit an empty `tls: {}` block on the router.
- **FR-003**: When the Traefik gateway plugin is active and an alias entry sets `tls: false` or omits the `tls` field, Shrine MUST generate the alias router exactly as today — `entryPoints: [web]` only, no `tls` block, no other 443-related artefact. The primary-domain router MUST remain unchanged regardless of any alias's `tls` value.
- **FR-004**: Manifest parsing MUST reject a `tls` value that is not a YAML boolean (i.e., reject strings, numbers, objects, lists), surfacing a clear error that names the offending application and alias index.
- **FR-005**: Manifest parsing MUST reject a `tls` field declared anywhere outside a `routing.aliases` entry (e.g., at the `routing` top level). The error MUST name the application and the offending field path. This forecloses an ambiguity where operators could appear to opt the primary domain into HTTPS via this feature.
- **FR-006**: The cross-application collision check (per spec 006 FR-008a) MUST treat host+path collisions independently of the `tls` flag — two aliases on different applications with the same host+path collide regardless of whether either sets `tls: true`. The TLS flag is a routing decoration, not a uniqueness key.
- **FR-007**: When `tls: true` is set on at least one alias of an application AND the active Traefik static configuration (Shrine-generated or operator-preserved) does not declare a `websecure` entrypoint, Shrine MUST emit a deploy-time warning naming the application, the alias index/host+path, and instructing the operator to wire `websecure` (via spec 011's `tlsPort` or an operator-edited static config). The warning MUST be emitted on every deploy where the mismatch is still present and MUST NOT block the deploy — the alias router is still written so operators can land both changes in any order.
- **FR-008**: When the application's dynamic config file is operator-preserved (per spec 009), Shrine MUST NOT rewrite the file in response to `tls` field changes in the manifest, in line with the existing preservation regime. Shrine MUST emit the same `gateway.route.preserved` info-level signal it emits today; no new warning is required for `tls`-specific drift between manifest and preserved file.
- **FR-009**: Manifests that omit `tls` from every alias entry (including manifests with no `routing.aliases` field at all) MUST produce dynamic config that is byte-identical (modulo unrelated, already-shipped changes) to the file generated by the previous release for the same input. This is the regression guard for the existing fleet.
- **FR-010**: The deploy log MUST include an observable signal (info-level or equivalent, consistent with the existing per-alias log marker established by spec 008) indicating, for each alias, whether `tls` is enabled. Operators inspecting the log MUST be able to confirm at a glance which aliases were published with HTTPS without reading the generated dynamic config file.
- **FR-011**: The new `tls` field MUST be documented on the same surface that already documents `routing.aliases`, `host`, `pathPrefix`, and `stripPrefix`. The documentation MUST state that `tls: true` only opens the routing/entrypoint side — TLS certificate provisioning and termination configuration on the `websecure` entrypoint remain entirely operator-owned via standard Traefik mechanisms (mirroring spec 011 FR-010).
- **FR-012**: Shrine MUST NOT generate, validate, reference, distribute, or otherwise interact with TLS certificates, certificate file paths, certificate resolvers, ACME providers, default-cert configuration, HTTP→HTTPS redirects, or any other TLS-termination concern as part of this feature. The `tls: {}` block emitted on alias routers MUST be empty — Shrine MUST NOT inject `certResolver`, `domains`, `options`, or any sibling key into it. Operators configure TLS termination on the `websecure` entrypoint exclusively through standard Traefik mechanisms outside this feature's surface.

### Key Entities

- **Alias entry (manifest field)**: Already defined by spec 006 as a triple of `host`, `pathPrefix`, `stripPrefix`. This feature adds a fourth optional boolean field `tls` (default `false`). The field is a routing decoration: it controls which entrypoints the generated alias router attaches to and whether an empty `tls: {}` block is emitted.
- **Alias router (gateway dynamic config)**: Already defined by spec 006 as the per-alias entry in the generated dynamic config. This feature extends the alias router's shape: when the source alias sets `tls: true`, the router declares `entryPoints: [web, websecure]` and carries `tls: {}`; otherwise the router shape is unchanged.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator who needs to expose an existing alias over HTTPS can do so by adding a single `tls: true` line to the existing alias entry — no second manifest, no hand-edit of generated config, no extra commands beyond a normal `shrine deploy`.
- **SC-002**: After deploying a manifest with `tls: true` on N of M alias entries, the generated dynamic config contains exactly N alias routers attached to `web` + `websecure` with `tls: {}`, and exactly M − N alias routers attached to `web` only with no `tls` block. The primary-domain router is unchanged.
- **SC-003**: 100% of existing Shrine deployments whose manifests do NOT use `tls` on any alias continue to deploy successfully after upgrading to the release containing this feature, with byte-identical generated dynamic config (modulo unrelated, already-shipped changes).
- **SC-004**: A manifest with an invalid `tls` value (non-boolean, or set outside an alias entry) is rejected at validation time with a single clear error naming the offending field; no dynamic config file is written or mutated for that application.
- **SC-005**: Removing `tls: true` from an alias and re-deploying causes the corresponding alias router to lose its `websecure` entrypoint and `tls: {}` block within one deploy cycle (subject to spec 009 preservation, which is unchanged here). Operators do not need to hand-edit the generated dynamic config to revert.
- **SC-006**: Zero new GitHub issues filed against Shrine in the 30 days following the release reporting "had to hand-edit dynamic config to publish an alias over HTTPS" or "tls: true on an alias did not produce the expected router shape."

## Assumptions

- The `websecure` entrypoint exists in the active Traefik static configuration. Whether it was put there by spec 011's `tlsPort` flow, by an operator hand-edit of a preserved `traefik.yml`, or by some future Shrine surface, is out of scope for this feature. When the entrypoint is missing, Shrine still writes the alias router as specified (FR-002) and emits a warning (FR-007) — it does not block the deploy or fall back to a different router shape.
- TLS certificate provisioning and termination are entirely operator-owned, exactly as established by spec 011 FR-010. Shrine emits an empty `tls: {}` block to mark "Traefik should terminate TLS on this route using whatever it already knows," and contributes nothing else to the TLS layer. Operators wanting per-route certs, resolvers, or options configure them via standard Traefik mechanisms (preserved `traefik.yml`, separately-managed dynamic config, external cert management).
- Spec 009's operator-edit preservation regime for per-app dynamic config files applies unchanged. The new `tls` field is advisory once a dynamic config file is operator-owned; Shrine MUST NOT rewrite preserved files in response to `tls` flips. Operators who want the new generated shape on a preserved file delete the file and let Shrine regenerate it.
- Spec 006's manifest validation surface (host shape, pathPrefix shape, cross-application collision detection) applies unchanged; `tls` is a separate field added to the existing entry shape and inherits the existing validation pipeline for everything else on the alias entry.
- The `tls` field is intentionally scoped to alias entries (FR-005). Adding `tls: true` semantics for the primary `routing.domain` is a larger conversation — it would interact with default-router behavior, with the issue's existing workaround, and with the migration story for the existing fleet — and this spec defers that conversation rather than pre-empting it.
- The `entryPoints` list order `[web, websecure]` is chosen for stability with the example in the GitHub issue and for ease of byte-comparison in tests; Traefik treats the list as a set, so the order is not semantically meaningful at runtime.
- This feature does not introduce a separate flag, env var, or migration. The new field is additive and optional; existing manifests are unaffected.
