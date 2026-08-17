# Implementation Plan: Publish Application Ports on Localhost

**Branch**: `023-publish-host-ports` | **Date**: 2026-08-17 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/023-publish-host-ports/spec.md`

## Summary

Add an opt-in `spec.networking.publish` manifest field that binds an application's
declared service port to the host's loopback interface (`localhost:<port>`), either
on an explicit host port or on one allocated automatically from a dedicated range.
Explicit ports are conflict-checked in the planner at both dry-run and deploy —
against other explicit claims in the manifest set, the Traefik gateway's reserved
host ports, and persisted allocations held by other applications. Automatic
allocations are persisted in a new `HostPortStore` (mirroring `SubnetStore`) so an
application keeps its port across redeploys, container recreation, and teardown;
allocations are released only by `shrine delete team` or a new `shrine delete
application` subcommand. Publishing implies attachment to the platform network
without requiring `exposeToPlatform`, and grants no cross-team dependency access.

The port-binding plumbing (`CreateContainerOp.PortBindings` → `buildPortBindings` →
Docker `HostConfig`) already exists end-to-end for the Traefik container; this
feature routes application manifests through it and adds loopback binding,
allocation state, and planner conflict detection.

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: Cobra (CLI), Docker SDK (`client.FromEnv`, `nat` port types), `gopkg.in/yaml.v3` (manifest parsing)
**Storage**: plain-text state files under the state directory — new `hostports.txt` (`team/app=port` lines, atomic temp+rename, `sync.Mutex`), mirroring `subnets.txt`
**Testing**: `go test ./...` for unit tests (no filesystem access — stores use injectable file-op functions); `make test-integration` (`NewDockerSuite`, real binary + real Docker) as the phase gate
**Target Platform**: Linux single-host with a local Docker daemon
**Project Type**: CLI tool with a pluggable-backend execution engine
**Performance Goals**: N/A beyond existing deploy latency — allocation is an in-memory scan of ≤2768 slots plus one file write
**Constraints**: dry-run must be side-effect free (no allocation, no state writes); allocations must be deterministic and stable across redeploys; existing container config-hashes must not be invalidated for apps that do not opt in
**Scale/Scope**: homelab scale — automatic range 30000–32767 (2768 ports); one published port per application

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] Pass — `spec.networking.publish` field; range/exclusivity enforced in `manifest.Validate` with multi-error reporting |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] Pass — no new deploy flags; new `shrine delete application <name>` follows the existing `delete team` pattern, `--team` optional, `--dry-run` supported |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass — port resolution/allocation happens inside the Docker `ContainerBackend` (mirroring subnet allocation in `CreateNetwork`); the engine only projects manifest → `CreateContainerOp`; dry-run remains a print-only backend |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — `HostPortStore` mirrors the established `SubnetStore` pattern (2nd concrete usage of an existing pattern, not a new abstraction); no new backend interface; hardcoded automatic range |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass — new `tests/integration/publish_test.go` scenarios written before implementation (TDD); see quickstart.md for the manual round-trip |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] Pass with precedent note — deployment records stay post-`ContainerStart`; the port allocation record is persisted at allocation time (immediately before container create), exactly as `AllocateSubnet` persists before network create; `delete application` queries Docker by name and refuses while the container exists |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | [x] Pass — `shouldAttachToPlatform()`, `resolvePublishBinding()`, `DetectHostPortCollisions()`; no WHAT comments |

**Post-Phase-1 re-check**: all gates still pass. One governance item (not a principle
violation): the constitution's Technical Stack section currently states "External
access: Traefik only; host-port publishing is unsupported by design". This feature
supersedes that line by design decision (see research.md R11); the plan includes a
constitution amendment (MINOR bump 1.1.1 → 1.2.0) updating it to "External access:
Traefik by default; per-application loopback-only host publishing via
`networking.publish`". The stale note at `specs/progress.md:101` is updated in the
same change.

## Project Structure

### Documentation (this feature)

```text
specs/023-publish-host-ports/
├── plan.md              # This file
├── research.md          # Phase 0 output — all design decisions with alternatives
├── data-model.md        # Phase 1 output — types, store, state formats, projections
├── quickstart.md        # Phase 1 output — manual end-to-end verification script
├── contracts/           # Phase 1 output
│   ├── manifest-schema.md       # networking.publish field contract + combination table
│   ├── planner-errors.md        # conflict error formats (dry-run and deploy)
│   ├── hostport-store.md        # HostPortStore interface + hostports.txt format
│   └── operator-output.md       # dry-run / deploy / delete output contract
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/
├── manifest/
│   ├── types.go             # + Publish type on Networking; custom bool|map unmarshaler
│   └── validate.go          # + explicit-port range checks in validateApplicationSpec
├── planner/
│   ├── hostports.go         # NEW — DetectHostPortCollisions (sibling of collisions.go)
│   ├── hostports_test.go    # NEW
│   └── plan.go              # + PortContext param; collision call for all filters
├── state/
│   ├── hostports.go         # NEW — HostPortStore interface, HostPortMap, sentinels
│   ├── state.go             # + HostPorts field on Store
│   └── local/
│       ├── hostports.go     # NEW — hostports.txt store (mirrors local/subnets.go)
│       ├── hostports_test.go# NEW — injected file-ops, no real filesystem
│       └── local.go         # + wire NewHostPortStore (reserved ports seeded here)
├── engine/
│   ├── backends.go          # + PublishPort op field; + HostIP on PortBinding
│   ├── engine.go            # + shouldAttachToPlatform derivation; + op.Publish projection
│   ├── dryrun/
│   │   ├── dry_run_engine.go    # + persisted-port snapshot param
│   │   └── dry_run_container.go # + publish + implied-attachment print lines
│   └── local/dockercontainer/
│       └── docker_container.go  # + resolvePublishBinding (claim/allocate); hash update
├── config/                  # unchanged — Traefik ports already exposed; read as reserved set
└── handler/
    ├── deploy.go            # + build PortContext for Plan; pass snapshot to dry-run
    ├── deployments.go       # + DeleteApplication handler
    └── teams.go             # + release team host ports in DeleteTeam

cmd/
└── delete.go                # + `shrine delete application <name>` subcommand

tests/integration/
└── publish_test.go          # NEW — written first (TDD), gates every phase

docs/content/reference/
└── manifest-schema.md       # + networking.publish reference + combination table
docs/content/guides/
└── publish-localhost.md     # NEW — how-to guide (US5)
```

**Structure Decision**: single-project Go CLI layout already in place; no new
top-level directories. New files follow the exact placement conventions of their
siblings (`planner/collisions.go`, `state/local/subnets.go`).

## Complexity Tracking

No constitution principle violations to justify.

| Item | Why Needed | Simpler Alternative Rejected Because |
|------|------------|-------------------------------------|
| Custom `yaml.v3` unmarshaler for `publish: true \| {hostPort: N}` | Single manifest option with two modes (FR-001, SC-004) | Two sibling fields (`publish` + `publishPort`) split one concept across two keys and reintroduce "which flag do I set" ambiguity; an `{}`-only object form makes the common automatic case read as `publish: {}` |
| Constitution amendment (Technical Stack line) | The stack section explicitly declares host publishing unsupported; governance requires amendment before merging | Ignoring the line would make the constitution and shipped behavior contradict each other |
