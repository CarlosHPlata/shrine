# Contract: Production Seams

**Feature**: `027-app-ui-unit-coverage` | **Date**: 2026-08-28
**Scope**: The only production-code changes this feature makes. Both are behaviour-preserving; neither adds exported API. Internal packages; consumers are `cmd/` (unchanged call sites), `internal/handler` (unchanged), and the packages' own tests.

## 1. `internal/ui/file_logger.go` — injectable log destination

```go
// Before
type FileLogger struct {
	file *os.File
	mu   sync.Mutex
}
func NewFileLogger(stateDir string) (*FileLogger, error)

// After
type FileLogger struct {
	out io.WriteCloser
	mu  sync.Mutex
}
func NewFileLogger(stateDir string) (*FileLogger, error)   // signature and file semantics unchanged
func newFileLogger(out io.WriteCloser) *FileLogger         // new, unexported
```

**Guarantees**

1. `NewFileLogger(stateDir)` still creates `<stateDir>/logs` (`0755`) if missing, opens `<stateDir>/logs/shrine.log` with `O_APPEND|O_CREATE|O_WRONLY` (`0644`), and returns the same error texts as today (`creating logs dir %q: %w`, `opening log file %q: %w`). On success it returns `newFileLogger(f)`.
2. `OnEvent(e)` writes exactly one line to `out` per call, under `mu`, in the grammar of `data-model.md` §6 — byte-identical to today's output.
3. `Close()` closes `out` and returns its result. With the real file a second call returns `*os.PathError{Err: os.ErrClosed}` (unchanged stdlib behaviour).
4. `newFileLogger` performs no I/O.

**Non-goals**: no clock injection; no exported writer-based constructor; no change to the log path, permissions, or format.

## 2. `internal/app/components.go` — constructor variables

```go
// Before: five unexported funcs
func newObserverPair(out io.Writer, paths *config.Paths) (engine.Observer, func() error, error) // calls ui.NewFileLogger directly
func newVault(cfg *config.Config) (secrets.SecretsPlugin, error)
func newContainerBackend(store *state.Store, registries []config.RegistryConfig, observer engine.Observer) (engine.ContainerBackend, error)
func newTraefikPlugin(cfg *config.Config, container engine.ContainerBackend, specsDir string, observer engine.Observer) (*traefik.Plugin, error)
func newLocalEngine(opts local.EngineOptions) (*engine.Engine, error)

// After
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

func newObserverPair(out io.Writer, paths *config.Paths) (engine.Observer, func() error, error) // now calls newFileLogger(paths.StateDir)
```

**Guarantees**

1. Every variable has exactly the signature of the func it replaces; every call site in `app.go` compiles unchanged.
2. Production values delegate to the same constructors with the same arguments as today; the observable behaviour of `BuildApplyBundle`, `BuildDeployBundle`, and `BuildTeardownBundle` — slots, error prefixes, unwind, cleanup — is unchanged.
3. `newFileLogger`'s production closure returns a **true nil** interface on error (explicit check), never a typed nil.
4. `newVault`'s production closure preserves today's representation of an absent vault: `infisicalplugin.New(nil)` yields `(nil, nil)`, which becomes a typed-nil `secrets.SecretsPlugin` whose `IsActive()` is `false`.
5. Nothing outside `package app` can observe or assign the variables. Only `package app` tests assign them, always through `swapConstructor`, always restored in `t.Cleanup`.
6. 017 FR-002 holds: each collaborator is still constructed in exactly one production location (`components.go`).

**Non-goals**: no parameters added to `Build*Bundle`; no options struct; no exported provider interface; `cmd/` untouched.

## 3. `internal/app/app.go` — doc comment only

```go
// Before (BuildApplyBundle)
// On success the returned cleanup func is non-nil and idempotent — callers
// MUST defer it.
// After
// On success the returned cleanup func is non-nil and safe to call more than
// once (a repeated call reports the already-closed writer) — callers MUST defer it.
```

No code change. Behaviour of the cleanup func is unchanged and now pinned (`coverage-matrix.md` A-9…A-11).

## 4. Everything else — unchanged

`internal/ui/terminal_logger.go`, `internal/app/app.go` (code), `internal/handler/*.go`, `internal/engine/**`, `internal/plugins/**`, `cmd/**`: no edits. `internal/handler/deployments_test.go` gains one field on an existing test fake (`memDeploymentStore.listErr`).
