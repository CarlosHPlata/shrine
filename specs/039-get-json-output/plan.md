# Implementation Plan: Machine-Readable Output for `shrine get`

**Branch**: `039-get-json-output` | **Date**: 2026-10-11 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/039-get-json-output/spec.md`

## Summary

`shrine get deployed`, `get applications`, `get resources` and `get teams` gain a
local `--json` flag. With it, each command prints one JSON document,
`{"data": [...]}`, instead of the table. Workload entries carry what `describe`
shows for the artifact plus the table's version: team, name, kind, image, pull
policy, version, a nested `pin` (or `null`), the live `runningImage` (or `null`
when the runtime cannot report it), the full container ID and the full config
hash. Team entries carry name, display name and the two quotas as numbers. The
cmd layer branches on the flag; the handler gains four `List…JSON` functions
built on helpers extracted from the table path (`listDeployments`, `pinOf`,
`versionOf`, `inspectRunningImage`), so the table and `describe` keep their
output byte for byte and the table stays state-only. The JSON path builds a
query container backend like `describe` does, but a backend it cannot build or
reach never fails the listing.

## Technical Context

**Language/Version**: Go 1.25 (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: Cobra (CLI), standard library `encoding/json`, Docker SDK v28 through the existing `engine.ContainerBackend.InspectContainer`; no new dependency
**Storage**: none changed; reads `<state-dir>/<team>/deployments.txt`, `pins.txt` and the team store as the table does
**Testing**: `go test ./...` for unit tests (formatters return bytes and are decoded in-test; container access through the existing `imageBackend` fake; no filesystem, no stdout capture); `go vet -tags integration ./tests/integration/...` to compile the integration suite locally; `make test-integration` in CI as the gate
**Target Platform**: Linux single host with a local Docker daemon (optional for this feature)
**Project Type**: CLI tool with a pluggable-backend execution engine
**Performance Goals**: a `--json` listing of tens of workloads completes in well under a second with the daemon up, and without waiting on timeouts with it down (refused connections fail fast; research R3, R4)
**Constraints**: output without `--json` is byte-for-byte unchanged and the table touches no container runtime (FR-011, FR-003a); stdout holds exactly one JSON document or nothing (FR-002, FR-009); every key present in every entry (FR-007); empty listing is `{"data": []}` (FR-008)
**Scale/Scope**: homelab scale, tens of records per team

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] Pass — no infrastructure capability is added; `--json` selects the output format of a read-only query, which is not something a manifest can express. No manifest change. |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] Pass — no new command; a flag on four existing read-only `get` subcommands (no `--dry-run` needed: nothing is mutated). `--team` stays optional. The JSON goes to stdout, errors to stderr (FR-002, FR-009). `cmd/get.go` only branches on the flag; the logic lives in `internal/handler/`. |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass — the running image is read through the existing `ContainerBackend.InspectContainer`, built by `app.NewQueryContainerBackend` as `describe` does; a nil backend is handled, not special-cased in the engine. `internal/engine/` is untouched. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — one generic envelope type serves both entry types (research R6); helpers are extracted from existing code with two call sites each (data-model "Helpers"). No options struct, no format enum, no worker pool, no `-o` selector (research R1, R2, R4). |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass — `TestGetNoDocker`, `TestGetDocker` and `TestPinnedVersionQueries` are extended first (research R8), driving the real binary; CI executes them. |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] Pass / N/A — nothing is written. `runningImage` is read live from Docker, the authority for what a container runs, rather than inferred from state. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | [x] Pass — `listDeployments`, `pinOf`, `versionOf`, `inspectRunningImage`, `encodeListing` each replace logic that would otherwise be duplicated between table, `describe` and JSON paths; `pinOf` uses the `(value, ok)` idiom and `inspectRunningImage` returns its error so `describe` can print it. |

**Post-Phase-1 re-check**: all gates still pass. Governance: this pull request touches `cmd/`, so its description carries the one-line Constitution Check.

## Project Structure

### Documentation (this feature)

```text
specs/039-get-json-output/
├── plan.md              # This file
├── research.md          # Phase 0: R1–R8 decisions with alternatives
├── data-model.md        # Phase 1: output types, field sources, shared helpers
├── quickstart.md        # Phase 1: manual end-to-end verification with jq
├── contracts/
│   └── cli-json-output.md   # flag surface, streams/exit codes, exact document shapes
├── checklists/requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
cmd/
└── get.go                              # + getJSONFlag; --json on teams/applications/resources/deployed; RunE picks table or JSON; JSON workload path builds the query backend, nil on failure (R1, R3); Long help gains a --json paragraph

internal/handler/
├── deployments.go                      # + listDeployments (table List* use it); pinOf, versionOf (versionCell wraps it); inspectRunningImage (runningImage wraps it)
├── deployments_json.go                 # NEW: deploymentEntry, pinEntry, toDeploymentEntry, runningImageOrNull, formatDeploymentsJSON, ListDeployedJSON/ListApplicationsJSON/ListResourcesJSON
├── deployments_json_test.go            # NEW: shape, key order, nulls, "-" → "", pin cases, runningImage cases, RFC 3339, empty listing, escaping
├── json_listing.go                     # NEW: jsonListing[T], encodeListing, printListing (stdout only after encode succeeds)
├── json_listing_test.go                # NEW: empty/nil → {"data": []}, escaping round-trip
├── teams.go                            # + ListTeamsJSON, teamEntry, formatTeamsJSON
└── teams_json_test.go                  # NEW: fields, numeric quotas, empty listing

tests/integration/
├── get_test.go                         # TestGetNoDocker: teams JSON, empty deployed JSON, seeded record with DOCKER_HOST closed → runningImage null, config-dir --json fails; TestGetDocker: three workload commands ± --team match the table, live runningImage, full container ID
└── pinned_version_queries_test.go      # + "get --json carries the pin describe shows"

docs/content/cli/get_teams.md, get_applications.md, get_resources.md, get_deployed.md   # regenerated (make docs-gen-cli)
docs/content/guides/image-versions.md   # "See which version runs": one sentence + example pointing to --json
AGENTS.md                               # CLI Reference: new "shrine get deployed/applications/resources/teams" entry for --json
specs/progress.md                       # 039 entry
CLAUDE.md                               # spec-kit plan pointer
```

**Structure Decision**: the existing single-project Go CLI layout; no new package. JSON output gets its own handler files beside the table code so `deployments.go` (already the largest handler) does not grow by the whole feature, and the shared envelope sits in `json_listing.go` because both `deployments_json.go` and `teams.go` use it. `internal/engine/`, `internal/state/`, `internal/manifest/` and `internal/app/` are untouched.

## Complexity Tracking

No constitution principle violations to justify.

| Item | Why Needed | Simpler Alternative Rejected Because |
|------|------------|-------------------------------------|
| Generic `jsonListing[T]` | Two entry types share one envelope and one encode-then-print rule (non-nil `data`, no partial output) | Two hand-written wrapper structs duplicate the envelope and the empty-slice rule; `map[string]any` loses key order and types |
| Separate `List…JSON` entry points | The JSON workload path needs a container backend the table path must not take (FR-003a) | One function with a format parameter forces an unused backend onto the table path and hides the state-only guarantee |
