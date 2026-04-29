package local

import (
	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/engine/local/dockercontainer"
	"github.com/CarlosHPlata/shrine/internal/resolver"
	"github.com/CarlosHPlata/shrine/internal/state"
)

func NewLocalEngine(store *state.Store, registries []config.RegistryConfig, observer engine.Observer) (*engine.Engine, error) {
	return NewLocalEngineWithRouting(store, registries, observer, nil)
}

func NewLocalEngineWithRouting(store *state.Store, registries []config.RegistryConfig, observer engine.Observer, routing engine.RoutingBackend) (*engine.Engine, error) {
	resolver := resolver.NewLiveResolver(store.Secrets)

	containerBackend, err := dockercontainer.NewDockerBackend(store, registries, observer)
	if err != nil {
		return nil, err
	}

	return &engine.Engine{
		Container: containerBackend,
		Routing:   routing,
		DNS:       nil,
		Resolver:  resolver,
		Observer:  observer,
	}, nil
}

// NewContainerBackend creates a standalone container backend, intended for
// callers (e.g. plugins) that need to deploy containers outside the engine
// orchestration loop while still emitting observer events and recording state.
func NewContainerBackend(store *state.Store, registries []config.RegistryConfig, observer engine.Observer) (engine.ContainerBackend, error) {
	return dockercontainer.NewDockerBackend(store, registries, observer)
}
