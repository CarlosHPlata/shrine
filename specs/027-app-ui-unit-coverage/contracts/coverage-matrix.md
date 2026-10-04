# Contract: Coverage Matrix

**Feature**: `027-app-ui-unit-coverage` | **Date**: 2026-08-28
**Scope**: Every test this feature adds, what it asserts, and which spec requirement / success criterion it closes. All unit tests are hermetic (FR-001): no filesystem, network, Docker daemon, or credentials; `t.Setenv` only; pass under `go test -race`.

## A. `internal/app` (package `app`)

| ID | Test | Assertions | Closes |
|----|------|------------|--------|
| A-1 | `TestBuildApplyBundle_ComposesMinimalConfig` | success; slots per data-model §1 (Out/ErrOut/Cfg/Store/Paths identity, Observer len-2 MultiObserver, `!Vault.IsActive()`, Engine non-nil with `Engine.Observer == Observer`); cleanup non-nil; fake logger `stateDir == paths.StateDir`; nothing written to `out` | FR-002, SC-001 |
| A-2 | `TestBuildDeployBundle_ComposesMinimalConfig` | as A-1 plus `SpecsDir == "/abs/specs"`, `ContainerBackend` non-nil, `Routing == nil`, `Engine.Routing == nil` | FR-002, SC-001 |
| A-3 | `TestBuildTeardownBundle_ComposesMinimalConfig` | as A-1 (no ErrOut/Vault) plus `SpecsDir == "/abs/specs"`, `Routing == nil`; sub-case with `SpecsDir: ""` → `SpecsDir == ""` and success | FR-002, SC-001 |
| A-4 | `TestBuildTeardownBundle_NeverConstructsVault` | `newVault` swapped with `mustNotBeCalled`; assembly succeeds | FR-003, SC-001 |
| A-5 | `TestBuildApplyBundle_NeverConstructsGatewayOrContainerBackend` | `newTraefikPlugin` and `newContainerBackend` swapped with `mustNotBeCalled`; assembly succeeds; second sub-case with the real invalid Traefik config (dashboard without credentials) also succeeds | FR-003, SC-001 |
| A-6 | `TestBuildBundles_SlotFailures` (table, data-model §3: 8 prefixes × affected bundles = 17 rows incl. the existing `specsDir` rows kept as they are) | `HasPrefix(err, "<slot>: ")`; cause preserved (`errors.Is` / substring); `bundle == nil && cleanup == nil`; fake logger `closes == 1` iff the slot follows the observer, `created == false` otherwise | FR-004 (a)–(d), SC-001, SC-007 |
| A-7 | `TestBuildDeployBundle_FailsBeforeConstructionWhenSpecsDirUnresolvable` (existing) | unchanged | spec 026 |
| A-8 | `TestBuildTeardownBundle_FailsBeforeConstructionWhenSpecsDirUnresolvable` (existing) + `TestResolveOptionalSpecsDir` (existing) | unchanged | spec 026 |
| A-9 | `TestBundleCleanup_ClosesLogWriterOnce` | after successful assembly, `cleanup()` → nil; `closes == 1` | FR-005 |
| A-10 | `TestBundleCleanup_ReportsCloseError` | fake `closeErr = sentinel` → `errors.Is(cleanup(), sentinel)` | FR-005 |
| A-11 | `TestBundleCleanup_SecondCallDoesNotPanic` | second `cleanup()` returns without panic; `closes == 2` (documents the observed behaviour — research D6) | spec edge case |
| A-12 | `TestJoinCleanup` | nil closers skipped; all closers invoked; two failing closers → `errors.Is` finds both; no failures → nil | FR-005 |
| A-13 | `TestRoutingFromPlugin` | inactive plugin (`traefik.New(nil, …)`) → `(nil, nil)`; active plugin with `RoutingDir: "~/x"` and `HOME=""` → error containing `resolving routing-dir` | FR-004 (routing slot cause) |

Helpers (`testdoubles_test.go`): `fakeFileLogger`, `swapConstructor[T]`, `useInMemoryFileLogger`, `pinHermeticEnv`, `minimalConfig`, `emptyStore`, `fakePaths`, `failing[T](sentinel)`, `mustNotBeCalled[T](t)`, `assertSlotFailure(t, err, bundleNil, cleanupNil, prefix, cause)`.

## B. `internal/handler` (package `handler`)

| ID | Test | Assertions | Closes |
|----|------|------------|--------|
| B-1 | `TestTeardown_RemovesPlannedDeploymentsThenNetwork` | bundle built by hand from stand-ins (data-model §7); `removed == [{t, web}, {t, db}]` (application before resource); `networks == [t]`; observer saw `application.teardown` and `resource.teardown` started events (lowercase since spec 028 — see tasks.md Implementation Notes) with `team`/`name` fields; error nil | FR-006, SC-004 |
| B-2 | `TestTeardown_ReturnsListErrorWithoutTouchingBackend` | `listErr` → `errors.Is(err, listErr)`; `removed` and `networks` empty | FR-006 |
| B-3 | `TestTeardown_StopsAtFirstRemovalFailure` | `removeErr` → `errors.Is(err, removeErr)`; `len(removed) == 1`; `networks` empty; an `application.remove` `error` event observed | FR-006 |

Guard: none of B-* imports or calls `app.Build*`, `local.*`, `traefik.*`, `infisicalplugin.*`, `ui.*`; the bundle literal is the only use of `app`.

## C. `internal/ui` — terminal observer (package `ui`)

| ID | Test | Assertions | Closes |
|----|------|------------|--------|
| C-1 | `TestTerminalObserver_RendersEachKind` (table, data-model §5 rows 1–28 with populated fields) | `buf.String() == want` byte-exact per row | FR-007, SC-002 |
| C-2 | `TestTerminalObserver_RoutingConfigureAliases` | with `aliases` → routing line + `    ↳ Aliases: …`; without → routing line only | FR-007 |
| C-3 | `TestTerminalObserver_GenericErrorLine` | `error` status on a kind with no dedicated rendering (`routing.finalize`) → exactly `  ❌ Error [routing.finalize]: boom\n`; on an "any status" kind (`dns.register`) → generic line followed by the kind line | FR-008 |
| C-4 | `TestTerminalObserver_ContainerCreate` (existing) | unchanged | FR-007 |
| C-5 | `TestTerminalObserver_StepStartsIndicator` (table over rows 29–32, 34) | after start: `obs.spinner != nil`, `obs.spinner.msg == prefix+message` | FR-009 |
| C-6 | `TestTerminalObserver_StepFinishedStopsIndicatorAndPrintsOnce` (same table) | after finished: `obs.spinner == nil`; `Count(out, "\r\033[K") == 1`; `Count(out, finishedLine) == 1` (rows with a finished line); `volume.create` finished prints no line | FR-009 |
| C-7 | `TestTerminalObserver_StepErrorStopsIndicatorWithoutCompletion` (same table) | after error: `obs.spinner == nil`; generic error line present; `Count(out, finishedLine) == 0` | FR-009 |
| C-8 | `TestTerminalObserver_FinishedWithoutStart` | finished line only; no clear sequence; returns promptly | FR-009 |
| C-9 | `TestTerminalObserver_VolumeCreatedCompletesVolumeIndicator` | `volume.create` info → spinner; `volume.created` finished → spinner nil + `    ✅ Volume v is created` once | FR-009 |
| C-10 | `TestTerminalObserver_ContainerRemoveNotFoundSkipsIndicator` | info + `reason=not found` → skip line; `obs.spinner == nil` | FR-009 |
| C-11 | `TestTerminalObserver_WarningLeavesIndicatorRunning` | start, then `warning` → `obs.spinner` unchanged, no output added; cleanup stops it | spec edge case |
| C-12 | `TestTerminalObserver_SilentKinds` | `routing.finalize` started/info → `""`; status-gated kinds at another status (`application.deploy` finished, `container.create` started) → `""` | FR-010 |

Helpers: `safeBuffer` (mutex-guarded `bytes.Buffer` — research D8), `newObserverWithBuffer(t) (*TerminalObserver, *safeBuffer)`, `stopSpinner(t, obs)` registered in `t.Cleanup` by every test that starts an indicator, `ev(name, status, kv...)` event builder.

## D. `internal/ui` — file logger (package `ui`)

| ID | Test | Assertions | Closes |
|----|------|------------|--------|
| D-1 | `TestFileLogger_FormatsFieldsSortedAndQuoted` | `assertLogLine(line, ` [started] container.create name="alias-app" team="shrine-deploy-test"`)`; value with a space and a quote is `%q`-escaped | FR-011, SC-003 |
| D-2 | `TestFileLogger_FieldlessLineEndsAfterName` | `assertLogLine(line, " [info] routing.finalize")` | FR-011 |
| D-3 | `TestFileLogger_WritesOneLinePerEventForEveryStatus` | 5 statuses → 5 lines, each `assertLogLine` with `[<status>]` | FR-011 |
| D-4 | `TestFileLogger_ConcurrentEventsProduceWholeLines` | 50 goroutines × 20 events, each event carrying two fields (`team`, `name`) → exactly 1000 lines, every line passes `assertLogLine` (so unsorted or interleaved output is detected); run under `-race` | FR-012 |
| D-5 | `TestFileLogger_CloseClosesDestination` | counting closer `closes == 1`; close error forwarded | FR-011 (seam contract) |

Helper: `assertLogLine(t, line, wantRest)` — `time.Parse(time.RFC3339, line[:20])` succeeds, `line[19] == 'Z'`, `line[20:] == wantRest`; `nopWriteCloser` / `countingCloser` around `bytes.Buffer`.

## E. `tests/integration` (build tag `integration`, Docker)

| ID | Test | Assertions | Closes |
|----|------|------------|--------|
| E-1 | `TestFileLogger` / "should create the log on first use and append across runs" | after `BeforeEach` (apply teams + deploy basic): `AssertFileExists(<state>/logs/shrine.log)`, `AssertFileContains(log, "[started] application.deploy")`; after `teardown <testTeam>`: the deploy entry is still present and `[finished] network.remove name="shrine.<team>.private"` has been appended (a teardown-only entry that does not depend on the teardown event casing — see tasks.md Implementation Notes) | FR-013, SC-003 |

Authored and compile-checked locally (`go vet -tags integration ./tests/integration/...`); executed by CI (`make test-integration`).

## F. Cross-cutting verification (SC-005, SC-006, SC-007)

| Check | Command / method | Expected |
|-------|------------------|----------|
| Hermetic | `HOME= DOCKER_HOST= DOCKER_CERT_PATH= go test ./internal/app/ ./internal/ui/ ./internal/handler/` with Docker stopped / unreachable | pass |
| Race-free | `go test -race -count=3 ./internal/app/ ./internal/ui/ ./internal/handler/` | pass |
| Time budget | `go test -count=1 ./internal/ui/ ./internal/app/ ./internal/handler/` | each package well under 5 s combined |
| No behaviour change | `go test ./...` unchanged tests pass; CI integration suite green; `go build ./...` | pass |
| Mutation spot-checks (SC-007) | temporarily (i) drop `vault:` prefix, (ii) remove one `_ = closeObserver()`, (iii) swap `e.Fields["name"]`→`["ref"]` in a rendered line, (iv) remove `sort.Strings(keys)` in `formatFields`, (v) give teardown a `newVault` call | each mutation makes ≥1 unit test fail; revert |
