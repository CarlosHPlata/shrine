package local

import (
	"context"
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
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

func (backend *DockerBackend) emitErr(name string, fields map[string]string, err error) error {
	if fields == nil {
		fields = map[string]string{}
	}
	fields["error"] = err.Error()
	backend.observer.OnEvent(engine.Event{Name: name, Status: engine.StatusError, Fields: fields})
	return err
}

func (backend *DockerBackend) CreateNetwork(team string) error {
	ctx := context.Background()
	name := networkName(team)

	// Get (or allocate) the team's CIDR. AllocateSubnet will be idempotent
	cidr, err := backend.state.Subnets.AllocateSubnet(team)
	if err != nil {
		return backend.emitErr("subnet.allocate", map[string]string{"team": team},
			fmt.Errorf("Allocating subnet for %q: %w", team, err))
	}

	// Check if network already exists
	existing, err := backend.client.NetworkInspect(ctx, name, network.InspectOptions{})
	if err == nil {
		// If network exists verify that the subnets matches what we expect.
		if len(existing.IPAM.Config) == 0 || existing.IPAM.Config[0].Subnet != cidr {
			return backend.emitErr("network.inspect", map[string]string{"name": name, "want": cidr},
				fmt.Errorf("Network %q exists with wrong subnet: want %s, have %+v", name, cidr, existing.IPAM.Config))
		}
		return nil
	}

	// If the error is not "not found", return it.
	if !errdefs.IsNotFound(err) {
		return backend.emitErr("network.inspect", map[string]string{"name": name},
			fmt.Errorf("inspecting network %q: %w", name, err))
	}

	// If not found, create it.
	backend.observer.OnEvent(engine.Event{
		Name:   "network.create",
		Status: engine.StatusInfo,
		Fields: map[string]string{"name": name, "cidr": cidr},
	})
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

	return nil
}

func (backend *DockerBackend) RemoveNetwork(name string) error {
	return nil
}

func (backend *DockerBackend) CreateContainer(op engine.CreateContainerOp) error {
	ctx := context.Background()
	cName := containerName(op.Team, op.Name)
	netName := networkName(op.Team)

	// Ensure image locally -- check with ImageList; pull if missing, using per-registry auth.
	// filter
	if err := backend.ensureImage(ctx, op.Image); err != nil {
		return backend.emitErr("image.ensure", map[string]string{"ref": op.Image},
			fmt.Errorf("ensuring image %q: %w", op.Image, err))
	}

	// Inspect existing container (reconcile by name) 3 cases
	//  1. Not found -> create fresh
	//  2. Found with matching image -> ensure running (start if stopped), done.
	//  3. Found with different image -> recreate (rm, create, start)
	existing, err := backend.client.ContainerInspect(ctx, cName)
	switch {
	case err == nil && existing.Config.Image == op.Image:
		if !existing.State.Running {
			backend.observer.OnEvent(engine.Event{
				Name:   "container.start",
				Status: engine.StatusInfo,
				Fields: map[string]string{"name": cName},
			})
			if err := backend.client.ContainerStart(ctx, existing.ID, container.StartOptions{}); err != nil {
				return backend.emitErr("container.start", map[string]string{"name": cName},
					fmt.Errorf("starting container %q: %w", cName, err))
			}
		}
		return backend.recordDeployment(op, existing.ID)

	case err == nil:
		// Image drift: remove old container
		backend.observer.OnEvent(engine.Event{
			Name:   "container.recreate",
			Status: engine.StatusInfo,
			Fields: map[string]string{"name": cName},
		})
		if err := backend.client.ContainerRemove(ctx, existing.ID, container.RemoveOptions{Force: true}); err != nil {
			return backend.emitErr("container.remove", map[string]string{"name": cName},
				fmt.Errorf("removing stale container %q: %w", cName, err))
		}

	case !errdefs.IsNotFound(err):
		return backend.emitErr("container.inspect", map[string]string{"name": cName},
			fmt.Errorf("inspecting container %q: %w", cName, err))
	}

	// Create + start - with labels, env, network attachment.
	backend.observer.OnEvent(engine.Event{
		Name:   "container.fresh",
		Status: engine.StatusInfo,
		Fields: map[string]string{"name": cName},
	})
	labels := map[string]string{
		"shrine.team":     op.Team,
		"shrine.resource": op.Name,
		"shrine.kind":     op.Kind,
	}

	// ensuring volumes are created
	mounts := make([]mount.Mount, len(op.Volumes))
	for i, v := range op.Volumes {
		if err := backend.ensureVolume(ctx, op, v); err != nil {
			return err // ensureVolume already emitted error event
		}

		mounts[i] = mount.Mount{
			Type:   mount.TypeVolume,
			Source: volumeName(op.Team, op.Name, v.Name),
			Target: v.MountPath,
		}
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
			EndpointsConfig: map[string]*network.EndpointSettings{
				netName: {},
			},
		},
		nil, //platform
		cName,
	)
	if err != nil {
		return backend.emitErr("container.create", map[string]string{"name": cName},
			fmt.Errorf("creating container %q: %w", cName, err))
	}

	// start
	if err := backend.client.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return backend.emitErr("container.start", map[string]string{"name": cName},
			fmt.Errorf("starting container %q: %w", cName, err))
	}

	backend.observer.OnEvent(engine.Event{
		Name:   "container.created",
		Status: engine.StatusFinished,
		Fields: map[string]string{"name": cName},
	})

	// record, best-effort; failure here shouldn't tear down a running container
	return backend.recordDeployment(op, created.ID)
}

func (backend *DockerBackend) RemoveContainer(name string) error {
	return nil
}

func (backend *DockerBackend) recordDeployment(op engine.CreateContainerOp, ID string) error {
	return backend.state.Deployments.Record(op.Team, state.Deployment{
		Kind:        op.Kind,
		Name:        op.Name,
		ContainerID: ID,
	})
}

func (backend *DockerBackend) ensureVolume(ctx context.Context, op engine.CreateContainerOp, v engine.VolumeMount) error {
	name := volumeName(op.Team, op.Name, v.Name)

	_, err := backend.client.VolumeInspect(ctx, name)
	if err == nil {
		// Volume exists, trust it.
		return nil
	}

	if !errdefs.IsNotFound(err) {
		return backend.emitErr("volume.inspect", map[string]string{"name": name},
			fmt.Errorf("inspecting volume %q: %w", name, err))
	}

	backend.observer.OnEvent(engine.Event{
		Name:   "volume.create",
		Status: engine.StatusInfo,
		Fields: map[string]string{"name": name, "mount": v.MountPath},
	})

	_, err = backend.client.VolumeCreate(ctx, volume.CreateOptions{
		Name: name,
		Labels: map[string]string{
			"shrine.team":     op.Team,
			"shrine.resource": op.Name,
			"shrine.kind":     op.Kind,
			"shrine.volume":   v.Name,
		},
	})
	if err != nil {
		return backend.emitErr("volume.create", map[string]string{"name": name},
			fmt.Errorf("creating volume %q: %w", name, err))
	}

	backend.observer.OnEvent(engine.Event{
		Name:   "volume.created",
		Status: engine.StatusFinished,
		Fields: map[string]string{"name": name, "mount": v.MountPath},
	})
	return nil
}

func (backend *DockerBackend) ensureImage(ctx context.Context, ref string) error {
	args := filters.NewArgs()
	args.Add("reference", ref)
	existing, err := backend.client.ImageList(ctx, image.ListOptions{Filters: args})
	if err != nil {
		return backend.emitErr("image.list", map[string]string{"ref": ref},
			fmt.Errorf("listing images: %w", err))
	}

	if len(existing) > 0 {
		return nil // already cached
	}

	authB64, err := backend.registryAuthFor(ref)
	if err != nil {
		return backend.emitErr("registry.auth", map[string]string{"ref": ref}, err)
	}

	backend.observer.OnEvent(engine.Event{
		Name:   "image.pull",
		Status: engine.StatusStarted,
		Fields: map[string]string{"ref": ref},
	})

	reader, err := backend.client.ImagePull(ctx, ref, image.PullOptions{
		RegistryAuth: authB64,
	})
	if err != nil {
		return backend.emitErr("image.pull", map[string]string{"ref": ref},
			fmt.Errorf("pulling image %q: %w", ref, err))
	}
	defer reader.Close()

	// ImagePull returns a streaming reader; the pull only happens as we read.
	// Drain the EOF so the pull actually completes.
	if _, err = io.Copy(io.Discard, reader); err != nil {
		return backend.emitErr("image.pull", map[string]string{"ref": ref},
			fmt.Errorf("reading image stream for %q: %w", ref, err))
	}

	backend.observer.OnEvent(engine.Event{
		Name:   "image.pull",
		Status: engine.StatusFinished,
		Fields: map[string]string{"ref": ref},
	})
	return nil
}

func networkName(team string) string {
	return fmt.Sprintf("shrine.%s.private", team)
}

func volumeName(team string, res string, name string) string {
	return fmt.Sprintf("shrine.%s.%s.%s", team, res, name)
}

func containerName(team string, name string) string {
	return fmt.Sprintf("%s.%s", team, name)
}
