package dockercontainer

import (
	"context"
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/engine"
)

func (backend *DockerBackend) InspectContainer(containerID string) (engine.ContainerInfo, error) {
	ctx := context.Background()
	resp, err := backend.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return engine.ContainerInfo{}, backend.emitErr("container.inspect",
			map[string]string{"id": containerID},
			fmt.Errorf("inspecting container %q: %w", containerID, err))
	}
	return engine.ContainerInfo{
		Running: resp.State.Running,
		Status:  resp.State.Status,
		ImageID: resp.Image,
	}, nil
}
