# Data Model: Unit Coverage for the Composition Root and Event Renderers

**Feature**: `027-app-ui-unit-coverage` | **Date**: 2026-08-28

No persisted schema changes. This document captures the in-memory shapes the tests pin: the three dependency sets and their construction sequences, the failure matrix, the constructor variables (the seam), the terminal rendering catalogue, the log-line grammar, and the stand-ins used by the handler test.

## 1. Dependency sets (`internal/app`)

| Slot | `DeployBundle` | `ApplyBundle` | `TeardownBundle` | Happy-path assertion (no gateway, no vault configured) |
|------|:--------------:|:-------------:|:----------------:|--------------------------------------------------------|
| `Out` | ✓ | ✓ | ✓ | `== out` |
| `ErrOut` | ✓ | ✓ | — | `== errOut` |
| `Cfg` | ✓ | ✓ | ✓ | `== cfg` |
| `Store` | ✓ | ✓ | ✓ | `== store` |
| `Paths` | ✓ | ✓ | ✓ | `== paths` |
| `SpecsDir` | ✓ | — | ✓ | deploy: `== cfg.ResolveSpecsDir(manifestDir)`; teardown: `== cfg.SpecsDir` resolved (or `""` when absent) |
| `Observer` | ✓ | ✓ | ✓ | non-nil `engine.MultiObserver` of length 2 (terminal + file logger); the fake logger's recorded `stateDir == paths.StateDir` |
| `Vault` | ✓ | ✓ | — | **typed-nil** `*infisical.InfisicalPlugin` inside the interface → assert `!Vault.IsActive()` (never `== nil`) |
| `ContainerBackend` | ✓ | — | — | non-nil |
| `Routing` | ✓ | — | ✓ | `== nil` (true nil — inactive Traefik plugin) |
| `Engine` | ✓ | ✓ | ✓ | non-nil; `Engine.Observer == Observer`; `Engine.Routing == Routing` (deploy/teardown) |
| cleanup | ✓ | ✓ | ✓ | non-nil; calling it closes the fake logger exactly once |

## 2. Construction sequences (order matters for "log writer closed iff opened")

| Step | `BuildApplyBundle` | `BuildDeployBundle` | `BuildTeardownBundle` |
|-----:|--------------------|---------------------|-----------------------|
| 1 | `cfg.ValidateRegistries()` → `validating registries:` | `cfg.ValidateRegistries()` → `validating registries:` | `resolveOptionalSpecsDir(cfg)` → unwrapped `resolving specsDir: …` |
| 2 | `newObserverPair` → `observer:` | `cfg.ResolveSpecsDir(manifestDir)` → unwrapped | `newObserverPair` → `observer:` |
| 3 | `newVault` → `vault:` | `newObserverPair` → `observer:` | `newTraefikPlugin(cfg, nil, specsDir, observer)` → `traefik:` |
| 4 | `newLocalEngine` → `engine:` | `newContainerBackend` → `container backend:` | `routingFromPlugin` → `routing:` |
| 5 | return bundle + `joinCleanup(closeObserver)` | `newTraefikPlugin(cfg, containerBackend, …)` → `traefik:` | `newLocalEngine` → `engine:` |
| 6 | | `newVault` → `vault:` | return bundle + cleanup |
| 7 | | `routingFromPlugin` → `routing:` | |
| 8 | | `newLocalEngine` → `engine:` | |
| 9 | | return bundle + cleanup | |

The observer pair is opened at step 2 (apply, teardown) or step 3 (deploy). Every later failure calls `closeObserver()` before returning `(nil, nil, err)`; every earlier failure returns without having opened it.

## 3. Failure matrix (one table drives the tests)

| Slot prefix | Bundles | Trigger in tests | Cause assertion | Log writer |
|-------------|---------|------------------|-----------------|------------|
| `validating registries: ` | apply, deploy | real: `Registries: []config.RegistryConfig{{Alias: "bad alias!"}}` | message contains `registries: alias "bad alias!" contains invalid characters` | never opened |
| *(unwrapped)* `resolving specsDir: ` | deploy, teardown | real: `SpecsDir: "~/manifests"`, `HOME=""` — **existing tests, unchanged** | contains `expanding ~` | never opened |
| `observer: ` | apply, deploy, teardown | injected: `newFileLogger` returns `(nil, sentinel)` | `errors.Is(err, sentinel)`; message contains `initializing file logger:` | never opened (the failure *is* the open) |
| `container backend: ` | deploy | injected: `newContainerBackend` returns `(nil, sentinel)` | `errors.Is` | closed (`closes == 1`) |
| `traefik: ` | deploy, teardown | real: `Traefik: &config.TraefikPluginConfig{Dashboard: &config.TraefikDashboardConfig{Port: 8080}}` | contains `dashboard.port is set but username and password are required` | closed |
| `vault: ` | apply, deploy | injected: `newVault` returns `(nil, sentinel)` | `errors.Is` | closed |
| `routing: ` | deploy, teardown | real: `Traefik: &config.TraefikPluginConfig{RoutingDir: "~/routes"}`, `HOME=""` (deploy passes an absolute `manifestDir` so `specsDir` resolves first; teardown uses an absolute `SpecsDir`) | contains `resolving routing-dir: expanding ~` | closed |
| `engine: ` | apply, deploy, teardown | injected: `newLocalEngine` returns `(nil, sentinel)` | `errors.Is` | closed |

Every row also asserts `bundle == nil && cleanup == nil` and `strings.HasPrefix(err.Error(), "<slot prefix>")`.

### Subset-shape rows (assembly must **succeed**)

| Bundle | Stand-in that must never be called | Also with real config |
|--------|------------------------------------|-----------------------|
| teardown | `newVault` swapped for a recorder that fails if invoked | — |
| apply | `newTraefikPlugin` and `newContainerBackend` swapped for recorders that fail if invoked | `Traefik: {Dashboard: {Port: 8080}}` (invalid) — apply still succeeds |

## 4. Constructor variables (the seam, `internal/app/components.go`)

| Variable | Signature (unchanged from today's func) | Production value | Test replacements |
|----------|------------------------------------------|------------------|-------------------|
| `newFileLogger` | `func(stateDir string) (closableObserver, error)` | closure over `ui.NewFileLogger` with explicit nil-on-error | `useInMemoryFileLogger(t)` → records `stateDir`, events, `closes`, returns `closeErr`; or `failing(sentinel)` |
| `newVault` | `func(cfg *config.Config) (secrets.SecretsPlugin, error)` | closure over `infisicalplugin.New(cfg.Plugins.Secrets.Infisical)` | `failing(sentinel)`; `mustNotBeCalled(t)` |
| `newContainerBackend` | `func(*state.Store, []config.RegistryConfig, engine.Observer) (engine.ContainerBackend, error)` | `local.NewContainerBackend` | `failing(sentinel)`; `mustNotBeCalled(t)` |
| `newTraefikPlugin` | `func(cfg *config.Config, container engine.ContainerBackend, specsDir string, observer engine.Observer) (*traefik.Plugin, error)` | closure over `traefik.New(cfg.Plugins.Gateway.Traefik, …)` | `mustNotBeCalled(t)` (failures use the real validator) |
| `newLocalEngine` | `func(local.EngineOptions) (*engine.Engine, error)` | `local.NewLocalEngine` | `failing(sentinel)` |

```go
type closableObserver interface {
	engine.Observer
	Close() error
}
```

Rules: variables are assigned only in their declaration and in `package app` tests through `swapConstructor(t, &v, replacement)`, which restores the original in `t.Cleanup`; tests that swap must not call `t.Parallel()`.

### Test doubles (`internal/app/testdoubles_test.go`)

| Double | Shape | Purpose |
|--------|-------|---------|
| `fakeFileLogger` | `{stateDir string; events []engine.Event; closes int; closeErr error}`; `OnEvent` appends; `Close` increments and returns `closeErr` | the only collaborator that must be faked in every bundle test |
| `swapConstructor[T any](t, target *T, replacement T)` | saves `*target`, assigns, restores in `t.Cleanup` | one helper for every swap |
| `useInMemoryFileLogger(t) *fakeFileLogger` | swaps `newFileLogger` | happy path, failure rows after the observer, cleanup tests |
| `pinHermeticEnv(t)` | `t.Setenv("HOME", "/home/test-user")`, `t.Setenv("DOCKER_HOST", "")`, `t.Setenv("DOCKER_CERT_PATH", "")` | deterministic `specsDir` resolution; Docker client built from defaults, no cert files read |
| `minimalConfig()` / `emptyStore()` / `fakePaths()` | `&config.Config{SpecsDir: "/abs/specs"}` / `&state.Store{}` / `&config.Paths{StateDir: "/nonexistent/state"}` | happy-path inputs; the store is non-nil because engine construction dereferences it |

## 5. Terminal rendering catalogue (`internal/ui/terminal_logger.go`)

Generic line, written **before** the switch for any kind: `StatusError` → `  ❌ Error [<name>]: <fields.error>\n`.

### Single-line kinds

| # | Kind | Status gate | Exact line (`\n`-terminated) | Fields used |
|--:|------|-------------|-------------------------------|-------------|
| 1 | `application.deploy` | `started` only | `🚀 Deploying Application: <name> (owner: <owner>)` | name, owner |
| 2 | `application.teardown` | `started` only | `🗑️  Tearing down Application: <name> (team: <team>)` | name, team |
| 3 | `resource.deploy` | `started` only | `📦 Deploying Resource: <name> (type: <type>)` | name, type |
| 4 | `resource.teardown` | `started` only | `🗑️  Tearing down Resource: <name> (team: <team>)` | name, team |
| 5 | `network.ensure` | any | `  🌐 Ensuring network: shrine.<owner>.private` | owner |
| 6 | `container.create` | `info` only | `  🏗️  Creating container: <team>.<name>` | team, name |
| 7 | `routing.configure` | any | `  🔗 Configuring routing: <domain> -> port <port>` then, iff `aliases != ""`, `    ↳ Aliases: <aliases>` | domain, port, aliases |
| 8 | `gateway.config.preserved` | any | `  📄 Preserving operator-owned traefik.yml: <path>` | path |
| 9 | `gateway.config.generated` | any | `  📝 Generated default traefik.yml: <path>` | path |
| 10 | `gateway.config.legacy_http_block` | any | `  ⚠️  Legacy http block in traefik.yml at <path> — <hint>` | path, hint |
| 11 | `gateway.config.tls_port_no_websecure` | any | `  ⚠️  tlsPort set but traefik.yml is missing websecure entrypoint at <path> — <hint>` | path, hint |
| 12 | `gateway.alias.tls_no_websecure` | any | `  ⚠️  alias tls: true but websecure entrypoint missing in <path> for <team>.<name> (<tls_aliases>) — <hint>` | path, team, name, tls_aliases, hint |
| 13 | `gateway.config.legacy_probe_error` | any | `  ⚠️  Could not probe traefik.yml for legacy http block (deploy continues): <path> (<error>)` | path, error |
| 14 | `gateway.config.tls_port_probe_error` | any | `  ⚠️  Could not probe traefik.yml for websecure entrypoint (deploy continues): <path> (<error>)` | path, error |
| 15 | `gateway.dashboard.generated` | any | `  📝 Generated dashboard dynamic file: <path>` | path |
| 16 | `gateway.dashboard.preserved` | any | `  📄 Preserving operator-owned dashboard dynamic file: <path>` | path |
| 17 | `gateway.dashboard.removed` | any | `  🗑️  Removed stale dashboard dynamic file: <path>` | path |
| 18 | `gateway.route.generated` | any | `  📝 Generated route file: <path>` | path |
| 19 | `gateway.route.preserved` | any | `  📄 Preserving operator-owned route file: <path>` | path |
| 20 | `gateway.route.stat_error` | any | `  ⚠️  Could not stat route file (deploy continues): <path> (<error>)` | path, error |
| 21 | `gateway.route.orphan` | any | `  ⚠️  Orphan route file left on disk; remove with: rm <path>` | path |
| 22 | `dns.register` | any | `  🌍 Registering DNS: <domain>` | domain |
| 23 | `container.start` | any | `    ▶️  Starting existing container: <name>` | name |
| 24 | `container.recreate` | any | `    🔄 Image changed for <name>, replacing container...` | name |
| 25 | `container.fresh` | any | `    ✨ Creating fresh container: <name>` | name |
| 26 | `container.created` | any | `    ✅ Container <name> is running` | name |
| 27 | `hostport.published` | any | `    📡 Published <team>/<name> on 127.0.0.1:<hostPort> -> <containerPort>/<proto>` | team, name, hostPort, containerPort, proto |
| 28 | `container.remove` (info + `reason=not found`) | `info` with reason | `    ℹ️  Container <name> not found, skipping removal` (no spinner) | name, reason |

"any" means the line is written for every status — an `error` event of such a kind writes the generic line *and* the kind line. Catalogue tests use the status the emitter uses (`info` unless stated).

### Step-style kinds (progress indicator via `handleStep`)

| # | Kind | Start status | Indicator message (`prefix + msg`) | Finished line | Error |
|--:|------|--------------|------------------------------------|---------------|-------|
| 29 | `network.create` | `started` | `    🔨 Creating Docker network: <name>` | `    ✅ Network created: <name> (<cidr>)` | stop, no line |
| 30 | `network.remove` | `started` | `  🌐 Removing network: <name>` | `  ✅ Network removed: <name>` | stop, no line |
| 31 | `container.remove` (not "not found") | `started` | `    🗑️  Removing container: <name>` | `    ✅ Container <name> removed` | stop, no line |
| 32 | `volume.create` | **`info`** | `    📦 Creating volume: <name>` | *(nothing — `finishFmt` is empty)* | stop, no line |
| 33 | `volume.created` | *(never starts)* | — | `    ✅ Volume <name> is created` (also stops a running `volume.create` indicator) | — |
| 34 | `image.pull` | `started` | `    📥 Pulling image <name>...` — field `ref` | `    ✅ Pulled image <ref>` | stop, no line |

(33 distinct `case` arms: rows 1–27 plus `container.remove` once plus rows 29, 30, 32, 33, 34.)

### Indicator lifecycle (observable state)

| Event | `obs.spinner` after | Output added |
|-------|---------------------|--------------|
| start status of a step kind | non-nil, `msg == prefix+message` | 0..n animation frames (nondeterministic — not asserted) |
| `finished` | nil | `\r\033[K` (clear) exactly once, then the finished line exactly once (if any) |
| `error` | nil | generic error line (written first), `\r\033[K`; **no** finished line |
| `warning` / other | unchanged | nothing |
| `finished` with no running indicator | nil | finished line only; no clear sequence; no blocking |

### Silence

| Event | Output |
|-------|--------|
| `routing.finalize` `started` / `info` | none |
| `routing.finalize` `error` | generic error line only |
| status-gated kind at another status (e.g. `application.deploy` `finished`, `container.create` `started`) | none |

## 6. Log line grammar (`internal/ui/file_logger.go`)

```
line      := timestamp " [" status "] " name fields "\n"
timestamp := RFC3339 in UTC, exactly 20 bytes, ends in "Z"        e.g. 2026-08-28T10:15:30Z
status    := started | finished | info | warning | error
name      := event name verbatim
fields    := "" | (" " key "=" quoted)+     keys in ascending byte order; quoted = Go %q (spaces, quotes, newlines escaped)
```

Examples:

| Event | Line after the timestamp |
|-------|--------------------------|
| `{container.create, started, {team: "shrine-deploy-test", name: "alias-app"}}` | ` [started] container.create name="alias-app" team="shrine-deploy-test"` |
| `{routing.finalize, info, nil}` | ` [info] routing.finalize` |
| `{container.create, error, {error: `creating "x": bad "ref"`}}` | ` [error] container.create error="creating \"x\": bad \"ref\""` |

Rules pinned: exactly one line per `OnEvent` for every status (no filtering); lines from concurrent callers never interleave (mutex); `Close` closes the destination once.

### Seam

```go
type FileLogger struct {
	out io.WriteCloser
	mu  sync.Mutex
}
func NewFileLogger(stateDir string) (*FileLogger, error) // unchanged: MkdirAll(<stateDir>/logs), OpenFile(shrine.log, O_APPEND|O_CREATE|O_WRONLY, 0644), then newFileLogger(f)
func newFileLogger(out io.WriteCloser) *FileLogger       // new, unexported
```

## 7. Handler-test stand-ins (`internal/handler/teardown_test.go`)

| Stand-in | Implements | Records / controls |
|----------|-----------|--------------------|
| `memDeploymentStore` (existing, +`listErr`) | `state.DeploymentStore` | `byTeam`; `List` returns `listErr` when set |
| `recordingContainerBackend` | `engine.ContainerBackend` | `removed []engine.RemoveContainerOp` (in call order), `networks []string`; `removeErr` returned by `RemoveContainer`; other methods return zero values |
| `recordingObserver` | `engine.Observer` | `events []engine.Event` |

Bundle under test: `&app.TeardownBundle{Out: &bytes.Buffer{}, Cfg: &config.Config{}, Store: &state.Store{Deployments: dep}, Engine: &engine.Engine{Container: rec, Observer: obs}}` — `Routing` nil (route removal and finalize skipped), `Resolver` nil (unused by teardown), no constructor from `app`, `local`, or any plugin invoked.

Expected flow for deployments `[{Resource, "db"}, {Application, "web"}]` and team `t`:

| Step | Backend call | Observer event |
|------|--------------|----------------|
| 1 | `RemoveContainer({Team: t, Name: "web"})` | `application.teardown` started `{team: t, name: web}` |
| 2 | `RemoveContainer({Team: t, Name: "db"})` | `resource.teardown` started `{team: t, name: db}` |
| 3 | `RemoveNetwork(t)` | — |

> **Implementation correction**: the engine builds these names from the manifest kind as recorded (`manifest.ApplicationKind` = `"Application"`), so the events actually observed are `Application.teardown`, `Resource.teardown`, and `Application.remove` — capitalised. The handler test pins the observed names; see tasks.md Implementation Notes. Resolved by spec 028 (issue #46): the engine now emits the lowercase names shown in the table above and the handler test asserts them.

Failure rows: `listErr` → returned unchanged, no backend calls; `removeErr` on the first removal → returned wrapped (`errors.Is` true), `len(removed) == 1`, `networks` empty, an `application.remove` error event observed.
