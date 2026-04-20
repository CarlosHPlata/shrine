package local

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

type DockerBackend struct {
	client     *client.Client
	state      *state.Store
	registries []config.RegistryConfig
}

func NewDockerBackend(s *state.Store, registries []config.RegistryConfig) (*DockerBackend, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerBackend{
		client:     cli,
		state:      s,
		registries: registries,
	}, nil
}

func (backend *DockerBackend) CreateNetwork(team string) error {
	ctx := context.Background()
	name := networkName(team)

	// Get (or allocate) the team's CIDR. AllocateSubnet will be idempotent
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
	fmt.Printf("    🔨 Creating Docker network: %s (%s)\n", name, cidr)
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
	ctx := context.Background()
	cName := containerName(op.Team, op.Name)
	netName := networkName(op.Team)

	// Ensure image locally -- check with ImageList; pull if missing, using per-registry auth.
	// filter
	if err := backend.ensureImage(ctx, op.Image); err != nil {
		return fmt.Errorf("ensuring image %q: %w", op.Image, err)
	}

	// Inspect existing container (reconcile by name) 3 cases
	//  1. Not found -> create fresh
	//  2. Found with matching image -> ensure running (start if stopped), done.
	//  3. Found with different image -> recreate (rm, create, start)
	existing, err := backend.client.ContainerInspect(ctx, cName)
	switch {
	case err == nil && existing.Config.Image == op.Image:
		if !existing.State.Running {
			fmt.Printf("    ▶️  Starting existing container: %s\n", cName)
			if err := backend.client.ContainerStart(ctx, existing.ID, container.StartOptions{}); err != nil {
				return fmt.Errorf("starting container %q: %w", cName, err)
			}
		}
		return backend.recordDeployment(op, existing.ID)

	case err == nil:
		// Image drift: remove old container
		fmt.Printf("    🔄 Image changed for %s, replacing container...\n", cName)
		if err := backend.client.ContainerRemove(ctx, existing.ID, container.RemoveOptions{Force: true}); err != nil {
			return fmt.Errorf("removing stale container %q: %w", cName, err)
		}

	case !errdefs.IsNotFound(err):
		return fmt.Errorf("inspecting container %q: %w", cName, err)
	}

	// Create + start - with labels, env, network attachment.
	fmt.Printf("    ✨ Creating fresh container: %s\n", cName)
	labels := map[string]string{
		"shrine.team":     op.Team,
		"shrine.resource": op.Name,
		"shrine.kind":     op.Kind,
	}

	created, err := backend.client.ContainerCreate(ctx,
		&container.Config{
			Image:  op.Image,
			Env:    op.Env,
			Labels: labels,
		},
		&container.HostConfig{},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				netName: {},
			},
		},
		nil, //platform
		cName,
	)
	if err != nil {
		return fmt.Errorf("creating container %q: %w", cName, err)
	}

	// start
	if err := backend.client.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting container %q: %w", cName, err)
	}

	fmt.Printf("    ✅ Container %s is running\n", cName)

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

func (backend *DockerBackend) ensureImage(ctx context.Context, ref string) error {
	args := filters.NewArgs()
	args.Add("reference", ref)
	existing, err := backend.client.ImageList(ctx, image.ListOptions{Filters: args})
	if err != nil {
		return fmt.Errorf("listing images: %w", err)
	}

	if len(existing) > 0 {
		return nil // already cached
	}

	authB64, err := backend.registryAuthFor(ref)
	if err != nil {
		return err
	}

	// Loading animation for image pull
	stopSpinner := startSpinner(fmt.Sprintf("Pulling image %s...", ref))
	defer stopSpinner()

	reader, err := backend.client.ImagePull(ctx, ref, image.PullOptions{
		RegistryAuth: authB64,
	})
	if err != nil {
		return fmt.Errorf("pulling image %q: %w", ref, err)
	}
	defer reader.Close()

	// ImagePull returns a streaming reader; the pull only happens as we read.
	// Drain the EOF so the pull actually completes.
	if _, err = io.Copy(io.Discard, reader); err != nil {
		return fmt.Errorf("reading image stream for %q: %w", ref, err)
	}
	return nil
}

// startSpinner runs a simple terminal animation in a goroutine.
// Returns a stop function.
func startSpinner(msg string) func() {
	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		// Braille patterns for a "premium" feel
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		i := 0
		for {
			select {
			case <-stop:
				fmt.Print("\r\033[K") // Clear the line
				close(done)
				return
			default:
				fmt.Printf("\r    %s %s", frames[i%len(frames)], msg)
				i++
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	return func() {
		close(stop)
		<-done
	}
}

func networkName(team string) string {
	return fmt.Sprintf("shrine.%s.private", team)
}

func containerName(team string, name string) string {
	return fmt.Sprintf("%s.%s", team, name)
}
