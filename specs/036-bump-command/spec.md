# Feature Specification: `shrine bump`

**Feature Branch**: `036-bump-command`
**Created**: 2026-10-07
**Status**: Draft
**Input**: User description: "https://github.com/CarlosHPlata/shrine/issues/57"
**Epic**: Pinned Image Versions, ticket T6 (issue #57). Source documents: [prd.md](../epics/pinned-image-versions/prd.md) (journeys J4, J5, J6; goal G4; requirements R-05, R-14, R-21 to R-26, R-30; metric M4; open decision OD-5), [design.md](../epics/pinned-image-versions/design.md) (decisions TD-8 and TD-12; sections 4.2, 4.6, 4.9, 4.11; requirement list T6-01 to T6-07), [tickets.md](../epics/pinned-image-versions/tickets.md#t6-shrine-bump) section T6. Builds on ticket T3 ([spec 033](../033-pinned-image-policy/spec.md)), which introduced the `Pinned` policy, the pin record, and the immediate resolution of an image into an exact version, and on ticket T5 ([spec 035](../035-pinned-version-queries/spec.md)), which shows a pin that differs from the running image in `describe`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Upgrade one artifact to a chosen version (Priority: P1)

An operator learns that a new version of a database or an application is out. Without touching the manifest, which deliberately names no version, they tell Shrine to move that one pinned artifact to the chosen readable version. Shrine looks the version up in the registry right away, records the exact version it found as the artifact's new pin, and prints the version that was pinned before and the one pinned now. Nothing restarts. When the operator is ready, they deploy, and only that artifact is recreated on the new exact version. A typo in the version is rejected by the bump itself, with nothing recorded, so it is never discovered halfway through a deploy.

**Why this priority**: This is journey J4 and PRD goal G4 ("move deliberately"), the reason the epic has a bump command at all. Metric M4 is measured here: upgrading one artifact is two commands, and a version that does not exist is rejected with zero containers touched.

**Independent Test**: Against the loopback registry fixture of ticket T3, push two versions of an image, deploy a pinned artifact on the first, bump it to the second, verify the printed previous and new versions, verify the container is the same one as before the bump, then deploy and verify the container was recreated on the second exact version. Bump to a version that was never pushed and verify the failure and that the pin on record is unchanged.

**Acceptance Scenarios**:

1. **Given** a deployed pinned artifact whose pin is at version `1` and a registry that serves version `2` of the same repository, **When** the operator runs `shrine bump resource <name> -v 2` (or `bump application`), **Then** the command succeeds, the pin on record now holds the exact version the registry serves for `2`, and the output states the previous version (`1` with its short exact version) and the new version (`2` with its short exact version).
2. **Given** the bump in scenario 1 succeeded, **When** the operator inspects the running container, **Then** it is the same container as before the bump, still created from the previous exact version; no container was started, stopped, or recreated.
3. **Given** the bump in scenario 1 succeeded, **When** the operator runs `shrine deploy` for the team, **Then** that artifact is recreated from the new exact version and `shrine describe` shows the pin and the running image agreeing again.
4. **Given** a deployed pinned artifact and a readable version the registry does not serve, **When** the operator bumps to it, **Then** the command fails with a message naming the artifact and the reference it tried, the pin on record is byte for byte what it was, and no container is touched.
5. **Given** a deployed pinned artifact, **When** the operator bumps to an exact version (`-v sha256:<64 hex>`) that the registry serves for the manifest's repository, **Then** the pin is recorded at that exact version and the output shows it as the new version.
6. **Given** any bump, **When** the operator reads the output, **Then** it tells them a deploy is still needed to apply the change.
7. **Given** a pinned artifact, **When** the operator bumps it with `-v` set to a value that is neither a valid tag nor a `sha256:` exact version, **Then** the command refuses before contacting the registry, says what a valid value looks like, and records nothing.

---

### User Story 2 - Roll back (Priority: P2)

The upgrade went badly. The operator bumps the same artifact back, either to the readable version it had before or to the exact version Shrine printed on the previous bump, and deploys. There is no separate rollback command; going back is the same move in the other direction.

**Why this priority**: Journey J5 and PRD requirement R-26. It needs nothing beyond story 1, but the ticket's acceptance names it and an operator who cannot go back will not upgrade.

**Independent Test**: After the upgrade of story 1, bump the artifact to the exact version printed as "previous", verify the pin on record equals it, deploy, and verify the container runs the earlier exact version again.

**Acceptance Scenarios**:

1. **Given** an artifact bumped from `1` to `2`, **When** the operator runs `shrine bump resource <name> -v 1`, **Then** the pin is recorded at the exact version the registry serves for `1`, and the output shows `2` as previous and `1` as new.
2. **Given** an artifact bumped from `1` to `2` and the exact version of `1` noted from the earlier output or from `describe`, **When** the operator bumps to that exact version, **Then** the pin is recorded at it and the output shows it as the new version.
3. **Given** the rollback bump succeeded, **When** the operator deploys, **Then** the artifact is recreated on the earlier exact version.

---

### User Story 3 - Take the newest again (Priority: P3)

Months later the operator wants whatever is newest for an artifact. They bump it with no version. Shrine resolves the newest version of the manifest's image the way a first deploy would, records it, and the next deploy applies it. "Newest" is an explicit, recorded act, never a side effect of a redeploy.

**Why this priority**: Journey J6 and the second half of PRD requirement R-22. Together with stories 1 and 2 it completes goal G4.

**Independent Test**: With a pinned artifact at version `1`, push a new image under the repository's newest tag, bump without `-v`, and verify the pin now holds the exact version of the newest tag while the container is untouched.

**Acceptance Scenarios**:

1. **Given** a pinned artifact whose manifest names no version (so its reference means newest), **When** the operator runs `shrine bump app <name>` with no `-v`, **Then** the pin is recorded at the exact version the registry currently serves for the manifest's reference, and the output shows previous and new.
2. **Given** the newest version has not changed since the pin was recorded, **When** the operator bumps with no `-v`, **Then** the command succeeds and the previous and new exact versions are the same.
3. **Given** a redeploy without a bump, **When** the newest version has moved in the registry, **Then** the artifact stays on its pin, as ticket T3 guarantees; only a bump moves it.

---

### User Story 4 - Only pinned artifacts can be bumped, and any pinned artifact in the manifest directory can (Priority: P4)

An operator bumps an artifact whose version is manifest-owned, because its effective policy is `Always` or `IfNotPresent`. Shrine refuses and says that the version is manifest-owned, names the manifest, and says to edit the manifest to change it. An operator bumps a name that is in no manifest; Shrine refuses and names the directory it searched. An operator bumps a pinned artifact that has never been deployed; Shrine records the pin so that the first deploy runs the chosen version instead of the newest.

**Why this priority**: PRD requirements R-05 and R-24. The refusals keep the manifest the single owner of a version under the two existing policies (goal G6, nothing changes for anyone who does not opt in), and the undeployed case is what lets an operator choose the version of a first deploy in advance.

**Independent Test**: In one manifest directory, place a pinned artifact that has never been deployed and a manifest-owned one. Bump the manifest-owned one and verify the refusal names the manifest and the policy. Bump an unknown name and verify the refusal names the directory. Bump the undeployed pinned artifact to a chosen version, deploy, and verify the container runs that exact version.

**Acceptance Scenarios**:

1. **Given** an artifact whose effective policy is `Always` or `IfNotPresent`, whether set on the manifest, by the configuration default, or by today's derived rule, **When** the operator bumps it, **Then** the command refuses with a message that names the artifact, says its version is manifest-owned, names the effective policy, and says to edit the manifest; nothing is recorded and no registry is contacted.
2. **Given** a name that matches no application or resource manifest of the requested kind in the manifest directory, **When** the operator bumps it, **Then** the command refuses with a message naming the kind, the name, and the directory searched.
3. **Given** a pinned artifact whose manifest is in the directory but which has never been deployed and has no pin, **When** the operator bumps it to version `1`, **Then** the pin is recorded, the output says the artifact was pinned at `1` with its exact version and shows no previous version, and the next `shrine deploy` creates the container from that exact version.
4. **Given** a pinned artifact that was deployed and then torn down, so its pin remains, **When** the operator bumps it, **Then** the bump records the new pin and shows the retained pin as previous; the next deploy of the team runs the new version.
5. **Given** a name present in several teams, **When** the operator bumps it without `--team`, **Then** the command refuses with the same ambiguity error the other per-artifact commands use; **When** they add `--team <team>`, **Then** that team's artifact is bumped.
6. **Given** a name present in exactly one team, **When** the operator bumps it without `--team`, **Then** the artifact is found by the usual automatic search.
7. **Given** a manifest directory elsewhere than the configured one, **When** the operator passes `--path <dir>`, **Then** the artifact is looked up in that directory, as `deploy` does.

---

### User Story 5 - Preview a bump without recording it (Priority: P5)

An operator wants to see what a bump would do before doing it. With `--dry-run` the command prints the reference it would resolve and the artifact it would pin, and writes nothing: no pin, no registry pull, no container change.

**Why this priority**: PRD requirement R-25. Small, and the other verbs offer the same flag, so its absence would be a surprise.

**Independent Test**: Bump a pinned artifact with `--dry-run` and a version, verify the output names the reference and the artifact, and verify the pin on record is unchanged and no image was pulled.

**Acceptance Scenarios**:

1. **Given** a pinned artifact, **When** the operator runs `shrine bump resource <name> -v 2 --dry-run`, **Then** the output is marked as a dry run, names the reference that would be resolved (`<repository>:2`) and the artifact that would be pinned, and nothing is recorded.
2. **Given** a pinned artifact, **When** the operator runs the dry run without `-v`, **Then** the output names the manifest's own reference as what would be resolved.
3. **Given** a manifest-owned or unknown artifact, **When** the operator runs the dry run, **Then** the same refusal as without `--dry-run` is printed, because the refusals need no write to decide.

---

### User Story 6 - A vanished exact version points at bump (Priority: P6)

A registry stops serving the exact version an artifact is pinned to. The deploy fails before touching anything, as ticket T3 made it, and its message now tells the operator to run `shrine bump` for that artifact to choose another version, instead of ticket T3's interim advice to switch the policy.

**Why this priority**: PRD requirement R-14's last sentence, which ticket T3 could not satisfy because bump did not exist. One message, no new behaviour.

**Independent Test**: Deploy a pinned artifact, delete its exact version from the loopback registry and the local image cache, deploy again, and verify the failure message names the artifact, the exact version, and the bump command for that kind and name.

**Acceptance Scenarios**:

1. **Given** a pinned artifact whose exact version the registry no longer serves and whose image is absent locally, **When** the operator deploys, **Then** the deploy stops with zero changes and the message says the exact version is no longer served and names `shrine bump <kind> <name>` as the way to choose another.
2. **Given** that message, **When** the operator runs the bump it names, to a version the registry serves, and deploys again, **Then** the deploy succeeds on the new exact version.

---

### User Story 7 - The documentation describes bump (Priority: P7)

An operator reading the command reference finds a page for `bump application` and one for `bump resource`, generated from the command help, naming the flags and what the command does and does not do. A contributor reading the project's own reference finds bump in the CLI reference. The manifest reference's image pull policy subsection names bump as the way to move a pinned version.

**Why this priority**: PRD requirement R-30 and the repository's rule that documentation and code change together. Last because it documents the six stories above. The full operator guide is ticket T8's.

**Independent Test**: From the command pages and the manifest reference alone, answer: how do I move a pinned resource to version 17, how do I go back, how do I take the newest, and what happens to the running container when I do.

**Acceptance Scenarios**:

1. **Given** the generated command reference, **When** an operator reads it, **Then** it has pages for `bump application` and `bump resource` listing `-v`, `--team`, `--dry-run`, and `--path`, and saying that bump records and deploy applies.
2. **Given** the manifest reference's image pull policy subsection, **When** an operator reads it, **Then** it names bump as the way to change a pinned version and says a manifest-owned version is changed by editing the manifest.
3. **Given** the contributor reference, **When** a contributor reads the CLI reference lines, **Then** bump is listed with the other verbs.

---

### Edge Cases

- `-v latest`: a valid tag; the target is the repository at `latest`, which resolves to the newest, the same as no `-v`.
- `-v` equal to the version already pinned: the registry is consulted, and the pin is rewritten with whatever exact version that readable version serves now; previous and new may be equal or differ, and both are printed.
- `-v` given as an exact version the registry does not serve for this repository: fails like any unknown version; nothing recorded.
- `-v` given as a full reference (`postgres:17`, `repo@sha256:…`) rather than a bare version: refused by validation, because the repository always comes from the manifest and a bump can never point an artifact at a different image (R-22).
- A Resource whose manifest sets an image override: the repository is the one the override names, the same reference a deploy of that resource would resolve.
- The manifest names the image through a registry alias: the alias is expanded as deploy expands it, and the pin records the expanded reference; the output may show the expanded form.
- The pin on record is for a different repository than the manifest now names (the manifest was edited to another image and not yet redeployed): the bump replaces the pin with one for the manifest's current repository and shows the old pin as previous.
- The pin file is missing or holds no entry for the artifact although it was deployed: the bump behaves as for an undeployed artifact and prints the new pin with no previous version.
- The manifest directory does not exist or has no manifests: the refusal names the directory.
- The manifest directory contains a manifest that fails validation: the bump reports the validation errors as deploy would and records nothing, because it loads the set the way deploy does.
- The registry is unreachable or refuses credentials: the bump fails with the registry's error, names the reference, and records nothing.
- The container runtime is unreachable: the bump fails, because resolution pulls through the runtime; nothing is recorded. The command needs no running container, only the runtime.
- Both `--team` and an unambiguous name: `--team` is honoured; a `--team` that does not own the name is reported as not found in that team.
- Ambiguous name and `--dry-run`: the ambiguity error, unchanged.
- An artifact under `Pinned` whose manifest names a fixed version: that manifest fails validation under ticket T3's rules before bump runs; the bump reports the validation error.
- Output of `deploy`, `get`, `describe`, `status`, and `delete`: unchanged by this feature except for the reworded vanished-version message in `deploy`.

## Requirements *(mandatory)*

### Functional Requirements

Identifiers in brackets bind each requirement to the design's T6 list and to the PRD.

- **FR-001** [T6-01, R-21]: Shrine MUST provide `shrine bump application <name>` and `shrine bump resource <name>`, with the `app` and `res` aliases the other verbs use, the flags `-v`/`--version`, `-t`/`--team`, `--dry-run`, and `-p`/`--path`, and `--path` MUST name the manifest directory the way `deploy` does.
- **FR-002** [T6-02, R-21]: Without `--team`, the artifact MUST be found by the automatic search the other per-artifact commands use, with the same ambiguity error when the name is in several teams; with `--team`, the search MUST be limited to that team.
- **FR-003** [T6-02, R-05]: When the effective policy of the named artifact is not `Pinned`, the bump MUST refuse with a message that names the artifact, says its version is manifest-owned, names the effective policy, and says to edit the manifest; it MUST record nothing and contact no registry.
- **FR-004** [T6-02, R-24]: When no manifest of the requested kind and name is found, the bump MUST refuse with a message naming the kind, the name, and the directory searched.
- **FR-005** [T6-02, R-24]: The bump MUST work for any pinned artifact whose manifest is in the manifest directory, whether or not it is deployed and whether or not it has a pin, so that a pin recorded before the first deploy is the version that deploy runs.
- **FR-006** [T6-03, R-22]: The target reference MUST be built from the manifest's image with the repository kept and only the version replaced: a readable version becomes the repository at that tag; an exact version (`sha256:` and 64 hexadecimal characters) becomes the repository at that exact version; no `-v` means the manifest's own reference, which under `Pinned` means newest. A bump MUST NOT be able to point an artifact at a different repository.
- **FR-007** [T6-03, R-22]: `-v` MUST be validated before any registry access as either a tag (a letter, digit, or underscore followed by up to 127 letters, digits, underscores, dots, or dashes) or an exact version; any other value MUST be refused with a message that says what is accepted.
- **FR-008** [T6-04, R-23, TD-8, TD-12]: The bump MUST resolve the target immediately through the same resolution that a deploy uses, verifying the target exists in the registry; on success it MUST record the new pin (the exact version, the reference it was resolved from, and the date) through the same single writer that records pins on deploy. When resolution fails, the bump MUST fail with a message naming the artifact and the reference, and the pin on record MUST be unchanged.
- **FR-009** [T6-04, R-23]: The bump MUST NOT start, stop, create, or recreate any container; applying a pin is the next deploy's job.
- **FR-010** [T6-04, R-23]: On success the output MUST state the previous version and the new version, each as the readable form of its pin (readable version, `@`, short exact version, as ticket T5 shows them), and MUST say that a deploy applies the change. When there was no previous pin, the output MUST say the artifact was pinned at the new version and show no previous version.
- **FR-011** [T6-05, R-25]: With `--dry-run` the bump MUST print, marked as a dry run, the reference it would resolve and the artifact it would pin, and MUST write nothing, pull nothing, and touch no container. The refusals of FR-003, FR-004, and FR-007 apply unchanged under `--dry-run`.
- **FR-012** [T6-06, R-14]: When a deploy finds that a pinned exact version is no longer served by the registry, the failure message MUST name the artifact and the exact version and MUST point at `shrine bump <kind> <name>` as the way to choose another version, replacing ticket T3's interim advice; the rest of ticket T3's behaviour (fail before any change, cause shown under the resolving line) is unchanged.
- **FR-013** [T6-07, R-30]: The command reference MUST gain generated pages for `bump application` and `bump resource`; the contributor reference's CLI lines MUST list bump; the manifest reference's image pull policy subsection MUST name bump as the way to move a pinned version.
- **FR-014** [G6]: No command other than `bump` and the `deploy` message of FR-012 changes its behaviour or output; the existing integration suites MUST pass without edits to their assertions, apart from the reworded message where a suite asserts it.
- **FR-015**: A new integration suite for bump MUST cover, against ticket T3's loopback registry fixture: bump to an existing version records it, prints previous and new, leaves the container untouched, and the next deploy recreates it on the new exact version; bump to a missing version fails and records nothing; bump without `-v` records the current newest; bump of a manifest-owned artifact is refused with the named message; bump before the first deploy followed by a deploy runs the chosen version; dry run prints and writes nothing. The suite is written before the implementation and runs in CI.

### Key Entities

- **Pinned artifact**: an application or resource whose effective policy is `Pinned`, found by kind, name, and optionally team among the manifests of the manifest directory. The only kind of artifact a bump accepts. It need not be deployed.
- **Target reference**: the image reference a bump resolves: the manifest's repository with the requested version in place of the manifest's, or the manifest's own reference when no version is given. Never a different repository.
- **Pin** (existing, from T3): Shrine's record of a Shrine-owned version: the reference it was resolved from, the pullable exact version, and the date. A bump replaces it; the previous one is read first so both can be printed. Written only by the one resolution path deploy also uses.
- **Previous and new version**: the pin before the bump and the pin after, printed in the readable form ticket T5 established. "Previous" is absent when there was no pin.
- **Refusal**: a bump that stops before resolution: manifest-owned artifact, unknown artifact, ambiguous name, or an invalid version value. Records nothing and contacts no registry.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Upgrading one pinned artifact to a chosen version is exactly two commands, a bump and a deploy, with no manifest edit; after the bump and before the deploy the running container is unchanged, and after the deploy it runs the chosen exact version (PRD metric M4).
- **SC-002**: A bump to a version the registry does not serve, or to an invalid version value, fails with zero containers touched and the pin on record byte for byte unchanged (PRD metric M4).
- **SC-003**: Rolling back an upgrade uses the bump command and no other, to either the readable or the exact previous version, and the next deploy runs the earlier exact version again.
- **SC-004**: A bump with no version records the exact version the registry currently serves as newest, and a redeploy without a bump never moves a pinned artifact.
- **SC-005**: Every bump of a manifest-owned artifact is refused with a message naming the manifest and the policy; every bump of an unknown name is refused with a message naming the directory; neither records anything.
- **SC-006**: A pin recorded before an artifact's first deploy is the version that first deploy runs.
- **SC-007**: A dry run leaves the pin store, the local image cache, and every container exactly as they were.
- **SC-008**: An operator whose deploy fails because a pinned exact version vanished can read the exact bump command to run from the failure message.
- **SC-009**: The existing integration suites pass without edits to their assertions, apart from the reworded vanished-version message (PRD goal G6).
- **SC-010**: From the command pages and the manifest reference alone, a reader can state how to move a pinned artifact to a version, how to go back, how to take the newest, and that the container changes only on the next deploy.

## Assumptions

- **Scope**: this ticket delivers the bump command with its two kinds, aliases, and four flags; the refusal rules; immediate resolution and pin recording; the previous-and-new output; dry run; the reworded vanished-version message in deploy; and the documentation of FR-013. Out of scope: applying a bump (deploy does that, PRD non-goal), a team-wide or all-artifacts bump (open decision OD-5, proposed out of scope), version increments or ranges (PRD non-goal: bump means set), `shrine delete resource` and pin release (T7), and the operator guide (T8).
- **Readable form in the output**: previous and new are shown as ticket T5 shows a pin in a table, `<readable>@<twelve hex>`, with the short exact version alone when the pin was resolved from an exact version. The exact output strings follow design section 4.6 and are fixed in the plan's contract; the spec requires the facts on the lines.
- **Dry run does not consult the registry**: per design section 4.6, a dry run prints what would be resolved and calls nothing, so it cannot tell whether the version exists. The refusals that need no resolution (manifest-owned, unknown, ambiguous, invalid value) still apply.
- **Resolution pulls the image**: verifying that a version exists is done the way deploy does it, by pulling the target through the container runtime with the registry credentials of its host. The image may therefore land in the local image cache. That is not "touching a container"; no container is created, changed, or removed.
- **The manifest set is loaded the way deploy does**: the whole manifest directory is loaded and validated, the configuration default policy is applied, and the artifact is found by kind, name, and team in that set (TD-12). A validation error anywhere in the directory stops the bump as it stops a deploy. The manifest directory comes from `--path`, otherwise from the configured specs directory, as deploy.
- **Team lookup**: "the usual automatic search and ambiguity error" is the behaviour `describe` and `delete application` already have: search every team when `--team` is absent, fail when the name is in more than one.
- **Previous version**: read from the pin store before resolution, whatever it holds, including a pin whose repository no longer matches the manifest. When there is no entry, the output shows the "pinned at" form with no previous version.
- **Vanished-version message**: the second clause of ticket T3's message ("deploy the kind under Always or IfNotPresent to release the pin, then return to Pinned") is replaced by the bump wording; the first clause and the pull cause stay. Any integration assertion on the old wording is updated to the new one, which the ticket allows.
- **Documentation locations**: the command pages are generated from the command help into the existing command reference; the contributor reference's CLI lines gain bump next to the other verbs; the manifest reference's existing image pull policy subsection gains a sentence pointing at bump.
- **Integration scenarios**: per the project's rules, authored first, compile-checked locally, and run only in CI, in a new bump suite against ticket T3's loopback registry fixture, which can push several versions of one image and remove one.
- **Dependencies**: ticket T3 (spec 033) provides the `Pinned` policy, the pin record, the resolution path, and the registry fixture. Ticket T5 (spec 035) is not required but makes the result of a bump visible in `describe`. Shares with its wave only the deploy resolution path and the contributor reference's CLI lines.
