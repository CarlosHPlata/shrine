# Feature Specification: Reconcile Specs, Tracking Metadata, and Docs with Shipped Behaviour

**Feature Branch**: `030-reconcile-spec-docs`
**Created**: 2026-10-05
**Status**: Draft
**Input**: GitHub issue [#39](https://github.com/CarlosHPlata/shrine/issues/39) — "docs: reconcile preserve-policy across specs, descope unshipped FRs, refresh stale progress/status metadata"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A reader of the specs gets one answer about what happens to a generated gateway file after its first deploy (Priority: P1)

Spec 009 made per-application routing files write-once: Shrine writes the file when it is absent and never touches it again. That decision silently contradicts promises made by earlier and later specs that were never revisited. Spec 006 still says removing an alias from the manifest and redeploying removes its route. Spec 008 still says flipping `stripPrefix` and redeploying rewrites the file. Spec 011 still says removing `tlsPort` regenerates the gateway's static configuration. Spec 012 still says removing `tls: true` reverts the route within one deploy. None of these happen on a host where the file already exists. A maintainer, contributor, or AI assistant reading any one of those specs in isolation comes away with a wrong model of the product, and will write tests, docs, or fixes against behaviour that does not exist. After this change, the preserve policy is stated once, in spec 009, and every spec that touches it points there and no longer promises otherwise.

**Why this priority**: This is the contradiction the issue leads with and the one with the widest blast radius — four specs are wrong about the same rule. Spec 029 already had to work around it (its revert scenario deletes the generated file by hand before redeploying). One canonical statement with cross-references is also what prevents the next spec from drifting the same way.

**Independent Test**: Read specs 006, 008, 009, 011, and 012 and, for each, answer "the operator changes this routing field in the manifest and redeploys onto a host that already has the generated file — what changes on disk?" All five give the same answer, and four of them give it by pointing at the same statement in 009.

**Acceptance Scenarios**:

1. **Given** spec 009, **When** a reader looks for the lifecycle of generated gateway files, **Then** they find a single labelled statement of the policy: written only when absent, never modified or deleted once present, and how an operator makes a manifest change take effect.
2. **Given** spec 006's requirement and success criterion on alias removal, **When** a reader reads them, **Then** each says the route is removed only when the per-application file is regenerated, and references the canonical statement in 009.
3. **Given** spec 008's requirement on `stripPrefix` changes, spec 011's edge case on `tlsPort` removal, and spec 012's success criterion on TLS revert, **When** a reader reads them, **Then** each is qualified the same way and references the same statement.
4. **Given** any of the amended requirements, **When** a reader compares it against the task lists and tests that cite it, **Then** its identifier is unchanged and an amendment note shows what changed, when, and why.

---

### User Story 2 - An operator learns from the docs why a routing change in the manifest did not take effect, and how to apply it (Priority: P1)

The contradiction in the specs is mirrored by a gap in the published docs. The routing and aliases guide explains how to declare aliases, `stripPrefix`, and `tls`, and says nothing about what happens when an operator changes any of them after the first deploy: nothing changes, and the only hint is an info-level "preserved" line in the deploy output. The TLS guide goes further and tells the operator that adding `tlsPort` and redeploying adds the secure entrypoint "when the static config is Shrine-generated (not operator-preserved)" — a distinction the product does not make; any existing static configuration file is preserved, whoever wrote it. An operator following either guide on a host that has already deployed will edit the manifest, redeploy, see success, and find the gateway unchanged. After this change, the docs site states the limitation plainly, the guides link to it at the point where it bites, and no guide promises propagation that does not happen.

**Why this priority**: This is the only part of the issue that reaches people who never read the specs. The behaviour is deliberate, but an undocumented deliberate behaviour is indistinguishable from a bug to the person hitting it. It ranks alongside Story 1 because it is the same fact, told to the audience that pays for not knowing it.

**Independent Test**: Starting from the routing and aliases guide, the TLS guide, or the troubleshooting page with the symptom "I changed an alias and redeployed but the route did not change," reach an explanation of the limitation and the steps to apply the change within one link.

**Acceptance Scenarios**:

1. **Given** the docs site, **When** an operator looks for known limitations, **Then** they find a section on generated gateway file preservation that says which files are preserved, which manifest and configuration changes consequently do not propagate, how to recognise that a file was preserved, and how to apply the change.
2. **Given** the routing and aliases guide and the TLS guide, **When** an operator reads the passage describing a field that is affected, **Then** that passage links to the limitation.
3. **Given** the troubleshooting page, **When** an operator scans it for "my routing change did not take effect," **Then** an entry names the cause and links to the limitation.
4. **Given** the TLS guide, **When** an operator reads the steps for adding the secure port, **Then** the guide says the secure entrypoint is added to the static configuration only when that file does not yet exist, and no longer implies Shrine distinguishes a file it generated from one an operator edited.

---

### User Story 3 - A reader of specs 015, 009, 021, and 012 sees what shipped, not what was once intended (Priority: P2)

Four specs describe behaviour the product does not have. Spec 015 still specifies vault references on Resource *outputs*; spec 021 deliberately reversed that — vault references live in a Resource's `env`, and outputs are a name-only export allowlist. Spec 009 says the orphaned-routing-file warning fires on *deploy* when an application is removed from the manifest; the plan resolved that it fires on *teardown*, and that is what shipped, but the spec text was never amended. Spec 021's requirement that dry-run render a resource's container environment and published interface separately, and spec 012's requirement that a non-boolean `tls` value be rejected with an error naming the application and alias index, are both marked done in their task lists and are not implemented. After this change, each spec says what the product does, and requirements that did not ship are visibly descoped and recorded as future work rather than claimed as delivered.

**Why this priority**: These are four independent corrections, each confined to one spec. A reader misled by one of them is misled about one feature, not about a cross-cutting rule. They rank below the preserve policy for that reason, and above the tracking metadata because spec text is what tests and future specs are written against.

**Independent Test**: For each of the four specs, take the statement the issue names, compare it with the shipped product, and confirm they agree — or that the statement is marked descoped with a pointer to where the work is tracked.

**Acceptance Scenarios**:

1. **Given** spec 015, **When** a reader looks for where vault references are valid, **Then** every statement says Application `env` and Resource `env`, none says Resource outputs, and the spec names 021 as the change that superseded the original shape.
2. **Given** spec 009, **When** a reader looks for when the orphaned-file warning fires, **Then** every statement says on teardown, and none says on deploy.
3. **Given** spec 021's dry-run requirement, **When** a reader reads it, **Then** it is marked descoped, states which part shipped and which did not, and points to where the remainder is tracked.
4. **Given** spec 012's `tls` validation requirement, **When** a reader reads it, **Then** the rejection of non-boolean values stands as delivered, and the clause requiring the error to name the application and alias index is marked descoped with the same kind of pointer.
5. **Given** the task lists of specs 021 and 012, **When** a reader checks the tasks that cover the descoped behaviour, **Then** no task is marked complete for work that did not ship.

---

### User Story 4 - An operator wires a database to an application by following a guide (Priority: P2)

Spec 021 split a Resource's configuration into `env` (what its own container receives) and `outputs` (what other manifests may read). The manifest reference documents every field correctly, but only as table rows. The patterns an operator needs to build something real — keep a generated password private, publish a composed connection string without exposing the raw password, give exports stable names that do not change when internal variable names do, chain one resource into another, point one application at another's host and port — exist nowhere as a worked example. The deploy-order inference added by spec 020, and its rule that cross-team references must be declared explicitly, has no guide at all. After this change, the docs site has a walkthrough that builds a database Resource and a consuming Application step by step and covers each of these patterns with a manifest the operator can copy.

**Why this priority**: This is new content rather than a correction, and the information is already reachable in the reference for a determined reader. It ranks with Story 3: valuable, self-contained, and not the cause of anyone being actively misinformed.

**Independent Test**: Hand the guide to someone who has deployed a single application with Shrine but never used Resource outputs. Starting from an empty directory and using only the guide, they produce manifests for a database and an application that receives its connection string, and the manifests pass a preview deploy as written.

**Acceptance Scenarios**:

1. **Given** the guides section of the docs site, **When** an operator browses it, **Then** a guide on wiring env and outputs is listed alongside the existing guides.
2. **Given** the guide, **When** an operator reads its opening, **Then** it explains the mental model: `env` is the container's private runtime configuration, `outputs` is the published allowlist, and nothing is exported unless listed.
3. **Given** the guide's worked example, **When** an operator copies the manifests as written, **Then** they validate and preview successfully without modification.
4. **Given** the guide, **When** an operator looks for any of the seven patterns named in the issue, **Then** each is present with a manifest fragment and a sentence on when to use it.
5. **Given** the guide's section on deploy order, **When** an operator reads it, **Then** they learn that same-team references order themselves automatically, that cross-team references require an explicit dependency entry, and what the failure looks like when one is missing.

---

### User Story 5 - A contributor following the documented session-start ritual lands on accurate status (Priority: P3)

The specs directory tells every new contributor and every AI assistant how to start: read the project reference, read the progress file, read the feature spec. All three are stale. The progress file lists routing as the pending next phase though it shipped long ago. The specs README describes a layout with three legacy feature files and does not mention the numbered spec directories where every feature since has been specified. Three legacy feature files carry statuses that no longer match reality. The project reference documents a planner entry point that was deleted. Two empty spec directories from withdrawn features sit in the working tree. After this change, a contributor who follows the ritual arrives at a true picture of what shipped and what is next.

**Why this priority**: The cost of stale metadata is a few wasted minutes and some doubt, and anyone who checks the code recovers quickly. It is included because the ritual is the project's stated onboarding path and should not open with false statements, but it is the lowest-stakes part of the issue.

**Independent Test**: Follow the three session-start steps exactly as the specs README states them, then list what the materials say has shipped and what is next. Compare with the product.

**Acceptance Scenarios**:

1. **Given** the progress file, **When** a contributor reads the phase list and the current-state summary, **Then** no shipped phase is shown as pending, routing is shown as complete with a pointer to its specs, and the "next phase" names something that has not shipped.
2. **Given** the specs README, **When** a contributor reads the directory layout and the session-start steps, **Then** the numbered spec directories are described and the steps lead to the right spec for a feature.
3. **Given** the legacy routing, logging-observer, and integration-tests feature files, **When** a contributor reads each status, **Then** it matches what shipped, and where a numbered spec superseded the file, the file says which.
4. **Given** the project reference, **When** a contributor reads the planner section, **Then** it names only entry points that exist.
5. **Given** the specs directory, **When** a contributor lists it, **Then** the two empty directories are gone and the unused numbers are accounted for.

---

### Edge Cases

- **Amending history versus rewriting it**: Specs are also the record of what was decided and when. An amendment that silently replaces the original sentence erases why tests and tasks cite it. Each amendment must leave the requirement's identifier intact and be visibly marked as a later change.
- **Spec 011's edge case concerns a different file than spec 009's policy**: The issue attributes all four contradictions to spec 009, but 011's "`tlsPort` removed ⇒ regenerate" concerns the gateway's *static* configuration, whose preservation was established by spec 004. The canonical statement must cover both file classes, or 011 would be left pointing at a rule that does not mention its file.
- **Not every generated gateway file follows the policy**: The dashboard route file is written when absent and preserved when present, but — unlike per-application files — Shrine deletes it when the dashboard is no longer configured (spec 024). A canonical statement that says "Shrine never deletes a generated gateway file" would introduce a new contradiction. The statement must enumerate what it covers.
- **Spec 012's success criterion is already half-hedged**: It carries a parenthetical "subject to spec 009 preservation" and then says operators "do not need to hand-edit." Both halves are true and the sentence still misleads, because it omits the step the operator must take. Spec 029's revert scenario deletes the file before redeploying; the amended text must agree with that scenario.
- **The stale statements are more numerous than the ones the issue names**: Spec 015 repeats the Resource-output vault shape in an edge case, a second requirement, a key entity, and an assumption; spec 009 repeats "on deploy" in an edge case and two success criteria. Fixing only the cited identifiers would leave the spec contradicting itself.
- **The audit is older than the code it describes**: The issue was written against a revision seven merged changes ago. Every claim was re-checked for this spec and still holds, but line references have moved, and any statement corrected by this feature must be checked against the product as it is on the day of the change, not as the issue describes it.
- **Task lists claim the descoped work**: One completed task in spec 021 names a file that does not exist; one in spec 012 describes an assertion its test does not make. Descoping the requirement without correcting the task leaves the checkbox lying.
- **The new guide must not promise descoped behaviour**: A wiring walkthrough naturally reaches for "preview it with dry-run and see your exports." Dry-run does not render a resource's published interface separately — that is one of the requirements this feature descopes. The guide must describe only what the preview actually shows.
- **Other phases in the progress file are stale too**: The issue names routing, but teardown is also listed as pending and shipped. Correcting one line and leaving its neighbour wrong does not fix the file.
- **The two empty directories are not under version control**: They exist only in working copies, so removing them produces no reviewable change, and a fresh clone never had them. What a reviewer can verify is that the numbering gap they leave is explained.
- **Generated release notes will not carry this change**: Release notes are produced from the change history and exclude documentation changes. The limitation must live on the docs site to be found.

## Requirements *(mandatory)*

### Functional Requirements

**Canonical preserve policy (specs 006, 008, 009, 011, 012)**

- **FR-001**: Spec 009 MUST contain exactly one clearly labelled, directly linkable statement of the lifecycle policy for generated gateway files, written so it can be read without the rest of the spec.
- **FR-002**: The canonical statement MUST say that a covered file is written only when absent; that once present it is never modified or deleted by a deploy or a teardown, regardless of whether Shrine or an operator last wrote it; that Shrine does not distinguish between the two; and that the operator applies a manifest change by deleting the file and redeploying, or by editing the file directly.
- **FR-003**: The canonical statement MUST enumerate the files it covers — per-application routing files and the gateway static configuration, citing spec 004 as the origin of the latter — and MUST state that the dashboard route file follows a different lifecycle, citing spec 024.
- **FR-004**: The canonical statement MUST list every input whose change consequently does not propagate to an existing file. At minimum: for a per-application file, the primary domain and path prefix, the application's port, adding or removing an alias, and an alias's host, path prefix, `stripPrefix`, and `tls`; for the static configuration, the gateway's HTTP port, adding, changing, or removing the secure port, and enabling or changing the dashboard port.
- **FR-005**: Spec 006's alias-removal requirement (FR-009) and success criterion (SC-004), spec 008's `stripPrefix`-change requirement (FR-007), spec 011's "`tlsPort` removed between deploys" edge case, and spec 012's TLS-revert success criterion (SC-005) MUST each be amended to say the described outcome occurs only when the affected file is regenerated, and MUST each reference the canonical statement rather than restating the policy.
- **FR-006**: After the change, no statement in specs 004, 006, 008, 009, 011, or 012 may promise that a manifest or configuration change alone alters an existing generated gateway file, or imply that Shrine distinguishes a file it generated from one an operator edited.
- **FR-007**: Every amendment to an existing spec made by this feature MUST keep the requirement's or criterion's identifier unchanged and MUST carry a visible, dated note naming this feature as the source of the change.

**Operator-facing limitation (docs site)**

- **FR-008**: The docs site MUST include a known-limitations section on generated gateway file preservation that states which files are preserved, which changes do not propagate (the list in FR-004), the deploy-output signal that tells the operator a file was preserved, the steps to apply a change, and that teardown leaves the per-application file in place with a warning naming it.
- **FR-009**: The routing and aliases guide, the TLS guide, and the Traefik gateway guide MUST link to that section from the passages describing affected fields, and the troubleshooting page MUST include an entry for the symptom "a routing change in the manifest did not take effect" that links to it.
- **FR-010**: Statements in existing guides that contradict the policy MUST be corrected. In particular, the TLS guide MUST say the secure entrypoint is written to the static configuration only when that file does not yet exist.

**Specs describe what shipped (specs 009, 012, 015, 021)**

- **FR-011**: Spec 015 MUST state that vault references are valid in Application `env` and Resource `env` and are not valid on Resource outputs, MUST describe outputs as a name-only export allowlist with an optional template, and MUST name spec 021 as the superseding change. This applies to every statement in the spec that describes the old shape, not only the requirement and acceptance scenario the issue cites.
- **FR-012**: Spec 009 MUST state that the orphaned-file warning fires on teardown of the application, in every statement that describes when it fires, and MUST NOT state or imply that it fires on a deploy from which the application has been removed.
- **FR-013**: Spec 021's dry-run requirement (FR-013) MUST be marked descoped, with a note stating which part shipped (the preview resolves values to placeholders internally, without generating secrets or reading the vault) and which did not (the preview prints neither the container environment nor the published interface).
- **FR-014**: Spec 012's `tls` validation requirement (FR-004) MUST be marked as delivered for the rejection of non-boolean values and descoped for the clause requiring the error to name the offending application and alias index.
- **FR-015**: A descoped requirement MUST keep its identifier and original text, MUST be visibly marked descoped with a date and a reason, and MUST point to where the remaining work is recorded.
- **FR-016**: Each descoped item MUST be recorded in the project's list of known gaps so the work is not lost.
- **FR-017**: Tasks in specs 021 and 012 that are marked complete for descoped behaviour MUST be corrected so no task claims work that did not ship, and so no task describes a file or assertion that does not exist.
- **FR-032** *(added during planning, 2026-10-05)*: The same unshipped preview behaviour claimed elsewhere MUST be corrected to match FR-013: spec 015's acceptance scenario that the preview shows a vault placeholder for each env var MUST be marked descoped under the same known-gaps entry, and the secrets vault guide MUST NOT show sample preview output the product does not print.

**Tracking metadata**

- **FR-018**: The progress file's phase list and current-state summary MUST match the shipped product: routing shown as complete with a pointer to the specs that delivered it; every other phase line checked and corrected where stale; the "next phase" naming work that has not shipped.
- **FR-019**: The specs README MUST describe the actual layout of the specs directory, including the numbered spec directories, and its session-start steps MUST lead a reader to the correct spec for a feature whether it lives in a numbered directory or a legacy feature file. It MUST NOT describe a shipped feature as upcoming.
- **FR-020**: The legacy routing, logging-observer, and integration-tests feature files MUST each carry a status that matches the shipped product. Where a numbered spec superseded the file, the file MUST name it; where the shipped design differs from the one the file describes, the file MUST say so.
- **FR-021**: The project reference MUST NOT document a planner entry point that no longer exists, and its description of how the gateway plugin treats generated files MUST agree with the canonical statement.
- **FR-022**: The two empty spec directories for withdrawn features (005 and 007) MUST be removed, and the specs README MUST record that those two numbers are intentionally unused.

**Wiring guide (docs site)**

- **FR-023**: The docs site MUST include a guide on wiring env and outputs, listed in the guides index and following the structure of the existing guides.
- **FR-024**: The guide MUST open with the mental model: a Resource's `env` is its container's private runtime configuration; its `outputs` are the published allowlist other manifests may read; an output is never injected into the Resource's own container; an env var that is not exported is private.
- **FR-025**: The guide MUST build one worked example end to end — a database Resource and an Application that consumes it — showing complete manifests at each step.
- **FR-026**: The guide MUST cover, each with a manifest fragment and a statement of when to use it: (a) a generated secret kept private; (b) a composed connection string exported through an output template without exporting the raw password; (c) export names that stay stable when internal env names change; (d) an Application env var reading a Resource output; (e) one Application reaching another through the `host` and `port` built-ins with a template-composed URL; (f) a Resource consuming another Resource's output; (g) inferred deploy order for same-team references and the explicit-dependency requirement for cross-team references, including what the failure reports.
- **FR-027**: Every manifest in the guide MUST validate and preview successfully as written, and the guide MUST NOT describe behaviour that is unshipped or descoped — in particular it MUST NOT claim the preview renders a resource's published interface.
- **FR-028**: The guide MUST link to the manifest reference for field-level detail and to the secrets vault guide for vault references rather than duplicating them, and the manifest reference MUST link to the guide.

**Change hygiene**

- **FR-029**: The change MUST NOT alter product behaviour: no product source, test, or build configuration is modified.
- **FR-030**: The docs site MUST build cleanly with the new and edited pages, with no broken internal links.
- **FR-031**: Where re-checking a statement against the product shows that shipped behaviour differs from what the issue describes, the corrected text MUST follow the shipped behaviour, and the divergence MUST be reported with the change rather than resolved silently.

### Key Entities

- **Canonical preserve-policy statement**: The single passage in spec 009 that defines the lifecycle of generated gateway files. Every other mention in the spec set and the docs site refers to it or agrees with it.
- **Amendment note**: A visible, dated marker on an existing requirement or success criterion recording that this feature changed it, with the identifier preserved.
- **Descope note**: A marker on a requirement that did not ship, preserving its text and identifier, stating what did and did not ship, and pointing to the known-gaps entry.
- **Known-limitations section**: The operator-facing statement of the preserve policy on the docs site, reachable from the guides and the troubleshooting page.
- **Wiring guide**: The new walkthrough on env and outputs, built around one database-and-application example.
- **Tracking metadata**: The progress file, the specs README, the legacy feature files' status lines, and the project reference — the materials the session-start ritual sends a reader to.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All five contradictory statements named in issue #39 (two in spec 006, one each in 008, 011, and 012) reference the canonical statement, and a full read of specs 006, 008, 009, 011, and 012 finds zero statements promising that a manifest change alone alters an existing generated gateway file.
- **SC-002**: The question "I removed an alias and redeployed — is the route gone?" receives the same answer from spec 006, spec 009, the routing and aliases guide, and the known-limitations section.
- **SC-003**: From the routing and aliases guide, the TLS guide, or the troubleshooting page, an operator reaches the explanation of the limitation and the steps to apply a change in one link.
- **SC-004**: Spec 015 contains zero statements presenting vault references on Resource outputs as supported, and spec 009 contains zero statements placing the orphaned-file warning on deploy.
- **SC-005**: Both descoped requirements are marked descoped in their specs, both appear in the known-gaps list, and zero tasks in specs 021 and 012 are marked complete for behaviour that did not ship.
- **SC-006**: All seven wiring patterns named in issue #39 appear in the guide with a manifest fragment each, and 100% of the guide's manifests pass validation and a preview deploy as written.
- **SC-007**: A reader with no prior exposure to Resource outputs, using only the guide, produces a working database-plus-application pair on the first attempt.
- **SC-008**: A contributor following the session-start steps finds zero shipped phases listed as pending, zero legacy feature files with a status contradicting the product, and zero references in the project reference to entry points that do not exist.
- **SC-009**: The change modifies zero product source, test, or build files; an operator running any command before and after sees identical results.
- **SC-010**: The docs site builds with zero errors and zero broken internal links.

## Assumptions

- **Source of scope**: With no description typed on the command beyond the issue link, issue #39's six problems and its proposed solution are taken as the feature scope in full.
- **Re-verified on 2026-10-05**: The issue's audit predates seven merged changes. Each of its six claims was re-checked against the current main branch while writing this spec and all still hold; only line references have shifted. Planning must still confirm exact wording at the time of the edit (FR-031).
- **Adjacent stale text is in scope**: Re-verification found the same defects in text the issue does not cite — further Resource-output vault statements in spec 015, further "on deploy" statements in spec 009, a teardown phase listed as pending in the progress file, the TLS guide's "Shrine-generated (not operator-preserved)" wording, and two task entries that claim the descoped work. These are included because fixing only the cited lines would leave each document contradicting itself. Planning research found four more of the same kind and they are included on the same grounds (see [research.md](research.md) Part 3): spec 004's requirement that per-route files are "written and removed by Shrine", spec 011's success criterion on adding HTTPS to an existing deployment, the project reference's description of generated files as "managed", and spec 015's and the secrets vault guide's claim that the preview prints env placeholders (FR-032).
- **Shipped behaviour is the reference**: Where spec and product disagree, this feature changes the spec. It does not change the preserve policy, add a way to force regeneration, or implement either descoped requirement.
- **Where descoped work is tracked**: The project has no backlog document. The existing known-gaps list in the progress file is taken as the backlog the issue refers to. Opening tracker issues for the two descoped items is optional and outside this feature.
- **Where the limitation is published**: The project keeps no hand-written release notes; they are generated from the change history and exclude documentation changes. The docs site is therefore the home for the known-limitations section. Adding a line to a published release's notes is a maintainer step outside this feature.
- **The empty directories**: They are untracked leftovers present only in working copies. Removing them is done locally and leaves nothing to review; the durable, reviewable part of FR-022 is the README note explaining the unused numbers.
- **Delivery shape**: One documentation-only change set, as the issue proposes. The five stories are independent and could be split if review size becomes a problem.
- **Guide verification**: The guide's manifests are checked by previewing them as written before the change is merged. Adding automated checks that keep docs examples valid over time is not part of this feature.
- **Out of scope**: The `Status: Draft` header carried by every numbered spec regardless of whether it shipped — a template default the project has never maintained and the issue does not name; the legacy daemon feature file, whose "planned" status is accurate; restructuring the legacy feature files beyond their status and supersession pointers; and any change to how release notes are generated.
