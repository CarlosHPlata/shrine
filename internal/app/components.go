package app

import (
	"errors"
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/engine/local"
	"github.com/CarlosHPlata/shrine/internal/plugins/gateway/traefik"
	infisicalplugin "github.com/CarlosHPlata/shrine/internal/plugins/secrets/infisical"
	"github.com/CarlosHPlata/shrine/internal/plugins/secrets"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/CarlosHPlata/shrine/internal/ui"
)

// newObserverPair composes the standard terminal + file-logger observer pair
// used by every long-running command. The returned cleanup func closes the
// file logger; callers must defer it.
func newObserverPair(out io.Writer, paths *config.Paths) (engine.Observer, func() error, error) {
	terminal := ui.NewTerminalObserver(out)
	fileLogger, err := ui.NewFileLogger(paths.StateDir)
	if err != nil {
		return nil, nil, fmt.Errorf("initializing file logger: %w", err)
	}
	observer := engine.MultiObserver{terminal, fileLogger}
	return observer, fileLogger.Close, nil
}

// newVault constructs the secrets vault plugin from config.
func newVault(cfg *config.Config) (secrets.SecretsPlugin, error) {
	return infisicalplugin.New(cfg.Plugins.Secrets.Infisical)
}

// newContainerBackend constructs the standalone container backend used by the
// Traefik plugin during deploys (it deploys its own container outside the engine
// orchestration loop).
func newContainerBackend(store *state.Store, registries []config.RegistryConfig, observer engine.Observer) (engine.ContainerBackend, error) {
	return local.NewContainerBackend(store, registries, observer)
}

// newTraefikPlugin constructs the Traefik gateway plugin.
func newTraefikPlugin(cfg *config.Config, container engine.ContainerBackend, specsDir string, observer engine.Observer) (*traefik.Plugin, error) {
	return traefik.New(cfg.Plugins.Gateway.Traefik, container, specsDir, observer)
}

// newLocalEngine constructs the local deploy engine.
func newLocalEngine(opts local.EngineOptions) (*engine.Engine, error) {
	return local.NewLocalEngine(opts)
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
