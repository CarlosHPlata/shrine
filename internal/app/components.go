package app

import (
	"errors"
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/engine/local"
	"github.com/CarlosHPlata/shrine/internal/plugins/gateway/traefik"
	"github.com/CarlosHPlata/shrine/internal/plugins/secrets"
	infisicalplugin "github.com/CarlosHPlata/shrine/internal/plugins/secrets/infisical"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/CarlosHPlata/shrine/internal/ui"
)

// newObserverPair composes the standard terminal + file-logger observer pair
// used by every long-running command. The returned cleanup func closes the
// file logger; callers must defer it.
func newObserverPair(out io.Writer, paths *config.Paths) (engine.Observer, func() error, error) {
	terminal := ui.NewTerminalObserver(out)
	fileLogger, err := newFileLogger(paths.StateDir)
	if err != nil {
		return nil, nil, fmt.Errorf("initializing file logger: %w", err)
	}
	observer := engine.MultiObserver{terminal, fileLogger}
	return observer, fileLogger.Close, nil
}

// closableObserver is the file logger as newObserverPair uses it.
type closableObserver interface {
	engine.Observer
	Close() error
}

// The collaborator constructors are variables so tests in this package can
// substitute them without a filesystem, Docker daemon, or network. Production
// code never reassigns them: each collaborator is still built in one place.
var (
	newFileLogger = func(stateDir string) (closableObserver, error) {
		logger, err := ui.NewFileLogger(stateDir)
		if err != nil {
			return nil, err
		}
		return logger, nil
	}

	// newVault constructs the secrets vault plugin from config.
	newVault = func(cfg *config.Config) (secrets.SecretsPlugin, error) {
		return infisicalplugin.New(cfg.Plugins.Secrets.Infisical)
	}

	// newContainerBackend constructs the standalone container backend used by the
	// Traefik plugin during deploys (it deploys its own container outside the engine
	// orchestration loop).
	newContainerBackend = local.NewContainerBackend

	// newTraefikPlugin constructs the Traefik gateway plugin.
	newTraefikPlugin = func(cfg *config.Config, container engine.ContainerBackend, specsDir string, observer engine.Observer) (*traefik.Plugin, error) {
		return traefik.New(cfg.Plugins.Gateway.Traefik, container, specsDir, observer)
	}

	// newLocalEngine constructs the local deploy engine.
	newLocalEngine = local.NewLocalEngine
)

// NewQueryContainerBackend builds a container backend for commands that only
// query Docker state (e.g. delete application's is-it-still-running probe) —
// no observer output.
func NewQueryContainerBackend(cfg *config.Config, store *state.Store) (engine.ContainerBackend, error) {
	return local.NewContainerBackend(store, cfg.Registries, engine.NoopObserver{})
}

// Teardown reads no manifests, so an unset specsDir is not an error.
func resolveOptionalSpecsDir(cfg *config.Config) (string, error) {
	if cfg.SpecsDir == "" {
		return "", nil
	}
	return cfg.ResolveSpecsDir("")
}

// routingFromPlugin extracts the routing backend from an active Traefik plugin.
// Returns (nil, nil) when the plugin is inactive — callers must treat a nil
// routing backend as "routing disabled" per Constitution Principle III.
func routingFromPlugin(plugin *traefik.Plugin) (engine.RoutingBackend, error) {
	if !plugin.IsActive() {
		return nil, nil
	}
	return plugin.RoutingBackend()
}

// joinCleanup returns a cleanup func that invokes all provided closers and
// joins their errors via errors.Join. nil entries are skipped.
func joinCleanup(closers ...func() error) func() error {
	return func() error {
		var errs []error
		for _, c := range closers {
			if c == nil {
				continue
			}
			if err := c(); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
}
