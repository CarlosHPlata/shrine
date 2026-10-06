# Feature Specification: A Configuration Default for the Image Pull Policy, and Manifests Generated to Match

**Feature Branch**: `034-pull-policy-config-default`
**Created**: 2026-10-06
**Status**: Draft
**Input**: User description: "https://github.com/CarlosHPlata/shrine/issues/55"
**Epic**: Pinned Image Versions, ticket T4 (issue #55). Source documents: [prd.md](../epics/pinned-image-versions/prd.md) (journey J7; requirements R-04, R-06, R-07, R-08, R-28, R-29; decisions D3, D10; open decision OD-2), [design.md](../epics/pinned-image-versions/design.md) (decisions TD-7, TD-10; sections 3.2, 4.5, 4.10; requirement list T4-01 to T4-05), [tickets.md](../epics/pinned-image-versions/tickets.md#t4-configuration-default-and-generated-manifests) section T4. Builds on ticket T3 ([spec 033](../033-pinned-image-policy/spec.md), merged in #62), which delivered the `Pinned` value, plan-time normalisation, and the no-fixed-version rule with the configuration layer of the precedence left empty.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Make pinning the house rule with one line of configuration (Priority: P1)

An operator who wants every artifact pinned by default writes one line in Shrine's configuration file: the default image pull policy, set to `Pinned`. From then on every manifest that names no policy of its own is pinned: its first deploy resolves the newest version and records a pin, and every later deploy reuses it, exactly as if the manifest had named `Pinned` itself. A manifest that does name a policy keeps it; the house rule never overrides an explicit choice. Dry run and single-manifest apply follow the same rule as deploy.

**Why this priority**: This is journey J7 of the PRD and the reason the ticket exists. Without it, pinning has to be requested manifest by manifest.

**Independent Test**: With the configuration default set to `Pinned`, deploy a fixture set holding a manifest with no policy and no fixed version, a manifest with its own `IfNotPresent` and a fixed version, and a manifest with its own `Always`; verify the first pins and the other two behave as before. Repeat with the setting removed and verify the first artifact follows the derived rule again.

**Acceptance Scenarios**:

1. **Given** the configuration default is `Pinned` and an Application manifest names no policy and an image with no tag or the tag `latest`, **When** the operator deploys, **Then** the artifact is pinned: the deploy output states the exact version and that it was pinned on this deploy, and a later deploy reuses the pin and says so.
2. **Given** the configuration default is `Pinned` and a Resource manifest names no policy and no `version`, **When** the operator deploys, **Then** the manifest is accepted, the resource's repository is its type (or its image override when one is set), and the resource is pinned.
3. **Given** the configuration default is `Pinned` and a manifest names `IfNotPresent` or `Always` and a fixed version, **When** the operator deploys, **Then** the manifest's own policy applies, the fixed version is accepted, and the deploy output is the manifest-owned output of today.
4. **Given** the configuration default is `Pinned` and a manifest names `Pinned` itself, **When** the operator deploys, **Then** behaviour is identical to the same manifest deployed with no configuration default: same pin, same output.
5. **Given** the configuration default is `Pinned`, **When** the operator runs a dry run or applies one manifest with `shrine apply -f`, **Then** the effective policy of each artifact is the same one deploy would use, and the dry run prints the pinned preview lines of T3 for artifacts the default pins.
6. **Given** the configuration default is `Pinned` and a team is deployed, **When** the operator removes the setting and deploys again, **Then** every artifact the default had pinned now resolves under the derived rule as a manifest-owned artifact and its pin is released, as a manifest-owned deploy releases a pin today; returning the setting pins afresh on the next deploy.

---

### User Story 2 - Existing manifests that contradict the house rule fail loudly and say what to do (Priority: P2)

An operator who sets the default to `Pinned` on an installation with existing manifests runs the next deploy and is told, before anything changes, which manifests still name a fixed version. Each message names the artifact and the field, says the value names a fixed version while the image pull policy is `Pinned`, says the policy came from the configuration setting, and names the two ways out: set a policy on that manifest, or change the default. All such errors are reported together, with the manifests' other validation errors, so the operator can fix every manifest in one pass.

**Why this priority**: PRD R-07 is loud by design; the PRD's risk list says the ticket that owns it must get the message right. The house rule is only safe to adopt if the manifests it breaks are named precisely.

**Independent Test**: With the configuration default set to `Pinned`, deploy a fixture set holding an Application with a fixed tag and no policy, a Resource with `version: "16"` and no policy, a Resource with an image override naming a fixed tag, and an Application that names `Pinned` itself with a fixed tag; verify every message, that the four are reported in one run, and that nothing was deployed.

**Acceptance Scenarios**:

1. **Given** the configuration default is `Pinned` and an Application manifest names no policy and the image `ghcr.io/me/web:1.2`, **When** the operator deploys, **Then** validation fails before any change, naming the application and `spec.image`, saying the value names a fixed version while the image pull policy is `Pinned`, that the policy comes from the configuration setting (named by its key), and the two ways out: set the policy on the manifest or change the default.
2. **Given** the configuration default is `Pinned` and a Resource manifest names no policy and `version: "16"`, **When** the operator deploys, **Then** validation fails naming the resource and `spec.version` with the same configuration-sourced ending.
3. **Given** the configuration default is `Pinned` and a Resource manifest names no policy and an image override with a fixed tag, **When** the operator deploys, **Then** validation fails naming the resource and `spec.image`, with the configuration-sourced ending, and the mistake is reported once.
4. **Given** the configuration default is `Pinned` and a manifest names `Pinned` itself and a fixed version, **When** the operator deploys, **Then** the message is the manifest-sourced message of T3, unchanged: it does not mention the configuration setting, because the policy did not come from it.
5. **Given** several manifests in one deploy set break the rule, **When** the operator deploys, **Then** all of them are named in one run, together with any other validation errors, and zero containers or networks are created, changed, or removed.
6. **Given** a failing manifest, **When** the operator takes either way out (adds `spec.imagePullPolicy: IfNotPresent` to it, or changes the default), **Then** the next deploy accepts it.

---

### User Story 3 - The other two values work as a default too, and no setting changes nothing (Priority: P3)

An operator may prefer a house rule of `IfNotPresent` or `Always`. Either value, set as the default, applies to every manifest that names no policy, and the manifest's own field still wins. An installation with no setting behaves exactly as today: the derived rule, `Always` for `latest` or no tag and `IfNotPresent` for any other tag, applies to manifests that name no policy. An invalid value in the configuration file is refused when the file is read, naming the setting and the three accepted values, before any command does anything.

**Why this priority**: PRD R-06 and decision D3. The precedence has three layers and all three values, not only `Pinned`, must flow through the middle layer; and the empty setting is the compatibility promise that lets existing installations upgrade without change.

**Independent Test**: Deploy the same fixture set under each of the three defaults and with no setting, and compare per-artifact effective policies with the expected table; start any command with an invalid value and verify the refusal.

**Acceptance Scenarios**:

1. **Given** the configuration default is `IfNotPresent` and an Application manifest names no policy and the image `web:latest`, **When** the operator deploys, **Then** the artifact's effective policy is `IfNotPresent` (where the derived rule would have given `Always`), the local image is reused when present, and the deploy output reports it as manifest-owned.
2. **Given** the configuration default is `Always` and an Application manifest names no policy and the image `web:1.2`, **When** the operator deploys, **Then** the effective policy is `Always` and the image is pulled on every deploy.
3. **Given** the configuration default is `IfNotPresent` or `Always` and a Resource manifest names no policy and no `version`, **When** the operator deploys, **Then** validation fails with today's "version is required" message, because only `Pinned` makes the version optional.
4. **Given** the configuration default is `IfNotPresent` or `Always` and a manifest names `Pinned`, **When** the operator deploys, **Then** the manifest's `Pinned` applies and the artifact pins.
5. **Given** no configuration default (the key absent, or an empty value), **When** the existing fixtures are deployed, **Then** every artifact's effective policy is what it was before this feature and the existing integration suites pass without edits to their assertions.
6. **Given** the configuration file sets the default to a value other than the three (for example `pinned` in lower case, or `Never`), **When** the operator runs any command that reads the configuration, **Then** the command fails before doing anything, with a message naming the setting and the three accepted values.

---

### User Story 4 - Generated manifests follow the house rule (Priority: P4)

An operator who has set the default to `Pinned` runs `shrine generate application` or `shrine generate resource` and gets a manifest that is valid under that rule: the application's image names only the repository, with no tag, and the resource skeleton names no version. Under the other two defaults, or with no setting, the generated manifests are what they are today. The generated manifest never writes a policy line of its own, so it keeps following the configuration default, and deploying it right away works.

**Why this priority**: PRD R-08. Without it, the first thing a new user of the house rule does, generating a manifest, produces a file the very next deploy rejects.

**Independent Test**: Generate one application and one resource under each of the three defaults and with no setting; load each under the default it was generated with and verify it validates; under `Pinned`, deploy the generated pair and verify both pin.

**Acceptance Scenarios**:

1. **Given** the configuration default is `Pinned`, **When** the operator runs `shrine generate application web` with no image flag, **Then** the generated manifest's image is `web`, not `web:latest`, and it has no policy line.
2. **Given** the configuration default is `Pinned`, **When** the operator runs `shrine generate resource db` with no version flag, **Then** the generated manifest has no `version` line and no policy line.
3. **Given** the configuration default is `IfNotPresent`, `Always`, or absent, **When** the operator generates an application and a resource with no image or version flag, **Then** the manifests are byte for byte what today's command produces: image `web:latest`, resource version `"16"`.
4. **Given** manifests generated under each of the three defaults and under no setting, **When** each is validated under the default it was generated with, **Then** each is accepted.
5. **Given** the configuration default is `Pinned`, **When** the operator generates an application and a resource and deploys them, **Then** both validate, both are pinned on that deploy, and the output says so.
6. **Given** the configuration default is `Pinned`, **When** the operator passes an explicit image or version on the command line, **Then** the generated manifest carries the value exactly as given; a fixed version so written is caught by the next deploy with the configuration-sourced message of User Story 2.
7. **Given** a manifest generated under a `Pinned` default, **When** the setting is later removed and the manifest is deployed, **Then** the application follows the derived rule (`Always`, because the image has no tag) and the resource fails with "version is required", because a resource without a version is valid only under `Pinned`.

---

### User Story 5 - The documentation explains the setting and the precedence (Priority: P5)

An operator reading the configuration documentation learns that the configuration file accepts a default image pull policy, which three values it takes, that an absent setting means the derived rule, and that setting it to `Pinned` makes manifests with a fixed version and no policy fail until they choose. An operator reading the manifest reference learns the full precedence: the manifest's own field, then the configuration default, then the derived rule. A contributor reading the project's own reference finds the key in the configuration layout. The reference pages for the generate commands say how the defaults depend on the setting.

**Why this priority**: PRD R-29 and the precedence part of R-28, plus the repository's doc-and-code drift rule. Last because it documents the other four stories.

**Independent Test**: Read the configuration documentation, the manifest reference, and the contributor reference after the change and answer, from the text alone: what the key is called, what its three values are, what happens when it is absent, what happens to a fixed-version manifest under a `Pinned` default, and in which order the three policy sources are consulted.

**Acceptance Scenarios**:

1. **Given** the configuration documentation, **When** an operator reads the configuration key table and example, **Then** the default image pull policy key is listed with its three values, the meaning of an absent setting, and the consequence of a `Pinned` default for manifests that name a fixed version and no policy, with the two ways out.
2. **Given** the manifest reference, **When** an operator reads the image pull policy subsection, **Then** it states the precedence in order: manifest field, configuration default, derived rule; and shows the configuration-sourced error text beside the manifest-sourced one.
3. **Given** the contributor reference, **When** a contributor reads the configuration layout, **Then** the key appears in the example configuration with its values and its default.
4. **Given** the generate command reference pages, **When** an operator reads the image and version flag descriptions, **Then** they say the default depends on the configuration's image pull policy and what it is under `Pinned`.

---

### Edge Cases

- The configuration default is `Pinned` and a manifest names `Pinned` with a fixed version: the manifest-sourced message of T3 is used, not the configuration-sourced one, because the manifest's field is the source. The two messages never both appear for one field.
- The configuration default is `Pinned` and a Resource names no policy, no `version`, and an image override without a tag: accepted; the repository is the override.
- The configuration default is `Pinned` and a Resource names no policy and a digest reference as its image override: rejected with the configuration-sourced Application-shaped message, since a digest is a fixed version.
- The configuration default is `Pinned`, and a Resource with no policy and `version: latest`: accepted and pinned, as `latest` is not a fixed version.
- The configuration default is changed from `Pinned` to `IfNotPresent` or `Always`, or removed, while artifacts it pinned are deployed: on the next deploy each such artifact is manifest-owned, so its pin is released as ticket T3 releases a pin on any manifest-owned deploy; a Resource that relied on `Pinned` to omit its version now fails with "version is required". Changing the default is as consequential as editing every manifest that names no policy, and the documentation says so.
- The configuration default is changed from `IfNotPresent` or `Always` to `Pinned`: artifacts with no policy and no fixed version pin on their next deploy as a first deploy; artifacts with a fixed version and no policy fail with the configuration-sourced message until they choose.
- The configuration file is absent entirely: no default, derived rule, as today.
- The key is present with an empty value (`imagePullPolicy:` with nothing after it, or `""`): treated as absent.
- The value differs only in case or whitespace from an accepted one (`pinned`, `Pinned `): rejected on load with the three accepted values; matching is exact, as it is for the manifest field.
- The operator passes `--path` to deploy, apply, or generate: the path override changes where manifests are read or written, never the policy default, which always comes from the configuration file in use.
- The operator points at a different configuration directory with `--config-dir` or the environment variable: the default is read from that configuration, so two installations on one host can have different house rules.
- A deploy scoped to one team under a `Pinned` default: the default applies to that team's manifests; other teams' pins are untouched, as in T3.
- `shrine generate team` is unaffected: a Team manifest has no image and no policy.
- The generate commands run with no configuration file: no default, today's skeletons.
- The configuration default is `Pinned` and the operator generates an application with an explicit image `web:latest`: written as given; it is valid under `Pinned` because `latest` is not a fixed version.
- The generated resource skeleton under `Pinned` omits the version line rather than writing `version: latest`, matching design section 4.10 and the hand-written pinned example in the manifest reference.
- A manifest-sourced policy of `IfNotPresent` or `Always` on a Resource with no `version` under a `Pinned` default: "version is required", because the manifest's own policy is manifest-owned.
- Dry run under any default writes nothing, as in T3; it prints the previews the default implies.

## Requirements *(mandatory)*

### Functional Requirements

Identifiers in brackets bind each requirement to the design's T4 list and to the PRD.

- **FR-001** [T4-01, R-06, TD-10]: The configuration file MUST accept an optional top-level default image pull policy, keyed `imagePullPolicy`, taking exactly the three values the manifest field takes: `Always`, `IfNotPresent`, `Pinned`. An absent key or an empty value MUST mean "no default" and leave the derived rule in force, so an installation that does not opt in sees no change.
- **FR-002** [T4-01, R-06]: Any other value MUST be refused when the configuration is read, before any command acts, with a message naming the key and the three accepted values. Matching MUST be exact, as it is for the manifest field.
- **FR-003** [T4-02, R-04, TD-7]: The effective policy of every artifact MUST be decided in this order: the manifest's own field when named; else the configuration default when set; else the derived rule (`Always` for `latest` or no tag, `IfNotPresent` for any other tag). The same order MUST apply to deploy, dry run, team-scoped deploy, and single-manifest apply, because they all plan the same way.
- **FR-004** [T4-02, R-07, R-02]: Under a configuration default of `Pinned`, every manifest that names no policy MUST be held to the no-fixed-version rule of T3 (an Application image with no tag or `latest`; a Resource with no `version` or `latest`, and an image override following the Application rule; a digest reference is a fixed version), and a Resource that names no policy MAY omit `version`, exactly as a manifest that names `Pinned` itself.
- **FR-005** [T4-03, R-07]: When a no-fixed-version violation is found on an artifact whose policy came from the configuration default, the error MUST name the artifact by kind and name and the field (`spec.image` or `spec.version`), say the value names a fixed version while the image pull policy is `Pinned`, say the policy came from the configuration setting by naming its key, and name the two ways out: set the policy on that manifest, or change the default. When the policy came from the manifest's own field, the message of T3 MUST be used unchanged. All such errors MUST be reported together with the manifests' other validation errors, before any change is made.
- **FR-006** [T4-02, R-06]: Under a configuration default of `IfNotPresent` or `Always`, every manifest that names no policy MUST take that value as its effective policy, with today's pull semantics for that value; a Resource that names no policy MUST still require `version`, rejected with today's message.
- **FR-007** [T4-02, TD-6]: An artifact whose effective policy moves from `Pinned` to a manifest-owned value because the configuration default changed MUST release its pin on the next deploy, exactly as a manifest edit to a manifest-owned policy does today. An artifact whose effective policy moves to `Pinned` because the default changed MUST pin on its next deploy as a first deploy.
- **FR-008** [T4-04, R-08]: `shrine generate application` MUST default the image to the bare repository `<name>` when the effective default is `Pinned`, and to `<name>:latest` otherwise.
- **FR-009** [T4-04, R-08]: `shrine generate resource` MUST omit the `version` line when the effective default is `Pinned`, and write today's default version otherwise.
- **FR-010** [T4-04, R-08]: Generated Application and Resource manifests MUST NOT write a policy line of their own, so they keep following the configuration default; and the manifest each generate command produces with no image or version flag MUST validate under the default it was generated with.
- **FR-011** [T4-04]: An image or version the operator passes explicitly on the generate command line MUST be written exactly as given under every default; generate MUST NOT silently alter or drop it. A fixed version so written under a `Pinned` default is caught by the next deploy under FR-005.
- **FR-012** [M2]: With no configuration default set, every behaviour, output line, and recorded state MUST be exactly what it was after ticket T3; the existing integration suites MUST pass without edits to their assertions.
- **FR-013** [T4-05, R-29]: The configuration documentation MUST document the key, its three values, the meaning of an absent setting, and the consequence of a `Pinned` default for manifests that name a fixed version and no policy, including the two ways out and the effect of changing the default on pins and on Resources without a version.
- **FR-014** [R-28]: The manifest reference MUST state the three-layer precedence of FR-003 in its image pull policy subsection and show the configuration-sourced error text beside the manifest-sourced one.
- **FR-015** [T4-05]: The contributor reference's configuration layout MUST show the key, and the generate command reference pages MUST describe how the image and version defaults depend on the setting; the generated CLI pages MUST be regenerated from the command help so they do not drift.
- **FR-016**: The integration suite MUST cover the three defaults against fixture manifests (effective policy per artifact, acceptance and rejection, the configuration-sourced message) and generate followed by deploy under `Pinned`, written before the implementation and run in CI.

### Key Entities

- **Configuration default**: an optional, installation-wide image pull policy, one of the three values, read from the configuration file in use. It is the middle layer of the precedence and is never written into manifests or state.
- **Effective policy**: the policy an artifact is actually planned and deployed under, computed once at plan time (T3) from three sources in order: the manifest's field, the configuration default, the derived rule. Everything downstream, including pin lifecycle and output wording, sees only the effective value.
- **Policy source**: which of the three layers supplied the effective policy for an artifact. It decides the wording of a no-fixed-version error (manifest-sourced versus configuration-sourced) and nothing else.
- **Derived rule** (existing): `Always` for `latest` or no tag, `IfNotPresent` for any other tag; the bottom layer, used when neither manifest nor configuration names a policy.
- **Generated skeleton** (existing, from the generate commands): the manifest text `generate application` and `generate resource` write; this feature makes its image and version defaults depend on the configuration default and keeps it free of a policy line.
- **Pin** (existing, from T3): unchanged in shape; this feature only changes which artifacts hold one, by changing which artifacts are effectively `Pinned`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With the default set to `Pinned`, a manifest set with no policies and no fixed versions pins every artifact on its first deploy and reuses every pin on the second, and the pins match what the same manifests produce when each names `Pinned` itself.
- **SC-002**: With the default set to `Pinned`, every manifest in a set that names a fixed version and no policy is named in one deploy run, with its field and the configuration key in the message, and zero containers or networks are created, changed, or removed (PRD metric M5 shape).
- **SC-003**: For the same fixture set, the per-artifact effective policy under each of the four settings (`Pinned`, `IfNotPresent`, `Always`, absent) matches the precedence table of FR-003 for every artifact, and manifests with their own field keep it under all four.
- **SC-004**: With no setting, the existing integration suites pass without edits to their assertions, and the deploy output of the existing fixtures is unchanged (PRD metric M2).
- **SC-005**: Manifests generated with no image or version flag validate under the default they were generated with, for all four settings; under `Pinned`, a generated application and resource deploy and pin on the first attempt.
- **SC-006**: An invalid configuration value stops every command before it acts, and the message alone tells the operator the key to fix and the three values it accepts.
- **SC-007**: From the configuration documentation and the manifest reference alone, a reader can name the key, its three values, the meaning of an absent setting, the order of the three policy sources, and the two ways out of a configuration-sourced error.

## Assumptions

- **Scope**: this ticket delivers the configuration key and its validation, the middle layer of the precedence in every planning path, the configuration-sourced error wording, the generate defaults, and the documentation named in FR-013 to FR-015. Out of scope: anything per artifact beyond precedence; pins in `get`, `describe`, and `status` (T5); `shrine bump` (T6); `shrine delete resource` (T7); the operator guide (T8). No new command or flag is added; CLI pages change only where flag descriptions change.
- **Key name and placement**: `imagePullPolicy`, top level, beside `specsDir`, `teamsDir`, `registries`, and `plugins` (design TD-10, PRD OD-2). Empty and absent are equivalent. Validation happens when the configuration is loaded, so every command that reads the configuration is covered, including the generate commands, which already read it for the specs directory.
- **Validation message**: `imagePullPolicy: must be one of Always, IfNotPresent, Pinned` (design section 3.2); the exact string is fixed in the plan's contract.
- **Configuration-sourced error text**: the manifest-sourced messages of T3 keep their text, and the configuration-sourced variants take the shape of design section 4.5: `resource "db": spec.version "16" names a fixed version but the image pull policy is Pinned (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default`, and the Application-shaped equivalent for `spec.image`, including a Resource image override. Because the configuration-sourced message names two ways out and the manifest-sourced one names the repository forms to use, the two differ in their ending only. Exact strings are fixed in the plan's contract.
- **Where the precedence lives**: T3 already computes the effective policy once at plan time from the manifest field, a default-policy input, and the derived rule, with the default input left empty (design TD-7, section 4.5). This ticket supplies the configuration value as that input from every planning path and adds knowledge of the policy's source to the validation pass. No second place computes policy.
- **Pin release on a changed default**: follows from T3's rule that any manifest-owned deploy releases a pin (design TD-6); this spec states the consequence (FR-007) and asks the documentation to warn about it, but adds no new mechanism.
- **Generate and explicit flags**: the design says the defaults change under `Pinned` and the templates are otherwise unchanged (section 4.10). This spec reads that as: only the defaults follow the house rule, and an explicit `--image` or `--version` is honoured verbatim, because an explicit flag is the operator's instruction and the next deploy already reports a contradiction loudly (FR-011). The alternative, refusing a fixed version at generate time, is a UX call the owner may make in clarification.
- **Generate output under `Pinned`**: the application image is `<name>` (no tag) and the resource skeleton has no `version:` line rather than `version: latest`, matching design section 4.10. No `imagePullPolicy:` line is written under any default.
- **Documentation locations**: the configuration file is documented in the README's Configuration section (key table and example), which is where FR-013 lands; the precedence goes into the manifest reference's existing image pull policy subsection (FR-014); the configuration layout block in the contributor reference gains the key (FR-015); the generate command pages are regenerated from the command help after the flag descriptions change.
- **Integration scenarios**: per the project's rules, authored first, compile-checked locally, and run only in CI. The three-defaults scenario can run against the existing fixtures with a configuration file per case; the generate-then-deploy scenario under `Pinned` reuses the loopback registry fixture T3 introduced where it needs a pin to resolve.
- **Dependencies**: ticket T3 (spec 033, merged in #62) provides the `Pinned` value, plan-time normalisation with the default-policy input, the no-fixed-version rule, and the pin lifecycle this feature relies on. Shares with its wave only the configuration layout lines of the contributor reference, which T6 and T7 also edit.
