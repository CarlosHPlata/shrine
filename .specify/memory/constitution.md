<!--
SYNC IMPACT REPORT
==================
Version change: 1.1.0 → 1.1.1
Added sections: N/A
Modified principles:
  - V. Integration-Test Gate: canonical gate updated from test/smock/ to
    tests/integration/ (the formal integration test suite with NewDockerSuite harness)
Removed sections: N/A
Templates requiring updates:
  ✅ .specify/templates/plan-template.md — Principle V gate question updated
  ✅ .specify/templates/spec-template.md — no structural changes required
  ✅ .specify/templates/tasks-template.md — no structural changes required
Deferred TODOs: None
-->

# Shrine Constitution

## Core Principles

### I. Declarative Manifest-First

All infrastructure MUST be expressed as YAML manifests. Three manifest kinds exist
and are non-negotiable: **Team** (namespace with quotas), **Resource** (managed
dependency with typed outputs), and **Application** (deployable container with
routing and dependency injection). Manifests MUST be provider-agnostic — they
describe *what* to run, not *how* a specific backend runs it. The manifest schema
follows `apiVersion: shrine/v1` and mirrors Kubernetes conventions intentionally,
lowering the barrier for homelab operators already familiar with kubectl.

Rules:
- New infrastructure capabilities MUST be exposed as manifest fields, not as CLI flags.
- Manifest parsing MUST be a two-pass pipeline: probe `kind`, then unmarshal.
- Validation MUST produce multi-error reports (no fail-fast on first error).
- Field-level constraints MUST be enforced at parse/validate time, not at runtime.

### II. Kubectl-Style CLI

The CLI MUST follow kubectl's verb-first convention: `shrine <verb> <resource>`.
Every write operation MUST support `--dry-run`, which produces human-readable output
with no side effects. Commands MUST be thin dispatchers — business logic lives in
`internal/handler/`, not in `cmd/`.

Rules:
- Text-in / text-out protocol: user-visible output → stdout; errors → stderr.
- `--team`/`-t` is ALWAYS optional; Shrine searches all teams automatically and
  uses `--team` only for disambiguation.
- Subcommand naming: `shrine apply teams`, `shrine status app <name>`,
  `shrine describe resource <name>` — resource type comes before the name.
- Adding a new command requires a corresponding dry-run code path if the command
  mutates Docker or network state.

### III. Pluggable Backend Architecture

The execution engine MUST communicate with infrastructure through three optional
interfaces: `ContainerBackend`, `RoutingBackend`, and `DNSBackend`. Nil backends
MUST be silently skipped — the engine never special-cases a missing backend.
Dry-run is implemented as a print-only backend, not as a conditional branch inside
the engine.

Rules:
- Backend interfaces are defined in `internal/engine/backends/`.
- No backend-specific logic is permitted inside `internal/engine/engine.go`.
- A new infrastructure capability (e.g., secrets vault, object storage) MUST be
  modelled as a new optional backend interface, not as engine core logic.
- Real backends live under `internal/engine/local/`; test/dry-run backends live
  beside the interface definitions.

### IV. Simplicity & YAGNI

Features MUST serve concrete, current needs. Abstractions are justified only when
three or more concrete usages exist. YAGNI is the default: do not design for
hypothetical future requirements. The smock round-trip in `test/smock/` is the
reference baseline — complexity that cannot be exercised there requires explicit
justification.

Rules:
- No repository pattern, service-locator, or factory unless the alternative
  (direct instantiation) demonstrably causes a maintenance problem.
- Hardcoded values (e.g., `10.200.0.0/24` for the platform network) are acceptable
  when a single instance is all that is needed now.
- Complexity violations MUST be documented in the plan's Complexity Tracking table
  with a concrete reason and the simpler alternative that was rejected.
- "Three similar lines is better than a premature abstraction."

### V. Integration-Test Gate

Every phase MUST end with a passing end-to-end round-trip before it is declared
complete. The canonical gate is the integration test suite at `tests/integration/`,
run with `go test -tags integration ./tests/integration/...` (or `make test-integration`).
Tests use the `NewDockerSuite` harness which builds the real shrine binary once in
`TestMain`, runs it as a subprocess against a live Docker daemon, and asserts results
at the filesystem, Docker, and YAML level. Unit tests supplement but NEVER substitute
for the integration gate.

Rules:
- Integration tests MUST run the real shrine binary as a subprocess (black-box).
  In-process testing via `cmd.Execute()` belongs in `cmd/cmd_test.go`, not here.
- Integration tests MUST use a real Docker daemon, not mocks. Mocking Docker
  diverges from production and masks failures.
- Each test suite MUST wire `BeforeEach`/`AfterEach` cleanup to prevent leftover
  containers and networks from contaminating subsequent tests.
- New backend implementations MUST be covered by an integration test scenario before
  merging.
- We will enforce TDD so integration test files are created before the implementation
  code.

### VI. Docker-Authoritative State

Docker container state is the source of truth. State files under the config/state
directories are a cache of beliefs. Reconciliation MUST query Docker by container
name on every operation. "Not found" during teardown is soft success. State records
are updated *after* Docker operations complete — never before.

Rules:
- `DeploymentStore` records a container-id only after `ContainerStart` succeeds.
- Teardown MUST succeed even if a container was manually removed outside Shrine.
- Subnet and secret stores use atomic temp+rename writes with `sync.Mutex` for
  in-process serialization.
- No in-memory caching of secret values — always read from disk.

### VII. Clean Code & Readability

Code MUST follow clean code standards. Repeated logic MUST be extracted into named
functions rather than duplicated inline. Well-named private and helper functions
are the primary communication tool — comments are a last resort, reserved for
non-obvious WHY, never for describing WHAT the code does.

Rules:
- **DRY**: Any logic appearing in two or more places MUST be extracted into a shared
  private or helper function with a clear, intention-revealing name.
- **Descriptive naming over comments**: prefer `ensureNetworkExists()` over a block
  comment explaining what the next ten lines do. Function names MUST read like
  sentences: `resolveValueFrom`, `flattenEnvVars`, `validateManifestOutputs`.
- **Boolean Methods naming**: boolean methods should always start with `is`, `has`, or `should`.
  e.g. prefer `shouldNetworkExists()` over `checkNetworkExists()`.
- **No inline documentation of WHAT**: do not explain what the code does in a
  comment — the code should be self-explaining through naming.
- **Comments are for WHY only**: a hidden constraint, a subtle invariant, a
  workaround for a specific upstream bug. One short line max; no multi-line
  comment blocks.
- **Private helpers over duplication**: when a behaviour needs to be shared across
  two call sites, extract it immediately — do not defer the refactor.

## Technical Stack & Constraints

**Language**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**CLI framework**: Cobra (verb-first subcommand tree under `cmd/`)
**Container runtime**: Docker SDK via `client.FromEnv` + API version negotiation
**Manifest pipeline**: parse → validate → plan (resolve deps + quotas) → resolve
  (secrets + templates) → execute (backend calls)
**Networking model**:
- Per-team bridge: `shrine.<team>.private`, `/24` subnet from `10.100.5.0/24`
  through `10.100.255.0/24` (allocated by `SubnetStore`)
- Platform bridge: `shrine.platform`, `10.200.0.0/24` (hardcoded, single instance)
- External access: Traefik only; host-port publishing is unsupported by design
**State directories**: Flag > Env > `.env` (godotenv) > XDG/FHS defaults
**Template engine**: Go `text/template` with topological sort (Kahn's algorithm)
  and cycle detection for both Resource outputs and Application env vars
**Testing**: `go test ./...`; integration via `test/integration`

## Development Workflow

Feature work follows a phased model tracked in `specs/progress.md`. Each phase has
a defined gate (see Principle V). New features MUST have a spec in
`specs/features/<name>.md` before implementation begins.

Workflow:
1. Write or update the feature spec in `specs/features/`.
2. Update `specs/progress.md` with the new phase and its acceptance criteria.
3. Plan and write the expected integration test scenarios in `test/integration/`.
4. Implement in small, commit-able increments.
5. Run `go test ./...` after each logical change.
6. Execute the integration scenarios (`go test -tags integration ./tests/integration/...` or `make test-integration`) as the final gate before marking the phase complete.
7. Update `specs/progress.md` to `[x]` and commit.

CLI commands MUST be self-documenting via `--help`. The `AGENTS.md` file at the
repository root is the canonical quick-reference for AI assistants and new
contributors; keep it synchronized with CLI changes.

## Governance

This Constitution supersedes all other development practices and conventions. When
a code review, spec, or task conflicts with a principle, the Constitution wins.
Amendments require: (1) a written rationale, (2) a version bump per the semantic
versioning policy below, (3) propagation across all dependent templates and docs.

**Versioning policy**:
- MAJOR: Principle removal, redefinition that breaks existing manifests or workflows.
- MINOR: New principle or section added, material expansion of existing guidance.
- PATCH: Clarifications, wording, or typo fixes with no semantic change.

**Compliance**: All PRs touching `internal/engine/`, `internal/manifest/`, or
`cmd/` MUST include a one-line Constitution Check confirming no principles are
violated. If a violation is necessary, it MUST be documented in the plan's
Complexity Tracking table.

**Guidance file**: `AGENTS.md` is the runtime development reference. It MUST be
kept consistent with this Constitution.

**Version**: 1.1.1 | **Ratified**: 2026-04-15 | **Last Amended**: 2026-04-29
