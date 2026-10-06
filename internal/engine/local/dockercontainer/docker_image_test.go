package dockercontainer

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// recordingDockerAPI answers the three image calls from canned results and
// records their order, so a test can prove which calls a pull policy makes.
// Every other call keeps fakeDockerAPI's behaviour.
type recordingDockerAPI struct {
	fakeDockerAPI
	calls      []string
	pulledRefs []string
	listed     []image.Summary
	listErr    error
	pullErr    error
	inspected  image.InspectResponse
	inspectErr error
}

func (r *recordingDockerAPI) ImageList(context.Context, image.ListOptions) ([]image.Summary, error) {
	r.calls = append(r.calls, "ImageList")
	return r.listed, r.listErr
}

func (r *recordingDockerAPI) ImagePull(_ context.Context, ref string, _ image.PullOptions) (io.ReadCloser, error) {
	r.calls = append(r.calls, "ImagePull")
	r.pulledRefs = append(r.pulledRefs, ref)
	if r.pullErr != nil {
		return nil, r.pullErr
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (r *recordingDockerAPI) ImageInspect(context.Context, string, ...client.ImageInspectOption) (image.InspectResponse, error) {
	r.calls = append(r.calls, "ImageInspect")
	return r.inspected, r.inspectErr
}

const (
	expandedWhoami = "docker.io/traefik/whoami:latest"
	pulledImageID  = "sha256:pulled-image-id"
	localImageID   = "sha256:local-image-id"
	whoamiDigest   = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func inspectedWhoami() image.InspectResponse {
	return image.InspectResponse{ID: pulledImageID, RepoDigests: []string{"traefik/whoami@" + whoamiDigest}}
}

func resolveTestOp(imageRef, policy string) engine.ResolveImageOp {
	return engine.ResolveImageOp{
		Team:            "team-a",
		Name:            "web",
		Kind:            "Application",
		Image:           imageRef,
		ImagePullPolicy: policy,
	}
}

func resolveWith(t *testing.T, api *recordingDockerAPI, op engine.ResolveImageOp) (engine.ResolvedImage, *recordingObserver) {
	t.Helper()
	obs := &recordingObserver{}
	backend := &DockerBackend{client: api, registries: testRegistries, observer: obs}
	resolved, err := backend.ResolveImage(op)
	if err != nil {
		t.Fatalf("ResolveImage failed: %v", err)
	}
	return resolved, obs
}

func assertCalls(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("docker calls = %v, want %v", got, want)
	}
}

func TestResolveImage_AlwaysPullsThenInspects(t *testing.T) {
	api := &recordingDockerAPI{inspected: inspectedWhoami()}

	resolved, _ := resolveWith(t, api, resolveTestOp(expandedWhoami, "Always"))

	assertCalls(t, api.calls, []string{"ImagePull", "ImageInspect"})
	want := engine.ResolvedImage{Ref: expandedWhoami, Digest: whoamiDigest, ImageID: pulledImageID, Source: engine.ImageSourceManifest}
	if resolved != want {
		t.Errorf("resolved = %+v, want %+v", resolved, want)
	}
}

func TestResolveImage_IfNotPresentReusesLocalImageWithoutRegistryCall(t *testing.T) {
	api := &recordingDockerAPI{listed: []image.Summary{{ID: localImageID, RepoDigests: []string{"traefik/whoami@" + whoamiDigest}}}}

	resolved, _ := resolveWith(t, api, resolveTestOp(expandedWhoami, "IfNotPresent"))

	assertCalls(t, api.calls, []string{"ImageList"})
	if resolved.ImageID != localImageID {
		t.Errorf("ImageID = %q, want the local image %q", resolved.ImageID, localImageID)
	}
	if resolved.Digest != whoamiDigest {
		t.Errorf("Digest = %q, want the digest recorded on the local image %q", resolved.Digest, whoamiDigest)
	}
}

func TestResolveImage_IfNotPresentPullsWhenAbsent(t *testing.T) {
	api := &recordingDockerAPI{inspected: inspectedWhoami()}

	resolved, _ := resolveWith(t, api, resolveTestOp(expandedWhoami, "IfNotPresent"))

	assertCalls(t, api.calls, []string{"ImageList", "ImagePull", "ImageInspect"})
	if resolved.ImageID != pulledImageID {
		t.Errorf("ImageID = %q, want %q", resolved.ImageID, pulledImageID)
	}
}

func TestResolveImage_ExpandsAliasOnceBeforeEveryCall(t *testing.T) {
	api := &recordingDockerAPI{inspected: inspectedWhoami()}

	resolved, obs := resolveWith(t, api, resolveTestOp("reg:myregistry/traefik/whoami:latest", "Always"))

	if resolved.Ref != expandedWhoami {
		t.Errorf("Ref = %q, want the expanded reference %q", resolved.Ref, expandedWhoami)
	}
	if !slices.Equal(api.pulledRefs, []string{expandedWhoami}) {
		t.Errorf("pulled refs = %v, want only the expanded reference", api.pulledRefs)
	}
	started, ok := obs.find("image.resolve", engine.StatusStarted)
	if !ok {
		t.Fatal("no image.resolve started event")
	}
	if started.Fields["ref"] != expandedWhoami {
		t.Errorf("started ref = %q, want %q", started.Fields["ref"], expandedWhoami)
	}
}

func TestResolveImage_EmitsStartedAndFinishedWithArtifactAndVersion(t *testing.T) {
	api := &recordingDockerAPI{inspected: inspectedWhoami()}

	_, obs := resolveWith(t, api, resolveTestOp(expandedWhoami, "Always"))

	started, ok := obs.find("image.resolve", engine.StatusStarted)
	if !ok {
		t.Fatal("no image.resolve started event")
	}
	assertFields(t, started.Fields, map[string]string{"team": "team-a", "name": "web", "ref": expandedWhoami})

	finished, ok := obs.find("image.resolve", engine.StatusFinished)
	if !ok {
		t.Fatal("no image.resolve finished event")
	}
	assertFields(t, finished.Fields, map[string]string{
		"team": "team-a", "name": "web", "ref": expandedWhoami, "digest": whoamiDigest, "source": engine.ImageSourceManifest,
	})
}

func assertFields(t *testing.T, got, want map[string]string) {
	t.Helper()
	for key, value := range want {
		if got[key] != value {
			t.Errorf("field %q = %q, want %q", key, got[key], value)
		}
	}
}

func TestResolveImage_PullFailureNamesReferenceAndEmitsNoFinished(t *testing.T) {
	api := &recordingDockerAPI{pullErr: errors.New("connection refused")}
	obs := &recordingObserver{}
	backend := &DockerBackend{client: api, registries: testRegistries, observer: obs}

	_, err := backend.ResolveImage(resolveTestOp(expandedWhoami, "Always"))
	if err == nil {
		t.Fatal("expected the pull failure to surface")
	}

	want := `pulling image "docker.io/traefik/whoami:latest": connection refused`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if _, ok := obs.find("image.resolve", engine.StatusFinished); ok {
		t.Error("a failed resolution must not emit image.resolve finished")
	}
	if _, ok := obs.find("image.pull", engine.StatusError); !ok {
		t.Error("the failing operation must emit its own error event")
	}
}

func TestResolveImage_InspectFailureNamesReference(t *testing.T) {
	api := &recordingDockerAPI{inspectErr: errors.New("no such image")}
	backend := &DockerBackend{client: api, registries: testRegistries, observer: engine.NoopObserver{}}

	_, err := backend.ResolveImage(resolveTestOp(expandedWhoami, "Always"))
	if err == nil {
		t.Fatal("expected the inspect failure to surface")
	}

	want := `inspecting image "docker.io/traefik/whoami:latest": no such image`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveImage_ListFailureNamesReference(t *testing.T) {
	api := &recordingDockerAPI{listErr: errors.New("daemon down")}
	backend := &DockerBackend{client: api, registries: testRegistries, observer: engine.NoopObserver{}}

	_, err := backend.ResolveImage(resolveTestOp(expandedWhoami, "IfNotPresent"))
	if err == nil {
		t.Fatal("expected the list failure to surface")
	}

	want := `listing images matching "docker.io/traefik/whoami:latest": daemon down`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveImage_NoMatchingRepoDigestYieldsEmptyDigest(t *testing.T) {
	api := &recordingDockerAPI{inspected: image.InspectResponse{ID: pulledImageID, RepoDigests: []string{"other/repo@" + whoamiDigest}}}

	resolved, _ := resolveWith(t, api, resolveTestOp(expandedWhoami, "Always"))

	if resolved.Digest != "" {
		t.Errorf("Digest = %q, want empty for an image without a digest of its own repository", resolved.Digest)
	}
	if resolved.ImageID != pulledImageID {
		t.Errorf("ImageID = %q, want %q", resolved.ImageID, pulledImageID)
	}
}

func TestPickRepoDigest(t *testing.T) {
	cases := []struct {
		name        string
		repoDigests []string
		repository  string
		want        string
	}{
		{"exact repository", []string{"ghcr.io/me/api@sha256:aaa"}, "ghcr.io/me/api", "sha256:aaa"},
		{"docker hub prefix dropped by the daemon", []string{"traefik/whoami@sha256:bbb"}, "docker.io/traefik/whoami", "sha256:bbb"},
		{"official image under library", []string{"postgres@sha256:ccc"}, "docker.io/library/postgres", "sha256:ccc"},
		{"daemon form with prefixes matches bare form", []string{"docker.io/library/postgres@sha256:ddd"}, "postgres", "sha256:ddd"},
		{"other repository skipped", []string{"mirror.local/traefik/whoami@sha256:eee", "traefik/whoami@sha256:fff"}, "traefik/whoami", "sha256:fff"},
		{"no match", []string{"other/repo@sha256:ggg"}, "traefik/whoami", ""},
		{"entry without digest separator", []string{"traefik/whoami"}, "traefik/whoami", ""},
		{"no digests at all", nil, "traefik/whoami", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickRepoDigest(tc.repoDigests, tc.repository); got != tc.want {
				t.Errorf("pickRepoDigest(%v, %q) = %q, want %q", tc.repoDigests, tc.repository, got, tc.want)
			}
		})
	}
}

func TestRepositoryOf(t *testing.T) {
	cases := map[string]string{
		"postgres:16":                      "postgres",
		"traefik/whoami":                   "traefik/whoami",
		"docker.io/traefik/whoami:latest":  "docker.io/traefik/whoami",
		"localhost:5000/shrine/app":        "localhost:5000/shrine/app",
		"localhost:5000/shrine/app:1.2":    "localhost:5000/shrine/app",
		"ghcr.io/me/api@sha256:abc":        "ghcr.io/me/api",
		"ghcr.io/me/api:1.0@sha256:abc":    "ghcr.io/me/api",
		"127.0.0.1:5000/shrine/app:latest": "127.0.0.1:5000/shrine/app",
	}
	for ref, want := range cases {
		t.Run(ref, func(t *testing.T) {
			if got := repositoryOf(ref); got != want {
				t.Errorf("repositoryOf(%q) = %q, want %q", ref, got, want)
			}
		})
	}
}
