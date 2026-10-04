# Research: Teardown Headers That Print and Teardown Event Names That Match the Rest of the Log

**Feature**: `028-fix-teardown-event-names` | **Date**: 2026-10-03
**Purpose**: Resolve every design choice behind the plan so implementation has no open questions. The Technical Context in `plan.md` contains no `NEEDS CLARIFICATION` markers; the decisions below record the alternatives weighed for each judgment call.

## Current-state findings (what the code does today)

| Area | Location | Finding |
|------|----------|---------|
| Teardown emitter | `internal/engine/engine.go:241-265` | `teardownKind(kind, team, step)` builds three event names by concatenation: `kind + ".teardown"` (status `started`, fields `team`, `name`), `kind+".remove"` (error) and `kind+".routing_remove"` (error). `kind` is `step.Kind`, passed in from `ExecuteTeardown` (`engine.go:94`). |
| Where the kind comes from | `internal/planner/plan.go:110-131`, `internal/engine/local/dockercontainer/docker_container.go:302`, `internal/manifest/types.go:11-12` | `PlanTeardown` copies `state.Deployment.Kind` into each step; its `switch` keeps only `manifest.ApplicationKind` / `manifest.ResourceKind` and orders applications (name-sorted) before resources (name-sorted). The container backend records the kind as `op.Kind`, i.e. `"Application"` / `"Resource"` — capitalised. |
| Other uses of the kind in teardown | `engine.go:254,257,260` | `step.Kind == manifest.ApplicationKind` gates route removal; `fmt.Errorf("%s %q: %w", kind, …)` and `"%s %q routing: %w"` put the kind into the human-readable error text. Both need the recorded (capitalised) value. |
| Deploy emitter | `engine.go:131-135` and resource path | Uses fixed literals (`"application.deploy"`, `"resource.deploy"`, `"application.resolve"`, …). Not affected. |
| Kind-derived names elsewhere | grep `Kind *+ *"` / `kind *+ *"` over `internal/`, `cmd/` | The three names in `teardownKind` are the **only** event names built from data instead of a literal. |
| Terminal renderer | `internal/ui/terminal_logger.go:22-24,32-45` | Any event with status `error` prints `  ❌ Error [<name>]: <error>` using the name verbatim. `case "application.teardown"` / `case "resource.teardown"` print the `🗑️  Tearing down …` header at `started`. A name of `Application.teardown` matches no case, so nothing prints. |
| File logger | `internal/ui/file_logger.go:46-52,58-72` | Writes `<RFC3339> [<status>] <name>` followed by ` key="value"` pairs in sorted key order, using the name verbatim → today `[started] Application.teardown name="whoami" team="…"`. |
| Backend removal events | `internal/engine/local/dockercontainer/docker_container.go:278-296` | The Docker backend emits its own `container.remove` events (`started` / `finished` / `info reason="not found"` / `error`). On a failed removal the operator therefore sees the backend's `❌ Error [container.remove]` line and then the engine's `❌ Error [<kind>.remove]` line. That two-level reporting is pre-existing and unchanged; only the second line's name changes. |
| Existing engine tests | `internal/engine/engine_test.go` | Four `ExecuteTeardown` tests (routing-remove call order, finalize once, failure skips finalize, nil routing) — **none asserts an event name**. Doubles available: `fakeContainerBackend{calls}`, `fakeRoutingBackend{calls, finalizeErr}`, `recordingObserver{events}`, `teardownFailingContainerBackend`. |
| Test pinning the defect | `internal/handler/teardown_test.go:97-103,136` | Asserts `manifest.ApplicationKind+".teardown"`, `manifest.ResourceKind+".teardown"`, `manifest.ApplicationKind+".remove"`, with the comment "Teardown events are named after the manifest kind as it was recorded." Spec 027 pinned observed behaviour on purpose. |
| Renderer tests | `internal/ui/terminal_logger_test.go:131-136`, `terminal_logger_steps_test.go:229-231` | Already pin the lowercase names → header text. They pass today because they feed the renderer the names it expects; they stay untouched. |
| Integration coverage | `tests/integration/teardown_test.go`, `tests/integration/file_logger_test.go` | `TestTeardown` (fixture `deploy/basic`: Application `whoami`, team `shrine-deploy-test`) and `TestTeardownMultiTeam` (fixture `teardown/`: Resource `shared-cache` owned by `shrine-teardown-a`) assert Docker state only — **no assertion on teardown output**. `TestFileLogger` asserts `[finished] network.remove …` after teardown, chosen in spec 027 so it would survive this fix. |
| Harness | `tests/integration/testutils/assert_general.go`, `testsuite.go:76-80` | `tc.Run(...)` returns the `*TestCase`; `AssertOutputContains` checks stdout, `AssertFileContains` checks a file. Teardown's terminal observer writes to `cmd.OutOrStdout()` (`cmd/teardown.go`), so headers land on stdout. No new helper is needed. |
| Documented convention | `specs/features/logging-observer.md:61-65`, `specs/009-preserve-app-configs/contracts/log-events.md:109` | "Event Name Conventions" prescribes `<subsystem>.<operation>` with all-lowercase examples but never states the casing rule. Spec 009's event contract already shows `application.teardown`. |
| User docs | `docs/content/` | No page shows teardown output or lists event names. `docs/content/cli/teardown.md` is generated from Cobra strings, which do not change. |
| Records of the defect | `specs/027-app-ui-unit-coverage/tasks.md` (Implementation Note 1), `data-model.md` §7 ("Implementation correction"), `contracts/coverage-matrix.md` (B-1, B-3), `specs/progress.md` (027 entry) | All describe the capitalised names as observed behaviour with the fix "tracked separately". |

## Decision 1 — Lowercase the kind once, inside `teardownKind`, into a named local

**Decision**: Add `eventPrefix := strings.ToLower(kind)` as the first statement of `teardownKind` and build the three names from it: `eventPrefix + ".teardown"`, `eventPrefix+".remove"`, `eventPrefix+".routing_remove"`. `strings` is already imported in `engine.go`. The function signature is unchanged.

**Rationale**: The defect is confined to how three names are spelled, and all three are built in this one function — so the fix belongs here and nowhere else. A single named local says what the value is for (Constitution VII), replaces three repetitions of the conversion (DRY), and leaves `kind` available, untouched, for the two places that need the recorded value (Decision 2). Lowercasing rather than mapping means FR-005 ("no uppercase in any teardown event name") holds by construction for whatever kind reaches the function.

**Alternatives considered**:
- *Lowercase at the call site* (`engine.teardownKind(strings.ToLower(step.Kind), …)`) — one line, but it also lowercases the error prose built from `kind` (`application "web": …`), which the spec leaves unchanged, and it makes `kind` mean something different from `step.Kind` inside the same function.
- *A `switch` mapping each kind to a literal* (`"application"` / `"resource"`), mirroring deploy's literals — more lines, and it needs a default branch for a case `PlanTeardown` already filters out.
- *A private helper* (`teardownEventPrefix(kind)`) — a function for a single call site wrapping one stdlib call; the named local carries the same intent (Constitution IV).
- *Lowercase the kind in the planner or in state* — changes what is recorded and compared, breaks `step.Kind == manifest.ApplicationKind`, and would need a migration for existing state (violates FR-008).
- *Teach the terminal to match the capitalised names* — fixes the header but leaves the log and error names inconsistent; rejected in the spec's Assumptions.
- *Shared event-name constants used by emitter and renderer* — would make this class of mismatch a compile error, but every other event name in the product is a string literal today (33 rendered kinds alone, per spec 027); converting them is a refactor well beyond a bug fix (Constitution IV).

## Decision 2 — The recorded kind keeps driving behaviour and error prose

**Decision**: `step.Kind == manifest.ApplicationKind` and the two `fmt.Errorf("%s %q…", kind, …)` calls keep using the original value. Returned errors and the `error` field of failure events read exactly as today (`Application "whoami": …`).

**Rationale**: FR-007 / FR-008 — which deployments get route removal, and what state records, must not change. The spec's Assumptions put the error prose out of scope: the issue asks for the event *names* only. A unit assertion on the returned error text pins this, so a later "simplification" to Decision 1's first rejected alternative turns a test red.

**Alternatives considered**: Lowercasing the prose to match deploy's `application %q: %w` — a cosmetic change the issue did not ask for; it can be its own change if wanted.

## Decision 3 — One name per event; no alias for the capitalised spelling

**Decision**: Emit only the lowercase names. No second event under the old name, no compatibility flag.

**Rationale**: FR-006. The capitalised names never matched the documented convention (spec 009, `logging-observer.md`), nothing in the repository consumes them, and a dual emission would print each error line twice and double every teardown entry in the log.

**Alternatives considered**: Emitting both spellings for a transition period — no known consumer to transition, and it would itself be a visible regression (duplicate lines).

## Decision 4 — Engine unit tests assert the names, their order, and the unchanged prose

**Decision**: Add two tests to `internal/engine/engine_test.go`:

1. `TestEngine_ExecuteTeardown_AnnouncesEachDeploymentBeforeRemovingIt` — a `timelineObserver{calls *[]string}` appends `"Event:"+name` into the same `calls` slice the fake container backend already writes to. With `Routing: nil` and steps `[{Application, "web"}, {Resource, "db"}]` the timeline must equal exactly `Event:application.teardown`, `RemoveContainer:team-x/web`, `Event:resource.teardown`, `RemoveContainer:team-x/db`. A second case with no steps expects an empty timeline.
2. `TestEngine_ExecuteTeardown_NamesFailureEventsInLowercase` — table of three failures: application container removal → `application.remove`; resource container removal → `resource.remove`; application route removal → `application.routing_remove`. Each row asserts the last recorded event's name, `error` status and `team`/`name` fields, that every recorded name equals its lowercase form, and that the returned error text is unchanged (`Application "web": remove container failed`, …).

`fakeRoutingBackend` gains a `removeRouteErr error` field returned by `RemoveRoute`; its zero value keeps every existing test unchanged.

**Rationale**: FR-010 and SC-006 — the engine had no test on any teardown event name, which is how the defect shipped. An exact, interleaved timeline covers four requirements at once: the names (FR-003), header-before-removal ordering (FR-001/FR-002), one name per event with nothing extra (FR-006), and no header for an empty team. The failure table covers FR-004 and pins Decision 2. All in-memory, no filesystem.

**Alternatives considered**:
- *Assert only "the observer saw `application.teardown`"* — does not catch dual emission or a header emitted after the removal.
- *A cross-package test wiring the real `TerminalObserver` to the engine* — needs a container-backend stub in `internal/ui` and duplicates what the integration scenario proves through the real binary; both sides of the contract are already pinned to the same literal in their own packages.

## Decision 5 — Flip the handler test to lowercase literals

**Decision**: In `internal/handler/teardown_test.go` replace `manifest.ApplicationKind+".teardown"`, `manifest.ResourceKind+".teardown"` and `manifest.ApplicationKind+".remove"` with the literals `"application.teardown"`, `"resource.teardown"`, `"application.remove"`, and delete the comment stating that teardown events are named after the recorded kind. The stand-in deployments keep `manifest.ResourceKind` / `manifest.ApplicationKind` as their recorded kind.

**Rationale**: FR-010 names this test explicitly. Literals are the point: building the expectation from the same constant the engine used is what let the test agree with the bug. Keeping the capitalised kind in the stand-in store makes this the unit proof of FR-008 — state recorded the old way produces lowercase events with no migration.

**Alternatives considered**: Deleting the event assertions from the handler test now that the engine test owns them — they are spec 027's proof that the handler wires planner → engine → observer (coverage-matrix B-1/B-3), so they stay, corrected.

## Decision 6 — End-to-end proof by extending existing Docker scenarios

**Decision**: Add assertions to scenarios that already run a teardown, instead of adding scenarios:

- `TestTeardown` — the teardown run also asserts `AssertOutputContains("Tearing down Application: whoami (team: " + testTeam + ")")`.
- `TestTeardownMultiTeam`, first scenario — the teardown run also asserts `AssertOutputContains("Tearing down Resource: shared-cache (team: " + teardownTeamA + ")")`.
- `TestFileLogger` — after teardown, also `AssertFileContains(logFile, `[started] application.teardown name="whoami" team="<team>"`)`. The existing `network.remove` assertion stays as the append proof.

Assertions match the ASCII part of the header, not the `🗑️` prefix. The integration files are edited **before** the engine change and compile-checked with `go vet -tags integration ./tests/integration/...`; CI executes them.

**Rationale**: FR-011 and Constitution V — the fix must be proven through the real binary, and it doubles as the live confirmation the issue says is missing. Each Docker scenario costs a full apply + deploy + teardown round-trip, so piggy-backing on the three existing teardown runs adds zero CI time while covering the application header, the resource header, and the log entry. Matching the ASCII text avoids coupling the test to the emoji's variation-selector bytes.

**Alternatives considered**:
- *A new dedicated scenario* — another Docker round-trip for assertions that fit on runs that already happen.
- *An ordering helper* (`AssertOutputContainsInOrder`) to prove header-before-removal end to end — the ordering is established by the engine emitting the event before calling the backend and is pinned by Decision 4's timeline; a new harness helper for one use is not justified.
- *`AssertFileNotContains(log, "Application.teardown")`* — needs a new helper; single-name emission is pinned exactly at unit level.

## Decision 7 — Failure-path names are verified at unit level only

**Decision**: No integration scenario forces a container- or route-removal failure during teardown.

**Rationale**: There is no deterministic way to make Docker refuse a container removal from a black-box test (a container removed out-of-band is a soft success by Constitution VI, not a failure). The names on the failure path are produced by the same `eventPrefix` as the started event, and Decision 4's table asserts all three.

**Alternatives considered**: Fault injection through a test-only flag or environment variable — production surface added for a test.

## Decision 8 — Documentation and bookkeeping

**Decision**:
- `specs/features/logging-observer.md` — state the casing rule in "Event Name Conventions": event names are lowercase; a name derived from a manifest kind is lowercased.
- `specs/features/integration-tests.md` — add the three new assertions to the teardown and file-logger entries.
- `specs/progress.md` — add the fix entry (issue #46) and amend the 027 entry's "fix tracked separately" to point at spec 028.
- `specs/027-app-ui-unit-coverage/` — annotate Implementation Note 1 in `tasks.md` and the "Implementation correction" in `data-model.md` §7 as resolved by spec 028, and update coverage-matrix rows B-1 / B-3 to the lowercase names the test now asserts.
- No change under `docs/content/` and no Cobra string change, so `make docs-check` does not drift.
- The PR is titled `fix: …` and carries the one-line Constitution Check required for changes under `internal/engine/`.

**Rationale**: FR-012. The convention document is where the next contributor will look before naming an event; stating the rule there is the cheapest guard against a repeat. Spec 027's notes are annotated rather than rewritten so the record of what was observed at the time stays intact.

**Alternatives considered**: A troubleshooting page for the renamed log entries — nothing for an operator to fix; the change is recorded in the release notes via the `fix:` commit.

## Decision 9 — `internal/ui` is not touched

**Decision**: No production or test change in `internal/ui`.

**Rationale**: The renderer and the file logger are already correct for the lowercase names and their tests already pin that (33/33 rendered kinds, spec 027). The defect is entirely on the emitting side.

**Alternatives considered**: Adding a case-insensitive match in the terminal observer as defence in depth — it would hide the next emitter-side naming mistake from the terminal while leaving it in the log.
