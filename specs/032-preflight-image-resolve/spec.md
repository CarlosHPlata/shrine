# Feature Specification: Resolve Every Image Before Touching Any Container

**Feature Branch**: `032-preflight-image-resolve`
**Created**: 2026-10-06
**Status**: Draft
**Input**: User description: "Resolve the image of every artifact in the deploy set as a backend method and engine pre-pass before any container or network operation, so an unresolvable reference fails with zero changes, deploy output states the exact version per artifact, and dry run prints the step"
**Epic**: Pinned Image Versions, ticket T2 (issue #53). Source documents: [prd.md](../epics/pinned-image-versions/prd.md) (R-14, R-15, R-16, G5, D4), [design.md](../epics/pinned-image-versions/design.md) (TD-2, TD-5, sections 4.1 to 4.4, 4.9, 4.11, requirement list T2-01 to T2-07), [tickets.md](../epics/pinned-image-versions/tickets.md#t2-resolve-every-image-before-touching-any-container).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A bad image reference changes nothing (Priority: P1)

An operator deploys a set of several applications and resources. One of them names an image the registry cannot serve: a typo in the tag, a repository that does not exist, a registry that is down. Today the deploy creates or recreates every container that comes before the bad one in deploy order and then fails, leaving the host half-updated. After this feature, the deploy checks every image first and stops before it has created, changed, or removed a single container or network. The failure message names the artifact and the image reference so the operator can fix the manifest without reading logs.

**Why this priority**: This is the whole point of the ticket and the seam the rest of the epic plugs into. Goal G5 of the PRD ("fail before touching anything") and decision D4 (fail-fast, not transactional) are delivered by this story alone.

**Independent Test**: Deploy a set holding two healthy artifacts and one artifact whose image reference points at a registry that does not answer, ordered so the broken artifact comes last. Verify the command fails, that no container of the set exists afterwards, that the team's private network was not created, and that the error text names the broken artifact and its reference.

**Acceptance Scenarios**:

1. **Given** a deploy set of several artifacts where the last one in deploy order has an unresolvable image reference, **When** the operator deploys, **Then** the deploy fails, no container of the set is created, changed, or removed, and the team's private network is not created.
2. **Given** the same deploy set, **When** the deploy fails, **Then** the error names the artifact (kind and name) and the image reference that could not be resolved.
3. **Given** a deploy set whose artifacts were all deployed before and one manifest now names an unresolvable reference, **When** the operator redeploys, **Then** every running container keeps running unchanged and none is recreated or removed.
4. **Given** a deploy scoped to one team, **When** an artifact of another team has an unresolvable reference, **Then** the scoped deploy is unaffected, because only the artifacts in the deploy set are resolved.

---

### User Story 2 - Deploy output states the exact version per artifact (Priority: P2)

While deploying, the operator sees, for each artifact, which image reference is being resolved and, once resolved, the exact version it will run: the reference plus a short form of the registry's content digest. For artifacts whose version is owned by the manifest (today's two pull policies), the line shows that the version came from the manifest.

**Why this priority**: Knowing the exact version at deploy time is PRD requirement R-15 and is what makes the later pin and bump tickets legible. It is cheap once resolution is a step of its own.

**Independent Test**: Deploy a healthy set and verify the output contains, for every artifact, one "resolving" line naming the artifact and its reference, followed by one "resolved" line naming the artifact and the reference with a short digest.

**Acceptance Scenarios**:

1. **Given** a healthy deploy set, **When** the operator deploys, **Then** the output holds, per artifact, a line stating that its image is being resolved, with the artifact name and the reference.
2. **Given** a healthy deploy set whose images come from a registry, **When** resolution finishes, **Then** the output holds, per artifact, a line with the artifact name, the reference, and a short form of the registry digest.
3. **Given** an artifact whose image is present locally but has no registry digest, **When** resolution finishes, **Then** the output states the reference alone, without a digest, and the deploy continues.
4. **Given** an artifact whose manifest names the image through a registry alias, **When** the output names the reference, **Then** it shows the expanded reference, never the alias form.

---

### User Story 3 - Dry run shows the step and touches nothing (Priority: P3)

An operator previews a deploy with dry run. The preview lists the image resolution step, one line per artifact with the reference and the effective pull policy, before any container or network operation, so the operator sees the new step in the order it will run. The dry run contacts no registry, pulls nothing, and writes nothing.

**Why this priority**: Every write operation must have a faithful dry run (constitution principle II). The step must be visible in the preview from the day it exists; the later pinned-policy lines are added by another ticket in the same place.

**Independent Test**: Run a dry-run deploy of a healthy set and verify the output holds one resolution line per artifact, that every such line appears before the first container operation line, and that no container exists afterwards.

**Acceptance Scenarios**:

1. **Given** a deploy set, **When** the operator runs a dry run, **Then** the output lists one image-resolution line per artifact, naming the artifact, the reference as the manifest wrote it, the effective pull policy, and that the version is manifest-owned.
2. **Given** the same dry run, **When** the output is read top to bottom, **Then** every resolution line appears before the first network or container operation.
3. **Given** the same dry run, **When** it completes, **Then** no container exists for the set and no image was pulled.
4. **Given** the same dry run repeated, **When** recorded state is compared before and after, **Then** it is byte for byte unchanged.

---

### User Story 4 - Healthy deploys behave exactly as before (Priority: P4)

An operator who never touches the new behaviour sees no change beyond the added output lines. Images under the `latest` tag or with no tag are still pulled on every deploy; images with a fixed tag still reuse the local copy and are pulled only when absent. A redeploy without manifest changes still leaves every container in place; upgrading Shrine to this version recreates nothing.

**Why this priority**: Metric M2 of the PRD and goal G6: nothing changes for anyone who does not opt in. Moving resolution out of container creation must not alter what gets pulled or when a container is recreated.

**Independent Test**: Deploy a set with a `latest` image twice and verify both runs pull; deploy a set with a fixed-tag image twice and verify the second run does not pull and the container id is unchanged.

**Acceptance Scenarios**:

1. **Given** an artifact whose image has no tag or the tag `latest`, **When** it is deployed twice, **Then** both deploys pull the image.
2. **Given** an artifact whose image has a fixed tag and is already present locally, **When** it is deployed, **Then** no pull happens and the local image is used.
3. **Given** an artifact deployed before this feature, **When** it is redeployed with an unchanged manifest after upgrading, **Then** its container is not recreated.
4. **Given** the existing integration scenarios for deploy, dry run, registry aliases, publishing, and the gateway plugin, **When** they run against this feature, **Then** they pass without any change to their assertions.

---

### User Story 5 - Contributors see the step in the pipeline reference (Priority: P5)

A contributor reading the deploy pipeline diagram in the contributor reference sees image resolution as a step of its own, placed before the platform network is created, with one sentence stating that a failure there changes nothing.

**Why this priority**: The repository's doc/code drift policy requires the diagram to change in the same pull request as the seam it describes. It is last only because it documents the other stories.

**Independent Test**: Open the contributor reference and verify the deploy pipeline diagram shows the resolution step before the platform network step and states the zero-change guarantee.

**Acceptance Scenarios**:

1. **Given** the contributor reference, **When** a contributor reads the deploy pipeline diagram, **Then** image resolution appears as a pre-pass before the platform network step.
2. **Given** the same diagram, **When** the contributor reads the text around it, **Then** one sentence states that a failure during resolution leaves networks and containers untouched.

---

### Edge Cases

- An image present locally without a registry digest (for example loaded from a file) under a fixed tag resolves successfully with an empty digest; the output shows the reference alone. Only the later pinned policy treats a missing digest as an error.
- A registry alias that is not declared is rejected at planning time today; if an alias ever reaches resolution undeclared, resolution fails in the pre-pass with zero changes, like any other failure.
- The same image referenced by several artifacts is resolved once per artifact, in deploy order, following each artifact's own pull policy; a `latest` image shared by two artifacts is pulled twice, as it is today.
- A deploy set with no steps prints "No steps generated." and resolves nothing.
- The gateway container run by the routing plugin is not part of the deploy set and is not resolved by the pre-pass; it keeps resolving its own image when it is created, with no change in its output.
- Resolution succeeds for every artifact but a later container operation fails: the deploy stops there, as today. The deploy is fail-fast, not transactional (PRD decision D4); this feature only moves image failures in front of the first change.
- A pull that starts but is interrupted (registry drops the connection) fails the pre-pass with zero changes, exactly like a reference that does not exist.
- A deploy scoped to one team or to one manifest file resolves only the artifacts in that scope.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001** [T2-02, R-14]: Before any container or network is created, changed, or removed, deploy MUST resolve the image of every artifact in the deploy set, in deploy order.
- **FR-002** [T2-02, R-14]: When resolving any artifact's image fails, deploy MUST stop before the first container or network operation, leaving zero changes, and the error MUST name the artifact by kind and name and the image reference that failed.
- **FR-003** [T2-01, R-14]: Resolving an image MUST yield the reference the container will be created from (with any registry alias expanded), the registry's exact version of that image when the local image carries one, and the local image identity; the exact version MAY be empty for manifest-owned artifacts without failing.
- **FR-004** [T2-07, M2]: Pull semantics MUST be unchanged: an image under the `Always` policy (no tag or `latest`) is pulled on every deploy; an image under `IfNotPresent` (any other tag) reuses the local image when present and is pulled only when absent.
- **FR-005** [T2-03, TD-2]: Container creation MUST use the image resolved by the pre-pass instead of resolving again, and the decision to recreate an existing container MUST rest on the same inputs as before this feature, so upgrading Shrine recreates no container.
- **FR-006** [T2-03]: Containers created outside the deploy set, such as the routing plugin's gateway container, MUST keep today's behaviour of resolving their own image at creation time.
- **FR-007** [T2-04, R-15]: Deploy output MUST state, per artifact, that its image is being resolved (artifact and reference) and, when resolution finishes, the exact version it will run: the expanded reference followed by a short form of the registry digest when one exists, and the reference alone otherwise.
- **FR-008** [T2-05, R-16]: Dry run MUST print the resolution step, one line per artifact naming the artifact, the reference as the manifest wrote it, the effective pull policy, and that the version is manifest-owned, before any network or container operation, and MUST contact no registry, pull nothing, and write nothing.
- **FR-009** [T2-02]: Only the artifacts in the deploy set are resolved: a deploy scoped to one team resolves that team's artifacts only, and a deploy of one manifest file resolves that artifact only.
- **FR-010** [T2-06, R-32]: The contributor reference's deploy pipeline diagram MUST show image resolution as a pre-pass before the platform network step, with one sentence stating that a failure there leaves networks and containers untouched.
- **FR-011** [M2]: The existing integration scenarios MUST pass without edits to their assertions; the only observable change for existing manifests is the added output lines.
- **FR-012**: Registry alias expansion MUST happen exactly once per artifact, during resolution; the expanded reference is what the output shows and what the container is created from, as today.

### Key Entities

- **Deploy set**: the planned steps of one deploy command, each naming one artifact (application or resource) by kind, name, and team, in deploy order.
- **Image reference**: the image an artifact's manifest names, either as written (possibly a registry alias form) or expanded to the registry host.
- **Exact version**: the registry's content digest for the pulled image's repository; absent when the local image carries none.
- **Resolution result**: for one artifact, the expanded reference, the exact version, the local image identity, and the source of the version. In this ticket the source is always "manifest"; later tickets add pinned sources.
- **Effective pull policy**: the policy an artifact deploys under: the manifest's value, or the derived rule (`latest` or no tag pulls always; any other tag pulls only when absent).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With one unresolvable reference among any number of artifacts, a deploy creates, changes, and removes zero containers and zero team networks (PRD metric M5).
- **SC-002**: The failure text of SC-001 names the artifact and the reference; an operator can locate and fix the manifest from the message alone.
- **SC-003**: A healthy deploy set produces the same containers as before this feature, and a redeploy with unchanged manifests keeps every container id; the only difference in output is the added resolution lines (PRD metric M2).
- **SC-004**: Every artifact in a deploy's output has exactly one "resolving" line and exactly one "resolved" line stating its version.
- **SC-005**: A dry run prints exactly one resolution line per artifact, all of them before the first network or container operation, and repeated dry runs leave recorded state byte for byte unchanged (PRD metric M6).
- **SC-006**: An image under `latest` is pulled on every deploy; an image under a fixed tag that is present locally is not pulled.
- **SC-007**: The deploy pipeline diagram in the contributor reference shows the resolution step before the platform network step.

## Assumptions

- **Scope**: only the two existing pull policies, `Always` and `IfNotPresent`, are resolved by this ticket. The `Pinned` policy, pin records, bump, and any new manifest or configuration field are out of scope (tickets T3 to T7). No manifest reference, configuration page, or CLI page changes; the contributor reference is the only documentation this ticket owns.
- **Exact version**: the registry digest is taken from the local image's repository digests, choosing the entry whose repository matches the pulled reference's repository. Docker Hub repositories are matched regardless of the `docker.io/` and `library/` prefixes the daemon may or may not record. An image without a matching entry yields an empty exact version, which is allowed for manifest-owned artifacts (design section 4.2).
- **Short digest form**: twelve hexadecimal characters of the digest, matching how container ids are shortened elsewhere (design TD-11).
- **Output shape**: the deploy output lines follow design section 4.9: a started line `🔎 Resolving image for <team>.<name> (<ref>)` and a finished line `🔎 Resolved <team>.<name> <ref>@<short digest>` for manifest-owned artifacts; the exact strings, indentation, and the no-digest form are fixed in this feature's output contract. A resolution failure renders through the existing generic error line.
- **Dry-run line**: `[DOCKER] ImageResolve: name=<team>.<name> image=<ref> policy=<policy> -> manifest-owned`, with the reference as the manifest wrote it, so a registry alias stays visible in the preview as it does on the container line today (design section 4.4, manifest-owned line only).
- **Failure text**: the error is the existing per-operation message (pull, inspect, alias expansion) prefixed by the artifact's kind and name, in the form today's container errors use: `application "<name>": pulling image "<ref>": <cause>`.
- **Recreation**: the container reconcile keeps using the local image identity as its version input (design TD-2), so no container is recreated merely because Shrine was upgraded.
- **Per-artifact resolution**: an image shared by several artifacts is resolved once per artifact; deduplicating pulls is not a goal of this ticket.
- **Gateway container**: the routing plugin creates its container outside the deploy set and keeps today's resolution path; its output is unchanged.
