package dockercontainer

import (
	"context"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// dockerAPI is the test seam over the Docker SDK client: it exists so unit
// tests can observe the exact container specification handed to Docker
// (spec 022 VR-001/VR-002) without a live daemon. *client.Client satisfies it
// structurally; production behaviour is unchanged.
type dockerAPI interface {
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ImageInspect(ctx context.Context, imageID string, inspectOpts ...client.ImageInspectOption) (image.InspectResponse, error)
	ImageList(ctx context.Context, options image.ListOptions) ([]image.Summary, error)
	ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error)
	NetworkCreate(ctx context.Context, name string, options network.CreateOptions) (network.CreateResponse, error)
	NetworkInspect(ctx context.Context, networkID string, options network.InspectOptions) (network.Inspect, error)
	NetworkRemove(ctx context.Context, networkID string) error
	VolumeCreate(ctx context.Context, options volume.CreateOptions) (volume.Volume, error)
	VolumeInspect(ctx context.Context, volumeID string) (volume.Volume, error)
}
