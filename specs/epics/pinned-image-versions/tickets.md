# Ticket breakdown: Pinned Image Versions

**Status**: Draft for review
**Source**: [prd.md](prd.md) for the what and why; [design.md](design.md) for the technical binding every ticket builds on; [delivery-plan.md](delivery-plan.md) for when each ticket runs
**Created**: 2026-10-06

## Rules this breakdown follows

- Every ticket is shippable on its own: when it merges, the product is complete and useful without the tickets after it. No ticket leaves a value, flag, or command that exists but does nothing.
- A ticket may depend on tickets before it. Each ticket has a Dependencies section naming what blocks it, what it blocks, what it shares with tickets in the same wave, and anything external it needs. [delivery-plan.md](delivery-plan.md) orders the tickets into waves.
- One ticket becomes one GitHub issue, one numbered spec under `specs/NNN-<name>/`, one branch, one agent.
- A ticket owns the documentation for what it changes: manifest reference, configuration docs, generated CLI pages, and the contributor reference. The operator guide is its own ticket at the end.
- Each ticket names the integration scenarios that gate it, per the constitution.
- Each ticket has a technical overview that summarises the design decisions it implements and points at the matching section of [design.md](design.md). The design document's per-ticket requirement list (`T<n>-<nn>`) is the starting point for that ticket's spec.
- Requirement ids are the PRD's R-xx.

## Dependency sketch

```text
T1 deployed version in get/describe ──┐
                                      ├──> T5 pinned versions in queries
T2 pre-deploy image check ──> T3 pinned policy on deploy ──┤
                                      │                    ├──> T6 bump
                                      │                    ├──> T7 delete resource
                                      └──> T4 config default + generate
T8 operator guide: after everything it documents
```

T1 and T2 have no dependencies and can run in parallel. T3 is the core and the largest. T4, T5, T6, T7 are independent of each other once T3 is in. T8 closes.

---

## T1. Show the deployed version in `get` and `describe`

**Why it stands alone.** Today `get deployed` shows team, name, kind, and container id; nobody can tell which image an artifact was deployed with. Recording the image reference at deploy time and showing it is useful before pinning exists and is the display surface pinning will extend.

**Scope**
- Deploy records, per artifact, the image reference the manifest named and the effective pull policy, alongside today's deployment record.
- `get deployed`, `get applications`, `get resources` gain a VERSION column showing that reference; with and without `--team`; state only, no Docker access.
- `describe app` and `describe resource` show the image reference and the pull policy.
- Records written before this ticket show the version as unknown.

**Out of scope.** Pins, exact versions, running image.

**Requirements.** R-17 (manifest-owned part), R-18 (repository, manifest version, policy only).

**Dependencies**
- Blocked by: none.
- Blocks: T5.
- Shares with its wave: nothing. T2 touches none of the files T1 changes, so the two merge in either order.
- External: none.

**Technical overview.** The deployment record grows by two trailing fields, image and policy, written by the Docker backend when it records a deployment and read back tolerantly so four-field lines from earlier releases still load. The image is stored as the manifest wrote it, alias unexpanded, which means capturing it before the backend expands the reference. The tables and the describe output read the new fields; no command file changes. Recording the policy now, while it only has two values, is what lets T5 tell pinned from manifest-owned without a second format change. See [design.md, section 3.3](design.md#33-deployment-record-extended) and the T1 requirement list in [design.md](design.md#t1-deployed-version-in-get-and-describe).

**Acceptance**
- After deploying the standard fixtures, `get deployed` lists each artifact with the image reference from its manifest.
- `describe` on one of them shows the same reference and the pull policy.
- A deployment record from a previous release still lists, with the version shown as unknown, and the next deploy fills it in.
- Existing columns and their order are unchanged.

**Integration scenarios.** Extend the existing `get` and `describe` suites: version column present after deploy; legacy record tolerated.

**Size.** Small.

---

## T2. Resolve every image before touching any container

**Why it stands alone.** Today a bad image reference on the fifth artifact fails after four containers were already created or recreated. Resolving every image of the deploy set up front, as a step of its own, makes any pull failure a zero-change failure. It is the seam pinning plugs into, but it is a fix worth shipping by itself.

**Scope**
- A new deploy step, after planning and before the first container or network operation, that resolves the image of every artifact in the deploy set and stops the deploy with no changes if any one fails. The failure names the artifact and the reference.
- Resolution yields the registry's exact version, not Docker's local image id, so a later ticket can pin it and so the deploy can print it.
- Deploy output states, per artifact, the exact version it will run.
- Dry run prints the step, one line per artifact, and touches neither Docker nor state.
- Contributor reference: the deploy pipeline diagram gains the step.

**Out of scope.** Pins, policy changes, any new manifest field.

**Requirements.** R-14, R-15 (manifest-owned part), R-16 (the step appears in dry run).

**Dependencies**
- Blocked by: none.
- Blocks: T3.
- Shares with its wave: nothing. T1 touches none of the engine, backend, or renderer files T2 changes.
- External: none.

**Technical overview.** Image resolution leaves `CreateContainer` and becomes a method on the container backend contract, `ResolveImage`, returning the expanded reference, the registry digest picked from the pulled image's repository digests, and the local image id. The engine calls it for every step as a pre-pass at the very top of the deploy, before even the platform network, and hands the result to the container op, which then skips resolution; the Traefik plugin's direct path is untouched. The config hash keeps the image id as its input so nothing is recreated on upgrade. The dry-run backend prints one line per artifact. Two terminal renderers appear for the new `image.resolve` events. See [design.md, sections 4.1 to 4.4](design.md#41-backend-contract), decisions TD-2 and TD-5, and the T2 requirement list in [design.md](design.md#t2-resolve-every-image-before-touching-any-container).

**Acceptance**
- A deploy set with one unresolvable reference among several artifacts creates, changes, and removes zero containers and networks, and the error names the artifact and reference.
- A healthy deploy set deploys exactly as before, with the new per-artifact version lines added to the output.
- Dry run shows the resolution step before the container operations.
- Pull policy semantics are unchanged: `latest` still re-pulls, fixed tags still reuse the local image.

**Integration scenarios.** New: unresolvable image among healthy ones leaves Docker untouched; dry run shows the step. Existing deploy scenarios unchanged.

**Size.** Medium.

---

## T3. The pinned policy: resolve once, keep the exact version

**Why it stands alone.** This is goals G1 and G5 end to end. After it merges an operator can set the policy on a manifest, deploy, and get the same exact version back after redeploy, recreate, teardown, and a wiped image cache.

**Scope**
- Manifests accept the third policy value on Application and Resource.
- Validation: under the third value no fixed version may be named; Resource `version` becomes optional; violations are reported with the manifest's other errors, naming artifact and field.
- Effective policy: the manifest field, else today's derived rule. The configuration layer is T4.
- A pin record per artifact: exact version, readable version resolved from, date. Survives containers and teardown. Released by `delete application`, `delete team`, and by a deploy of the artifact under a manifest-owned policy.
- First deploy resolves newest and pins; later deploys run the pinned exact version, fetching by exact version when the local cache lacks it and making no registry call when it has it.
- A pinned exact version the registry no longer serves fails in the T2 step with a message pointing at bump, which does not exist yet; the message names the manifest as the way out until T6 lands.
- A deploy under a manifest-owned policy releases the pin; the next pinned deploy pins afresh.
- Deploy output says, per artifact, pinned now, reused, or manifest-owned. Dry run says would-pin or would-reuse and writes nothing.
- Manifest reference: the third value, the no-fixed-version rule, the pin lifecycle. Contributor reference: state layout gains the pin record.

**Out of scope.** Configuration default (T4), version shown in queries (T5), bump (T6), delete resource (T7).

**Requirements.** R-01, R-02, R-03, R-04 (without the configuration layer), R-09, R-10, R-11 (existing deletes), R-12, R-13, R-15, R-16, R-28 (partial), R-32.

**Dependencies**
- Blocked by: T2, for the `ResolveImage` seam and the pre-pass it plugs into.
- Blocks: T4, T5, T6, T7, and through them T8.
- Shares with its wave: nothing; it runs alone.
- External: a `registry:2` container the integration suite starts on the CI runner, bound to the loopback interface.

**Technical overview.** Three things land together. A new per-team state file, `pins.txt`, behind an `ImagePinStore` interface copied from the host-port store's lifecycle and file discipline, holding the requested reference, the pullable digest reference, and the date. The Docker backend's `ResolveImage` grows the pinned branch: reuse the pin, pulling by digest only when it is absent locally, or pull newest and write the pin; a manifest-owned resolution releases any pin, which is the rule that makes "return to pinned" a first deploy. And the planner gains a normalisation step that writes the effective policy back into every manifest before a new validation pass enforces the no-fixed-version rule and takes over the version-required check from parse time; `Plan` gains a default-policy parameter that this ticket threads as empty. The dry-run backend receives a read-only pin snapshot the way it receives host ports. The integration gate needs a registry the suite can push to, so this ticket also adds a local registry helper to the test utilities. See [design.md, sections 3.4, 4.2, 4.5, and 5](design.md#34-pin-record-new), decisions TD-1, TD-3, TD-6, TD-7, TD-8, TD-13, and the T3 requirement list in [design.md](design.md#t3-the-pinned-policy).

**Acceptance**
- A manifest under the third value that names a fixed version is rejected at validation, naming the field.
- First deploy pins; the output shows the exact version and says it was pinned.
- Ten cycles mixing redeploy, forced recreate, teardown then deploy, and removing the image locally all run the same exact version while the registry's `latest` has moved.
- `delete application` releases the pin; the next deploy pins afresh. Deploying the artifact under a fixed tag releases the pin too.
- Repeated dry runs leave recorded state byte for byte unchanged.
- Manifests without the value behave exactly as before.

**Integration scenarios.** New suite for the pinned policy covering the cycles above, the validation rejection, the release paths, and the dry-run stability, against a local registry started by the suite.

**Size.** Large. It can be split into value-and-validation first and pinning second, at the cost of one release in which the value is accepted but behaves as `Always`. The recommendation is to keep it whole.

---

## T4. Configuration default and generated manifests

**Why it stands alone.** With T3 in, the house rule of journey J7 is one line of configuration, and new manifests from `generate` follow it.

**Scope**
- The configuration file accepts a default image pull policy with the three values. Absent means the derived rule, so nothing changes for existing installations.
- Effective policy becomes: manifest field, then configuration default, then derived rule.
- With the default set to the third value, manifests that name a fixed version and no policy fail validation with a message naming the setting and the two ways out.
- `generate application` and `generate resource` emit manifests valid under the effective default.
- Configuration docs and manifest reference updated for precedence.

**Out of scope.** Anything per artifact beyond precedence.

**Requirements.** R-04 (complete), R-06, R-07, R-08, R-29.

**Dependencies**
- Blocked by: T3, for the `Pinned` value and the planner's default-policy parameter.
- Blocks: T8.
- Shares with its wave: the plan call sites in `internal/handler/deploy.go` and `internal/handler/apply.go` with nobody; `internal/planner/resolve.go` with nobody; the configuration layout lines of `AGENTS.md` with the CLI reference lines T6 and T7 edit.
- External: none.

**Technical overview.** A top-level `imagePullPolicy` key on the config struct, validated on load, is threaded into the planner's existing default-policy parameter by the three handlers that plan, so the precedence order is already implemented and this ticket only supplies the middle layer. The validation pass learns whether a policy came from the default so the configuration-sourced message can name the key. The generate skeletons take the effective default and, under the pinned value, drop the tag from the app image and the version line from the resource. See [design.md, sections 3.2, 4.5, and 4.10](design.md#32-configuration), decision TD-10, and the T4 requirement list in [design.md](design.md#t4-configuration-default-and-generated-manifests).

**Acceptance**
- No setting: derived rule, existing suites unchanged.
- Setting is the third value: a manifest with no policy and no fixed version pins; a manifest with a fixed version fails with the named message; a manifest with its own policy field is unaffected.
- Setting is `IfNotPresent` or `Always`: applied to manifests without their own field.
- Generated manifests validate under each of the three defaults.

**Integration scenarios.** New: the three defaults against fixture manifests; generate then deploy under the third value.

**Size.** Small to medium.

---

## T5. Pinned versions in `get`, `describe`, and `status`

**Why it stands alone.** Journey J3 completed for pinned artifacts: a person can read what is pinned, when, and whether what runs differs from it.

**Scope**
- The VERSION column from T1 shows, for pinned artifacts, the readable version and the short exact version.
- `describe` shows the effective policy, the pin with its readable version, exact version, and date, and the image the running container was started from, so a recorded-but-not-deployed pin shows as a difference.
- Per OD-3, `status` shows the running image next to the running state.
- Pins of artifacts that are not deployed are never shown.

**Out of scope.** Any write.

**Requirements.** R-17 (pinned part), R-18 (complete), R-19, R-20.

**Dependencies**
- Blocked by: T1, for the image and policy in the record, and T3, for the pins.
- Blocks: T8.
- Shares with its wave: `internal/handler/deployments.go` with T7; `internal/engine/backends.go` for `ContainerInfo.Image` with nobody, since T6 adds its own field to a different type; `cmd/describe.go` and `internal/handler/status.go` with nobody.
- External: none.

**Technical overview.** The tables switch on the policy T1 recorded: a pinned record reads its pin and prints the readable form, tag plus twelve hex characters; everything else prints the recorded reference. `describe` gains a query-only container backend, like `delete application` has, and `ContainerInfo` gains the reference the container was created from, so the running image can sit next to the pin; Docker being unreachable degrades that one line rather than failing the command. `status` prints the same field as an IMAGE column. Nothing lists pins on their own, which is how torn-down pins stay internal. See [design.md, sections 3.5 and 4.7](design.md#35-readable-form), decision TD-11, and the T5 requirement list in [design.md](design.md#t5-pinned-versions-in-get-describe-and-status).

**Acceptance**
- After a pinned deploy, `get deployed` shows readable and short exact version for that artifact and the manifest reference for the others.
- `describe` shows policy, pin, date, and running image; after the pin changes without a deploy, the difference is visible.
- After teardown, the artifact is absent from `get` and `describe` reports it as not deployed, with no pin shown.

**Integration scenarios.** Extend `get`, `describe`, and `status` suites with a pinned fixture.

**Size.** Medium.

---

## T6. `shrine bump`

**Why it stands alone.** Journeys J4, J5, and J6: move a pinned artifact to a chosen version, to the newest, or back, without editing the manifest, applied on the next deploy.

**Scope**
- `shrine bump application|resource <name>` with the `app` and `res` aliases, optional `--team` with automatic search and ambiguity error, `--dry-run`, and `--path`.
- `-v` takes a readable or exact version; without it, newest. The repository always comes from the manifest.
- Resolves immediately, verifies existence in the registry, records the new pin, prints previous and new, touches no container.
- Works for any pinned artifact in the manifest directory, deployed or not. Refuses manifest-owned artifacts with a message naming the manifest, and unknown artifacts with a message naming the directory.
- The T3 message for a vanished exact version now points at bump.
- Generated CLI page for bump.

**Out of scope.** Team-wide bump (OD-5), applying the bump.

**Requirements.** R-05, R-21 to R-26, R-30 (bump page).

**Dependencies**
- Blocked by: T3, for the pin store, `ResolveImage`, and the local registry helper.
- Blocks: T8. T5 is not required, but with it the result of a bump is visible in `describe`.
- Shares with its wave: `internal/engine/backends.go`, where it adds `Repin` to `ResolveImageOp`, and `docker_image.go`, where it adds the fourth branch and reworded T3 message, with nobody; `internal/app/app.go` with nobody; the CLI reference lines of `AGENTS.md` with T4 and T7.
- External: the local registry helper from T3, reused by its suite.

**Technical overview.** A new command file and a new bump bundle in the composition root: specs directory, terminal observer, and a container backend, with no engine. The handler loads the manifest set the way deploy does, applies the effective policy, finds the artifact, refuses anything not pinned, and builds the target by swapping the manifest image's tag for the requested one or appending a digest. Resolution goes through the backend's `ResolveImage` with a `Repin` field this ticket adds to the op, so the backend remains the only writer of pins and bump is a fourth branch of a method that already exists. Previous and new come from a pin read before and the resolution result after. See [design.md, section 4.6](design.md#46-bump), decisions TD-8 and TD-12, and the T6 requirement list in [design.md](design.md#t6-shrine-bump).

**Acceptance**
- Bump to an existing version records it and prints previous and new; the container is untouched; the next deploy recreates the container on the new exact version.
- Bump to a version that does not exist fails and records nothing.
- Bump without `-v` records the current newest.
- Bump of a manifest-owned artifact is refused with the named message.
- Bump before first deploy, then deploy, runs the chosen version.
- Dry run prints and writes nothing.

**Integration scenarios.** New bump suite covering the six points above, against the local registry from T3.

**Size.** Medium.

---

## T7. `shrine delete resource` and pin release on every delete

**Why it stands alone.** Journey J8 closed for resources: the retire path exists for both kinds and every delete releases what the artifact held.

**Scope**
- `shrine delete resource <name>` mirroring `delete application`: refuses while the container exists, forgets the record, releases the pin, supports `--team` and `--dry-run`.
- Confirms `delete application` and `delete team` release pins, as T3 introduced.
- Generated CLI page for delete resource.

**Out of scope.** Anything beyond delete.

**Requirements.** R-27, R-11 (complete), R-30 (delete resource page).

**Dependencies**
- Blocked by: T3, for the pin release semantics.
- Blocks: T8.
- Shares with its wave: `internal/handler/deployments.go` with T5; `cmd/delete.go` with nobody; the CLI reference lines of `AGENTS.md` with T4 and T6.
- External: none.

**Technical overview.** `DeleteApplication` is generalised over the kind into one handler that both commands call; the resource path has no host port to release. The command file gains the subcommand beside `delete application`. The ticket's main value is the end-to-end assertion that all three delete verbs release pins. See [design.md, section 4.8](design.md#48-delete) and the T7 requirement list in [design.md](design.md#t7-shrine-delete-resource-and-pin-release-on-every-delete).

**Acceptance**
- Delete of a torn-down resource removes record and pin; the next deploy pins afresh.
- Delete while the container exists is refused and points at teardown.
- Dry run prints what would be released and writes nothing.
- Delete of a team releases every pin it held.

**Integration scenarios.** Extend the `delete` suite with resource cases and a pin-release assertion for each delete verb.

**Size.** Small.

---

## T8. Operator guide: managing image versions

**Why it stands alone.** The behaviour contract becomes public. Every example is captured from the real binary, as the wiring guide was.

**Scope**
- A guide walking journeys J1 to J8 with real output, linked from the guides index, the manifest reference, and troubleshooting where a pin cannot be honoured.
- A pass over the manifest reference, configuration docs, and CLI pages written by T3 to T7 for coherence of vocabulary.

**Requirements.** R-28 to R-30 (coherence), R-31.

**Dependencies**
- Blocked by: T3 to T7 for the final capture. Drafting can begin once T3 is on main.
- Blocks: nothing.
- Shares with its wave: the docs site only.
- External: the docs build in CI.

**Technical overview.** Documentation only, under the Hugo site's guides section, following the existing guide pattern and the capture discipline of the wiring guide. See [design.md, section 4.11](design.md#411-documentation-touch-points) and the T8 requirement list in [design.md](design.md#t8-operator-guide).

**Acceptance.** Metric M7: an operator new to the feature answers the six questions from the documentation alone. Docs build and link check pass.

**Size.** Medium.

---

## Notes for the delivery plan

The waves, gates, merge order, and the agent protocol live in [delivery-plan.md](delivery-plan.md). In short: T1 and T2 run in parallel first; T3 runs alone and is the critical path; T4, T5, T6, and T7 run in parallel after it; T8 closes.

## PRD amendments surfaced

- **M2** says deploy output for existing fixtures is unchanged. T2 and T3 add lines to deploy output by design. Proposed wording: existing output lines are unchanged; new lines may be added.
- **R-15** is delivered in two halves: the exact version per artifact in T2, the pinned, reused, or manifest-owned wording in T3.
- **R-11 and R-12**: the design releases a pin when the artifact deploys under a manifest-owned policy, instead of keeping it inert. Reason and wording in [design.md, section 6](design.md#6-prd-amendments-this-design-asks-for).
- **R-17 and T1**: the deployment record stores the policy as well as the image, so the record format changes once.
