# Feature Specification: Per-Alias Opt-Out of Path Prefix Stripping

**Feature Branch**: `008-alias-strip-prefix`
**Created**: 2026-05-01
**Status**: Draft
**Input**: GitHub issue #9 — "Add stripPrefix option to routing.aliases to control whether path prefix is stripped before forwarding"

## Clarifications

### Session 2026-05-01

- Q: How should this feature be scoped given that the `stripPrefix` field was already shipped in PR #7? → A: Audit-then-fix-gaps — planning starts by verifying each FR against `main`; only un-satisfied FRs receive new implementation work. Issue #9 closes once any remaining gaps (most likely FR-008 documentation) ship.
- Q: Where must the FR-008 Next.js opt-out documentation live? → A: Two homes — (1) the canonical manifest schema contract at `specs/006-routing-aliases/contracts/manifest-schema.md`, and (2) the operator-context files (`AGENTS.md` and/or `CLAUDE.md`) so Claude surfaces the opt-out automatically when an operator describes a redirect-loop symptom.
- Q: Should the deploy log indicate which aliases forward the prefix unchanged (`stripPrefix: false`)? → A: Yes — extend the existing per-alias log line from spec 006 FR-012 with a marker (e.g., `(no strip)`) when the alias has `pathPrefix` set and `stripPrefix: false`. Same line, same level, one extra token; default-stripping aliases keep their current log shape unchanged.
- Q: How deep should test coverage go for the `stripPrefix: false` path? → A: Lightweight verification — confirm existing unit tests in PR #7 cover FR-002/FR-003, and add (only if missing) an integration-level assertion that inspects the generated dynamic-config YAML for a `stripPrefix: false` alias and verifies no strip middleware is attached. No new HTTP-level or container-level reproduction of the Next.js failure mode (cost not justified given the slow integration suite).

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Publish a Next.js app under a Tailscale alias without redirect loops (Priority: P1)

An operator runs a Next.js application — a personal-finance app — that is reachable inside their LAN at `finances.home.lab` and from their tailnet at `gateway.tail9a6ddb.ts.net/finances`. The app is configured internally with `basePath: '/finances'` in `next.config.ts`, which is the Next.js way of saying "I own the `/finances` prefix; route every page and asset under it."

Today, when the operator declares the tailnet hostname as a `routing.aliases` entry with `pathPrefix: /finances`, Shrine assumes the backend serves at root and rewrites the request to strip `/finances` before forwarding. The Next.js process never sees the prefix it expects. Its first response redirects the browser to `/_next/static/...` (no prefix), the browser follows that path through the alias router, no router matches, and every static asset 404s. The page is unusable.

The operator needs a way to say, on a single alias entry, "this backend handles the prefix itself — please forward the full path." With that opt-out, Shrine emits no strip middleware for that alias, the Next.js process receives `/finances/...` exactly as the browser sent it, and asset URLs the framework generates resolve correctly through the same alias router on subsequent requests.

```yaml
spec:
  routing:
    domain: finances.home.lab
    aliases:
      - host: gateway.tail9a6ddb.ts.net
        pathPrefix: /finances
        stripPrefix: false   # app handles basePath internally
```

**Why this priority**: This is the entire feature. Operators with any framework that owns its basePath internally (Next.js, Nuxt, Astro, Grafana sub-path mode, JupyterLab, etc.) cannot today expose that app under a prefixed alias without forking the app or running a separate root-mounted instance. P1 because no other piece of this feature delivers value on its own.

**Independent Test**: Deploy an application whose backend echoes the request path it receives, with `routing.domain` set to a primary host and a single alias declaring `host: gateway.example`, `pathPrefix: /finances`, `stripPrefix: false`. Send `GET http://gateway.example/finances/whoami`; verify the backend echoes `/finances/whoami` (not `/whoami`). Then change the alias to `stripPrefix: true` (or remove the field) and re-deploy; verify the backend now echoes `/whoami`. Both cases use the same backend container — only the gateway middleware composition changes.

**Acceptance Scenarios**:

1. **Given** a manifest declaring `routing.domain` plus one alias with `pathPrefix: /finances` and `stripPrefix: false`, **When** the operator runs `shrine deploy` with the Traefik gateway plugin active, **Then** the generated dynamic Traefik config for the alias router has no strip middleware attached — requests to the alias reach the backend with the prefix intact.
2. **Given** a manifest declaring `routing.domain` plus one alias with `pathPrefix: /finances` and `stripPrefix: true` (or no `stripPrefix` field at all), **When** the operator runs `shrine deploy` with the Traefik gateway plugin active, **Then** the generated config attaches a strip middleware to the alias router that removes the `/finances` prefix before forwarding.
3. **Given** a manifest with multiple aliases on one application, some with `stripPrefix: false` and some with `stripPrefix: true`, **When** the operator runs `shrine deploy`, **Then** each alias router gets exactly the middleware composition its own `stripPrefix` value implies — they do not influence each other.
4. **Given** an existing manifest that uses `routing.aliases` without ever setting `stripPrefix`, **When** the operator re-deploys after this feature ships, **Then** the generated dynamic config is byte-identical to what was generated before (default behavior is preserved).

---

### Edge Cases

- **`stripPrefix: false` on an alias with no `pathPrefix`**: There is no prefix to strip, so the field has no effect. The deploy succeeds; Shrine MUST NOT treat this as an error. The alias router matches the alias host on all paths and forwards every path unchanged, exactly as if `stripPrefix` were absent.
- **`stripPrefix: true` (explicit) on an alias with no `pathPrefix`**: Same as the previous case — no prefix to strip, so the field is a no-op. No strip middleware is generated.
- **`stripPrefix` field omitted on an alias with `pathPrefix` set**: Behavior is identical to today's release — Shrine attaches a strip middleware. This is the implicit default that preserves backwards compatibility.
- **`stripPrefix` field omitted on an alias with no `pathPrefix`**: Still a no-op. No middleware, no strip.
- **`stripPrefix: false` with `pathPrefix: /` or empty**: The `pathPrefix` value is already invalid per existing alias validation (FR-005a in spec 006), so the deploy fails on `pathPrefix` shape before `stripPrefix` is considered. The error message is about the `pathPrefix`, not about `stripPrefix`.
- **A trailing slash on `pathPrefix` (e.g. `/finances/`) combined with `stripPrefix: false`**: The trailing slash is normalized away by existing alias parsing before the alias router is generated. Whether or not stripping happens, the operator-visible `pathPrefix` in the generated router is the no-trailing-slash form.
- **Operator changes `stripPrefix` from `true` to `false` (or vice versa) and re-deploys**: The alias router and its associated strip middleware are regenerated to match the new value. The previous middleware (if any) is replaced cleanly the same way other manifest changes propagate today; no orphaned strip middleware remains in the dynamic config file.
- **A non-Traefik gateway plugin is active**: `stripPrefix` is parsed (so the manifest is portable across hosts) and silently ignored, in line with how the rest of `routing.aliases` behaves on non-Traefik hosts (spec 006 FR-010). Other gateways that later choose to consume aliases will define their own `stripPrefix` semantics.
- **The same backend serves both a primary `routing.domain` (root) and a `stripPrefix: false` alias under a prefix**: Whether this works depends on the backend, not on Shrine. A Next.js app with `basePath: /finances` typically only handles requests under `/finances`, so root-domain requests will 404 inside the app. Shrine's job is to forward what the operator asked for; choosing whether the backend is reachable at both addresses is the operator's responsibility.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The application manifest schema MUST already accept (and continue to accept) `stripPrefix` as an optional boolean field on each `routing.aliases` entry, alongside `host` and `pathPrefix`.
- **FR-002**: When an alias entry has a non-empty `pathPrefix` and `stripPrefix` is unset or `true`, the Traefik gateway plugin MUST attach a strip middleware to the generated alias router that removes the `pathPrefix` from the request path before forwarding to the backend. This is the existing default and MUST NOT change.
- **FR-003**: When an alias entry has a non-empty `pathPrefix` and `stripPrefix` is explicitly `false`, the Traefik gateway plugin MUST NOT generate a strip middleware for that alias router. The alias router MUST forward the original request path (prefix included) to the backend.
- **FR-004**: When an alias entry has no `pathPrefix` (or an empty one), `stripPrefix` MUST be a no-op regardless of value. The Traefik gateway plugin MUST NOT generate a strip middleware for such aliases.
- **FR-005**: `stripPrefix` settings on one alias entry MUST NOT influence the middleware composition of any other alias router on the same application or of the primary-domain router. Each alias is independent.
- **FR-006**: Manifest validation MUST accept `stripPrefix: false` (and `stripPrefix: true`, and an absent field) without emitting an error or warning. No new manifest validation rule is introduced by this feature beyond what already exists for `routing.aliases`.
- **FR-007**: When an operator changes the `stripPrefix` value on an existing alias and re-deploys, the gateway dynamic config MUST be rewritten so the alias router and its middleware composition reflect the new value. Stale strip-middleware entries from the previous deploy MUST NOT remain in the rewritten config file.
- **FR-008**: The `stripPrefix` opt-out MUST be documented in two places so operators can self-serve from either the schema reference or the in-repo agent context: (a) the manifest schema contract at `specs/006-routing-aliases/contracts/manifest-schema.md` MUST describe `stripPrefix` (type, default of `true` when `pathPrefix` is set, no-op when `pathPrefix` is absent) and include at least one canonical example showing `stripPrefix: false` for a framework that owns its basePath (Next.js or equivalent); (b) the canonical agent-context file `AGENTS.md` (per Constitution governance: "AGENTS.md is the runtime development reference") MUST surface the opt-out and the redirect-loop / asset-404 symptom that triggers it, so Claude can recommend the fix automatically when an operator describes the failure mode.
- **FR-009**: Existing manifests that do not set `stripPrefix` on any alias MUST continue to produce byte-identical dynamic Traefik config compared to the prior release. This feature MUST NOT cause incidental config churn for unaffected applications.
- **FR-010**: When the Traefik gateway plugin logs the alias addresses published for an application (per spec 006 FR-012), each alias that has `pathPrefix` set and `stripPrefix: false` MUST be annotated with the literal suffix `(no strip)` (per `contracts/log-format.md`) on its existing log line, so an operator can confirm at a glance which aliases forward the prefix unchanged. Aliases with `stripPrefix: true` (or unset, or with no `pathPrefix`) MUST log unchanged from the prior release — no marker, no extra log line.

### Key Entities

- **Alias entry (`stripPrefix` field)**: An optional boolean on a single `routing.aliases` entry. Defaults to `true` when `pathPrefix` is set; treated as a no-op when `pathPrefix` is absent. Operator-facing meaning: "when the gateway forwards a request that matched my `pathPrefix`, should it remove the prefix from the path, or pass the original path through?"
- **Strip middleware (gateway dynamic config)**: A Traefik `stripPrefix` middleware entry generated per alias router that needs prefix stripping. One alias entry produces at most one strip middleware; an alias with `stripPrefix: false` (or no `pathPrefix`) produces zero.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator who is hitting the redirect-loop / asset-404 failure mode on a basePath-aware backend (Next.js or equivalent) can resolve it by adding `stripPrefix: false` to the affected alias entry and re-deploying — no other manifest, code, or container change is required.
- **SC-002**: After a deploy in which an alias has `stripPrefix: false`, 100% of requests to that alias arrive at the backend with the original path (prefix included), and 0% arrive with the prefix stripped.
- **SC-003**: After a deploy in which an alias has `stripPrefix: true` or omits the field, 100% of requests to that alias arrive at the backend with the prefix removed, matching pre-feature behavior.
- **SC-004**: An existing application whose manifest never references `stripPrefix` produces a generated dynamic Traefik config that is byte-identical before and after this feature ships — no churn, no diff in source-controlled config snapshots.
- **SC-005**: The shipped manifest documentation contains at least one worked Next.js example showing `stripPrefix: false` in the manifest schema contract, AND the `AGENTS.md` / `CLAUDE.md` operator-context files describe the redirect-loop symptom and the opt-out fix, so an operator hitting the bug can find the resolution either by reading the schema reference or by asking Claude.

## Assumptions

- The Traefik gateway plugin is the only gateway plugin in scope. Other gateway plugins, if added later, will define their own `stripPrefix` semantics; the manifest field stays plugin-agnostic and stable.
- The default of `true` (when `pathPrefix` is set) is the correct default because it preserves the behavior shipped in spec 006 (`routing-aliases`). Flipping the default to `false` would silently change behavior for every operator already relying on automatic stripping; that is out of scope and intentionally not done here.
- This feature is *not* about introducing the `stripPrefix` field — that field was already specified in spec 006 and shipped in PR #7 (commit `1a31dac`). This spec exists to formalize the user-facing problem (the basePath / redirect-loop failure mode reported in issue #9), nail down the acceptance scenarios from the operator's point of view, and ensure the shipped documentation surfaces the opt-out clearly enough that operators can self-serve.
- **Scope is audit-then-fix-gaps** (per Clarifications, 2026-05-01): the plan MUST begin by verifying each FR above against the current `main` branch. Only FRs that are not actually satisfied today receive new implementation work; FRs already satisfied are recorded as "verified, no change required" in the plan and tasks artifacts. The expected gap on entry is FR-008 (operator-facing documentation with a Next.js example), but the audit governs the actual deliverable list.
- "Backend handles the prefix internally" is operator-asserted, not Shrine-detected. Shrine cannot inspect a container image to know whether it is Next.js with `basePath: /finances` or not. The operator declares intent via `stripPrefix: false` and accepts responsibility for matching the backend's expectation.
- Per-alias TLS, middleware ordering, and entry-point selection remain inherited from the primary domain (consistent with spec 006). This feature does not introduce any new alias-level configuration beyond what `stripPrefix` already provides.
- The fix is delivered by the manifest field plus its documentation; no separate flag, env var, or migration is introduced. Existing manifests are untouched.
- **Test strategy is lightweight verification** (per Clarifications, 2026-05-01): the audit step records whether PR #7's existing unit tests in `routing_test.go`, `engine_aliases_test.go`, `validate_test.go`, and `parser_test.go` already prove FR-002 / FR-003 / FR-004 / FR-005 / FR-006. Any FR not yet covered receives a unit-level test (or, only if no unit-level reach exists, a structural integration-level YAML assertion that the generated dynamic-config has no strip middleware for a `stripPrefix: false` alias). No HTTP-level or real-backend reproduction of the Next.js redirect-loop is in scope — the bug class is "wrong middleware composition," and structural coverage of middleware emission is sufficient evidence.
