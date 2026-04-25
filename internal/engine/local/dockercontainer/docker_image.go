package dockercontainer

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
)

func (backend *DockerBackend) ensureImage(ctx context.Context, ref string) error {
	args := filters.NewArgs()
	args.Add("reference", ref)
	existing, err := backend.client.ImageList(ctx, image.ListOptions{Filters: args})
	if err != nil {
		return backend.emitErr("image.list", map[string]string{"ref": ref},
			fmt.Errorf("listing images: %w", err))
	}

	if len(existing) > 0 {
		return nil
	}

	authB64, err := backend.registryAuthFor(ref)
	if err != nil {
		return backend.emitErr("registry.auth", map[string]string{"ref": ref}, err)
	}

	backend.emitStarted("image.pull", map[string]string{"ref": ref})

	reader, err := backend.client.ImagePull(ctx, ref, image.PullOptions{
		RegistryAuth: authB64,
	})
	if err != nil {
		return backend.emitErr("image.pull", map[string]string{"ref": ref},
			fmt.Errorf("pulling image %q: %w", ref, err))
	}
	defer reader.Close()

	// ImagePull returns a streaming reader; drain it so the pull actually completes.
	if _, err = io.Copy(io.Discard, reader); err != nil {
		return backend.emitErr("image.pull", map[string]string{"ref": ref},
			fmt.Errorf("reading image stream for %q: %w", ref, err))
	}

	backend.emitFinished("image.pull", map[string]string{"ref": ref})
	return nil
}
