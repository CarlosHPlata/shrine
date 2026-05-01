# Feature Specification: Fix Traefik Dashboard Generated in Static Config

**Feature Branch**: `010-fix-dashboard-static-config`
**Created**: 2026-05-01
**Status**: Draft
**Input**: GitHub issue [#8](https://github.com/CarlosHPlata/shrine/issues/8) — "Traefik plugin generates dashboard router/middleware in static config instead of dynamic config"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Dashboard works after a clean deploy (Priority: P1)

An operator configures Shrine with the Traefik plugin and a dashboard password, then runs `shrine deploy` against a clean environment (no pre-existing Traefik config). After the deploy, the operator opens the Traefik dashboard URL in a browser and reaches the dashboard (after entering the configured credentials). Today the dashboard returns 404 because the dashboard router is silently dropped by Traefik.

**Why this priority**: This is the headline bug. The dashboard is the primary observability surface for Traefik in a Shrine deployment, and right now it is unreachable on every clean deploy. There is no Shrine-level workaround short of hand-editing the generated config — which then fights with redeploys.

**Independent Test**: On a clean host with no prior Traefik config, run `shrine deploy` with the Traefik plugin enabled and a dashboard password set. Issue an HTTP request against the dashboard URL. The request returns an authentication challenge (not 404), and an authenticated request returns the dashboard page.

**Acceptance Scenarios**:

1. **Given** a clean environment with no existing Traefik config and a Shrine configuration with the Traefik plugin and a dashboard password, **When** `shrine deploy` completes successfully, **Then** an unauthenticated request to the dashboard URL returns an authentication challenge response (HTTP 401), not 404.
2. **Given** the same successful deploy, **When** an authenticated request is made to the dashboard URL using the configured credentials, **Then** the Traefik dashboard page is returned (HTTP 200 with dashboard content).
3. **Given** a Shrine configuration with the Traefik plugin but **no** dashboard password configured, **When** `shrine deploy` completes, **Then** no dashboard router or authentication middleware is generated in any config file (the dashboard surface is simply not exposed).

---

### User Story 2 - Generated static config is valid Traefik static config (Priority: P1)

The Traefik static configuration file produced by Shrine on a clean deploy contains only keys that Traefik actually processes as static config (such as entry points, api, providers, log). It does not contain dynamic-only sections (such as `http` routers, services, or middlewares), which Traefik silently drops at startup and which therefore mask configuration bugs.

**Why this priority**: P1 because this is the root cause of Story 1 and shares its fix. A correct static config also prevents future regressions where a contributor adds a new dynamic-shaped section to the static file and gets no error from Traefik to alert them.

**Independent Test**: After a clean `shrine deploy` with the Traefik plugin and dashboard password, inspect the generated static configuration file. Confirm it contains no top-level `http` section. Cross-check the file against Traefik's published static configuration reference and confirm every top-level key is a recognized static key.

**Acceptance Scenarios**:

1. **Given** a clean deploy with the Traefik plugin and a dashboard password, **When** the generated static configuration file is inspected, **Then** it contains no `http` section and no other dynamic-only sections.
2. **Given** the same deploy, **When** the configured file-provider dynamic directory is inspected, **Then** the dashboard router and authentication middleware appear in a dedicated dynamic configuration file in that directory.
3. **Given** the same deploy, **When** Traefik starts up against the generated configuration, **Then** Traefik logs no warnings or errors about unknown or ignored configuration keys related to the dashboard.

---

### User Story 3 - Operator edits to the generated dashboard dynamic file are preserved across redeploys (Priority: P2)

After the dashboard router and middleware are moved into a dedicated dynamic configuration file, operators may edit that file (e.g., adjust the auth realm, tighten the router rule, add IP allow-listing) the same way they already edit the static `traefik.yml` and per-application routing files. A subsequent `shrine deploy` does not silently overwrite those operator edits.

**Why this priority**: P2 because the existing project conventions (see specs 004 and 009 — preservation of `traefik.yml` and per-app routing files) treat operator-edited generated files as sacred. The new dashboard dynamic file enters that same regime, and not honouring the convention would be a smaller-but-real regression. P1 (above) is functionally complete without this; P2 closes the operator-experience gap.

**Independent Test**: Run `shrine deploy` (clean host) to produce the dashboard dynamic file. Manually edit a non-credential field in that file (e.g., add a comment or an IP allow-list middleware reference). Run `shrine deploy` again with no Shrine-side configuration changes. The operator's edits remain in the file.

**Acceptance Scenarios**:

1. **Given** a previously generated dashboard dynamic file that the operator has manually edited, **When** `shrine deploy` is run again with no relevant configuration changes, **Then** the operator's edits remain in the file.
2. **Given** a Shrine configuration change that affects the dashboard (e.g., the dashboard password is rotated) AND the existing dashboard dynamic file, **When** `shrine deploy` is run, **Then** the file is preserved unchanged and the deploy output reports the file was kept (`gateway.dashboard.preserved`); rotation is operator-driven (delete the file and redeploy, or hand-edit the credential line).

---

### Edge Cases

- **Pre-existing buggy `traefik.yml` from an earlier Shrine version** that contains an `http` block with the dashboard router: Shrine MUST leave the pre-existing static file untouched, MUST still generate the new dashboard dynamic file alongside it, and MUST emit a deploy-time warning to the operator naming the offending file and instructing them to remove the `http` block manually. Because Traefik already silently drops the dynamic-only `http` block in static config, the dashboard becomes reachable immediately on upgrade via the new dynamic file; the warning is the cleanup nudge for the now-dead legacy block.
- **Dashboard password configured but no dynamic-file directory writable**: deploy MUST fail loudly at validation time with a clear error pointing at the directory, not silently skip dashboard generation.
- **Two Shrine deployments sharing a routing directory**: the dashboard dynamic file MUST be named/located so that it does not collide with per-application dynamic routing files generated by the same Shrine instance.
- **Dashboard password removed in a subsequent deploy**: the previously generated dashboard dynamic file MUST be removed (or left in a clearly-disabled state) so that the dashboard is no longer reachable. Default expectation: removal, consistent with how Shrine treats other generated artefacts whose source manifest no longer references them.
- **Operator manually deletes the dashboard dynamic file** between deploys while the dashboard password is still configured: the next deploy MUST regenerate the file (treat absence as "needs creation", not "operator-edited").

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The Traefik static configuration file generated by Shrine MUST contain only keys that Traefik treats as static configuration (e.g., entry points, api, providers, log, certificate resolvers). It MUST NOT contain an `http` section or any other dynamic-only section.
- **FR-002**: When a dashboard password is configured, Shrine MUST generate the dashboard router and the basic-auth middleware into a dedicated dynamic configuration file located inside the file provider's watched dynamic directory.
- **FR-003**: The generated static configuration MUST configure Traefik's file provider to watch the dynamic directory in which the dashboard dynamic file lives, so that Traefik picks up the dashboard router at startup.
- **FR-004**: After a successful clean `shrine deploy` with the Traefik plugin and a dashboard password, an unauthenticated HTTP request to the dashboard URL MUST return an authentication challenge response (not 404), and an authenticated request with the configured credentials MUST return the dashboard.
- **FR-005**: When the dashboard password is **not** configured, Shrine MUST NOT generate any dashboard router or authentication middleware in any configuration file (static or dynamic).
- **FR-006**: A subsequent `shrine deploy` with unchanged dashboard-related configuration MUST preserve operator edits to the generated dashboard dynamic file, mirroring the preservation behaviour already in place for the static `traefik.yml` and per-application routing files.
- **FR-007**: When the dashboard-related Shrine configuration changes between deploys (e.g., the dashboard password is rotated), the existing dashboard dynamic file MUST be preserved with no modification, mirroring the preservation regime applied to the static `traefik.yml` and per-application routing files. Shrine MUST emit a `gateway.dashboard.preserved` event so the operator can see the file was kept. To rotate the dashboard password, operators delete the dashboard dynamic file (and re-run deploy) or edit the file's credential line directly.
- **FR-008**: When the dashboard password is removed between deploys, Shrine MUST remove the previously-generated dashboard dynamic file so that the dashboard is no longer routed, mirroring how Shrine handles other generated artefacts whose source manifest no longer references them.
- **FR-009**: The dashboard dynamic file's name and location MUST be deterministic and namespaced such that it cannot collide with per-application dynamic routing files generated by Shrine in the same directory.
- **FR-010**: When Shrine encounters a pre-existing static configuration file containing a dynamic-only `http` block (the artefact of an earlier buggy Shrine version), Shrine MUST NOT modify, rewrite, or remove that file. It MUST still generate the new dashboard dynamic file in the file provider's dynamic directory, and it MUST emit a clearly-identified warning to deploy output (and to any deploy log) that names the offending file path, identifies the offending block, and instructs the operator to remove it manually. The warning MUST be emitted on every deploy where the legacy block is still detected (not only the first), so the cleanup nudge is not lost.
- **FR-011**: The legacy-block detection in FR-010 MUST be limited to the specific dynamic-only sections that the buggy Shrine version is known to have generated (the `http` top-level section and its descendants). Shrine MUST NOT warn about, or attempt to interpret, any other content in the pre-existing static file — operator-added static keys are not Shrine's concern.

### Key Entities *(include if feature involves data)*

- **Traefik static configuration file**: The single Shrine-generated file consumed by Traefik at process start. Contains only static-configuration keys. Configures the file provider that points at the dynamic directory.
- **Dashboard dynamic configuration file**: A new Shrine-generated file inside the file provider's dynamic directory. Contains exactly the dashboard router and the basic-auth middleware. Subject to the same operator-edit preservation regime as other generated dynamic files.
- **Dynamic directory**: The directory watched by Traefik's file provider. Already exists as a concept in Shrine for per-application routing files; the dashboard dynamic file becomes a co-resident sibling of those files.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of clean Shrine deploys configured with the Traefik plugin and a dashboard password produce a dashboard URL that returns an authentication challenge (rather than 404) on the first attempt, with no manual operator edits required.
- **SC-002**: 100% of clean Shrine deploys configured with the Traefik plugin produce a static configuration file whose top-level keys are all valid static-configuration keys per the Traefik reference for the supported Traefik version, with zero dynamic-only sections present.
- **SC-003**: An operator who manually edits the dashboard dynamic file retains 100% of those edits across at least 5 consecutive `shrine deploy` runs, provided the dashboard-related Shrine configuration is unchanged.
- **SC-004**: Zero new GitHub issues filed against Shrine reporting "dashboard 404 on clean deploy" in the 30 days following the fix's release.

## Assumptions

- The fix targets the Traefik plugin's deploy-time configuration generator. Runtime Traefik behaviour, the Traefik version policy, and the broader Shrine plugin contract are out of scope for this spec.
- "Dashboard password configured" is the existing trigger Shrine already uses to decide whether dashboard surface area should be exposed; this spec inherits that trigger and does not redefine it.
- The file provider's dynamic directory already exists in the project model (per-application routing files live there) and is the natural home for the dashboard dynamic file. No new directory concept is being introduced.
- The operator-edit preservation regime established by spec 004 (preserving operator-edited `traefik.yml`) and spec 009 (preserving per-app routing files) is the reference behaviour for the new dashboard dynamic file.
- HTTPS/cert-resolver concerns for the dashboard URL are independent of this fix; the bug occurs equally over HTTP and HTTPS and the fix is the same.
- Backwards compatibility for previously-deployed (buggy) hosts is captured as the single open clarification (FR-010) rather than spread across the spec.

## Resolved Clarifications

### Question 1: Handling pre-existing buggy `traefik.yml` containing an `http` block — RESOLVED

**Decision**: Option C — leave the pre-existing static file untouched, write the new dashboard dynamic file alongside it, and emit a loud deploy-time warning naming the offending file and instructing the operator to remove the `http` block manually.

**Rationale**: Most compliant with the spirit of plugins in Shrine. Plugins generate their own artefacts and surface diagnostics; they do not reach into and rewrite operator-owned or legacy state. A practical bonus: because Traefik already silently drops the dynamic-only `http` block in static config, the dashboard becomes reachable immediately on upgrade via the new dynamic file — the warning is purely a cleanup nudge for now-dead config, not a precondition for the headline fix.

**Spec impact**: Encoded as FR-010 (warning behaviour and idempotence) and FR-011 (scope of legacy-block detection). The "pre-existing buggy `traefik.yml`" edge case has been updated to reflect this behaviour.
