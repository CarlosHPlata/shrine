package dockercontainer

import (
	"context"
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
)

func (backend *DockerBackend) CreateContainer(op engine.CreateContainerOp) error {
	ctx := context.Background()
	cName := containerName(op.Team, op.Name)
	netName := networkName(op.Team)

	if err := backend.ensureImage(ctx, op.Image); err != nil {
		return backend.emitErr("image.ensure", map[string]string{"ref": op.Image},
			fmt.Errorf("ensuring image %q: %w", op.Image, err))
	}

	existing, err := backend.client.ContainerInspect(ctx, cName)
	switch {
	case err == nil && existing.Config.Image == op.Image:
		if !existing.State.Running {
			backend.emitInfo("container.start", map[string]string{"name": cName})
			if err := backend.client.ContainerStart(ctx, existing.ID, container.StartOptions{}); err != nil {
				return backend.emitErr("container.start", map[string]string{"name": cName},
					fmt.Errorf("starting container %q: %w", cName, err))
			}
		}
		return backend.recordDeployment(op, existing.ID)

	case err == nil:
		backend.emitInfo("container.recreate", map[string]string{"name": cName})
		if err := backend.client.ContainerRemove(ctx, existing.ID, container.RemoveOptions{Force: true}); err != nil {
			return backend.emitErr("container.remove", map[string]string{"name": cName},
				fmt.Errorf("removing stale container %q: %w", cName, err))
		}

	case !errdefs.IsNotFound(err):
		return backend.emitErr("container.inspect", map[string]string{"name": cName},
			fmt.Errorf("inspecting container %q: %w", cName, err))
	}

	backend.emitInfo("container.fresh", map[string]string{"name": cName})
	labels := map[string]string{
		"shrine.team":     op.Team,
		"shrine.resource": op.Name,
		"shrine.kind":     op.Kind,
	}

	mounts := make([]mount.Mount, len(op.Volumes))
	for i, v := range op.Volumes {
		if err := backend.ensureVolume(ctx, op, v); err != nil {
			return err
		}
		mounts[i] = mount.Mount{
			Type:   mount.TypeVolume,
			Source: volumeName(op.Team, op.Name, v.Name),
			Target: v.MountPath,
		}
	}

	endpoints := map[string]*network.EndpointSettings{
		netName: {},
	}
	if op.ExposeToPlatform {
		endpoints[platformNetworkName] = &network.EndpointSettings{}
	}

	created, err := backend.client.ContainerCreate(ctx,
		&container.Config{
			Image:  op.Image,
			Env:    op.Env,
			Labels: labels,
		},
		&container.HostConfig{
			Mounts: mounts,
		},
		&network.NetworkingConfig{
			EndpointsConfig: endpoints,
		},
		nil,
		cName,
	)
	if err != nil {
		return backend.emitErr("container.create", map[string]string{"name": cName},
			fmt.Errorf("creating container %q: %w", cName, err))
	}

	if err := backend.client.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return backend.emitErr("container.start", map[string]string{"name": cName},
			fmt.Errorf("starting container %q: %w", cName, err))
	}

	backend.emitFinished("container.created", map[string]string{"name": cName})
	return backend.recordDeployment(op, created.ID)
}

func (backend *DockerBackend) RemoveContainer(op engine.RemoveContainerOp) error {
	ctx := context.Background()
	cName := containerName(op.Team, op.Name)

	existing, err := backend.client.ContainerInspect(ctx, cName)
	if err != nil && !errdefs.IsNotFound(err) {
		return backend.emitErr("container.inspect", map[string]string{"name": cName},
			fmt.Errorf("inspecting container %q: %w", cName, err))
	}

	if errdefs.IsNotFound(err) {
		backend.emitInfo("container.remove", map[string]string{"name": cName, "reason": "not found"})
	} else {
		backend.emitStarted("container.remove", map[string]string{"name": cName})
		if err = backend.client.ContainerRemove(
			ctx,
			existing.ID,
			container.RemoveOptions{Force: true},
		); err != nil {
			return backend.emitErr("container.remove", map[string]string{"name": cName},
				fmt.Errorf("removing container %q: %w", cName, err))
		}
	}

	if err := backend.removeDeployment(op); err != nil {
		return backend.emitErr("deployment.remove", map[string]string{"name": cName},
			fmt.Errorf("removing deployment for %q: %w", cName, err))
	}

	backend.emitFinished("container.remove", map[string]string{"name": cName})
	return nil
}

func (backend *DockerBackend) recordDeployment(op engine.CreateContainerOp, ID string) error {
	return backend.state.Deployments.Record(op.Team, state.Deployment{
		Kind:        op.Kind,
		Name:        op.Name,
		ContainerID: ID,
	})
}

func (backend *DockerBackend) removeDeployment(op engine.RemoveContainerOp) error {
	return backend.state.Deployments.Remove(op.Team, op.Name)
}

func containerName(team string, name string) string {
	return fmt.Sprintf("%s.%s", team, name)
}
