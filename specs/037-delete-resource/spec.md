# Feature Specification: `shrine delete resource` and pin release on every delete

**Feature Branch**: `037-delete-resource`
**Created**: 2026-10-07
**Status**: Draft
**Input**: User description: "https://github.com/CarlosHPlata/shrine/issues/58"
**Epic**: Pinned Image Versions, ticket T7 (issue #58). Source documents: [prd.md](../epics/pinned-image-versions/prd.md) (journey J8; requirements R-11, R-27, R-30), [design.md](../epics/pinned-image-versions/design.md) (section 4.8; requirement list T7-01 to T7-03), [tickets.md](../epics/pinned-image-versions/tickets.md#t7-shrine-delete-resource-and-pin-release-on-every-delete) section T7. Builds on ticket T3 ([spec 033](../033-pinned-image-policy/spec.md)), which introduced the pin record and made `delete application` and `delete team` release pins, and on the existing `shrine delete application`, whose behaviour this ticket mirrors for resources.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Retire a resource for good (Priority: P1)

An operator has torn a team down and decided that one of its resources, say a database, is not coming back. They tell Shrine to delete that resource. Shrine forgets the resource's deployment record and releases the image pin it held, and says what it released. If a resource of the same name is deployed later under the pinned policy, it starts fresh: the first deploy resolves the newest version again and records a new pin instead of inheriting the retired one.

**Why this priority**: Journey J8 ("retire an artifact") and PRD requirement R-27. Today the retire path exists for applications only; a resource that was deployed once keeps its record and its pin forever, so a future resource with the same name inherits a stale version. This story is the whole reason the ticket exists.

**Independent Test**: Against the loopback registry fixture of ticket T3, deploy a team with one pinned resource, tear the team down, run `shrine delete resource <name>`, verify the output names the released pin and the removed record, verify neither is on record any more, move the registry's newest version, deploy again, and verify the resource pins afresh at the new newest version.

**Acceptance Scenarios**:

1. **Given** a pinned resource that was deployed and then torn down, so its container and its deployment record are gone but its pin remains, **When** the operator runs `shrine delete resource <name>`, **Then** the command succeeds, the output says the image pin was released for `<team>/<name>`, and no pin is on record afterwards. **Given** instead a resource whose container was removed outside Shrine, so its record outlived it, **Then** the output also says the deployment record was removed, and neither is on record afterwards.
2. **Given** the delete in scenario 1 succeeded and the registry's newest version has moved since the original pin, **When** the operator deploys the team again, **Then** the resource is created from the current newest version, a new pin is recorded, and the deploy output says it was pinned on this deploy.
3. **Given** a resource that was deployed under a manifest-owned policy and torn down, so it has a record but no pin, **When** the operator deletes it, **Then** the output says the deployment record was removed and says nothing about a pin.
4. **Given** a name that no team holds a resource record or pin for, **When** the operator deletes it, **Then** the command succeeds and says there is nothing to delete for that resource.
5. **Given** two teams each owning a resource of the same name, **When** one team's resource is deleted with `--team`, **Then** the other team's record and pin are untouched.

---

### User Story 2 - A live resource cannot be deleted (Priority: P2)

An operator runs the delete while the resource's container still exists. Shrine refuses, says the resource still has a container, and names the teardown command for its team as the way out. Nothing is released.

**Why this priority**: PRD requirement R-27's first clause and the rule `delete application` already follows: the container runtime is authoritative, and forgetting a record while its container runs would leave the container orphaned. Without this refusal story 1 would be unsafe.

**Independent Test**: Deploy a pinned resource, run `shrine delete resource <name>` without tearing down, verify the failure message names the resource and the teardown command, and verify the record and the pin are unchanged.

**Acceptance Scenarios**:

1. **Given** a deployed resource whose container exists, **When** the operator runs `shrine delete resource <name>`, **Then** the command fails, the message says the resource `<team>/<name>` still has a container and names `shrine teardown <team>` as the step to take first, and the deployment record and pin are unchanged.
2. **Given** the refusal in scenario 1, **When** the operator tears the team down and runs the delete again, **Then** it succeeds as in story 1.

---

### User Story 3 - Preview what a delete would release (Priority: P3)

Before deleting, an operator wants to see what Shrine holds for the resource. With `--dry-run` the command lists what it would release and remove, marked as a dry run, and changes nothing.

**Why this priority**: PRD requirement R-27 names the flag, and the sibling `delete application` offers it; its absence on `delete resource` would be a surprise. It needs stories 1 and 2 to exist.

**Independent Test**: With a torn-down pinned resource, run the delete with `--dry-run`, verify the output names the pin and the record that would go, and verify both are still on record afterwards.

**Acceptance Scenarios**:

1. **Given** a torn-down pinned resource, **When** the operator runs `shrine delete resource <name> --dry-run`, **Then** the output, marked as a dry run, says the image pin (with its exact version) would be released for `<team>/<name>`, plus that the deployment record would be removed when one is still held, and everything remains on record.
2. **Given** a name nothing is held for, **When** the operator runs the dry run, **Then** the output, marked as a dry run, says there is nothing to delete.
3. **Given** a resource whose container exists, **When** the operator runs the dry run, **Then** the same refusal as without `--dry-run` is printed, because the refusal needs no write to decide.

---

### User Story 4 - Find the resource by name, or name its team (Priority: P4)

An operator deletes a resource without saying which team owns it. When exactly one team holds a record or pin for that name, Shrine finds it. When several teams do, Shrine refuses with the same ambiguity error the other per-artifact commands use, and the operator adds `--team`. A name that appears as an application in one team and as a resource in another is not ambiguous for `delete resource`, because only resource records are searched.

**Why this priority**: PRD requirement R-27 names `--team`, and `delete application` already resolves the team this way. Mirroring it keeps the two delete verbs interchangeable in the operator's hands.

**Independent Test**: Seed two teams each holding a resource record of the same name, run the delete without `--team` and verify the ambiguity error, run it with `--team` for one team and verify only that team's record and pin are gone.

**Acceptance Scenarios**:

1. **Given** a resource name held by exactly one team, **When** the operator deletes it without `--team`, **Then** that team's resource is deleted.
2. **Given** a resource name held by several teams, **When** the operator deletes it without `--team`, **Then** the command refuses with the ambiguity error that names the teams and asks for `--team`, and nothing is released.
3. **Given** a resource name held by several teams, **When** the operator deletes it with `--team <team>`, **Then** only that team's record and pin are released.
4. **Given** `--team` names a team that holds nothing for the name, **When** the operator deletes it, **Then** the command succeeds and says there is nothing to delete for that resource in that team.
5. **Given** a name held as an application in team A and as a resource in team B, **When** the operator runs `shrine delete resource <name>` without `--team`, **Then** team B's resource is found with no ambiguity, and team A's application is untouched.

---

### User Story 5 - Every delete verb releases what the artifact held (Priority: P5)

An operator who deletes anything, an application, a resource, or a whole team, can rely on one rule: the delete releases the pins of what it deletes, and nothing else does. After any of the three deletes, the next deploy of the affected artifacts under the pinned policy pins afresh.

**Why this priority**: PRD requirement R-11, which this ticket completes. Ticket T3 made `delete application` and `delete team` release pins; this ticket adds the third verb and asserts, end to end, that all three behave the same. The ticket's main value is that assertion.

**Independent Test**: In one integration suite, for each of the three delete verbs: deploy pinned artifacts, tear down, delete, verify the pins are gone, move the registry's newest, deploy, and verify the artifacts pin afresh.

**Acceptance Scenarios**:

1. **Given** a torn-down pinned application, **When** the operator runs `shrine delete application <name>` and deploys again, **Then** the pin is released by the delete and the application pins afresh on the deploy.
2. **Given** a torn-down pinned resource, **When** the operator runs `shrine delete resource <name>` and deploys again, **Then** the pin is released by the delete and the resource pins afresh on the deploy.
3. **Given** a torn-down team holding several pinned artifacts of both kinds, **When** the operator runs `shrine delete team <name>`, **Then** every pin the team held is released, the output says how many were released, and a later deploy of the team pins every artifact afresh.
4. **Given** pinned artifacts, **When** the operator redeploys, recreates a container, or tears down, **Then** no pin is released, as ticket T3 guarantees.

---

### User Story 6 - The documentation describes delete resource (Priority: P6)

An operator reading the command reference finds a page for `delete resource`, generated from the command help, beside the pages for `delete application` and `delete team`, naming its flags and saying that a live container blocks the delete. A contributor reading the project's own reference finds `delete resource` in the CLI reference lines beside `delete application`.

**Why this priority**: PRD requirement R-30 and the repository's rule that documentation and code change together. Last because it documents the stories above.

**Independent Test**: From the command pages and the contributor reference alone, answer: how do I retire a resource, what must be true before I can, how do I preview it, and what happens to its pin.

**Acceptance Scenarios**:

1. **Given** the generated command reference, **When** an operator reads it, **Then** it has a page for `delete resource` listing `--team` and `--dry-run`, saying the resource's container must already be torn down, and saying the delete releases the image pin and drops the deployment record; the `delete` parent page lists it beside `application` and `team`.
2. **Given** the contributor reference, **When** a contributor reads the CLI reference lines, **Then** `delete resource` is described beside `delete application`, and the note that `delete team` releases every pin the team held still stands.

---

### Edge Cases

- A resource that was never deployed and has no pin: nothing is held, the command succeeds and says so; with `--team` the message names the team.
- A resource with a pin but no deployment record (the record was lost or the store was hand-edited): the pin is released and the output mentions only the pin.
- A resource with a record but no pin (deployed under a manifest-owned policy, or its pin was already released): the record is removed and the output mentions only the record.
- The container runtime is unreachable: the delete cannot confirm the container is gone and fails with the runtime's error, releasing nothing, because the runtime is authoritative and a delete must not guess.
- A resource's container exists but belongs to a different team than the one named with `--team`: the container check is per team and name, so the delete of the named team proceeds; the other team's container and state are untouched.
- The pin on record is for the other kind (an application of the same name in the same team): pins carry their kind, so `delete resource` releases only a resource pin and leaves an application pin alone, and the reverse.
- `delete resource` of a name that only exists as an application in the same team: nothing is held for the resource kind, the command says there is nothing to delete, and the application's record and pin are untouched.
- Resources hold no published host port: the output never mentions a host port, and the delete releases no port allocation.
- `--dry-run` with an ambiguous name: the ambiguity error, unchanged.
- The `delete` parent command with no subcommand, `delete application`, and `delete team`: existing output unchanged, apart from the new entry on the parent's help.
- Output of `deploy`, `get`, `describe`, `status`, `teardown`, and `bump`: unchanged by this feature.

## Requirements *(mandatory)*

### Functional Requirements

Identifiers in brackets bind each requirement to the design's T7 list and to the PRD.

- **FR-001** [T7-01, R-27]: Shrine MUST provide `shrine delete resource <name>` beside `shrine delete application <name>`, with the flags `-t`/`--team` and `--dry-run`, taking exactly one name.
- **FR-002** [T7-01, R-27]: Without `--team`, the resource MUST be found by searching every team's resource deployment records and resource pins for the name; when exactly one team holds it, that team is used; when several do, the command MUST refuse with the ambiguity error the per-artifact commands use, naming the teams and asking for `--team`; when none does, the command MUST succeed and say there is nothing to delete for that resource. Application records and pins MUST NOT count as candidates. With `--team`, the search MUST be limited to that team.
- **FR-003** [T7-01, R-27]: When the resource's container exists in the container runtime, the delete MUST refuse with a message naming the resource as `<team>/<name>`, saying it still has a container, and naming `shrine teardown <team>` as the step to take first; it MUST release nothing. The container runtime is authoritative: when it cannot be consulted, the delete MUST fail with that error and release nothing.
- **FR-004** [T7-01, R-27]: When the container is absent, the delete MUST release the resource's pin if one is held and remove the resource's deployment record if one exists, in that order, printing one line for each thing released or removed naming `<team>/<name>`, and MUST print a nothing-to-delete line naming the resource and team when neither is held. A resource MUST NOT be treated as holding a host port.
- **FR-005** [T7-01, R-27]: With `--dry-run` the delete MUST print, each line marked as a dry run, the pin (with its exact version) that would be released and the record that would be removed, or that there is nothing to delete, and MUST write nothing. The refusals of FR-002 and FR-003 apply unchanged under `--dry-run`.
- **FR-006** [T7-01, R-27]: Releasing a pin MUST honour the pin's kind: `delete resource` MUST release only a resource pin for the name, and `delete application` MUST continue to release only an application pin, so that an application and a resource of the same name in one team never release each other's pin.
- **FR-007** [T7-01]: `delete resource` and `delete application` MUST share one delete behaviour differing only in kind and in the host-port step, so that the refusal, the search, the dry run, and the output lines stay identical in form between the two verbs.
- **FR-008** [T7-02, R-11]: `shrine delete application`, `shrine delete resource`, and `shrine delete team` MUST each release the pins of what they delete, and no other operation (redeploy, container recreation, teardown) MUST release a pin. After any of the three, the next deploy of the affected artifacts under the pinned policy MUST behave as a first deploy and record a fresh pin.
- **FR-009** [T7-02, R-11]: The integration suite for delete MUST assert, for each of the three delete verbs against ticket T3's loopback registry fixture, that the delete releases the pin and that the next deploy pins afresh, and MUST cover the resource cases of stories 1 to 4: delete of a torn-down resource, refusal while the container exists, dry run writes nothing, team resolution and ambiguity. The suite is written before the implementation and runs in CI.
- **FR-010** [T7-03, R-30]: The command reference MUST gain a generated page for `delete resource`, the `delete` parent page MUST list it, and the contributor reference's CLI lines MUST describe `delete resource` beside `delete application`.
- **FR-011** [G6]: No command other than `delete resource` changes its behaviour or output, apart from the new entry on the `delete` parent's help; the existing integration suites MUST pass without edits to their assertions.

### Key Entities

- **Resource deployment record** (existing): Shrine's memory that a resource of a given name was deployed in a given team, with its kind. Found by team, name, and kind. Removed by teardown with the container, and by `delete resource` when it outlived the container (removed outside Shrine).
- **Pin** (existing, from T3): Shrine's record of a Shrine-owned version for an artifact, keyed by team and name and carrying the artifact's kind. Released by `delete resource` when its kind is resource. Never released by teardown.
- **Held state**: what a delete would release for an artifact: for a resource, its pin and its deployment record; for an application, additionally its published host port. The dry run lists it; the real delete releases it; an artifact holding nothing yields a nothing-to-delete line.
- **Refusal**: a delete that stops before releasing anything: the container still exists, the name is ambiguous across teams, or the container runtime cannot be consulted.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Retiring a torn-down resource is one command, after which neither its deployment record nor its pin is on record, and the next deploy of a resource with that name resolves the newest version again and records a new pin (journey J8).
- **SC-002**: Every delete of a resource whose container exists is refused with a message naming the teardown command, with zero records and zero pins changed.
- **SC-003**: A dry run of `delete resource` leaves the deployment records and the pin store byte for byte as they were.
- **SC-004**: All three delete verbs release the pins of what they delete, verified end to end by the same integration suite, and no non-delete operation releases a pin (PRD requirement R-11 complete).
- **SC-005**: Deleting a resource never changes the record, pin, or host port of an application of the same name, nor of any artifact in another team.
- **SC-006**: The existing integration suites pass without edits to their assertions (PRD goal G6).
- **SC-007**: From the command pages and the contributor reference alone, a reader can state how to retire a resource, that its container must be torn down first, how to preview the delete, and that the delete releases the pin.

## Assumptions

- **Scope**: this ticket delivers `shrine delete resource` with `--team` and `--dry-run`, the end-to-end assertion that all three delete verbs release pins, and the documentation of FR-010. Out of scope: anything beyond delete; in particular no change to `delete team`'s own behaviour beyond confirming it releases pins (T3 delivered that), no change to teardown, and no operator guide (T8).
- **Mirroring `delete application` exactly**: the refusal message, the search over every team, the ambiguity error, the dry-run marker, and the per-line output follow the form `delete application` prints today, with the word "resource" in place of "application" and no host-port line. The exact strings are fixed in the plan's contract; the spec requires the facts on the lines. `delete application` has no short alias today, so `delete resource` has none either; adding `app` and `res` aliases to the delete verbs is a separate decision.
- **Candidate search**: `delete application` finds candidate teams from host-port allocations and application deployment records. A resource holds no host port, so `delete resource` finds candidate teams from resource deployment records and from resource pins, so that a resource whose record was lost but whose pin remains can still be retired by name.
- **Order of release**: the pin is released before the record is removed, matching `delete application`, which releases its allocations before dropping the record. A failure midway reports the step that failed; what was already released stays released.
- **Container check**: the container's existence is checked by the same team-and-name convention the deploy uses to name a resource's container, through the container runtime, which is authoritative. A delete needs no running container, only the runtime.
- **`delete team` output**: the count of released pins printed by `delete team` is ticket T3's and is unchanged; this ticket asserts it covers pins of both kinds.
