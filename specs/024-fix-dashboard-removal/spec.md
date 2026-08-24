# Feature Specification: Remove Stale Dashboard Config on Dashboard Removal

**Feature Branch**: `024-fix-dashboard-removal`
**Created**: 2026-08-23
**Status**: Draft
**Input**: GitHub issue [#35](https://github.com/CarlosHPlata/shrine/issues/35) — "fix: stale dashboard dynamic config survives dashboard removal; dead port-binding code tested instead of live path"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Removing the dashboard config stops the dashboard (Priority: P1)

An operator has a running Shrine deployment with the gateway dashboard enabled (port, username, and password configured). They decide to retire the dashboard — perhaps because the credentials were shared too widely or the surface is no longer needed — so they delete the dashboard block from their configuration and run a deploy. After the deploy completes, the dashboard is no longer served: the generated dashboard routing file is gone and the old credentials no longer grant access to anything.

Today, the stale generated file survives the deploy indefinitely and keeps routing the dashboard with the old credentials — the operator has no configuration-level way to turn the dashboard off once it has been deployed.

**Why this priority**: This is the headline bug and a security concern. An operator who removes the dashboard reasonably believes it is gone, yet it remains reachable with credentials they may consider revoked. Spec 010 (FR-008) already promised this behavior; this story delivers it.

**Independent Test**: Deploy with a dashboard configured, verify the dashboard is reachable, remove the dashboard block from the configuration, deploy again, and verify the generated dashboard routing file is absent and the dashboard URL is no longer routed.

**Acceptance Scenarios**:

1. **Given** a deployment where the dashboard was previously configured and its generated routing file exists, **When** the operator removes the dashboard configuration and runs a deploy, **Then** the generated dashboard routing file is removed and the gateway stops serving the dashboard.
2. **Given** the same redeploy, **When** a request is made to the former dashboard URL with the old credentials, **Then** the request is not routed to the dashboard.
3. **Given** the same redeploy, **When** the routing directory is inspected, **Then** only the generated dashboard file has been removed — all other routing files (including user-authored application configs) are untouched.
4. **Given** a deployment where the dashboard was never configured and no generated dashboard file exists, **When** a deploy runs, **Then** the deploy completes normally with no removal action and no error.

---

### User Story 2 - The removal is visible in deploy output (Priority: P2)

While the deploy from Story 1 runs, the operator watches the deploy output. The removal of the stale dashboard file is reported through the same progress reporting as every other deploy action, so the operator can confirm — without inspecting the filesystem — that the dashboard was actually torn down.

**Why this priority**: Silent cleanup of a security-relevant surface is almost as bad as no cleanup: the operator needs positive confirmation that the dashboard is gone. It builds directly on Story 1 but is separately testable through the deploy's reported events.

**Independent Test**: Run the Story 1 redeploy and assert that the deploy's progress output includes an event describing the dashboard file removal.

**Acceptance Scenarios**:

1. **Given** a redeploy that removes a stale dashboard file, **When** the deploy runs, **Then** the deploy output includes a notification that the dashboard routing file was removed.
2. **Given** a deploy where no stale dashboard file exists, **When** the deploy runs, **Then** no removal notification is emitted.
3. **Given** a stale dashboard file that cannot be removed (e.g., a filesystem permission error), **When** the deploy runs, **Then** the failure is surfaced to the operator with the file path and reason rather than being silently ignored.

---

### User Story 3 - Port-binding tests protect the shipped behavior (Priority: P3)

A maintainer modifies how the gateway derives its published port bindings (gateway port, TLS port, dashboard port). If the change breaks the shipped behavior, the unit test suite fails. Today this safety net is an illusion: the port-binding unit tests exercise an unused duplicate implementation, so a regression in the live path goes undetected, and the duplicate itself misleads readers about which code is real.

**Why this priority**: This is internal quality work with no operator-visible behavior change, but it removes a trap for maintainers: tests that pass while the shipped code is broken are worse than no tests. It is bundled here because the dead code and the stale-file bug were introduced by the same incomplete feature work.

**Independent Test**: Introduce a deliberate fault into the live port-binding behavior and confirm the unit test suite fails; restore it and confirm the suite passes. Verify the codebase contains a single port-binding implementation.

**Acceptance Scenarios**:

1. **Given** the refactored test suite, **When** a regression is introduced into the live port-binding behavior, **Then** at least one unit test fails.
2. **Given** the cleanup is complete, **When** the gateway plugin code is inspected, **Then** there is exactly one port-binding implementation and no unused file-removal or port-binding code paths remain.
3. **Given** the cleanup is complete, **When** the full test suite runs, **Then** all tests pass and the port-binding coverage that existed before (gateway port, TLS port, dashboard port combinations) is preserved against the live implementation.

---

### Edge Cases

- Dashboard configuration removed but the generated file was already deleted manually: the deploy treats the absence as success (nothing to remove), emits no removal notification, and does not error.
- Dashboard was never configured in the environment's history: no removal is attempted on any deploy.
- Dashboard configuration is present but changed (e.g., new port or credentials): existing regeneration behavior applies; this feature only governs the fully-removed case.
- The routing directory itself does not exist (e.g., first deploy against a clean host with no dashboard configured): the deploy proceeds without attempting removal and without error.
- Removal fails due to filesystem permissions: the deploy surfaces the failure with the file path and reason; the stale file's continued existence is never silent.
- Preview/dry-run deploy with the dashboard removed: the planned removal is reported but the file is not deleted.
- Other generated or user-authored files in the routing directory: never touched by the dashboard cleanup, regardless of naming.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When the dashboard is not configured and a previously generated dashboard routing file exists in the routing directory, a deploy MUST remove that file so the gateway stops serving the dashboard. "Not configured" covers every condition under which the deploy would not generate a dashboard routing file (per spec 010: dashboard block absent or no password set).
- **FR-002**: After a deploy that removes the dashboard, the gateway MUST no longer route the dashboard, and the previously configured dashboard credentials MUST no longer grant access to any surface.
- **FR-003**: The removal MUST be reported through the deploy's standard progress notification mechanism so the operator sees that the stale file was removed.
- **FR-004**: When no stale dashboard file exists (never generated, or already gone), a deploy MUST complete without error and without emitting a removal notification.
- **FR-005**: The cleanup MUST remove only the Shrine-generated dashboard routing file; all other files in the routing directory — user-authored application configs and other generated artifacts — MUST be left untouched.
- **FR-006**: If removing the stale file fails, the deploy MUST surface the failure to the operator, including the file path and the reason; the failure MUST NOT be silently swallowed.
- **FR-007**: In a preview (dry-run) deploy, the pending removal MUST be reported as a planned action and the file MUST NOT be deleted.
- **FR-008**: The codebase MUST contain exactly one implementation of gateway port-binding derivation — the one exercised in production — with the unused duplicate removed.
- **FR-009**: The port-binding unit tests MUST exercise the live implementation, preserving the existing coverage of gateway port, TLS port, and dashboard port combinations, such that a regression in shipped behavior causes a test failure.
- **FR-010**: Unused code paths left over from the incomplete spec-010 removal work (including the never-invoked file-removal hook) MUST be removed or put into productive use by this feature — no dead code may remain.

### Key Entities

- **Generated dashboard routing file**: A Shrine-owned artifact (reserved name `__shrine-dashboard.yml`) written into the routing directory's dynamic-config area when the dashboard is configured. Its lifecycle must now be symmetric: created/updated when the dashboard is configured, removed when it is not.
- **Routing directory**: The directory the gateway watches for routing configuration. Contains both Shrine-generated artifacts and user-authored application configs; only the former are subject to Shrine lifecycle management.
- **Deploy removal notification**: The operator-visible event emitted when a stale generated file is removed, delivered through the same observer/progress channel as other deploy events.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In 100% of deploys where the dashboard configuration was removed, the generated dashboard routing file is absent after the deploy and the dashboard URL is no longer routed.
- **SC-002**: After such a deploy, the old dashboard credentials grant access to zero surfaces — verified by requesting the former dashboard URL.
- **SC-003**: Operators can confirm the dashboard teardown from deploy output alone, without inspecting the filesystem, in a single deploy run.
- **SC-004**: Zero user-authored or unrelated generated files are modified or removed by the cleanup across all deploy scenarios.
- **SC-005**: A deliberately introduced regression in the live port-binding behavior is detected by the unit test suite (at least one test fails).
- **SC-006**: Deploys on environments with no stale dashboard file complete with no new warnings, errors, or notifications compared to today.

## Assumptions

- "Dashboard removed" is interpreted broadly as "the deploy would not generate a dashboard file," matching spec 010's generation conditions (dashboard block absent, or password absent) — the stale-file cleanup triggers in all such cases, not only when the whole block is deleted.
- The cleanup runs during the same deploy lifecycle stage that would otherwise regenerate the dashboard file (finalize), so no new deploy phases or commands are introduced.
- The gateway's file-provider watches the routing directory, so deleting the file is sufficient for the gateway to stop routing the dashboard — no additional gateway restart logic is required beyond what deploys already do.
- Only the reserved Shrine-generated dashboard filename is targeted; identification of the file by its reserved name is sufficient and safe because the name is namespaced (`__shrine-` prefix) and documented as Shrine-owned.
- A removal failure is surfaced through the deploy's existing error-reporting path; the deploy's overall failure semantics follow the existing convention for file-write failures during finalize.
- Repointing the port-binding tests preserves the intent and case coverage of the existing spec-011 tests; no reduction in scenario coverage is acceptable.
- The dead-code removal is behavior-neutral for operators: no configuration surface, command, or output changes other than those described in the user stories.
