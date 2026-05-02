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
	"github.com/docker/go-connections/nat"
)

func (backend *DockerBackend) CreateContainer(op engine.CreateContainerOp) error {
	ctx := context.Background()
	cName := containerName(op.Team, op.Name)
	netName := networkName(op.Team)

	digest, err := backend.resolveImage(ctx, op.Image, op.ImagePullPolicy)
	if err != nil {
		return err
	}

	wantHash := configHash(op, digest)

	existing, err := backend.client.ContainerInspect(ctx, cName)
	switch {
	case err == nil:
		deployments, stateErr := backend.state.Deployments.List(op.Team)
		if stateErr != nil {
			return backend.emitErr("deployment.list", map[string]string{"team": op.Team},
				fmt.Errorf("listing deployments for team %q: %w", op.Team, stateErr))
		}

		if isContainerUpToDate(deployments, op.Name, wantHash) {
			return backend.ensureRunning(ctx, cName, existing, op, wantHash)
		}

		if err := backend.removeStaleContainer(ctx, cName, existing.ID); err != nil {
			return err
		}

	case !errdefs.IsNotFound(err):
		return backend.emitErr("container.inspect", map[string]string{"name": cName},
			fmt.Errorf("inspecting container %q: %w", cName, err))
	}

	return backend.createFreshContainer(ctx, op, cName, netName, wantHash)
}

func (backend *DockerBackend) ensureRunning(ctx context.Context, cName string, existing container.InspectResponse, op engine.CreateContainerOp, wantHash string) error {
	if !existing.State.Running {
		backend.emitInfo("container.start", map[string]string{"name": cName})
		if err := backend.client.ContainerStart(ctx, existing.ID, container.StartOptions{}); err != nil {
			return backend.emitErr("container.start", map[string]string{"name": cName},
				fmt.Errorf("starting container %q: %w", cName, err))
		}
	}
	return backend.recordDeployment(op, existing.ID, wantHash)
}

func (backend *DockerBackend) removeStaleContainer(ctx context.Context, cName, existingID string) error {
	backend.emitInfo("container.recreate", map[string]string{"name": cName})
	if err := backend.client.ContainerRemove(ctx, existingID, container.RemoveOptions{Force: true}); err != nil {
		return backend.emitErr("container.remove", map[string]string{"name": cName},
			fmt.Errorf("removing stale container %q: %w", cName, err))
	}
	return nil
}

func (backend *DockerBackend) createFreshContainer(ctx context.Context, op engine.CreateContainerOp, cName, netName, wantHash string) error {
	backend.emitInfo("container.fresh", map[string]string{"name": cName})

	labels := map[string]string{
		"shrine.team":     op.Team,
		"shrine.resource": op.Name,
		"shrine.kind":     op.Kind,
	}

	mounts, err := backend.buildMounts(ctx, op)
	if err != nil {
		return err
	}

	exposedPorts, portBindings := buildPortBindings(op.PortBindings)

	created, err := backend.client.ContainerCreate(ctx,
		&container.Config{
			Image:        op.Image,
			Env:          op.Env,
			Labels:       labels,
			ExposedPorts: exposedPorts,
		},
		&container.HostConfig{
			Mounts:        mounts,
			PortBindings:  portBindings,
			RestartPolicy: buildRestartPolicy(op.RestartPolicy),
		},
		buildNetwork(op, netName),
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
	return backend.recordDeployment(op, created.ID, wantHash)
}

func (backend *DockerBackend) buildMounts(ctx context.Context, op engine.CreateContainerOp) ([]mount.Mount, error) {
	mounts := make([]mount.Mount, 0, len(op.Volumes)+len(op.BindMounts))
	for _, v := range op.Volumes {
		if err := backend.ensureVolume(ctx, op, v); err != nil {
			return nil, err
		}
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeVolume,
			Source: volumeName(op.Team, op.Name, v.Name),
			Target: v.MountPath,
		})
	}
	for _, b := range op.BindMounts {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: b.Source,
			Target: b.Target,
		})
	}
	return mounts, nil
}

func buildRestartPolicy(name string) container.RestartPolicy {
	if name == "" {
		return container.RestartPolicy{}
	}
	return container.RestartPolicy{Name: container.RestartPolicyMode(name)}
}

func buildPortBindings(bindings []PortBinding) (nat.PortSet, nat.PortMap) {
	if len(bindings) == 0 {
		return nil, nil
	}
	exposed := nat.PortSet{}
	pmap := nat.PortMap{}
	for _, b := range bindings {
		proto := b.Protocol
		if proto == "" {
			proto = "tcp"
		}
		key := nat.Port(b.ContainerPort + "/" + proto)
		exposed[key] = struct{}{}
		pmap[key] = append(pmap[key], nat.PortBinding{HostPort: b.HostPort})
	}
	return exposed, pmap
}

// PortBinding mirrors engine.PortBinding to keep the local helper signature
// independent of the engine import path inside this file.
type PortBinding = engine.PortBinding

func buildNetwork(op engine.CreateContainerOp, netName string) *network.NetworkingConfig {
	if op.Team == platformTeam {
		return &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{
			platformNetworkName: {},
		}}
	}
	endpoints := map[string]*network.EndpointSettings{
		netName: {},
	}
	if op.ExposeToPlatform {
		endpoints[platformNetworkName] = &network.EndpointSettings{}
	}
	return &network.NetworkingConfig{EndpointsConfig: endpoints}
}

func isContainerUpToDate(deployments []state.Deployment, name, wantHash string) bool {
	for _, d := range deployments {
		if d.Name == name {
			return d.ConfigHash == wantHash
		}
	}
	return false
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

func (backend *DockerBackend) recordDeployment(op engine.CreateContainerOp, ID string, hash string) error {
	return backend.state.Deployments.Record(op.Team, state.Deployment{
		Kind:        op.Kind,
		Name:        op.Name,
		ContainerID: ID,
		ConfigHash:  hash,
	})
}

func configHash(op engine.CreateContainerOp, digest string) string {
	volSpecs := make([]string, len(op.Volumes))
	for i, v := range op.Volumes {
		volSpecs[i] = v.Name + ":" + v.MountPath
	}
	portSpecs := make([]string, len(op.PortBindings))
	for i, b := range op.PortBindings {
		proto := b.Protocol
		if proto == "" {
			proto = "tcp"
		}
		portSpecs[i] = fmt.Sprintf("%s:%s/%s", b.HostPort, b.ContainerPort, proto)
	}
	return state.ConfigHash(digest, op.Env, volSpecs, portSpecs, op.ExposeToPlatform)
}

func (backend *DockerBackend) removeDeployment(op engine.RemoveContainerOp) error {
	return backend.state.Deployments.Remove(op.Team, op.Name)
}

func containerName(team string, name string) string {
	return fmt.Sprintf("%s.%s", team, name)
}
