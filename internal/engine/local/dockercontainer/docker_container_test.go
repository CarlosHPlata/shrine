package dockercontainer

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// fakeDockerAPI drives CreateContainer down the fresh-create path and stops it
// at the point of creation: ImageList resolves the digest without a pull,
// ContainerInspect reports no existing container, and ContainerCreate captures
// the container spec then fails — so the flow never reaches recordDeployment
// and the test needs no state store and touches no filesystem.
type fakeDockerAPI struct {
	createdConfig *container.Config
	createdHost   *container.HostConfig
	createErr     error
}

func (f *fakeDockerAPI) ContainerCreate(_ context.Context, cfg *container.Config, host *container.HostConfig, _ *network.NetworkingConfig, _ *ocispec.Platform, _ string) (container.CreateResponse, error) {
	f.createdConfig = cfg
	f.createdHost = host
	return container.CreateResponse{}, f.createErr
}

func (f *fakeDockerAPI) ContainerInspect(context.Context, string) (container.InspectResponse, error) {
	return container.InspectResponse{}, errdefs.ErrNotFound
}

func (f *fakeDockerAPI) ImageList(context.Context, image.ListOptions) ([]image.Summary, error) {
	return []image.Summary{{ID: "sha256:test-digest"}}, nil
}

func (f *fakeDockerAPI) ContainerRemove(context.Context, string, container.RemoveOptions) error {
	panic("unexpected ContainerRemove")
}

func (f *fakeDockerAPI) ContainerStart(context.Context, string, container.StartOptions) error {
	panic("unexpected ContainerStart")
}

func (f *fakeDockerAPI) ImageInspect(context.Context, string, ...client.ImageInspectOption) (image.InspectResponse, error) {
	panic("unexpected ImageInspect")
}

func (f *fakeDockerAPI) ImagePull(context.Context, string, image.PullOptions) (io.ReadCloser, error) {
	panic("unexpected ImagePull")
}

func (f *fakeDockerAPI) NetworkCreate(context.Context, string, network.CreateOptions) (network.CreateResponse, error) {
	panic("unexpected NetworkCreate")
}

func (f *fakeDockerAPI) NetworkInspect(context.Context, string, network.InspectOptions) (network.Inspect, error) {
	panic("unexpected NetworkInspect")
}

func (f *fakeDockerAPI) NetworkRemove(context.Context, string) error {
	panic("unexpected NetworkRemove")
}

func (f *fakeDockerAPI) VolumeCreate(context.Context, volume.CreateOptions) (volume.Volume, error) {
	panic("unexpected VolumeCreate")
}

func (f *fakeDockerAPI) VolumeInspect(context.Context, string) (volume.Volume, error) {
	panic("unexpected VolumeInspect")
}

type recordingObserver struct {
	events []engine.Event
}

func (r *recordingObserver) OnEvent(e engine.Event) {
	r.events = append(r.events, e)
}

func (r *recordingObserver) find(name string, status engine.EventStatus) (engine.Event, bool) {
	for _, e := range r.events {
		if e.Name == name && e.Status == status {
			return e, true
		}
	}
	return engine.Event{}, false
}

var testRegistries = []config.RegistryConfig{{Host: "docker.io", Alias: "myregistry"}}

func aliasTestOp(imageRef string) engine.CreateContainerOp {
	return engine.CreateContainerOp{
		Team:            "shrine-deploy-test",
		Name:            "alias-app",
		Kind:            "Application",
		Image:           imageRef,
		ImagePullPolicy: "IfNotPresent",
	}
}

// captureCreatedImage runs a fresh creation for imageRef and returns the image
// reference that reached ContainerCreate — the operator-observable outcome the
// spec's VR-002 demands tests assert on.
func captureCreatedImage(t *testing.T, imageRef string) string {
	t.Helper()
	fake := &fakeDockerAPI{createErr: errors.New("create rejected by fake")}
	backend := &DockerBackend{
		client:     fake,
		registries: testRegistries,
		observer:   engine.NoopObserver{},
	}

	if err := backend.CreateContainer(aliasTestOp(imageRef)); err == nil {
		t.Fatal("expected CreateContainer to surface the fake's creation error")
	}
	if fake.createdConfig == nil {
		t.Fatal("ContainerCreate was never called")
	}
	return fake.createdConfig.Image
}

func TestCreateContainer_ExpandsAliasIntoContainerSpec(t *testing.T) {
	got := captureCreatedImage(t, "reg:myregistry/traefik/whoami:latest")

	if want := "docker.io/traefik/whoami:latest"; got != want {
		t.Errorf("container spec image = %q, want %q", got, want)
	}
	if strings.HasPrefix(got, "reg:") {
		t.Errorf("container spec image %q still carries the reg: alias prefix", got)
	}
}

func TestCreateContainer_BareAliasExpandsToHost(t *testing.T) {
	got := captureCreatedImage(t, "reg:myregistry")

	if want := "docker.io"; got != want {
		t.Errorf("container spec image = %q, want %q", got, want)
	}
}

func TestCreateContainer_PlainReferencePassesThroughUnchanged(t *testing.T) {
	got := captureCreatedImage(t, "nginx:latest")

	if want := "nginx:latest"; got != want {
		t.Errorf("container spec image = %q, want %q", got, want)
	}
}

func TestCreateContainer_CreateFailureNamesImage(t *testing.T) {
	fake := &fakeDockerAPI{createErr: errors.New("invalid reference format")}
	obs := &recordingObserver{}
	backend := &DockerBackend{
		client:     fake,
		registries: testRegistries,
		observer:   obs,
	}

	if err := backend.CreateContainer(aliasTestOp("reg:myregistry/traefik/whoami:latest")); err == nil {
		t.Fatal("expected CreateContainer to surface the creation error")
	}

	event, ok := obs.find("container.create", engine.StatusError)
	if !ok {
		t.Fatal("no container.create error event was emitted")
	}
	if got, want := event.Fields["image"], "docker.io/traefik/whoami:latest"; got != want {
		t.Errorf("error event image field = %q, want %q", got, want)
	}
	if got, want := event.Fields["name"], "shrine-deploy-test.alias-app"; got != want {
		t.Errorf("error event name field = %q, want %q", got, want)
	}
	want := `creating container "shrine-deploy-test.alias-app" (image "docker.io/traefik/whoami:latest"): invalid reference format`
	if got := event.Fields["error"]; got != want {
		t.Errorf("error event text:\ngot  %q\nwant %q", got, want)
	}
}
