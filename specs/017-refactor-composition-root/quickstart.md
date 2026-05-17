# Quickstart — Adding a new command after the composition refactor

**Feature**: 017-refactor-composition-root
**Audience**: Maintainers adding a new `shrine` command that needs the composed dependency graph (vault, observers, engine, routing).

---

## TL;DR

1. Decide which existing bundle fits — or add a new one.
2. Write the handler so its first parameter is the bundle.
3. In `cmd/`, call `app.BuildXBundle(...)`, defer the cleanup, then call the handler.

You will write **zero** lines of plugin / engine / observer construction code in `internal/handler/`. That code lives in `internal/app/` and only there.

---

## Scenario 1 — Your new command fits an existing bundle

The existing scenarios are:

| Bundle | Dependencies |
|--------|--------------|
| `app.DeployBundle` | observer, vault, container backend, Traefik plugin, routing, local engine |
| `app.ApplyBundle` | observer, vault, local engine |
| `app.TeardownBundle` | observer, Traefik plugin, routing, local engine |

If your command needs a strict subset of one of these, reuse the bundle.

```go
// internal/handler/myverb.go
package handler

import (
    "fmt"
    "github.com/CarlosHPlata/shrine/internal/app"
)

func MyVerb(b *app.ApplyBundle, target string) error {
    fmt.Fprintf(b.Out, "doing my verb on %s\n", target)
    // ... use b.Engine, b.Vault, b.Observer ...
    return nil
}
```

```go
// cmd/myverb.go
package cmd

import (
    "github.com/CarlosHPlata/shrine/internal/app"
    "github.com/CarlosHPlata/shrine/internal/handler"
    "github.com/spf13/cobra"
)

var myVerbCmd = &cobra.Command{
    Use:   "myverb",
    Short: "...",
    RunE: func(cmd *cobra.Command, args []string) error {
        bundle, cleanup, err := app.BuildApplyBundle(cfg, store, paths, cmd.OutOrStdout())
        if err != nil {
            return err
        }
        defer cleanup()
        return handler.MyVerb(bundle, args[0])
    },
}
```

That's the entire change. No imports of `internal/plugins/...`, `internal/engine/local`, or `internal/ui` in either file.

---

## Scenario 2 — Your new command needs a *new* dependency combination

Add a new bundle in `internal/app/`:

1. **Define the struct** in `internal/app/app.go`:

   ```go
   type SyncBundle struct {
       Out      io.Writer
       Cfg      *config.Config
       Store    *state.Store
       Observer engine.Observer
       Engine   engine.DeployEngine
       // ... only the fields your scenario actually uses ...
   }
   ```

2. **Add the constructor** alongside `BuildDeployBundle`/`BuildApplyBundle`/`BuildTeardownBundle`:

   ```go
   func BuildSyncBundle(
       cfg *config.Config,
       store *state.Store,
       paths *config.Paths,
       out io.Writer,
   ) (*SyncBundle, func() error, error) {
       if err := cfg.ValidateRegistries(); err != nil {
           return nil, nil, fmt.Errorf("validating registries: %w", err)
       }
       observer, closeObs, err := newObserverPair(out, paths)
       if err != nil {
           return nil, nil, fmt.Errorf("observer: %w", err)
       }
       engine, err := newLocalEngine(local.EngineOptions{
           Store:      store,
           Registries: cfg.Registries,
           Observer:   observer,
       })
       if err != nil {
           _ = closeObs()
           return nil, nil, fmt.Errorf("engine: %w", err)
       }
       return &SyncBundle{
           Out: out, Cfg: cfg, Store: store,
           Observer: observer, Engine: engine,
       }, closeObs, nil
   }
   ```

3. **Reuse the private helpers** (`newObserverPair`, `newVault`, etc. in `components.go`). Do not call leaf constructors (`infisicalplugin.New`, `traefik.New`, …) directly — that is the entire point of the package boundary.

4. **Add a unit test** in `internal/app/app_test.go` that smoke-tests `BuildSyncBundle` with a representative config.

---

## Scenario 3 — Your command needs a *new* leaf dependency (e.g., a new plugin)

1. Add the plugin under `internal/plugins/<category>/<name>/` as today.
2. Add a private helper in `internal/app/components.go`:

   ```go
   func newWidget(cfg *config.Config) (*widget.Plugin, error) {
       return widget.New(cfg.Plugins.Widget)
   }
   ```

3. Add the new dep to whatever bundles need it. New private helpers do **not** need to be public from `internal/app/`.

---

## Scenario 4 — Unit-testing a migrated handler with a stand-in bundle

The bundle types expose interface-typed fields where injection is useful — `Observer engine.Observer` and `Engine *engine.Engine`-via-its-collaborator-interfaces. To unit-test a handler's request-shaped logic without booting plugins:

```go
package handler_test

import (
    "bytes"
    "testing"

    "github.com/CarlosHPlata/shrine/internal/app"
    "github.com/CarlosHPlata/shrine/internal/handler"
    // ... fakes for state.Store, config.Config, engine.Observer ...
)

func TestApplySingle_OutputFormatting(t *testing.T) {
    var buf bytes.Buffer
    bundle := &app.ApplyBundle{
        Out:      &buf,
        Cfg:      newTestConfig(t),    // tiny in-memory config
        Store:    newTestStore(t),     // in-memory store
        Observer: &fakeObserver{},     // no terminal, no file logger
        // Engine: a stand-in engine constructed without local.NewLocalEngine
        // (skip if the test only exercises pre-engine paths like validation errors)
    }
    err := handler.ApplySingle(bundle, "missing.yaml", "/nonexistent")
    // assert on err and buf contents — zero plugin construction took place
}
```

Per `[[feedback_unit_tests_no_filesystem]]`, the test must not write to disk. Use `t.Setenv` to direct `Paths.StateDir` to a path that the handler's code paths under test never read or write — typically the handler is invoked on validation-error paths that exit before touching the engine.

### Known limitations

- `Vault secrets.SecretsPlugin` is an interface and is mockable.
- `TraefikPlugin *traefik.Plugin` (DeployBundle, TeardownBundle) is a concrete type — handlers that exercise `bundle.TraefikPlugin.Deploy()` or `bundle.TraefikPlugin.IsActive()` cannot use a fake without an interface boundary. Today, `handler.Deploy` is the only such caller. If unit-testing those paths becomes important, introduce an interface in `internal/plugins/gateway/traefik/` first; this is a follow-up beyond the scope of feature 017.
- `ContainerBackend engine.ContainerBackend` (DeployBundle) is already an interface — mockable when needed.

---

## Common mistakes

- **Constructing a plugin in `cmd/<command>.go`.** Don't — that recreates the duplication this refactor removed. Always go through `app.BuildXBundle`.
- **Adding a field to an existing bundle "just in case."** Don't — Constitution Principle IV. Add it when a real handler reads it.
- **Forgetting `defer cleanup()`** in `cmd/`. The bundle returns the cleanup function precisely so the caller defers it; skipping it leaks the file logger.
- **Wrapping an error twice.** `BuildXBundle` already wraps with the slot prefix; don't re-wrap in the handler.

---

## Verification before opening a PR

Run, in order:

1. `go build ./...` — must compile.
2. `go test ./...` — all unit tests including `internal/app/app_test.go`.
3. `grep -rE 'infisicalplugin\.New|traefik\.New|local\.NewLocalEngine|ui\.NewTerminalObserver|ui\.NewFileLogger' internal/handler/` — must produce no output (SC-001).
4. `make test-integration` (Constitution Principle V gate, slow — final gate only, per [[feedback_integration_tests_slow]]).
5. Capture `shrine deploy --dry-run`, `shrine apply --file ...`, and `shrine teardown ...` output against a representative manifest set; diff against pre-refactor output (SC-004).

If steps 1–3 pass and step 4 is clean, the refactor is good.
