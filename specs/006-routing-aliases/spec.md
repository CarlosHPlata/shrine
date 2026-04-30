# Feature Specification: Routing Aliases for Application Manifests

**Feature Branch**: `006-routing-aliases`  
**Created**: 2026-04-30  
**Status**: Draft  
**Input**: User description: "Issue 1 — Feature: routing.aliases in application manifests. Applications can only declare a single routing.domain. Add a routing.aliases field that accepts additional host + optional pathPrefix combinations. When the Traefik plugin is active, each alias generates an additional router in the dynamic config file pointing to the same backend service. If the plugin is inactive, aliases are parsed but silently ignored."

## Clarifications

### Session 2026-04-30

- Q: When an alias has `pathPrefix` set, should the prefix be stripped before forwarding to the backend or passed through unchanged? → A: Make stripping configurable per alias via a `stripPrefix` boolean field (default `true`); the same backend then works behind both root and prefixed mounts without code changes, while operators can opt out for backends that expect the prefix on the wire.
- Q: How should Shrine handle a cross-application collision where two different applications would produce routers for the same host+path combination (one app's primary or alias matches another app's primary or alias)? → A: Fail the deploy with a clear error naming both applications and the colliding host+path; operators must rename or remove the colliding alias before deploy succeeds. Silent gateway tie-breaking is exactly the kind of ghost-in-the-system bug that erodes operator trust, and cross-app collisions are almost always a manifest mistake.
- Q: How strict should `pathPrefix` validation be on shape (leading slash, trailing slash, empty)? → A: Require a leading `/`; reject empty string and bare `/`; ignore trailing slash by normalizing it away internally so `/finances` and `/finances/` behave identically. Loud, early failure on the most common operator mistake (missing `/`) without churning over cosmetic differences.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Expose an Application Under Multiple Hostnames (Priority: P1)

An operator runs an application — say, a personal-finance app — that they reach today at `finances.home.lab` from inside their LAN. They also want to reach the same app from outside the LAN over their Tailscale tailnet, at `gateway.tail9a6ddb.ts.net/finances`. Today they must either give up one of the two access paths or stand up a second application manifest pointing at the same image, which doubles maintenance and breaks single-instance assumptions. With routing aliases, they declare the additional host (and optional path prefix) under a new `routing.aliases` list in the same manifest, deploy once, and reach the running app at every declared address.

```yaml
spec:
  routing:
    domain: finances.home.lab
    aliases:
      - host: gateway.tail9a6ddb.ts.net
        pathPrefix: /finances
```

**Why this priority**: This is the entire point of the feature. Without it, operators with multi-network setups (LAN + Tailscale, internal + public, dev + canary hostname) cannot expose a single app at multiple addresses without duplicating manifests. P1 because nothing else in this feature delivers value on its own.

**Independent Test**: Deploy an application with `routing.domain: finances.home.lab` and one alias `host: gateway.tail9a6ddb.ts.net, pathPrefix: /finances`. With the Traefik gateway plugin enabled, send an HTTP request to each address and verify both reach the same running container and return identical responses.

**Acceptance Scenarios**:

1. **Given** an application manifest declaring `routing.domain` plus one alias with only a `host`, **When** the operator runs `shrine deploy` with the Traefik gateway plugin active, **Then** the app is reachable at both the primary domain and the alias host, and both routes resolve to the same backend service.
2. **Given** an application manifest declaring `routing.domain` plus one alias with both `host` and `pathPrefix`, **When** the operator runs `shrine deploy` with the Traefik gateway plugin active, **Then** the app is reachable at the primary domain (root path) and at the alias host under the declared path prefix, and both reach the same backend.
3. **Given** an application manifest declaring `routing.domain` plus multiple aliases (e.g., two or more entries), **When** the operator runs `shrine deploy` with the Traefik gateway plugin active, **Then** the app is reachable at the primary domain and at every alias address, all resolving to the same backend.

---

### User Story 2 - Aliases Are Inert When the Traefik Plugin Is Not Active (Priority: P2)

An operator authoring or sharing manifests across hosts may include `routing.aliases` even when the deploying host has no Traefik gateway plugin enabled. Shrine accepts the manifest, parses the aliases, and proceeds with the deploy without emitting an error or warning. Aliases simply have no effect on hosts where no gateway plugin is consuming them — the primary `routing.domain` continues to behave exactly as it does today.

**Why this priority**: This keeps manifests portable. Operators can keep one manifest in source control and deploy it to hosts with different gateway setups without conditionalizing the file. P2 because it does not deliver new functionality on its own — it preserves manifest portability so P1 is usable in mixed environments.

**Independent Test**: Deploy an application manifest containing `routing.aliases` to a host where the Traefik gateway plugin is **not** enabled. Verify the deploy completes successfully, no error or warning about aliases is emitted, and the application's existing routing (whatever the host's setup provides for `routing.domain`) is unchanged.

**Acceptance Scenarios**:

1. **Given** a manifest with `routing.aliases` populated, **When** the operator runs `shrine deploy` on a host where no Traefik gateway plugin is enabled, **Then** the deploy completes successfully and Shrine does not emit an error related to aliases.
2. **Given** a manifest with `routing.aliases` populated, **When** the same manifest is deployed on a host with the Traefik plugin and another host without it, **Then** both deploys succeed; alias routes appear only on the Traefik-enabled host.

---

### Edge Cases

- **`aliases` field is omitted or is an empty list**: Behavior is identical to today — only the primary `routing.domain` is published. No empty-list handling is required from the operator.
- **An alias has only `host` (no `pathPrefix`)**: The alias route matches the alias host on every path (root included), like the primary domain does today.
- **An alias has both `host` and `pathPrefix`**: The alias route matches only requests to the alias host whose path is the prefix or below it. Requests to the alias host outside the prefix are not handled by this app's alias route. By default the prefix is stripped before the request reaches the backend (so a backend that serves at root keeps working unchanged); operators who run a backend that expects the prefix on the wire can set `stripPrefix: false` on the alias entry.
- **An alias sets `stripPrefix` without setting `pathPrefix`**: The `stripPrefix` value has no effect (there is no prefix to strip). The deploy succeeds; Shrine SHOULD NOT treat this as an error since the field is harmless when no prefix is declared.
- **An alias declares the same `host`+`pathPrefix` as `routing.domain` (or as another alias on the same application)**: The manifest is invalid. The deploy fails with a clear error that names the application and the duplicate alias index, so the operator can remove the redundant entry. (When the alias shares only the host but not the path prefix, no duplicate exists and the deploy succeeds — see the next bullet.)
- **Two aliases share the same `host` with different `pathPrefix` values**: Both aliases produce distinct routers and both are reachable. This is a supported configuration.
- **An alias has an empty or missing `host`**: The manifest is invalid. The deploy fails with a clear error naming the application and the offending alias entry; no partial routing config is written.
- **`aliases` is present but the application has no `routing.domain`**: The manifest is invalid. Aliases extend a primary domain — they do not replace one. The deploy fails with a clear error.
- **Alias `host` or `pathPrefix` contains characters that would corrupt the generated gateway config** (e.g., spaces, control characters): The manifest is invalid and the deploy fails with a clear error identifying the bad value. Validation does not attempt full RFC compliance — it rejects the obviously-malformed values that would break the gateway router rule.
- **`pathPrefix` is present but does not start with `/`, is empty, or is just `/`**: The manifest is invalid and the deploy fails with a clear error naming the application and the offending value. A trailing `/` (e.g., `/finances/`) is accepted and normalized internally to its no-trailing-slash form so two cosmetically different manifests produce identical gateway behavior.
- **Operator removes or changes an alias and re-deploys**: The router(s) corresponding to the removed alias are cleaned up the same way Shrine already cleans up routes for the primary domain when a manifest changes.
- **A gateway plugin other than Traefik is active**: Aliases are still parsed but silently ignored unless that plugin chooses to consume them. This spec only defines Traefik behavior.
- **Two different applications declare a colliding host+path** (one app's primary or alias matches another app's primary or alias): The deploy fails with a clear error that names both applications and the colliding host+path. No gateway config is updated for the conflicting pair until the operator removes or renames one side; other applications in the same deploy that are unaffected by the conflict are not punished by the failure (i.e., the validation error identifies exactly which applications are in conflict).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The application manifest schema MUST accept an optional `routing.aliases` field that is a list of zero or more alias entries.
- **FR-002**: Each alias entry MUST declare a `host` (required, non-empty string), MAY declare a `pathPrefix` (optional string), and MAY declare a `stripPrefix` (optional boolean; defaults to `true` when `pathPrefix` is set; ignored when `pathPrefix` is absent).
- **FR-003**: Manifest parsing MUST reject as invalid any alias entry that omits `host` or sets `host` to an empty string, surfacing a clear error that names the offending application and entry.
- **FR-004**: Manifest parsing MUST reject any manifest that declares `routing.aliases` without a populated `routing.domain`, with an error that names the application.
- **FR-005**: Manifest parsing MUST reject `host` or `pathPrefix` values that contain characters that would corrupt a generated gateway router rule (e.g., spaces, control characters), with an error that names the offending value.
- **FR-005a**: When `pathPrefix` is present, manifest parsing MUST reject any value that does not start with `/`, that is the empty string, or that consists only of `/`, with an error that names the application and the offending value. A trailing `/` is permitted and MUST be normalized away internally before the alias router is generated, so `/finances` and `/finances/` produce identical routers.
- **FR-006**: When the Traefik gateway plugin is active and an application declares one or more aliases, Shrine MUST generate, in the application's dynamic Traefik config file, one additional router per alias entry that points to the same backend service as the primary-domain router.
- **FR-007**: Alias routers MUST match the alias `host` and, when `pathPrefix` is set, match only requests whose path is at or below that prefix; when `pathPrefix` is unset, alias routers match the alias host on all paths (matching the primary-domain behavior).
- **FR-007a**: When an alias has a non-empty `pathPrefix`, the alias router MUST strip that prefix from the request path before forwarding to the backend by default (`stripPrefix: true`), so backends that serve at root can be reached unchanged behind a prefixed alias. When the operator explicitly sets `stripPrefix: false`, the alias router MUST forward the original path (prefix included) to the backend.
- **FR-008**: When alias entries would produce a router identical (same host+path matching) to one already produced by the primary domain or by another alias on the same application, manifest validation MUST fail with a clear error that names the application and the duplicate alias index. Shrine MUST NOT silently dedup such entries; loud failure matches the FR-008a cross-application stance and avoids the operator wondering why their second alias never resolved.
- **FR-008a**: When two different applications would produce routers for the same host+path combination (whether via primary domain or alias on either side), Shrine MUST fail the deploy with a clear error that names both applications and the colliding host+path. Shrine MUST NOT write or update gateway config for either side of the conflict until the operator resolves it.
- **FR-009**: Alias routers MUST be removed from the gateway dynamic config when the alias is removed from the manifest and the application is re-deployed, the same way primary-domain routers are removed today when `routing.domain` changes.
- **FR-010**: When no gateway plugin is active, or when a gateway plugin other than Traefik is active and does not consume aliases, Shrine MUST parse `routing.aliases` without error and MUST NOT emit a warning; aliases simply have no effect on routing for that deploy.
- **FR-011**: Manifests that omit `routing.aliases` entirely MUST behave exactly as they do today; this feature MUST NOT change behavior for any existing manifest.
- **FR-012**: The deploy log MUST include an observable signal (info-level or equivalent) listing the alias addresses published for each application when the Traefik plugin is active, so operators can confirm aliases took effect without inspecting generated config files.

### Key Entities

- **Routing.Aliases (manifest field)**: An optional list on an application's `routing` block. Each list entry is an alias declaring an additional address (host, optional path prefix) at which the application should be reachable. Aliases extend, never replace, the primary `routing.domain`.
- **Alias entry**: A triple of `host` (required, non-empty hostname), `pathPrefix` (optional URL path prefix), and `stripPrefix` (optional boolean; defaults to `true` when `pathPrefix` is set, no-op when not). Conceptually equivalent to "publish this app at this additional address" without standing up a second application. The `stripPrefix` flag controls whether the matched prefix is removed from the request path before the backend sees it.
- **Alias router (gateway dynamic config)**: An entry in the application's generated dynamic config that matches an alias's host (and optional path prefix) and forwards to the same backend service as the primary-domain router. One alias entry produces one alias router unless deduplication collapses it with an existing router.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator who needs to expose an existing application at a second hostname can do so by adding a single `aliases` entry to the existing manifest — no second manifest, no duplicated image reference, no extra commands beyond a normal `shrine deploy`.
- **SC-002**: After deploying a manifest with N aliases, the application is reachable at the primary `routing.domain` and at all N alias addresses, with 100% of requests to those addresses landing on the same backend instance.
- **SC-003**: The same manifest can be deployed to a host with the Traefik plugin active and to a host without it; both deploys complete successfully with zero alias-related warnings or errors on the non-Traefik host.
- **SC-004**: Removing an alias from the manifest and re-deploying removes the corresponding route from the gateway within one deploy cycle — the alias address stops resolving without operator intervention beyond `shrine deploy`.
- **SC-005**: Existing applications (manifests with no `aliases` field) experience zero behavioral change after this feature ships — first-deploy and re-deploy produce byte-identical primary-domain routing config compared to the prior release.

## Assumptions

- The Traefik gateway plugin is the only gateway plugin in scope for this feature. Other gateway plugins, if any are added later, will define their own alias-consumption semantics; the manifest field is plugin-agnostic and stable.
- "Active" means the operator has enabled the Traefik gateway plugin in Shrine's plugin configuration on the deploying host. The presence or absence of a Traefik container on the host is not directly checked — Shrine's existing plugin-active determination is reused as-is.
- The "same backend service" an alias router points to is the same backend the primary-domain router for the application already points to; alias generation reuses, not redefines, that backend wiring.
- Validation of `host` and `pathPrefix` rejects empty/whitespace/control-character values (FR-005) and shape errors on `pathPrefix` (FR-005a: must start with `/`, may not be empty or bare `/`, trailing `/` normalized away) — full RFC 1035 hostname or RFC 3986 path validation is out of scope. An operator can still write a syntactically permissive but semantically wrong host (e.g., a typo) and the deploy will succeed; the failure surfaces when DNS does not resolve, which is acceptable because Shrine cannot meaningfully tell typos from valid private hostnames.
- The cross-application collision check (FR-008a) treats the primary `routing.domain` and every alias `host`+`pathPrefix` on equal footing — a collision between an alias on app A and the primary domain on app B (or vice versa, or two primaries, or two aliases) all fail the deploy with the same error shape. This is broader than "alias-introduced collisions only," and it intentionally hardens primary-vs-primary collisions that may have been silently mis-routing in earlier releases.
- Aliases inherit TLS, middleware, and entry-point selection from the primary domain's existing configuration. This feature does not introduce per-alias TLS or middleware tuning; if an operator needs different TLS/middleware on a different host, that is out of scope for v1 and may be added later as additional alias-entry fields.
- This feature does not introduce a separate flag, env var, or migration. Existing manifests are unaffected; the new field is additive and optional.
- The "silently ignored" behavior on non-Traefik hosts is intentional, not a temporary state: it is what makes the manifest portable. Operators wanting an audit signal of "this host did not honor the aliases" should consult their gateway plugin's own deploy log; Shrine itself MUST NOT warn about a field that is simply not consumed by the active gateway.
