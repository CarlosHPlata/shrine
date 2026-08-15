# Feature Specification: Expand `reg:` Registry Aliases Before the Container Is Created

**Feature Branch**: `022-fix-registry-alias-expansion`
**Created**: 2026-08-14
**Status**: Draft
**Input**: User description: "Fix registry alias expansion in the container spec (GitHub issue #33)"
**Source Issue**: [#33 — Registry alias not expanded in container spec — "invalid reference format" on fresh deploy](https://github.com/CarlosHPlata/shrine/issues/33)

## User Scenarios & Testing *(mandatory)*

### User Story 1 — A new workload deploys using the `reg:` alias form (Priority: P1)

A platform engineer declares a private registry once in their Shrine config, giving it a short alias, then writes an `Application` manifest whose image is `reg:<alias>/<path>:<tag>` instead of repeating the registry host in every manifest. They deploy the application for the first time. Today the deploy fails outright — the alias is handed to the container runtime verbatim and rejected as a malformed image reference — so the alias feature is unusable for any workload that does not already exist. After this change the deploy succeeds, and the running container is indistinguishable from one deployed with the fully-qualified registry reference.

**Why this priority**: This is the bug. The registry alias feature has never worked for creating a container, so every engineer who adopts the documented alias syntax hits a hard failure on their first real deploy. Nothing else in this feature matters if this is not fixed.

**Independent Test**: Declare a registry alias in config, write an `Application` manifest using `image: reg:<alias>/<path>:<tag>` for a workload that has no existing container, and run `shrine deploy`. Verify the deploy succeeds and the created container's image reference is the fully-qualified one. Compare against the same manifest with the host written out in full — both must produce the same result.

**Acceptance Scenarios**:

1. **Given** a config declaring an alias for a registry host, and an `Application` manifest using `image: reg:<alias>/<path>:<tag>` for a workload with **no existing container**, **When** the user runs `shrine deploy`, **Then** the container is created successfully and its image reference is the alias expanded to the configured registry host.
2. **Given** the same alias config, **When** the user deploys one manifest using the alias form and an otherwise identical manifest using the fully-qualified reference, **Then** both deploys produce containers with the same image reference and the same observable outcome.
3. **Given** a manifest whose image is a plain reference with no `reg:` prefix, **When** the user deploys it, **Then** the image reference reaches the container runtime unchanged, exactly as it does today.
4. **Given** an alias-form manifest, **When** the deploy runs, **Then** the image is pulled from, authenticated against, and recorded for the expanded registry host — the pull, the credential lookup, and the container's image reference all agree on a single reference.

---

### User Story 2 — Resources using the alias form deploy too (Priority: P1)

A platform engineer defines a `Resource` (a database, a cache, a queue) whose image comes from the same private registry and uses the alias form. Resources reach the container runtime through the same creation path as applications, so they fail identically today. After this change, alias-form resources deploy just like alias-form applications.

**Why this priority**: Same defect, same severity, same fix surface — a fix that covered only applications would leave the feature half-broken and would look, from the operator's seat, like a partial and arbitrary repair. It is P1 alongside User Story 1 rather than after it.

**Independent Test**: Write a `Resource` manifest with `image: reg:<alias>/<path>:<tag>` for a resource with no existing container, deploy it, and verify the container is created with the expanded reference.

**Acceptance Scenarios**:

1. **Given** a config declaring an alias, and a `Resource` manifest using the alias image form for a resource with **no existing container**, **When** the user runs `shrine deploy`, **Then** the container is created successfully with the expanded image reference.
2. **Given** a manifest set containing both an alias-form `Application` and an alias-form `Resource`, **When** the user deploys, **Then** both are created and neither carries an unexpanded `reg:` reference into the container runtime.

---

### User Story 3 — A rejected image reference is named in the error (Priority: P2)

A platform engineer's deploy fails at container creation. The message says the reference was rejected but never says *which* reference, so the engineer cannot tell whether the problem is a typo in the tag, an undeclared alias, or a malformed host — they have only the container name to go on. After this change, any container-creation failure names the image reference that was rejected, so the engineer can act on the message without reproducing the failure under a debugger.

**Why this priority**: This is what turned a one-line fix into a bug-hunt: the original report had to be reconstructed by reading source, because the error text carried nothing actionable. It is a diagnosability improvement, valuable independently of the alias bug and applicable to every container-creation failure — but the deploy already works once P1 lands, so it ranks below it.

**Independent Test**: Trigger a container-creation failure with a deliberately malformed image reference and verify the error output contains that reference.

**Acceptance Scenarios**:

1. **Given** a manifest whose image reference the container runtime rejects for any reason, **When** the deploy fails at container creation, **Then** the error output names the rejected image reference alongside the container name.
2. **Given** a container-creation failure, **When** the user reads the error, **Then** they can identify the offending manifest and the offending field without re-running the deploy or consulting source code.

---

### User Story 4 — The deploy log does not invent container names (Priority: P3)

A platform engineer reads the output of a failed deploy. Today a single failed container creation prints the "creating container" line three times, one of them with a malformed name carrying a leading dot and the team segment missing — which reads as though Shrine retried the operation against a differently-named container. It did not; one creation was attempted. After this change, the log reflects what actually happened.

**Why this priority**: No deploy outcome changes, but the phantom lines actively mislead during incident triage — the original bug report drew an incorrect conclusion about a "retry path" directly from this output. Cosmetic relative to the failure itself, so it ranks last.

**Independent Test**: Trigger a container-creation failure and count the "creating container" lines in the output: exactly one per creation attempt, none with a malformed name.

**Acceptance Scenarios**:

1. **Given** a deploy in which container creation fails, **When** the user reads the output, **Then** the "creating container" line appears exactly once for that container and no line displays a name with a missing team segment or a leading separator.
2. **Given** a deploy in which container creation succeeds, **When** the user reads the output, **Then** the progress lines are unchanged from today's successful-deploy output.

---

### Edge Cases

- **Alias not declared in config**: an image referencing an alias with no matching entry must still be rejected with the existing plan-time error naming the unknown alias. This feature does not weaken that check, and the failure must continue to occur before anything is created.
- **Empty alias name** (`reg:/path:tag`): must continue to be rejected as malformed, with the existing diagnostic.
- **Alias with no path segment** (`reg:<alias>`): behaviour must be defined and consistent between the validation performed at plan time and the expansion performed before creation — the two must not disagree about whether a given reference is acceptable.
- **Manifest edited from a fully-qualified reference to the equivalent alias form (or back)**: the two references denote the same image, so a workload that is already running the correct image must not be needlessly recreated by the edit alone. Conversely, an edit that genuinely changes which image is wanted must still trigger a recreate.
- **Workload with an existing, up-to-date container**: continues to take the existing early-return path and is unaffected by this change. This is precisely the case that masked the bug, so it must be covered explicitly rather than assumed.
- **Dry-run output**: `--dry-run` must continue to display the `reg:` alias form as the operator wrote it. The expansion is an execution concern; showing the operator a host they did not type would be a regression in the preview's fidelity to their manifest.
- **Plain and fully-qualified references**: unchanged in every respect. The alias path must be the only behaviour this feature alters.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When an image reference uses the `reg:<alias>` form, Shrine MUST expand it to the configured registry host before the container specification reaches the container runtime, so that no container is ever created from an unexpanded reference.
- **FR-002**: Expansion MUST apply to both `Application` and `Resource` containers.
- **FR-003**: For a single container creation, the image pull, the registry credential lookup, and the created container's image reference MUST all use the same expanded reference. No stage of a single creation may operate on a different reference than another.
- **FR-004**: A deploy of an alias-form manifest MUST produce the same observable result as a deploy of the equivalent fully-qualified manifest.
- **FR-005**: Image references without the `reg:` prefix MUST reach the container runtime unchanged.
- **FR-006**: An alias that is not declared in config, or a malformed alias reference, MUST continue to be reported as an error, and MUST fail before any container is created.
- **FR-007**: `shrine deploy --dry-run` MUST continue to display image references in the `reg:` alias form as authored in the manifest.
- **FR-008**: When container creation fails, the error reported to the operator MUST name the image reference that was used, in addition to the container name.
- **FR-009**: The deploy output MUST print at most one "creating container" progress line per container-creation attempt, and every displayed container name MUST include both the team and resource segments.
- **FR-010**: Changing a manifest's image reference between the alias form and the equivalent fully-qualified form MUST NOT, by itself, cause a running workload to be recreated; a change that resolves to a different image MUST still cause a recreate.
- **FR-011**: Deploys of workloads that already have an up-to-date container MUST behave exactly as they do today.

### Verification Requirements

*This section exists because of how feature 014 failed: its spec correctly demanded that "the container engine receives the expanded reference," but the task breakdown translated that into dry-run-only tests and mechanism-level tasks ("call function X"), all of which were completed honestly while the requirement itself went unimplemented for over a year. These rules bind how acceptance of THIS spec may be evidenced, and any task list derived from this spec inherits them.*

- **VR-001 (Execution-path evidence)**: Acceptance of FR-001 through FR-006 MUST be demonstrated by a real deploy against the real container runtime, asserting on the container that was actually created. A dry-run invocation MUST NOT be accepted as evidence for any requirement that describes what the container runtime receives — dry-run exercises a different code path by design.
- **VR-002 (Outcome assertions only)**: Every test covering a requirement in this spec MUST assert an operator-observable outcome — the created container's image reference, the deploy's exit status, or the deploy's output text. A test that verifies an internal step occurred (a function was called, a value was computed) MUST NOT be counted as covering any requirement. Feature 014's expansion function had thorough passing tests while the expansion never reached the container.
- **VR-003 (Fail-first proof)**: Each test introduced to cover FR-001, FR-002, FR-008, or FR-009 MUST be demonstrated to fail against the unfixed code before the fix is applied. A regression test that has never been seen red proves only that it passes, not that it detects the defect it guards.
- **VR-004 (Requirement-to-test traceability)**: The task breakdown derived from this spec MUST contain an explicit mapping from every functional requirement and every acceptance scenario to the specific test(s) that evidence it. A requirement with no covering test is unimplemented, regardless of how many tasks referencing it are checked off. Any requirement found unmapped at task-generation time MUST block progression to implementation, not be deferred.
- **VR-005 (Checkpoint fidelity)**: Any phase checkpoint, "definition of done," or validation step derived from this spec MUST restate the acceptance scenario it claims to satisfy without narrowing it. A checkpoint that weakens "run `shrine deploy`" to "run `shrine deploy --dry-run`" — the exact substitution through which feature 014's live-deployment requirement disappeared — is a traceability defect, not a paraphrase.

### Key Entities

- **Registry alias**: an operator-declared short name bound to a registry host in Shrine's configuration. Written in a manifest as the `reg:<alias>/` prefix on an image reference.
- **Image reference**: the identifier of a container image as authored in a manifest. Exists in two equivalent forms — the alias form, which is what the operator writes and what previews display, and the expanded form, which is what the container runtime is given.
- **Container specification**: the complete description of a container handed to the runtime at creation time. Its image field is the field this feature corrects.
- **Deploy diagnostic**: the operator-facing output of a deploy — progress lines and error messages. Must faithfully describe both what was attempted and what was rejected.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A first-time deploy of an application whose image uses the `reg:` alias form completes successfully, where today it fails 100% of the time.
- **SC-002**: A first-time deploy of a resource whose image uses the `reg:` alias form completes successfully.
- **SC-003**: For every fixture in the alias regression set, the container specification handed to the runtime contains no reference beginning with `reg:` — verified by inspecting the specification at the point of creation, not by inferring success from a passing deploy.
- **SC-004**: Deploying an alias-form manifest and its fully-qualified equivalent yields containers with identical image references.
- **SC-005**: `shrine deploy --dry-run` output for alias-form manifests still shows the `reg:` form — confirmed by the existing dry-run assertions continuing to pass unmodified.
- **SC-006**: Every container-creation failure message names the rejected image reference; an operator handed only the deploy output can identify the offending manifest field without access to the source.
- **SC-007**: A failed container creation produces exactly one "creating container" line, down from three today, with zero malformed names.
- **SC-008**: Editing a manifest between the alias form and the equivalent fully-qualified form causes zero container recreations.
- **SC-009**: No existing deploy behaviour regresses — plain references, fully-qualified references, undeclared-alias rejection, and up-to-date-container early return all behave as they do today.
- **SC-010**: 100% of functional requirements and acceptance scenarios in this spec are mapped to at least one automated test in the traceability mapping required by VR-004, with zero requirements evidenced solely by dry-run when they describe live-execution behaviour.
- **SC-011**: Every regression test guarding the defects in this spec has a recorded red run against the unfixed code (VR-003) — the count of never-seen-red regression tests introduced by this feature is zero.

## Assumptions

- The alias-to-host binding, its config schema, and its plan-time validation are already correct and are not changed by this feature. Only the propagation of the expanded reference into the container specification is at fault.
- Moving alias resolution to plan time — so that the execution layer only ever receives fully-qualified references and this class of defect becomes structurally impossible — is deliberately **out of scope** and tracked separately. It would require the plan to carry both forms (the alias for display, the expanded reference for execution) to satisfy FR-007. This feature fixes the defect where it occurs.
- The `reg:` alias syntax, the config format, and the manifest schema are unchanged. No manifest an operator has already written needs editing, and no migration is required.
- Verifying FR-003 and SC-003 requires observing the container specification as it is handed to the runtime. The current design offers no seam for that observation, which is why the defect shipped undetected; introducing one is understood to be part of the work.
- "No existing container" in the P1 and P2 scenarios means no container exists under the expected name for that workload. This is the condition under which the defect manifests; workloads with an existing up-to-date container take a different path and never reach the faulty code.
- Registry credentials are resolved from the expanded host, as they already are today. This feature does not change how credentials are matched or how anonymous pulls are handled.
