package dockercontainer

import (
	"context"
	"fmt"

	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/network"
)

const (
	platformNetworkName = "shrine.platform"
	platformSubnet      = "10.200.0.0/24"
)

func (backend *DockerBackend) CreateNetwork(team string) error {
	ctx := context.Background()
	name := networkName(team)

	cidr, err := backend.state.Subnets.AllocateSubnet(team)
	if err != nil {
		return backend.emitErr("subnet.allocate", map[string]string{"team": team},
			fmt.Errorf("Allocating subnet for %q: %w", team, err))
	}

	existing, err := backend.client.NetworkInspect(ctx, name, network.InspectOptions{})
	if err == nil {
		if len(existing.IPAM.Config) == 0 || existing.IPAM.Config[0].Subnet != cidr {
			return backend.emitErr("network.inspect", map[string]string{"name": name, "want": cidr},
				fmt.Errorf("Network %q exists with wrong subnet: want %s, have %+v", name, cidr, existing.IPAM.Config))
		}
		return nil
	}

	if !errdefs.IsNotFound(err) {
		return backend.emitErr("network.inspect", map[string]string{"name": name},
			fmt.Errorf("inspecting network %q: %w", name, err))
	}

	backend.emitStarted("network.create", map[string]string{"name": name})

	_, err = backend.client.NetworkCreate(ctx, name, network.CreateOptions{
		Driver: "bridge",
		IPAM: &network.IPAM{
			Driver: "default",
			Config: []network.IPAMConfig{{Subnet: cidr}},
		},
	})
	if err != nil {
		return backend.emitErr("network.create", map[string]string{"name": name},
			fmt.Errorf("creating network %q: %w", name, err))
	}

	backend.emitFinished("network.create", map[string]string{"name": name, "cidr": cidr})
	return nil
}

func (backend *DockerBackend) RemoveNetwork(team string) error {
	ctx := context.Background()
	name := networkName(team)

	existing, err := backend.client.NetworkInspect(ctx, name, network.InspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}

	backend.emitStarted("network.remove", map[string]string{"name": name})
	if err != nil {
		return backend.emitErr("network.inspect", map[string]string{"name": name},
			fmt.Errorf("inspecting network %q: %w", name, err))
	}

	if len(existing.Containers) > 0 {
		return backend.emitErr("network.remove", map[string]string{"network": name},
			fmt.Errorf("network %q is not empty: %d containers", name, len(existing.Containers)))
	}

	if err := backend.client.NetworkRemove(ctx, existing.ID); err != nil {
		return backend.emitErr("network.remove", map[string]string{"id": existing.ID},
			fmt.Errorf("removing network %q: %w", existing.ID, err))
	}

	backend.emitFinished("network.remove", map[string]string{"name": name})
	return nil
}

func (backend *DockerBackend) CreatePlatformNetwork() error {
	ctx := context.Background()

	existing, err := backend.client.NetworkInspect(ctx, platformNetworkName, network.InspectOptions{})
	if err == nil {
		if len(existing.IPAM.Config) == 0 || existing.IPAM.Config[0].Subnet != platformSubnet {
			return backend.emitErr("network.inspect", map[string]string{"name": platformNetworkName, "want": platformSubnet},
				fmt.Errorf("Network %q exists with wrong subnet: want %s, have %+v", platformNetworkName, platformSubnet, existing.IPAM.Config))
		}
		return nil
	}

	if !errdefs.IsNotFound(err) {
		return backend.emitErr("network.inspect", map[string]string{"name": platformNetworkName},
			fmt.Errorf("inspecting network %q: %w", platformNetworkName, err))
	}

	backend.emitStarted("network.create", map[string]string{"name": platformNetworkName})

	_, err = backend.client.NetworkCreate(ctx, platformNetworkName, network.CreateOptions{
		Driver: "bridge",
		IPAM: &network.IPAM{
			Driver: "default",
			Config: []network.IPAMConfig{{Subnet: platformSubnet}},
		},
	})
	if err != nil {
		return backend.emitErr("network.create", map[string]string{"name": platformNetworkName},
			fmt.Errorf("creating network %q: %w", platformNetworkName, err))
	}

	backend.emitFinished("network.create", map[string]string{"name": platformNetworkName, "cidr": platformSubnet})
	return nil
}

func networkName(team string) string {
	return fmt.Sprintf("shrine.%s.private", team)
}
