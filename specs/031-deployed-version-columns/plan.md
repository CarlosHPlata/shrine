# Implementation Plan: Deployed Version in get and describe

**Branch**: `031-deployed-version-columns` | **Date**: 2026-10-06 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/031-deployed-version-columns/spec.md`
**Epic binding**: [design.md](../epics/pinned-image-versions/design.md) sections 1, 3.3, 4.7; decisions TD-2 and TD-4; requirements T1-01 to T1-05. Ticket T1 of the Pinned Image Versions epic, issue #52.

## Summary

The deployment record grows by two trailing fields, the image reference as the
manifest wrote it and the effective pull policy, written by the Docker backend
every time it records a deployment and read back tolerantly so lines from earlier
releases still load. `shrine get deployed`, `get applications`, and `get resources`
gain a VERSION column after KIND that prints the recorded reference or `-`;
`shrine describe app` and `describe resource` print `Image:` and `Pull policy:`
lines. The reference is captured before the backend expands a `reg:` alias, so the
record shows what the operator wrote. No command, manifest, engine, or config-hash
change; the local store gains the injectable file operations the host-port store
already uses so its unit tests stay off the filesystem.

## Technical Context

**Language/Version**: Go 1.25 (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: Cobra (CLI), Docker SDK v28 (`client.FromEnv`), no new dependency
**Storage**: `<state-dir>/<team>/deployments.txt`, one space-separated line per artifact, written whole by atomic temp-and-rename; gains fields five and six (design section 3.3)
**Testing**: `go test ./...` for unit tests (no filesystem: the store is tested through injected read/write functions, the backend through the existing fake Docker API, the handler through string-returning formatters); `go vet -tags integration ./tests/integration/...` to compile the integration suite locally; `make test-integration` in CI as the gate
**Target Platform**: Linux single host with a local Docker daemon
**Project Type**: CLI tool with a pluggable-backend execution engine
**Performance Goals**: none beyond today; listing is a read of one small file per team
**Constraints**: legacy four-field and three-field lines must load with empty Image and Policy; the writer always writes six fields; the config hash inputs are unchanged (TD-2) so no container is recreated by the upgrade; `get` must not touch Docker; existing columns keep header, order, and values; image references never contain spaces
**Scale/Scope**: homelab scale, tens of records per team

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

The seams this feature touches are the ones the epic design names (section 1: deployment record, container reconcile, queries), so the gates pass by construction; the design section is cited per row.

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] Pass — no new capability and no manifest change; the feature records what the manifest already declares (`spec.image`, `spec.imagePullPolicy` or its derived value). No new flag. |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] Pass — no new command or flag; `get` and `describe` are read-only queries, `--team` stays optional. `cmd/get.go` and `cmd/describe.go` are untouched (T1-03). |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass — the record is written by the Docker `ContainerBackend` (`recordDeployment` in `docker_container.go`, design section 1 "Container reconcile"); `internal/engine/engine.go` is untouched; the dry-run backend records nothing, as today. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — two string fields on an existing struct (TD-4); the store adopts the injectable file-ops pattern `HostPortStore` already has (an existing pattern, not a new abstraction); two string-returning formatters follow `formatDeployPlan`. No new interface, store, or type. |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass — the `TestGetDocker` and `TestDescribeDocker` suites are extended first (version column after deploy; legacy record tolerated and healed); CI executes them (quickstart.md for the manual round-trip). |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] Pass — the record is still written only after `ContainerStart` succeeds, on both the fresh and the up-to-date path; the new fields ride the same write. The config hash keeps its inputs (TD-2), so the upgrade recreates nothing. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | [x] Pass — `newDeploymentRecord`, `fieldAt`, `valueOrUnknown`, `formatDeploymentsTable`, `formatDeploymentDetail`; the one comment added states why the reference is captured before alias expansion. |

**Post-Phase-1 re-check**: all gates still pass. Governance: this pull request touches `internal/engine/local/`, so its description carries the one-line Constitution Check the constitution requires.

## Project Structure

### Documentation (this feature)

```text
specs/031-deployed-version-columns/
├── plan.md              # This file
├── research.md          # Phase 0: decisions with alternatives, each bound to the design
├── data-model.md        # Phase 1: the extended record, file format, reader and writer rules
├── quickstart.md        # Phase 1: manual end-to-end verification
├── contracts/
│   ├── deployment-record.md   # deployments.txt line format and the store's tolerance rules
│   └── operator-output.md     # exact get table and describe output
├── checklists/requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

Exactly these files change.

```text
internal/
├── state/
│   ├── deployments.go                  # + Image, Policy on Deployment (design 3.3)
│   └── local/
│       ├── deployments.go              # injectable read/write file ops; loadTeam reads fields 5–6 as optional; saveTeam writes six fields (T1-02)
│       ├── deployments_test.go         # rewritten on in-memory file ops; legacy and six-field cases
│       └── hostports.go                # atomicWriteFile: temp-file prefix derived from the target name so both stores share it
├── engine/local/dockercontainer/
│   ├── docker_container.go             # CreateContainer captures the record before op.Image = expanded; recordDeployment fills Image + Policy (T1-01)
│   └── docker_container_record_test.go # NEW: unexpanded alias + policy recorded; up-to-date path refreshes a legacy record (T1-05)
└── handler/
    ├── deployments.go                  # formatDeploymentsTable (+ VERSION after KIND), formatDeploymentDetail (+ Image:, Pull policy:) (T1-03, T1-04)
    └── deployments_output_test.go      # NEW: column order, `-` fallback, describe lines

tests/integration/
├── get_test.go                         # + VERSION column after deploy; legacy records show `-`; redeploy heals
├── describe_test.go                    # + Image: and Pull policy: for app and resource; legacy shows `-`
└── testutils/
    ├── assert_general.go               # + AssertOutputLineContains (line-scoped stdout assertion)
    └── seed_state.go                   # NEW: SeedLegacyDeploymentRecords (rewrites deployments.txt to four fields)

AGENTS.md                               # State Directory Layout: the deployments.txt line
specs/progress.md                       # one entry for spec 031
CLAUDE.md                               # spec-kit plan pointer (workflow bookkeeping, as .specify/feature.json)
```

**Structure Decision**: single-project Go CLI layout already in place; no new package or directory. The two new test files sit beside the code they cover, named for what they pin. `cmd/`, `internal/engine/engine.go`, `internal/engine/backends.go`, the dry-run backend, `internal/manifest`, and `internal/planner` are untouched.

**Documentation site**: unchanged. No page under `docs/content/` documents the listing columns, the describe lines, or the `deployments.txt` format; the CLI reference is generated from the Cobra tree, which does not change, so `make docs-gen-cli` produces no diff.

## Complexity Tracking

No constitution principle violations to justify.

| Item | Why Needed | Simpler Alternative Rejected Because |
|------|------------|-------------------------------------|
| Injectable file operations on `DeploymentStore` | The project's test policy forbids filesystem access in unit tests, and the store's reader and writer are exactly what this feature changes | Keeping the existing `t.TempDir()` tests would leave the new parsing rules covered only by filesystem tests; the pattern already exists in `hostports.go`, so this is reuse, not a new abstraction |
| `format*` string-returning functions behind the existing `print*` wrappers | The table and the describe output are what the feature changes and must be unit-tested without capturing `os.Stdout` | Reassigning `os.Stdout` in tests is global, racy state; `formatDeployPlan` already sets the precedent for string-returning formatters in this package |
