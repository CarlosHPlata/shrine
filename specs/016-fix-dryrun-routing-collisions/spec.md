# Feature Specification: Detect Routing Domain Collisions in `shrine deploy --dry-run`

**Feature Branch**: `016-fix-dryrun-routing-collisions`
**Created**: 2026-05-15
**Status**: Draft
**Input**: User description: "base it upon https://github.com/CarlosHPlata/shrine/issues/21"
**Source Issue**: [#21 — `--dry-run` does not detect routing domain collisions](https://github.com/CarlosHPlata/shrine/issues/21)

## User Scenarios & Testing *(mandatory)*

### User Story 1 — `--dry-run` surfaces duplicate-domain errors before real deploy (Priority: P1)

A platform engineer prepares a manifest set in which two `Application` manifests inadvertently declare the same `routing.domain`. They run `shrine deploy --dry-run <dir>` expecting the same plan-time validation a real deploy performs. Today the dry-run exits cleanly, so the engineer believes the manifests are valid; the conflict is only revealed when the real deploy runs against the cluster. After this change, the dry-run reports the collision and identifies the conflicting applications so the engineer can fix the manifests before touching the live environment.

**Why this priority**: This is the entire point of `--dry-run`. A preview command that silently omits a class of plan-time errors actively misleads users and erodes trust in the rest of the dry-run output. Fixing it is the minimum viable change required to close the bug.

**Independent Test**: Author two `Application` manifests with the same `routing.domain` value, run `shrine deploy --dry-run` against that directory, and verify the command exits non-zero with an error naming both conflicting applications. Compare the error against the error a real `shrine deploy` produces for the same input — they must report the same collision.

**Acceptance Scenarios**:

1. **Given** a manifest directory containing two `Application` manifests declaring the same `routing.domain` (whether on the same team or different teams), **When** the user runs `shrine deploy --dry-run <dir>`, **Then** the command exits non-zero and the error output names every application participating in the collision and the duplicated domain.
2. **Given** the same manifest directory, **When** the user compares the dry-run output to the real `shrine deploy` output, **Then** the collision error reported by both paths refers to the same applications and the same domain.
3. **Given** a manifest directory with no duplicate `routing.domain` values, **When** the user runs `shrine deploy --dry-run <dir>`, **Then** the command continues to succeed exactly as it does today, with no new false-positive collision errors.

---

### User Story 2 — Collision detection runs independently of routing backend wiring (Priority: P2)

A platform engineer runs `shrine deploy --dry-run` against an environment where the routing backend (e.g., Traefik) is not configured, or against a manifest set that does not yet target a real cluster. Currently the collision check is gated on a successfully constructed routing backend, so engineers in these situations can author conflicting manifests with no warning. After this change, collision detection runs as part of plan-time validation regardless of whether a routing backend is wired, because the conflict is a property of the manifests themselves.

**Why this priority**: Without this, the P1 fix would still leave a gap: dry-run users without a configured routing backend would continue to receive false-clean runs. Closing that gap makes the validation predictable for every dry-run invocation, but it is secondary to surfacing the error at all.

**Independent Test**: Run `shrine deploy --dry-run` on a manifest set with duplicate `routing.domain` values in an environment where no routing backend is configured. Verify the collision is still reported.

**Acceptance Scenarios**:

1. **Given** a manifest set with duplicate routing domains and an environment with no routing backend configured, **When** the user runs `shrine deploy --dry-run`, **Then** the collision is still reported.
2. **Given** a manifest set with duplicate routing domains and an environment with a routing backend configured, **When** the user runs `shrine deploy --dry-run`, **Then** the collision is reported once (not duplicated by a second validation pass).

---

### Edge Cases

- **Multiple collisions in one manifest set**: when three or more applications share a domain, or when several independent collision groups exist, the user must be able to see every conflicting application from a single dry-run invocation — the command must not stop at the first collision.
- **Same team vs. different teams**: a collision between two applications under the same team and a collision between applications under different teams are both real conflicts at deploy time. Both must be reported.
- **Empty / missing `routing.domain`**: applications that do not declare a `routing.domain` must not be flagged as colliding with each other or with anything else. Only applications that actually request a domain participate in the check.
- **Collision-free manifests**: dry-run on manifests without duplicate domains must continue to exit successfully with no new diagnostics introduced by this change.
- **Parity with real deploy**: any future change that adjusts collision-detection rules (e.g., domain normalization) must apply to both `deploy` and `deploy --dry-run` in lockstep, because users now rely on the dry-run as a faithful preview.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `shrine deploy --dry-run` MUST detect routing domain collisions across all `Application` manifests in the target manifest set.
- **FR-002**: The collision check MUST execute as part of plan-time validation, independent of whether a routing backend (such as Traefik) is configured or successfully initialized.
- **FR-003**: When duplicate routing domains are detected, `shrine deploy --dry-run` MUST exit with a non-zero status and the same collision diagnostic that `shrine deploy` would emit for the same input.
- **FR-004**: Each collision diagnostic MUST identify every application participating in the collision by a stable identifier (such as name and team) and MUST name the duplicated domain, so users can locate the offending manifests.
- **FR-005**: A single `--dry-run` invocation MUST report all collisions present in the manifest set, not only the first one encountered, so users can resolve them in one editing pass.
- **FR-006**: Applications that do not declare a `routing.domain` MUST be excluded from the collision check and MUST NOT be reported as conflicting with anything.
- **FR-007**: When no collisions exist, `shrine deploy --dry-run` MUST continue to succeed with no new diagnostics introduced by this change.
- **FR-008**: Collision detection MUST be invoked exactly once per `--dry-run` (or `deploy`) invocation; the diagnostic for a given collision MUST NOT be duplicated.

### Key Entities

- **Application manifest**: a user-authored declaration that may include a `routing.domain` value. Identified for diagnostic purposes by its name and team.
- **Manifest set**: the collection of application manifests passed to a single `shrine deploy` or `shrine deploy --dry-run` invocation. The unit over which collision detection runs.
- **Routing domain collision**: a condition in which two or more applications in the same manifest set request the same `routing.domain`. A property of the manifests, not of the runtime environment.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For every manifest set containing one or more routing-domain collisions, `shrine deploy --dry-run` and `shrine deploy` produce equivalent collision diagnostics (same set of conflicting applications, same duplicated domains) — measured by exercising both paths against a shared fixture set and confirming 100% parity.
- **SC-002**: From a single `--dry-run` invocation on a manifest set with N independent collisions, users can identify all N collisions without re-running the command.
- **SC-003**: On collision-free manifest sets, `shrine deploy --dry-run` continues to exit successfully with zero false-positive collision diagnostics across the regression fixture set.
- **SC-004**: Removing the routing backend configuration from a host has no effect on whether `--dry-run` reports manifest-level routing-domain collisions on that host.
- **SC-005**: The change introduces no measurable regression in `--dry-run` runtime on collision-free manifest sets at typical user scale (tens of applications), so the validation cost is invisible to engineers running it interactively.

## Assumptions

- The existing collision-detection algorithm used by the `deploy` path is correct and is the reference behavior; this work concerns where and when that check runs, not the algorithm itself.
- The application manifest schema and the semantics of `routing.domain` remain unchanged. No new manifest fields are introduced as part of this fix.
- Domain values are compared using whatever comparison the current collision check performs (the bug is about coverage, not normalization). Any change to how domains are matched (e.g., case-insensitive comparison) is out of scope for this feature and would be filed separately.
- The fix is scoped to the planner / handler boundary so that both `Deploy` and `DryRun` flow through the same validation. No CLI flags, output formats, or user-facing commands are added or renamed.
- Collision diagnostics surfaced to the user are textual error messages already produced by the existing check; this feature does not introduce a new diagnostic format.
