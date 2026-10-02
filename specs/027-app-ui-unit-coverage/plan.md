# Implementation Plan: Unit Coverage for the Composition Root and Event Renderers

**Branch**: `027-app-ui-unit-coverage` | **Date**: 2026-08-28 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/027-app-ui-unit-coverage/spec.md` (GitHub issue #40)

## Summary

Three packages that exist to produce operator-facing output and error detail have almost no unit coverage. `internal/app` — the spec-017 composition root — is tested only for the spec-026 `specsDir` failure; the happy-path assembly of each bundle, the subset shapes (teardown never builds the vault; apply never builds the gateway/routing/container backend), the seven slot-prefixed failure wraps (`validating registries:`, `observer:`, `container backend:`, `traefik:`, `vault:`, `routing:`, `engine:`), the unwind-on-failure (close the log writer), and the cleanup func are pinned by nothing. `internal/ui/file_logger.go` has no test. `internal/ui/terminal_logger.go` renders 33 event kinds and tests one. No test has ever exercised a handler through a bundle of stand-ins (017 US3 / FR-006).

The approach is two minimal, behaviour-preserving seams plus table-driven tests. Seam 1: `FileLogger` writes to an `io.WriteCloser` behind an unexported `newFileLogger(w)` constructor; `NewFileLogger(stateDir)` keeps opening `<state>/logs/shrine.log` in append mode and delegates to it. Seam 2: the five collaborator constructors in `internal/app/components.go` become package-level function variables (`newFileLogger`, `newVault`, `newContainerBackend`, `newTraefikPlugin`, `newLocalEngine`) that tests in `package app` swap and restore with `t.Cleanup`; production code never reassigns them, so the single-construction-site guarantee of 017 FR-002 is untouched. Tests: bundle assembly per command with an in-memory fake file logger and pinned env (`HOME`, `DOCKER_HOST`, `DOCKER_CERT_PATH`) — every other collaborator is the real constructor, which is already hermetic; a failure-injection table over all seven slots asserting prefix, `errors.Is` cause, `(nil, nil, err)`, and "log writer closed iff it was opened"; subset-shape tests; `handler.Teardown` driven by an `app.TeardownBundle` built from the existing in-memory store fakes plus a recording container backend and observer; an exact-line catalogue for every rendered terminal kind, the generic error line, spinner lifecycle through a mutex-guarded buffer with the spinner inspected via its unexported field, and pinned silence for `routing.finalize`; file-logger format (timestamp parsed, remainder compared byte-exact), every-status, concurrency, and Close-forwarding tests; and one Docker integration scenario (`tests/integration/file_logger_test.go`) pinning log creation and append across a deploy → teardown pair. No user-visible behaviour changes; `cmd/`, the engine, and the plugins are untouched.

## Technical Context

**Language/Version**: Go 1.25 (`go.mod`: `go 1.25.0`); one generic test helper (`swapConstructor[T]`)
**Primary Dependencies**: stdlib only (`testing`, `bytes`, `errors`, `strings`, `sync`, `time`); existing `engine.Event`/`engine.Observer`; Docker SDK client construction via `client.FromEnv` + `WithAPIVersionNegotiation` (reads env only, negotiates on first request — no daemon contact at construction); no new dependencies
**Storage**: N/A — no schema, state, or log-format change
**Testing**: `go test ./...` (CI); locally `go test -race ./internal/app/ ./internal/ui/ ./internal/handler/` for FR-001's data-race requirement (CI runs without `-race` today; unchanged); unit tests hermetic — no filesystem, network, Docker daemon, or credentials, `t.Setenv` only; integration via `tests/integration/file_logger_test.go` on `NewDockerSuite`, compile-checked locally with `go vet -tags integration ./tests/integration/...` and executed by CI (`make test-integration`)
**Target Platform**: Linux (Docker host); unit tests run anywhere Go runs
**Project Type**: Single Go CLI
**Performance Goals**: Tests added by this feature < 5 s wall-clock (SC-005). Each spinner stop waits at most one 100 ms animation frame; ≈ 15 lifecycle stops ≈ 1.5 s worst case; everything else is sub-millisecond
**Constraints**: FR-001 hermetic; FR-014 no behaviour change — the only production edits are the two seams and one doc-comment correction; 017 FR-002 single construction site preserved; the typed-nil "absent vault" representation is preserved (assert `!Vault.IsActive()`, never `Vault == nil`); tests in `package app` that swap constructor variables must not use `t.Parallel()`
**Scale/Scope**: 2 production files (`internal/ui/file_logger.go` ≈ 8 lines changed; `internal/app/components.go` ≈ 20 lines changed) + 1 doc comment (`internal/app/app.go`); unit tests: 2 new + 1 extended in `internal/app`, 2 new + 1 extended in `internal/ui`, 1 new + 1 extended in `internal/handler`; 1 new integration file; 2 spec bookkeeping files; no docs (nothing user-visible changes)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | Pass — no new capability, flag, or manifest field; tests plus two internal seams |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | Pass — no new commands; `cmd/` untouched; the constructor variables are consumed by the same `Build*Bundle` entry points `cmd/` already calls |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | Pass — no engine or backend logic; the handler test's stand-ins implement the existing `engine.ContainerBackend` and `engine.Observer` interfaces; no new backend interface |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | Pass with one documented item — constructor **variables** were chosen over an options struct, a constructor-provider interface, or functional options precisely because they add zero types and zero call-site changes (research D2). The one new type, the two-method `closableObserver` interface, has fewer than three implementations and is recorded in Complexity Tracking. No clock injection (the timestamp is parsed, not faked — D10); no spinner-interval seam (the wait is bounded — D8); `swapConstructor[T]` is one helper used by every failure and subset test |
| V. Integration-Test Gate | Does this phase map to an integration test scenario using the real binary? | Pass — `tests/integration/file_logger_test.go` on `NewDockerSuite` pins the on-disk behaviours (FR-013) against the real binary; unit tests supplement, never substitute. TDD order: every test is written red first; each seam is introduced only when the first red test needs it (research D1, D2) |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | N/A — no state or Docker operation is added or reordered |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | Pass — shared helpers (`swapConstructor`, `useInMemoryFileLogger`, `pinHermeticEnv`, `assertSlotFailure`, `safeBuffer`, `stopSpinner`, `assertLogLine`) replace repetition; tests are table-driven; the only new comments are one-line WHYs (why the writer is injectable, why the variables exist, why the test buffer is mutex-guarded) |

**Post-Phase-1 re-check**: all gates unchanged — Pass. One Complexity Tracking entry (`closableObserver`).

## Project Structure

### Documentation (this feature)

```text
specs/027-app-ui-unit-coverage/
├── plan.md                    # This file
├── research.md                # Phase 0 — current-state findings + 12 decisions (seams, env pinning, failure matrix, spinner, log format, integration scenario)
├── data-model.md              # Phase 1 — bundle slots, construction sequences, failure matrix, constructor variables, rendered-line catalogue, log-line grammar, stand-ins
├── quickstart.md              # Phase 1 — how to run each suite hermetically, with -race, plus mutation spot-checks for SC-007
├── contracts/
│   ├── test-seams.md          # Phase 1 — the two production seams: FileLogger writer injection; app constructor variables + closableObserver
│   └── coverage-matrix.md     # Phase 1 — every test to write, its assertions, and the FR/SC it closes
└── tasks.md                   # Phase 2 (/speckit-tasks — NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/ui/
├── file_logger.go                  # MODIFIED: FileLogger{out io.WriteCloser}; NewFileLogger(stateDir) opens the file then
│                                   #           returns newFileLogger(f); newFileLogger(w io.WriteCloser) *FileLogger (unexported);
│                                   #           OnEvent/Close/formatFields unchanged in behaviour
├── file_logger_test.go             # NEW (package ui): line format (timestamp parsed, remainder byte-exact), sorted quoted fields,
│                                   #      field-less line, every status → one line, concurrent OnEvent → whole lines, Close forwards
├── terminal_logger.go              # UNCHANGED
├── terminal_logger_test.go         # EXTENDED: safeBuffer (mutex-guarded writer); rendered-kind catalogue table (33 kinds);
│                                   #           routing.configure with/without aliases; generic error line with and without a
│                                   #           dedicated rendering; existing container.create tests kept verbatim
└── terminal_logger_steps_test.go   # NEW (package ui): spinner lifecycle per step kind (start → spinner set with message; finished →
                                    #      spinner nil, clear sequence, completion line once; error → spinner nil, no completion line;
                                    #      finished without start; container.remove not-found; volume.create/volume.created;
                                    #      warning leaves spinner untouched) + silence cases (routing.finalize, ungated statuses)

internal/app/
├── app.go                          # MODIFIED (comment only): BuildApplyBundle doc — "idempotent" → "safe to call more than once;
│                                   #           a repeated call reports the already-closed writer" (research D6)
├── components.go                   # MODIFIED: type closableObserver interface{ engine.Observer; Close() error };
│                                   #           var newFileLogger, newVault, newContainerBackend, newTraefikPlugin, newLocalEngine
│                                   #           (same signatures as today's funcs); newObserverPair uses newFileLogger
├── app_test.go                     # EXTENDED: keeps the spec-026 specsDir tests; adds happy-path table (deploy/apply/teardown slots),
│                                   #           subset-shape tests, failure-injection table (7 slot prefixes × affected bundles),
│                                   #           cleanup tests (closes writer, forwards error, second call does not panic)
├── components_test.go              # NEW (package app): joinCleanup (nil closers skipped, errors joined, all closers run),
│                                   #      routingFromPlugin (inactive → nil,nil; active unresolvable routing-dir → error)
└── testdoubles_test.go             # NEW (package app): fakeFileLogger, failing/recording constructor stand-ins,
                                    #      swapConstructor[T], useInMemoryFileLogger, pinHermeticEnv, minimalConfig helpers

internal/handler/
├── teardown.go                     # UNCHANGED
├── deployments_test.go             # EXTENDED: memDeploymentStore gains a listErr field so List can be made to fail
└── teardown_test.go                # NEW (package handler): recordingContainerBackend, recordingObserver; Teardown over a hand-built
                                    #      app.TeardownBundle — removal order (apps then resources, name-sorted) then network removal,
                                    #      events observed, List failure → error + no ops, RemoveContainer failure → cause preserved,
                                    #      no further ops

tests/integration/
└── file_logger_test.go             # NEW: NewDockerSuite(testTeam); apply teams + deploy basic → <state>/logs/shrine.log exists and
                                    #      contains "[started] application.deploy"; teardown → the deploy entry is still present and
                                    #      "[started] application.teardown" follows (append across runs)

specs/progress.md                   # MODIFIED: entry for this feature (issue #40)
specs/features/integration-tests.md # MODIFIED: file-logger scenario listed
```

**Structure Decision**: Single-project Go CLI. Production edits are confined to the two packages whose behaviour is being pinned (`internal/ui`, `internal/app`) and consist only of seams; every test lives beside the code it covers (internal `package` tests so they can reach unexported constructors and the spinner field), the handler test lives in `internal/handler` where the in-memory store fakes already are, and the one filesystem-dependent behaviour is pinned in `tests/integration/` per Constitution V. `cmd/`, `internal/engine`, and the plugins are not touched.

## Design Outline

1. **File-logger seam** (`internal/ui/file_logger.go`, research D1): change the field to `out io.WriteCloser`; add `func newFileLogger(out io.WriteCloser) *FileLogger`; `NewFileLogger(stateDir)` keeps `MkdirAll` + `OpenFile(O_APPEND|O_CREATE|O_WRONLY)` and returns `newFileLogger(f), nil`. `OnEvent` writes to `l.out`; `Close` closes `l.out`. One WHY comment: the writer is injectable so the line format can be tested without a file.
2. **File-logger tests** (`file_logger_test.go`, D10): a `nopWriteCloser` around `bytes.Buffer`; `assertLogLine(t, line, wantRest)` parses the first 20 bytes with `time.Parse(time.RFC3339, …)` (asserting the trailing `Z`) and compares the remainder byte-exact — `[started] container.create name="alias-app" team="shrine-deploy-test"` — plus the field-less line, values containing spaces/quotes (`%q` quoting), every status producing exactly one line, 50 goroutines × 20 events producing 1000 whole lines, and `Close` forwarding to a counting closer.
3. **Composition-root seam** (`internal/app/components.go`, D2): declare `closableObserver`; turn the five constructors into variables with today's signatures; `newObserverPair` calls `newFileLogger(paths.StateDir)` and returns `fileLogger.Close`. `newVault` and `newTraefikPlugin` remain thin closures over the plugin constructors so the config-field selection stays in one place; `newContainerBackend` and `newLocalEngine` are direct references to `local.NewContainerBackend` / `local.NewLocalEngine`.
4. **App test doubles** (`testdoubles_test.go`, D3–D5): `fakeFileLogger{events, closes, closeErr}`; `swapConstructor[T](t, *T, T)` saving and restoring via `t.Cleanup`; `useInMemoryFileLogger(t) *fakeFileLogger`; `pinHermeticEnv(t)` setting `HOME=/home/test-user`, `DOCKER_HOST=""`, `DOCKER_CERT_PATH=""`; `minimalConfig()` returning `&config.Config{SpecsDir: "/abs/specs"}` (no plugins); `emptyStore()` → `&state.Store{}`; `fakePaths()` → `&config.Paths{StateDir: "/nonexistent/state"}`.
5. **App tests** (`app_test.go`, `components_test.go`, D4–D6): happy-path table asserting each required slot non-nil, `Routing == nil`, `!Vault.IsActive()`, `SpecsDir` for deploy/teardown, `cleanup != nil`, and the fake logger's recorded `stateDir`; subset tests swapping `newVault`/`newTraefikPlugin`/`newContainerBackend` with `fail(sentinel)` and asserting teardown/apply still succeed with the stand-in never called (plus the real invalid Traefik config for apply); failure table per data-model §3 asserting `strings.HasPrefix(err.Error(), "<slot>: ")`, `errors.Is(err, sentinel)` (or the real cause substring), bundle and cleanup nil, and `fake.closes == 1` iff the observer had been opened; cleanup tests (closes once, forwards `closeErr`, second call does not panic and calls the closer again); `joinCleanup` and `routingFromPlugin` direct tests.
6. **Terminal-observer tests** (`terminal_logger_test.go`, `terminal_logger_steps_test.go`, D8–D9): `safeBuffer` (mutex around `bytes.Buffer`, because production writes to `os.Stdout`, which is safe for concurrent writes, and the spinner goroutine writes while `OnEvent` writes the error line); catalogue table of the 33 kinds from data-model §5 with populated fields and the exact expected line; step lifecycle tests reading `obs.spinner` and `obs.spinner.msg` directly and registering `stopSpinner(t, obs)` in `t.Cleanup` so no goroutine outlives a test; assertions count occurrences of the completion line and the `\r\033[K` clear sequence rather than comparing the whole buffer; silence tests for `routing.finalize` (started/info → empty; error → only the generic line) and status-gated kinds.
7. **Handler test** (`internal/handler/teardown_test.go`, D7): `memDeploymentStore` gains `listErr`; `recordingContainerBackend{removed []engine.RemoveContainerOp, networks []string, removeErr error}`; `recordingObserver`; bundle = `&app.TeardownBundle{Out: &buf, Cfg: &config.Config{}, Store: &state.Store{Deployments: dep}, Engine: &engine.Engine{Container: rec, Observer: obs}}`; three scenarios per spec US3.
8. **Integration scenario** (`tests/integration/file_logger_test.go`, D11): mirrors `TestTeardown`'s `BeforeEach` (apply teams + deploy basic), then asserts the log path exists and contains `[started] application.deploy`, runs `teardown`, and asserts both entries are present — the deploy entry's survival is the append proof.
9. **Bookkeeping** (D12): `specs/progress.md` entry; `specs/features/integration-tests.md` scenario; `graphify update .` after the code change; no docs pages change (nothing user-visible changes; no page documents the log file today).

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| `closableObserver` interface in `internal/app/components.go` (2 implementations: `*ui.FileLogger`, the test fake — below Principle IV's ≥3 threshold) | The `newFileLogger` variable must return something a test can substitute without touching the filesystem; `*ui.FileLogger` can only be built over a real file from outside `package ui` | (a) Export a writer-injecting constructor from `ui` for `app`'s tests — exports test-only API; (b) return `*ui.FileLogger` and let `app` tests use a temp dir — violates the repo's no-filesystem rule; (c) type the variable as `engine.Observer` and close via a separate hook — splits one collaborator into two seams. The interface is two methods, unexported, and mirrors the pair of operations `newObserverPair` already performs on the logger |
