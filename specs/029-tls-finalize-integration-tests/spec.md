# Feature Specification: Integration Coverage for TLS Alias Routers and the Routing Finalize Phase

**Feature Branch**: `029-tls-finalize-integration-tests`
**Created**: 2026-10-04
**Status**: Draft
**Input**: GitHub issue [#38](https://github.com/CarlosHPlata/shrine/issues/38) — "test: integration coverage for TLS alias routers (012) and deferred finalize scenarios (018)"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A maintainer is told when `tls: true` on an alias stops producing an HTTPS-capable route (Priority: P1)

Spec 012 shipped the ability to publish a routing alias over HTTPS by adding `tls: true` to the alias entry. Its end-to-end verification was deferred to the integration pipeline and never written: no fixture declares a TLS alias, and no scenario deploys one. The promise an operator relies on — "add one line, redeploy, and the generated route for that alias listens on both the plain and secure entrypoints and is marked for TLS termination, while the primary-domain route is untouched" — is checked only in isolation at the unit level. A regression anywhere between the manifest, the deploy command, and the file written to disk would ship unnoticed. After this change, a real deploy of a manifest with a TLS alias is exercised on every pipeline run and fails loudly if the generated route loses its secure shape.

**Why this priority**: This is the headline behaviour of spec 012 (its US1 and SC-001/SC-002) and the reason the feature exists. It is also the foundation the other TLS scenarios build on: the mixed and revert stories only make sense once the single-alias case is pinned.

**Independent Test**: Deploy a manifest with one alias that sets `tls: true`, with the secure entrypoint configured, through the real command against a real container runtime; read the generated routing file for that application and confirm the alias route and the primary route have the expected shapes, and that the deploy output marks the alias as TLS-enabled.

**Acceptance Scenarios**:

1. **Given** an application whose single alias sets `tls: true` and a gateway configured with a secure entrypoint, **When** the operator deploys, **Then** the generated routing file's alias route is attached to both the plain and secure entrypoints and carries an empty TLS block.
2. **Given** the same deploy, **When** the generated routing file is read, **Then** the primary-domain route is attached to the plain entrypoint only and carries no TLS block.
3. **Given** the same deploy, **When** the operator reads the deploy output, **Then** it contains the per-alias marker indicating the alias was published with TLS, and no warning about a missing secure entrypoint.

---

### User Story 2 - A maintainer is told when per-alias TLS stops being independent, or stops following the manifest (Priority: P1)

Spec 012 US2 promises that TLS is decided per alias — an application can expose one alias on the LAN over plain HTTP and another externally over HTTPS — and spec 012 SC-005 promises that removing `tls: true` reverts that alias's route within one deploy cycle, without hand-editing generated files. Neither promise is tested at any level today. After this change, the pipeline deploys an application with a mix of TLS-on and TLS-off aliases and asserts each route's shape independently, then removes the opt-in and confirms the route reverts.

**Why this priority**: These are the only two spec 012 outcomes with zero coverage of any kind (the issue calls this out explicitly). The revert path additionally crosses spec 009's preserve-on-redeploy policy, which makes it the scenario most likely to break through an unrelated change.

**Independent Test**: Deploy a manifest with two aliases, only one of which sets `tls: true`; confirm only that alias's route has the secure shape and that all routes target the same backend service. Then remove the opt-in from the manifest, clear the generated per-application routing file as the preserve policy requires, redeploy, and confirm the route has reverted.

**Acceptance Scenarios**:

1. **Given** an application with two aliases where only the second sets `tls: true`, **When** the operator deploys, **Then** the first alias's route is attached to the plain entrypoint only with no TLS block, and the second alias's route is attached to both entrypoints with an empty TLS block.
2. **Given** the same deploy, **When** the generated routing file is read, **Then** both alias routes and the primary-domain route point at the same backend service.
3. **Given** an application previously deployed with `tls: true` on an alias, **When** the operator removes `tls: true` from the manifest, removes the generated per-application routing file, and redeploys, **Then** the regenerated alias route is attached to the plain entrypoint only and carries no TLS block.
4. **Given** the reverted deploy, **When** the operator reads the deploy output, **Then** it no longer contains the TLS marker for that alias.

---

### User Story 3 - A maintainer is told when deploying non-TLS aliases stops being stable and quiet (Priority: P2)

Spec 012 FR-009 and SC-003 promise that applications which never use `tls` are unaffected: their generated routing is the same as before the feature shipped, and their deploy output carries no TLS-related noise. This is the regression guard for every existing deployment. After this change, the pipeline deploys a non-TLS alias manifest twice in a row and asserts that the generated file is byte-for-byte identical across the two deploys, contains nothing TLS-related, and that neither deploy's output mentions TLS.

**Why this priority**: It protects the existing fleet rather than the new capability. It ranks below the first two stories because the non-TLS path is already exercised indirectly by the existing alias scenarios; what is missing is the explicit stability and silence guarantee.

**Independent Test**: Deploy an existing non-TLS alias fixture with no secure entrypoint configured (the configuration most likely to trigger a spurious warning), capture the generated routing file, redeploy unchanged, and compare.

**Acceptance Scenarios**:

1. **Given** an application whose alias does not set `tls` and a gateway with no secure entrypoint configured, **When** the operator deploys twice with no changes in between, **Then** the generated routing file after the second deploy is byte-identical to the file after the first.
2. **Given** the same deploys, **When** the generated routing file is read, **Then** it contains no secure entrypoint reference and no TLS block.
3. **Given** the same deploys, **When** the operator reads the output of each, **Then** neither contains the per-alias TLS marker nor the missing-secure-entrypoint warning.

---

### User Story 4 - A maintainer is told when a routing finalize failure stops failing the deploy, or stops being attributable (Priority: P2)

Spec 018 moved the gateway's post-deploy publish step onto a dedicated finalize phase and promised (FR-003, SC-004) that a failure in that phase makes the deploy exit non-zero and produces an operator-visible entry naming routing finalize as the source — distinguishable from a per-application step failure. The integration scenario that proves this through the real command was deferred and never landed. An operator whose gateway fails to come up at the end of an otherwise successful deploy must not see a zero exit code, and must be able to tell from the output that the applications deployed but the gateway publish did not. After this change, the pipeline provokes a finalize failure through the real command and asserts both the exit code and the attribution.

**Why this priority**: A silently swallowed finalize failure would leave an operator with deployed applications and no working gateway while automation reports success. It ranks below the TLS stories only because the engine-level behaviour is already unit-tested; what is unverified is that the exit code and the attributed message survive all the way to the operator.

**Independent Test**: Arrange a real-world condition under which every per-application step succeeds but the gateway's finalize step cannot, run the real deploy command, and inspect the exit code and output.

**Acceptance Scenarios**:

1. **Given** an environment in which the gateway's finalize step cannot succeed, **When** the operator deploys a manifest with at least one routed application, **Then** the deploy command exits with a non-zero status.
2. **Given** the same deploy, **When** the operator reads the output, **Then** it contains an error entry attributed to the routing finalize phase.
3. **Given** the same deploy, **When** the operator reads the output, **Then** the per-application steps that preceded finalize are reported as having run, so the failure is distinguishable from a per-step failure.

---

### User Story 5 - A maintainer is told when the finalize phase disappears from the dry-run preview or stops producing the gateway (Priority: P3)

Spec 018 FR-008 promises that `--dry-run` remains a faithful preview of the deploy lifecycle, which now includes the routing finalize phase; FR-007 and SC-003 promise that after a real deploy the end state is unchanged by the refactor — the gateway's static configuration is on disk and the gateway container exists, and they got there through the finalize phase. After this change, the pipeline asserts that a dry-run's output includes the routing finalize operation, and that a real deploy of a routed application leaves the gateway's static configuration and container in place.

**Why this priority**: These are confirmations of already-working behaviour with the lowest regression likelihood: existing deploy scenarios would likely fail in other ways if the gateway stopped appearing. They are included to close the deferred tasks and make the guarantees explicit rather than incidental.

**Independent Test**: Run a dry-run deploy of a routed application and inspect the output for the finalize operation; run a real deploy of the same and confirm the static configuration files and the gateway container exist afterwards.

**Acceptance Scenarios**:

1. **Given** a manifest with at least one routed application and the gateway enabled, **When** the operator runs a dry-run deploy, **Then** the output includes the routing finalize operation, after the per-application operations.
2. **Given** the same dry-run, **When** it completes, **Then** no gateway container has been created and no routing files have been written.
3. **Given** the same manifest, **When** the operator runs a real deploy, **Then** the gateway's static configuration exists on disk and the gateway container exists.

---

### Edge Cases

- **Preserve policy on the revert path**: Spec 009 preserves an existing per-application routing file across redeploys. The revert scenario must remove that file between deploys; a scenario that skipped this step would observe the stale TLS route and report a false failure. The scenario must make this step explicit so the test documents the real operator workflow.
- **Secure entrypoint absent while an alias sets `tls: true`**: The route is still written and a warning is emitted (spec 012 FR-007). This is already covered at the unit level and is not added here; the TLS scenarios configure the secure entrypoint so the warning's absence can be asserted.
- **Port conflicts between scenarios**: Scenarios that configure a secure entrypoint bind a host port. Each must use a port that does not collide with other scenarios in the suite or with leftovers from a failed earlier run.
- **Leftover state from a failed run**: A scenario that fails midway must not leave containers, networks, or generated files that cause the next scenario to fail for an unrelated reason. This matters most for the finalize-failure scenario, which deliberately leaves the system in a half-finished state.
- **Finalize failure must be a finalize failure**: The condition used to provoke the failure must let every per-application step succeed first; if it breaks an earlier step, the scenario would pass on a non-zero exit code without ever reaching finalize. The attribution assertion guards against this.
- **Ordering in dry-run output**: The finalize operation must appear after the per-application operations, not merely somewhere in the output.

## Requirements *(mandatory)*

### Functional Requirements

**TLS alias routers (spec 012)**

- **FR-001**: The integration suite MUST include a fixture declaring an application with a single alias that sets `tls: true`, following the layout and naming of the existing alias fixtures.
- **FR-002**: The suite MUST deploy that fixture with a secure entrypoint configured and assert that the alias route is attached to both the plain and secure entrypoints and carries an empty TLS block, while the primary-domain route is attached to the plain entrypoint only and carries no TLS block.
- **FR-003**: The same scenario MUST assert that the deploy output contains the per-alias TLS marker.
- **FR-004**: The suite MUST include a fixture declaring an application with at least two aliases of which exactly one sets `tls: true`.
- **FR-005**: The suite MUST deploy the mixed fixture and assert that only the opted-in alias's route has the secure shape, that the other alias's route has the plain shape, and that all of the application's routes target the same backend service.
- **FR-006**: The suite MUST verify the revert path: after a deploy with `tls: true` on an alias, removing the opt-in from the manifest, removing the generated per-application routing file, and redeploying MUST yield an alias route with the plain shape.
- **FR-007**: The suite MUST verify stability for non-TLS manifests: two consecutive unchanged deploys of a non-TLS alias fixture, with no secure entrypoint configured, MUST produce byte-identical generated routing files that contain no secure entrypoint reference and no TLS block.
- **FR-008**: The stability scenario MUST assert that neither deploy's output contains the per-alias TLS marker or the missing-secure-entrypoint warning.

**Routing finalize phase (spec 018)**

- **FR-009**: The suite MUST include a scenario in which the routing finalize phase fails during a real deploy, and MUST assert that the command exits non-zero.
- **FR-010**: That scenario MUST assert that the operator-visible output contains an error entry attributed to the routing finalize phase.
- **FR-011**: The finalize failure MUST be provoked through conditions an operator could actually encounter, driven from outside the command; the shipped command MUST NOT gain test-only switches, hooks, or code paths to make the failure injectable.
- **FR-012**: The suite MUST assert that a dry-run deploy of a routed application includes the routing finalize operation in its output, positioned after the per-application operations.
- **FR-013**: The suite MUST assert that after a real deploy of a routed application, the gateway's static configuration exists on disk and the gateway container exists.

**Suite hygiene**

- **FR-014**: Every new scenario MUST drive the product only through its public command surface and observe only operator-visible results (exit code, output, files on disk, containers), consistent with the existing integration suite's isolation from internal packages.
- **FR-015**: Every new scenario MUST clean up the containers, networks, and files it creates, whether it passes or fails, so scenarios remain order-independent.
- **FR-016**: No new scenario may change product behaviour. If authoring a scenario reveals that shipped behaviour diverges from spec 012 or spec 018, the divergence MUST be reported and resolved as a separate decision rather than adjusted silently — neither by weakening the assertion nor by changing product behaviour within this feature.
- **FR-017**: The deferred integration tasks in spec 012 (T016, T017, T023, T024, T026) and spec 018 (T012, T014, T018, T019) MUST be traceable to the scenarios that now deliver them, and their status in those specs' task lists updated accordingly.

### Key Entities

- **Scenario**: One end-to-end check run by the integration pipeline — a starting state, one or more real command invocations, and assertions on operator-visible results.
- **Fixture**: A set of manifests (a team and an application) that a scenario deploys. This feature adds a single-TLS-alias fixture and a mixed-TLS fixture, and reuses an existing non-TLS alias fixture.
- **Generated routing file**: The per-application file the gateway reads, containing one route for the primary domain and one per alias. Its per-route entrypoints and TLS block are the main objects under assertion.
- **Deferred task**: An integration task in spec 012 or 018 marked as deferred to the pipeline; each must map to a scenario delivered here.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All nine deferred integration tasks named in issue #38 (five from spec 012, four from spec 018) are delivered by at least one scenario each, with the mapping recorded.
- **SC-002**: Spec 012's US2 (mixed per-alias TLS) and SC-005 (removing `tls: true` reverts the route) go from zero coverage at any level to one end-to-end scenario each.
- **SC-003**: Deliberately breaking any one of the guarded behaviours — dropping the TLS block from an opted-in alias, applying it to a non-opted-in alias, emitting TLS output for a non-TLS manifest, returning success on a finalize failure, losing the finalize attribution, or omitting finalize from the dry-run — causes at least one new scenario to fail.
- **SC-004**: The full integration pipeline passes with the new scenarios included, with no existing scenario modified in behaviour or removed.
- **SC-005**: The new scenarios pass on three consecutive pipeline runs with no intermittent failures.
- **SC-006**: The change contains no modification to product behaviour: an operator running any command before and after sees identical results.

## Assumptions

- **Source of scope**: With no description typed on the command, issue #38's six proposed items are taken as the feature scope in full.
- **Delivery shape**: One change set, as the issue proposes, extending the existing gateway integration scenarios and adding fixtures alongside the existing alias fixtures (which live in the suite's shared test-data area, not the location the spec 012 task list originally named).
- **Shipped behaviour is correct**: The scenarios are expected to pass against the current product. The issue notes the existing unit tests "missed nothing"; this feature closes a verification gap rather than fixing a known defect.
- **Finalize failure is provoked externally**: Spec 018 T012 described injecting a routing backend whose finalize returns an error. Because the integration suite runs the real command as a separate process and is isolated from internal packages, the failure is instead provoked through a real environmental condition (FR-011). Which condition is the planning phase's decision; candidates must satisfy the "finalize, not an earlier step" edge case.
- **"Finalize ran through the lifecycle seam" is asserted by its effects**: Spec 018 T019 asked for proof that the engine's finalize, not a plugin-specific method, performed the publish. From outside the process this is observable only as its effects — the finalize entry in the output and the gateway existing afterwards — which is how issue #38 frames it. The structural guarantee (no direct plugin call from the handler) remains covered by spec 018's unit tests and SC-002 grep.
- **Verification runs in the pipeline**: Per project practice, the integration suite is slow and requires a container runtime; scenarios are authored and compile-checked locally and the CI pipeline is the pass/fail gate.
- **Out of scope**: The missing-secure-entrypoint warning path (spec 012 FR-007), manifest validation of invalid `tls` values (spec 012 SC-004), and the "mid-loop failure does not trigger finalize" guarantee (spec 018 SC-005) are already covered at the unit level and are not among the deferred tasks.
