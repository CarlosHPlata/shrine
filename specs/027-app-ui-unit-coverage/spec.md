# Feature Specification: Unit Coverage for the Composition Root and Event Renderers

**Feature Branch**: `027-app-ui-unit-coverage`
**Created**: 2026-08-28
**Status**: Draft
**Input**: GitHub issue [#40](https://github.com/CarlosHPlata/shrine/issues/40) — "test: unit coverage for internal/app, ui/file_logger, and terminal_logger event branches"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A maintainer changing the composition root is told when a command's wiring or its failure messages regress (Priority: P1)

Spec 017 moved every piece of dependency construction for `deploy`, `apply`, and `teardown` into a single composition root and promised three things: one construction site (017 FR-002), each command receives only the subset of collaborators it needs (017 FR-003), and a construction failure keeps its full detail — the slot that failed and the underlying cause (017 FR-004). None of those promises is pinned by a test today. A maintainer could drop the `vault:` prefix from an error, accidentally make `teardown` construct the secrets vault (so a broken vault configuration would start failing teardowns that never needed secrets), or forget to close the log writer when a later collaborator fails — and nothing would notice short of a live-Docker integration run or an operator reading a worse error message. After this change, the unit suite fails within seconds on any of those regressions.

**Why this priority**: This is the largest gap the audit found and the stated payoff of the 017 refactor that was never realised. The behaviours it guards are user-visible: the exact error an operator reads when a plugin is misconfigured, and which commands can be broken by which configuration.

**Independent Test**: With no Docker daemon reachable, no network, no credentials, and no home directory, run the unit suite for the composition root and confirm it assembles each command's dependency set, exercises every failure point, and asserts on the exact error prefix and the preserved cause.

**Acceptance Scenarios**:

1. **Given** a valid configuration with no gateway plugin and no secrets vault configured, **When** the deploy dependency set is assembled, **Then** assembly succeeds, every collaborator the deploy command relies on (output writers, configuration, state, paths, resolved manifest directory, observer, container backend, engine) is present, the routing and secrets-vault slots reflect "not configured", and a cleanup function is returned.
2. **Given** the same configuration, **When** the apply dependency set is assembled, **Then** assembly succeeds, every collaborator the apply command relies on (output writers, configuration, state, paths, observer, engine) is present, the secrets-vault slot reflects "not configured", and a cleanup function is returned.
3. **Given** the same configuration, **When** the teardown dependency set is assembled, **Then** assembly succeeds, every collaborator the teardown command relies on (output writer, configuration, state, paths, observer, engine) is present, the routing slot reflects "not configured", and a cleanup function is returned.
4. **Given** a configuration whose gateway plugin settings are invalid, **When** the apply dependency set is assembled, **Then** assembly succeeds — apply never constructs the gateway plugin, the routing backend, or the standalone container backend, so their configuration cannot fail it.
5. **Given** a configuration whose secrets vault would fail to construct, **When** the teardown dependency set is assembled, **Then** assembly succeeds — teardown never constructs the secrets vault.
6. **Given** a configuration whose secrets vault would fail to construct, **When** the deploy or apply dependency set is assembled, **Then** assembly fails with an error that begins with `vault:` and whose underlying cause remains identifiable by the caller, no dependency set and no cleanup are returned, and the log writer opened earlier in the sequence has been closed.
7. **Given** any other collaborator fails to construct — registry validation, the observer pair, the standalone container backend, the gateway plugin, the routing backend, or the engine — **When** the affected dependency set is assembled, **Then** the error begins with that slot's name (`validating registries:`, `observer:`, `container backend:`, `traefik:`, `routing:`, `engine:`), the underlying cause remains identifiable, nothing is returned, and any log writer opened before the failure has been closed.
8. **Given** a successfully assembled dependency set, **When** the returned cleanup runs, **Then** the log writer is closed and any error from closing it is reported to the caller.

---

### User Story 2 - Every line the terminal shows during a deploy or teardown is pinned (Priority: P2)

The terminal observer turns engine events into the lines an operator reads while a command runs: lifecycle headers ("Deploying Application: web"), progress indicators for slow steps (network creation, image pulls, container removal), gateway warnings that carry a remediation hint, the "orphan route file left on disk; remove with: rm …" instruction, and the generic error line. Thirty-three distinct event kinds have a dedicated rendering; exactly one of them is covered by a test. A wrong field key, a swapped argument, or a dropped branch renders an empty or misleading value with no failing test. After this change, every rendered kind has a test asserting its exact line, the progress-indicator lifecycle is pinned, and event kinds that intentionally render nothing are pinned as silent.

**Why this priority**: This output is the product's primary user interface during its most important commands, and several lines (the gateway warnings, the orphan-route instruction) exist solely to tell the operator how to fix a problem. A regression here is immediately user-visible but currently undetectable by the suite.

**Independent Test**: Feed each event kind, at each status it renders, through the terminal observer with populated fields into an in-memory writer and compare the output against the expected line; feed the step-style kinds through their start → finish and start → error sequences and confirm the indicator starts, stops, and the completion line appears exactly once.

**Acceptance Scenarios**:

1. **Given** each event kind that has a dedicated rendering — application/resource deploy and teardown headers, network ensure, container create/start/recreate/fresh/created, routing configure, every gateway config/dashboard/route/alias line, DNS register, host-port published — **When** an event of that kind with populated fields is observed at the status it renders, **Then** the output is exactly the documented line for that kind with every field value in its expected position.
2. **Given** a `routing.configure` event, **When** it carries aliases, **Then** an additional aliases sub-line follows the routing line; **When** it carries no aliases, **Then** no sub-line is written.
3. **Given** an event of any kind with status `error`, **When** it is observed, **Then** a line of the form `❌ Error [<kind>]: <error text>` is written, whether or not the kind has a dedicated rendering.
4. **Given** a step-style kind (network create, network remove, container remove, volume create, image pull), **When** its start event is observed, **Then** a progress indicator carrying the step's message begins; **When** the matching finished event is observed, **Then** the indicator stops, its line is cleared, and the completion line with the finished fields is written exactly once; **When** an error event is observed instead, **Then** the indicator stops and no completion line is written.
5. **Given** a finished event for a step-style kind with no prior start event, **When** it is observed, **Then** the completion line is written and the observer does not block.
6. **Given** a `volume.created` finished event following a `volume.create` start, **When** it is observed, **Then** the volume indicator stops and the "Volume … is created" line is written.
7. **Given** a `container.remove` event with status `info` and reason `not found`, **When** it is observed, **Then** the "not found, skipping removal" line is written and no indicator starts.
8. **Given** an event kind with no dedicated rendering (for example `routing.finalize`), **When** it is observed with status `started` or `info`, **Then** nothing is written; **When** observed with status `error`, **Then** only the generic error line is written.
9. **Given** an event kind observed at a status it does not render (for example an application deploy `finished` event), **When** it is observed, **Then** nothing is written.

---

### User Story 3 - A handler can be exercised with stand-in collaborators (Priority: P3)

Spec 017 (US3, FR-006) promised that after the composition-root refactor a handler could be unit-tested by handing it a dependency set built from stand-ins — no real plugins, no Docker, no credentials. That promise has never been exercised: no test constructs a dependency set with stand-ins, so nobody knows whether the isolation seam actually works or whether a handler quietly reaches around it. After this change, at least one handler has such a test, proving the seam and giving future handler tests a pattern to copy.

**Why this priority**: It verifies a design property rather than a user-visible behaviour, so it ranks below the two output-facing stories, but it is cheap, it closes an explicit open item from spec 017, and it is the foundation for every future handler test.

**Independent Test**: Build a teardown dependency set by hand from in-memory stand-ins — a deployment record listing, a container backend that records the operations it receives, an observer that records events — call the teardown handler, and assert the recorded operations and events. The test must need no Docker daemon, network, filesystem, or credentials, and must invoke no real plugin, engine, backend, or observer constructor.

**Acceptance Scenarios**:

1. **Given** a hand-built teardown dependency set whose deployment listing reports one application and one resource for a team, **When** the teardown handler runs for that team, **Then** the container backend receives a removal for exactly those two deployments, application before resource, followed by removal of the team's network, and the observer sees the corresponding events.
2. **Given** a hand-built teardown dependency set whose deployment listing fails, **When** the handler runs, **Then** the listing error is returned to the caller and the container backend receives no operation.
3. **Given** a hand-built teardown dependency set whose container backend fails a removal, **When** the handler runs, **Then** the failure is returned to the caller with its cause identifiable and no further operations are attempted.

---

### User Story 4 - The on-disk log's line format is pinned (Priority: P4)

The file logger is the only durable record of what a command did once the terminal scrolls away; operators and maintainers grep it when diagnosing a deploy. It records every event — including the ones the terminal renders selectively and the errors — as one line: a UTC timestamp, the status, the event name, and the event's fields in a stable, quoted, sorted form. That format has no test. After this change, the line format, the "every event, every status" behaviour, and whole-line writes under concurrent observation are pinned in memory, and the log file's location and append-across-runs behaviour are pinned in the integration suite, where filesystem behaviour belongs.

**Why this priority**: The log is a debugging aid rather than the primary interface, and a format regression is far less disruptive than a wrong terminal line or a lost error cause. It is included because the audit flagged it and the cost is small.

**Independent Test**: Observe a handful of events with an in-memory destination and compare each produced line against the expected format; observe events from several concurrent callers at once and confirm every line is whole. Separately, run two shrine commands against the same state directory in the integration suite and confirm the log file holds entries from both.

**Acceptance Scenarios**:

1. **Given** an event with several fields, **When** it is logged, **Then** the line is `<UTC RFC3339 timestamp> [<status>] <name>` followed by one ` key="value"` pair per field in ascending key order, with values quoted so that spaces and quotes are unambiguous.
2. **Given** an event with no fields, **When** it is logged, **Then** the line ends immediately after the event name with no trailing separator.
3. **Given** events of every status — started, finished, info, warning, error — **When** they are logged, **Then** each produces exactly one line; the file logger never filters.
4. **Given** events observed concurrently from several callers, **When** they are logged, **Then** every line is complete and uninterleaved.
5. **Given** a state directory with no log yet, **When** a shrine command runs, **Then** `logs/shrine.log` is created beneath the state directory; **When** a second command runs against the same state directory, **Then** the earlier entries are still present and the new ones follow them.

---

### Edge Cases

- A construction failure that occurs *after* the log writer has been opened (secrets vault, gateway plugin, routing backend, container backend, engine) must leave the log writer closed; a failure *before* it is opened (registry validation, manifest-directory resolution) must not attempt to close anything.
- The cleanup function is documented as safe to call more than once. A second call MUST NOT panic; the tests pin the observed result of a second call and, if it disagrees with the documented contract, the discrepancy is reported in the implementation notes rather than silently encoded as expected behaviour.
- An unresolvable manifest directory (spec 026) continues to fail deploy and teardown assembly with the field-naming error, unprefixed; the existing tests for that path remain in place and unchanged.
- An error event for a kind that also renders a progress line (`container.create`) renders only the error line, never the progress line — already pinned; that test stays.
- A `warning` status on a step-style kind renders nothing and leaves any running indicator untouched.
- A rendered kind observed with an empty field map does not panic; the missing values render as empty (existing behaviour, not to be improved here).
- Tests for the progress indicator must terminate promptly (bounded by one animation frame interval per indicator) and pass with data-race detection enabled; they must assert on the presence and count of lines rather than on the exact animation bytes.
- Starting a second indicator while one is still running is existing behaviour outside this feature's scope; tests must neither depend on it nor hang because of it.
- Log-line timestamps are not compared for exact value, only for format; tests do not depend on wall-clock time.

## Requirements *(mandatory)*

### Functional Requirements

**Hermetic unit tests**

- **FR-001**: Every unit test added by this feature MUST run without a Docker daemon, network access, credentials, or any filesystem read or write (no temporary directories, no fixture files, no log files); process environment variables are the only permitted environmental manipulation. The added tests MUST pass with data-race detection enabled and MUST NOT depend on wall-clock time beyond a bounded wait for a progress indicator to stop.

**Composition root**

- **FR-002**: For each of the deploy, apply, and teardown dependency sets, assembly from a valid configuration with no gateway plugin and no secrets vault MUST be pinned: it succeeds, every collaborator the command relies on is present, optional collaborators that are not configured are reported as absent, and a cleanup function is returned.
- **FR-003**: The subset shape of each dependency set MUST be pinned as a behaviour: teardown assembly never constructs the secrets vault; apply assembly never constructs the gateway plugin, the routing backend, or the standalone container backend — so that an invalid or failing configuration for those collaborators cannot fail the command that does not use them.
- **FR-004**: For every construction failure point — registry validation, observer pair, standalone container backend, gateway plugin, secrets vault, routing backend, engine — the failure MUST be pinned to (a) produce an error that begins with that slot's name, (b) preserve the underlying cause so the caller can still identify it, (c) return neither a dependency set nor a cleanup function, and (d) leave any log writer opened before the failure closed.
- **FR-005**: The cleanup function returned by a successful assembly MUST be pinned to close the log writer and to report any closer error to the caller; absent closers are skipped.

**Handler isolation seam**

- **FR-006**: At least one handler MUST have a unit test that builds the handler's dependency set by hand from in-memory stand-ins — invoking no real plugin, engine, backend, or observer constructor — and asserts the handler's own logic: the planned operations reach the backend in order, events reach the observer, and a collaborator's failure is returned to the caller with its cause identifiable. This closes spec 017 US3 / FR-006.

**Terminal rendering**

- **FR-007**: Every event kind with a dedicated terminal rendering MUST have at least one test asserting the exact rendered line for populated fields, at the status it renders. This covers the lifecycle headers, network ensure, container create/start/recreate/fresh/created, routing configure (with and without aliases), all gateway config, dashboard, route, and alias lines, DNS register, host-port published, and the step-style kinds (network create/remove, container remove, volume create/created, image pull).
- **FR-008**: The generic error line MUST be pinned: an event of any kind with status `error` renders `❌ Error [<kind>]: <error text>`, whether or not the kind has a dedicated rendering.
- **FR-009**: The progress-indicator lifecycle MUST be pinned for the step-style kinds: the start status begins an indicator carrying the step's message; the finished status stops it, clears its line, and writes the completion line exactly once; the error status stops it and writes no completion line; a finished event with no prior start writes only the completion line; `container.remove` with status `info` and reason `not found` writes the skip line and starts no indicator; `volume.created` completes the volume indicator.
- **FR-010**: Silence MUST be pinned: an event kind with no dedicated rendering (for example `routing.finalize`) writes nothing at `started` or `info` and only the generic error line at `error`; a rendered kind observed at a status it does not render writes nothing.

**File logger**

- **FR-011**: The log line format MUST be pinned: `<UTC RFC3339 timestamp> [<status>] <name>` followed by one ` key="value"` pair per field in ascending key order with quoted values; nothing follows the name when there are no fields; every event of every status produces exactly one line.
- **FR-012**: Whole-line writes under concurrent observation MUST be pinned: lines from concurrently observed events never interleave.
- **FR-013**: The log file's location (`logs/shrine.log` beneath the state directory, created on first use) and its accumulation across command runs (appended, never truncated) MUST be pinned by the integration suite, which is the designated home for filesystem behaviour.

**No behaviour change**

- **FR-014**: This feature MUST NOT change any user-visible behaviour: command output, error text, exit codes, and log format are identical before and after. Production changes are limited to minimal, behaviour-preserving seams that allow the behaviours above to be observed in memory; they MUST NOT weaken the single-construction-site guarantee of spec 017 FR-002. All pre-existing unit and integration tests pass unchanged.

### Key Entities

- **Dependency set**: The fully assembled group of collaborators one command receives from the composition root — one shape each for deploy, apply, and teardown. Each has required slots (always present) and optional slots (absent when the corresponding plugin is not configured).
- **Slot**: One named position in a dependency set (registry validation, observer, container backend, gateway plugin, secrets vault, routing, engine). Slot names are the prefixes an operator sees when construction fails.
- **Construction failure**: A collaborator that cannot be built from configuration. It is reported with its slot name and its underlying cause, and it unwinds anything already opened.
- **Event**: A named occurrence emitted by the engine or a backend during a command, carrying a status (started, finished, info, warning, error) and a map of string fields.
- **Rendered line**: The exact terminal text produced for one event kind at one status; the unit of coverage for the terminal observer.
- **Step-style event**: An event kind whose start status opens a progress indicator and whose finished or error status closes it.
- **Log line**: The single-line, timestamped, sorted-field record the file logger writes for every event.
- **Stand-in**: An in-memory replacement for a collaborator (deployment listing, container backend, observer) that records what it receives so a handler test can assert on it.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of the composition root's assembly entry points (deploy, apply, teardown) have a passing happy-path test; the two subset-shape behaviours (teardown without vault, apply without gateway/routing/container backend) each have a test; and every slot prefix (`validating registries:`, `observer:`, `container backend:`, `traefik:`, `vault:`, `routing:`, `engine:`) appears in at least one failure assertion that also checks the cause is preserved and the log writer is closed.
- **SC-002**: 100% of event kinds with a dedicated terminal rendering — 33 at the time of writing, up from 1 covered — have at least one exact-line test, and the generic error line, the progress-indicator lifecycle, and the silence cases each have dedicated tests.
- **SC-003**: The file logger has tests for a fielded event, a field-less event, every status, and concurrent observation, and the integration suite has a scenario asserting the log's location and accumulation across two command runs.
- **SC-004**: At least one handler unit test exercises a hand-built dependency set with stand-ins and runs with no Docker daemon, network, filesystem access, or credentials.
- **SC-005**: The complete unit suite runs successfully with no Docker daemon reachable, no network, no home directory, and no writable filesystem, with data-race detection enabled, and the tests added by this feature add less than five seconds of wall-clock time.
- **SC-006**: Zero behaviour change: every pre-existing test passes unmodified, the integration suite passes in CI, and the terminal output, error text, and log lines of each command are identical before and after.
- **SC-007**: A deliberately introduced regression in any pinned behaviour — dropping a slot prefix, giving teardown a vault, changing a rendered line's field, unsorting log fields, skipping the log-writer close on failure — is caught by at least one failing unit test. (Verifiable by a throwaway mutation pass during review.)

## Assumptions

- The issue text is partly stale: since it was filed, spec 026 added a test file for the composition root covering manifest-directory resolution failures only. The gaps the issue names — happy paths, subset shapes, slot-prefixed failures, a handler test with stand-ins — remain open and are what this feature covers. Likewise the terminal observer now has 33 dedicated renderings rather than the "~15" the issue estimated; the requirement is "every rendered kind", not a fixed number.
- `routing.finalize` is listed in the issue among the branches to cover, but it has no dedicated terminal rendering today. Adding one would be a behaviour change, so this feature pins its silence (FR-010) rather than inventing output for it.
- Unit tests in this repository are hermetic by rule: no temporary directories, no fixture files, no filesystem writes. Behaviours that can only be observed through a real filesystem — the log file's location and append-across-runs — are therefore covered in the integration suite (FR-013), authored and compile-checked locally and gated by CI.
- Assembling any dependency set today always opens the log file on disk and creates a container-runtime client, and the file logger writes straight to an operating-system file handle. To pin these behaviours in memory, minimal behaviour-preserving seams in production code (for example, allowing the log destination or the collaborator constructors to be supplied) are in scope; the planning phase chooses their shape. Such seams must not change user-visible behaviour or reintroduce construction outside the composition root.
- Teardown is the natural handler for the stand-in test (US3): it is the only handler that reads no manifests from disk. Deploy and single-file apply both load manifests from the filesystem, so a hermetic handler test for them is out of reach without larger changes and is out of scope.
- "A valid configuration with no gateway plugin and no secrets vault" is the minimal happy-path input: it avoids any network attempt during vault construction and any gateway validation, while still exercising every required slot.
- Creating the container-runtime client from the environment does not contact a daemon, so engine construction is hermetic as it stands; tests need not stub it unless the plan chooses to for clarity.
- Progress-indicator tests accept that animation frames land in the output between the start and finish lines; they assert on the presence and count of the meaningful lines, and each indicator adds at most one frame interval of wait.
- This feature is the lowest-priority item of its release ticket set and lands after the release; it verifies existing behaviour only.
