# Feature Specification: Separate Composition Root from `internal/handler/`

**Feature Branch**: `017-refactor-composition-root`
**Created**: 2026-05-15
**Status**: Draft
**Input**: User description: "based on https://github.com/CarlosHPlata/shrine/issues/24"
**Source Issue**: [#24 — design: handler/ is doing composition-root work; reconsider package responsibilities](https://github.com/CarlosHPlata/shrine/issues/24)

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Adding a new command does not require copy-pasting wiring (Priority: P1)

A maintainer is asked to add a new `shrine` command — for example `describe`, `status`, or any future command that needs to read state, interact with the routing backend, or talk to the secrets vault. Today they would have to copy the same construction sequence already present in `deploy.go` and `apply.go` (validate registries, build observers + log files, construct the secrets vault plugin, construct the routing backend plugin, construct the engine) into the new handler. Each copy then drifts independently as plugins evolve. After this change, the maintainer obtains a fully constructed dependency set from a single place and writes only the request-shaped logic in the handler.

**Why this priority**: This is the active failure mode the issue calls out. The wiring is **already** duplicated between `deploy.go` and `apply.go` (issue #3 was a previous instance of the same drift). Every additional command added before this change deepens the duplication and makes any future plugin signature change a multi-file edit. Closing this gap is the minimum change required to address the issue.

**Independent Test**: Add a trivial second consumer of the composed dependencies (or convert one existing handler) such that it obtains its backend, vault plugin, observer set, and engine from the new composition site rather than constructing them inline. Verify that the handler file no longer imports the plugin construction packages and that the handler body is shorter than today's `Deploy` / `ApplySingle`.

**Acceptance Scenarios**:

1. **Given** a handler in `internal/handler/` after this change, **When** a reader inspects its imports, **Then** it does not import plugin constructors (`plugins/gateway/...`, `plugins/secrets/...`), engine constructors (`engine/local`), or logger/observer constructors directly — those dependencies arrive as already-constructed values.
2. **Given** a maintainer adds a hypothetical new command that needs the same dependency set, **When** they wire it up, **Then** they reuse the existing composition entry point and write zero new lines that duplicate plugin / engine / observer construction.
3. **Given** the `apply` and `deploy` flows after this change, **When** their handler bodies are compared, **Then** the construction code that currently differs only by accident (vault constructed in `apply` and `deploy` separately, observers constructed in both) appears in exactly one source location.

---

### User Story 2 — Plugin and backend signature changes are a single-location edit (Priority: P2)

A maintainer changes the signature, configuration shape, or construction error contract of a shared dependency — for example, adding a parameter to `infisical.New`, renaming a field on the Traefik plugin config, or changing how the local engine receives its observer. Today this requires hunting every handler that constructs that dependency and updating each one identically; a missed site silently keeps using the old shape until the affected command runs. After this change, the construction site is unique, so the change is a single edit and the compiler catches incomplete migrations.

**Why this priority**: This makes the P1 benefit durable. Without it, the codebase could re-grow duplicated wiring the next time a new command lands; with it, the structure itself prevents regression. Secondary because it does not change observable CLI behavior on its own.

**Independent Test**: Pick any plugin/engine constructor currently invoked from a handler. Grep for its constructor name in `internal/handler/` after the change — there should be zero matches. Then change that constructor's signature (locally) and confirm only one file in the project needs updating to compile.

**Acceptance Scenarios**:

1. **Given** the codebase after this change, **When** the user greps for `infisicalplugin.New`, `traefik.New`, `local.NewLocalEngine`, `ui.NewTerminalObserver`, or `ui.NewFileLogger` inside `internal/handler/`, **Then** there are no matches.
2. **Given** a maintainer modifies the signature of any one of those constructors, **When** they recompile, **Then** the only handler-layer file that requires editing is the composition site itself (not each individual handler).

---

### User Story 3 — Handlers can be exercised in tests without booting the full plugin stack (Priority: P3)

A maintainer writes a unit test for a handler's request-shaped logic — input validation, output formatting, error mapping. Today, exercising a handler such as `Deploy` requires either constructing real plugin instances (Infisical, Traefik) or accepting that the handler is effectively untestable in isolation. After this change, the handler accepts its dependencies as already-constructed values, so a test can supply test doubles for those collaborators and exercise just the handler's own behaviour.

**Why this priority**: Improves long-term maintainability and unlocks targeted unit tests, but the immediate user-visible value is smaller than P1/P2. Listed so that the design satisfies it rather than accidentally precluding it.

**Independent Test**: Identify one handler whose business logic (e.g., error mapping, output formatting) is currently entangled with plugin construction. After the refactor, write a unit test that exercises that handler with stand-in dependency values without invoking real plugin constructors. The test must run without environment variables, network, or filesystem state required by the real plugins.

**Acceptance Scenarios**:

1. **Given** a handler after this change, **When** a test calls it, **Then** the test can supply the handler's dependencies directly (as values or interfaces) without invoking `infisicalplugin.New`, `traefik.New`, `local.NewLocalEngine`, or any other real plugin constructor.
2. **Given** such a test, **When** it runs, **Then** it does not require Infisical credentials, a reachable Traefik instance, or any state the real plugins would normally need.

---

### Edge Cases

- **Handlers that legitimately need to construct something per request** (e.g., a fresh file-backed log writer for that specific deployment run): the design must still allow this — composition is for shared, reusable graph nodes, not for per-invocation allocations that must live and die with the request.
- **Commands that need only a subset of the dependency graph** (e.g., a hypothetical `status` command that needs the state store but not the routing backend): the composition site must permit a command to take only what it needs, without forcing every command to construct or accept the entire graph.
- **Construction errors** (e.g., Infisical credentials missing, Traefik config invalid): the failure mode and message surface must remain at least as informative as today — a composition failure must not be silently downgraded to a generic "wiring error" that hides the underlying cause.
- **Existing tests in `internal/handler/`** (e.g., `status_test.go`): the refactor must not break or weaken existing test coverage; existing assertions on handler behaviour must continue to pass.
- **Backward compatibility of the CLI surface**: this is an internal restructure; the user-facing `shrine` commands, their flags, output, and exit codes must remain identical.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Files under `internal/handler/` MUST NOT directly construct shared dependencies — specifically the routing backend plugin, the secrets vault plugin, the deploy engine, the terminal observer, or the file logger. They MUST receive these as already-constructed values from a dedicated composition entry point.
- **FR-002**: There MUST be exactly one source location at which each shared dependency named in FR-001 is constructed for normal CLI execution. Future additions of commands or handlers MUST reuse that location rather than re-construct the dependency.
- **FR-003**: Each handler MUST be able to obtain only the subset of dependencies it needs. The design MUST NOT force a command that does not require, for example, a routing backend, to construct or accept one.
- **FR-004**: Composition failures (invalid config, missing credentials, plugin construction errors) MUST surface to the user with at least the same level of detail and the same exit-code semantics as today. No new generic wrapper error may obscure the underlying cause.
- **FR-005**: All existing `shrine` commands (`deploy`, `deploy --dry-run`, `apply`, `teardown`, `status`, `describe`, `generate`, `get`, `create`, `update`, `delete`, `version`) MUST behave identically after the change with respect to flags accepted, stdout/stderr output, and exit codes. No CLI behaviour regression is permitted.
- **FR-006**: Existing tests under `internal/handler/` MUST continue to pass without weakening their assertions. New tests MAY be added that exercise composition or handlers in isolation.
- **FR-007**: After this change, a contributor MUST be able to add a new command that consumes the existing dependency graph without writing any new plugin, engine, or observer construction code in `internal/handler/`.
- **FR-008**: The scope of this refactor MUST cover, at minimum, the wiring duplicated today between `deploy.go` and `apply.go` (vault plugin construction, observer + file logger construction, local engine construction). Any other handler that today inlines construction of one of those dependencies MUST also be migrated in this change.

### Key Entities

- **Handler**: A function in `internal/handler/` that implements the request-shaped logic of one CLI command. After this change, it consumes pre-built dependencies and contains only command-specific logic (input parsing, planning, output formatting, error mapping).
- **Composition Root**: The single location responsible for assembling the runtime dependency graph (routing backend, secrets vault, deploy engine, observers, log writers) from configuration. Its placement in the package layout is an implementation choice for the planning phase; this spec only requires that it exists, is unique, and is not inside `internal/handler/`.
- **Shared Dependency**: A constructed object that more than one handler legitimately uses (today: secrets vault plugin, routing backend plugin, local deploy engine, terminal observer, file logger). These are the values that the composition root produces and handlers consume.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Zero direct calls to `infisicalplugin.New`, `traefik.New`, `local.NewLocalEngine`, `ui.NewTerminalObserver`, or `ui.NewFileLogger` remain inside any file under `internal/handler/`.
- **SC-002**: For every shared dependency listed in FR-001, the project contains exactly one source-code location that constructs it for normal CLI execution (test doubles excluded).
- **SC-003**: Adding a new command that reuses the existing dependency graph requires zero new lines of plugin / engine / observer construction code inside `internal/handler/`. (Verifiable by attempting a small spike: a new no-op command that needs the same deps should compile after referencing only the composition entry point.)
- **SC-004**: All existing `shrine` CLI commands produce byte-identical stdout/stderr and identical exit codes for a representative manifest set before and after the change. (Verifiable by capturing output of each command pre-refactor and diffing against post-refactor output.)
- **SC-005**: All tests pass (`go test ./...`) and the previously-passing tests under `internal/handler/` continue to pass without modification of their assertions.
- **SC-006**: A signature change to any one constructor named in SC-001 requires editing exactly one file outside `cmd/` to make the project compile again. (Verifiable by performing a throwaway local rename and counting touched files.)

## Assumptions

- The CLI surface (`cmd/`) and the underlying package boundaries (`internal/engine`, `internal/plugins/...`, `internal/state`, `internal/config`, `internal/ui`) are stable for this change. This refactor only re-shapes who *constructs* what within `internal/`, not the package boundaries themselves.
- The architectural choice between Option A (leave it, document convention), Option B (new `internal/app` or `internal/wire` package), and Option C (push composition into `cmd/`) is a planning-phase decision and is intentionally **not** prescribed by this specification. The spec requires the *outcomes* (no construction inside `internal/handler/`, single source of construction); the plan picks the structure.
- Other handlers not yet inspected from source (`teardown.go`, `apps.go`, `deployments.go`, `resources.go`, `teams.go`, `status.go`) follow one of two patterns: they either already replicate construction (in which case they are in scope per FR-008) or they delegate to a helper / do not need shared deps (in which case they are out of scope for this refactor). Inventorying them is part of the planning phase.
- No new external runtime dependencies are required. The change is a structural reorganisation, not a new feature.
- Existing integration tests provide sufficient coverage to detect any CLI behaviour regression caused by the move; no new integration tests are required by this spec, though they may be added at the planner's discretion.
