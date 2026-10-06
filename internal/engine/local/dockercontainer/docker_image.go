package dockercontainer

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
)

type localImage struct {
	ID          string
	RepoDigests []string
}

func (backend *DockerBackend) ResolveImage(op engine.ResolveImageOp) (engine.ResolvedImage, error) {
	ctx := context.Background()

	ref, err := expandRegistryAlias(op.Image, backend.registries)
	if err != nil {
		return engine.ResolvedImage{}, backend.emitErr("registry.alias", map[string]string{"ref": op.Image}, err)
	}

	backend.emitStarted("image.resolve", map[string]string{"team": op.Team, "name": op.Name, "ref": ref})

	located, err := backend.locateImage(ctx, ref, op.ImagePullPolicy)
	if err != nil {
		return engine.ResolvedImage{}, err
	}

	resolved := engine.ResolvedImage{
		Ref:     ref,
		Digest:  pickRepoDigest(located.RepoDigests, repositoryOf(ref)),
		ImageID: located.ID,
		Source:  engine.ImageSourceManifest,
	}
	backend.emitFinished("image.resolve", map[string]string{
		"team":   op.Team,
		"name":   op.Name,
		"ref":    ref,
		"digest": resolved.Digest,
		"source": resolved.Source,
	})
	return resolved, nil
}

// locateImage keeps today's pull semantics: any policy but Always reuses a
// matching local image; Always pulls every time. ref must already be fully
// qualified.
func (backend *DockerBackend) locateImage(ctx context.Context, ref string, policy string) (localImage, error) {
	if policy != manifest.ImagePullPolicyAlways {
		local, found, err := backend.findLocalImage(ctx, ref)
		if err != nil || found {
			return local, err
		}
	}

	if err := backend.pullImage(ctx, ref); err != nil {
		return localImage{}, err
	}
	return backend.inspectImage(ctx, ref)
}

func (backend *DockerBackend) findLocalImage(ctx context.Context, ref string) (localImage, bool, error) {
	args := filters.NewArgs()
	args.Add("reference", ref)
	existing, err := backend.client.ImageList(ctx, image.ListOptions{Filters: args})
	if err != nil {
		return localImage{}, false, backend.emitErr("image.list", map[string]string{"ref": ref},
			fmt.Errorf("listing images matching %q: %w", ref, err))
	}
	if len(existing) == 0 {
		return localImage{}, false, nil
	}
	return localImage{ID: existing[0].ID, RepoDigests: existing[0].RepoDigests}, true, nil
}

func (backend *DockerBackend) pullImage(ctx context.Context, ref string) error {
	authB64, err := backend.registryAuthFor(ref)
	if err != nil {
		return backend.emitErr("registry.auth", map[string]string{"ref": ref},
			fmt.Errorf("registry credentials for %q: %w", ref, err))
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

	if _, err = io.Copy(io.Discard, reader); err != nil {
		return backend.emitErr("image.pull", map[string]string{"ref": ref},
			fmt.Errorf("reading image stream for %q: %w", ref, err))
	}

	backend.emitFinished("image.pull", map[string]string{"ref": ref})
	return nil
}

func (backend *DockerBackend) inspectImage(ctx context.Context, ref string) (localImage, error) {
	inspected, err := backend.client.ImageInspect(ctx, ref)
	if err != nil {
		return localImage{}, backend.emitErr("image.inspect", map[string]string{"ref": ref},
			fmt.Errorf("inspecting image %q: %w", ref, err))
	}
	return localImage{ID: inspected.ID, RepoDigests: inspected.RepoDigests}, nil
}

// pickRepoDigest returns the registry digest recorded for repository, or ""
// when the local image carries none for it, as a loaded or locally built
// image does.
func pickRepoDigest(repoDigests []string, repository string) string {
	want := normalizeRepository(repository)
	for _, entry := range repoDigests {
		repo, digest, found := strings.Cut(entry, "@")
		if found && normalizeRepository(repo) == want {
			return digest
		}
	}
	return ""
}

func repositoryOf(ref string) string {
	repository, _, _ := strings.Cut(ref, "@")
	slash := strings.LastIndex(repository, "/")
	if colon := strings.LastIndex(repository, ":"); colon > slash {
		repository = repository[:colon]
	}
	return repository
}

// normalizeRepository drops the Docker Hub prefixes the daemon omits in
// RepoDigests, so an expanded alias compares equal to what Docker records.
func normalizeRepository(repository string) string {
	repository = strings.TrimPrefix(repository, "docker.io/")
	return strings.TrimPrefix(repository, "library/")
}
