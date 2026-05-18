# Feature Specification: Backend lifecycle finalize step

**Feature Branch**: `018-routing-backend-finalize`
**Created**: 2026-05-17
**Status**: Draft
**Input**: User description: "Plugin post-engine callback lives outside any interface — add Finalize() to RoutingBackend so engine owns the full deploy lifecycle (GitHub issue #23)"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - New routing backend integrates without handler changes (Priority: P1)

A contributor implements an alternative routing backend (for example, one that talks to a cloud load balancer API and must batch-publish at the end, or one that swaps a configuration atomically after all routes are staged). They wire it into Shrine the same way the existing Traefik backend is wired in. The deploy command works end-to-end — including any commit/publish step the new backend needs — without modifying the deploy handler, the CLI command, or any code outside the plugin itself.

**Why this priority**: This is the entire reason for the change. The current handler reaches into a plugin-specific method (`plugin.Deploy()`) to publish accumulated routing config, so the "pluggable routing backend" abstraction does not actually cover the full routing lifecycle. Until a finalize seam exists on the interface, every new routing backend that needs a closing phase will have to either expose its own out-of-interface lifecycle method (replicating the leak) or get bespoke glue in the handler.

**Independent Test**: Replace the Traefik routing backend in the deploy bundle with a test double that records lifecycle calls. Run a deploy against a manifest that produces at least one route. Confirm that (a) the double's per-route writes are called during the step loop, (b) its finalize call is invoked exactly once after the step loop, (c) no plugin-specific lifecycle method is called from the handler, and (d) the deploy succeeds.

**Acceptance Scenarios**:

1. **Given** a deploy bundle whose routing backend is a fresh implementation that only satisfies the public routing-backend interface, **When** `shrine deploy` runs against a manifest with at least one routed application, **Then** the deploy command exits successfully and the backend's finalize phase has been invoked after all routes were written.
2. **Given** a routing backend whose finalize phase returns an error (e.g., remote publish fails), **When** `shrine deploy` runs, **Then** the deploy command exits non-zero, the error is surfaced to the operator with enough context to identify it as a routing publish failure, and no further deploy steps are attempted.
3. **Given** a routing backend implementation that has nothing to do at the end of a deploy (no batching, no remote publish), **When** `shrine deploy` runs, **Then** the deploy completes successfully and the implementation incurs no additional work beyond a no-op call.

---

### User Story 2 - Existing Traefik deploy keeps working unchanged from the operator's perspective (Priority: P1)

An operator who already uses Shrine with the Traefik plugin runs `shrine deploy` after this change. They see the same outcome they saw before: containers are created, per-route configuration is written, and the remote Traefik instance ends up serving the new routes. They notice no behavior change, no new flags, no new failure modes, and no migration step.

**Why this priority**: The change is an internal refactor. If real-world deploys regress, the refactor is a failure regardless of how clean the new abstraction looks.

**Independent Test**: Run the existing deploy integration test suite against the refactored code path. All previously passing tests must still pass without modification to manifests, configuration files, or invocation flags.

**Acceptance Scenarios**:

1. **Given** a Shrine setup that successfully deployed before this change with the Traefik routing backend, **When** the operator runs `shrine deploy` against the same manifests after the change, **Then** the outcome (containers running, routes served by Traefik) is identical and no new operator-facing steps are required.
2. **Given** the same Traefik setup, **When** the operator runs `shrine deploy --dry-run`, **Then** dry-run output is unchanged or differs only in ways that are clearly equivalent (the dry-run finalize is a no-op or an explicit "would publish" line consistent with the rest of the dry-run output style).

---

### User Story 3 - Deploy handler stops knowing about plugin-specific lifecycle (Priority: P2)

A maintainer reviewing the deploy handler reads a single linear flow: plan → engine executes deploy → done. They can trace the routing lifecycle entirely through the routing-backend abstraction without having to know which specific plugin is wired in.

**Why this priority**: This is the maintainability payoff and the assertion the architecture is supposed to make. It is testable by code-shape, not by user behavior, so it ranks below the operator-visible guarantees.

**Independent Test**: A reviewer inspects the deploy handler after the change. They confirm the handler no longer references any specific plugin's lifecycle method (no direct `TraefikPlugin.Deploy()` call or equivalent); the routing lifecycle is driven entirely from the engine.

**Acceptance Scenarios**:

1. **Given** the deploy handler source after the change, **When** a reviewer searches it for plugin-specific lifecycle calls, **Then** none remain — every routing lifecycle action goes through the routing-backend abstraction.
2. **Given** the engine source after the change, **When** a reviewer reads `ExecuteDeploy`, **Then** the finalize step appears explicitly in the deploy flow and runs exactly once per deploy, after the per-step loop.

---

### Edge Cases

- **Deploy produces zero routed applications**: The finalize call is still made (or explicitly skipped under a documented rule), and the deploy completes without error. The behavior must be deterministic — the routing backend must not be left in a "half-initialized" state where a later deploy in the same process behaves differently.
- **Engine step loop fails partway through**: Finalize is NOT called for the failed deploy (we do not commit a partial routing state). The user sees the underlying step error, not a finalize error.
- **Finalize itself fails**: The deploy exits non-zero and the error is attributable to the finalize phase, not silently swallowed or merged into an unrelated step error.
- **Teardown path**: Teardown also invokes the routing backend's finalize phase after its per-step loop (FR-005). For the current Traefik backend this is a no-op; the seam exists for backends that batch removals. If the teardown step loop fails partway through, finalize is not invoked, mirroring the deploy rule (FR-004).
- **Dry-run engine**: The dry-run routing backend must satisfy the new interface and produce dry-run output that matches the production lifecycle (so that the dry-run command remains a faithful preview).
- **Backend implementations that don't need finalization**: The cost of "I have nothing to do" must be near-zero — a single no-op method, not boilerplate per implementation.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The routing-backend abstraction MUST expose a finalize lifecycle phase that runs after all per-route writes for a single deploy invocation have completed. This phase is the seam where batch-publish, atomic-swap, remote-config-upload, transaction-commit, or "do nothing" implementations all fit.
- **FR-002**: The deploy execution path MUST invoke the routing backend's finalize phase exactly once per successful deploy, after the per-step loop completes. The deploy command MUST NOT call any plugin-specific lifecycle method outside the routing-backend abstraction.
- **FR-003**: If the routing backend's finalize phase returns an error, the deploy command MUST exit non-zero and the error MUST be surfaced to the operator with enough context (event/log entry attributable to routing finalize) to distinguish it from a per-step error.
- **FR-004**: If the per-step deploy loop fails before completion, the routing backend's finalize phase MUST NOT be invoked for that deploy. Partial routing state MUST NOT be committed implicitly.
- **FR-005**: Teardown MUST also invoke the routing backend's finalize phase exactly once after `ExecuteTeardown`'s per-step loop completes successfully. This keeps the lifecycle symmetric and lets backends that batch-commit removals (atomic swap, remote re-publish without the removed routes) do so the same way they batch-commit additions. For the current Traefik backend, the teardown-finalize call is a no-op because per-route file removals are complete after the step loop; the seam is added for future backends and to keep the engine's deploy/teardown shapes parallel. If the teardown step loop fails partway through, finalize MUST NOT be invoked.
- **FR-006**: A routing-backend implementation that has nothing to do at finalize MUST be able to satisfy the new lifecycle requirement with a no-op method that returns nil — no scaffolding, no required configuration, no extra wiring per implementation.
- **FR-007**: The existing Traefik routing backend MUST move its current post-engine publish step (today reached via the plugin's `Deploy()` method) onto the new lifecycle seam. After the change there MUST be no path by which the deploy handler reaches into the Traefik plugin directly for a lifecycle action.
- **FR-008**: The dry-run routing backend MUST implement the new lifecycle method and produce output consistent with the rest of the dry-run engine's style (so dry-run remains a faithful preview of the production deploy lifecycle).
- **FR-009**: The finalize lifecycle seam is added to `RoutingBackend` only. `ContainerBackend` and `DNSBackend` keep their current interfaces unchanged. Rationale: the only concrete usage today is the Traefik plugin's post-engine publish step, which is a routing concern; no container or DNS backend has a deploy-scoped commit phase. Adding a no-op `Finalize()` to the other two interfaces now would violate Constitution Principle IV (YAGNI / ≥3 concrete usages) and force every existing and future backend implementation to carry boilerplate with no caller. When a container or DNS backend that needs batching appears, the seam can be introduced for that interface in its own spec.

### Key Entities *(include if feature involves data)*

- **Routing backend abstraction**: The contract that every routing implementation satisfies. Currently covers per-route writes and per-route removals; after this change it also covers the deploy-scoped finalize phase.
- **Deploy lifecycle**: The ordered phases a single `shrine deploy` invocation goes through — plan, per-step execution (containers, routes, DNS), and (after this change) a finalize phase that closes out any deferred backend work.
- **Routing backend implementation**: A concrete plugin satisfying the routing-backend abstraction (today: the Traefik plugin and the dry-run printer). Each implementation chooses what its finalize phase does — a remote publish, a config swap, or nothing.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A new routing-backend implementation can be added to Shrine with zero edits to the deploy handler, the CLI command code, or any other code outside the new implementation's own package. (Verified by a contributor walkthrough or by adding a stub implementation in a test and confirming no handler-level changes are needed.)
- **SC-002**: Every reference to a plugin-specific lifecycle method (e.g., `TraefikPlugin.Deploy()`) is removed from non-plugin code. (Verified by grep of the post-change tree.)
- **SC-003**: 100% of pre-existing deploy and teardown integration tests pass against the refactored code with no manifest, config, or invocation changes.
- **SC-004**: A finalize-phase failure produces a deploy exit code that is non-zero and an operator-facing log/event entry that names the routing finalize phase as the source. (Verified by an integration test that injects a failing finalize and asserts on the output.)
- **SC-005**: A failure in the per-step loop does not trigger the finalize phase. (Verified by an integration or engine-level test that injects a mid-loop failure and asserts finalize was not invoked.)
- **SC-006**: The open scope question about container and DNS backend finalize phases (FR-009) is resolved before the plan phase, and the resolution is recorded in this spec.

## Assumptions

- The change is an internal refactor — there is no operator-facing configuration, no new manifest field, and no migration. Operators with existing setups continue to use Shrine identically.
- "Finalize" is used in this spec as a placeholder name for the new lifecycle phase. Final naming (Finalize / Commit / Flush / Publish) is an implementation-level decision and is left to the plan phase; the spec only requires that the seam exists.
- The deploy bundle composition root (introduced in PR #26 / spec 017) is the wiring point. The engine receives the routing backend through the same dependency-injection seam that exists today; this work does not introduce a new lifecycle of its own outside that seam.
- Concurrent or partial-failure semantics beyond "step-loop error suppresses finalize" are out of scope for this spec. Specifically, there is no requirement here to make per-step writes transactional or to support rollback of already-written-but-unfinalized routes. A backend implementation that wants those semantics may build them internally.
- The dry-run engine is treated as a first-class routing-backend implementation for the purpose of this work — it must satisfy the new interface, and dry-run output must remain a faithful preview.
