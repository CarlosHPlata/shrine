# Tasks: Teardown Headers That Print and Teardown Event Names That Match the Rest of the Log

**Input**: Design documents from `/specs/028-fix-teardown-event-names/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/observer-events.md, quickstart.md

**Tests**: Included — Constitution Principle V mandates TDD (tests authored red-first, before implementation), and spec FR-010 / FR-011 explicitly require the coverage. Per team practice, unit tests run locally and never touch the filesystem (in-memory fakes only); integration tests are authored and compile-checked locally (`go vet -tags integration ./tests/integration/...`) and executed by the CI pipeline as the gate — never run locally.

**Organization**: Tasks are grouped by user story. Both stories are delivered inside one function, `engine.teardownKind` (`internal/engine/engine.go:241-265`): US1 (the headers print) introduces the `eventPrefix` local and uses it for the `started` event name; US2 (consistent names) uses the same local for the two failure names. US2 therefore builds on US1. The end-to-end assertions for both stories are authored first, in the Foundational phase, because Constitution V requires the integration tests to exist before any implementation code and they cannot be part of a local red/green loop.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 (P1 the operator sees which deployment is being torn down), US2 (P2 teardown entries in the log and in error lines use the same naming as everything else)

## Phase 1: Setup

**Purpose**: Confirm the baseline — this is a bug fix on existing code, no scaffolding needed.

- [X] T001 Record the baseline at repository root before any change: `go build ./...` succeeds; `go test ./internal/engine/ ./internal/handler/ ./internal/ui/` passes; `go vet ./...` and `go vet -tags integration ./tests/integration/...` are clean; and save the output of `gofmt -l .`. That list is **not empty on `main`** — it already contains `internal/engine/engine.go` and `internal/engine/engine_test.go` (plus ten unrelated files, among them `internal/ui/terminal_logger.go` and `tests/integration/registry_alias_test.go`). Do **not** run `gofmt -w` on any file in this feature: reformatting those two files would bury the fix in unrelated whitespace changes. The rule for every later task is "no new file appears in `gofmt -l .`, and added lines are gofmt-shaped"

---

## Phase 2: Foundational — end-to-end assertions first (Blocking Prerequisites)

**Purpose**: Author the integration assertions for both stories before any production code changes (Constitution V; plan Design Outline step 1; research D6). No new scenario is added — three teardown runs that already exist gain one assertion each, so CI time is unchanged.

**⚠️ CRITICAL**: These are red against the unfixed engine by construction. Compile-check only — do not run them locally.

- [X] T002 [P] Add the header assertions to `tests/integration/teardown_test.go` (FR-011; US1 acceptance scenarios 1–2). In `TestTeardown`, scenario `"should teardown a deployed team and remove its containers and network"`, replace `tc.Run("teardown", testTeam, "--state-dir", tc.StateDir).AssertSuccess()` with the chained form below. In `TestTeardownMultiTeam`, **first** scenario only (`"should only teardown the target team leaving the other running"`), do the same for the `teardownTeamA` run with `"Tearing down Resource: shared-cache (team: " + teardownTeamA + ")"`. Leave the second multi-team scenario and every Docker assertion untouched. Match the ASCII text only — not the `🗑️` prefix

  ```go
  tc.Run("teardown", testTeam, "--state-dir", tc.StateDir).
  	AssertSuccess().
  	AssertOutputContains("Tearing down Application: whoami (team: " + testTeam + ")")
  ```

- [X] T003 [P] Add the log-entry assertion to `TestFileLogger` in `tests/integration/file_logger_test.go` (FR-011; US2 acceptance scenario 1): next to the existing `teardownEntry` declaration add ``teardownStartedEntry := `[started] application.teardown name="whoami" team="` + testTeam + `"` `` and, after the existing `tc.AssertFileContains(logFile, teardownEntry)`, add `tc.AssertFileContains(logFile, teardownStartedEntry)`. Keep the `network.remove` assertion and its comment — it remains the append-across-runs proof
- [X] T004 Compile-check the integration package: `go vet -tags integration ./tests/integration/...` is clean and `gofmt -l tests/integration/` prints only the baseline entry `tests/integration/registry_alias_test.go` (neither edited file appears). Do not execute the tests — CI runs them (T021)

**Checkpoint**: The end-to-end gate exists and is red against the current engine — production changes may begin.

---

## Phase 3: User Story 1 - The operator sees which deployment is being torn down (Priority: P1) 🎯 MVP

**Goal**: Every deployment removed by `shrine teardown <team>` is introduced by exactly one `🗑️  Tearing down Application|Resource: <name> (team: <team>)` line, printed before that deployment's removal steps. Achieved by emitting the `started` event under the lowercase name the terminal observer already matches (`application.teardown` / `resource.teardown`).

**Independent Test**: `go test ./internal/engine/ -run TestEngine_ExecuteTeardown_AnnouncesEachDeploymentBeforeRemovingIt` and `go test ./internal/handler/ -run TestTeardown_RemovesPlannedDeploymentsThenNetwork` pass; `internal/ui`'s existing catalogue test already proves those names render the header. End to end (CI): `TestTeardown` and `TestTeardownMultiTeam` find the application and resource headers on stdout (T002).

### Tests for User Story 1 (write FIRST — must FAIL before T008 lands) ⚠️

- [X] T005 [P] [US1] In `internal/engine/engine_test.go` add `"slices"` to the imports, add the `timelineObserver` type after `recordingObserver`, and add `TestEngine_ExecuteTeardown_AnnouncesEachDeploymentBeforeRemovingIt` after `TestEngine_ExecuteTeardown_StepLoopFailureSkipsFinalize` (research D4; data-model invariants 2–4). `Routing` is deliberately nil so no `routing.finalize` event enters the timeline. Do not touch any existing test or fake

  ```go
  // timelineObserver records event names into the slice the fake backends
  // write to, so a test can assert how events and backend calls interleave.
  type timelineObserver struct {
  	calls *[]string
  }

  func (o *timelineObserver) OnEvent(e Event) { *o.calls = append(*o.calls, "Event:"+e.Name) }
  ```

  ```go
  func TestEngine_ExecuteTeardown_AnnouncesEachDeploymentBeforeRemovingIt(t *testing.T) {
  	cases := []struct {
  		name  string
  		steps []planner.PlannedStep
  		want  []string
  	}{
  		{
  			name: "application then resource",
  			steps: []planner.PlannedStep{
  				{Kind: manifest.ApplicationKind, Name: "web"},
  				{Kind: manifest.ResourceKind, Name: "db"},
  			},
  			want: []string{
  				"Event:application.teardown",
  				"RemoveContainer:team-x/web",
  				"Event:resource.teardown",
  				"RemoveContainer:team-x/db",
  			},
  		},
  		{name: "team with no deployments", steps: nil, want: nil},
  	}

  	for _, tc := range cases {
  		t.Run(tc.name, func(t *testing.T) {
  			var calls []string
  			e := &Engine{
  				Container: &fakeContainerBackend{calls: &calls},
  				Observer:  &timelineObserver{calls: &calls},
  			}

  			if err := e.ExecuteTeardown("team-x", tc.steps); err != nil {
  				t.Fatalf("unexpected error: %v", err)
  			}

  			if !slices.Equal(calls, tc.want) {
  				t.Errorf("timeline = %v, want %v", calls, tc.want)
  			}
  		})
  	}
  }
  ```

- [X] T006 [P] [US1] In `internal/handler/teardown_test.go`, `TestTeardown_RemovesPlannedDeploymentsThenNetwork` (lines 97-103): delete the comment line `// Teardown events are named after the manifest kind as it was recorded.`, and change the two expectations from `manifest.ApplicationKind+".teardown"` to the literal `"application.teardown"` and from `manifest.ResourceKind+".teardown"` to the literal `"resource.teardown"` (research D5). Leave `newTeardownStandIns` as is — its deployments keep `manifest.ResourceKind` / `manifest.ApplicationKind` as the recorded kind, which makes this the unit proof of FR-008 (existing state, no migration). Do not touch `TestTeardown_StopsAtFirstRemovalFailure` yet (that is T011)
- [X] T007 [US1] Verify red: `go test ./internal/engine/ -run TestEngine_ExecuteTeardown_AnnouncesEachDeploymentBeforeRemovingIt` fails in the `application then resource` case with a timeline containing `Event:Application.teardown` and `Event:Resource.teardown` (the `team with no deployments` case passes); `go test ./internal/handler/ -run TestTeardown_RemovesPlannedDeploymentsThenNetwork` fails with `observer did not see the application teardown start`. These failures are the SC-006 evidence that a capitalised name turns the unit tests red

### Implementation for User Story 1

- [X] T008 [US1] In `internal/engine/engine.go`, `teardownKind` (line 241): add `eventPrefix := strings.ToLower(kind)` as the first statement and change the `started` event's `Name` from `kind + ".teardown"` to `eventPrefix + ".teardown"` (research D1). `strings` is already imported. Change nothing else in this task: the signature, the `step.Kind == manifest.ApplicationKind` gate, the two `fmt.Errorf` calls, and the two `emitErr` names stay as they are (the failure names are US2, T013). Add no comment

  ```go
  func (engine *Engine) teardownKind(kind string, team string, step planner.PlannedStep) error {
  	eventPrefix := strings.ToLower(kind)
  	engine.Observer.OnEvent(Event{
  		Name:   eventPrefix + ".teardown",
  ```

- [X] T009 [US1] Verify green: `go build ./...`; `go test ./internal/engine/ ./internal/handler/ ./internal/ui/` passes — T005 and T006 are green, `TestTeardown_StopsAtFirstRemovalFailure` still passes unchanged (it still expects, and the engine still emits, the capitalised `.remove` name), and no other test changes status; `go vet -tags integration ./tests/integration/...` clean

**Checkpoint**: The "Tearing down" headers print in a real teardown and the log records `application.teardown` / `resource.teardown` — MVP complete for the visible defect in issue #46 (CI executes T002/T003 as the gate).

---

## Phase 4: User Story 2 - Teardown entries in the log and in error lines use the same naming as everything else (Priority: P2)

**Goal**: Teardown failures are reported as `application.remove`, `resource.remove`, and `application.routing_remove` on the terminal (`❌ Error [...]`) and in the log; after this phase no event emitted during a teardown has an uppercase letter in its name. The error text after the name (`Application "web": …`) is unchanged.

**Independent Test**: `go test ./internal/engine/ -run TestEngine_ExecuteTeardown_NamesFailureEventsInLowercase` and `go test ./internal/handler/ -run TestTeardown_StopsAtFirstRemovalFailure` pass. End to end (CI): `TestFileLogger` finds `[started] application.teardown name="whoami" team="<team>"` in the log (T003). The failure names have no integration scenario — a removal failure cannot be forced deterministically against a real daemon (research D7).

### Tests for User Story 2 (write FIRST — must FAIL before T013 lands) ⚠️

- [X] T010 [P] [US2] In `internal/engine/engine_test.go`: add `"strings"` to the imports; give `fakeRoutingBackend` a `removeRouteErr error` field, return it from `RemoveRoute` (in place of `return nil`), and update the type's doc comment to `finalizeErr and removeRouteErr are returned from Finalize and RemoveRoute when non-nil.` (the zero value keeps every existing test unchanged); then add `TestEngine_ExecuteTeardown_NamesFailureEventsInLowercase` after the test added in T005 (research D2, D4; contracts/observer-events.md). The `wantErr` strings pin that the error prose keeps the recorded, capitalised kind

  ```go
  func TestEngine_ExecuteTeardown_NamesFailureEventsInLowercase(t *testing.T) {
  	cases := []struct {
  		name      string
  		step      planner.PlannedStep
  		container ContainerBackend
  		routing   RoutingBackend
  		wantEvent string
  		wantErr   string
  	}{
  		{
  			name:      "application container removal",
  			step:      planner.PlannedStep{Kind: manifest.ApplicationKind, Name: "web"},
  			container: &teardownFailingContainerBackend{},
  			wantEvent: "application.remove",
  			wantErr:   `Application "web": remove container failed`,
  		},
  		{
  			name:      "resource container removal",
  			step:      planner.PlannedStep{Kind: manifest.ResourceKind, Name: "db"},
  			container: &teardownFailingContainerBackend{},
  			wantEvent: "resource.remove",
  			wantErr:   `Resource "db": remove container failed`,
  		},
  		{
  			name:      "application route removal",
  			step:      planner.PlannedStep{Kind: manifest.ApplicationKind, Name: "web"},
  			container: &fakeContainerBackend{calls: new([]string)},
  			routing:   &fakeRoutingBackend{calls: new([]string), removeRouteErr: errors.New("remove route failed")},
  			wantEvent: "application.routing_remove",
  			wantErr:   `Application "web" routing: remove route failed`,
  		},
  	}

  	for _, tc := range cases {
  		t.Run(tc.name, func(t *testing.T) {
  			obs := &recordingObserver{}
  			e := &Engine{Container: tc.container, Routing: tc.routing, Observer: obs}

  			err := e.ExecuteTeardown("team-x", []planner.PlannedStep{tc.step})

  			if err == nil || err.Error() != tc.wantErr {
  				t.Fatalf("error = %v, want %q", err, tc.wantErr)
  			}
  			for _, ev := range obs.events {
  				if ev.Name != strings.ToLower(ev.Name) {
  					t.Errorf("event name %q is not lowercase", ev.Name)
  				}
  			}
  			failure := obs.events[len(obs.events)-1]
  			if failure.Name != tc.wantEvent || failure.Status != StatusError {
  				t.Errorf("last event = %s [%s], want %s [%s]", failure.Name, failure.Status, tc.wantEvent, StatusError)
  			}
  			if failure.Fields["team"] != "team-x" || failure.Fields["name"] != tc.step.Name || failure.Fields["error"] != tc.wantErr {
  				t.Errorf("failure event fields = %v", failure.Fields)
  			}
  		})
  	}
  }
  ```

- [X] T011 [P] [US2] In `internal/handler/teardown_test.go`, `TestTeardown_StopsAtFirstRemovalFailure` (line 136): change the expectation from `manifest.ApplicationKind+".remove"` to the literal `"application.remove"`. The `manifest` import stays — `newTeardownStandIns` still uses it
- [X] T012 [US2] Verify red: `go test ./internal/engine/ -run TestEngine_ExecuteTeardown_NamesFailureEventsInLowercase` fails in all three rows with `event name "Application.remove" is not lowercase` / `"Resource.remove"` / `"Application.routing_remove"`; `go test ./internal/handler/ -run TestTeardown_StopsAtFirstRemovalFailure` fails with `observer did not see the removal error`. The returned-error assertions (`wantErr`) already pass — the prose is not changing

### Implementation for User Story 2

- [X] T013 [US2] In `internal/engine/engine.go`, `teardownKind`: change the two failure names to use the local introduced in T008 — `engine.emitErr(kind+".remove", …)` → `engine.emitErr(eventPrefix+".remove", …)` and `engine.emitErr(kind+".routing_remove", …)` → `engine.emitErr(eventPrefix+".routing_remove", …)`. Leave both `fmt.Errorf("%s %q…", kind, …)` calls and the `step.Kind == manifest.ApplicationKind` gate on the recorded value (research D2; FR-007, FR-008). Afterwards `grep -nE 'kind *\+ *"' internal/engine/engine.go` prints nothing — no event name is built from the recorded kind any more
- [X] T014 [US2] Verify green: `go build ./...`; `go test ./internal/engine/ ./internal/handler/ ./internal/ui/` passes with T010 and T011 green and no other test changing status; `go vet -tags integration ./tests/integration/...` clean; `git diff --stat internal/ui internal/planner internal/state cmd` is empty (plan: those packages are untouched)

**Checkpoint**: Both stories complete — every teardown event name is lowercase, on the terminal and in the log (CI executes T002/T003 as the gate).

---

## Phase 5: Polish & Cross-Cutting Concerns

- [X] T015 [P] State the casing rule in `specs/features/logging-observer.md`, section `## Event Name Conventions` (line 61): after the paragraph that ends ``… `started` → `finished` | `error`.`` (line 63) insert a new paragraph — ``Event names are always lowercase. A name derived from a manifest kind uses the kind in lowercase (`application.teardown`, `resource.remove`), never the recorded form (`Application`): observers match names exactly, so a capitalised name is silently dropped by the terminal and stands out in the log.`` (research D8)
- [X] T016 [P] Update `specs/features/integration-tests.md` (FR-012). (a) In `### File logger — spec 027`, under ``Test cases (`TestFileLogger`):`` (line 147) add a third bullet: ``- after the same `teardown <team>` — the log also holds `[started] application.teardown name="whoami" team="<team>"` (spec 028: teardown event names are lowercase, like every other entry)``. (b) Insert a new subsection between the end of the File logger section and `### Phase 4 — Docker-backed deploy tests ✅` (line 151): heading `### Teardown output — spec 028`; a line stating it requires Docker and adds no scenario — existing teardown runs gain one stdout assertion each; two bullets — ``- `TestTeardown` — `teardown <team>` stdout contains `Tearing down Application: whoami (team: <team>)` `` and ``- `TestTeardownMultiTeam` ("should only teardown the target team leaving the other running") — stdout contains `Tearing down Resource: shared-cache (team: <team-a>)` ``; and a closing note that the failure names (`application.remove`, `resource.remove`, `application.routing_remove`) are covered by unit tests in `internal/engine` only, because a removal failure cannot be forced deterministically against a real daemon
- [X] T017 [P] Update `specs/progress.md` (FR-012). (a) Insert a new entry directly above the 027 entry (line 89, the one starting `- [x] **Test: unit coverage for the composition root and event renderers**`), in the same style: ``- [x] **Fix: teardown headers print and teardown event names are lowercase** — see `specs/028-fix-teardown-event-names/` (issue #46). …`` summarising: `engine.teardownKind` built its event names from the kind as state records it (`Application` / `Resource`), so teardown emitted `Application.teardown` / `Application.remove` / `Application.routing_remove` — the terminal's `application.teardown` / `resource.teardown` arms never matched and `shrine.log` held the product's only capitalised event names; one named local (`eventPrefix := strings.ToLower(kind)`) now feeds the three names; the route-removal gate and the error prose keep the recorded kind, so state, ordering, and error text are unchanged and no migration is needed; `internal/ui` untouched; acceptance SC-001 (one header per deployment, before its removal lines), SC-003 (log records `application.teardown` / `resource.teardown`), SC-004 (failures named `application.remove` / `resource.remove` / `application.routing_remove`), SC-005 (same containers, routes, network removed; existing state tears down as is), SC-006 (capitalised names turn the engine and handler unit tests red); gate: `TestTeardown` + `TestTeardownMultiTeam` (headers on stdout) and `TestFileLogger` (`[started] application.teardown` in the log), CI executes. (b) In the 027 entry replace `pinned as observed, fix tracked separately` with ``pinned as observed at the time, fixed in `specs/028-fix-teardown-event-names/` (issue #46)``
- [X] T018 [P] Mark the defect resolved in the spec 027 records (FR-012) — annotate, do not rewrite the history. (a) `specs/027-app-ui-unit-coverage/tasks.md`, Implementation Note 1: after its last sentence (`…it is tracked as a separate task.`) append ``**Resolved by spec 028 (issue #46)**: the engine now lowercases the kind when naming teardown events, `internal/handler/teardown_test.go` asserts `application.teardown` / `resource.teardown` / `application.remove`, and the header prints in a real teardown.`` (b) `specs/027-app-ui-unit-coverage/data-model.md`, the `> **Implementation correction**` blockquote in §7 (line 210): append ``Resolved by spec 028 (issue #46): the engine now emits the lowercase names shown in the table above and the handler test asserts them.`` (c) `specs/027-app-ui-unit-coverage/contracts/coverage-matrix.md`: in row B-1 replace ``observer saw `Application.teardown` and `Resource.teardown` started events (named after the recorded manifest kind — see tasks.md Implementation Notes)`` with ``observer saw `application.teardown` and `resource.teardown` started events (lowercase since spec 028 — see tasks.md Implementation Notes)``; in row B-3 replace ``an `Application.remove` `error` event observed`` with ``an `application.remove` `error` event observed``
- [X] T019 Run the full local gate at repository root: `go build ./...`; `go vet ./...` clean; `go test ./...` zero failures; `go vet -tags integration ./tests/integration/...` clean; `gofmt -l .` prints exactly the list recorded in T001 (no new file — in particular not `internal/handler/teardown_test.go`, `tests/integration/teardown_test.go`, or `tests/integration/file_logger_test.go`); `git diff --stat` shows only `internal/engine/engine.go`, `internal/engine/engine_test.go`, `internal/handler/teardown_test.go`, `tests/integration/teardown_test.go`, `tests/integration/file_logger_test.go`, and the spec/docs files from T015–T018 (SC-005)
- [X] T020 [P] Run `graphify update .` to refresh the knowledge graph after the code changes (project rule, AST-only)
- [ ] T021 Once the branch is pushed and the PR is open — titled `fix: …`, closing issue #46, and carrying the one-line Constitution Check required for changes under `internal/engine/` — confirm the CI integration job is green: it executes `TestTeardown`, `TestTeardownMultiTeam` (T002) and `TestFileLogger` (T003), which together form the Constitution V gate and the first live confirmation of the fix. Optionally walk `specs/028-fix-teardown-event-names/quickstart.md` against a built binary on a Docker host

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: after T001 — touches only `tests/integration/teardown_test.go` and `tests/integration/file_logger_test.go`; must be complete before any production change (T008)
- **US1 (Phase 3)**: after Phase 2 — touches `internal/engine/engine.go`, `internal/engine/engine_test.go`, `internal/handler/teardown_test.go`
- **US2 (Phase 4)**: after US1 (T009) — T013 uses the `eventPrefix` local T008 introduces, and T010/T011 edit the same two test files as T005/T006
- **Polish (Phase 5)**: after both story phases

### Story Completion Order

```text
T001 ─▶ Foundational (T002…T004) ─▶ US1 (T005…T009) ─▶ US2 (T010…T014) ─▶ Polish (T015…T021)
```

### Within Each User Story

- Tests are written and verified red (T007, T012) before the production change (T008, T013)
- Each story ends with its own green verification (T009, T014)

### Parallel Opportunities

- **T002, T003 [P]** — two different integration files
- **T005, T006 [P]** — `internal/engine/engine_test.go` vs `internal/handler/teardown_test.go`; they can also be written alongside T002/T003 (four disjoint files), as long as T004 passes before T008
- **T010, T011 [P]** — the same two unit-test files, again disjoint from each other
- **T015, T016, T017, T018, T020 [P]** — disjoint files
- T008 and T013 are sequential: same function in `internal/engine/engine.go`

## Parallel Example: User Story 1

```bash
# After T004, two authors, disjoint files:
Task: "T005: timelineObserver + TestEngine_ExecuteTeardown_AnnouncesEachDeploymentBeforeRemovingIt in internal/engine/engine_test.go"
Task: "T006: lowercase started-event literals in internal/handler/teardown_test.go"
```

## Parallel Example: User Story 2

```bash
# After T009:
Task: "T010: removeRouteErr + TestEngine_ExecuteTeardown_NamesFailureEventsInLowercase in internal/engine/engine_test.go"
Task: "T011: lowercase application.remove literal in internal/handler/teardown_test.go"
```

## Requirement Coverage

| Requirement | Tasks |
|-------------|-------|
| FR-001, FR-002 (one header per deployment, in order, none for an empty team) | T005, T008; end to end T002 |
| FR-003 (`application.teardown` / `resource.teardown` everywhere, incl. the log) | T005, T006, T008; end to end T003 |
| FR-004 (lowercase failure names) | T010, T011, T013 |
| FR-005, FR-006 (no uppercase; one name per event) | T005 (exact timeline), T010 (lowercase sweep), T013 |
| FR-007 (behaviour and error text unchanged) | T010 (`wantErr`), T009, T014, existing engine teardown tests |
| FR-008 (existing state, no migration) | T006 (stand-ins keep the capitalised recorded kind) |
| FR-009 (deploy/apply/status/dry-run unchanged) | T014 (`internal/ui`, `cmd` untouched), T019 |
| FR-010 (unit coverage; 027 handler test flipped) | T005, T006, T010, T011 |
| FR-011 (end-to-end assertion) | T002, T003, T021 |
| FR-012 (documentation records the fix) | T015–T018 |

## Implementation Strategy

### MVP First (US1 only)

1. T001, then the end-to-end assertions (T002–T004), then US1 red tests (T005–T007), then T008–T009.
2. **STOP and VALIDATE**: the timeline test and the handler happy-path test are green; the integration files compile.
3. This alone closes the visible defect in issue #46 — the "Tearing down" headers print and the log records the lowercase started events.

### Incremental Delivery

1. US1 → headers print; started events lowercase.
2. US2 → failure names lowercase; no capitalised event name left in a teardown.
3. Polish → convention documented, integration-tests spec and progress entry updated, spec 027 notes marked resolved, graph refreshed, CI gate; ship as one PR closing issue #46.

### Format validation

All 21 tasks use the checklist format (`- [ ] Txxx [P?] [Story?] description + exact file path`); story labels appear only in Phases 3–4.

---

## Implementation Notes

Recorded during `/speckit-implement` (2026-10-04). T001–T020 are complete; T021 is open until the branch is pushed and CI has run.

1. **The gofmt baseline is 12 files, not 10.** The count written into T001 and T004 at task-generation time came from a truncated listing. The real `gofmt -l .` baseline on `main` also includes `internal/ui/terminal_logger.go` and `tests/integration/registry_alias_test.go`; T001, T004, and T019 were corrected in place. The rule itself was applied as written: no file was reformatted, the `gofmt -l .` list after the change is identical to the baseline, and `gofmt -d` shows no hunk touching an added line.
2. **Red-first evidence (SC-006).** Before T008 the timeline test reported `Event:Application.teardown` / `Event:Resource.teardown` and the handler test reported `observer did not see the application teardown start`. Before T013 the failure table reported `Application.remove`, `Resource.remove`, and `Application.routing_remove` as not lowercase, while every `wantErr` (error prose) assertion already passed.
3. **Cross-package confirmation without Docker.** A throwaway test wiring the real `engine.Engine` to the real `TerminalObserver` and file logger (deleted afterwards, not part of the change) printed `🗑️  Tearing down Application: whoami (team: shrine-deploy-test)`, `🗑️  Tearing down Resource: db (team: shrine-deploy-test)`, and on a forced removal failure `❌ Error [application.remove]: Application "whoami": daemon unreachable`, with matching lowercase log entries. The live confirmation through the real binary and Docker remains T021 (CI).
4. **Production diff**: `internal/engine/engine.go` only — one line added, three modified, all inside `teardownKind`. `internal/ui`, `internal/planner`, `internal/state`, and `cmd` are untouched.
