# Implementation Plan: The Pinned Policy: Resolve Once, Keep the Exact Version

**Branch**: `033-pinned-image-policy` | **Date**: 2026-10-06 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/033-pinned-image-policy/spec.md`
**Epic binding**: [design.md](../epics/pinned-image-versions/design.md) sections 3.1, 3.4, 3.5, 4.2, 4.4, 4.5, 4.8, 4.9, 4.11, 5; decisions TD-1, TD-2, TD-3, TD-6, TD-7, TD-8, TD-9, TD-11, TD-13; requirement list T3-01 to T3-10. Decisions TD-1 to TD-13 are settled and are not reopened here; the five places where this plan refines or deviates from the design are listed in [contracts/state-and-docs.md](contracts/state-and-docs.md) and are recorded in `design.md` with the pull request.

## Summary

Three things land together. A new per-team state file, `pins.txt`, behind `state.ImagePinStore`, copied from the host-port and deployment stores' lifecycle and file discipline, holds per artifact the expanded tag reference that was resolved, the pullable digest reference, and the UTC date. The Docker backend's `ResolveImage` grows two branches beside T2's manifest-owned path: reuse the pin, pulling by digest only when the image is absent locally, or pull the newest, pick the repository digest, and write the pin; a manifest-owned resolution releases any pin, which is what makes "return to pinned" a first deploy. The planner gains a normalisation step that writes the effective policy back into every manifest, and a validation pass that enforces the no-fixed-version rule and takes over the version-required check from parse time; `Plan` gains a `defaultPullPolicy` parameter threaded as empty by the three handlers until T4. The dry-run backend receives a read-only pin snapshot the way it receives host ports. Two terminal arms print the pinned-now and reused lines; `delete application` and `delete team` release pins. The integration gate runs a `registry:2` container the suite starts on the loopback interface, pushes two `whoami` tags as `latest` in turn, and asserts the pinned digest across ten cycles.

## Technical Context

**Language/Version**: Go 1.25 (`go.mod`: `go 1.25.0`), module `github.com/CarlosHPlata/shrine`
**Primary Dependencies**: Docker SDK `github.com/docker/docker v28.5.2` (`ImageInspect` with a digest reference for local presence, `ImagePull` by digest, `ImageTag`/`ImagePush`/`ImageRemove` in the test utilities), Cobra (untouched: no new command or flag)
**Storage**: new `<state-dir>/<team>/pins.txt` (five fields, atomic writes, forgiving reader); `deployments.txt` unchanged in shape, now records `Pinned` as a policy value
**Testing**: `go test ./...` for unit tests (in-memory file fakes for the store; `fakeDockerAPI` and a fake pin store for the backend; no filesystem); `go vet -tags integration ./tests/integration/...` to compile the suite; CI runs `make test-integration` against a real daemon and the suite-started registry. Integration scenarios are never run locally for this ticket.
**Target Platform**: Linux single host with a local Docker daemon; CI is `ubuntu-latest` with Docker, where `127.0.0.1` registries are insecure by default
**Project Type**: CLI with a pluggable-backend execution engine
**Performance Goals**: a reused pin with the image present costs one `ImageInspect` and no registry call; a first pin costs the same pull T2 performs plus one file write
**Constraints**: zero Docker or pin logic in the engine (constitution III); dry run is a backend, not a branch; existing integration assertions pass unedited; manifests that do not name `Pinned` produce exactly T2's output; unit tests off the filesystem
**Scale/Scope**: homelab scale; one pin line per pinned artifact per team

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] Pass: the capability is the third value of the existing `spec.imagePullPolicy` field (design TD-9, D2); the enum check and the no-fixed-version rule are enforced at validate and plan time, as multi-error reports, never at runtime |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] Pass: no new command; `deploy --dry-run` previews would-pin and reuse without writing (design section 4.4); `delete application --dry-run` previews the pin release |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass by construction: every pin read and write on the deploy path lives in `DockerBackend.ResolveImage`; the engine is untouched except for reading two new result fields it does not interpret; the dry-run backend's own `ResolveImage` prints the preview |
| IV. Simplicity & YAGNI | Is every abstraction justified by three or more concrete usages? | [x] Pass: `ImagePinStore` has the three readers the design names (deploy, dry run, delete) and T5 and T6 to come; the store copies an existing pattern rather than introducing one; no `Repin`, no bump, no config default; `now` is an injected function, not a clock interface |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass: `tests/integration/pinned_image_policy_test.go`, written before the implementation, on `NewDockerSuite` with a suite-started registry; CI executes it (ticket T3 integration scenarios, design TD-13) |
| VI. Docker-Authoritative State | Does state update happen after Docker operations complete? | [x] Pass: a pin is written only after the pull and inspect succeed, before any container exists, because the pin records an image, not a container; the deployment record is still written after `ContainerStart`; delete still refuses while the container exists |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | [x] Pass: `resolveManifestOwned`, `reusePin`, `pinNewest`, `usablePin`, `releasePin`, `notServedError`, `sameRepository`, `applyEffectivePullPolicy`, `validateImagePolicies`, `validatePullPolicy`, `TagOf`, `IsDigestReference`, `readableVersion`, `releaseTeamImagePins`; comments only for WHY (why the pull is by digest, why a mismatched repository re-pins) |

**Post-Phase-1 re-check**: all gates still pass. The one behaviour change for manifests that do not opt in, the `TagOf` fix to the derived rule for untagged images on a registry with a port, aligns the code with the documented rule and is listed under Deviations rather than Complexity, because it removes a special case instead of adding one.

## Design binding

Every functional requirement of the spec is bound to the seam that satisfies it, so tasks can be derived mechanically.

| Spec | Design | Where |
|---|---|---|
| FR-001 | T3-01, R7 | `manifest.ImagePullPolicyPinned`, `IsKnownPullPolicy`; `validatePullPolicy` in `validate.go` for both kinds |
| FR-002, FR-003 | T3-02, section 4.5, R6 | `applyEffectivePullPolicy` and `validateImagePolicies(set, defaultPullPolicy)` in `internal/planner/policy.go`; `Resolve` and `Plan` signatures; `validateResourceSpec` drops the version check; `parseManifest` defaults the Resource image to `<type>` without a version; `manifest.TagOf`, `IsDigestReference` |
| FR-004 | T3-02, TD-7 | `EffectivePullPolicyWithDefault`; handlers pass `""` as the default |
| FR-005 | T3-03, section 4.2 case 3b, R3 | `pinNewest` in `docker_image.go`; `pickRepoDigest`; `ImagePins.Put`; `now` |
| FR-006, FR-007 | T3-03, section 4.2 case 3a, R3 | `reusePin`: `inspectImage(pin.Pinned)`, pull by digest on not-found; `Ref` is the digest reference |
| FR-008 | T3-08, TD-2, R11 | `op.ImageID` from the pin's inspect; `configHash` unchanged |
| FR-009 | T3-04, section 4.8, R1, R10 | `internal/state/pins.go`, `internal/state/local/pins.go`, `state.Store.ImagePins`, `NewLocalStore`; `DeleteApplication`, `DeleteTeam`, `releaseTeamImagePins` |
| FR-010 | T3-04, TD-6, section 4.2 case 2 | `resolveManifestOwned` calls `releasePin` |
| FR-011 | R3 | `usablePin` compares repositories with `sameRepository`; `Put` replaces |
| FR-012 | T3-05, R8 | `notServedError`; backend-emitted `image.resolve` error; engine wrap unchanged |
| FR-013 | T3-06, section 4.9, R5 | `ImageSourceResolved`, `ImageSourcePinned`, `ResolvedImage.Requested`/`PinnedAt`; two arms in `terminal_logger.go`; `readableVersion` |
| FR-014 | T3-07, section 4.4, R9 | `DryRunContainerBackend.Pins`; `NewDryRunEngine(out, hostPorts, pins)`; `handler.DryRun` snapshot; three line shapes |
| FR-015 | M2, G6 | existing suites untouched; T2's lines unchanged; manifest-owned path identical apart from the idempotent release |
| FR-016 | T3-09, section 4.11, R14 | `docs/content/reference/manifest-schema.md` per the state-and-docs contract |
| FR-017 | T3-09, R14 | `AGENTS.md` state layout, delete entry, pipeline note |
| FR-018 | T3-10, section 5, R12 | `tests/integration/testutils/registry.go`; `pinned_image_policy_test.go`; fixtures under `tests/testdata/pinned/` |

## Project Structure

### Documentation (this feature)

```text
specs/033-pinned-image-policy/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0: decisions with the design section that settles each, and the deviations found
├── data-model.md        # Phase 1: pin record, policy value, backend result, planner and handler projections
├── quickstart.md        # Phase 1: manual verification script (validation and dry run locally, real deploys in CI)
├── contracts/
│   ├── backend-contract.md   # ResolveImage behaviour per policy, errors, events, dry-run lines
│   ├── operator-output.md    # exact terminal, dry-run, delete, log, and failure strings
│   └── state-and-docs.md     # pins.txt format, dry-run invariant, documentation changes, design deviations
├── checklists/requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/
├── manifest/
│   ├── types.go                 # + ImagePullPolicyPinned, IsKnownPullPolicy, IsManifestOwnedPolicy, EffectivePullPolicyWithDefault, TagOf, IsDigestReference; EffectivePullPolicy uses TagOf
│   ├── types_test.go            # + TagOf table (ports, digests, untagged), derived-rule fix
│   ├── validate.go              # + validatePullPolicy for both kinds; validateResourceSpec drops the version check
│   ├── validate_test.go         # + enum rejection; resource without version accepted at parse time
│   ├── parser.go                # Resource image defaults to <type> when no version
│   └── parser_test.go           # + default without version
├── planner/
│   ├── plan.go                  # Plan(..., defaultPullPolicy); applyEffectivePullPolicy first
│   ├── resolve.go               # Resolve(..., defaultPullPolicy); + validateImagePolicies
│   ├── policy.go                # NEW: applyEffectivePullPolicy, validateImagePolicies, message helpers
│   ├── policy_test.go           # NEW: normalisation precedence; the four error shapes; untouched manifests unchanged
│   └── *_test.go                # Plan and Resolve call sites gain the "" argument
├── state/
│   ├── pins.go                  # NEW: ImagePin, ImagePinStore, ErrImagePinNotFound, ImagePinKey
│   ├── state.go                 # + ImagePins
│   └── local/
│       ├── pins.go              # NEW: file-backed store over readFile/writeTeamFile, mutex, forgiving reader
│       ├── pins_test.go         # NEW: in-memory file fake; load, put, release, release team, list all
│       └── local.go             # NewLocalStore constructs ImagePins
├── engine/
│   ├── backends.go              # ResolvedImage + Requested, PinnedAt; ImageSourceResolved, ImageSourcePinned
│   ├── dryrun/
│   │   ├── dry_run_engine.go         # NewDryRunEngine(out, hostPorts, pins)
│   │   ├── dry_run_container.go      # + Pins; ResolveImage prints the three shapes
│   │   └── dry_run_container_test.go # + would-pin and pinned lines
│   └── local/dockercontainer/
│       ├── docker_backend.go         # + now func() time.Time
│       ├── docker_image.go           # ResolveImage branches; resolveManifestOwned, usablePin, reusePin, pinNewest, releasePin, notServedError, sameRepository
│       ├── docker_image_test.go      # + scripted fake (inspect/pull per ref), fake pin store, the eight scenarios of research R13
│       └── docker_container_test.go  # fakeDockerAPI gains per-reference knobs if needed
├── handler/
│   ├── deploy.go                # Plan(..., ""); DryRun passes the pin snapshot
│   ├── apply.go                 # Plan(..., "")
│   ├── deployments.go           # DeleteApplication releases the pin; dry-run line
│   ├── deployments_test.go      # + pin release and dry-run cases; stub store gains ImagePins
│   ├── teams.go                 # DeleteTeam releases team pins; releaseTeamImagePins
│   └── teams_test.go            # + release count line
└── ui/
    ├── terminal_logger.go       # + resolved and pinned arms; readableVersion
    └── terminal_logger_test.go  # + two rendered kinds byte-exact, untagged request reads as latest

tests/
├── integration/
│   ├── pinned_image_policy_test.go   # NEW, written first: validation rejection; first pin; ten cycles; release paths; delete dry run; dry-run stability; no longer served
│   └── testutils/
│       └── registry.go               # NEW: StartLocalRegistry, PushAs, ImageIDOf, RemoveImage, WritePinnedFixture
└── testdata/pinned/
    └── fixed-version/                # NEW: Pinned app with a tag, Pinned resource with a version, resource with an unknown policy

docs/content/reference/manifest-schema.md   # three values, version optional under Pinned, "Image pull policy" subsection
AGENTS.md                                   # state layout, delete entry, pipeline note
specs/progress.md                           # entry for 033
specs/epics/pinned-image-versions/design.md # deviations recorded
```

**Structure Decision**: single-project Go layout already in place. New code sits beside the seam it extends: the pin store beside the host-port store it copies, the pinned branches in the file that already owns image resolution, the policy rules in a new planner file beside `resolve.go` because `resolve.go` is already long and the rules are one concern. The registry helper lives in the integration test utilities, which are isolated from internal packages by project rule.

## Deviations from the epic design

| Deviation | Why | Recorded |
|---|---|---|
| An enum check on `spec.imagePullPolicy` is added; the design assumed one existed | constitution I; three values make a typo dangerous | research R7; `design.md` section 1 with the PR |
| `EffectivePullPolicy` parses the tag with `TagOf`; untagged images on a registry with a port now derive `Always` as documented | one tag-parsing rule for the derived policy and the fixed-version check | research R6; PR description names it |
| Fixture images are two `traefik/whoami` tags, not alpine | alpine exits at once; whoami stays up and is cached | research R12 |
| `ResolvedImage` gains `Requested` and `PinnedAt` | the terminal needs the readable tag and the date; `ref` is a digest reference under `Pinned` | research R5 |
| A pin whose repository no longer matches the manifest is replaced | the design is silent; a pin for an image the manifest no longer names is meaningless | spec FR-011; research R3 |
| The "no longer served" and Resource fixed-version messages are reworded; the "no registry digest" failure is a backend event | the design's "edit the manifest" and "change the default" clauses do not fit a manifest-sourced `Pinned` policy | research R6, R8; contracts |
| The dry-run pinned line prints the full digest reference | the preview is where an operator copies the exact version from | research R9 |
| Integration helper names and the run-time fixture differ from design section 5 | `Config.Image` already carries the digest reference; the registry port is only known at run time | research R12 |
| Planner helpers live in a new `policy.go` | `resolve.go` is already long; the rules are one concern | plan structure |

## Complexity Tracking

No constitution principle violations to justify.
