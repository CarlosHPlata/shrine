# Feature Specification: Operator guide, managing image versions

**Feature Branch**: `038-image-versions-guide`
**Created**: 2026-10-07
**Status**: Draft
**Input**: User description: "Operator guide 'Managing image versions' (epic Pinned Image Versions, ticket T8, issue #59). Documentation only. A new guide that walks PRD journeys J1 to J8 with output captured from the real binary, linked from the guides index, the manifest reference, and troubleshooting where a pin cannot be honoured; and a vocabulary-coherence pass over the pages tickets T3 to T7 wrote."
**Epic**: Pinned Image Versions, ticket T8 (issue #59). Source documents: [prd.md](../epics/pinned-image-versions/prd.md) (journeys J1 to J8; requirements R-28 to R-31; vocabulary in section 5; metric M7), [design.md](../epics/pinned-image-versions/design.md) (sections 4.9 and 4.11; requirement list T8-01 and T8-02), [tickets.md](../epics/pinned-image-versions/tickets.md#t8-operator-guide-managing-image-versions) section T8. Documents the behaviour shipped by tickets T1 to T7 ([specs 031](../031-deployed-version-columns/spec.md) to [037](../037-delete-resource/spec.md)); changes none of it.

## Clarifications

### Session 2026-10-07

- Q: Where is the guide's output produced: real deploys on this host's Docker against a throwaway local registry, a capture script run in CI, or no container runtime at all? → A: No container runtime. Output the binary prints without a daemon is captured from real runs; output that needs a daemon or a registry is assembled from the code's format strings and checked against the integration suites' assertions, with placeholder exact versions (FR-003, FR-011).

## User Scenarios & Testing *(mandatory)*

The reader of every story is an operator who knows Docker and Shrine's manifests but has never used the `Pinned` policy, `shrine bump`, or the `imagePullPolicy` configuration default. Each story is one part of the guide, or one supporting page, and is tested by giving that reader the documentation alone and checking that they can answer the story's question and run its commands.

### User Story 1 - Pin a service and keep its version (Priority: P1)

An operator wants a new application to start on the newest version of its image and then stay on that exact version. The guide shows them how to write the manifest (the repository only, `imagePullPolicy: Pinned`), what the first deploy prints when it pins, what a later redeploy prints when it reuses the pin, and that the version stays put after the upstream `latest` moves, after the container is recreated, after a teardown and redeploy, and after the local image cache is wiped (journeys J1 and J2).

**Why this priority**: This is the feature's reason to exist (PRD goal G1) and the first two of the six questions metric M7 asks: how to pin, and what happens on prune. Without this part the rest of the guide has nothing to build on.

**Independent Test**: Give the reader only this part of the guide and ask: how do I pin an application, what will the first deploy print, what happens to the version if I prune all images and deploy again. They answer correctly, and running the part's commands in order on a clean state reproduces the printed lines, with only the exact versions, dates, container ids, and registry address differing.

**Acceptance Scenarios**:

1. **Given** the guide, **When** the reader follows the first journey, **Then** they write a manifest that names only the repository and the `Pinned` policy, deploy it, and see a line saying the artifact was pinned at a readable version and a short exact version.
2. **Given** the first deploy, **When** the reader redeploys after the upstream newest version has moved, **Then** the guide shows the line saying the existing pin was reused, with the date it was pinned, and the same exact version as before.
3. **Given** a pinned artifact, **When** the reader wipes the local image cache, or tears down and redeploys, **Then** the guide shows that the deploy fetches the same exact version again, and explains that this holds for as long as the registry serves it.
4. **Given** the guide's statement of the no-fixed-version rule, **When** the reader writes `imagePullPolicy: Pinned` with a fixed tag, **Then** the guide has already shown the validation error they get and the two ways out.

---

### User Story 2 - See which version is deployed (Priority: P2)

An operator wants to know which version each application and resource of a team runs, without opening the Docker CLI. The guide shows the version column of `shrine get deployed` (and the per-kind `get` tables, with and without `--team`), the `Pinned:` and `Running image:` lines of `shrine describe`, and the IMAGE column of `shrine status`, and explains how to read each: the readable version plus a short exact version for a pinned artifact, the manifest's reference for a manifest-owned one, and a `Pinned:` line that differs from `Running image:` as a recorded change waiting for the next deploy (journey J3).

**Why this priority**: Question three of M7 (how to see versions) and PRD goal G3. The later stories use these commands to show the effect of every move, so this part comes before them.

**Independent Test**: Give the reader only this part and a printed table and describe output; they say which artifacts are pinned, at which readable and exact version, since when, and whether a newer pin is waiting to be deployed.

**Acceptance Scenarios**:

1. **Given** a team with a pinned and a manifest-owned artifact, **When** the reader runs the `get` command the guide shows, **Then** the guide's output and its explanation tell them which row is pinned and what the two halves of the pinned version mean.
2. **Given** a pinned artifact, **When** the reader runs `describe`, **Then** the guide explains the `Pinned:` line (exact version, readable form, date) and the `Running image:` line, and what it means when they differ.
3. **Given** the guide, **When** the reader asks whether these commands need a running container runtime, **Then** the guide answers: `get` does not, `describe` still succeeds and marks the running image unavailable.

---

### User Story 3 - Upgrade, roll back, and take the newest on purpose (Priority: P3)

An operator wants to move one pinned artifact to a chosen version, later move it back, and months later take whatever is newest. The guide shows `shrine bump` with `-v` set to a readable version, the output that names the previous and the new version, the deploy that applies it and recreates only that artifact, a rollback as a bump to the earlier readable or exact version, and a bump without `-v` that takes the newest. It shows that a version the registry does not serve is rejected by the bump before anything changes, that `--dry-run` writes nothing, that a manifest-owned artifact is refused, and that an artifact can be bumped before its first deploy (journeys J4, J5, and J6).

**Why this priority**: Questions four and five of M7 (how to upgrade, how to roll back) and PRD goal G4. Builds on stories 1 and 2.

**Independent Test**: Give the reader only this part; they upgrade a pinned artifact to a named version in two commands, roll it back to the version it had before, and take the newest again, and they can say when the container changes (only on the next deploy).

**Acceptance Scenarios**:

1. **Given** a pinned artifact, **When** the reader bumps it with `-v <version>`, **Then** the guide shows the output naming the previous and the new version, and states that no container was touched.
2. **Given** a recorded bump, **When** the reader deploys, **Then** the guide shows that only that artifact's container is recreated, and that `describe` now shows `Pinned:` and `Running image:` agreeing again.
3. **Given** an upgrade the reader wants to undo, **When** they follow the rollback part, **Then** the guide shows the same command with the earlier readable version, and with the exact version, and tells them where to find the earlier exact version (the bump output's previous version, or `describe` before the bump).
4. **Given** a version that does not exist, **When** the reader bumps to it, **Then** the guide shows the failure and states that the pin on record is unchanged.
5. **Given** an artifact under `Always` or `IfNotPresent`, **When** the reader bumps it, **Then** the guide shows the refusal and explains that its version is changed by editing the manifest.

---

### User Story 4 - Make pinning the house rule (Priority: P4)

An operator wants every manifest that names no policy to be pinned. The guide shows the one line of `config.yml` that sets the default, what happens on the next deploy to manifests that name no policy and no fixed version (they pin), what happens to manifests that still name a fixed version (the deploy is refused before any change, every such manifest is named, and the message gives the two ways out), that a manifest's own policy field still wins, that `shrine generate` follows the default, and what changing the default back does (journey J7).

**Why this priority**: Question six of M7 (what the configuration default changes) and PRD goal G2. It changes the behaviour of many manifests at once, so the guide must state its consequences plainly, but it is an opt-in step after the per-manifest workflow is understood.

**Independent Test**: Give the reader only this part; they set the default, predict which of a given set of manifests will pin, which will be refused, and which keep their own policy, and fix a refused manifest both ways.

**Acceptance Scenarios**:

1. **Given** a directory mixing manifests with no policy, with a fixed tag, and with their own policy field, **When** the reader sets the default to `Pinned` and deploys, **Then** the guide shows the refusal that names each fixed-version manifest, the setting, and the two ways out, and states that nothing was deployed.
2. **Given** the refusal, **When** the reader applies either way out, **Then** the guide shows the deploy that follows.
3. **Given** the default, **When** the reader generates a new manifest, **Then** the guide shows that it names only the repository and deploys as is.
4. **Given** the default is later removed or changed, **When** the reader deploys, **Then** the guide states that the artifacts it had pinned are released on that deploy and become manifest-owned.

---

### User Story 5 - Retire an artifact (Priority: P5)

An operator tears a team down, changes their mind, and deploys it again, and later deletes one artifact for good. The guide shows that teardown keeps pins, so the redeploy comes back on the same versions, and that `shrine delete application`, `shrine delete resource`, and `shrine delete team` release pins, so a future artifact of the same name starts fresh. It states that a delete is refused while the container exists, and that deploying the artifact under a manifest-owned policy also releases its pin (journey J8).

**Why this priority**: Completes the lifecycle (PRD R-11, R-27) and answers the question operators ask after the first teardown: is my version still there. Needed once, at the end of an artifact's life.

**Independent Test**: Give the reader only this part; they list every action that releases a pin and every action that does not, and retire a resource in the right order.

**Acceptance Scenarios**:

1. **Given** a pinned team, **When** the reader tears it down and deploys it again, **Then** the guide shows the reused-pin lines with the original dates.
2. **Given** a torn-down pinned resource, **When** the reader deletes it, **Then** the guide shows the line saying its pin was released, and states that the next deploy of a resource with that name pins afresh.
3. **Given** a running artifact, **When** the reader tries to delete it, **Then** the guide shows the refusal that names the teardown command.

---

### User Story 6 - Recover when a pin cannot be honoured (Priority: P6)

An operator's deploy stops because the registry no longer serves a pinned exact version, for example after the registry garbage-collected an untagged image. The troubleshooting page has an entry for the message they see, explains that nothing was changed, that the cause is the registry not serving that exact version (or not being reachable with the configured credentials, which the appended cause tells apart), and that the way out is the `shrine bump` command the message names. The entry links to the guide.

**Why this priority**: PRD risk "registry retention" and requirement R-14's message. It is a failure path, rarer than the journeys, but it is the one place an operator arrives at the guide from an error.

**Independent Test**: Give the reader the error message alone; starting from the troubleshooting page they find the entry, learn whether anything changed, and know the command to run.

**Acceptance Scenarios**:

1. **Given** the deploy error for an exact version the registry no longer serves, **When** the reader looks it up in troubleshooting, **Then** an entry headed by that symptom explains the cause, states that no container or network was touched, gives the bump command, and links to the guide.
2. **Given** an error whose appended cause is an authentication or connection failure rather than a missing image, **When** the reader reads the same entry, **Then** it tells them to fix registry access before bumping.

---

### User Story 7 - One vocabulary across the documentation (Priority: P7)

An operator who reads the guide, the manifest reference, the configuration section, and the command pages meets the same words for the same things: repository, version, exact version, policy, manifest-owned, Shrine-owned, pin, bump. A page written by one ticket does not call the exact version a "digest" where another calls it an "exact version" without saying they are the same, and no page uses "version" for the digest alone.

**Why this priority**: PRD requirements R-28 to R-30 asked for coherence once all tickets had landed, and design requirement T8-02. It polishes pages that already work, so it comes last.

**Independent Test**: For each PRD vocabulary term, search the audited pages and verify each use carries the PRD meaning, and that the first use of "digest" on each page ties it to "exact version".

**Acceptance Scenarios**:

1. **Given** the audited pages, **When** a reviewer checks every use of the eight terms, **Then** each carries the meaning PRD section 5 gives it.
2. **Given** a command page generated from the command's own help, **When** its wording needs a correction, **Then** the correction is made in the help text and the page is regenerated, so the page and `shrine <command> --help` agree.

---

### Edge Cases

- Exact versions, dates, container ids, and the registry address in the guide's output are placeholders. The guide says once, near the top, that the reader's values will differ, and never asks the reader to type a value copied from the guide's output.
- Some steps need the upstream `latest` to move between two deploys, which a public registry does not let the reader do. The guide shows those steps against a registry the reader controls and says how to reproduce them; a reader without one can still follow every other step.
- A refusal printed by the command is followed by the command's usage text. Captured output may be trimmed to the lines that matter; a trim is marked, and no line that remains is altered.
- A reader whose registry garbage-collects untagged images: the guide states, where it explains a pin, that a pin lasts only while the registry serves the exact version, and links the troubleshooting entry.
- A reader who sets the configuration default while existing manifests name fixed tags: covered by story 4; the guide does not suggest the default is harmless.
- A pinned artifact moved to a different repository in its manifest: the next deploy pins afresh. The guide mentions it in one line, because the reference already documents it.
- Behaviour the PRD lists as non-goals (version ranges, team-wide bump, the gateway's own image, local-only images): the guide does not suggest any of them works, and says in one short passage what the feature does not do.

## Requirements *(mandatory)*

### Functional Requirements

**The guide**

- **FR-001**: The documentation site MUST gain a guide titled "Managing image versions" in the guides section, listed in the guides index with a one-line description (design T8-01).
- **FR-002**: The guide MUST walk journeys J1 to J8 of the PRD, each in its own section in that order, each with the commands to run and the output they print.
- **FR-003**: Every block of command output in the guide MUST match what the Shrine binary built from the main branch, with tickets T1 to T7 merged, prints for that command, line for line. Output the binary produces without a container runtime (validation errors, the configuration-default refusal, dry runs, `generate`, and refusals that fail before any registry or runtime call) MUST be captured from a real run. Output that needs a container runtime or a registry MUST be assembled from the format strings in the code and checked against the integration suites' assertions, with only the variable parts (exact versions, dates, container ids, registry address) filled by placeholders. No line may be invented or reworded. Output MAY be trimmed to the relevant lines, with each trim marked.
- **FR-004**: Before the journeys, the guide MUST explain the three policy values and who owns the version under each (manifest-owned, Shrine-owned), the order that decides an artifact's effective policy (manifest field, then configuration default, then the rule derived from the tag), and what a pin records (exact version, the readable version it was resolved from, the date). It MUST link to the manifest reference for the field-level rules rather than repeat them.
- **FR-005**: The guide MUST answer each of the six questions of metric M7 in a passage a reader can find from the page's headings: how to pin; what happens on prune; how to see versions; how to upgrade; how to roll back; what the configuration default changes.
- **FR-006**: The guide MUST state the conditions under which a pin is honoured, namely for as long as the registry serves the exact version, and MUST say that registries which delete untagged images can break long-lived pins.
- **FR-007**: The guide MUST show, for `shrine bump`: a readable version, an exact version, and no version; the output naming the previous and new version; that no container changes until the next deploy; `--dry-run`; the rejection of a version the registry does not serve; the refusal of a manifest-owned artifact; and a bump before an artifact's first deploy.
- **FR-008**: The guide MUST list every action that releases a pin (the three delete commands and a deploy under a manifest-owned policy) and state that redeploy, container recreation, and teardown never do.
- **FR-009**: The guide MUST show the configuration default's effect on the three kinds of manifest (no policy and no fixed version, a fixed version and no policy, their own policy), the refusal message with its two ways out, `shrine generate` under the default, and the consequence of removing or changing the default later.
- **FR-010**: The guide MUST state what the feature does not do, matching the PRD's non-goals that an operator could expect: no version ranges, no automatic bump, no team-wide bump, the gateway's own image is not covered, and images must come from a registry.
- **FR-011**: The guide MUST say once that exact versions, dates, ids, and addresses in its output are illustrative and will differ on the reader's host, and MUST describe how to reproduce the steps that need a registry the reader can push to. Placeholder exact versions MUST be the same across the guide and the manifest reference, so one artifact's version reads the same everywhere it appears.

**Links**

- **FR-012**: The manifest reference's image pull policy section MUST link to the guide.
- **FR-013**: The troubleshooting page MUST gain an entry for a deploy that stops because a pinned exact version is no longer served, covering the cause, that nothing was changed, the bump command, the case where the appended cause is a registry access failure, and a link to the guide.
- **FR-014**: The guide MUST link to the command pages of `bump`, `delete resource`, `describe`, `get`, and `status` and to the README configuration section or the page where the configuration file is introduced.

**Vocabulary pass**

- **FR-015**: The manifest reference, the README configuration section, the guide, the troubleshooting entry, and the command pages for `bump`, `delete`, `describe`, `get`, `status`, and `generate` MUST use the PRD section 5 terms with their PRD meaning (design T8-02). Where a page says "digest", its first use MUST tie it to "exact version".
- **FR-016**: A correction to a command page that is generated from command help MUST be made in the help text and the page regenerated; the generated pages MUST NOT be edited by hand.
- **FR-017**: The vocabulary pass MUST NOT change product behaviour. Help-text wording is the only change outside documentation files that the pass may make.

**Build and record**

- **FR-018**: The documentation build and its link and shape checks MUST pass with the new and changed pages.
- **FR-019**: Any place where the guide finds the shipped behaviour differs from the design MUST be recorded in the design document and named in the pull request, as the delivery plan requires; the guide documents what the product does.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A reader new to the feature answers all six M7 questions correctly from the documentation alone, and for each question can point to the passage that answers it (PRD M7).
- **SC-002**: Every output line in the guide either was captured from a real run of the binary or traces to a format string in the code and an integration-suite assertion; running the guide's commands in order on a clean state would reproduce every line, with only exact versions, dates, container ids, and registry addresses differing.
- **SC-003**: Each of the eight journeys has its own section with at least one command and its captured output.
- **SC-004**: The guide is reachable in one click from the guides index, from the manifest reference's image pull policy section, and from the troubleshooting entry.
- **SC-005**: In the audited pages, every use of the eight PRD vocabulary terms carries its PRD meaning, and no page uses "version" alone to mean the exact version.
- **SC-006**: The documentation build, link check, and page-shape checks report zero errors.
- **SC-007**: The unit and integration suites pass without edits to their assertions; the only change outside documentation, if any, is command help wording.

## Assumptions

- The guide documents the behaviour on main after ticket T7 merged (pull request #66). Where the documentation and the product disagree, the product wins and the documentation is corrected.
- No container runtime is used to produce the guide (see Clarifications). This departs from the ticket's "every example is captured from the real binary"; the departure is recorded in the design document's T8 list (FR-019).
- The examples use small public images, so a reader can reproduce them without building anything, and a registry the reader controls for the steps where the newest version must move.
- The site's existing guide conventions apply: front matter with title, description, and weight; the checks the documentation build runs on every page; links written the way existing guides write them.
- There is no configuration reference page on the site today (design 4.11). The README configuration section, written by ticket T4, stays the place the setting is defined; the guide links to it and does not create a new configuration page.
- The ticket names no integration scenario; the documentation build and its checks are this ticket's gate. Existing integration suites are untouched.
- Findings outside documentation that the earlier review surfaced (for example, `delete` treating an unreachable container runtime as "no container") are not fixed here. If the guide would have to describe such behaviour, it describes only the intended path and the finding is raised separately.
