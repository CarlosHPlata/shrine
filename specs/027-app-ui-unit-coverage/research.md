# Research: Unit Coverage for the Composition Root and Event Renderers

**Feature**: `027-app-ui-unit-coverage` | **Date**: 2026-08-28
**Purpose**: Resolve every design choice behind the plan so implementation has no open questions. The Technical Context in `plan.md` contains no `NEEDS CLARIFICATION` markers; the decisions below record the alternatives weighed for each judgment call.

## Current-state findings (what the code does today)

| Area | Location | Finding |
|------|----------|---------|
| Composition root | `internal/app/app.go` | `BuildApplyBundle`, `BuildDeployBundle`, `BuildTeardownBundle` each return `(*Bundle, cleanup, error)`. Every failure after the observer is opened calls `_ = closeObserver()` and wraps with a slot prefix: `validating registries:`, `observer:`, `container backend:`, `traefik:`, `vault:`, `routing:`, `engine:`. `specsDir` failures pass through unwrapped (spec 026). `ValidateTraefikConfig` and `NewQueryContainerBackend` are separate entry points, out of scope. |
| Collaborator constructors | `internal/app/components.go` | Five unexported funcs: `newObserverPair` (→ `ui.NewTerminalObserver` + `ui.NewFileLogger(paths.StateDir)`), `newVault` (→ `infisicalplugin.New(cfg.Plugins.Secrets.Infisical)`), `newContainerBackend` (→ `local.NewContainerBackend`), `newTraefikPlugin` (→ `traefik.New(cfg.Plugins.Gateway.Traefik, …)`), `newLocalEngine` (→ `local.NewLocalEngine`). Plus `routingFromPlugin` and `joinCleanup`. |
| Existing app tests | `internal/app/app_test.go` | Three tests (spec 026): deploy/teardown bundles fail before construction on an unresolvable `specsDir`; `resolveOptionalSpecsDir` table. Nothing else. |
| File logger | `internal/ui/file_logger.go` | `FileLogger{file *os.File; mu}`; `NewFileLogger(stateDir)` does `MkdirAll(<state>/logs)` + `OpenFile(shrine.log, O_APPEND\|O_CREATE\|O_WRONLY, 0644)`. `OnEvent` writes `"%s [%s] %s%s\n"` = RFC3339-UTC timestamp, status, name, `formatFields` (` k=%q` pairs, keys sorted). `Close` closes the file. Writes straight to the `*os.File` — untestable in memory today. |
| Terminal observer | `internal/ui/terminal_logger.go` | `OnEvent` prints `  ❌ Error [%s]: %s` for any `StatusError` event **before** the switch; the switch has **33** `case` arms (data-model §5). Six are step-style through `handleStep` (network.create, network.remove, container.remove, volume.create, volume.created, image.pull): start status → `newSpinner(out, prefix+msg).start()` (goroutine writes `\r    <frame> <msg>` every 100 ms), finished → `spinner.stop()` (closes `stopCh`, waits `doneCh`; the goroutine prints `\r\033[K` and exits) then the completion line, error → stop only. `spinner` and `msg` are unexported fields reachable from `package ui` tests. `routing.finalize` has **no** case. |
| Existing ui tests | `internal/ui/terminal_logger_test.go` | One kind (`container.create`): failed-creation sequence and the success line. |
| Handlers consuming bundles | `internal/handler/{deploy,apply,teardown}.go` | `Deploy` → `planner.LoadDir(manifestDir)` (reads disk); `ApplySingle` → `manifest.Parse(file)` (reads disk); `Teardown(b, team)` → `planner.PlanTeardown(team, b.Store.Deployments)` → `b.Engine.ExecuteTeardown(team, steps)` — **no disk**. |
| Teardown execution | `internal/engine/engine.go:88-122,241-265` | For each planned step: emit `<kind>.teardown` started, `Container.RemoveContainer({Team, Name})` (error → `<kind>.remove` error event, wrapped `"%s %q: %w"`), then `Routing.RemoveRoute` for applications if `Routing != nil`; after all steps `finalizeRouting` (no-op when `Routing == nil`) then `Container.RemoveNetwork(team)`. `PlanTeardown` orders applications (name-sorted) before resources (name-sorted). `Observer` nil → `NoopObserver`. |
| Existing handler fakes | `internal/handler/deployments_test.go` | `memTeamStore`, `memDeploymentStore{byTeam}` (List never fails), `memHostPortStore`, `memSubnetStore`, `stubContainerBackend` (records nothing), `deleteTestStore(...)`. `status_test.go` still uses `os.MkdirTemp` (pre-dates the no-filesystem rule; untouched here). |
| Store / engine shapes | `internal/state/state.go`, `internal/engine/engine.go:15` | `state.Store` is a struct of five **interfaces**; `engine.Engine` is a struct of interface fields (`Container`, `Routing`, `DNS`, `Resolver`, `Observer`) — both can be built by hand from stand-ins. |
| Engine construction | `internal/engine/local/engine.go:21-36`, `dockercontainer/docker_backend.go:19-31` | `NewLocalEngine` → `resolver.NewLiveResolver(opts.Store.Secrets, opts.Vault)` (stores only; dereferences `opts.Store`, so the store must be non-nil) and `dockercontainer.NewDockerBackend` → `client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())`. `FromEnv` reads `DOCKER_HOST` (empty → default socket, nothing dialled), `DOCKER_API_VERSION`, and `DOCKER_CERT_PATH` (non-empty → **reads cert files from disk**). Version negotiation is performed on the first request, not at construction. Hermetic when `DOCKER_CERT_PATH` is empty. |
| Vault construction | `internal/plugins/secrets/infisical/plugin.go:128-150` | `New(nil)` → `(nil, nil)`; non-nil config → SDK client + `UniversalAuthLogin` (**network**). `newVault` returns the concrete result as `secrets.SecretsPlugin`, so an absent vault is a **typed-nil** interface value; `IsActive()` is nil-receiver-safe and the resolver checks `r.Vault == nil \|\| !r.Vault.IsActive()`. |
| Traefik construction | `internal/plugins/gateway/traefik/plugin.go:31-89,119-139` | `New` only runs `validate()` — pure: inactive (nil cfg) passes; dashboard without credentials, out-of-range or colliding `tlsPort` fail with `traefik plugin: …`. `RoutingBackend()` → `resolvedRoutingDir()` → `cfg.ResolveRoutingDir(<specsDir>/traefik)` — pure; a `~`-prefixed `routing-dir` with `HOME=""` fails with `traefik plugin: resolving routing-dir: expanding ~: $HOME is not defined`. No disk I/O at construction. |
| Registry validation | `internal/config/config.go:88-103` | Pure; alias with invalid characters → `registries: alias %q contains invalid characters …`. |
| Integration harness | `tests/integration/testutils/*`, `teardown_test.go`, `deploy_test.go` | `NewDockerSuite(t, team)`, `tc.TempDir()`, `tc.StateDir`, `tc.Run(...)`, `AssertFileExists`, `AssertFileContains(path, want)`, `SeedSubnetState`, `fixturesPath("team"\|"basic")`, `const testTeam = "shrine-deploy-test"`. `TestTeardown` already runs apply teams → deploy basic → teardown against one state dir. `config_paths_test.go` asserts `<state>/logs` is **absent** on failure — nothing asserts its content on success. |
| CI | `.github/workflows/ci.yml` | `go build ./...`, `go test ./...`, `make test-integration`. No `-race`, no linter. |
| Docs | `docs/content`, `AGENTS.md` | No page mentions `shrine.log` or the observer output format — nothing to keep in sync. |
| Double close | Go stdlib `os.(*File).Close` | A second `Close` returns `*PathError{Op: "close", Err: os.ErrClosed}`; it does not panic. So `joinCleanup(fileLogger.Close)` is safe to call twice but its second result is an error — the "idempotent" wording on `BuildApplyBundle` overstates it. |

## Decision 1 — File logger: inject the writer behind an unexported constructor

**Decision**: `FileLogger` holds `out io.WriteCloser`. Add `func newFileLogger(out io.WriteCloser) *FileLogger`. `NewFileLogger(stateDir)` keeps its signature and file semantics (`MkdirAll`, append-mode open) and ends with `return newFileLogger(f), nil`. `OnEvent` and `Close` operate on `l.out`.

**Rationale**: The format, the every-status behaviour, and the mutex are properties of `OnEvent`, not of the file; the file is incidental. An `io.WriteCloser` is the narrowest type that keeps `Close` forwarding intact. The constructor is unexported: only `package ui` tests need it, and nothing outside gains API surface. Behaviour is byte-identical — the same `Fprintf` runs against the same opened file.

**Alternatives considered**:
- *Test through a temp file* — forbidden by the repo's no-filesystem rule for unit tests.
- *Extract `formatLogLine(now, e) string` and test only that* — leaves `OnEvent` (mutex, newline, "every status") untested and adds a function with one caller; the writer seam covers both format and behaviour.
- *Inject a clock too* — unnecessary: the test parses the 20-byte RFC3339 prefix and compares the remainder byte-exact (Decision 10).

## Decision 2 — Composition root: package-level constructor variables

**Decision**: In `components.go`, replace the five constructor funcs with variables of the same signatures:

```go
type closableObserver interface {
	engine.Observer
	Close() error
}

var (
	newFileLogger = func(stateDir string) (closableObserver, error) {
		logger, err := ui.NewFileLogger(stateDir)
		if err != nil {
			return nil, err
		}
		return logger, nil
	}
	newVault = func(cfg *config.Config) (secrets.SecretsPlugin, error) {
		return infisicalplugin.New(cfg.Plugins.Secrets.Infisical)
	}
	newContainerBackend = local.NewContainerBackend
	newTraefikPlugin = func(cfg *config.Config, container engine.ContainerBackend, specsDir string, observer engine.Observer) (*traefik.Plugin, error) {
		return traefik.New(cfg.Plugins.Gateway.Traefik, container, specsDir, observer)
	}
	newLocalEngine = local.NewLocalEngine
)
```

`newObserverPair` calls `newFileLogger(paths.StateDir)`. Tests swap a variable with `swapConstructor(t, &newVault, failing(sentinel))`, which restores the original in `t.Cleanup`. Production code never assigns to them.

**Rationale**: This is the idiomatic Go seam for same-package tests and the smallest possible diff: no new parameters on `Build*Bundle` (so `cmd/` is untouched — Constitution II), no options struct or provider interface (Constitution IV), and the five construction sites stay exactly where 017 FR-002 put them. `newFileLogger` must return an interface because `*ui.FileLogger` cannot be built in memory from outside `package ui` (Decision 1 keeps that constructor unexported); `closableObserver` is the two-method shape `newObserverPair` already relies on (observe + close). The explicit nil check in the `newFileLogger` closure avoids the typed-nil-interface trap on the error path.

**Alternatives considered**:
- *An `Options`/`Providers` struct parameter on each `Build*Bundle`* — changes three public signatures and four `cmd/` call sites, adds a type, and invites production callers to vary construction — the drift 017 removed.
- *Functional options* — same API growth for a test-only need.
- *Drive real constructors to failure via the environment* (`DOCKER_HOST=foo` → `unable to parse docker host`) — works for the first Docker touch only (`container backend:` in deploy, `engine:` in apply/teardown), cannot reach `engine:` in deploy or `vault:` anywhere without network, and couples tests to Docker SDK error text. Used for nothing; real constructors are kept only where they are hermetic and pure (Decision 4).
- *Temp state dir so the real file logger can open* — violates the no-filesystem rule.
- *Keep `newFileLogger` returning `*ui.FileLogger` and export a writer-injecting constructor from `ui`* — exports test-only API (see Complexity Tracking).

## Decision 3 — Happy path uses real constructors except the file logger, under a pinned environment

**Decision**: `useInMemoryFileLogger(t)` swaps only `newFileLogger`. `pinHermeticEnv(t)` sets `HOME=/home/test-user`, `DOCKER_HOST=""`, `DOCKER_CERT_PATH=""`. Inputs: `minimalConfig()` = `&config.Config{SpecsDir: "/abs/specs"}` (no registries, no gateway, no vault); `emptyStore()` = `&state.Store{}` (non-nil because `NewLocalEngine` dereferences it; its interface fields may be nil — nothing calls them at construction); `fakePaths()` = `&config.Paths{StateDir: "/nonexistent/state"}` (the fake logger records it and never touches it); `io.Discard` writers (or a `bytes.Buffer` to assert nothing is printed during assembly).

**Rationale**: The value of a happy-path test is that the *real* wiring composes: `traefik.New(nil, …)` (inactive → `Routing == nil`), `infisicalplugin.New(nil)` (typed-nil vault, `!IsActive()`), `local.NewContainerBackend` and `local.NewLocalEngine` (Docker client from env — no daemon contact; with `DOCKER_CERT_PATH` empty, no disk). Only the file logger is inherently disk-bound, so only it is faked. Pinning `HOME` keeps the `specsDir` resolution deterministic; pinning `DOCKER_HOST`/`DOCKER_CERT_PATH` removes the only environment-dependent branches in `client.FromEnv`.

**Alternatives considered**: fake every constructor in the happy path — proves the wiring of fakes, not the wiring of Shrine; kept only for the failure and subset tests where a fake is the point.

## Decision 4 — Failure matrix: real trigger where pure, injected sentinel otherwise

**Decision**: One table drives all slot-failure tests (data-model §3). Real triggers: `validating registries:` via `Registries: []config.RegistryConfig{{Alias: "bad alias!"}}`; `traefik:` via `Traefik: &config.TraefikPluginConfig{Dashboard: &config.TraefikDashboardConfig{Port: 8080}}` (no credentials); `routing:` via `Traefik: &config.TraefikPluginConfig{RoutingDir: "~/routes"}` with `HOME=""` (and, for deploy, an absolute `manifestDir` so `specsDir` resolves first). Injected via `swapConstructor` with `errors.New("boom")`: `observer:` (`newFileLogger`), `container backend:` (`newContainerBackend`), `vault:` (`newVault`), `engine:` (`newLocalEngine`). Every row asserts: `strings.HasPrefix(err.Error(), "<slot>: ")`; cause preserved — `errors.Is(err, sentinel)` for injected rows, substring of the real message otherwise; `bundle == nil && cleanup == nil`; and the fake logger's `closes == 1` when the failing slot comes after the observer in the sequence, `created == false` when it comes before (`validating registries:`, `specsDir`).

**Rationale**: FR-004 (a)–(d) in one place; real triggers keep the test honest about the actual plugin/config error text; sentinels isolate the wrap from network-bound constructors. The "closed iff opened" assertion is the regression guard for the `_ = closeObserver()` lines.

## Decision 5 — Assert "absent" the way the code represents it

**Decision**: Happy-path assertions use `bundle.Routing == nil` (true nil — `routingFromPlugin` returns a nil interface for an inactive plugin) but `!bundle.Vault.IsActive()` for the vault, never `bundle.Vault == nil`.

**Rationale**: `newVault` returns `infisicalplugin.New(nil)`'s `(*InfisicalPlugin)(nil)` as a non-nil `secrets.SecretsPlugin` interface holding a nil pointer; the resolver and `IsActive()` handle it. Changing `newVault` to return a true nil would be a behaviour change outside a coverage ticket (FR-014); the test pins the representation as it is and the data model documents it.

## Decision 6 — Cleanup: pin "safe to call twice", correct the comment, do not change behaviour

**Decision**: Cleanup tests assert: a single call closes the logger once and returns nil; a `closeErr` from the logger is returned (`errors.Is`); a second call does not panic and invokes the closer again (`closes == 2`). The `BuildApplyBundle` doc comment's "idempotent" is corrected to "safe to call more than once; a repeated call reports the already-closed writer". Behaviour is unchanged.

**Rationale**: Spec edge case — pin what is observed and surface the discrepancy. With the real file, the second `Close` returns `os.ErrClosed` wrapped in a `PathError`; that is harmless, so making cleanup truly idempotent (a `sync.Once`) is unnecessary work and a behaviour change. A `joinCleanup` unit test separately pins: nil closers skipped, all closers invoked, errors joined (`errors.Is` finds each).

## Decision 7 — Handler isolation test: `Teardown` with the existing in-memory fakes

**Decision**: New `internal/handler/teardown_test.go` (`package handler`). Reuse `memDeploymentStore` (add `listErr error`, returned by `List` when set). Add `recordingContainerBackend{removed []engine.RemoveContainerOp; networks []string; removeErr error}` (other methods return nil) and `recordingObserver{events []engine.Event}`. Bundle: `&app.TeardownBundle{Out: &buf, Cfg: &config.Config{}, Store: &state.Store{Deployments: dep}, Engine: &engine.Engine{Container: rec, Observer: obs}}`. Scenarios: (1) deployments `{Kind: Resource, Name: "db"}, {Kind: Application, Name: "web"}` → `removed == [{team, web}, {team, db}]` then `networks == [team]`, and `obs.events` contains `application.teardown`/`resource.teardown` started with the right fields; (2) `listErr` set → `errors.Is(err, listErr)`, `removed` and `networks` empty; (3) `removeErr` set → `errors.Is(err, removeErr)`, exactly one removal attempted, no network removal.

**Rationale**: Teardown is the only handler whose logic is disk-free (`Deploy`/`ApplySingle` read manifests), so it is the one that can prove 017 US3 hermetically. `engine.Engine` and `state.Store` are plain structs of interfaces, so a hand-built bundle needs no constructor; the test invokes no `app.Build*`, plugin, or `local` constructor — exactly FR-006. Extending `memDeploymentStore` with one field is DRY-er than a second fake.

## Decision 8 — Terminal observer: mutex-guarded buffer, inspect the spinner, bounded waits

**Decision**: Tests write into `safeBuffer` (`sync.Mutex` around `bytes.Buffer`, `Write`/`String` locked). Step-lifecycle tests assert `obs.spinner != nil && obs.spinner.msg == "<prefix><message>"` after the start event, `obs.spinner == nil` after finished/error, `strings.Count(out, "\r\033[K") == 1` and `strings.Count(out, "<completion line>") == 1` after finished, `== 0` completion lines after error (with the generic error line present). Every test that starts a spinner registers `stopSpinner(t, obs)` in `t.Cleanup`. No spinner-interval seam; no assertion on animation frames.

**Rationale**: Production writes to `os.Stdout` (an `*os.File`, safe for concurrent writes); `OnEvent` writes the `❌ Error` line *before* `handleStep` stops the spinner goroutine, so a plain `bytes.Buffer` would be a real data race under `-race` — the test double must offer the same guarantee the real writer does. Whether a frame was ever written is nondeterministic (the goroutine may observe `stopCh` before its first `default` branch), so tests inspect the unexported `spinner` field for "an indicator began" and count only deterministic output. `stop()` waits at most one 100 ms sleep, so ≈ 15 stops stay well inside SC-005's 5 s. The "second start while running" case (existing behaviour: the earlier goroutine is orphaned) is not pinned and not triggered.

**Alternatives considered**: make the frame interval a package variable set to 1 ms in tests — faster but a seam for a non-problem; assert on frame bytes — flaky by construction.

## Decision 9 — Catalogue every rendered kind from the switch, and pin `routing.finalize` as silent

**Decision**: The rendered-kind table (data-model §5) is derived from the 33 `case` arms of `TerminalObserver.OnEvent`; each row carries the status the emitter uses (`started` for the four lifecycle headers, `info` for `container.create` and the gateway/route/dashboard/host-port lines, `started`/`finished` for step kinds) and populated fields so every `%s` is exercised. `routing.finalize` (emitted by `engine.finalizeRouting` at started/info/error) is asserted to write nothing at started/info and only the generic error line at error.

**Rationale**: FR-007/FR-010 and SC-002 ("100% of rendered kinds"). The issue's "~15 branches" and its mention of `routing.finalize` are stale — there are 33 arms and no `routing.finalize` arm; adding a rendering would be a behaviour change, so its silence is what gets pinned.

## Decision 10 — File-logger tests: parse the timestamp, compare the rest

**Decision**: `assertLogLine(t, line, wantRest)`: `line` must be ≥ 21 bytes; `time.Parse(time.RFC3339, line[:20])` must succeed and `line[19] == 'Z'`; `line[20:]` must equal `wantRest`, e.g. ` [started] container.create name="alias-app" team="shrine-deploy-test"`. Cases: multi-field (sorted keys, `%q` quoting of a value with a space and a quote), field-less (`… [info] routing.finalize` with nothing after the name), every status (5 events → 5 lines), concurrency (50 goroutines × 20 events → 1000 lines each passing `assertLogLine`), `Close` forwards to the underlying closer.

**Rationale**: FR-011/FR-012 without a clock seam; the timestamp *format* is the contract, not its value. The concurrency case is the one that would fail if the mutex were dropped.

## Decision 11 — Integration: log accumulation across a deploy → teardown pair

**Decision**: New `tests/integration/file_logger_test.go` (`NewDockerSuite(t, testTeam)`), `BeforeEach` identical to `TestTeardown` (temp state dir, `SeedSubnetState`, `apply teams --path fixtures/team`, `deploy --path fixtures/basic`). One scenario: `log := filepath.Join(tc.StateDir, "logs", "shrine.log")`; `AssertFileExists(log)`; `AssertFileContains(log, "[started] application.deploy")`; `tc.Run("teardown", testTeam, …).AssertSuccess()`; `AssertFileContains(log, "[started] application.deploy")` again and `AssertFileContains(log, "[started] application.teardown")`. Listed in `specs/features/integration-tests.md`.

**Rationale**: FR-013 — the only way to prove `O_APPEND` across processes is two processes. Deploy and teardown are the two cheapest commands that both compose a bundle (so both open the log) and both emit events (so both write lines); the suite already pays for them in `TestTeardown`. Authored and compile-checked locally; executed by CI (Constitution V, repo policy on slow integration runs).

## Decision 12 — Bookkeeping and CI

**Decision**: Add a `specs/progress.md` entry (issue #40) and the integration scenario to `specs/features/integration-tests.md`; run `graphify update .` after the code change. No docs pages, `AGENTS.md`, or CLI strings change. CI stays as is; `-race` is run locally per `quickstart.md` (adding `-race` to CI is a reasonable follow-up but outside a coverage ticket).

**Rationale**: Nothing user-visible changes, so there is nothing to document; the spec's data-race requirement is met by the tests being race-free (verified locally), not by a CI change.
