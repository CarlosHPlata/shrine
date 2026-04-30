# Phase 0 Research: Routing Aliases

**Feature**: 006-routing-aliases
**Date**: 2026-04-30

This document resolves the open questions captured in `plan.md` Phase 0 with concrete decisions, rationales, and rejected alternatives. The decisions feed directly into the Phase 1 data model and contracts.

---

## R1. Traefik `StripPrefix` middleware shape (file provider)

**Decision**: Render an in-file middleware named `<team>-<service>-strip-<index>` of kind `stripPrefix` with `prefixes: [<pathPrefix>]`, and attach it to the alias router via `middlewares: [<name>]`.

```yaml
http:
  routers:
    team-a-hello-api-alias-0:
      rule: "Host(`gateway.tail9a6ddb.ts.net`) && PathPrefix(`/finances`)"
      service: team-a-hello-api
      entryPoints: [web]
      middlewares: [team-a-hello-api-strip-0]
  middlewares:
    team-a-hello-api-strip-0:
      stripPrefix:
        prefixes: [/finances]
  services:
    team-a-hello-api:
      loadBalancer:
        servers:
          - url: http://team-a.hello-api:8080
```

**Rationale**:
- File-provider middlewares live in the same dynamic file as the router that references them; cross-file references work but make removal logic harder to reason about.
- Naming `<team>-<service>-strip-<index>` mirrors the existing `<team>-<service>` router/service naming convention in `routing.go`, gives a deterministic byte-stable output, and makes orphan middlewares trivially detectable in test assertions.
- One middleware per alias-with-strip is fine even when prefixes coincide — the per-alias index keeps router → middleware wiring 1:1, which we audit in unit tests. The `prefixes` list field is one element each; we do not coalesce middlewares.

**Alternatives rejected**:
- *Reuse a single `strip` middleware with all prefixes*: tempting but breaks 1:1 router→middleware traceability and would require diffing the prefix list on re-deploy. YAGNI.
- *Use Traefik labels via Docker provider*: out of scope — Shrine's Traefik plugin uses the file provider exclusively, and the constitution forbids backend-specific logic in the engine core.

---

## R2. Where cross-app collision detection lives

**Decision**: A new function `planner.detectRoutingCollisions(set *ManifestSet) error` runs once, immediately after `Load()`/`Plan()` constructs the `ManifestSet` and before the engine `ExecuteDeploy` call. It returns a multi-error (list of `(host, pathPrefix, app1, app2)` tuples) using the same `\n- ` delimited format as `manifest.Validate`.

**Rationale**:
- Constitution I requires multi-error reports — a per-app validate pass cannot see siblings, so detection must be set-level.
- Running it in the planner (where the full manifest set already exists) avoids a separate "collision validator" subsystem (Constitution IV — YAGNI). The planner is also where teardown/dependency analysis already happens, so this is its natural neighborhood.
- Failing before `ExecuteDeploy` means zero gateway-side side effects on collision (FR-008a: "MUST NOT write or update gateway config for either side of the conflict").

**Alternatives rejected**:
- *Detect inside the Traefik plugin's `WriteRoute`*: backend-specific; would mean other future routing backends each re-implement collision detection. Constitution III: backends should not own cross-app validation.
- *Detect lazily during engine `deployApplication` via a registry*: works, but the first colliding app would already have its router written before the second one is checked. This violates FR-008a's "MUST NOT write...until the operator resolves it."
- *Detect at manifest parse time*: a single manifest file rarely contains both colliding apps. The collision is a set-level invariant, not a per-manifest one.

---

## R3. Engine surface: widen `WriteRouteOp` vs new `WriteAppRoutesOp`

**Decision**: Widen `engine.WriteRouteOp` with a single new field `AdditionalRoutes []AliasRoute` (where `AliasRoute = struct{ Host, PathPrefix string; StripPrefix bool }`). The primary domain stays in `Domain`/`PathPrefix`/`Service*` fields as today; aliases ride along in the new slice. The Traefik backend renders all of them into a single dynamic config file per app.

**Rationale**:
- Backwards-compatible: callers that don't set `AdditionalRoutes` get exactly today's behavior — meeting SC-005 (zero behavioral change for existing manifests).
- One file per app continues to map 1:1 with `RemoveRoute(team, host)` cleanup semantics; the existing `routeFileName(team, name)` stays the unique key. No new remove op needed.
- A single op call is the natural unit because all routers share one backend service; splitting into multiple `WriteRoute` calls would require the backend to merge dynamic-config files, which is more complex and error-prone.
- "Three concrete usages" rule (Constitution IV): `AliasRoute` has only one usage today (Traefik plugin), but it's plain data — not an abstraction. Adding a field to a struct is not an abstraction violation.

**Alternatives rejected**:
- *New `WriteAppRoutesOp` taking a list of routes*: cleaner type-theoretically but doubles the backend interface surface area. The `RoutingBackend` interface today has two methods (`WriteRoute`, `RemoveRoute`); adding a third (or replacing `WriteRoute`) would force the dry-run/mock backends used in tests to grow as well, for marginal gain.
- *Per-alias `WriteRoute` calls with the backend internally merging files*: violates the "one write = one observable side effect" pattern the existing engine follows; harder to reason about partial-failure semantics if the second alias fails after the first wrote a file.

---

## R4. Is `stripPrefix` retroactively added to the primary `routing.domain`?

**Decision**: No. The existing `Routing.PathPrefix` on the primary domain keeps its current behavior (no strip — backend sees the full path). `stripPrefix` is added only to alias entries. If operators eventually need strip on the primary, that is a follow-up feature with its own spec.

**Rationale**:
- Spec scope: the user's input and the clarification session both describe `stripPrefix` as an alias-entry field. Touching primary-domain semantics retroactively is a behavior change to existing manifests, violating SC-005.
- The asymmetry is documented explicitly in the data model and the manifest contract so operators are not surprised. In practice, the primary domain typically uses no `pathPrefix`, so the asymmetry is invisible to the common case.

**Alternatives rejected**:
- *Apply default `stripPrefix: true` to the primary domain too*: would silently change behavior for any existing manifest using `routing.pathPrefix`. Hard rejection — SC-005 is non-negotiable.
- *Add `stripPrefix` to the primary domain in a follow-up but document parity here*: speculative; do not design for hypothetical future requirements (Constitution IV).

---

## R5. Observer event for alias-listing log (FR-012)

**Decision**: Reuse the existing `routing.configure` event with a new `aliases` field carrying a comma-separated list of `host[+pathPrefix]` strings (sorted lexically for byte-stable output). When the app has no aliases, the field is omitted. No new event name is introduced.

```text
routing.configure status=info domain=finances.home.lab port=8080 aliases=gateway.tail9a6ddb.ts.net+/finances
```

**Rationale**:
- Reusing the event name keeps the deploy log shape stable and reduces test churn (existing scenarios don't grow new event-name expectations).
- The field is optional/omitted for alias-less apps, so existing log assertions (e.g., `engineintegration` tests that verify deploy events) keep passing without modification.
- Sorting the alias list before joining gives deterministic output, which both humans and integration test assertions can rely on.

**Alternatives rejected**:
- *New `routing.alias.publish` event per alias*: would multiply events linearly with alias count and force every consumer (terminal logger, JSON output, integration tests) to learn a new event name. Marginal benefit.
- *Log the strip flag in the field*: noise — operators care about addresses, not transformation details. The flag is observable in the generated config file and is asserted in unit tests.

---

## Summary of decisions

| ID | Question | Decision |
|----|----------|----------|
| R1 | StripPrefix YAML shape | `stripPrefix` middleware named `<team>-<service>-strip-<index>`, attached via `middlewares:` on the router |
| R2 | Where collision check lives | `planner.detectRoutingCollisions(set)` called once before `ExecuteDeploy`; multi-error report; no gateway side effects on failure |
| R3 | Engine surface for aliases | Widen `WriteRouteOp` with `AdditionalRoutes []AliasRoute`; one op call per app, one dynamic config file per app |
| R4 | `stripPrefix` on primary domain | No — alias-only; primary keeps today's no-strip semantics |
| R5 | Alias log signal | Reuse `routing.configure`, add optional `aliases` field with sorted comma-joined list |

All Phase 0 unknowns resolved. Proceeding to Phase 1.
