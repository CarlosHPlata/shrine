# Implementation Plan: A Configuration Default for the Image Pull Policy, and Manifests Generated to Match

**Branch**: `034-pull-policy-config-default` | **Date**: 2026-10-06 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/034-pull-policy-config-default/spec.md`
**Epic binding**: [design.md](../epics/pinned-image-versions/design.md) sections 3.2, 4.5, 4.10; decisions TD-6, TD-7, TD-10; requirement list T4-01 to T4-05. Decisions TD-1 to TD-13 are settled and are not reopened here; the three places where this plan refines the design are listed in [contracts/docs-and-deviations.md](contracts/docs-and-deviations.md) and are recorded in `design.md` with the pull request.

## Summary

Ticket T3 built the whole precedence machine and left its middle layer empty: `planner.Plan` already takes a `defaultPullPolicy` and `applyEffectivePullPolicy` already prefers the manifest field, then that default, then the derived rule, and the three planning handlers pass `""`. This ticket supplies the value. `config.Config` gains a top-level `ImagePullPolicy` field, validated in `Load` against the three manifest values so every command refuses a bad setting before it acts, and the three handlers pass `cfg.ImagePullPolicy` where they pass `""` today. Because normalisation overwrites the manifest field, the planner records where each artifact's policy came from while it normalises, and the fixed-version validation uses that record to choose between T3's manifest-sourced message and the new configuration-sourced one that names the key and the two ways out. The generate commands move their image and version defaults out of `cmd/` into the handler, where the effective default decides them: under `Pinned` the application image is the bare `<name>` and the resource skeleton has no `version:` line; otherwise the skeletons are byte for byte what they are today, and no skeleton ever writes a policy line. Documentation gains the key (README configuration table, manifest reference precedence and error text, `AGENTS.md` layout) and the generate CLI pages are regenerated from the changed flag help. Two new integration scenarios, written first, cover the three defaults against new fixture directories by dry run and generate-then-deploy under `Pinned` against the loopback registry T3 introduced.

## Technical Context

**Language/Version**: Go 1.25 (`go.mod`: `go 1.25.0`), module `github.com/CarlosHPlata/shrine`
**Primary Dependencies**: `gopkg.in/yaml.v3` (already used by `config.Load`), Cobra (two flag help strings change, one flag default changes; no new command or flag), Docker SDK untouched
**Storage**: none new. `config.yml` gains one optional top-level key; `pins.txt` and `deployments.txt` are unchanged in shape and are written by T3's code paths only
**Testing**: `go test ./...` for unit tests (struct-literal validation for config; table tests in the planner for precedence, source recording, and the four message shapes; pure skeleton rendering for generate; no filesystem); `go vet -tags integration ./tests/integration/...` to compile the suite; CI runs `make test-integration` for the two new scenarios. Integration scenarios are never run locally for this ticket.
**Target Platform**: Linux single host with a local Docker daemon; CI is `ubuntu-latest` with Docker
**Project Type**: CLI with a pluggable-backend execution engine
**Performance Goals**: none measurable; the default is one string read once per command and compared per artifact at plan time
**Constraints**: no new mechanism (constitution IV); the planner and handlers change only where T3 left the `""`; dry run stays a backend, not a branch; existing integration assertions pass unedited; installations without the key produce exactly T3's output and generate exactly today's skeletons; unit tests off the filesystem
**Scale/Scope**: homelab scale; one configuration value per installation

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] Pass: no CLI flag is added; the capability is a configuration default for an existing manifest field (PRD D3, design TD-10), the manifest field keeps precedence, and the constraint it enforces is checked at plan time as a multi-error report together with the manifests' other errors |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] Pass: no new command; `deploy --dry-run` previews the default's effect through the existing dry-run backend; the generate image and version defaults move from `cmd/generate.go` into `internal/handler/`, which is where the constitution wants business logic |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] N/A: nothing touches the engine or any backend; the policy reaches them already normalised, as after T3 |
| IV. Simplicity & YAGNI | Is every abstraction justified by three or more concrete usages? | [x] Pass: one string field, one validation method, one unexported source record on the planner's set read by one validator, two pure skeleton renderers; no policy type, no option struct, no second precedence computation |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass: `tests/integration/pull_policy_default_test.go`, written before the implementation: the precedence scenarios on `NewSuite` by dry run (no daemon needed), generate-then-deploy on `NewDockerSuite` with the loopback registry; CI executes both (ticket T4 integration scenarios) |
| VI. Docker-Authoritative State | Does state update happen after Docker operations complete? | [x] N/A: no state write is added or moved; pin writes and releases stay inside T3's `ResolveImage` |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | [x] Pass: `validateImagePullPolicy`, `recordPullPolicySource`, `isPolicyFromDefault`, `fixedVersionRemedy`, `defaultAppImage`, `defaultResourceVersion`, `renderAppSkeleton`, `renderResourceSkeleton`, `versionLine`; the dead `defaultPullPolicy` parameter of `validateImagePolicies` and `Resolve` is removed rather than left unused; comments only for WHY |

**Post-Phase-1 re-check**: all gates still pass. No behaviour changes for installations that do not set the key; the one visible change for everyone is the generate `--version` flag's help text and the disappearance of its `(default "16")` suffix, because the default now depends on the setting (research R6).

## Design binding

Every functional requirement of the spec is bound to the seam that satisfies it, so tasks can be derived mechanically.

| Spec | Design | Where |
|---|---|---|
| FR-001 | T4-01, section 3.2, TD-10, R1 | `config.Config.ImagePullPolicy` (`yaml:"imagePullPolicy,omitempty"`) |
| FR-002 | T4-01, section 3.2, R1 | `(*Config).validateImagePullPolicy` called from `Load` beside `validateSecretsPlugins`; `manifest.IsKnownPullPolicy`; root's `loading config:` wrap reaches every command |
| FR-003 | T4-02, TD-7, R2 | `Deploy`, `DryRun`, `ApplySingle` pass `cfg.ImagePullPolicy` to `planner.Plan`; `applyEffectivePullPolicy` unchanged in rule |
| FR-004 | T4-02, R-07, R3 | `validateImagePolicies` runs on the normalised set as today; the Resource version rule already keys on the effective value |
| FR-005 | T4-03, section 4.5, R3, R4 | `ManifestSet.pullPolicySources` filled by `recordPullPolicySource` inside `applyEffectivePullPolicy`; `isPolicyFromDefault`; `fixedVersionRemedy` chooses the ending; `validateImagePolicies(set)` and `Resolve(set, store, registries)` lose the dead parameter |
| FR-006 | T4-02, R2 | same threading; `IsManifestOwnedPolicy` keeps the version-required rule |
| FR-007 | TD-6, R2 | no code: T3's `resolveManifestOwned` releases and `pinNewest` pins on the effective value; covered by the generate-then-deploy scenario's final step and the quickstart |
| FR-008 | T4-04, section 4.10, R5 | `defaultAppImage(name, policy)` in `internal/handler/apps.go`; `AppOptions.PullPolicy`; `cmd/generate.go` stops defaulting the image |
| FR-009 | T4-04, section 4.10, R5, R6 | `defaultResourceVersion(policy)` and `versionLine` in `internal/handler/resources.go`; `ResourceOptions.PullPolicy`; the `--version` flag default becomes `""` |
| FR-010 | T4-04, R5 | skeleton constants keep no `imagePullPolicy:` line; `renderAppSkeleton` and `renderResourceSkeleton` are tested under the four settings |
| FR-011 | R5 | explicit `Image` and `Version` pass through the renderers untouched |
| FR-012 | M2 | existing suites untouched; `""` default reproduces T3 exactly |
| FR-013 | T4-05, R-29, R7 | `README.md` Configuration section per the docs contract |
| FR-014 | R-28, R7 | `docs/content/reference/manifest-schema.md` default column and subsection |
| FR-015 | T4-05, R7 | `AGENTS.md` Config Directory Layout; `make docs-gen-cli` regenerates `generate_application.md` and `generate_resource.md` |
| FR-016 | ticket T4 scenarios, R8 | `tests/integration/pull_policy_default_test.go`; fixtures under `tests/testdata/pull-policy-default/` |

## Project Structure

### Documentation (this feature)

```text
specs/034-pull-policy-config-default/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0: decisions with the design section that settles each, and the refinements found
├── data-model.md        # Phase 1: configuration field, policy source record, effective-policy table, generate options and skeletons
├── quickstart.md        # Phase 1: manual verification script (no daemon needed except the last step)
├── contracts/
│   ├── config-contract.md        # the key, its validation, how it reaches the planner and generate
│   ├── operator-output.md        # exact validation messages, dry-run lines, generated skeletons, flag help
│   └── docs-and-deviations.md    # documentation changes, progress entry, design refinements to record
├── checklists/requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/
├── config/
│   ├── config.go                # + ImagePullPolicy field; validateImagePullPolicy called from Load
│   └── config_test.go           # + TestValidateImagePullPolicy: empty, three values, lower case, unknown
├── planner/
│   ├── loader.go                # ManifestSet + pullPolicySources (unexported); NewManifestSet unchanged (lazy map)
│   ├── policy.go                # applyEffectivePullPolicy records the source; validateImagePolicies(set); fixedVersionRemedy; config-sourced endings
│   ├── policy_test.go           # + source recording; config-sourced messages for app, resource version, resource override; manifest-sourced unchanged when the field is set; IfNotPresent/Always defaults; hand-built sets default to manifest-sourced
│   ├── plan.go                  # Resolve(set, store, registries) call
│   ├── resolve.go               # Resolve loses defaultPullPolicy; validateImagePolicies(set)
│   └── *_test.go                # Resolve call sites drop the "" argument
├── handler/
│   ├── deploy.go                # DryRun and Deploy pass cfg.ImagePullPolicy
│   ├── apply.go                 # ApplySingle passes b.Cfg.ImagePullPolicy
│   ├── apps.go                  # AppOptions.PullPolicy; defaultAppImage; renderAppSkeleton; GenerateApp writes the rendered text
│   ├── apps_test.go             # NEW: renderAppSkeleton under the four settings; explicit image verbatim (no filesystem)
│   ├── resources.go             # ResourceOptions.PullPolicy; defaultResourceVersion; versionLine; renderResourceSkeleton
│   └── resources_test.go        # NEW: renderResourceSkeleton under the four settings; explicit version verbatim; byte-identical to today's skeleton when not Pinned
cmd/
└── generate.go                  # image default removed from the command; --version default "" with new help; --image help updated; PullPolicy: cfg.ImagePullPolicy on both option structs

tests/
├── integration/
│   └── pull_policy_default_test.go   # NEW, written first: TestPullPolicyDefaultPrecedence (NewSuite, dry run), TestPullPolicyDefaultGenerateThenDeploy (NewDockerSuite, registry)
└── testdata/pull-policy-default/
    ├── versioned/                    # NEW: app :latest no policy; app fixed tag no policy; resource version "16" no policy; app own IfNotPresent with fixed tag
    ├── pinned-shape/                 # NEW: app untagged no policy; resource no version no policy; resource own Pinned no version
    └── own-pinned-fixed/             # NEW: app own Pinned with fixed tag (manifest-sourced message under any default)

README.md                                   # Configuration: example line, table row, paragraph on the Pinned default
docs/content/reference/manifest-schema.md   # default column text; precedence sentence; config-sourced error line
docs/content/cli/generate_application.md    # regenerated
docs/content/cli/generate_resource.md       # regenerated
AGENTS.md                                   # Config Directory Layout example gains imagePullPolicy
specs/progress.md                           # entry for 034
specs/epics/pinned-image-versions/design.md # refinements recorded
```

**Structure Decision**: single-project Go layout already in place. Every change lands in the file that owns the seam T3 left: the config struct, the planner policy file, the three planning handlers, and the two generate handlers. The only new source files are the two handler test files and the integration scenario file; the only new fixtures are three small directories, split by expected outcome because one failing manifest fails a whole set.

## Deviations from the epic design

| Deviation | Why | Recorded |
|---|---|---|
| `validateImagePolicies` reads the policy's source from a record on the set instead of a `defaultPullPolicy` parameter; the parameter is removed from it and from `Resolve` | after normalisation a `Pinned` field is indistinguishable from a `Pinned` default without a record; a parameter that no code reads would be dead | research R3; `design.md` section 4.5 with the PR |
| The configuration-sourced Application message takes the Resource message's ending (`set spec.imagePullPolicy on the manifest or change the default`) rather than the manifest-sourced `use "<repo>" or "<repo>:latest"` | R-07 names the two ways out; the manifest-sourced hint names the field fix instead, which T3 kept for the manifest case | research R4; contracts |
| The generate `--version` flag default changes from `"16"` to `""` and the handler fills `16` unless the default is `Pinned`; the image default moves from the command into the handler | the handler cannot tell an explicit `--version 16` from the flag default, and constitution II places the logic in the handler | research R6 |

## Complexity Tracking

No constitution principle violations to justify.
