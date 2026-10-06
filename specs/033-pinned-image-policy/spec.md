# Feature Specification: The Pinned Policy: Resolve Once, Keep the Exact Version

**Feature Branch**: `033-pinned-image-policy`
**Created**: 2026-10-06
**Status**: Draft
**Input**: User description: "https://github.com/CarlosHPlata/shrine/issues/54 ticket will point to docs in the epic specs/epics/pinned-image-versions/prd.md and specs/epics/pinned-image-versions/design.md"
**Epic**: Pinned Image Versions, ticket T3 (issue #54). Source documents: [prd.md](../epics/pinned-image-versions/prd.md) (goals G1, G5, G6; journeys J1, J2, J8; requirements R-01 to R-04, R-09 to R-13, R-15, R-16, R-28, R-32; decisions D1, D2, D4, D10; metrics M1, M2, M5, M6), [design.md](../epics/pinned-image-versions/design.md) (decisions TD-1, TD-3, TD-6, TD-7, TD-8, TD-9, TD-11, TD-13; sections 3.1, 3.4, 3.5, 4.2, 4.4, 4.5, 4.8, 4.9, 4.11, 5; requirement list T3-01 to T3-10), [tickets.md](../epics/pinned-image-versions/tickets.md#t3-the-pinned-policy-resolve-once-keep-the-exact-version) section T3.

> **Amendment in force.** PRD R-11 and R-12 still say a pin is "kept, inert" when the artifact deploys under a manifest-owned policy. Design decision TD-6 and the ticket prevail: that deploy releases the pin, and returning to the pinned policy is a first deploy. This spec follows TD-6.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Freeze what you got (Priority: P1)

An operator writes a manifest that names only an image repository, sets the image pull policy to the new value, `Pinned`, and deploys. Shrine takes the newest version of that image, deploys it, and tells the operator the exact version it chose. From then on every deploy of that artifact runs that exact version: a plain redeploy, a redeploy that recreates the container, a teardown followed by a deploy, a deploy after the local image cache was wiped, and any mix of these. The upstream `latest` can move as often as it likes; the artifact does not follow it until the operator decides to move.

**Why this priority**: This is goal G1 of the PRD and the reason the epic exists. Without it, the new policy value is accepted but means nothing.

**Independent Test**: Against a registry the test controls, push an image as `latest`, deploy a manifest under `Pinned` that names that repository, record the exact version from the output, push a different image as `latest`, then run ten deploy cycles that include a plain redeploy, a forced container recreation, a teardown and deploy, and a deploy after removing the pinned image from the host. Every cycle must run the exact version recorded on the first deploy.

**Acceptance Scenarios**:

1. **Given** a manifest under `Pinned` whose artifact has never been deployed, **When** the operator deploys, **Then** Shrine resolves the newest version of the repository from the registry, deploys it, records a pin holding the exact version, the readable version it was resolved from, and the date, and the output states the exact version and that it was pinned on this deploy.
2. **Given** a pinned artifact whose registry `latest` now points at a different image, **When** the operator redeploys, **Then** the artifact runs the pinned exact version, the output states that the pin was reused, and the running container is not recreated.
3. **Given** a pinned artifact, **When** the operator changes an environment value and redeploys so that the container is recreated, **Then** the new container runs the pinned exact version.
4. **Given** a pinned artifact, **When** the operator tears the team down and deploys it again, **Then** the artifact comes back on the pinned exact version.
5. **Given** a pinned artifact whose image has been removed from the host, **When** the operator deploys, **Then** Shrine fetches the pinned exact version from the registry, never the tag, and the artifact runs the pinned exact version.
6. **Given** a pinned artifact whose image is present on the host, **When** the operator deploys, **Then** Shrine makes no registry call for that artifact.
7. **Given** ten deploy cycles mixing the cases above while the registry's `latest` has moved, **When** they complete, **Then** the exact version is identical in all ten (PRD metric M1).

---

### User Story 2 - A manifest can ask for the pinned policy, and cannot contradict it (Priority: P2)

An operator sets `imagePullPolicy: Pinned` on an Application or a Resource manifest. Shrine accepts the value. Because the version is now Shrine-owned, the manifest must not name a fixed version: an Application image carries no tag or the tag `latest`; a Resource omits `version` or sets it to `latest`, and a Resource image override follows the Application rule. A manifest that breaks this rule is rejected with the manifest's other validation errors, naming the artifact and the offending field. Manifests under the two existing values behave exactly as before.

**Why this priority**: The value and its validation are the entry point to everything else; the rejection is what makes a pin trustworthy, because a pinned manifest can never also name a version. It is second only because, on its own, it delivers no pinning.

**Independent Test**: Load a directory holding manifests under each of the three values, with and without a fixed version, and verify which are accepted and which are rejected, the text of each rejection, and that the existing fixtures load with no change.

**Acceptance Scenarios**:

1. **Given** an Application under `Pinned` whose image is `ghcr.io/me/hello-api` or `ghcr.io/me/hello-api:latest`, **When** the operator deploys, **Then** the manifest is accepted.
2. **Given** an Application under `Pinned` whose image is `ghcr.io/me/hello-api:1.2.0` or a digest reference, **When** the operator deploys, **Then** validation fails, the error names the application and the field `spec.image`, says the value names a fixed version while the policy is `Pinned`, and the error is reported together with the manifest's other validation errors, before any change is made.
3. **Given** a Resource under `Pinned` with no `version`, or with `version: latest`, **When** the operator deploys, **Then** the manifest is accepted and the resource's repository is its type (for example `postgres`), or its image override when one is set.
4. **Given** a Resource under `Pinned` with `version: "16"`, **When** the operator deploys, **Then** validation fails naming the resource and `spec.version`.
5. **Given** a Resource under `Pinned` whose image override names a fixed tag, **When** the operator deploys, **Then** validation fails naming the resource and `spec.image`.
6. **Given** a Resource under `Always` or `IfNotPresent`, or with no policy named, and no `version`, **When** the operator deploys, **Then** validation fails with the same "version is required" message as today.
7. **Given** a manifest whose policy is any value other than the three, **When** the operator deploys, **Then** validation rejects it, naming the artifact, the field, and the three accepted values. (Today an unknown value is not rejected and silently behaves as `IfNotPresent`; the check is new, because the third value makes the field's meaning depend on an exact match.)
8. **Given** the existing fixtures, none of which name `Pinned`, **When** they are deployed, **Then** every manifest is accepted and each artifact's effective policy is what it was before this feature: the manifest's own field, else `Always` for `latest` or no tag and `IfNotPresent` for any other tag.

---

### User Story 3 - Pins outlive containers and are released only on purpose (Priority: P3)

A pin is Shrine's record, not the container's. Redeploy, container recreation, and teardown never release it. Three things do: `shrine delete application`, `shrine delete team`, and a deploy of the artifact under a manifest-owned policy (`Always` or `IfNotPresent`). After a release, the next deploy under `Pinned` is a first deploy again and records a fresh pin.

**Why this priority**: Journey J8 and goal G1 depend on the pin surviving everything short of a deliberate act; the release paths are what keep a retired name from inheriting a stale version. It builds on User Story 1.

**Independent Test**: Pin an artifact, move the registry's `latest`, then exercise each release path and each non-release path in turn, deploying after each and checking whether the exact version stayed or moved to the new `latest`.

**Acceptance Scenarios**:

1. **Given** a pinned artifact whose container has been torn down, **When** the operator runs `shrine delete application <name>` and then deploys under `Pinned`, **Then** the deploy resolves the newest version again, records a new pin, and the output says it was pinned on this deploy.
2. **Given** a team with several pinned artifacts, torn down, **When** the operator runs `shrine delete team <name>` and later deploys the team again under `Pinned`, **Then** every artifact pins afresh.
3. **Given** a pinned artifact, **When** the operator changes its manifest to a fixed tag (so its policy becomes manifest-owned) and deploys, **Then** the artifact runs the manifest's version and its pin is released; **When** the operator then returns the manifest to `Pinned` and deploys, **Then** the deploy behaves as a first deploy and pins the current newest version.
4. **Given** a pinned artifact, **When** the operator tears the team down and deploys again, **Then** the pin is reused (no fresh resolution).
5. **Given** a pinned artifact, **When** the operator runs `shrine delete application <name> --dry-run`, **Then** the output says the pin would be released and nothing is released.
6. **Given** two teams each owning an artifact of the same name, **When** one team's artifact is deleted, **Then** the other team's pin is untouched.

---

### User Story 4 - The operator can see what each deploy decided, before and after it runs (Priority: P4)

Deploy output already states the exact version of every artifact. With this feature it also says how that version was decided: pinned on this deploy, reused from an existing pin, or manifest-owned. A dry run says, per pinned artifact, whether it would resolve the newest version and pin it or reuse the existing pin, and writes nothing: repeating a dry run leaves recorded state byte for byte unchanged.

**Why this priority**: Without the wording, a pinned deploy is indistinguishable from a floating one in the output, and dry run is the only way to preview a first pin safely. It adds nothing without User Stories 1 and 3.

**Independent Test**: Deploy a set with one artifact under each decision and compare the output lines; run the dry run twice with a pin present and once with none, diffing the recorded state before and after.

**Acceptance Scenarios**:

1. **Given** a first deploy under `Pinned`, **When** it completes, **Then** the artifact's output block states the exact version and that it was pinned on this deploy.
2. **Given** a later deploy of a pinned artifact, **When** it completes, **Then** the artifact's output block states the pinned exact version, that it was reused, and the date it was pinned.
3. **Given** a manifest-owned artifact, **When** it is deployed, **Then** its output lines are exactly what they were after ticket T2.
4. **Given** a pinned artifact with no pin yet, **When** the operator runs a dry run, **Then** the preview says the artifact would resolve the newest version and pin it, contacts no registry, and writes no pin.
5. **Given** a pinned artifact with a pin, **When** the operator runs a dry run, **Then** the preview shows the pin it would reuse, with its exact version, readable version, and date.
6. **Given** any deploy set, **When** the operator runs the dry run twice, **Then** the recorded state is byte for byte identical before and after both runs (PRD metric M6).

---

### User Story 5 - A pin the registry no longer serves stops the deploy before anything changes (Priority: P5)

A registry may stop serving an exact version that is no longer tagged. When a pinned artifact's exact version can no longer be fetched and is not present on the host, the deploy fails in the pre-deploy resolution step, with zero containers created, changed, or removed. The message names the artifact and the pinned exact version, says the registry no longer serves it, and names the manifest as the way out: deploying the artifact under a manifest-owned policy releases the pin. Once the bump command exists (ticket T6), the message will point at it instead.

**Why this priority**: Goal G5 for the pinned case. The failure already stops before any change thanks to ticket T2; this story owns the pinned message and its wording.

**Independent Test**: Pin an artifact against a registry the test controls, remove the image from the registry and from the host, deploy, and verify the failure text and that no container was touched.

**Acceptance Scenarios**:

1. **Given** a pinned artifact whose exact version the registry no longer serves and which is absent from the host, **When** the operator deploys, **Then** the deploy fails before any container or network operation, and the message names the artifact, the pinned exact version, says the registry no longer serves it, and tells the operator that deploying under a manifest-owned policy releases the pin.
2. **Given** the same artifact but with the image still present on the host, **When** the operator deploys, **Then** the deploy succeeds using the local image and makes no registry call.
3. **Given** a deploy set with several artifacts where one pinned artifact cannot be fetched, **When** the operator deploys, **Then** zero containers are created, changed, or removed for any artifact of the set (PRD metric M5).

---

### User Story 6 - The documentation explains the policy and the pin (Priority: P6)

An operator reading the manifest reference learns that the image pull policy has three values, what `Pinned` requires of the image and version fields, and how a pin is created, kept, and released. A contributor reading the project's own reference finds the pin record in the state layout.

**Why this priority**: Required by the ticket (R-28 partial, R-32) and by the repository's doc-and-code drift rule; last because it documents the other five stories.

**Independent Test**: Read the manifest reference and the contributor reference after the change and answer, from the text alone: which three values are accepted, what a `Pinned` manifest may and may not name, and which operations release a pin.

**Acceptance Scenarios**:

1. **Given** the manifest reference, **When** an operator reads the `spec.imagePullPolicy` rows and YAML blocks for Application and Resource, **Then** all three values are listed, the Resource `spec.version` row says it is required unless the policy is `Pinned`, and a subsection states the no-fixed-version rule and the pin lifecycle (created on first deploy, reused across redeploy, recreation, and teardown, released by `delete application`, `delete team`, and a deploy under a manifest-owned policy).
2. **Given** the contributor reference, **When** a contributor reads the state directory layout, **Then** the per-team pin record is listed with its fields.

---

### Edge Cases

- A pinned artifact whose newest image carries no registry exact version after the pull (for example an image loaded onto the host by hand and never pushed) cannot be pinned: the deploy fails in the resolution step naming the artifact, with zero changes, because a pin without an exact version would not survive a cache wipe. Manifest-owned artifacts keep tolerating a missing exact version, as after T2.
- Several artifacts under `Pinned` name the same repository: each gets its own pin, resolved and recorded per artifact, so a later deploy of one does not depend on another. Two artifacts pinned from the same `latest` on the same day hold the same exact version, by coincidence, not by rule.
- Two teams own artifacts of the same name: pins are per team and per artifact; neither team's deploy or delete touches the other's pin.
- A pinned artifact whose manifest now names a different repository than the one its pin was resolved from (the operator pointed the manifest at another image, still under `Pinned`): the old pin is for an image the manifest no longer names, so the deploy treats it as absent, resolves the newest version of the new repository, and replaces the pin. The output says it was pinned on this deploy.
- A `Pinned` image written with a registry alias (`reg:lab/hello-api`): the alias is expanded once, the pin records the expanded reference, and the exact version is fetched with the credentials of that registry, exactly as a tag pull is today.
- A pinned exact version absent from the host and a registry that needs credentials: the fetch uses the same credentials a tag pull of that registry uses; a failed authentication fails the resolution step with zero changes.
- A deploy set in which one artifact is pinned on this deploy and a later artifact then fails to resolve: no container or network is touched. The pin already recorded for the first artifact is kept, since it reflects a real resolution from the registry; the next deploy reuses it and says so.
- A pinned artifact whose image is present on the host but which the operator pulled by hand as `latest` after the pin: the deploy still runs the pinned exact version, not the newer local `latest`.
- A manifest under `Pinned` is applied alone with `shrine apply -f`: the same validation, pinning, reuse, output, and dry-run behaviour apply to that one artifact.
- A deploy scoped to one team resolves and pins that team's artifacts only; pins of other teams are neither read nor written.
- An installation whose state directory predates this feature has no pin records: every pinned artifact behaves as a first deploy; nothing needs migration.
- A pin record line that cannot be read (malformed by hand editing) is skipped, as a malformed host port line is today; the artifact then behaves as a first deploy and the line is rewritten by the pin.
- A manifest-owned deploy of an artifact that has no pin releases nothing and prints nothing about pins; the release is idempotent.
- Dry run of a manifest-owned artifact that still has a pin from an earlier `Pinned` deploy: the preview line is the manifest-owned line of T2 and the pin is not released, because dry run writes nothing; the real deploy releases it.
- A Resource under `Pinned` with no `version` and no image override resolves the newest version of its type's repository (for example `postgres`, which the registry reads as `postgres:latest`); its readable version is shown as `latest`.

## Requirements *(mandatory)*

### Functional Requirements

Identifiers in brackets bind each requirement to the design's T3 list and to the PRD.

- **FR-001** [T3-01, R-01]: The image pull policy field on Application and Resource manifests MUST accept the value `Pinned` beside `Always` and `IfNotPresent`, and MUST reject any other non-empty value at validation time, naming the field and the three accepted values. The two existing values MUST keep their current meaning exactly.
- **FR-002** [T3-02, R-02]: Under `Pinned` a manifest MUST NOT name a fixed version: an Application image carries no tag or the tag `latest`; a Resource omits `version` or sets it to `latest`; a Resource image override, when present, follows the Application rule; a digest reference is a fixed version. A violation MUST be a validation error reported together with the manifest's other validation errors, before any change is made, naming the artifact by kind and name and the field (`spec.image` or `spec.version`), and stating that the value names a fixed version while the image pull policy is `Pinned`.
- **FR-003** [T3-02, R-03]: Under `Pinned` a Resource's `version` MUST be optional, and the resource's repository is then its type, or its image override when one is set. Under `Always`, `IfNotPresent`, or no named policy, `version` MUST stay required, rejected with today's message, and still before any change is made.
- **FR-004** [T3-02, R-04]: The effective policy of an artifact MUST be the manifest's own field when named, else today's derived rule: `Always` for `latest` or no tag, `IfNotPresent` for any other tag. The configuration default of R-04 is ticket T4; this feature MUST leave the precedence open for it without changing the rule above.
- **FR-005** [T3-03, R-09]: The first deploy of an artifact under `Pinned` (no pin on record) MUST resolve the newest version of the manifest's repository from the registry, deploy that exact version, and record a pin holding the exact version in a form that can be fetched again, the readable version it was resolved from, and the date. A newest version without a registry exact version MUST fail the resolution step for that artifact.
- **FR-006** [T3-03, R-10]: Every later deploy of a pinned artifact MUST run the pinned exact version, across a plain redeploy, a redeploy that recreates the container, teardown followed by deploy, a wiped local image cache, and any combination, for as long as the registry serves it.
- **FR-007** [T3-03, R-13]: When the pinned exact version is present on the host, deploy MUST make no registry call for that artifact. When it is absent, deploy MUST fetch that exact version from the registry, never the tag, using the registry credentials a tag pull of that registry uses.
- **FR-008** [T3-08, TD-2]: Reusing a pin MUST NOT by itself recreate a running container: a redeploy with an unchanged manifest and a reused pin keeps the container.
- **FR-009** [T3-04, R-11]: A pin MUST outlive containers and teardown. Redeploy, container recreation, and teardown MUST NOT release it. `shrine delete application` and `shrine delete team` MUST release the pins of what they delete, after they release the other allocation-type state they release today; `delete application --dry-run` MUST say the pin would be released and release nothing.
- **FR-010** [T3-04, R-12, TD-6]: A deploy of an artifact under a manifest-owned policy (`Always` or `IfNotPresent`) MUST release that artifact's pin during resolution, so that the next deploy under `Pinned` behaves as a first deploy and records a fresh pin. Releasing a pin that does not exist MUST be a no-op.
- **FR-011** [T3-04]: Pins MUST be recorded per team and per artifact. A pin MUST be treated as absent when its repository is no longer the repository the manifest names; the deploy then pins afresh from the new repository and replaces the record.
- **FR-012** [T3-05, R-14]: When a pinned exact version is absent from the host and the registry no longer serves it, the deploy MUST fail in the pre-deploy resolution step with zero containers or networks created, changed, or removed, and the message MUST name the artifact by kind and name and the pinned exact version, say the registry no longer serves it, and name the way out available in this release: deploying the artifact under a manifest-owned policy releases the pin. The message will point at bump once ticket T6 lands.
- **FR-013** [T3-06, R-15]: Deploy output MUST state, per artifact, in the resolution block T2 introduced, whether the exact version was pinned on this deploy (with the exact version), reused from an existing pin (with the exact version and the date it was pinned), or manifest-owned (the T2 line, unchanged).
- **FR-014** [T3-07, R-16]: Dry run MUST print, per artifact under `Pinned`, either that it would resolve the newest version and pin it, or the existing pin it would reuse with its exact version, readable version, and date. Dry run MUST contact no registry and write nothing: it MUST neither record nor release a pin, and repeating it MUST leave recorded state byte for byte unchanged.
- **FR-015** [M2, G6]: Manifests that do not name `Pinned`, and installations with no pin records, MUST behave exactly as before this feature: same effective policy, same pulls, same containers, same recorded state apart from the absence of pins, and the same output lines, with new lines allowed only where this spec adds them.
- **FR-016** [T3-09, R-28]: The manifest reference MUST list the three values on both the Application and Resource `spec.imagePullPolicy` rows and YAML blocks, mark Resource `spec.version` as required unless the policy is `Pinned`, and state, in its own subsection, the no-fixed-version rule and the pin lifecycle.
- **FR-017** [T3-09, R-32]: The contributor reference's state directory layout MUST list the per-team pin record and its fields.
- **FR-018** [T3-10, TD-13]: The integration suite MUST prove the pin against a registry whose `latest` the suite itself moves, running on the CI runner and bound to the loopback interface, so that "latest moved and the pin did not" is asserted, not assumed.

### Key Entities

- **Image pull policy**: the per-artifact mode that decides who owns the version. `Always` and `IfNotPresent` are manifest-owned: the version is whatever the manifest names. `Pinned` is Shrine-owned: the manifest names only the repository and Shrine resolves, records, and keeps the version. The effective policy is the manifest's field, else the derived rule.
- **Repository**: the image name without a version (`postgres`, `ghcr.io/me/hello-api`, or an alias form such as `reg:lab/hello-api` that expands to one).
- **Exact version**: the registry's immutable identity of an image, a digest. Two fetches of the same exact version yield identical bytes.
- **Pin**: Shrine's record for one artifact of one team under `Pinned`: the reference that was resolved (expanded, so an alias never has to be re-expanded), the exact version in a fetchable form, and the date in UTC. Created by the first `Pinned` deploy, reused by every later one, released by `delete application`, `delete team`, and a manifest-owned deploy. Survives containers and teardown. Lives beside the team's deployment records, not inside them.
- **Readable form of a pin**: the tag the pin was resolved from (an untagged request reads as `latest`) followed by a short form of the exact version, twelve hexadecimal characters, matching how container ids are shortened today (TD-11). Used in output lines; the full exact version is shown where there is room.
- **Deployment record** (existing, extended by T1): already carries the image reference as written and the effective policy at deploy time; it is how later tickets know an artifact was deployed under `Pinned`. This feature writes `Pinned` into it when that is the effective policy and changes nothing else about it.
- **Resolution step** (existing, from T2): the pre-deploy pass that resolves every artifact's image before any container or network is touched. This feature adds the pinned decisions to it.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Across ten deploys of a pinned artifact that include at least one container recreation, one teardown and redeploy, and one wiped image cache, with the registry's `latest` moved after the first deploy, the exact version is identical ten out of ten times (PRD metric M1).
- **SC-002**: A manifest under `Pinned` that names a fixed version is rejected before any change is made, and the error text alone lets an operator find the manifest, the artifact, and the field to fix.
- **SC-003**: With no manifest naming `Pinned` and no pin records, the existing integration suites pass without edits to their assertions, and the deploy output of the existing fixtures differs only by lines this spec adds (PRD metric M2).
- **SC-004**: Repeated dry runs, with and without pins on record, leave recorded state byte for byte unchanged (PRD metric M6).
- **SC-005**: With one pinned artifact whose exact version is no longer served among any number of artifacts, a deploy creates, changes, and removes zero containers and zero networks (PRD metric M5), and the failure text names the artifact and the exact version.
- **SC-006**: After `delete application`, after `delete team`, and after a deploy under a manifest-owned policy, the next `Pinned` deploy records a pin whose exact version is the registry's current newest, and the output says it was pinned on this deploy. After teardown and redeploy, the previous pin is reused.
- **SC-007**: A deploy of a pinned artifact whose image is present on the host completes with no registry call for that artifact; one whose image is absent fetches by exact version and never by tag.
- **SC-008**: Every artifact in a deploy's output has exactly one resolution block, and the block of a `Pinned` artifact says either pinned-now or reused; a manifest-owned artifact's block is unchanged from T2.
- **SC-009**: From the manifest reference alone, a reader can name the three policy values, what a `Pinned` manifest may name, and the three operations that release a pin.

## Assumptions

- **Scope**: this ticket delivers the policy value, its validation, the pin record, pinning and reuse on deploy, pin release on the existing delete verbs and on manifest-owned deploys, the output and dry-run wording, the pinned failure message, the manifest and contributor references, and the controllable-registry integration fixture. Out of scope: the configuration default (T4), pins shown in `get`, `describe`, and `status` (T5), `shrine bump` (T6), `shrine delete resource` (T7), and the operator guide (T8). No CLI page changes, because no command or flag is added.
- **Name of the value**: `Pinned` (design TD-9, PRD OD-1). The design assumes an existing enum check on the field; there is none today, so this feature adds one. A manifest that names an unknown policy value is rejected from this release on, where before it silently behaved as `IfNotPresent`; the manifest reference has never documented any value but the two, so no documented manifest is affected.
- **Pin record**: one per team and per artifact, beside the team's deployment records, holding the resolved reference in expanded form, the exact version as a fetchable reference, and the UTC date (design section 3.4). It follows the lifecycle and file discipline of the host port records: survives teardown, written atomically, malformed lines skipped on read. Older state directories simply have no pins.
- **Exact version**: the registry digest of the repository the reference names, matched the way T2 settled (Docker Hub prefixes stripped on both sides); stored as `<repository>@sha256:…` so it can be fetched as is (design TD-1). An image that yields no digest cannot be pinned (FR-005).
- **Readable form**: `<tag>@<twelve hex>`; an untagged request reads as `latest` (design section 3.5, TD-11). The pinned-now and reused output lines take the shapes of design section 4.9: `📌 Pinned <team>.<name> at latest@3f2a9c1b4d7e` and `📌 Using pinned <team>.<name> 17@9c1b4d7e3f2a (since 2026-10-06)`, each indented two spaces under the T2 started line, exactly as T2's manifest-owned finished line is. The exact strings are fixed in this feature's output contract.
- **Dry-run lines**: the shapes of design section 4.4, `-> would resolve newest and pin` and `-> pinned <exact version> (<readable>, <date>)`, beside T2's `-> manifest-owned` line, which is unchanged. The preview reads a snapshot of the pins and never writes.
- **Pinned failure text**: `pinned exact version <repository@sha256:…> for <team>/<name> is no longer served by the registry; deploy the artifact under Always or IfNotPresent to release the pin, then return to Pinned` is the form; the exact wording is fixed in the output contract and will be replaced by the bump wording in T6. It renders through the existing generic error lines, as T2's failures do.
- **Where pins are written and released**: all pin writes happen inside the resolution step; `delete application` and `delete team` release; dry run only reads (design TD-8). A pin recorded for an artifact earlier in a deploy set is kept when a later artifact fails to resolve; it reflects a real resolution and the next deploy reuses it.
- **Repository change under `Pinned`**: the design does not say what happens when the manifest's repository no longer matches the pin's. This spec chooses to treat such a pin as absent and re-pin from the new repository (FR-011), because a pin is meaningless for an image the manifest no longer names. The owner may override this in clarification.
- **Validation placement**: the no-fixed-version rule and the version-required rule are checked after the effective policy is known, in the same pass that validates registry aliases today (design TD-7). The version-required message is unchanged; it moves from parse time to plan time but is still reported before any change is made and with the manifest's other errors.
- **Container recreation**: the container reconcile keeps using the local image identity as its version input (design TD-2), so reusing a pin never recreates a container and a future bump always will.
- **Registry credentials**: fetching by exact version uses the same per-registry credentials a tag pull uses today; `127.0.0.1` registries need no credentials and no configuration entry, which is what the integration fixture relies on (design section 5).
- **Integration fixture**: the suite starts a `registry:2` container bound to the loopback interface on the CI runner, pushes a small public image already on the runner as `latest`, moves `latest` by pushing another, and asserts the running container's exact version (design section 5, TD-13). Per the project's rules, integration tests are authored and compile-checked locally and run only in CI.
- **Open technical points** (design section 7) are settled in the plan, not here: whether the pin of a multi-architecture image is the index digest or the platform digest, and how local presence of an exact version is checked across the daemon versions the project supports. Either answer satisfies this spec on one host.
- **Dependencies**: ticket T2 (spec 032, merged in #61) provides the resolution step and the backend seam this feature extends; ticket T1 (spec 031, merged in #60) provides the deployment record that carries the effective policy.
