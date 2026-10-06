# Feature Specification: Deployed Version in get and describe

**Feature Branch**: `031-deployed-version-columns`
**Created**: 2026-10-06
**Status**: Draft
**Input**: User description: "Record the manifest image reference and effective pull policy with each deployment and show a VERSION column in shrine get deployed/applications/resources and the image and pull policy in shrine describe"
**Epic**: Pinned Image Versions, ticket T1 (issue #52). Source documents: [PRD](../epics/pinned-image-versions/prd.md) R-17 and R-18, journey J3; [technical design](../epics/pinned-image-versions/design.md) sections 3.3 and 4.7, decisions TD-2 and TD-4, requirement list T1-01 to T1-05; [tickets.md](../epics/pinned-image-versions/tickets.md) section T1.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See which version each artifact was deployed with (Priority: P1)

An operator asks Shrine what is deployed, for one team or for all teams, and sees next to each application and resource the image reference its manifest named when it was last deployed. The answer comes from Shrine's own records; the operator does not need Docker to be reachable, and never has to open the Docker CLI to answer "which version is that?".

**Why this priority**: This is the whole value of the ticket and the display surface the rest of the epic extends. Without it, the deployment listing names artifacts but not what they run.

**Independent Test**: Deploy the standard fixtures, run `shrine get deployed`, `shrine get applications`, and `shrine get resources`, with and without `--team`, and verify that every row carries the image reference from its manifest in a VERSION column placed after KIND.

**Acceptance Scenarios**:

1. **Given** the standard fixtures are deployed, **When** the operator runs `shrine get deployed`, **Then** every row shows, in a VERSION column after KIND, the image reference the artifact's manifest named.
2. **Given** the same deployment, **When** the operator runs `shrine get applications` or `shrine get resources`, **Then** the same VERSION column appears with the same values for the rows each command lists.
3. **Given** the same deployment, **When** the operator adds `--team <team>` to any of the three commands, **Then** the VERSION column is present and filled for that team's rows.
4. **Given** a manifest whose image is written with a registry alias (for example `reg:lab/hello-api:1.2.0`), **When** the operator lists deployments, **Then** VERSION shows the reference exactly as the manifest wrote it, not the expanded registry host and not an image digest.
5. **Given** a deployment, **When** the operator lists deployments while the container runtime is unreachable, **Then** the commands still succeed and show the VERSION column.
6. **Given** the listing before this feature, **When** the operator compares it to the listing after, **Then** the TEAM, NAME, KIND, and CONTAINER ID columns keep their headers, order, and values; VERSION is the only addition.

---

### User Story 2 - See the image and the pull policy of one artifact (Priority: P2)

An operator asks for the details of a single application or resource and sees, with the record Shrine already shows, the image reference the manifest named and the image pull policy that was in force when it was deployed.

**Why this priority**: It completes the audit journey for one artifact (J3) and is where later tickets add the pin and the running image. It depends on the same recorded values as User Story 1.

**Independent Test**: Deploy the standard fixtures, run `shrine describe app <name>` and `shrine describe resource <name>`, and verify both print an `Image:` line with the manifest's reference and a `Pull policy:` line with the effective policy.

**Acceptance Scenarios**:

1. **Given** a deployed application, **When** the operator runs `shrine describe app <name>`, **Then** the output includes `Image:` with the reference the manifest named and `Pull policy:` with the policy that was effective for that deploy.
2. **Given** a deployed resource, **When** the operator runs `shrine describe resource <name>`, **Then** the output includes the same two lines.
3. **Given** a manifest that declares no pull policy, **When** the operator describes the artifact, **Then** `Pull policy:` shows the policy Shrine derived for it (`Always` for `latest` or an untagged image, `IfNotPresent` for any other tag), not an empty value.
4. **Given** a deployed artifact, **When** the operator describes it with or without `--team`, **Then** the two lines are present in both cases and the existing lines are unchanged.

---

### User Story 3 - Records from a previous release keep working and heal themselves (Priority: P3)

An operator upgrades Shrine on a host that already has deployments recorded by an earlier release. Listing and describing still work; the version of each such artifact is shown as unknown until it is deployed again, at which point its record gains the version.

**Why this priority**: Purely additive rollout is a PRD commitment (section 10): no state migration, nothing breaks on upgrade. It is third because it only matters once the first two stories exist.

**Independent Test**: Rewrite a team's deployment records to the shape the previous release wrote (without the two new values), list and describe them, then deploy again without changing any manifest and list again.

**Acceptance Scenarios**:

1. **Given** deployment records written by a previous release, **When** the operator lists deployments, **Then** every such row lists normally with `-` in the VERSION column.
2. **Given** the same records, **When** the operator describes one of those artifacts, **Then** the output succeeds and shows `-` for both `Image:` and `Pull policy:`.
3. **Given** the same records, **When** the operator deploys again without changing any manifest, so that no container needs to be recreated, **Then** the next listing shows the image reference for every artifact that was deployed, and nothing else about those artifacts changed.
4. **Given** a mix of records with and without the new values in one team, **When** the operator lists the team, **Then** each row shows its own value or `-` independently of its neighbours.

---

### Edge Cases

- A record from before config hashes were stored (three values on the line) still loads, with the version shown as `-`.
- A resource manifest that names no explicit image deploys with the image Shrine defaults for it from its type and version; that defaulted reference is what is recorded and shown.
- When a manifest's image reference changes between deploys, the record shows the new reference once the new deploy succeeds; until then it shows the previous one, because the record describes what was deployed, not what the manifest says now.
- When a deploy is up to date and only has to keep the container running, the record is still refreshed, so a legacy record heals on the first deploy after the upgrade.
- When a deploy fails before the container is started, no record is written, exactly as today.
- Teardown and `delete application` remove the whole record as today; the recorded version does not outlive the deployment.
- Image references never contain spaces, so a record line stays unambiguous when the two values are appended.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001** [T1-01, R-17]: Every successful deploy of an application or resource MUST record, alongside that artifact's existing deployment record, the image reference exactly as its manifest named it (a registry alias stays unexpanded) and the image pull policy that was effective for that deploy.
- **FR-002** [T1-02, R-17]: The recorded deployment MUST carry the two values as a fifth and sixth value after the four it holds today, and MUST read records that lack them (written by earlier releases) as having an unknown image and an unknown policy. When a record is written, all six values MUST be written, even when some are empty.
- **FR-003** [T1-03, R-17]: `shrine get deployed`, `shrine get applications`, and `shrine get resources` MUST print a VERSION column immediately after KIND and before CONTAINER ID, with and without `--team`, from recorded state alone, without contacting the container runtime.
- **FR-004** [T1-03, R-17]: The VERSION column MUST show the recorded image reference in full, as the manifest wrote it, and `-` when the record has none.
- **FR-005** [T1-03]: The existing TEAM, NAME, KIND, and CONTAINER ID columns MUST keep their headers, their relative order, and their values.
- **FR-006** [T1-04, R-18]: `shrine describe app <name>` and `shrine describe resource <name>` MUST print an `Image:` line with the recorded image reference and a `Pull policy:` line with the recorded policy, each showing `-` when the record has no value, while keeping every line they print today.
- **FR-007** [T1-05, R-17]: A record without the two values MUST gain them on the next successful deploy of that artifact, including a deploy that finds the container already up to date and only ensures it is running.
- **FR-008**: The policy recorded MUST be the effective one: the manifest's declared policy when present, otherwise the policy Shrine derives from the image tag. Any policy value Shrine supports now or later MUST be recordable without a further change to the record shape.
- **FR-009**: Existing behaviour MUST be unchanged for everything this feature does not name: deploy output, teardown, delete, status, the config hash that decides whether a container is recreated, and the set of commands and flags.

### Key Entities

- **Deployment record**: Shrine's belief about one deployed artifact in one team: its kind, its name, the container it created, the configuration fingerprint that decides whether the container is up to date, and, new here, the image reference the manifest named and the effective pull policy. One record per artifact per team; written after the container is started; removed on teardown or delete.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After deploying the standard fixtures, 100% of the rows printed by `shrine get deployed`, `shrine get applications`, and `shrine get resources`, with and without `--team`, show the image reference from the artifact's manifest in the VERSION column.
- **SC-002**: For every deployed artifact, `shrine describe` shows the same image reference as the listing and a non-empty pull policy.
- **SC-003**: A state directory written by the previous release lists and describes without error; every pre-existing row shows `-` as its version; after one deploy with unchanged manifests, zero rows show `-`.
- **SC-004**: The TEAM, NAME, KIND, and CONTAINER ID columns are unchanged in header, order, and values between the release before this feature and this one; the existing integration suites pass without edits to their assertions.
- **SC-005**: The three listing commands succeed with the container runtime stopped or unreachable.
- **SC-006**: An operator can answer "which image was this artifact deployed with?" from Shrine's output alone, without the Docker CLI (journey J3).

## Clarifications

### Session 2026-10-06

Answered from the epic documents by the ticket's agent; no owner question was needed.

- Q: Should `describe` print placeholders or omit the two new lines when the record predates this feature? → A: Print both lines with `-`, matching the VERSION column, so the output shape is stable and a legacy record is recognisable (design 4.7 names the lines unconditionally).
- Q: Which reference is recorded for a Resource that names no `spec.image`? → A: The image Shrine defaults for it from `type` and `version`, because that is the reference the manifest effectively names and the one the deploy used (design section 1, resource image defaulting).
- Q: Where exactly do `Image:` and `Pull policy:` go in the describe output? → A: Directly after `Kind:`, before `Container ID:`, so the identity lines come first and later tickets (T5) can append the pin and the running image after them without reordering.
- Q: The PRD's vocabulary defines "version" as the tag alone; does the VERSION column show only the tag or the whole image reference? → A: The whole reference as the manifest wrote it (repository and tag, alias unexpanded). The design binds the column to the recorded image reference (section 4.7), and a tag alone is ambiguous across repositories. The header stays VERSION because T5 fills the same column with the readable form for pinned artifacts.
- Q: Does a record written before config hashes existed (three values) count as legacy? → A: Yes. Any record lacking the fifth and sixth values reads as unknown image and unknown policy, whatever else it lacks.

## Assumptions

- **Pull policy values**: today the effective policy is `Always` or `IfNotPresent`; the epic's T3 adds `Pinned`. This feature records whatever effective value the deploy used and makes no assumption about the set of values, so T3 and T5 need no further change to the record (design TD-4).
- **PRD wording**: R-17 does not yet mention the policy; tickets.md and design section 3.3 prevail, so both the image reference and the policy are recorded here. The PRD amendment is listed in tickets.md.
- **What "the reference the manifest named" means**: the string in `spec.image` as written, including a `reg:<alias>/` prefix when one is used; never the expanded registry host, never a digest, never the id of the local image.
- **Column widths are not a contract**: only the header names, the relative order of the columns, and the values are. The separator line and the column widths may change to fit the new column.
- **Out of scope**: pins, exact versions, the running image, `shrine status`, and any documentation page beyond the agent brief's state-layout line. No documentation page today documents the listing columns, the describe lines, or the deployment record format, so the docs site is unchanged.
- **Rollout**: purely additive. No state migration; records from earlier releases are read as "version unknown" and heal on the next deploy (PRD section 10).
- **Platform containers**: the gateway container the Traefik plugin starts is recorded like any other deployment and therefore also gains the two values; nothing lists it today and nothing changes for it.
