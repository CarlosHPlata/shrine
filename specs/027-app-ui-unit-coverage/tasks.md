# Tasks: Unit Coverage for the Composition Root and Event Renderers

**Input**: Design documents from `/specs/027-app-ui-unit-coverage/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/test-seams.md, contracts/coverage-matrix.md, quickstart.md

**Tests**: Included — the feature *is* tests (GitHub issue #40), and Constitution Principle V mandates red-first authoring. Per team practice, unit tests never touch the filesystem (no `t.TempDir`, no `os.MkdirAll`, no file writes; `t.Setenv` only) and run locally; the one integration scenario is authored and compile-checked locally (`go vet -tags integration ./tests/integration/...`) and executed by the CI pipeline as the gate — never run locally.

**Organization**: Tasks are grouped by user story. Each story owns its own files, so the four stories are independent: US1 lives in `internal/app/`, US2 in the terminal-observer test files of `internal/ui/`, US3 in `internal/handler/`, US4 in the file-logger files of `internal/ui/` plus `tests/integration/`. Only two stories touch production code, each through one behaviour-preserving seam (contracts/test-seams.md): US1 → `internal/app/components.go`, US4 → `internal/ui/file_logger.go`. US1 does **not** need US4's seam — it fakes the file logger behind its own `newFileLogger` constructor variable. There is no shared scaffolding, so the Foundational phase is empty.

**Red-first note**: US1 and US4 go red by failing to compile (the test references a seam that does not exist yet), then green when the seam lands. US2 and US3 pin behaviour that already exists with no production change, so their tests pass on first run; they are proven to bite by the mutation pass in the Polish phase (T030).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 (P1 composition-root wiring and failure messages), US2 (P2 every terminal line pinned), US3 (P3 a handler driven by stand-ins), US4 (P4 on-disk log line format)

## Phase 1: Setup

**Purpose**: Confirm a green baseline — this feature adds tests and two seams to existing code; no scaffolding is needed.

- [X] T001 Verify green baseline at repository root before any change: `go build ./...`, `go test -count=1 ./internal/app/ ./internal/ui/ ./internal/handler/`, `go vet -tags integration ./tests/integration/...`, and `gofmt -l .` (prints nothing) all pass; note the wall-clock of the three unit packages as the reference for SC-005

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Not applicable — nothing is shared across stories. Each seam is introduced inside the story whose first red test needs it (research D1, D2).

---

## Phase 3: User Story 1 - A maintainer changing the composition root is told when a command's wiring or its failure messages regress (Priority: P1) 🎯 MVP

**Goal**: `BuildApplyBundle`, `BuildDeployBundle`, and `BuildTeardownBundle` are pinned for happy-path assembly, subset shape (teardown never builds the vault; apply never builds the gateway, routing, or standalone container backend), all seven slot-prefixed failure wraps with cause preserved and the log writer closed iff it was opened, and the cleanup func — via package-level constructor variables in `internal/app/components.go`.

**Independent Test**: `go test -race -run 'TestBuild|TestBundleCleanup|TestJoinCleanup|TestRoutingFromPlugin|TestResolveOptionalSpecsDir' -v ./internal/app/` passes with no Docker daemon, network, credentials, or home directory, and creates no `logs/` directory anywhere.

### Tests for User Story 1 (write FIRST — T002/T003 must FAIL to compile until T004 lands) ⚠️

- [X] T002 [US1] Create `internal/app/testdoubles_test.go` (`package app`) per data-model §4 "Test doubles": `fakeFileLogger{created bool; stateDir string; events []engine.Event; closes int; closeErr error}` with `OnEvent` appending and `Close` incrementing `closes` and returning `closeErr`; `swapConstructor[T any](t *testing.T, target *T, replacement T)` that saves `*target`, assigns, and restores in `t.Cleanup`; `useInMemoryFileLogger(t) *fakeFileLogger` that swaps `newFileLogger` for a closure setting `created = true`, recording `stateDir`, and returning the fake as a `closableObserver`; `pinHermeticEnv(t)` doing `t.Setenv("HOME", "/home/test-user")`, `t.Setenv("DOCKER_HOST", "")`, `t.Setenv("DOCKER_CERT_PATH", "")`; `minimalConfig()` → `&config.Config{SpecsDir: "/abs/specs"}`; `emptyStore()` → `&state.Store{}`; `fakePaths()` → `&config.Paths{StateDir: "/nonexistent/state"}`; failing stand-ins returning `(nil, sentinel)` and must-not-be-called stand-ins calling `t.Fatalf` — one small closure per swapped constructor signature (`newFileLogger`, `newVault`, `newContainerBackend`, `newTraefikPlugin`, `newLocalEngine`), since a single generic cannot synthesise a func of arbitrary signature; and `assertSlotFailure(t, err, bundleIsNil, cleanupIsNil bool, prefix string)` asserting `err != nil`, `strings.HasPrefix(err.Error(), prefix)`, and both nil flags true. No `t.Parallel()` anywhere in this package's swapping tests
- [X] T003 [US1] Add the happy-path tests to `internal/app/app_test.go`, leaving the three existing spec-026 tests byte-for-byte untouched (coverage-matrix A-1…A-3, data-model §1): `TestBuildApplyBundle_ComposesMinimalConfig`, `TestBuildDeployBundle_ComposesMinimalConfig`, `TestBuildTeardownBundle_ComposesMinimalConfig`. Each calls `pinHermeticEnv(t)` and `useInMemoryFileLogger(t)`, builds from `minimalConfig()`/`emptyStore()`/`fakePaths()` with a `bytes.Buffer` for `out`, and asserts: no error; `Out`/`ErrOut`/`Cfg`/`Store`/`Paths` identity where the bundle has the slot; `Observer` is a non-nil `engine.MultiObserver` of length 2; `Engine` non-nil with `Engine.Observer == Observer`; cleanup non-nil; `fake.stateDir == paths.StateDir`; nothing written to `out`. Apply and deploy also assert `!bundle.Vault.IsActive()` — never `bundle.Vault == nil` (typed-nil, research D5). Deploy also asserts `SpecsDir == "/abs/specs"`, `ContainerBackend != nil`, `Routing == nil`, `Engine.Routing == nil`. Teardown also asserts `SpecsDir == "/abs/specs"`, `Routing == nil`, plus a sub-case with `SpecsDir: ""` → success and `SpecsDir == ""`. Run `go test ./internal/app/` and confirm it fails to compile on `newFileLogger`/`closableObserver` (red)

### Implementation for User Story 1

- [X] T004 [US1] Introduce the composition-root seam in `internal/app/components.go` exactly as contracts/test-seams.md §2: declare `type closableObserver interface { engine.Observer; Close() error }`; replace the funcs `newVault`, `newContainerBackend`, `newTraefikPlugin`, `newLocalEngine` with a `var (...)` block of the same names and signatures (`newVault` and `newTraefikPlugin` as closures over `infisicalplugin.New(cfg.Plugins.Secrets.Infisical)` and `traefik.New(cfg.Plugins.Gateway.Traefik, container, specsDir, observer)`; `newContainerBackend = local.NewContainerBackend`; `newLocalEngine = local.NewLocalEngine`); add `newFileLogger = func(stateDir string) (closableObserver, error)` wrapping `ui.NewFileLogger` with an explicit `if err != nil { return nil, err }` so the error path returns a true-nil interface; change `newObserverPair` to call `newFileLogger(paths.StateDir)` and keep its existing `initializing file logger:` wrap and `Close` hand-off. One WHY comment on the var block (the variables exist so same-package tests can substitute collaborators; production never reassigns them). `internal/app/app.go` call sites must compile unchanged. Run `go test ./internal/app/` → T003 green, spec-026 tests still green
- [X] T005 [US1] Add the subset-shape tests to `internal/app/app_test.go` (coverage-matrix A-4, A-5; data-model §3 "Subset-shape rows"): `TestBuildTeardownBundle_NeverConstructsVault` swaps `newVault` for the must-not-be-called stand-in and asserts assembly succeeds; `TestBuildApplyBundle_NeverConstructsGatewayOrContainerBackend` swaps `newTraefikPlugin` and `newContainerBackend` for must-not-be-called stand-ins and asserts success, plus a sub-case with the real constructors and an invalid gateway config (`cfg.Plugins.Gateway.Traefik = &config.TraefikPluginConfig{Dashboard: &config.TraefikDashboardConfig{Port: 8080}}`, no credentials) that still succeeds. Each test calls the returned cleanup
- [X] T006 [US1] Add `TestBuildBundles_SlotFailures` to `internal/app/app_test.go` as one table driven by data-model §3 (coverage-matrix A-6) — 15 new rows; the two `resolving specsDir:` rows stay in the existing spec-026 tests. Rows: `validating registries: ` × apply, deploy (real: `Registries: []config.RegistryConfig{{Alias: "bad alias!"}}`; cause contains `registries: alias "bad alias!" contains invalid characters`; logger never created); `observer: ` × apply, deploy, teardown (injected: `newFileLogger` returns `(nil, sentinel)`; `errors.Is(err, sentinel)` and message contains `initializing file logger:`); `container backend: ` × deploy (injected); `traefik: ` × deploy, teardown (real: dashboard port 8080 without credentials; cause contains `dashboard.port is set but username and password are required`); `vault: ` × apply, deploy (injected); `routing: ` × deploy, teardown (real: `Traefik: &config.TraefikPluginConfig{RoutingDir: "~/routes"}` with `t.Setenv("HOME", "")` after `pinHermeticEnv`; deploy passes an absolute `manifestDir`, teardown keeps the absolute `SpecsDir`; cause contains `resolving routing-dir: expanding ~`); `engine: ` × apply, deploy, teardown (injected). Every row calls `assertSlotFailure` and then asserts the cause (`errors.Is` for injected rows, substring for real ones) and the log writer: `fake.closes == 1` for every slot after the observer (`container backend`, `traefik`, `vault`, `routing`, `engine`), `fake.created == false` for `validating registries`
- [X] T007 [US1] Add the cleanup tests to `internal/app/app_test.go` (coverage-matrix A-9…A-11, research D6): `TestBundleCleanup_ClosesLogWriterOnce` — after a successful assembly `cleanup()` returns nil and `fake.closes == 1`; `TestBundleCleanup_ReportsCloseError` — `fake.closeErr = sentinel` → `errors.Is(cleanup(), sentinel)`; `TestBundleCleanup_SecondCallDoesNotPanic` — a second `cleanup()` returns without panicking and `fake.closes == 2` (pins the observed behaviour)
- [X] T008 [P] [US1] Create `internal/app/components_test.go` (`package app`; coverage-matrix A-12, A-13): `TestJoinCleanup` — nil closers are skipped, every closer is invoked, two failing closers → `errors.Is` finds both sentinels, no failures → nil; `TestRoutingFromPlugin` — an inactive plugin (`traefik.New(nil, nil, "/abs/specs", engine.NoopObserver{})` or the package's equivalent no-op observer) → `(nil, nil)`; an active plugin with `RoutingDir: "~/x"` under `t.Setenv("HOME", "")` → error containing `resolving routing-dir`
- [X] T009 [P] [US1] Correct the `BuildApplyBundle` doc comment in `internal/app/app.go` (comment only, contracts/test-seams.md §3): replace "non-nil and idempotent — callers MUST defer it" with "non-nil and safe to call more than once (a repeated call reports the already-closed writer) — callers MUST defer it". No code change in this file
- [X] T010 [US1] Verify the story at repository root: `go test -race -count=1 -run 'TestBuild|TestBundleCleanup|TestJoinCleanup|TestRoutingFromPlugin|TestResolveOptionalSpecsDir' -v ./internal/app/` passes; `git diff --stat -- internal/app/` shows only `components.go`, `app.go` (comment), `app_test.go` (additions only), and the two new test files; `git diff -- cmd/` is empty (017 FR-002 and Constitution II untouched)

**Checkpoint**: US1 is complete and independently verifiable — every slot prefix, both subset shapes, and the unwind are pinned (SC-001).

---

## Phase 4: User Story 2 - Every line the terminal shows during a deploy or teardown is pinned (Priority: P2)

**Goal**: All 33 rendered event kinds have an exact-line test; the generic `❌ Error [<kind>]: …` line, the progress-indicator lifecycle for the step-style kinds, and the intentionally silent cases are pinned. No production change — `internal/ui/terminal_logger.go` is not edited.

**Independent Test**: `go test -race -run 'TestTerminalObserver' -v ./internal/ui/` passes; no test outlives its spinner goroutine; the package adds roughly 1–2 s of wall-clock.

### Tests for User Story 2 (characterisation — pass on first run against existing behaviour)

- [X] T011 [US2] Add shared helpers to `internal/ui/terminal_logger_test.go`, keeping the existing `container.create` tests verbatim (coverage-matrix C helpers, research D8): `safeBuffer` (a `sync.Mutex` around `bytes.Buffer` with locked `Write` and `String`) with a one-line WHY comment (the spinner goroutine writes concurrently with `OnEvent`, as `os.Stdout` allows in production); `newObserverWithBuffer(t) (*TerminalObserver, *safeBuffer)`; `stopSpinner(t, obs)` that stops and nils a still-running `obs.spinner`, for registration in `t.Cleanup`; `ev(name string, status engine.Status, kv ...string) engine.Event` building the field map from key/value pairs
- [X] T012 [US2] Add `TestTerminalObserver_RendersEachKind` to `internal/ui/terminal_logger_test.go` — a table with one sub-test per row 1–28 of data-model §5 "Single-line kinds", each with every listed field populated by a distinct value and the status from the "Status gate" column (`started` for the four lifecycle headers, `info` otherwise, `info` + `reason=not found` for row 28), asserting `buf.String() == want` byte-exact including the trailing `\n` and the leading indentation shown in the catalogue (coverage-matrix C-1). Row 7 here uses an event without `aliases`
- [X] T013 [US2] Add to `internal/ui/terminal_logger_test.go` (coverage-matrix C-2, C-3): `TestTerminalObserver_RoutingConfigureAliases` — with `aliases` set the routing line is followed by `    ↳ Aliases: <aliases>\n`, without it only the routing line is written; `TestTerminalObserver_GenericErrorLine` — `routing.finalize` at `error` with `error=boom` writes exactly `  ❌ Error [routing.finalize]: boom\n`, and an any-status kind (`dns.register`) at `error` writes the generic line followed by its kind line
- [X] T014 [P] [US2] Create `internal/ui/terminal_logger_steps_test.go` (`package ui`) with one table of the step kinds that start an indicator — data-model §5 rows 29 (`network.create`), 30 (`network.remove`), 31 (`container.remove`), 32 (`volume.create`, start status **`info`**, empty finished line), 34 (`image.pull`, field `ref`) — carrying start status, fields, expected `prefix+msg`, and expected finished line, driving three tests (coverage-matrix C-5…C-7): `TestTerminalObserver_StepStartsIndicator` — after the start event `obs.spinner != nil` and `obs.spinner.msg == want`; `TestTerminalObserver_StepFinishedStopsIndicatorAndPrintsOnce` — after `finished`, `obs.spinner == nil`, `strings.Count(out, "\r\033[K") == 1`, and the finished line appears exactly once (zero lines for `volume.create`); `TestTerminalObserver_StepErrorStopsIndicatorWithoutCompletion` — after `error`, `obs.spinner == nil`, the generic error line is present, and the finished line count is 0. Every sub-test registers `stopSpinner` in `t.Cleanup`; assert on counts and presence only, never on animation-frame bytes or whole-buffer equality. Depends on T011 for the helpers
- [X] T015 [US2] Add the remaining lifecycle tests to `internal/ui/terminal_logger_steps_test.go` (coverage-matrix C-8…C-11): `TestTerminalObserver_FinishedWithoutStart` — the finished line is written, no `\r\033[K`, and the call returns promptly; `TestTerminalObserver_VolumeCreatedCompletesVolumeIndicator` — `volume.create` `info` starts the indicator, `volume.created` `finished` leaves `obs.spinner == nil` and writes `    ✅ Volume <name> is created` once; `TestTerminalObserver_ContainerRemoveNotFoundSkipsIndicator` — `info` + `reason=not found` writes the skip line and `obs.spinner == nil`; `TestTerminalObserver_WarningLeavesIndicatorRunning` — after a start, a `warning` event leaves `obs.spinner` the same pointer and adds no completion line (the indicator is stopped by cleanup). Do not start a second indicator while one is running — that behaviour is out of scope
- [X] T016 [US2] Add `TestTerminalObserver_SilentKinds` to `internal/ui/terminal_logger_steps_test.go` (coverage-matrix C-12, FR-010): `routing.finalize` at `started` and at `info` → `""`; status-gated kinds at a status they do not render (`application.deploy` `finished`, `container.create` `started`) → `""`
- [X] T017 [US2] Verify the story at repository root: `go test -race -count=3 -run 'TestTerminalObserver' ./internal/ui/` passes on all three runs; `go test -count=1 -run 'TestTerminalObserver' -v ./internal/ui/` shows a sub-test for each of the 28 single-line rows plus the step kinds (33 `case` arms covered, SC-002); `git diff -- internal/ui/terminal_logger.go` is empty

**Checkpoint**: US2 is complete — every rendered kind, the error line, the indicator lifecycle, and silence are pinned.

---

## Phase 5: User Story 3 - A handler can be exercised with stand-in collaborators (Priority: P3)

**Goal**: `handler.Teardown` is unit-tested through an `app.TeardownBundle` built by hand from in-memory stand-ins, closing spec 017 US3 / FR-006 and giving future handler tests a pattern. No production change.

**Independent Test**: `go test -race -run 'TestTeardown' -v ./internal/handler/` passes with no Docker, network, filesystem, or credentials, and `grep -n 'app\.Build\|local\.\|traefik\.\|infisical\|ui\.' internal/handler/teardown_test.go` prints nothing.

### Tests for User Story 3 (characterisation — pass on first run against existing behaviour)

- [X] T018 [US3] Extend the existing fake in `internal/handler/deployments_test.go`: add a `listErr error` field to `memDeploymentStore` and make `List` return `(nil, listErr)` when it is set; every existing test in the package must still pass unchanged
- [X] T019 [US3] Create `internal/handler/teardown_test.go` (`package handler`) with the stand-ins of data-model §7: `recordingContainerBackend{removed []engine.RemoveContainerOp; networks []string; removeErr error}` implementing `engine.ContainerBackend` — `RemoveContainer` appends the op then returns `removeErr`, `RemoveNetwork` appends the team, every other method returns zero values (embed the package's existing `stubContainerBackend` for those if it satisfies the interface cleanly); `recordingObserver{events []engine.Event}` implementing `engine.Observer`; and a helper returning `&app.TeardownBundle{Out: &bytes.Buffer{}, Cfg: &config.Config{}, Store: &state.Store{Deployments: dep}, Engine: &engine.Engine{Container: rec, Observer: obs}}` (`Routing` and `Resolver` left nil). The file must not import or call `app.Build*`, `internal/engine/local`, any plugin package, or `internal/ui` — the bundle literal is the only use of `app`
- [X] T020 [US3] Add `TestTeardown_RemovesPlannedDeploymentsThenNetwork` to `internal/handler/teardown_test.go` (coverage-matrix B-1): with the store listing `{Kind: Resource, Name: "db"}` then `{Kind: Application, Name: "web"}` for team `t`, `Teardown(bundle, t)` returns nil; `rec.removed` equals `[{Team: t, Name: "web"}, {Team: t, Name: "db"}]` (application before resource); `rec.networks` equals `[t]`; `obs.events` contains an `application.teardown` and a `resource.teardown` event at `started` with the matching `team` and `name` fields
- [X] T021 [US3] Add the failure tests to `internal/handler/teardown_test.go` (coverage-matrix B-2, B-3): `TestTeardown_ReturnsListErrorWithoutTouchingBackend` — `listErr` set → `errors.Is(err, listErr)`, `rec.removed` and `rec.networks` empty; `TestTeardown_StopsAtFirstRemovalFailure` — `removeErr` set → `errors.Is(err, removeErr)`, `len(rec.removed) == 1`, `rec.networks` empty, and `obs.events` contains an `application.remove` event at `error`
- [X] T022 [US3] Verify the story at repository root: `go test -race -count=1 -run 'TestTeardown' -v ./internal/handler/` shows three passing tests; `go test -count=1 ./internal/handler/` (whole package) passes; the isolation guard `grep -n 'app\.Build\|local\.\|traefik\.\|infisical\|ui\.' internal/handler/teardown_test.go` prints nothing (SC-004)

**Checkpoint**: US3 is complete — the 017 isolation seam is proven by a real handler test.

---

## Phase 6: User Story 4 - The on-disk log's line format is pinned (Priority: P4)

**Goal**: The file logger's line grammar, its "every event, every status" behaviour, whole-line writes under concurrency, and `Close` forwarding are pinned in memory through an injectable `io.WriteCloser`; the log's location and append-across-runs behaviour are pinned by one Docker integration scenario.

**Independent Test**: `go test -race -run 'TestFileLogger' -v ./internal/ui/` passes without creating any file; `go vet -tags integration ./tests/integration/...` compiles `tests/integration/file_logger_test.go` (CI executes it).

### Tests for User Story 4 (write FIRST — T023 must FAIL to compile until T024 lands) ⚠️

- [X] T023 [US4] Create `internal/ui/file_logger_test.go` (`package ui`) with helpers and the two format tests (coverage-matrix D-1, D-2, research D10, data-model §6): `countingCloser` wrapping a `bytes.Buffer` with `closes int` and `closeErr error` (use it as the no-op write-closer too); `assertLogLine(t, line, wantRest)` requiring `len(line) >= 21`, `time.Parse(time.RFC3339, line[:20])` succeeds, `line[19] == 'Z'`, and `line[20:] == wantRest`. `TestFileLogger_FormatsFieldsSortedAndQuoted` — `{container.create, started, {team: "shrine-deploy-test", name: "alias-app"}}` → rest ` [started] container.create name="alias-app" team="shrine-deploy-test"`; and `{container.create, error, {error: creating "x": bad "ref"}}` → rest ` [error] container.create error="creating \"x\": bad \"ref\""`. `TestFileLogger_FieldlessLineEndsAfterName` — `{routing.finalize, info, nil}` → rest ` [info] routing.finalize`. Build events with plain `engine.Event{...}` literals (do not rely on US2's `ev` helper, so the stories stay independent). Construct the logger with `newFileLogger(closer)`; run `go test ./internal/ui/` and confirm the compile failure on `newFileLogger` (red). While red, the whole `ui` test package does not compile — land T024 immediately after

### Implementation for User Story 4

- [X] T024 [US4] Introduce the file-logger seam in `internal/ui/file_logger.go` exactly as contracts/test-seams.md §1: rename the field to `out io.WriteCloser`; add unexported `func newFileLogger(out io.WriteCloser) *FileLogger` (no I/O) with one WHY comment (the writer is injectable so the line format can be tested without a file); `NewFileLogger(stateDir)` keeps `MkdirAll(<stateDir>/logs, 0755)`, `OpenFile(shrine.log, O_APPEND|O_CREATE|O_WRONLY, 0644)`, and both existing error texts, then returns `newFileLogger(f), nil`; `OnEvent` writes to `l.out` under `mu`; `Close` returns `l.out.Close()`. Output must stay byte-identical. Run `go test ./internal/ui/` → T023 green
- [X] T025 [US4] Add the behaviour tests to `internal/ui/file_logger_test.go` (coverage-matrix D-3…D-5): `TestFileLogger_WritesOneLinePerEventForEveryStatus` — one event per status (`started`, `finished`, `info`, `warning`, `error`) → exactly 5 lines, each passing `assertLogLine` with its `[<status>]`; `TestFileLogger_ConcurrentEventsProduceWholeLines` — 50 goroutines × 20 events, each carrying `team` and `name` fields → exactly 1000 lines, every one passing `assertLogLine` against the expected sorted-field remainder (the buffer is only read after `wg.Wait()`); `TestFileLogger_CloseClosesDestination` — `Close()` → `closes == 1`, and a `closeErr` is returned via `errors.Is`
- [X] T026 [P] [US4] Create `tests/integration/file_logger_test.go` (`//go:build integration`, `package integration_test`, using only `tests/integration/testutils` — no `internal/` imports) per research D11 and coverage-matrix E-1: `TestFileLogger` on `NewDockerSuite(t, testTeam)` with a `BeforeEach` mirroring `TestTeardown` in `tests/integration/teardown_test.go` (temp state dir, `SeedSubnetState`, `apply teams --path fixturesPath("team")`, `deploy --path fixturesPath("basic")`); scenario `"should create the log on first use and append across runs"`: `log := filepath.Join(tc.StateDir, "logs", "shrine.log")`; `AssertFileExists(log)`; `AssertFileContains(log, "[started] application.deploy")`; `tc.Run("teardown", testTeam, "--state-dir", tc.StateDir).AssertSuccess()`; then `AssertFileContains` for both `"[started] application.deploy"` (survival = append proof) and `"[started] application.teardown"`. Compile-check only with `go vet -tags integration ./tests/integration/...` — do not run it locally; CI is the gate
- [X] T027 [P] [US4] List the scenario in `specs/features/integration-tests.md`: add a `### File logger — spec 027` subsection after the `### Config path resolution — spec 026` block, naming `TestFileLogger` (`tests/integration/file_logger_test.go`, Docker) and its assertions — `<state>/logs/shrine.log` exists after `apply teams` + `deploy` and holds `[started] application.deploy`; after `teardown` that entry is still present and `[started] application.teardown` follows (FR-013)
- [X] T028 [US4] Verify the story at repository root: `go test -race -count=3 -run 'TestFileLogger' -v ./internal/ui/` passes on all three runs; `go test -count=1 ./internal/ui/ ./internal/app/` passes (the `app` package consumes `ui.NewFileLogger` unchanged); `go vet -tags integration ./tests/integration/...` is clean; `git status --short` shows no `logs/` directory or `shrine.log` created by the unit run

**Checkpoint**: All four stories are independently complete.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Prove the suite is hermetic, race-free, fast, and actually bites (SC-005, SC-006, SC-007), then close the bookkeeping.

- [X] T029 Run the hermetic and race gates from quickstart.md §1–§2 at repository root: `env -u HOME -u DOCKER_HOST -u DOCKER_CERT_PATH -u DOCKER_TLS_VERIFY go test -count=1 ./internal/app/ ./internal/ui/ ./internal/handler/` passes, and `go test -race -count=3 ./internal/app/ ./internal/ui/ ./internal/handler/` passes; compare wall-clock against the T001 baseline and confirm the added tests cost under 5 s (SC-005); `git status --short` shows no stray files
- [X] T030 Run the mutation spot-checks from quickstart.md §4 plus the fifth from coverage-matrix §F, reverting with `git checkout <file>` after each and confirming `FAIL` then `ok`: (i) drop the `vault:` prefix in `internal/app/app.go`; (ii) delete one `_ = closeObserver()` in `internal/app/app.go`; (iii) render `e.Fields["ref"]` instead of `["name"]` in the `container.fresh` line of `internal/ui/terminal_logger.go`; (iv) remove `sort.Strings(keys)` in `internal/ui/file_logger.go` (run with `-count=3`); (v) add a `newVault(cfg)` call to `BuildTeardownBundle` in `internal/app/app.go`. Finish with `git diff --stat` showing no leftover mutation (SC-007)
- [X] T031 [P] Add the feature entry to `specs/progress.md` directly above the 026 entry, same style: `- [x] **Test: unit coverage for the composition root and event renderers** — see `specs/027-app-ui-unit-coverage/` (issue #40). …` summarising the two seams (`newFileLogger(io.WriteCloser)` in `internal/ui`; constructor variables + `closableObserver` in `internal/app/components.go`), what is now pinned (bundle happy paths, subset shapes, seven slot prefixes with unwind, cleanup; 33 terminal kinds, error line, indicator lifecycle, silence; `handler.Teardown` over stand-ins; log line grammar and concurrency), the acceptance criteria SC-001…SC-007 in one line each, and the gate (`TestFileLogger` in `tests/integration/file_logger_test.go`, CI executes)
- [X] T032 Run the full local gate at repository root: `gofmt -l .` prints nothing, `go vet ./...` clean, `go build ./...`, `go test ./...` zero failures with every pre-existing test unmodified, `go vet -tags integration ./tests/integration/...` clean; confirm `git diff --stat main -- cmd/ internal/engine/ internal/plugins/ internal/handler/teardown.go internal/ui/terminal_logger.go` is empty (FR-014, SC-006)
- [X] T033 [P] Run `graphify update .` to refresh the knowledge graph after the code changes (project rule, AST-only)
- [ ] T034 Push the branch and confirm the CI pipeline is green — `go test ./...` plus `make test-integration`, which executes `TestFileLogger` (T026) alongside the unchanged `TestTeardown` and `TestConfigPathResolution`; this is the Constitution V gate

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: none — start immediately
- **Foundational (Phase 2)**: empty
- **User Stories (Phases 3–6)**: each depends only on T001; no story depends on another
- **Polish (Phase 7)**: T029, T030, T032 need all four stories; T031 and T033 need the code to be final; T034 is last

### User Story Dependencies

- **US1 (P1)**: independent. Owns `internal/app/*`
- **US2 (P2)**: independent. Owns `internal/ui/terminal_logger_test.go` and `internal/ui/terminal_logger_steps_test.go`
- **US3 (P3)**: independent. Owns `internal/handler/teardown_test.go` and one field in `internal/handler/deployments_test.go`
- **US4 (P4)**: independent. Owns `internal/ui/file_logger.go`, `internal/ui/file_logger_test.go`, `tests/integration/file_logger_test.go`
- **One shared-package caveat**: US2 and US4 both add test files to `package ui`. They touch different files, but between T023 (red) and T024 (green) the `ui` test package does not compile, so US2's tests cannot run in that window — do T023 → T024 back to back. Keep helper names distinct across the two stories (`safeBuffer`/`ev`/`stopSpinner` in US2; `countingCloser`/`assertLogLine` in US4)

### Within Each User Story

- **US1**: T002 → T003 (red) → T004 (green) → T005 → T006 → T007 (all in `app_test.go`, sequential); T008 and T009 are parallel once T004 has landed; T010 last
- **US2**: T011 → T012 → T013 (same file); T014 is parallel with T012/T013 once T011 has landed; T014 → T015 → T016 (same file); T017 last
- **US3**: T018 → T019 → T020 → T021 → T022
- **US4**: T023 (red) → T024 (green) → T025; T026 and T027 are parallel with all of them; T028 last

### Parallel Opportunities

- After T001, all four stories can proceed in parallel (four different file sets)
- Inside US1: T008 ∥ T009 ∥ (T005 → T006 → T007)
- Inside US2: T014 ∥ (T012 → T013)
- Inside US4: T026 ∥ T027 ∥ (T023 → T024 → T025)
- Polish: T031 ∥ T033

---

## Parallel Example: User Story 1

```bash
# After T004 (seam) is green, these touch three different files:
Task: "T005–T007 subset-shape, slot-failure, and cleanup tests in internal/app/app_test.go"
Task: "T008 TestJoinCleanup and TestRoutingFromPlugin in internal/app/components_test.go"
Task: "T009 BuildApplyBundle doc-comment correction in internal/app/app.go"
```

## Parallel Example: across stories

```bash
# After T001, four independent tracks:
Task: "US1 — internal/app (T002 → T010)"
Task: "US2 — internal/ui terminal observer tests (T011 → T017)"
Task: "US3 — internal/handler teardown test (T018 → T022)"
Task: "US4 — internal/ui file logger + tests/integration (T023 → T028)"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. T001 baseline
2. T002–T010 — composition-root seam and tests
3. **STOP and VALIDATE**: `go test -race ./internal/app/` plus mutations (i), (ii), (v) from T030 — the largest gap in issue #40 is closed and shippable on its own

### Incremental Delivery

1. US1 → wiring, subset shapes, and failure messages pinned (SC-001)
2. US2 → every terminal line pinned (SC-002) — no production change
3. US3 → handler isolation seam proven (SC-004) — no production change
4. US4 → log format pinned in memory and on disk (SC-003)
5. Polish → hermetic, race, time-budget, mutation, and CI gates (SC-005…SC-007)

Each story is a separate, reviewable commit; any prefix of the list leaves the suite green.

---

## Notes

- Unit tests must not touch the filesystem — no `t.TempDir`, `os.MkdirAll`, or file writes; `t.Setenv` is the only environment manipulation (FR-001)
- Integration tests are never run locally: author, `go vet -tags integration`, let CI execute
- `tests/integration/` stays isolated from `internal/` packages — use `testutils` only
- Tests in `package app` that swap constructor variables must not call `t.Parallel()`
- Assert the absent vault with `!Vault.IsActive()`, never `== nil` (typed-nil representation is preserved, research D5)
- Spinner tests assert counts and presence, never animation bytes; each stop waits at most one 100 ms frame
- Existing tests (`internal/app/app_test.go` spec-026 tests, the `container.create` tests in `internal/ui/terminal_logger_test.go`) stay verbatim
- If a pinned behaviour turns out to differ from what data-model.md documents, pin what the code does and record the discrepancy in the implementation notes — do not change production behaviour (FR-014)

---

## Implementation Notes

Deviations from the design documents, recorded per FR-014 ("pin what the code does; do not change production behaviour"):

1. **Teardown event names are capitalised.** `engine.teardownKind` builds names as `kind + ".teardown"` from the recorded manifest kind (`"Application"` / `"Resource"`), so production emits `Application.teardown`, `Resource.teardown`, `Application.remove`. data-model §7, research D7/D11 and coverage-matrix B-1/B-3/E-1 assumed lowercase. Consequences: (a) `internal/handler/teardown_test.go` asserts the observed names via `manifest.ApplicationKind+".teardown"`; (b) the terminal observer's `application.teardown` / `resource.teardown` arms are pinned by the catalogue (they render correctly when given those names) but are unreachable in a real teardown — the "Tearing down Application…" header never prints; (c) `tests/integration/file_logger_test.go` proves append with `[finished] network.remove name="shrine.<team>.private"` — a teardown-only entry — instead of `[started] application.teardown`, so the scenario survives a future casing fix. The fix itself is a user-visible behaviour change and is out of scope here; it is tracked as a separate task. **Resolved by spec 028 (issue #46)**: the engine now lowercases the kind when naming teardown events, `internal/handler/teardown_test.go` asserts `application.teardown` / `resource.teardown` / `application.remove`, and the header prints in a real teardown.
2. **Observer identity cannot be compared with `==`.** `engine.MultiObserver` is a slice, so `bundle.Engine.Observer == bundle.Observer` panics at runtime. The happy-path tests instead emit a silent event through each and assert it reaches the fake file logger (`assertReachesLogger`).
3. **Stand-ins are per signature, not generic.** `failing[T]` / `mustNotBeCalled[T]` from coverage-matrix §A cannot be written — a Go generic cannot synthesise a func of arbitrary signature — so `testdoubles_test.go` has one small closure per swapped constructor. A `bundleBuilder` value per command erases the three bundle types so one table drives the slot-failure and cleanup tests.
4. **Catalogue statuses follow the real emitters**: `warning` for the ⚠️ gateway kinds, `finished` for `container.created` and `hostport.published`, `info` or `started` otherwise.
5. **Extra coverage beyond the matrix**: cleanup-closes-once runs for all three bundles; `TestRoutingFromPlugin` also pins the active-plugin branch; `TestTerminalObserver_FinishedWithoutStart` covers every step kind plus `volume.created`; the silent-kinds test covers all five status-gated kinds.
6. **T001 baseline**: `gofmt -l .` already listed 12 pre-existing files (including `internal/ui/terminal_logger.go` and `internal/handler/deployments_test.go`). They were left as they are; every file added or seam-edited by this feature is gofmt-clean.
7. **T030 mutations**: files were backed up and restored rather than `git checkout`-ed (the work was uncommitted). Mutation (iv) reversed the sort instead of deleting `sort.Strings(keys)`, because deleting it fails compilation on the unused import rather than exercising the test. Two extra mutations were run: removing the file logger's mutex (caught by `-race`) and leaving the spinner running on an error event. All seven were caught.
8. **T034** (CI green) stays open until the PR pipeline finishes — the integration suite is executed only there.
