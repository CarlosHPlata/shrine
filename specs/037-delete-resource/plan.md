# Implementation Plan: `shrine delete resource` and pin release on every delete

**Branch**: `037-delete-resource` | **Date**: 2026-10-07 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/037-delete-resource/spec.md`
**Epic binding**: [design.md](../epics/pinned-image-versions/design.md) section 4.8; requirement list T7-01 to T7-03; PRD R-11, R-27, R-30; journey J8. Decisions TD-1 to TD-13 are settled and are not reopened here; the two places where this plan refines the design are listed in [contracts/docs-and-deviations.md](contracts/docs-and-deviations.md) and are recorded in `design.md` with the pull request.

## Summary

Ticket T3 made `delete application` and `delete team` release pins and left `DeleteApplication` as the one per-artifact delete: team resolution over host ports, application records, and application pins; refusal while `<team>.<name>` exists in Docker; dry-run lines; release of port, pin, and record in that order. This ticket generalises that body over the kind into an unexported `deleteArtifact`, keeps `DeleteApplication` as a one-line wrapper, adds `DeleteResource` as the second, and renames the shared options to `DeleteOptions`. The resource path skips the host-port step and otherwise prints the same lines with `resource` for `application`. The candidate search and the pin read both compare the pin's kind to the requested kind, so the two verbs never release each other's pin. `cmd/delete.go` gains `delete resource <name>` with `--team` and `--dry-run` through a shared `runDelete(kind)` dispatcher over `app.NewQueryContainerBackend`. The delete integration suite gains `TestDeleteResource` on T3's loopback-registry world, written first, including one scenario that runs all three delete verbs and asserts each releases its pins. Documentation: a generated `delete resource` page, the `delete` parent page, the manifest reference's "things that release a pin" sentence, and the `AGENTS.md` CLI reference and command tree.

## Technical Context

**Language/Version**: Go 1.25 (`go.mod`: `go 1.25.0`), module `github.com/CarlosHPlata/shrine`
**Primary Dependencies**: Docker SDK (the existing `ContainerInspect` through `ContainerBackend.InspectContainer`, no new call), Cobra (one new subcommand with two flags), no new module
**Storage**: `pins.txt`, `deployments.txt`, `hostports.txt` per team, read and written through the existing `state.Store` interfaces; no format change, no new method
**Testing**: `go test ./...` for unit tests (`deleteArtifact` through both wrappers over `deleteTestStore`, `newMemImagePinStore`, and `stubContainerBackend`, already in `deployments_test.go`; `cmd/delete_test.go` for the arg-count error; no filesystem); `go vet -tags integration ./tests/integration/...` to compile the suite; CI runs `make test-integration` for `TestDeleteResource`. Integration scenarios are never run locally for this ticket
**Target Platform**: Linux single host with a local Docker daemon; CI is `ubuntu-latest` with Docker
**Project Type**: CLI with a pluggable-backend execution engine
**Performance Goals**: none measurable; one container inspect, one pin listing, one records listing, at most two writes per delete
**Constraints**: Docker authoritative (refuse while the container exists; fail when the runtime cannot be reached); dry run writes nothing; a resource never touches `HostPorts`; the two verbs share one body (constitution VII) and stay byte-identical in form; `delete application` and `delete team` output unchanged; existing integration assertions unchanged; thin command file (constitution II); no new abstraction (constitution IV); unit tests off the filesystem
**Scale/Scope**: homelab scale; tens of artifacts per team

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] Pass: no new capability; delete is a state operation the PRD (R-27) defines as a command, mirroring the existing `delete application`. `--team` and `--dry-run` are the constitution's own required flags, not infrastructure |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] Pass: `shrine delete resource <name>`, resource type before the name, `--team` optional with automatic search and an ambiguity error, `--dry-run` with a print-only path. `cmd/delete.go` dispatches through `runDelete(kind)`; every rule lives in `internal/handler/deployments.go` |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass: the only infrastructure call is `ContainerBackend.InspectContainer`, already used by `DeleteApplication`; interfaces and `engine.go` untouched; a nil backend is tolerated as today |
| IV. Simplicity & YAGNI | Is every abstraction justified by three or more concrete usages? | [x] Pass: no new type beyond the renamed `DeleteOptions`; `deleteArtifact` is the extraction of a body two exported functions would otherwise duplicate; `hasHostPortStep` is a one-line predicate; `runDelete` replaces two identical `RunE` closures; no interface, no option pattern, no new package |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass: `TestDeleteResource` in `tests/integration/delete_test.go` on `newPinnedSuite` (loopback registry, real binary), written before the implementation; CI executes (ticket T7 integration scenarios) |
| VI. Docker-Authoritative State | Does state update happen after Docker operations complete? | [x] Pass: the delete inspects Docker first and refuses while the container exists; state is released only after Docker reports it gone; an unreachable runtime fails the command before any release, as today |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | [x] Pass: `deleteArtifact`, `resolveDeleteTeam(kind)`, `findImagePin(kind)`, `findDeploymentRecord(kind)`, `hasHostPortStep`, `runDelete`; existing WHY comments kept, one updated to name both verbs |

**Post-Phase-1 re-check**: all gates still pass. Installations that never run `delete resource` see no change: `DeleteApplication`'s observable behaviour is preserved line for line (its unit tests keep their assertions), and the one internal change on its path, the kind guard on the pin read, cannot alter a result because one pin exists per `team/name` and the candidate search already applied the guard.

## Design binding

Every functional requirement of the spec is bound to the seam that satisfies it, so tasks can be derived mechanically.

| Spec | Design | Where |
|---|---|---|
| FR-001 | T7-01, R-27, 4.8 | `cmd/delete.go`: `deleteResourceCmd`, `-t/--team`, `--dry-run`, `cobra.ExactArgs(1)`, `runDelete(manifest.ResourceKind, …)` |
| FR-002 | T7-01, R-27, R2 | `resolveDeleteTeam(store, kind, name, team)`: records and pins of the requested kind; host ports only for applications; ambiguity error with the kind word; `--team` short-circuits |
| FR-003 | T7-01, R-27, constitution VI | `deleteArtifact` step 2: `InspectContainer(team + "." + name)`; refusal message with the kind word; `app.NewQueryContainerBackend` failure surfaces before the handler |
| FR-004 | T7-01, R-27, R1 | `deleteArtifact` steps 3 and 5: `hasHostPortStep(kind)` gates the port; pin then record; the per-line output and the nothing-held line |
| FR-005 | T7-01, R-27 | `deleteArtifact` step 4: the `[dry-run]` lines over the same held-state reads; refusals precede the branch |
| FR-006 | T7-01, R3 | `findImagePin` guard `pin.Kind == kind`; `resolveDeleteTeam` filters pins and records by kind; unit tests `…IgnoresAnApplicationOfTheSameName` and `…IgnoresAResourceOfTheSameName` |
| FR-007 | T7-01, R1, constitution VII | one `deleteArtifact` body; `DeleteApplication` and `DeleteResource` are wrappers |
| FR-008 | T7-02, R-11 | no code change beyond the new verb: T3's release paths stand; asserted by the three-verb scenario |
| FR-009 | T7-02, R5 | `TestDeleteResource` (six scenarios) in `tests/integration/delete_test.go`, written first; ambiguity at unit level (refinement 2) |
| FR-010 | T7-03, R-30, R6 | `make docs-gen-cli` (`delete_resource.md`, `delete.md`); `AGENTS.md` CLI reference and tree; manifest reference sentence |
| FR-011 | G6 | `DeleteApplication` behaviour preserved; existing suites untouched; `delete` parent help gains one line |

## Project Structure

### Documentation (this feature)

```text
specs/037-delete-resource/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0: decisions with the design section that settles each, and the refinements found
├── data-model.md        # Phase 1: DeleteOptions, held state per kind, kind word, pin invariant, command tree, integration world
├── quickstart.md        # Phase 1: manual verification script (steps 1 and 2 need no daemon)
├── contracts/
│   ├── operator-output.md        # command surface, help text, exact success, dry-run, refusal, and failure output
│   ├── handler-and-command.md    # signatures, dispatch order, rules the unit tests pin, cmd wiring, integration scenarios
│   └── docs-and-deviations.md    # documentation changes, progress entry, design refinements to record
├── checklists/requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
cmd/
├── delete.go                    # + deleteResourceCmd, deleteResTeam/deleteResDryRun, runDelete(kind, team, dryRun); application RunE through runDelete
└── delete_test.go               # NEW: TestDeleteResource_RequiresArg

internal/handler/
├── deployments.go               # DeleteApplicationOptions → DeleteOptions; DeleteApplication/DeleteResource wrappers; deleteArtifact; kind on resolveDeleteTeam, findImagePin, findDeploymentRecord; hasHostPortStep
└── deployments_test.go          # call sites → DeleteOptions; + TestDeleteResource_* (eight) and TestDeleteApplication_IgnoresAResourceOfTheSameName

tests/integration/
└── delete_test.go               # + TestDeleteResource on newPinnedSuite, written first (six scenarios)

docs/content/cli/delete.md                  # regenerated: SEE ALSO gains resource
docs/content/cli/delete_resource.md         # generated, NEW
docs/content/reference/manifest-schema.md   # "Only four things release a pin: … delete resource …"
AGENTS.md                                   # CLI reference heading and paragraph; cmd tree line for delete.go
specs/progress.md                           # entry for 037
specs/epics/pinned-image-versions/design.md # refinements recorded
graphify-out/                               # graphify update .
```

**Structure Decision**: single-project Go layout already in place. The handler change lands in the file that owns `DeleteApplication`; the command change lands in the file that owns the `delete` verb; the integration scenarios land in the delete suite the ticket names and reuse the pinned world from `pinned_image_policy_test.go` (same package, no new fixture). No new file beyond `cmd/delete_test.go` and the generated page.

## Deviations from the epic design

| Deviation | Why | Recorded |
|---|---|---|
| `findImagePin` reads a pin as the artifact's only when `pin.Kind` equals the requested kind; `Deployments.Remove` stays by name | the candidate search and the queries already apply the guard; one record, one pin, and one container exist per `team/name`, so a kind on `Remove` would change a store method for an unreachable case | research R3; `design.md` 4.8 with the PR |
| The ambiguity-across-teams case for `delete resource` is unit-tested, not an integration scenario; the integration suite covers `--team` given and omitted and the three-verb release | the pinned world has one team; a second team holding a resource of the same name would need hand-written state files; `delete application`'s ambiguity is unit-only too | research R5; T7 scenarios in `design.md` with the PR |

## Complexity Tracking

No constitution principle violations to justify.
