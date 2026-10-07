package dockercontainer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/containerd/errdefs"
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
	if op.Repin != "" {
		return backend.repin(ctx, op)
	}

	backend.emitStarted("image.resolve", map[string]string{"team": op.Team, "name": op.Name, "ref": ref})

	var resolved engine.ResolvedImage
	if manifest.IsManifestOwnedPolicy(op.ImagePullPolicy) {
		resolved, err = backend.resolveManifestOwned(ctx, op, ref)
	} else {
		resolved, err = backend.resolvePinned(ctx, op, ref)
	}
	if err != nil {
		return engine.ResolvedImage{}, err
	}

	backend.emitFinished("image.resolve", resolvedImageFields(op, resolved))
	return resolved, nil
}

// repin pins the exact version op.Repin resolves to in place of the
// manifest reference. Bump is its only caller; it lives here so the backend
// stays the one writer of pins (TD-8).
func (backend *DockerBackend) repin(ctx context.Context, op engine.ResolveImageOp) (engine.ResolvedImage, error) {
	if op.ImagePullPolicy != manifest.ImagePullPolicyPinned {
		return engine.ResolvedImage{}, fmt.Errorf("repin of %s/%s requires the Pinned policy, got %s", op.Team, op.Name, op.ImagePullPolicy)
	}
	target, err := expandRegistryAlias(op.Repin, backend.registries)
	if err != nil {
		return engine.ResolvedImage{}, backend.emitErr("registry.alias", map[string]string{"ref": op.Repin}, err)
	}

	backend.emitStarted("image.resolve", map[string]string{"team": op.Team, "name": op.Name, "ref": target})

	resolved, err := backend.pinReference(ctx, op, target, engine.ImageSourceRepinned)
	if err != nil {
		return engine.ResolvedImage{}, err
	}
	backend.emitFinished("image.resolve", resolvedImageFields(op, resolved))
	return resolved, nil
}

func resolvedImageFields(op engine.ResolveImageOp, resolved engine.ResolvedImage) map[string]string {
	fields := map[string]string{
		"team":   op.Team,
		"name":   op.Name,
		"ref":    resolved.Ref,
		"digest": resolved.Digest,
		"source": resolved.Source,
	}
	if resolved.Requested != "" {
		fields["requested"] = resolved.Requested
	}
	if resolved.Source == engine.ImageSourcePinned {
		fields["pinned_at"] = resolved.PinnedAt.UTC().Format(time.DateOnly)
	}
	return fields
}

func (backend *DockerBackend) resolveManifestOwned(ctx context.Context, op engine.ResolveImageOp, ref string) (engine.ResolvedImage, error) {
	located, err := backend.locateImage(ctx, ref, op.ImagePullPolicy)
	if err != nil {
		return engine.ResolvedImage{}, err
	}
	if err := backend.releasePin(op); err != nil {
		return engine.ResolvedImage{}, err
	}
	return engine.ResolvedImage{
		Ref:     ref,
		Digest:  pickRepoDigest(located.RepoDigests, manifest.RepositoryOf(ref)),
		ImageID: located.ID,
		Source:  engine.ImageSourceManifest,
	}, nil
}

func (backend *DockerBackend) resolvePinned(ctx context.Context, op engine.ResolveImageOp, ref string) (engine.ResolvedImage, error) {
	pin, found, err := backend.usablePin(op, ref)
	if err != nil {
		return engine.ResolvedImage{}, err
	}
	if found {
		return backend.reusePin(ctx, op, pin)
	}
	return backend.pinReference(ctx, op, ref, engine.ImageSourceResolved)
}

// usablePin returns the artifact's pin when it is for the repository the
// manifest names now; a pin for another repository is meaningless and is
// treated as absent so the next deploy pins afresh (spec 033 FR-011).
func (backend *DockerBackend) usablePin(op engine.ResolveImageOp, ref string) (state.ImagePin, bool, error) {
	pins, ok := backend.pinStore()
	if !ok {
		return state.ImagePin{}, false, nil
	}
	pin, err := pins.Get(op.Team, op.Name)
	if errors.Is(err, state.ErrImagePinNotFound) {
		return state.ImagePin{}, false, nil
	}
	if err != nil {
		return state.ImagePin{}, false, fmt.Errorf("reading image pin for %s/%s: %w", op.Team, op.Name, err)
	}
	return pin, sameRepository(pin.Requested, ref), nil
}

// reusePin runs the pinned exact version, fetching it by digest only when
// the host no longer has it; the tag is never consulted (R-13).
func (backend *DockerBackend) reusePin(ctx context.Context, op engine.ResolveImageOp, pin state.ImagePin) (engine.ResolvedImage, error) {
	local, present, err := backend.findImageByReference(ctx, pin.Pinned)
	if err != nil {
		return engine.ResolvedImage{}, err
	}
	if !present {
		if err := backend.pullImage(ctx, pin.Pinned); err != nil {
			return engine.ResolvedImage{}, backend.notServedError(op, pin, err)
		}
		if local, err = backend.inspectImage(ctx, pin.Pinned); err != nil {
			return engine.ResolvedImage{}, err
		}
	}
	_, digest, _ := strings.Cut(pin.Pinned, "@")
	return engine.ResolvedImage{
		Ref:       pin.Pinned,
		Digest:    digest,
		ImageID:   local.ID,
		Source:    engine.ImageSourcePinned,
		Requested: pin.Requested,
		PinnedAt:  pin.PinnedAt,
	}, nil
}

// pinReference records the exact version ref resolves to: a first deploy
// under Pinned passes the newest, a bump passes the chosen reference.
func (backend *DockerBackend) pinReference(ctx context.Context, op engine.ResolveImageOp, ref, source string) (engine.ResolvedImage, error) {
	if err := backend.pullImage(ctx, ref); err != nil {
		return engine.ResolvedImage{}, err
	}
	local, err := backend.inspectImage(ctx, ref)
	if err != nil {
		return engine.ResolvedImage{}, err
	}
	digest := pickRepoDigest(local.RepoDigests, manifest.RepositoryOf(ref))
	if digest == "" {
		return engine.ResolvedImage{}, backend.emitErr("image.resolve", resolveErrorFields(op, ref),
			fmt.Errorf("image %q carries no registry digest and cannot be pinned", ref))
	}

	pin := state.ImagePin{
		Kind:      op.Kind,
		Name:      op.Name,
		Requested: ref,
		Pinned:    pinnedReference(ref, digest),
		PinnedAt:  backend.clock(),
	}
	pins, ok := backend.pinStore()
	if !ok {
		return engine.ResolvedImage{}, fmt.Errorf("recording image pin for %s/%s: no image pin store", op.Team, op.Name)
	}
	if err := pins.Put(op.Team, pin); err != nil {
		return engine.ResolvedImage{}, fmt.Errorf("recording image pin for %s/%s: %w", op.Team, op.Name, err)
	}
	return engine.ResolvedImage{
		Ref:       pin.Pinned,
		Digest:    digest,
		ImageID:   local.ID,
		Source:    source,
		Requested: ref,
	}, nil
}

// releasePin makes the artifact manifest-owned again: a pin belongs to the
// pinned policy, and a stale one would surprise on return (design TD-6).
func (backend *DockerBackend) releasePin(op engine.ResolveImageOp) error {
	pins, ok := backend.pinStore()
	if !ok {
		return nil
	}
	if err := pins.Release(op.Team, op.Name); err != nil {
		return fmt.Errorf("releasing image pin for %s/%s: %w", op.Team, op.Name, err)
	}
	return nil
}

// notServedError is the pinned failure of R-14: neither the registry nor the
// host has the exact version, so the pin cannot be honoured and only a bump
// can choose another exact version. The kind is lower-cased to name the
// bump subcommand.
func (backend *DockerBackend) notServedError(op engine.ResolveImageOp, pin state.ImagePin, cause error) error {
	return backend.emitErr("image.resolve", resolveErrorFields(op, pin.Pinned),
		fmt.Errorf("pinned exact version %q for %s/%s is no longer served by the registry; run \"shrine bump %s %s\" to choose another version: %w",
			pin.Pinned, op.Team, op.Name, strings.ToLower(op.Kind), op.Name, cause))
}

func resolveErrorFields(op engine.ResolveImageOp, ref string) map[string]string {
	return map[string]string{"team": op.Team, "name": op.Name, "ref": ref}
}

func (backend *DockerBackend) pinStore() (state.ImagePinStore, bool) {
	if backend.state == nil || backend.state.ImagePins == nil {
		return nil, false
	}
	return backend.state.ImagePins, true
}

// findImageByReference is the local presence check for a digest reference;
// ImageList's reference filter does not match repo@digest, ImageInspect does.
func (backend *DockerBackend) findImageByReference(ctx context.Context, ref string) (localImage, bool, error) {
	inspected, err := backend.client.ImageInspect(ctx, ref)
	if errdefs.IsNotFound(err) {
		return localImage{}, false, nil
	}
	if err != nil {
		return localImage{}, false, backend.emitErr("image.inspect", map[string]string{"ref": ref},
			fmt.Errorf("inspecting image %q: %w", ref, err))
	}
	return localImage{ID: inspected.ID, RepoDigests: inspected.RepoDigests}, true, nil
}

func pinnedReference(ref, digest string) string {
	return manifest.RepositoryOf(ref) + "@" + digest
}

func sameRepository(a, b string) bool {
	return normalizeRepository(manifest.RepositoryOf(a)) == normalizeRepository(manifest.RepositoryOf(b))
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

// normalizeRepository drops the Docker Hub prefixes the daemon omits in
// RepoDigests, so an expanded alias compares equal to what Docker records.
func normalizeRepository(repository string) string {
	repository = strings.TrimPrefix(repository, "docker.io/")
	return strings.TrimPrefix(repository, "library/")
}
