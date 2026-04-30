# Implementation Plan: Fix Routing-Dir Manifest Scan Crash

**Branch**: `002-fix-routing-dir-manifest-scan` | **Date**: 2026-04-30 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/002-fix-routing-dir-manifest-scan/spec.md`

## Summary

`shrine deploy` crashes when the Traefik plugin's `routing-dir` is a subdirectory of `specsDir` (the documented default at `{specsDir}/traefik/`). The manifest scanner walks the tree, picks up the Traefik-generated YAML files, hands them to `manifest.Parse`, which probes `kind`, finds none it recognises, and returns `"unknown manifest kind"` — aborting the deploy.

The fix introduces a strict, two-step classification gate that runs once per candidate file and is shared by every directory scanner:

1. **Extension filter** (already in place): admit only `.yaml` / `.yml`.
2. **`apiVersion` classification** (new): a file is a shrine manifest **only** when its top-level `apiVersion` matches the regex `^shrine/v\d+([a-z]+\d+)?$`. Files that fail this check are classified **Foreign** and silently skipped. Files that pass it are then parsed, and any `kind`/spec defect fails loudly as today.

The classification helper lives in `internal/manifest/` (alongside `Parse` / `Validate`) and is consumed by both `internal/planner/loader.go` (`LoadDir`) and `internal/handler/teams.go` (`walkYAMLFiles` → `ApplyTeams`), so no scan path can crash on a foreign file. A single info-level notice lists foreign paths once per scan (FR-006). Malformed YAML inside a `.yaml`/`.yml` file remains a loud error (FR-004) since intent cannot be inferred from unparseable bytes.

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: `gopkg.in/yaml.v3` (already used by `internal/manifest/parser.go`); `regexp` (stdlib)
**Storage**: N/A — scanner is a read-only filesystem walk
**Testing**: `go test ./...` for unit tests; `go test -tags integration ./tests/integration/...` (or `make test-integration`) for the `NewDockerSuite` gate
**Target Platform**: Linux server (homelab Docker host)
**Project Type**: Single-binary Go CLI (`cmd/shrine`)
**Performance Goals**: Negligible — one extra regex match per admitted YAML file in `specsDir`; expected file counts are O(10s) per project
**Constraints**: Must not change behaviour for any project containing only valid shrine manifests; FR-007 explicitly forbids regression in parse/validation paths
**Scale/Scope**: Touches two scan call-sites (`internal/planner/loader.go`, `internal/handler/teams.go`), one new helper in `internal/manifest/`, plus unit + integration tests

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] N/A — pure scanner bug fix; no new manifest fields, no CLI flags |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] N/A — no new commands |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] N/A — no backend logic; touches `internal/manifest` and `internal/planner` only |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — one classification helper consumed by ≥2 existing scanners (`LoadDir`, `walkYAMLFiles`) and any future scanner; no speculative interfaces |
| V. Integration-Test Gate | Does this phase map to an integration test phase in the integration test suite using `NewDockerSuite` against a real binary? | [x] Pass — new scenario in [tests/integration/deploy_test.go](../../tests/integration/deploy_test.go) (or sibling) deploys a fixture where `specsDir` contains both valid manifests and a foreign YAML file (mimicking generated Traefik config), asserts success and exact applied-manifest set |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] N/A — no state writes |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | [x] Pass — the two existing copy-pasted `WalkDir` blocks (`getFiles` in [internal/planner/loader.go](../../internal/planner/loader.go) and `walkYAMLFiles` in [internal/handler/teams.go](../../internal/handler/teams.go)) collapse into one shared scanner; classification function is named for intent (`isShrineManifest`, `classifyManifestFile`) per the boolean-naming rule |

> No violations — Complexity Tracking table is intentionally empty.

## Project Structure

### Documentation (this feature)

```text
specs/002-fix-routing-dir-manifest-scan/
├── plan.md              # This file (/speckit-plan command output)
├── spec.md              # Feature specification (already exists)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── scanner-contract.md
└── tasks.md             # Phase 2 output (/speckit-tasks command)
```

### Source Code (repository root)

```text
cmd/
└── shrine/                          # CLI entrypoint — unchanged

internal/
├── manifest/
│   ├── parser.go                    # MODIFY: extract `probeKind` so it can be shared, OR add new file
│   ├── classify.go                  # NEW: regex `^shrine/v\d+([a-z]+\d+)?$`, `Classify(path) (Class, error)`
│   ├── classify_test.go             # NEW: unit tests for every spec edge case
│   ├── types.go                     # unchanged
│   └── validate.go                  # unchanged
├── planner/
│   ├── loader.go                    # MODIFY: LoadDir consumes shared scanner; emits foreign-file notice
│   └── loader_test.go               # MODIFY: add cases for foreign / mistyped-apiVersion / shrine-with-bad-kind
└── handler/
    └── teams.go                     # MODIFY: ApplyTeams uses shared scanner — no more "Skipping X: not a Team manifest" for foreign files

tests/
├── testdata/
│   └── deploy/
│       └── foreign-yaml/            # NEW fixture: valid app + a Traefik-shaped YAML alongside it
│           ├── team.yaml
│           ├── app.yaml
│           └── traefik/
│               └── traefik.yml      # No apiVersion (mirrors generator output)
└── integration/
    └── deploy_test.go               # MODIFY: add scenario "deploy succeeds when specsDir contains foreign YAML"
```

**Structure Decision**: The repo is a single-binary Go CLI (`cmd/shrine` + `internal/...`) with `tests/integration/` housing the `NewDockerSuite` gate. The fix is local to `internal/manifest/` (new helper) plus two existing scan call-sites in `internal/planner/` and `internal/handler/`. No new packages, no new commands, no new manifest kinds.

## Complexity Tracking

> No Constitution violations to track.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
