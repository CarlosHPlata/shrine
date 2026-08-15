# Implementation Plan: Expand `reg:` Registry Aliases Before the Container Is Created

**Branch**: `022-fix-registry-alias-expansion` | **Date**: 2026-08-14 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/022-fix-registry-alias-expansion/spec.md`

## Summary

Fix issue #33: a `reg:<alias>/<path>:<tag>` image reference is expanded to the
real registry host only inside `resolveImage`, in a local that is discarded, so
`ContainerCreate` receives the raw `reg:` string and rejects every fresh deploy
with `invalid reference format`. The fix expands the alias exactly once at the
top of `DockerBackend.CreateContainer` into the by-value op, so the pull, the
credential lookup, the config hash, and the container spec all agree on one
expanded reference (research.md D1). Two operator-facing diagnostics ride
along: container-creation errors now name the rejected image reference
(D5), and the terminal renderer stops printing phantom "Creating container"
lines on failure (D5). A package-internal `dockerAPI` interface (D2) provides
the observation seam the spec's verification requirements demand, enabling a
unit regression test on the exact container spec handed to Docker, plus real
non-dry-run integration deploys for applications and resources (D3, D4).

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: existing Docker SDK (`github.com/docker/docker/client`),
`containerd/errdefs` for not-found sentinel in the test fake. No new
third-party dependencies.
**Storage**: none new. Deployment/secret/subnet stores untouched; the unit
test deliberately stops before any state write (research.md D3).
**Testing**: `go test ./...` unit gate (no filesystem, no daemon — project
rule); `make test-integration` (`go test -tags integration ./tests/integration/...`)
with `NewDockerSuite` against a real Docker daemon as the Principle V gate.
Fail-first ordering per spec VR-003.
**Target Platform**: Linux server (homelab Docker host), unchanged.
**Project Type**: single-binary Go CLI (`cmd/shrine`), unchanged.
**Performance Goals**: nil — one string prefix expansion per container
creation, invisible against a Docker API round-trip.
**Constraints**: dry-run must keep displaying the alias form (spec FR-007 —
existing assertions must pass unmodified); unit tests must not touch the
filesystem; expansion semantics must stay identical to the planner's
validation view of `reg:` refs (spec edge case).
**Scale/Scope**: three production files touched
(`docker_container.go`, `docker_image.go`, `terminal_logger.go`), one new
production file (`docker_api.go`), one field-type change
(`docker_backend.go`), one test helper, one unit-test file, extended
integration suite + two fixture dirs.

## Constitution Check

*GATE: passed pre-Phase 0; re-checked after Phase 1 design — still passing.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | Pass — no new capability; existing `reg:` manifest syntax finally works as specified in 014. No schema, CLI-flag, or config change. |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | N/A — no new commands. Dry-run parity is preserved and pinned (FR-007/SC-005). |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | Pass — the fix lives entirely inside `DockerBackend`; `internal/engine/engine.go` is untouched. The renderer guard is presentation-layer (`internal/ui`). `dockerAPI` is package-internal to the backend, not a new engine interface. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | Violation, justified — `dockerAPI` is one interface with one production implementation and one test fake. See Complexity Tracking. |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | Pass — real non-dry-run deploys for alias app, alias resource, and form-equivalence, written red-first (spec VR-001/VR-003). This closes 014's gap, where the gate was satisfied by dry-run-only tests. |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | Pass — `recordDeployment` ordering unchanged; no state-layer edits. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | Pass — expansion gains a single owner instead of two; `resolveImage` gets a one-line WHY comment stating the caller owns expansion (a hidden invariant, the permitted comment kind). |

## Project Structure

### Documentation (this feature)

```text
specs/022-fix-registry-alias-expansion/
├── plan.md              # This file
├── spec.md              # Feature specification (with Verification Requirements)
├── research.md          # Phase 0 — decisions D1–D6
├── data-model.md        # Phase 1 — reference forms, invariants, touched types
├── quickstart.md        # Phase 1 — manual repro/verify + test gates
├── contracts/
│   └── deploy-diagnostics.md  # Operator-facing output contract
├── checklists/
│   └── requirements.md  # Spec quality checklist (16/16)
└── tasks.md             # Phase 2 — /speckit-tasks output (NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/
├── engine/local/dockercontainer/
│   ├── docker_api.go            # NEW — dockerAPI seam (12 methods, D2)
│   ├── docker_backend.go        # client field type → dockerAPI
│   ├── docker_container.go      # expansion in CreateContainer; error enrichment
│   ├── docker_image.go          # expansion removed; caller-owns-expansion contract
│   └── docker_container_test.go # NEW — fake-backed container-spec regression test
└── ui/
    ├── terminal_logger.go       # container.create case guarded on StatusInfo
    └── terminal_logger_test.go  # extended (or created) for the progress-line contract

tests/
├── integration/
│   ├── registry_alias_test.go   # + real-deploy tests (app, resource, equivalence)
│   └── testutils/
│       └── assert_docker.go     # + AssertContainerImage helper
└── testdata/deploy/
    ├── registry-alias/               # reused — alias → docker.io (pullable)
    ├── registry-alias-resource/      # reused — alias resource fixture
    ├── registry-alias-eq-full/       # NEW — fully-qualified form of the same app
    └── registry-alias-eq-alias/      # NEW — alias form of the same app (FR-010)
```

**Structure Decision**: Single-project layout as today. All production changes
sit inside the existing Docker backend package and the terminal renderer; the
engine core, planner, manifest schema, and handlers are untouched. Test
changes follow the established split: package-internal unit tests beside the
code, black-box integration tests under `tests/integration/` with fixtures
under `tests/testdata/deploy/`.

## Implementation Flow (input to /speckit-tasks)

Ordering is dictated by the spec's Verification Requirements — the seam is a
pure refactor and lands first so every test can be seen red before the fix:

1. **Seam (no behaviour change)**: add `docker_api.go`, retype
   `DockerBackend.client`. `go test ./...` and a build prove neutrality.
2. **Red tests** (VR-003):
   - Unit: `docker_container_test.go` — fake `dockerAPI`, capture-then-error
     `ContainerCreate`, assert captured `Config.Image` is the expanded form
     (fails: still `reg:…`). No filesystem, `nil` state store (D3).
   - Integration: real-deploy alias app + alias resource tests and
     `AssertContainerImage` helper (fail: `invalid reference format`); dry-run
     assertions untouched and still green; equivalence fixtures + no-recreate
     test (fails the same way on the alias leg).
   - UI: renderer test asserting one `🏗️` line per attempt (fails: three).
3. **Fix**: expansion into `op` at top of `CreateContainer`; remove expansion
   from `resolveImage` (+ WHY comment); error enrichment per
   contracts/deploy-diagnostics.md; `StatusInfo` guard in the renderer.
4. **Green + gates**: `go test ./...`, then `make test-integration` as the
   final Principle V gate. The traceability mapping (VR-004) is produced by
   /speckit-tasks and checked by /speckit-analyze before implementation.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| `dockerAPI` interface (Principle IV: one interface, one production impl + one test fake — not 3 usages) | Spec SC-003/VR-001/VR-002 require asserting on the container specification at the point of creation; with a concrete `*client.Client` field, `docker_container.go` is structurally untestable without a live daemon — the exact gap that shipped #33 undetected | "No seam, integration-only coverage" keeps the assertion solely in the ~10-minute suite and leaves the package with zero fast regression coverage; structural satisfaction means the seam costs one field type and one file, with no call-site or behaviour change |
