# Implementation Plan: Pinned Versions in `get`, `describe`, and `status`

**Branch**: `035-pinned-version-queries` | **Date**: 2026-10-07 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/035-pinned-version-queries/spec.md`
**Epic binding**: [design.md](../epics/pinned-image-versions/design.md) sections 3.3, 3.4, 3.5, 4.1 (`ContainerInfo.Image`), 4.7; decision TD-11; requirement list T5-01 to T5-04. Decisions TD-1 to TD-13 are settled and are not reopened here; the six places where this plan refines the design are listed in [contracts/docs-and-deviations.md](contracts/docs-and-deviations.md) and are recorded in `design.md` with the pull request.

## Summary

Tickets T1 and T3 left every input in place: the deployment record carries the image reference and the effective policy, `pins.txt` holds the exact version, the reference it was resolved from, and the date, and the terminal already renders a pin in the readable form of design section 3.5. This ticket shows those facts through the three query commands without writing anything. The listing commands load the pins once and, for a record whose policy is `Pinned`, print `<tag>@<twelve hex>` in the VERSION column instead of the manifest reference, which stays for every other row; the commands keep reading state only, so they work without Docker. `describe` gains the read-only container backend `delete application` already uses and prints two more lines: `Pinned:` with the full exact version, the readable form, and the date, only for a `Pinned` record, and `Running image:` with the reference the container was created from, read from Docker through a new `ContainerInfo.Image` field, so a pin recorded after the last deploy is visible as the two lines disagreeing; when Docker cannot be asked the line says `unavailable` and the command still succeeds. `status` prints the same reference as an IMAGE column beside the running state, a digest reference shortened to twelve hex characters as the IMAGE ID beside it already is. The readable-form helpers move from `internal/ui` into `internal/manifest`, exported, so the terminal, the table, and the detail block share one implementation. Pins are never listed on their own, which is what keeps a torn-down artifact's pin invisible. The manifest reference, five command help texts with their regenerated pages, and `AGENTS.md` describe the new output. One new integration file on the loopback-registry world, written first, covers the pinned rows, the difference, and teardown; three small scenarios appended to the existing `describe` and `status` suites cover the manifest-owned and no-runtime cases.

## Technical Context

**Language/Version**: Go 1.25 (`go.mod`: `go 1.25.0`), module `github.com/CarlosHPlata/shrine`
**Primary Dependencies**: Docker SDK (one more field read from `ContainerInspect`, no new call), Cobra (five `Long` texts change; no new command or flag), no new module
**Storage**: none new and nothing written. `deployments.txt` and `pins.txt` are read as T1 and T3 defined them
**Testing**: `go test ./...` for unit tests (pure renderers `formatDeploymentsTable`, `formatDeploymentDetail`, `formatStatusTable`, the `runningImage` rule, the moved helpers in `manifest`; in-memory stores and stub backends; no filesystem); `go vet -tags integration ./tests/integration/...` to compile the suite; CI runs `make test-integration` for `TestPinnedVersionQueries` and the appended scenarios. Integration scenarios are never run locally for this ticket
**Target Platform**: Linux single host with a local Docker daemon; CI is `ubuntu-latest` with Docker
**Project Type**: CLI with a pluggable-backend execution engine
**Performance Goals**: none measurable; one `pins.txt` read per team per listing, one `ContainerInspect` per described artifact (already one per row in `status`)
**Constraints**: no write of any kind (ticket scope); no engine change; `get` must not touch Docker; `describe` must succeed without Docker; existing integration assertions pass unedited apart from added lines; unit tests off the filesystem; one readable form (constitution VII); no new abstraction (constitution IV)
**Scale/Scope**: homelab scale; tens of artifacts per host

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] N/A: no capability is added; the commands show state that T1 and T3 record from manifest fields. No flag is added |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] Pass: no new command; the three existing read-only verbs change their output; nothing mutates, so no dry-run path is owed. Command files stay thin: `cmd/describe.go` builds one backend and dispatches, as `cmd/delete.go` does; the rendering rules live in `internal/handler/` |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass: the one Docker fact this feature needs, the container's creation reference, is read by `DockerBackend.InspectContainer` into `engine.ContainerInfo.Image`; the interface's method set is unchanged, the dry-run backend keeps its zero value, and `engine.go` is untouched |
| IV. Simplicity & YAGNI | Is every abstraction justified by three or more concrete usages? | [x] Pass: `ReadableVersion` and `ShortDigest` move because they now have three readers (terminal, VERSION column, `Pinned:` line); `deploymentDetail` is a plain struct that carries the renderer's five inputs; no option struct, no interface, no new package |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass: `tests/integration/pinned_version_queries_test.go` on `newPinnedSuite` (loopback registry, real binary), written before the implementation; scenarios appended to `TestDescribeDocker`, `TestDescribeNoDocker`, `TestStatusDocker`; CI executes (ticket T5 integration scenarios) |
| VI. Docker-Authoritative State | Does state update happen after Docker operations complete? | [x] N/A: no state is written. Docker stays authoritative for the running image, which is read live and never cached in state |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | [x] Pass: one readable form in `manifest.ReadableVersion`; `loadImagePins`, `versionCell`, `pinFor`, `pinLine`, `runningImage`, `shortImageReference`, `formatStatusTable`; the status separator stops being a hardcoded 84 and is computed from the header as the deployments table does; comments only for WHY |

**Post-Phase-1 re-check**: all gates still pass. The one visible change for installations with no pinned artifact is the `Running image:` line in `describe` and the IMAGE column in `status`, both named by the ticket as added output lines (PRD M2 allows additions).

## Design binding

Every functional requirement of the spec is bound to the seam that satisfies it, so tasks can be derived mechanically.

| Spec | Design | Where |
|---|---|---|
| FR-001 | T5-01, section 3.5, TD-11, R1, R2 | `versionCell` in `deployments.go` via `manifest.ReadableVersion(pin.Requested, manifest.DigestOf(pin.Pinned))`; `formatDeploymentsTable(deployments, pins)` |
| FR-002 | R-17, R2 | `loadImagePins(store)` reads `ImagePins.ListAll()`; no backend is built in `cmd/get.go` |
| FR-003 | R2, R5 | `pinFor` returns absent for a missing pin or a kind mismatch; `versionCell` falls back to `valueOrUnknown(d.Image)`; `pinLine` prints `-` |
| FR-004 | T5-02, R-18, R5 | `pinLine(d deploymentDetail)` in `formatDeploymentDetail`; `describeDeployment` reads `store.ImagePins.Get` only for a `Pinned` record |
| FR-005 | T5-02, section 4.1, R3, R4 | `engine.ContainerInfo.Image` from `docker_status.go`; `runningImage(backend, containerID)`; `Running image:` line for every record |
| FR-006 | section 4.7, R3 | `runningImage` returns `unavailable (…)` for a nil backend or an inspect error; `describeDeployment` never returns the backend's error; `cmd/describe.go` builds `app.NewQueryContainerBackend` |
| FR-007 | T5-04, R-20, OD-3, R6 | `containerStatusRow.Image`; `inspectDeployments` fills it; `formatStatusTable` prints IMAGE between STATUS and IMAGE ID |
| FR-008 | OD-4, TD-11, R1, R6 | `manifest.ShortDigest` in `ReadableVersion` and in `shortImageReference`; `describe` prints `pin.Pinned` and `info.Image` in full |
| FR-009 | T5-03, R-19 | no code lists pins; `describeDeployment` and the listing read deployment records first; asserted by the teardown scenario |
| FR-010 | ticket "Out of scope: any write" | no store `Put`/`Record`/`Release` call in the diff; asserted by reviewing the handler and by the teardown scenario's `pins.txt` byte check |
| FR-011 | M2 | existing suites untouched; `formatDeploymentsTable` with `nil` pins reproduces T1 output; the detail block adds one line for non-pinned records |
| FR-012 | R-28, R-30, R9 | manifest reference paragraph, five `Long` texts, `make docs-gen-cli`, `AGENTS.md` lines per the docs contract |
| FR-013 | ticket T5 scenarios, R7, R8 | `tests/integration/pinned_version_queries_test.go` plus three appended scenarios |

## Project Structure

### Documentation (this feature)

```text
specs/035-pinned-version-queries/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0: decisions with the design section that settles each, and the refinements found
├── data-model.md        # Phase 1: moved helpers, ContainerInfo.Image, listing and detail renderers, status row
├── quickstart.md        # Phase 1: manual verification script (steps 1 and 2 need no daemon)
├── contracts/
│   ├── query-output.md           # exact table and detail output, unavailable rule, help text
│   ├── handler-and-backend.md    # signatures, helper rules the unit tests pin, cmd wiring
│   └── docs-and-deviations.md    # documentation changes, progress entry, design refinements to record
├── checklists/requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/
├── manifest/
│   ├── types.go                 # + ReadableVersion, ShortDigest (moved from ui, exported), DigestOf (new)
│   └── types_test.go            # + table tests for the three helpers
├── ui/
│   └── terminal_logger.go       # readableVersion/shortDigest removed; calls manifest.ReadableVersion; exactVersion stays
├── engine/
│   ├── backends.go              # ContainerInfo + Image
│   └── local/dockercontainer/
│       └── docker_status.go     # Image from resp.Config.Image (nil-guarded)
├── handler/
│   ├── deployments.go           # loadImagePins; formatDeploymentsTable(deployments, pins); versionCell; pinFor; deploymentDetail; formatDeploymentDetail(detail); pinLine; runningImage; Describe* gain backend
│   ├── deployments_output_test.go # + pinned row, missing pin, kind mismatch, non-Pinned ignores pin; detail with Pinned: and Running image:, ordering; runningImage rule
│   ├── deployments_test.go      # + describeDeployment with nil ImagePins, ErrImagePinNotFound, failing backend (in-memory stores, stub backend)
│   ├── status.go                # containerStatusRow.Image; shortImageReference; formatStatusTable; separator from header
│   └── status_test.go           # mock gains Image; + formatStatusTable and shortImageReference tests
cmd/
├── describe.go                  # app/resource build app.NewQueryContainerBackend and pass it; Long texts
└── status.go                    # Long texts only

tests/
└── integration/
    ├── pinned_version_queries_test.go   # NEW, written first: TestPinnedVersionQueries on newPinnedSuite
    ├── describe_test.go                 # + Running image for a manifest-owned app (Docker); + unavailable on seeded records (no Docker)
    └── status_test.go                   # + IMAGE column for the resources fixture

docs/content/reference/manifest-schema.md   # "Reading what is pinned" paragraph
docs/content/cli/describe_app.md            # regenerated
docs/content/cli/describe_resource.md       # regenerated
docs/content/cli/status.md                  # regenerated
docs/content/cli/status_application.md      # regenerated
docs/content/cli/status_resource.md         # regenerated
AGENTS.md                                   # status and describe reference lines
specs/progress.md                           # entry for 035
specs/epics/pinned-image-versions/design.md # refinements recorded
graphify-out/                               # graphify update .
```

**Structure Decision**: single-project Go layout already in place. Every change lands in the file that owns the seam: the handler files that render the three commands, the backend file that inspects containers, the manifest package that owns reference helpers, and the two command files that wire a backend and carry help text. The only new source file is the integration scenario file; no new fixture directory is needed because the pinned world is built at run time by `newPinnedSuite` and the manifest-owned rows use the existing `basic` and `resources` fixtures.

## Deviations from the epic design

| Deviation | Why | Recorded |
|---|---|---|
| The readable-form helpers move from `internal/ui` to `internal/manifest`, exported, with `DigestOf` added | three readers now share one form; `manifest` already owns `TagOf` and `IsDigestReference` | research R1; `design.md` sections 3.5 and 4.9 with the PR |
| `describe` handlers take the backend as a parameter and tolerate nil; only `InspectContainer` failures degrade the line, while a backend construction failure fails the command | the client constructor fails only on a malformed environment, which `status` and `delete` also refuse; unreachability surfaces at inspect | research R3; section 4.7 |
| The listing reads pins once with `ListAll()`; a `Pinned` record without a pin shows the recorded reference | one file read per team, not per row; the design does not say what a pinless `Pinned` record shows | research R2; section 4.7 |
| `Pinned:` is one line: `<pinned> (<readable>, <date>)` | R-18's three facts on one scannable line, with the readable form as the table prints it | research R5; section 4.7 |
| The status IMAGE column shortens a digest reference to `<repository>@<twelve hex>` | TD-11 makes twelve hex the table form; a full loopback digest reference is wider than the row | research R6; section 4.7 |
| The "pin differs" scenario edits `pins.txt` until `bump` exists | T6 is in the same wave and may land later | research R7; section 5 |

## Complexity Tracking

No constitution principle violations to justify.
