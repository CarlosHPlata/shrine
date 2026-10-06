# Implementation Plan: Resolve Every Image Before Touching Any Container

**Branch**: `032-preflight-image-resolve` | **Date**: 2026-10-06 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/032-preflight-image-resolve/spec.md`
**Epic binding**: [design.md](../epics/pinned-image-versions/design.md) sections 4.1 to 4.4, 4.9, 4.11; decisions TD-2 and TD-5; requirement list T2-01 to T2-07. Decisions TD-1 to TD-13 are settled and are not reopened here.

## Summary

Image resolution leaves `CreateContainer` and becomes a method on the container backend contract, `ResolveImage(op ResolveImageOp) (ResolvedImage, error)`. The engine calls it for every planned step, in step order, as a pre-pass at the top of `ExecuteDeploy`, before `CreatePlatformNetwork`, so the first failure aborts the deploy with no network or container touched. Each result (expanded reference, registry digest, local image id, source `manifest`) is handed to the container op; `CreateContainer` then skips its own resolution when the op carries an image id and keeps that id as the config-hash input, so nothing is recreated on upgrade. The Traefik plugin, which calls `CreateContainer` directly, leaves the id empty and keeps today's path. The dry-run backend prints one `ImageResolve` line per artifact; two terminal renderers appear for the new `image.resolve` events. Pull semantics are unchanged: the moved code keeps the `ImageList` lookup for `IfNotPresent` and the pull for `Always`.

## Technical Context

**Language/Version**: Go 1.25 (`go.mod`: `go 1.25.0`), module `github.com/CarlosHPlata/shrine`
**Primary Dependencies**: Docker SDK `github.com/docker/docker v28.5.2` (`ImageList`, `ImagePull`, `ImageInspect`; `image.Summary` and `image.InspectResponse` both carry `ID` and `RepoDigests`), Cobra (untouched)
**Storage**: none new; `deployments.txt` and the config hash are unchanged (design TD-2)
**Testing**: `go test ./...` for unit tests (fakes over the `dockerAPI` seam and the `ContainerBackend` interface; no filesystem); `go vet -tags integration ./tests/integration/...` to compile the integration suite; CI runs `make test-integration` against a real daemon. Integration scenarios are never run locally for this ticket.
**Target Platform**: Linux single host with a local Docker daemon
**Project Type**: CLI with a pluggable-backend execution engine
**Performance Goals**: no new latency beyond today's pulls; the pre-pass performs the same list/pull/inspect calls `CreateContainer` performs today, moved earlier
**Constraints**: zero Docker or policy logic in the engine (constitution III); dry run is a backend, not a branch; existing integration assertions pass unedited; no new manifest, config, or CLI surface
**Scale/Scope**: homelab scale; one `ResolveImage` call per planned step

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] N/A, with pass: no new manifest, config, or CLI surface; the existing `spec.imagePullPolicy` field drives the behaviour (design section 4.2, cases 1, 2, 5) |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] Pass: no new command; the dry-run backend prints the new step so `deploy --dry-run` previews it faithfully (design section 4.4) |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass by construction: `ResolveImage` is a `ContainerBackend` method; the engine only builds `ResolveImageOp` from the manifest and projects the result onto `CreateContainerOp` (design TD-5, section 4.3); dry run is the print-only backend's own `ResolveImage` |
| IV. Simplicity & YAGNI | Is every abstraction justified by three or more concrete usages? | [x] Pass: one new interface method with three implementations that exist today (Docker, dry run, test fakes); no `Repin` field, no pin store, no new backend; `pickRepoDigest` and `shortDigest` are small named helpers the next tickets reuse |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass: `tests/integration/preflight_image_resolve_test.go`, written before the implementation and compiled under the integration tag; CI executes it (tickets.md T2 integration scenarios) |
| VI. Docker-Authoritative State | Does state update happen after Docker operations complete? | [x] Pass: no state write is added; `recordDeployment` still runs after `ContainerStart`; the pre-pass writes nothing |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | [x] Pass: `locateImage`, `findLocalImage`, `pullImage`, `inspectImage`, `pickRepoDigest`, `repositoryOf`, `resolveImages`, `resolveImageOpFor`, `resolveContainerImage`, `shortDigest`; the only comments kept are the WHY note on single alias expansion (#33) and the WHY on the started line not being an indicator |

**Post-Phase-1 re-check**: all gates still pass. The constitution's rule "backend interfaces are defined in `internal/engine/backends/`" is satisfied by today's `internal/engine/backends.go`, where every interface already lives.

## Design binding

Every functional requirement of the spec is bound to the seam that satisfies it, so tasks can be derived mechanically.

| Spec | Design | Where |
|---|---|---|
| FR-001, FR-009 | T2-02, section 4.3 | `resolveImages(set, steps)` in `internal/engine/engine.go`, called at the top of `ExecuteDeploy` before `CreatePlatformNetwork`, one op per planned step in step order |
| FR-002 | T2-02, section 4.3 | first error aborts; engine emits `image.resolve` error with `team`, `name`, `ref`, wrapped `<kind> "<name>": <backend error>`; backend messages name the reference |
| FR-003, FR-012 | T2-01, sections 4.1, 4.2 | `ResolveImageOp`, `ResolvedImage`, `ContainerBackend.ResolveImage` in `internal/engine/backends.go`; `DockerBackend.ResolveImage` in `docker_image.go` expands the alias once, locates the image per policy, picks the digest, returns `Source: manifest` |
| FR-004 | T2-07 | `locateImage` keeps today's `ImageList` lookup for non-`Always` and the pull for `Always`; a local hit reads `ID` and `RepoDigests` from the list result and makes no registry call |
| FR-005 | T2-03, TD-2 | `CreateContainerOp.ImageID`; `resolveContainerImage` in `docker_container.go` returns `op.ImageID` when set, else expands and locates as today; `configHash(op, imageID)` input unchanged |
| FR-006 | T2-03 | Traefik `RoutingBackend.ensureContainer` path untouched; `ImageID` empty keeps today's behaviour |
| FR-007 | T2-04, section 4.9 | backend emits `image.resolve` started and finished; two `case` arms in `internal/ui/terminal_logger.go`; `shortDigest` helper |
| FR-008 | T2-05, section 4.4 | `DryRunContainerBackend.ResolveImage` prints the `manifest-owned` line and returns `ResolvedImage{Ref: op.Image, Source: manifest}` |
| FR-010 | T2-06, section 4.11 | `AGENTS.md` Deploy Pipeline diagram gains `Container.ResolveImage()` before `CreatePlatformNetwork()` with one sentence on the zero-change guarantee |
| FR-011 | M2 | existing suites unchanged; new assertions only in the new file plus one output assertion appended as a new scenario in `deploy_test.go` |

## Project Structure

### Documentation (this feature)

```text
specs/032-preflight-image-resolve/
├── plan.md              # This file
├── spec.md              # Feature specification with clarifications
├── research.md          # Phase 0: decisions with the design section that settles each
├── data-model.md        # Phase 1: new types, events, projections
├── quickstart.md        # Phase 1: manual verification script (dry run locally, real deploys in CI)
├── contracts/
│   ├── backend-contract.md   # ResolveImage behaviour per policy, errors, events
│   └── operator-output.md    # exact terminal, dry-run, log, and failure strings
├── checklists/requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/
├── engine/
│   ├── backends.go              # + ResolveImageOp, ResolvedImage, ImageSourceManifest, ContainerBackend.ResolveImage, CreateContainerOp.ImageID
│   ├── engine.go                # + resolveImages pre-pass before CreatePlatformNetwork; deployApplication/deployResource take the result
│   ├── engine_test.go           # fakes gain ResolveImage; + pre-pass ordering and abort tests
│   ├── engine_publish_test.go   # fake gains ResolveImage
│   ├── dryrun/
│   │   ├── dry_run_container.go      # + ResolveImage printing the manifest-owned line
│   │   └── dry_run_container_test.go # + line test
│   └── local/dockercontainer/
│       ├── docker_image.go           # ResolveImage; locateImage, findLocalImage, pullImage, inspectImage, pickRepoDigest, repositoryOf
│       ├── docker_image_test.go      # NEW: pickRepoDigest, ResolveImage per policy over fakeDockerAPI, events
│       ├── docker_container.go       # resolveContainerImage: skip when ImageID set; hash input unchanged
│       └── docker_container_test.go  # + skip-resolution test; fake gains ImageInspect/ImageList behaviour knobs
├── handler/deployments_test.go   # stub gains ResolveImage
├── plugins/gateway/traefik/
│   ├── plugin_test.go            # fake gains ResolveImage
│   └── routing_test.go           # fake gains ResolveImage
└── ui/
    ├── terminal_logger.go        # + image.resolve started and finished (source manifest) arms; shortDigest
    └── terminal_logger_test.go   # + two rendered kinds, no-digest form, silent finished for other sources

tests/
├── integration/
│   ├── preflight_image_resolve_test.go  # NEW, written first: unresolvable leaves Docker untouched; dry run shows the step; fixed tag reuses, latest re-pulls
│   └── deploy_test.go                   # + one scenario asserting the resolved-version lines on a healthy deploy
└── testdata/deploy/
    ├── preflight-unresolvable/          # NEW: healthy resource + healthy app + app depending on both with an unreachable registry reference
    └── preflight-fixed-tag/             # NEW: app with a fixed tag

AGENTS.md            # Deploy Pipeline diagram gains the pre-pass
specs/progress.md    # entry for 032
```

**Structure Decision**: single-project Go layout already in place; no new directories beyond the two fixtures. New code sits beside the seam it extends (`docker_image.go` already owns image resolution; the dry-run and terminal files already hold one arm per operation).

## Complexity Tracking

No constitution principle violations to justify.
