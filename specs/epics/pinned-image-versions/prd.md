# PRD: Pinned Image Versions

**Status**: Draft for review
**Owner**: Carlos (product)
**Created**: 2026-10-06
**What follows this document**: a ticket breakdown, a delivery plan, GitHub issues, and then one numbered spec per ticket under `specs/NNN-<name>/`, each implemented by its own agent.

## 1. Problem

Shrine deploys whatever image reference a manifest names, which gives an operator exactly two behaviours:

- **`latest`, or no tag at all.** Shrine pulls on every deploy, so the running version can change under the operator's feet. A routine redeploy to fix an env var can silently become a major upgrade.
- **A fixed tag such as `postgres:16`.** Shrine reuses the local image when it is present. The version is only as fixed as the local image cache: an image prune, a Docker reinstall, or a fresh host pulls whatever that tag points at that day. A tag that looks exact is not.

What operators actually want for most services is a third behaviour: take the newest version the first time, then keep exactly that version until I decide to move. Two gaps sit next to that one. No command answers "which version is deployed?" without going to Docker by hand. And the only way to move one artifact to a new version is to edit its manifest, which is the wrong tool when the manifest deliberately says "newest".

## 2. Goals

- **G1. Freeze what you got.** A new image policy under which the first deploy takes the newest version of the image and every later deploy runs exactly that version, across redeploys, container recreation, teardown and redeploy, and a wiped local image cache, for as long as the registry serves it.
- **G2. One default, per-artifact overrides.** The operator sets the default policy once in the configuration file; any manifest can override it.
- **G3. See what is deployed.** One command shows the deployed version of a single artifact, of a team, or of everything, in a form a person can read.
- **G4. Move deliberately.** A pinned artifact can be moved to a chosen version, or to the newest, without editing its manifest. The move is recorded immediately and applied on the next deploy. Rolling back is the same move in the other direction.
- **G5. Fail before touching anything.** A version that cannot be resolved stops a deploy before any container is created, changed, or removed.
- **G6. Nothing changes for anyone who does not opt in.** Existing manifests and existing installations keep today's behaviour exactly.

## 3. Non-goals

- Version ranges, semver constraints, or "bump to the next minor". Bump means set, never increment.
- Translating an exact version back to every tag a registry has for it. Shrine shows the readable version it resolved from and the date; it does not list registry tags.
- Images that were never pushed to a registry. Shrine deploys from registries only; a local-only image fails at pull with Docker's own error, as it does today.
- The gateway's own image. It is configuration, not a manifest, and keeps its current fixed default.
- Applying a bump automatically. Bump records; deploy applies.
- Showing versions of artifacts that are torn down. Their pins exist for Shrine's own use and are not displayed.
- Changing what `Always` and `IfNotPresent` mean today.
- Multi-host deployments.

## 4. Users

- **The operator.** Runs Shrine on one host, authors manifests for one or more teams, deploys, tears down, rebuilds hosts, and wants to know what is running. Comfortable with Docker but does not want to inspect containers by hand to answer basic questions.
- **A future automation agent.** Reads this document and the specs derived from it to implement one ticket at a time. Requirements are numbered so tickets and specs can trace to them.

## 5. Vocabulary

| Term | Meaning |
|---|---|
| Repository | The image name without a version: `postgres`, `ghcr.io/me/hello-api`, `reg:lab/hello-api`. |
| Version | The readable part of an image reference, the tag: `16`, `latest`, `v1.4.0`. A Resource names it in `spec.version`; an Application names it inside `spec.image`. |
| Exact version | The registry digest of an image, `sha256:…`. Immutable. Two pulls of the same exact version yield identical bytes. |
| Policy | The image pull policy on a manifest, today `Always` or `IfNotPresent`, after this work also the third value introduced here. |
| Manifest-owned version | The version is whatever the manifest names. `Always` and `IfNotPresent` are manifest-owned. |
| Shrine-owned version | The manifest names no fixed version; Shrine resolves one, records it, and keeps it. Only the third policy is Shrine-owned. |
| Pin | Shrine's record of a Shrine-owned version: the exact version, the readable version it was resolved from, and the date. |
| Bump | Replacing an artifact's pin with a new one. |

Working name for the third policy value in this document: **`Pinned`**. See open decision OD-1.

## 6. User journeys

Each journey is written from the operator's chair and ends with what "done" looks like.

**J1. Adopt a service at newest, then freeze it.**
I write a manifest for a new app, name only the repository, set the policy to Pinned, and deploy. Shrine pulls the newest image, deploys it, and tells me the exact version it chose. A week later the upstream project publishes a new `latest`. I redeploy to change an env var. The app comes back on the same exact version as before. Done when: two deploys a week apart run the same exact version even though `latest` moved.

**J2. Rebuild the host.**
I run an aggressive Docker prune, or I reinstall Docker, and deploy everything again. Every pinned artifact comes back on the exact version it had before. Done when: no pinned artifact changed version because of the rebuild.

**J3. Audit what is running.**
I ask Shrine what is deployed for one team. For each app and resource I see the version in a form I can read, and for pinned ones the exact version next to it. I ask about a single resource and also see when its version was pinned and whether a newer pin is waiting to be deployed. Done when: I never open the Docker CLI to answer "which version?".

**J4. Upgrade one database on purpose.**
Postgres 17 is out. I bump the resource to version 17. Shrine checks the version exists in the registry, records it, and tells me the old and new exact versions. Nothing restarts yet. I deploy when I am ready, and only that resource is recreated. Done when: upgrade is two commands, and a typo in the version is rejected by the bump, not discovered mid-deploy.

**J5. Roll back.**
The upgrade in J4 went badly. I bump the resource back to the previous version, or to the exact version Shrine showed me before, and deploy. Done when: rollback uses the same command as upgrade.

**J6. Take the newest again.**
Months later I want whatever is newest for an app. I bump it with no version. Shrine resolves the newest, records it, and the next deploy applies it. Done when: "newest" is an explicit, recorded act, never a side effect.

**J7. Make pinning the house rule.**
I set the default policy to Pinned in the configuration file. Manifests that name no policy are now pinned. The next deploy tells me which existing manifests still name a fixed version and must choose: add a policy to the manifest or drop the fixed version. Done when: the house rule is one line of config, and the failure to comply is loud and specific.

**J8. Retire an artifact.**
I tear a team down, change my mind, and deploy it again: pins are kept, so every artifact comes back on the same version. Later I delete one resource for good. Its pin goes with it, so a future artifact with the same name starts fresh. Done when: teardown keeps pins and delete releases them.

## 7. Requirements

Identifiers are stable; tickets and specs cite them.

### 7.1 Policy and manifest

- **R-01.** The image pull policy field on Application and Resource manifests accepts a third value. The existing two values keep their current meaning exactly.
- **R-02.** Under the third policy the manifest must not name a fixed version. An Application image has no tag or the tag `latest`. A Resource omits `version` or sets it to `latest`; a Resource `image` override, when present, follows the Application rule. A violation is a validation error, reported together with the manifest's other errors, naming the artifact and the field.
- **R-03.** Under the third policy a Resource's `version` is optional. Under the other two it stays required, as today.
- **R-04.** The effective policy of an artifact is decided in this order: the manifest's own field, then the configuration default, then today's derived rule, which is `Always` for `latest` or no tag and `IfNotPresent` for any other tag.
- **R-05.** Only artifacts whose effective policy is the third value can be bumped. For the other two the version is manifest-owned, and bump refuses with a message that says so and names the manifest.

### 7.2 Configuration default

- **R-06.** The configuration file accepts a default image pull policy with the same three values. When the setting is absent the derived rule of R-04 applies, so existing installations see no change.
- **R-07.** When the default is the third value, every manifest that names no policy is held to R-02. Existing manifests that name a fixed version then fail validation with a message that names the configuration setting and the two ways out: add a policy to that manifest, or change the default.
- **R-08.** Manifests produced by `shrine generate application` and `shrine generate resource` are valid under the effective default. Under the third value they name no fixed version.

### 7.3 Pinning on deploy

- **R-09.** The first deploy of an artifact under the third policy resolves the newest version of the manifest's repository, deploys it, and records a pin: the exact version, the readable version it was resolved from, and the date.
- **R-10.** Every later deploy of a pinned artifact runs the pinned exact version. This holds across a plain redeploy, a redeploy that recreates the container, teardown followed by deploy, a wiped local image cache, and any combination of these, for as long as the registry serves that exact version.
- **R-11.** Pins outlive containers and teardown. Only `delete application`, `delete resource`, and `delete team` release them. Redeploy, container recreation, and teardown never release a pin.
- **R-12.** A deploy under a manifest-owned policy makes the artifact manifest-owned again; the next deploy under the third policy behaves as a first deploy and records a fresh pin.
- **R-13.** When the pinned exact version is present locally, deploy makes no registry call for it. When it is absent, deploy fetches that exact version from the registry, never the tag.
- **R-14.** Before any container is created, changed, or removed, deploy resolves or verifies the image of every artifact in the deploy set. If any one fails, the deploy stops with zero changes made. The message names the artifact and the reference. When a pinned exact version is no longer served by the registry, the message says so and points at bump.
- **R-15.** Deploy output states, per artifact, the exact version deployed and whether it was pinned on this deploy, reused from an existing pin, or manifest-owned.
- **R-16.** Dry run prints, per artifact under the third policy, either that it would resolve the newest version and pin it, or the existing pin it would reuse. Dry run writes nothing; repeating it leaves recorded state unchanged.

### 7.4 Querying versions

- **R-17.** `shrine get deployed`, `shrine get applications`, and `shrine get resources`, with and without `--team`, gain a version column. For a pinned artifact it shows the readable version and a short form of the exact version. For a manifest-owned artifact it shows the version the manifest named at deploy time. These commands keep working without access to Docker.
- **R-18.** `shrine describe app <name>` and `shrine describe resource <name>` show the effective policy, the repository, the version the manifest names, the pin when there is one with its readable version, exact version, and date, and the image the running container was started from, so a bump that is recorded but not yet deployed is visible as a difference.
- **R-19.** Pins of artifacts that are not deployed are never displayed.
- **R-20.** Should-have, see OD-3: `shrine status` shows the running image next to the running state.

### 7.5 Bumping

- **R-21.** New command: `shrine bump application <name>` and `shrine bump resource <name>`, with the `app` and `res` aliases the other verbs use, the optional `--team` with the usual automatic search and ambiguity error, `--dry-run`, and `--path` to name the manifest directory as deploy does.
- **R-22.** `-v` names the target: a readable version or an exact version. The repository always comes from the manifest, so a bump can never point an artifact at a different image. Without `-v`, bump resolves the newest version.
- **R-23.** Bump resolves immediately. It verifies that the target exists in the registry and records the new pin, and it fails with a clear message when the target does not exist. It never starts, stops, or recreates a container; the next deploy applies the pin. Output states the previous and the new version.
- **R-24.** Bump applies to any artifact under the third policy whose manifest is in the manifest directory, deployed or not, so an operator can choose the version of a first deploy in advance. It refuses manifest-owned artifacts per R-05 and unknown artifacts with a message naming the directory searched.
- **R-25.** `--dry-run` prints what bump would resolve and record, and writes nothing.
- **R-26.** Rolling back is a bump to an earlier readable or exact version. There is no separate rollback command.

### 7.6 Deleting a resource

- **R-27.** New command: `shrine delete resource <name>`, mirroring `shrine delete application`: it refuses while the resource's container exists, forgets the deployment record, and releases the pin. It supports `--team` and `--dry-run`. `shrine delete application` and `shrine delete team` also release pins.

### 7.7 Documentation

- **R-28.** The manifest reference documents the three policy values, the no-fixed-version rule, the precedence of R-04, and the pin lifecycle.
- **R-29.** The configuration documentation documents the default policy setting and the consequence described in R-07.
- **R-30.** The CLI reference includes pages for bump and delete resource.
- **R-31.** A guide, working title "Managing image versions", walks through journeys J1 to J8 with the product's real output.
- **R-32.** The project's own reference for contributors reflects the new deploy step and the new recorded state, per the repository's doc and code drift rule.

## 8. Surface summary

| Surface | Change |
|---|---|
| Manifest, Application and Resource | `spec.imagePullPolicy` accepts a third value; under it, no fixed version may be named and Resource `spec.version` becomes optional. |
| Configuration file | New default image pull policy setting. Absent means today's derived rule. |
| `shrine deploy`, `shrine deploy team`, `shrine apply -f` | Pre-deploy version check; pinning; new output lines; dry-run preview of pins. |
| `shrine get deployed`, `get applications`, `get resources` | Version column. |
| `shrine describe app`, `describe resource` | Policy, manifest version, pin, running image. |
| `shrine bump application`, `bump resource` | New. |
| `shrine delete resource` | New. `delete application` and `delete team` also release pins. |
| `shrine generate application`, `generate resource` | Templates follow the effective default. |
| Docs site and contributor reference | Per R-28 to R-32. |

## 9. Success metrics

- **M1.** Across ten deploys of a pinned artifact that include at least one container recreation, one teardown and redeploy, and one wiped image cache, the exact version is identical ten out of ten times.
- **M2.** With no configuration default set and no manifest using the new value, the existing integration suite passes unchanged, and deploy output for existing fixtures is unchanged.
- **M3.** An operator answers "which version is deployed?" for one artifact, one team, or everything with one command each and no Docker CLI.
- **M4.** Upgrading one artifact is two commands. A version that does not exist is rejected by the bump with zero containers touched.
- **M5.** With one unresolvable reference among any number of artifacts, a deploy creates, changes, or removes zero containers.
- **M6.** Repeated dry runs leave recorded state byte for byte unchanged.
- **M7.** An operator with no prior knowledge can answer, from the documentation alone: how to pin, what happens on prune, how to see versions, how to upgrade, how to roll back, and what the configuration default changes.

## 10. Compatibility and rollout

- Purely additive. No manifest, configuration file, or recorded state needs migration.
- The new policy value and the configuration default are opt-in. The derived rule stays the fallback, so a `latest` image keeps floating until someone decides otherwise.
- Recorded state gains new information: the pins, and the version a manifest-owned artifact was deployed with. Older recorded state without that information is read as "no pin" and "version unknown".
- No change to teardown semantics beyond pins being kept.

## 11. Risks and open decisions

Open decisions for the product owner:

- **OD-1. Name of the third policy value.** Proposed `Pinned`. Alternatives considered: `Managed`, `Locked`. The value sits beside `Always` and `IfNotPresent`, so it should read as a mode, not as a pull timing.
- **OD-2. Name of the configuration setting.** Proposed `imagePullPolicy` at the top level of the configuration file, mirroring the manifest field.
- **OD-3. Whether `shrine status` gains the running image.** Proposed yes as a should-have; it is the only live view and makes drift after a bump visible without `describe`.
- **OD-4. Short form of the exact version in tables.** Proposed the first twelve characters of the digest, matching how container ids are shortened today.
- **OD-5. Team-wide or all-artifacts bump.** Proposed out of scope; bump is per artifact.

Risks:

- **Registry retention.** A registry may garbage-collect an exact version that is no longer tagged. The pin then cannot be honoured; R-14 makes this a clean pre-deploy failure that points at bump. Operators who rely on long-lived pins need registries that keep untagged manifests.
- **Architecture moves.** When a registry publishes a multi-architecture image, the pin is the multi-architecture index, so moving to a host with a different CPU keeps working. A single-architecture image pins a single-architecture exact version and will not run elsewhere, which is already true today.
- **Readable versions can mislead.** `latest@sha256:3f2a…` says what was asked for and what was obtained, not what the registry calls it now. The date in the pin is there to make this honest.
- **A house rule of Pinned fails existing manifests on purpose.** R-07 is loud by design; the ticket for it must get the message right.

## 12. Decisions already taken

Recorded from the product discussion so later work does not reopen them.

- **D1.** The pin is the registry digest. The readable context, the version it was resolved from and the date, lives in the same record. There is no separate digest-to-version map.
- **D2.** The third behaviour is a third value of the existing image pull policy, not a new field. The three modes are mutually exclusive, so one field has no invalid combinations. The two existing values are untouched.
- **D3.** The configuration default exists, and when absent the derived rule of today applies. A flat default of `IfNotPresent` was rejected because it would silently stop `latest` from floating for existing users.
- **D4.** Version problems are caught in a step before any container is touched. The deploy is fail-fast, not transactional: it does not roll back, and this feature does not change that.
- **D5.** No new command noun for querying. The existing `get` tables and `describe` gain the version. Pins of torn-down artifacts are internal.
- **D6.** Shrine deploys from registries only. Local-only images are not a supported case.
- **D7.** `shrine delete resource` is added for symmetry with `delete application`.
- **D8.** Bump resolves immediately and records a fully resolved pin; it never touches containers.
- **D9.** Bump sets a version; it never increments one.
- **D10.** The manifest never names a version under the third policy, so there is no conflict between a manifest edit and a bump to resolve.

## 13. Delivery shape

Input to the ticket breakdown, not the breakdown itself. Each slice must be testable on its own and leave the product shippable.

1. Policy value, validation rules, and configuration default, including the generate templates. Visible as validation behaviour before any pinning exists.
2. Pinning on deploy with the pre-deploy check, the pin record, deploy output, and dry run. This is the minimum that delivers G1 and G5.
3. Version in `get` and `describe`, which needs deploy to record the version of manifest-owned artifacts too.
4. Bump.
5. Delete resource and pin release on every delete.
6. Documentation and the guide, which can start as soon as slice 2 is stable.
