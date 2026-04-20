package local

import (
	"context"
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

type DockerBackend struct {
	client     *client.Client
	state      *state.Store
	registries []config.RegistryConfig
}

func NewDockerBackend(state *state.Store, registries []config.RegistryConfig) (*DockerBackend, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerBackend{
		client:     cli,
		state:      state,
		registries: registries,
	}, nil
}

func networkName(team string) string {
	return fmt.Sprintf("shrine.%s.private", team)
}

func (backend *DockerBackend) CreateNetwork(team string) error {
	ctx := context.Background()
	name := networkName(team)

	// Get (or allocate) the team's CIRDR. AllocateSubnet will be idempotent
	cidr, err := backend.state.Subnets.AllocateSubnet(team)
	if err != nil {
		return fmt.Errorf("Allocating subnet for %q: %w", team, err)
	}

	// Check if network already exists
	existing, err := backend.client.NetworkInspect(ctx, name, network.InspectOptions{})
	if err == nil {
		// If network exists verify that the subnets matches what we expect.
		if len(existing.IPAM.Config) == 0 || existing.IPAM.Config[0].Subnet != cidr {
			return fmt.Errorf("Network %q exists with wrong subnet: want %s, have %+v", name, cidr, existing.IPAM.Config)
		}
		return nil
	}

	// If the error is not "not found", return it.
	if !errdefs.IsNotFound(err) {
		return fmt.Errorf("inspecting network %q: %w", name, err)
	}

	// If not found, create it.
	_, err = backend.client.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
		IPAM: &network.IPAM{
			Driver: "default",
			Config: []network.IPAMConfig{{Subnet: cidr}},
		},
	})
	if err != nil {
		return fmt.Errorf("creating network %q: %w", name, err)
	}

	return nil
}

func (backend *DockerBackend) RemoveNetwork(name string) error {
	return nil
}

func (backend *DockerBackend) CreateContainer(op engine.CreateContainerOp) error {
	return nil
}

func (backend *DockerBackend) RemoveContainer(name string) error {
	return nil
}
