package dockercontainer

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/containerd/errdefs"
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

// scriptedDockerAPI answers ImageInspect and ImagePull per reference and
// records every call, so a test can prove the pinned branches of ResolveImage
// make exactly the calls the contract allows. A one-shot inspect error is
// consumed on first use, which models "absent locally, then pulled".
type scriptedDockerAPI struct {
	fakeDockerAPI
	calls         []string
	pulledRefs    []string
	inspectedRefs []string
	inspected     map[string]image.InspectResponse
	inspectErrs   map[string]error
	pullErrs      map[string]error
}

func newScriptedDockerAPI() *scriptedDockerAPI {
	return &scriptedDockerAPI{
		inspected:   map[string]image.InspectResponse{},
		inspectErrs: map[string]error{},
		pullErrs:    map[string]error{},
	}
}

func (s *scriptedDockerAPI) ImageList(context.Context, image.ListOptions) ([]image.Summary, error) {
	s.calls = append(s.calls, "ImageList")
	return nil, nil
}

func (s *scriptedDockerAPI) ImagePull(_ context.Context, ref string, _ image.PullOptions) (io.ReadCloser, error) {
	s.calls = append(s.calls, "ImagePull")
	s.pulledRefs = append(s.pulledRefs, ref)
	if err := s.pullErrs[ref]; err != nil {
		return nil, err
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (s *scriptedDockerAPI) ImageInspect(_ context.Context, ref string, _ ...client.ImageInspectOption) (image.InspectResponse, error) {
	s.calls = append(s.calls, "ImageInspect")
	s.inspectedRefs = append(s.inspectedRefs, ref)
	if err, ok := s.inspectErrs[ref]; ok {
		delete(s.inspectErrs, ref)
		return image.InspectResponse{}, err
	}
	resp, ok := s.inspected[ref]
	if !ok {
		return image.InspectResponse{}, errors.New("unexpected ImageInspect of " + ref)
	}
	return resp, nil
}

// fakePinStore records writes and serves pins from memory.
type fakePinStore struct {
	pins       map[string]state.ImagePin
	puts       []state.ImagePin
	releases   []string
	putErr     error
	releaseErr error
}

func newFakePinStore(pins ...state.ImagePin) *fakePinStore {
	s := &fakePinStore{pins: map[string]state.ImagePin{}}
	for _, pin := range pins {
		s.pins[state.ImagePinKey("team-a", pin.Name)] = pin
	}
	return s
}

func (f *fakePinStore) Get(team, name string) (state.ImagePin, error) {
	pin, ok := f.pins[state.ImagePinKey(team, name)]
	if !ok {
		return state.ImagePin{}, state.ErrImagePinNotFound
	}
	return pin, nil
}

func (f *fakePinStore) Put(team string, pin state.ImagePin) error {
	if f.putErr != nil {
		return f.putErr
	}
	f.puts = append(f.puts, pin)
	f.pins[state.ImagePinKey(team, pin.Name)] = pin
	return nil
}

func (f *fakePinStore) Release(team, name string) error {
	if f.releaseErr != nil {
		return f.releaseErr
	}
	f.releases = append(f.releases, state.ImagePinKey(team, name))
	delete(f.pins, state.ImagePinKey(team, name))
	return nil
}

func (f *fakePinStore) ReleaseTeam(string) error                    { return nil }
func (f *fakePinStore) List(string) ([]state.ImagePin, error)       { return nil, nil }
func (f *fakePinStore) ListAll() (map[string]state.ImagePin, error) { return f.pins, nil }

const (
	pinnedRepo = "ghcr.io/me/app"
	pinnedTag  = pinnedRepo + ":latest"
	appDigest  = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	appPinned  = pinnedRepo + "@" + appDigest
)

var fixedNow = time.Date(2026, 10, 6, 10, 42, 17, 0, time.UTC)

func existingPin() state.ImagePin {
	return state.ImagePin{Kind: "Application", Name: "web", Requested: pinnedTag, Pinned: appPinned, PinnedAt: fixedNow.Add(-24 * time.Hour)}
}

func pinnedBackend(api dockerAPI, pins *fakePinStore) (*DockerBackend, *recordingObserver) {
	obs := &recordingObserver{}
	backend := &DockerBackend{
		client:     api,
		registries: testRegistries,
		observer:   obs,
		state:      &state.Store{ImagePins: pins},
		now:        func() time.Time { return fixedNow },
	}
	return backend, obs
}

func TestResolveImage_PinnedFirstDeployPullsNewestAndRecordsThePin(t *testing.T) {
	api := newScriptedDockerAPI()
	api.inspected[pinnedTag] = image.InspectResponse{ID: pulledImageID, RepoDigests: []string{appPinned}}
	pins := newFakePinStore()
	backend, obs := pinnedBackend(api, pins)

	resolved, err := backend.ResolveImage(resolveTestOp(pinnedTag, "Pinned"))
	if err != nil {
		t.Fatalf("ResolveImage failed: %v", err)
	}

	assertCalls(t, api.calls, []string{"ImagePull", "ImageInspect"})
	if !slices.Equal(api.pulledRefs, []string{pinnedTag}) {
		t.Errorf("pulled %v, want the tag reference only", api.pulledRefs)
	}
	wantPin := state.ImagePin{Kind: "Application", Name: "web", Requested: pinnedTag, Pinned: appPinned, PinnedAt: fixedNow}
	if len(pins.puts) != 1 || pins.puts[0] != wantPin {
		t.Errorf("puts = %+v, want exactly %+v", pins.puts, wantPin)
	}
	want := engine.ResolvedImage{Ref: appPinned, Digest: appDigest, ImageID: pulledImageID, Source: engine.ImageSourceResolved, Requested: pinnedTag}
	if resolved != want {
		t.Errorf("resolved = %+v, want %+v", resolved, want)
	}
	finished, ok := obs.find("image.resolve", engine.StatusFinished)
	if !ok {
		t.Fatal("no image.resolve finished event")
	}
	assertFields(t, finished.Fields, map[string]string{"source": "resolved", "requested": pinnedTag, "ref": appPinned, "digest": appDigest})
}

func TestResolveImage_PinnedReusesThePinWithoutRegistryCallWhenPresent(t *testing.T) {
	api := newScriptedDockerAPI()
	api.inspected[appPinned] = image.InspectResponse{ID: localImageID, RepoDigests: []string{appPinned}}
	pins := newFakePinStore(existingPin())
	backend, obs := pinnedBackend(api, pins)

	resolved, err := backend.ResolveImage(resolveTestOp(pinnedTag, "Pinned"))
	if err != nil {
		t.Fatalf("ResolveImage failed: %v", err)
	}

	assertCalls(t, api.calls, []string{"ImageInspect"})
	if len(pins.puts) != 0 {
		t.Errorf("a reused pin must not be rewritten, puts = %+v", pins.puts)
	}
	want := engine.ResolvedImage{Ref: appPinned, Digest: appDigest, ImageID: localImageID, Source: engine.ImageSourcePinned, Requested: pinnedTag, PinnedAt: existingPin().PinnedAt}
	if resolved != want {
		t.Errorf("resolved = %+v, want %+v", resolved, want)
	}
	finished, _ := obs.find("image.resolve", engine.StatusFinished)
	assertFields(t, finished.Fields, map[string]string{"source": "pinned", "requested": pinnedTag, "pinned_at": "2026-10-05"})
}

func TestResolveImage_PinnedPullsByDigestWhenAbsentLocally(t *testing.T) {
	api := newScriptedDockerAPI()
	api.inspectErrs[appPinned] = errdefs.ErrNotFound
	api.inspected[appPinned] = image.InspectResponse{ID: pulledImageID, RepoDigests: []string{appPinned}}
	pins := newFakePinStore(existingPin())
	backend, obs := pinnedBackend(api, pins)

	resolved, err := backend.ResolveImage(resolveTestOp(pinnedTag, "Pinned"))
	if err != nil {
		t.Fatalf("ResolveImage failed: %v", err)
	}

	assertCalls(t, api.calls, []string{"ImageInspect", "ImagePull", "ImageInspect"})
	if !slices.Equal(api.pulledRefs, []string{appPinned}) {
		t.Errorf("pulled %v, want the digest reference, never the tag", api.pulledRefs)
	}
	pullStarted, ok := obs.find("image.pull", engine.StatusStarted)
	if !ok || pullStarted.Fields["ref"] != appPinned {
		t.Errorf("image.pull started must carry the digest reference, got %+v", pullStarted.Fields)
	}
	if resolved.Source != engine.ImageSourcePinned || resolved.ImageID != pulledImageID {
		t.Errorf("resolved = %+v, want a pinned source with the pulled image id", resolved)
	}
}

func TestResolveImage_PinnedWithoutRegistryDigestCannotBePinned(t *testing.T) {
	api := newScriptedDockerAPI()
	api.inspected[pinnedTag] = image.InspectResponse{ID: pulledImageID, RepoDigests: []string{"other/repo@" + appDigest}}
	pins := newFakePinStore()
	backend, _ := pinnedBackend(api, pins)

	_, err := backend.ResolveImage(resolveTestOp(pinnedTag, "Pinned"))
	if err == nil {
		t.Fatal("an image without a registry digest must not be pinned")
	}
	want := `image "ghcr.io/me/app:latest" carries no registry digest and cannot be pinned`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if len(pins.puts) != 0 {
		t.Errorf("no pin may be written on failure, puts = %+v", pins.puts)
	}
}

func TestResolveImage_PinnedReplacesAPinWhoseRepositoryChanged(t *testing.T) {
	api := newScriptedDockerAPI()
	api.inspected[pinnedTag] = image.InspectResponse{ID: pulledImageID, RepoDigests: []string{appPinned}}
	stale := existingPin()
	stale.Requested = "ghcr.io/other/app:latest"
	stale.Pinned = "ghcr.io/other/app@" + appDigest
	pins := newFakePinStore(stale)
	backend, _ := pinnedBackend(api, pins)

	resolved, err := backend.ResolveImage(resolveTestOp(pinnedTag, "Pinned"))
	if err != nil {
		t.Fatalf("ResolveImage failed: %v", err)
	}

	assertCalls(t, api.calls, []string{"ImagePull", "ImageInspect"})
	if len(pins.puts) != 1 || pins.puts[0].Pinned != appPinned {
		t.Errorf("the stale pin must be replaced, puts = %+v", pins.puts)
	}
	if resolved.Source != engine.ImageSourceResolved {
		t.Errorf("Source = %q, want resolved (a first deploy for the new repository)", resolved.Source)
	}
}

func TestResolveImage_PinnedRecordsTheExpandedAliasAsRequested(t *testing.T) {
	api := newScriptedDockerAPI()
	api.inspected["docker.io/app"] = image.InspectResponse{ID: pulledImageID, RepoDigests: []string{"app@" + appDigest}}
	pins := newFakePinStore()
	backend, _ := pinnedBackend(api, pins)

	resolved, err := backend.ResolveImage(resolveTestOp("reg:myregistry/app", "Pinned"))
	if err != nil {
		t.Fatalf("ResolveImage failed: %v", err)
	}

	if len(pins.puts) != 1 || pins.puts[0].Requested != "docker.io/app" || pins.puts[0].Pinned != "docker.io/app@"+appDigest {
		t.Errorf("puts = %+v, want the expanded reference recorded", pins.puts)
	}
	if resolved.Requested != "docker.io/app" {
		t.Errorf("Requested = %q, want the expanded reference", resolved.Requested)
	}
}

func TestResolveImage_ManifestOwnedReleasesAnExistingPin(t *testing.T) {
	api := &recordingDockerAPI{inspected: inspectedWhoami()}
	pins := newFakePinStore(existingPin())
	backend, _ := pinnedBackend(api, pins)

	resolved, err := backend.ResolveImage(resolveTestOp(expandedWhoami, "Always"))
	if err != nil {
		t.Fatalf("ResolveImage failed: %v", err)
	}

	if !slices.Equal(pins.releases, []string{"team-a/web"}) {
		t.Errorf("releases = %v, want the artifact's pin released once", pins.releases)
	}
	if resolved.Source != engine.ImageSourceManifest {
		t.Errorf("Source = %q, want manifest", resolved.Source)
	}
}

func TestResolveImage_ManifestOwnedReleaseIsIdempotentWithoutAPin(t *testing.T) {
	api := &recordingDockerAPI{listed: []image.Summary{{ID: localImageID}}}
	pins := newFakePinStore()
	backend, _ := pinnedBackend(api, pins)

	if _, err := backend.ResolveImage(resolveTestOp(expandedWhoami, "IfNotPresent")); err != nil {
		t.Fatalf("ResolveImage failed: %v", err)
	}
	if !slices.Equal(pins.releases, []string{"team-a/web"}) {
		t.Errorf("releases = %v, want one idempotent release", pins.releases)
	}
}

func TestResolveImage_ManifestOwnedReleaseFailureSurfaces(t *testing.T) {
	api := &recordingDockerAPI{inspected: inspectedWhoami()}
	pins := newFakePinStore(existingPin())
	pins.releaseErr = errors.New("disk full")
	backend, _ := pinnedBackend(api, pins)

	_, err := backend.ResolveImage(resolveTestOp(expandedWhoami, "Always"))
	if err == nil {
		t.Fatal("a failed release must surface")
	}
	want := "releasing image pin for team-a/web: disk full"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestResolveImage_ManifestOwnedWithoutAPinStoreReleasesNothing(t *testing.T) {
	api := &recordingDockerAPI{inspected: inspectedWhoami()}
	backend := &DockerBackend{client: api, registries: testRegistries, observer: engine.NoopObserver{}}

	if _, err := backend.ResolveImage(resolveTestOp(expandedWhoami, "Always")); err != nil {
		t.Fatalf("ResolveImage must tolerate a nil store, got %v", err)
	}
}
