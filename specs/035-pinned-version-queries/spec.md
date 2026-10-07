# Feature Specification: Pinned Versions in `get`, `describe`, and `status`

**Feature Branch**: `035-pinned-version-queries`
**Created**: 2026-10-06
**Status**: Draft
**Input**: User description: "https://github.com/CarlosHPlata/shrine/issues/56"
**Epic**: Pinned Image Versions, ticket T5 (issue #56). Source documents: [prd.md](../epics/pinned-image-versions/prd.md) (journey J3; goal G3; requirements R-17, R-18, R-19, R-20; metric M3; open decisions OD-3, OD-4), [design.md](../epics/pinned-image-versions/design.md) (decision TD-11; sections 3.3, 3.4, 3.5, 4.7; requirement list T5-01 to T5-04), [tickets.md](../epics/pinned-image-versions/tickets.md#t5-pinned-versions-in-get-describe-and-status) section T5. Builds on ticket T1 ([spec 031](../031-deployed-version-columns/spec.md)), which gave every deployment record its image reference and effective policy and added the VERSION column and the `Image:` and `Pull policy:` lines, and on ticket T3 ([spec 033](../033-pinned-image-policy/spec.md)), which introduced the `Pinned` policy and the pin record.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Read the pinned version in the deployment listing (Priority: P1)

An operator lists what is deployed, for one team or for everything, and reads for each pinned artifact the version in a form a person can read: the readable version it was resolved from and a short form of the exact version, side by side in the VERSION column that ticket T1 added. Artifacts that are not pinned keep showing the reference their manifest named at deploy time. The listing needs no access to the container runtime, so it answers "which version?" on a host where Docker is down.

**Why this priority**: This is the listing half of journey J3 and the PRD's metric M3: one command answers "which version is deployed?" for a team or for everything, without the Docker CLI. Without it, a pinned artifact's row shows only the repository name, which says nothing about what runs.

**Independent Test**: Deploy a fixture set holding one pinned artifact and one manifest-owned artifact against the loopback registry fixture of ticket T3; list with each of the three listing commands, with and without `--team`, and verify the pinned row reads `<readable>@<twelve hex>`, the manifest-owned row reads the manifest reference as before, and the commands succeed with the container runtime stopped.

**Acceptance Scenarios**:

1. **Given** a pinned Application was deployed from the reference `127.0.0.1:<port>/shrine/whoami:latest`, **When** the operator runs `shrine get deployed`, **Then** its VERSION cell reads the readable version and the short exact version joined by `@`, for example `latest@3f2a9c1b4d7e`, where the twelve characters are the first twelve of the exact version the pin recorded.
2. **Given** a pinned Resource whose manifest named no version (so the readable version is `latest`), **When** the operator lists deployments, **Then** its VERSION cell reads `latest@<twelve hex>`.
3. **Given** a pinned artifact whose pin was resolved from an exact version rather than a tag, **When** the operator lists deployments, **Then** its VERSION cell reads the twelve-character short exact version alone, with no readable part and no `@`.
4. **Given** a manifest-owned artifact deployed beside the pinned one, **When** the operator lists deployments, **Then** its VERSION cell is unchanged from ticket T1: the reference the manifest named, alias unexpanded.
5. **Given** the same deployment, **When** the operator runs `shrine get applications`, `shrine get resources`, or any of the three commands with `--team <team>`, **Then** the same VERSION values appear for the rows each command lists, and the column order TEAM, NAME, KIND, VERSION, CONTAINER ID is unchanged.
6. **Given** a pinned deployment, **When** the operator lists deployments while the container runtime is unreachable, **Then** the commands succeed and show the pinned version, because the listing reads only Shrine's own records.
7. **Given** the listing before this feature, **When** the operator compares it to the listing after for manifest-owned artifacts and for records from earlier releases, **Then** every value is identical; only pinned rows change.

---

### User Story 2 - See the pin, its date, and what is actually running for one artifact (Priority: P2)

An operator asks about a single application or resource and sees, beside the lines ticket T1 added, the pin in full: the exact version as a pullable reference and the date it was pinned. Below it, the image the running container was started from. When the pin on record and the running image agree, the artifact runs what is pinned. When a new pin has been recorded but no deploy has applied it yet, the two lines differ, and the operator knows a deploy is waiting. When the container runtime cannot be asked, the running-image line says so and the rest of the description still prints.

**Why this priority**: This is the second half of journey J3 ("I ask about a single resource and also see when its version was pinned and whether a newer pin is waiting to be deployed") and completes PRD requirement R-18. It is the only place the full exact version and the date are shown, which the PRD's risk list relies on to keep readable versions honest.

**Independent Test**: Deploy the pinned fixture; describe the artifact and verify the `Pinned:` and `Running image:` lines; replace the pin on record without deploying and verify the two lines now differ; stop the container runtime and verify the command still succeeds with the running-image line marked unavailable.

**Acceptance Scenarios**:

1. **Given** a deployed pinned artifact, **When** the operator runs `shrine describe app <name>` or `shrine describe resource <name>`, **Then** the output keeps every line of ticket T1 (`Image:` with the manifest reference, `Pull policy:` reading `Pinned`) and adds `Pinned:` with the full pullable exact version (`<repository>@sha256:<64 hex>`), the readable version it was resolved from, and the date it was pinned, and `Running image:` with the reference the running container was created from.
2. **Given** a pinned artifact that was deployed and not changed since, **When** the operator describes it, **Then** the exact version on the `Pinned:` line and the reference on the `Running image:` line are the same.
3. **Given** a pinned artifact whose pin on record was replaced after its last deploy (as a future bump will do), **When** the operator describes it, **Then** `Pinned:` shows the new exact version and date and `Running image:` still shows the old one, so the difference is visible from the two lines.
4. **Given** a deployed manifest-owned artifact, **When** the operator describes it, **Then** no `Pinned:` line is shown, `Pull policy:` reads the manifest-owned policy as before, and `Running image:` shows the reference the container was created from (the expanded registry reference with its tag).
5. **Given** any deployed artifact, **When** the operator describes it while the container runtime is unreachable, **Then** the command succeeds, every line that comes from Shrine's records is printed as usual, and the `Running image:` line reads as unavailable rather than failing the command.
6. **Given** any deployed artifact, **When** the operator describes it with or without `--team`, **Then** the same lines are shown in both cases, and the ambiguity error for a name found in several teams is unchanged.
7. **Given** a deployment record from an earlier release with no image and no policy, **When** the operator describes it, **Then** the output is as ticket T1 left it (`-` for `Image:` and `Pull policy:`), with no `Pinned:` line, plus the `Running image:` line.

---

### User Story 3 - See the running image in the live status view (Priority: P3)

An operator runs the status command for the platform, a team, or one artifact and sees, next to each container's running state, the image it was started from. For a pinned container that is its exact version; for a manifest-owned one it is the tag it was pulled by. The status command already needs the container runtime, so nothing changes about when it succeeds.

**Why this priority**: PRD requirement R-20, a should-have taken per open decision OD-3: the status table is the only live view, and with the image on it an operator sees drift after a bump without opening `describe` for each artifact.

**Independent Test**: Deploy the pinned fixture beside a manifest-owned one and run the status command for the team; verify the new IMAGE column shows the exact version for the pinned row and the tag reference for the other, and that the existing columns are unchanged.

**Acceptance Scenarios**:

1. **Given** a deployed team, **When** the operator runs `shrine status <team>`, **Then** the table has an IMAGE column beside the running state, and each row shows the reference the container was created from.
2. **Given** a pinned container in that team, **When** the operator reads its IMAGE cell, **Then** it shows the exact version it was created from, shortened to its first twelve characters in the table as other exact values are.
3. **Given** a manifest-owned container in that team, **When** the operator reads its IMAGE cell, **Then** it shows the tag reference it was pulled by.
4. **Given** the status table before this feature, **When** the operator compares it to the table after, **Then** the NAME, KIND, RUNNING, STATUS, and IMAGE ID columns keep their headers, order, and values; IMAGE is the only addition.
5. **Given** the platform-wide, application, and resource forms of the status command, **When** the operator runs each, **Then** the IMAGE column is present wherever the table is printed.

---

### User Story 4 - Pins of artifacts that are not deployed stay invisible (Priority: P4)

An operator tears a team down. Ticket T3 keeps the pins so the team comes back on the same versions, but nothing an operator can run shows them: the artifact is absent from the listing, describing it reports it as not deployed, and no command lists pins on their own. The pin reappears in the output only once the artifact is deployed again.

**Why this priority**: PRD requirement R-19 and the non-goal "showing versions of artifacts that are torn down". It is a guarantee about what is never shown, so it is tested rather than built, but the acceptance of the ticket names it.

**Independent Test**: Deploy the pinned fixture, tear the team down, then list, describe, and run status; verify none of them shows the artifact or its pin, and that the pin is shown again after redeploy.

**Acceptance Scenarios**:

1. **Given** a pinned artifact whose team was torn down, **When** the operator lists deployments, **Then** the artifact is not listed.
2. **Given** the same artifact, **When** the operator describes it, **Then** the command reports it as not deployed, with today's not-found message, and prints no pin.
3. **Given** the same artifact, **When** the operator deploys the team again, **Then** the listing and the description show the pin again, with the same exact version and date as before the teardown.
4. **Given** any state of pins, **When** the operator runs any command, **Then** there is no output that enumerates pins independently of deployment records.

---

### User Story 5 - The documentation shows how to read a pinned version (Priority: P5)

An operator reading the manifest reference's image pull policy subsection learns how a pinned artifact appears in the listing (readable version, `@`, short exact version), where to find the full exact version and the pin date, and that a `Pinned:` line that differs from `Running image:` means a deploy is waiting. The command reference pages for `describe` and `status` describe the added lines and column. A contributor reading the project's own reference finds the query commands' new output named.

**Why this priority**: The repository's rule that documentation and code change together; last because it documents the other four stories. The full operator guide is ticket T8's.

**Independent Test**: From the manifest reference and the command pages alone, answer: what does `latest@3f2a9c1b4d7e` mean, where is the full exact version shown, and how do I tell a recorded-but-not-deployed pin from a deployed one.

**Acceptance Scenarios**:

1. **Given** the manifest reference, **When** an operator reads the image pull policy subsection, **Then** it shows the readable form with an example, says where the full exact version and date appear, and explains the `Pinned:` versus `Running image:` difference.
2. **Given** the command reference pages for `describe app`, `describe resource`, and the status commands, **When** an operator reads them, **Then** they name the `Pinned:` and `Running image:` lines and the IMAGE column, and they are regenerated from the command help so they do not drift.
3. **Given** the contributor reference, **When** a contributor reads the CLI reference lines for the query commands, **Then** the new column and lines are named.

---

### Edge Cases

- A deployment record says `Pinned` but no pin is on record for the artifact (the pin file was edited or lost): the VERSION cell shows the recorded manifest reference, as a manifest-owned row does, and `describe` shows `Pinned:` as `-`. Nothing fails.
- The pin's readable version comes from a reference with no tag: it reads `latest`, as ticket T3 amended for the readable form.
- The pin was resolved from an exact version (a digest bump): the readable form is the short exact version alone, in tables; `describe` shows the full reference.
- The exact version on record is shorter than twelve characters or malformed: the short form is whatever is there, never padded, and nothing fails.
- The container recorded for a deployment no longer exists in the runtime (removed out of band): `describe` prints the running-image line as unavailable and succeeds; `status` behaves as it does today for a missing container.
- The container runtime is unreachable: `get` is unaffected; `describe` succeeds with the running-image line unavailable; `status` fails as it does today, because it has nothing to show without the runtime.
- A manifest-owned artifact whose image was written with a registry alias: VERSION keeps the alias form from the record, as T1 specified; `Running image:` and the status IMAGE column show the expanded reference the container was created from, because that is what the runtime knows.
- A pinned record whose pin is for a different repository than the record's image (the manifest was edited to another image and not yet redeployed): the listing shows the pin on record, because ticket T3's repository-match rule applies at deploy time, not at query time; the next deploy replaces the pin and the listing follows.
- A team with a mix of pinned, manifest-owned, and legacy records: each row is rendered by its own record, independently of its neighbours.
- A name present in several teams: `describe` and the artifact forms of `status` keep today's ambiguity error; `--team` resolves it and the new lines are shown.
- A team that was torn down and whose pins remain: not listed, not describable, pins not shown anywhere.
- Dry run, deploy, apply, and delete output: unchanged by this feature, which adds no write.

## Requirements *(mandatory)*

### Functional Requirements

Identifiers in brackets bind each requirement to the design's T5 list and to the PRD.

- **FR-001** [T5-01, R-17, TD-11]: In the VERSION column of `shrine get deployed`, `shrine get applications`, and `shrine get resources`, with and without `--team`, an artifact whose recorded effective policy is `Pinned` MUST show the readable form of its pin: the readable version, `@`, and the first twelve characters of the exact version; when the pin was resolved from an exact version, the short exact version alone. Any other record MUST keep showing the recorded manifest reference exactly as ticket T1 left it.
- **FR-002** [R-17]: The three listing commands MUST read only Shrine's own records to fill VERSION and MUST succeed without access to the container runtime.
- **FR-003** [T5-01]: A `Pinned` record with no pin on record MUST show the recorded manifest reference in VERSION and `-` on the `Pinned:` line of `describe`; it MUST NOT fail either command.
- **FR-004** [T5-02, R-18]: `shrine describe app <name>` and `shrine describe resource <name>` MUST keep every line of ticket T1 and, for a `Pinned` record with a pin, add a `Pinned:` line carrying the full pullable exact version, the readable version it was resolved from, and the date it was pinned. A manifest-owned record MUST show no `Pinned:` line.
- **FR-005** [T5-02, R-18]: `describe` MUST add a `Running image:` line showing the reference the recorded container was created from, for every record, so that a pin recorded after the last deploy is visible as a difference between `Pinned:` and `Running image:`.
- **FR-006** [T5-02, section 4.7]: When the container runtime is unreachable or the recorded container cannot be inspected, `describe` MUST still succeed, print every record-sourced line, and print the `Running image:` line as unavailable. The commands MUST keep working with no runtime at all, as the existing no-Docker scenario asserts.
- **FR-007** [T5-04, R-20, OD-3]: Every form of `shrine status` that prints the container table MUST add an IMAGE column beside the running state, showing the reference the container was created from; the existing columns, including IMAGE ID, MUST keep their headers, order, and values.
- **FR-008** [OD-4, TD-11]: In tables, an exact version MUST be shortened to its first twelve characters; `describe` MUST show it in full. A value shorter than twelve characters MUST be shown as is.
- **FR-009** [T5-03, R-19]: Pins MUST only ever appear in output beside a deployment record. An artifact with no deployment record MUST be absent from the listing, MUST be reported by `describe` with today's not-found message, and MUST show no pin; no command MAY list pins on their own.
- **FR-010** [Out of scope: any write]: The commands in this feature MUST NOT create, change, or release a deployment record, a pin, a container, or any other state; all four remain read-only.
- **FR-011** [M2]: For manifest-owned artifacts and for records from earlier releases, the listing output MUST be byte for byte what ticket T1 produces, and the `describe` output MUST differ only by the added `Running image:` line; the existing integration suites MUST pass without edits to their assertions, apart from added output lines.
- **FR-012** [R-28, R-30]: The manifest reference's image pull policy subsection MUST explain the readable form, where the full exact version and date are shown, and the meaning of a `Pinned:` line that differs from `Running image:`; the command reference pages for `describe` and `status` MUST be regenerated after their help text names the new lines and column; the contributor reference's CLI lines MUST name them.
- **FR-013**: The integration suites for `get`, `describe`, and `status` MUST gain scenarios on a pinned fixture (readable form in VERSION, `Pinned:` and `Running image:` agreeing after a deploy and differing after the pin is replaced, the IMAGE column, nothing shown after teardown), written before the implementation and run in CI.

### Key Entities

- **Deployment record** (existing, from T1): what Shrine remembers about one deployed artifact: kind, name, container, configuration hash, the image reference the manifest named, and the effective policy. Every query in this feature starts from it; nothing is shown without one.
- **Pin** (existing, from T3): Shrine's record of a Shrine-owned version: the reference it was resolved from, the pullable exact version, and the date. Read here, never written. It is shown only when a deployment record with policy `Pinned` points at it.
- **Readable form**: how a pin appears in a table: the readable version (the tag of the reference it was resolved from, `latest` when untagged), `@`, and the twelve-character short exact version; the short exact version alone when the pin was resolved from an exact version.
- **Running image**: the reference the running container was created from, as the container runtime reports it: the exact version for a pinned container, the expanded tag reference for a manifest-owned one. Live information, so it is unavailable without the runtime.
- **Difference**: the state in which the pin on record and the running image disagree, meaning a pin was recorded after the last deploy. Shown, not computed: the operator reads it from the two lines.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After deploying a pinned fixture beside a manifest-owned one, each of the three listing commands, with and without `--team`, shows the readable form for the pinned row and the unchanged manifest reference for the other, in one run each and with the container runtime stopped (PRD metric M3 shape).
- **SC-002**: For a freshly deployed pinned artifact, `describe` shows a `Pinned:` exact version that equals the `Running image:` reference; after the pin on record is replaced without a deploy, the two differ, and an operator can state from the output alone which version runs and which is waiting.
- **SC-003**: With the container runtime unreachable, `describe` succeeds for every deployed artifact and the only line that changes is `Running image:`.
- **SC-004**: The status table for a team shows the IMAGE column for every row, with the exact version for pinned rows and the tag reference for manifest-owned rows, and the five existing columns are unchanged.
- **SC-005**: After a teardown, zero commands show the torn-down artifact or its pin; after redeploy, the pin is shown again with the same exact version and date.
- **SC-006**: The existing `get`, `describe`, and `status` integration suites pass without edits to their assertions, apart from added output lines (PRD metric M2).
- **SC-007**: From the manifest reference and the command pages alone, a reader can explain `latest@3f2a9c1b4d7e`, say where the full exact version and date are shown, and tell a waiting pin from a deployed one.

## Assumptions

- **Scope**: this ticket delivers the pinned readable form in VERSION, the `Pinned:` and `Running image:` lines in `describe`, the IMAGE column in `status`, the invisibility of pins without a record, and the documentation of FR-012. Out of scope: any write; `shrine bump` (T6), which is the command that will make a pin differ from the running image; `shrine delete resource` (T7); the operator guide (T8). No new command or flag is added.
- **Readable form**: exactly design section 3.5 with ticket T3's amendment: `<tag>@<twelve hex>`, `latest` for an untagged reference, the short digest alone for a digest reference. The twelve characters are the first twelve of the hexadecimal part of the exact version, matching how container ids are shortened today (OD-4, TD-11).
- **`Pinned:` line layout**: one line carrying the full pullable reference, the readable version, and the date in `YYYY-MM-DD`, for example `Pinned:       127.0.0.1:5000/shrine/whoami@sha256:<64 hex> (latest, 2026-10-06)`. The exact string is fixed in the plan's contract; the spec requires the three facts on the line.
- **`Running image:` content**: the reference the container runtime reports the container was created from, in full in `describe`. For a pinned container that is the exact version reference ticket T3 creates containers from; for a manifest-owned container it is the expanded tag reference. When unavailable, the line reads `unavailable` with the reason in parentheses; exact wording in the plan's contract.
- **Status IMAGE column**: the same reference as `Running image:`, with an exact version shortened to twelve characters so the table stays readable, consistent with the IMAGE ID column beside it. This is the one presentation choice the design leaves open; confirm in clarification if the owner prefers the full reference in the table.
- **Making a pin differ before bump exists**: the integration scenario for the difference replaces the pin on record directly in Shrine's state, since T6 lands in the same wave and may not be merged first. The scenario reads the same way once bump exists.
- **Pin lookup for `describe` of a torn-down artifact**: `describe` searches deployment records only and keeps today's not-found message, which this spec reads as "reports it as not deployed"; it never widens its search to pins, which is what keeps torn-down pins invisible (R-19).
- **Runtime access for `describe`**: `describe` gains a read-only connection to the container runtime, of the kind `delete application` already uses, and treats a failed connection or a failed inspection as "unavailable" rather than as an error. `get` gains no such connection.
- **Legacy records**: a record without image and policy shows `-` for both, as T1 left it, no `Pinned:` line, and the `Running image:` line like any other record.
- **Documentation locations**: the readable form and the difference go into the manifest reference's existing image pull policy subsection; the command pages for `describe app`, `describe resource`, and the status commands are regenerated from the command help after it names the new lines and column; the contributor reference's CLI reference lines for the query commands are updated.
- **Integration scenarios**: per the project's rules, authored first, compile-checked locally, and run only in CI. The pinned fixture is the loopback registry fixture ticket T3 introduced; the `get`, `describe`, and `status` suites each gain scenarios on it.
- **Dependencies**: ticket T1 (spec 031) provides the image and policy in the record and the VERSION column; ticket T3 (spec 033) provides the pins. Shares with its wave only the deployment query code with T7 and nothing with T6.
