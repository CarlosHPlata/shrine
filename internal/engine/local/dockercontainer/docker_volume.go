package dockercontainer

import (
	"context"
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/volume"
)

func (backend *DockerBackend) ensureVolume(ctx context.Context, op engine.CreateContainerOp, v engine.VolumeMount) error {
	name := volumeName(op.Team, op.Name, v.Name)

	_, err := backend.client.VolumeInspect(ctx, name)
	if err == nil {
		return nil
	}

	if !errdefs.IsNotFound(err) {
		return backend.emitErr("volume.inspect", map[string]string{"name": name},
			fmt.Errorf("inspecting volume %q: %w", name, err))
	}

	backend.emitInfo("volume.create", map[string]string{"name": name, "mount": v.MountPath})

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

	backend.emitFinished("volume.created", map[string]string{"name": name, "mount": v.MountPath})
	return nil
}

func volumeName(team string, res string, name string) string {
	return fmt.Sprintf("shrine.%s.%s.%s", team, res, name)
}
