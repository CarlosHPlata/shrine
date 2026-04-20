package local

import (
	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/resolver"
	"github.com/CarlosHPlata/shrine/internal/state"
)

func NewLocalEngine(store *state.Store, registries []config.RegistryConfig) (*engine.Engine, error) {
	resolver := resolver.NewLiveResolver(store.Secrets)

	containerBackend, err := NewDockerBackend(store, registries)
	if err != nil {
		return nil, err
	}

	return &engine.Engine{
		Container: containerBackend,
		Routing:   nil,
		DNS:       nil,
		Resolver:  resolver,
	}, nil
}
