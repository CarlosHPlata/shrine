# Implementation Plan: `shrine bump`

**Branch**: `036-bump-command` | **Date**: 2026-10-07 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/036-bump-command/spec.md`
**Epic binding**: [design.md](../epics/pinned-image-versions/design.md) sections 4.2 (point 4), 4.6, 4.9, 4.11; decisions TD-8 and TD-12; requirement list T6-01 to T6-07. Decisions TD-1 to TD-13 are settled and are not reopened here; the seven places where this plan refines the design are listed in [contracts/docs-and-deviations.md](contracts/docs-and-deviations.md) and are recorded in `design.md` with the pull request.

## Summary

Ticket T3 left every input in place: the `Pinned` policy, the pin record in `pins.txt`, and `DockerBackend.ResolveImage`, which is the only writer of pins. This ticket adds `shrine bump application|resource <name>` (aliases `app`, `res`; flags `-v`, `-t`, `-p`, `--dry-run`): a thin command that resolves the manifest directory and dispatches to a new bump bundle in the composition root (specs directory, terminal and file-logger observers, a container backend, no engine) and a new handler. The handler loads the directory with `planner.LoadDir`, finds the manifest by kind and name (a name is unique per directory, so `--team` verifies `metadata.owner` rather than disambiguating), validates `-v` as a tag or a `sha256:` exact version, plans the set with `planner.Plan` and a by-name filter through a helper extracted from `Deploy` and `DryRun`, refuses when the effective policy is not `Pinned` or when the name is unknown, and builds the target by swapping the manifest image's version for the requested one through a new `manifest.RepositoryOf` that also replaces two private copies. It reads the current pin for the "previous" line, then calls `ResolveImage` with a new `Repin` field on the op; the backend's fourth branch expands the target, pulls it, picks its digest, and records the pin through `pinReference`, the first-deploy branch renamed and given a source parameter, returning source `repinned`, which the terminal renders as `📌 Bumped team.name to v2@<12 hex>`. The handler then prints `Bumped team/name: <previous> -> <new>; run "shrine deploy" to apply` or `Pinned team/name at <new>; …`. No container is touched; the next deploy applies the pin. `--dry-run` runs a separate handler that builds no bundle and prints `[dry-run] would resolve <target> and pin team/name`. The T3 message for a vanished exact version now ends with `run "shrine bump <kind> <name>" to choose another version`. Three generated command pages, the manifest reference, and `AGENTS.md` document the command. One new integration file on the loopback-registry world, written first, covers the six ticket points plus rollback; one assertion in the T3 suite is reworded.

## Technical Context

**Language/Version**: Go 1.25 (`go.mod`: `go 1.25.0`), module `github.com/CarlosHPlata/shrine`
**Primary Dependencies**: Docker SDK (the existing `ImagePull` and `ImageInspect` calls, no new call), Cobra (one new verb with two subcommands and four persistent flags), no new module
**Storage**: `pins.txt` per team as T3 defined it, written only by `DockerBackend.ResolveImage`; `deployments.txt` and `hostports.txt` read through the planner's port context; nothing new on disk
**Testing**: `go test ./...` for unit tests (pure handler helpers over an in-memory `ManifestSet`, the `memTeamStore` and a new `memImagePinStore`, a recording container-backend stub; the backend's repin branch over `scriptedDockerAPI` and `fakePinStore`; the terminal arm byte-exact; the bundle shape over swapped constructors; `manifest.RepositoryOf` table; no filesystem); `go vet -tags integration ./tests/integration/...` to compile the suite; CI runs `make test-integration` for `TestBump` and the reworded `TestPinnedImagePolicy` assertion. Integration scenarios are never run locally for this ticket
**Target Platform**: Linux single host with a local Docker daemon; CI is `ubuntu-latest` with Docker
**Project Type**: CLI with a pluggable-backend execution engine
**Performance Goals**: none measurable; one directory load and plan, one pin read, one pull, one inspect, one pin write per bump
**Constraints**: the backend stays the only pin writer (TD-8); no engine call (TD-12); no container started, stopped, or recreated (R-23); dry run writes nothing and builds no Docker client (R-25); refusals contact no registry (R-05, R-24); unit tests off the filesystem; existing integration assertions unchanged apart from the reworded T3 clause the ticket names; thin command file (constitution II); no new abstraction beyond the fourth bundle (constitution IV); one repository helper and one planning helper (constitution VII)
**Scale/Scope**: homelab scale; tens of artifacts per directory

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] Pass: the capability (a Shrine-owned version) is the manifest's `imagePullPolicy: Pinned` from T3; bump is an operation on that record, which the PRD (R-21) defines as a command, and it adds no manifest field and no capability a flag would hide. `-v` names an argument of the operation, not infrastructure |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] Pass: `shrine bump application <name>` / `bump resource <name>`, resource type before the name, `app`/`res` aliases as `status` and `generate` use, `--team` optional, `--dry-run` with a print-only path and no side effects. `cmd/bump.go` resolves the directory and dispatches; every rule lives in `internal/handler/bump.go` |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass: the registry work is the `Repin` branch of `ContainerBackend.ResolveImage` in the Docker backend; the interface's method set is unchanged, the dry-run backend is untouched (bump's dry run calls no backend), and `engine.go` is untouched |
| IV. Simplicity & YAGNI | Is every abstraction justified by three or more concrete usages? | [x] Pass: `BumpBundle` is the fourth instance of an established pattern, not a new one; `planManifestSet` has three callers (`Deploy`, `DryRun`, bump); `manifest.RepositoryOf` has three (planner validation, the backend, the handler) and deletes two copies; `pinReference` has two callers and replaces a would-be duplicate of thirty lines; `bumpTarget` is a plain struct; no interface, no option pattern, no new package |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass: `tests/integration/bump_test.go`, `TestBump` on `newPinnedSuite` (loopback registry, real binary), written before the implementation; the T3 suite's one assertion reworded; CI executes (ticket T6 integration scenarios) |
| VI. Docker-Authoritative State | Does state update happen after Docker operations complete? | [x] Pass: the pin is written after the pull and the inspect succeed, inside the backend, as T3's first deploy does; a failed pull leaves the pin untouched; no container state is read or written, and the next deploy reconciles against Docker as it always does |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | [x] Pass: `planManifestSet`, `manifest.RepositoryOf`, `pinReference`; handler helpers `validateBumpVersion`, `findBumpArtifact`, `buildBumpTarget`, `repinOp`, `previousPin`, `bumpResolved`, `formatBumpResult`, `formatBumpDryRun`; comments only for WHY (the one on `notServedError` is rewritten because its WHY, "until bump exists", is gone) |

**Post-Phase-1 re-check**: all gates still pass. Installations with no pinned artifact see no change: the only shared code paths touched are the planning helper (behaviour-preserving) and the repository helper (same function, one home).

## Design binding

Every functional requirement of the spec is bound to the seam that satisfies it, so tasks can be derived mechanically.

| Spec | Design | Where |
|---|---|---|
| FR-001 | T6-01, R-21, section 4.6 | `cmd/bump.go`: `bumpCmd`, `bumpApplicationCmd` (alias `app`), `bumpResourceCmd` (alias `res`), persistent `-v`, `-t`, `-p`, `--dry-run`; `cfg.ResolveSpecsDir(bumpPath)` as `deploy` |
| FR-002 | T6-02, R-21, R2 | `findBumpArtifact`: lookup by kind and name; `--team` verified against `metadata.owner`; no ambiguity error is possible (design refinement 2) |
| FR-003 | T6-02, R-05, R1 | `prepareBump` reads the effective policy from `result.ManifestSet` after `planManifestSet` and refuses with the design's message before any backend call |
| FR-004 | T6-02, R-24, R2 | `findBumpArtifact` returns `<kind> "<name>": no manifest found in <dir>` |
| FR-005 | T6-02, R-24 | the handler never consults deployment records; `previousPin` tolerates `ErrImagePinNotFound`; the "no previous" output form |
| FR-006 | T6-03, R-22, R3 | `buildBumpTarget(image, version)` over `manifest.RepositoryOf`; a Resource's `Spec.Image` is the parser-filled reference |
| FR-007 | T6-03, R-22, R3 | `validateBumpVersion`, first statement of `prepareBump`, two anchored patterns, the exact message |
| FR-008 | T6-04, R-23, TD-8, TD-12, R4 | `ResolveImageOp.Repin`; `DockerBackend.repin` → `pinReference(…, ImageSourceRepinned)`; `Put` after pull and inspect; the handler wraps a failure as `<kind> "<name>": %w` |
| FR-009 | T6-04, R-23, R5 | `BumpBundle` has no engine; the handler calls only `ResolveImage`; asserted by the container-id and `Config.Image` checks in `TestBump` |
| FR-010 | T6-04, R-23, R6 | `previousPin` before, `formatBumpResult` after, over `manifest.ReadableVersion`; the `repinned` arm in `terminal_logger.go` |
| FR-011 | T6-05, R-25, R5 | `handler.BumpDryRun` shares `prepareBump` and prints `formatBumpDryRun`; `cmd/bump.go` builds no bundle for it |
| FR-012 | T6-06, R-14, R7 | `notServedError` reworded; backend unit test and the T3 suite's assertion updated |
| FR-013 | T6-07, R-30, R9 | `make docs-gen-cli` (three pages), manifest reference paragraph, `AGENTS.md` lines per the docs contract |
| FR-014 | G6 | existing suites untouched apart from the one reworded assertion; `planManifestSet` extraction keeps `Deploy`/`DryRun` output byte-identical (`TestDeployDryRun`) |
| FR-015 | ticket T6 scenarios, R8 | `tests/integration/bump_test.go`, seven scenarios on `newPinnedSuite`, written first |

## Project Structure

### Documentation (this feature)

```text
specs/036-bump-command/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0: decisions with the design section that settles each, and the refinements found
├── data-model.md        # Phase 1: Repin on the op, the pin after a bump, RepositoryOf, BumpBundle, handler values, planning helper, terminal arm
├── quickstart.md        # Phase 1: manual verification script (steps 1 and 2 need no daemon)
├── contracts/
│   ├── operator-output.md        # command surface, help text, exact success, dry-run, refusal, and failure output
│   ├── handler-and-backend.md    # signatures, dispatch order, rules the unit tests pin, cmd wiring
│   └── docs-and-deviations.md    # documentation changes, progress entry, design refinements to record
├── checklists/requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
cmd/
├── bump.go                      # NEW: bump + application (app) + resource (res); -v -t -p --dry-run; runBump(kind)
└── bump_test.go                 # NEW: arg-count errors, aliases resolve

internal/
├── app/
│   ├── app.go                   # + BumpBundle, BuildBumpBundle (registries, specs dir, observer pair, container backend)
│   └── app_test.go              # + bump bundle shape, slot prefixes, cleanup, fail-fast on specs dir
├── manifest/
│   ├── types.go                 # + RepositoryOf
│   └── types_test.go            # + RepositoryOf table
├── planner/
│   └── policy.go                # repositoryWithoutVersion → manifest.RepositoryOf (private copy removed)
├── engine/
│   ├── backends.go              # ResolveImageOp.Repin; ImageSourceRepinned
│   └── local/dockercontainer/
│       ├── docker_image.go      # repin branch; pinNewest → pinReference(ref, source); notServedError reworded; repositoryOf → manifest.RepositoryOf
│       └── docker_image_test.go # + repin cases; reworded message expectation
├── handler/
│   ├── bump.go                  # NEW: BumpOptions, Bump, BumpDryRun, prepareBump and the pure helpers
│   ├── bump_test.go             # NEW: helpers over an in-memory set, memImagePinStore, recording backend stub
│   ├── deploy.go                # planManifestSet extracted; Deploy and DryRun call it
│   └── deployments_test.go      # + memImagePinStore (shared fake) if not already present
└── ui/
    ├── terminal_logger.go       # + repinned arm
    └── terminal_logger_test.go  # + two byte-exact cases

tests/
└── integration/
    ├── bump_test.go                     # NEW, written first: TestBump on newPinnedSuite
    └── pinned_image_policy_test.go      # one assertion: "deploy the " → the bump clause

docs/content/cli/bump.md                    # generated
docs/content/cli/bump_application.md        # generated
docs/content/cli/bump_resource.md           # generated
docs/content/reference/manifest-schema.md   # "Moving a pin" paragraph; vanished-version sentence; describe clause
AGENTS.md                                   # quick start line, CLI reference section, cmd tree, pipeline note, pins.txt line
specs/progress.md                           # entry for 036
specs/epics/pinned-image-versions/design.md # refinements recorded
graphify-out/                               # graphify update .
```

**Structure Decision**: single-project Go layout already in place. The new command, bundle, and handler each land in the directory that owns their kind (`cmd/`, `internal/app/`, `internal/handler/`); the resolution change lands in the backend file that already owns the pin branches; the shared helpers land in the packages that already own their neighbours (`manifest` for reference helpers, `handler/deploy.go` for planning). The only new fixtures are built at run time by `newPinnedSuite`; `tests/testdata/pinned/` is unchanged.

## Deviations from the epic design

| Deviation | Why | Recorded |
|---|---|---|
| The effective policy comes from `planner.Plan` with a by-name filter, through `planManifestSet` shared with `Deploy` and `DryRun`, not from a direct `applyEffectivePullPolicy` call | `applyEffectivePullPolicy` is unexported and `Plan` is the one way to load "as deploy does", keeping the fixed-version rules in force | research R1; `design.md` 4.6 with the PR |
| `--team` verifies `metadata.owner`; there is no ambiguity error | a name is unique per manifest directory (`MergeManifest` rejects duplicates) | research R2; 4.6 and the R-21 reading |
| `manifest.RepositoryOf` replaces the planner's and the backend's private copies | three readers; constitution IV and VII | research R3; 4.6 step 3 |
| `Repin` carries the unexpanded target; the backend expands it, emits the started event with it, and records through `pinReference`; a repin under a manifest-owned policy is refused by the backend | one pull-inspect-pick-put sequence for first deploy and bump; defensive guard for TD-8 | research R4; 4.2 point 4, 4.6 step 4 |
| `--dry-run` runs `handler.BumpDryRun` with no bundle, no Docker client, no log | R-25 "writes nothing", parity with `deploy --dry-run` | research R5; 4.6 step 5 |
| The vanished-version clause reads `run "shrine bump <kind> <name>" to choose another version`, cause appended | T6-06; the subcommand word is the lower-cased kind | research R7; 4.2 point 3 |
| The T5 "pin differs" scenario is left on its `pins.txt` edit | it passes; edits to passing suites are not owed | research R8; section 5 |

## Complexity Tracking

No constitution principle violations to justify.
