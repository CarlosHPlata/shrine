package local

import (
	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/engine/local/dockercontainer"
	"github.com/CarlosHPlata/shrine/internal/resolver"
	"github.com/CarlosHPlata/shrine/internal/state"
)

func NewLocalEngine(store *state.Store, registries []config.RegistryConfig, observer engine.Observer) (*engine.Engine, error) {
	resolver := resolver.NewLiveResolver(store.Secrets)

	containerBackend, err := dockercontainer.NewDockerBackend(store, registries, observer)
	if err != nil {
		return nil, err
	}

	return &engine.Engine{
		Container: containerBackend,
		Routing:   nil,
		DNS:       nil,
		Resolver:  resolver,
		Observer:  observer,
	}, nil
}
