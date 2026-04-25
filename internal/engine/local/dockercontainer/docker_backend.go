package dockercontainer

import (
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/docker/docker/client"
)

type DockerBackend struct {
	client     *client.Client
	state      *state.Store
	registries []config.RegistryConfig
	observer   engine.Observer
}

func NewDockerBackend(s *state.Store, registries []config.RegistryConfig, observer engine.Observer) (*DockerBackend, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerBackend{
		client:     cli,
		state:      s,
		registries: registries,
		observer:   observer,
	}, nil
}

func (b *DockerBackend) emitErr(name string, fields map[string]string, err error) error {
	if fields == nil {
		fields = map[string]string{}
	}
	fields["error"] = err.Error()
	b.observer.OnEvent(engine.Event{Name: name, Status: engine.StatusError, Fields: fields})
	return err
}

func (b *DockerBackend) emitStarted(name string, fields map[string]string) {
	b.observer.OnEvent(engine.Event{Name: name, Status: engine.StatusStarted, Fields: fields})
}

func (b *DockerBackend) emitFinished(name string, fields map[string]string) {
	b.observer.OnEvent(engine.Event{Name: name, Status: engine.StatusFinished, Fields: fields})
}

func (b *DockerBackend) emitInfo(name string, fields map[string]string) {
	b.observer.OnEvent(engine.Event{Name: name, Status: engine.StatusInfo, Fields: fields})
}
