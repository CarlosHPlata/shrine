package local

import (
	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/engine/local/dockercontainer"
	"github.com/CarlosHPlata/shrine/internal/plugins/secrets"
	"github.com/CarlosHPlata/shrine/internal/resolver"
	"github.com/CarlosHPlata/shrine/internal/state"
)

// EngineOptions bundles the inputs required to construct a local engine.
type EngineOptions struct {
	Store      *state.Store
	Registries []config.RegistryConfig
	Observer   engine.Observer
	Routing    engine.RoutingBackend  // nil disables routing
	Vault      secrets.SecretsPlugin  // nil disables vault resolution
}

func NewLocalEngine(opts EngineOptions) (*engine.Engine, error) {
	res := resolver.NewLiveResolver(opts.Store.Secrets, opts.Vault)

	containerBackend, err := dockercontainer.NewDockerBackend(opts.Store, opts.Registries, opts.Observer)
	if err != nil {
		return nil, err
	}

	return &engine.Engine{
		Container: containerBackend,
		Routing:   opts.Routing,
		DNS:       nil,
		Resolver:  res,
		Observer:  opts.Observer,
	}, nil
}

// NewContainerBackend creates a standalone container backend, intended for
// callers (e.g. plugins) that need to deploy containers outside the engine
// orchestration loop while still emitting observer events and recording state.
func NewContainerBackend(store *state.Store, registries []config.RegistryConfig, observer engine.Observer) (engine.ContainerBackend, error) {
	return dockercontainer.NewDockerBackend(store, registries, observer)
}
