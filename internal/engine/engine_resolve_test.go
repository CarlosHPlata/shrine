package engine

import (
	"errors"
	"slices"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/planner"
)

// prepassContainerBackend records every container call on a shared timeline
// and answers ResolveImage with a distinct reference and image id per op, or
// with failErr for the artifact named failOn.
type prepassContainerBackend struct {
	calls      *[]string
	resolveOps []ResolveImageOp
	createOps  []CreateContainerOp
	failOn     string
	failErr    error
}

func (p *prepassContainerBackend) record(call string) { *p.calls = append(*p.calls, call) }

func (p *prepassContainerBackend) CreateNetwork(name string) error {
	p.record("CreateNetwork:" + name)
	return nil
}
func (p *prepassContainerBackend) RemoveNetwork(string) error { return nil }
func (p *prepassContainerBackend) CreatePlatformNetwork() error {
	p.record("CreatePlatformNetwork")
	return nil
}
func (p *prepassContainerBackend) InspectContainer(string) (ContainerInfo, error) {
	return ContainerInfo{}, nil
}
func (p *prepassContainerBackend) RemoveContainer(RemoveContainerOp) error { return nil }
func (p *prepassContainerBackend) CreateContainer(op CreateContainerOp) error {
	p.record("CreateContainer:" + op.Name)
	p.createOps = append(p.createOps, op)
	return nil
}
func (p *prepassContainerBackend) ResolveImage(op ResolveImageOp) (ResolvedImage, error) {
	p.record("ResolveImage:" + op.Name)
	p.resolveOps = append(p.resolveOps, op)
	if op.Name == p.failOn {
		return ResolvedImage{}, p.failErr
	}
	return ResolvedImage{
		Ref:     "resolved/" + op.Image,
		ImageID: "sha256:" + op.Name,
		Source:  ImageSourceManifest,
	}, nil
}

// prepassManifestSet holds a fixed-tag resource and an untagged application so
// both effective pull policies appear in the ops the engine builds.
func prepassManifestSet() (*planner.ManifestSet, []planner.PlannedStep) {
	set := emptyManifestSet()
	set.Resources["db"] = &manifest.ResourceManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ResourceKind},
		Metadata: manifest.Metadata{Name: "db", Owner: "team-a"},
		Spec:     manifest.ResourceSpec{Type: "postgres", Image: "postgres:16"},
	}
	set.Applications["svc-b"] = &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind},
		Metadata: manifest.Metadata{Name: "svc-b", Owner: "team-a"},
		Spec:     manifest.ApplicationSpec{Image: "img", Port: 8080},
	}
	steps := []planner.PlannedStep{
		{Kind: manifest.ResourceKind, Name: "db"},
		{Kind: manifest.ApplicationKind, Name: "svc-b"},
	}
	return set, steps
}

func runPrepassDeploy(t *testing.T, backend *prepassContainerBackend) (error, *recordingObserver) {
	t.Helper()
	obs := &recordingObserver{}
	e := &Engine{
		Container: backend,
		Routing:   &fakeRoutingBackend{calls: backend.calls},
		Resolver:  stubResolver{},
		Observer:  obs,
	}
	set, steps := prepassManifestSet()
	return e.ExecuteDeploy(steps, set), obs
}

func TestExecuteDeploy_ResolvesEveryImageInStepOrderBeforeAnyOperation(t *testing.T) {
	var calls []string
	backend := &prepassContainerBackend{calls: &calls}

	err, _ := runPrepassDeploy(t, backend)
	if err != nil {
		t.Fatalf("ExecuteDeploy failed: %v", err)
	}

	wantPrefix := []string{"ResolveImage:db", "ResolveImage:svc-b", "CreatePlatformNetwork"}
	if len(calls) < len(wantPrefix) || !slices.Equal(calls[:len(wantPrefix)], wantPrefix) {
		t.Errorf("timeline = %v, want it to start with %v", calls, wantPrefix)
	}
}

func TestExecuteDeploy_BuildsResolveImageOpFromTheManifest(t *testing.T) {
	var calls []string
	backend := &prepassContainerBackend{calls: &calls}

	if err, _ := runPrepassDeploy(t, backend); err != nil {
		t.Fatalf("ExecuteDeploy failed: %v", err)
	}

	want := []ResolveImageOp{
		{Team: "team-a", Name: "db", Kind: manifest.ResourceKind, Image: "postgres:16", ImagePullPolicy: manifest.ImagePullPolicyIfNotPresent},
		{Team: "team-a", Name: "svc-b", Kind: manifest.ApplicationKind, Image: "img", ImagePullPolicy: manifest.ImagePullPolicyAlways},
	}
	if !slices.Equal(backend.resolveOps, want) {
		t.Errorf("resolve ops = %+v, want %+v", backend.resolveOps, want)
	}
}

func TestExecuteDeploy_HandsTheResolvedImageToCreateContainer(t *testing.T) {
	var calls []string
	backend := &prepassContainerBackend{calls: &calls}

	if err, _ := runPrepassDeploy(t, backend); err != nil {
		t.Fatalf("ExecuteDeploy failed: %v", err)
	}

	if len(backend.createOps) != 2 {
		t.Fatalf("got %d CreateContainer ops, want 2", len(backend.createOps))
	}
	cases := []struct{ name, image, imageID string }{
		{"db", "resolved/postgres:16", "sha256:db"},
		{"svc-b", "resolved/img", "sha256:svc-b"},
	}
	for i, tc := range cases {
		op := backend.createOps[i]
		if op.Name != tc.name || op.Image != tc.image || op.ImageID != tc.imageID {
			t.Errorf("op[%d] = {Name:%q Image:%q ImageID:%q}, want {%q %q %q}", i, op.Name, op.Image, op.ImageID, tc.name, tc.image, tc.imageID)
		}
	}
}

func TestExecuteDeploy_FirstResolutionFailureAbortsBeforeAnyOperation(t *testing.T) {
	var calls []string
	backend := &prepassContainerBackend{calls: &calls, failOn: "svc-b", failErr: errors.New(`pulling image "img": boom`)}

	err, obs := runPrepassDeploy(t, backend)
	if err == nil {
		t.Fatal("expected the resolution failure to abort the deploy")
	}

	if want := `application "svc-b": pulling image "img": boom`; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if want := []string{"ResolveImage:db", "ResolveImage:svc-b"}; !slices.Equal(calls, want) {
		t.Errorf("timeline = %v, want exactly %v (no network, container, or finalize call)", calls, want)
	}

	var errorEvents []Event
	for _, e := range obs.events {
		if e.Name == "image.resolve" && e.Status == StatusError {
			errorEvents = append(errorEvents, e)
		}
	}
	if len(errorEvents) != 1 {
		t.Fatalf("got %d image.resolve error events, want 1", len(errorEvents))
	}
	fields := errorEvents[0].Fields
	if fields["team"] != "team-a" || fields["name"] != "svc-b" || fields["ref"] != "img" {
		t.Errorf("error event fields = %v, want team=team-a name=svc-b ref=img", fields)
	}
}

func TestExecuteDeploy_ResourceResolutionFailureNamesTheResource(t *testing.T) {
	var calls []string
	backend := &prepassContainerBackend{calls: &calls, failOn: "db", failErr: errors.New("boom")}

	err, _ := runPrepassDeploy(t, backend)
	if err == nil {
		t.Fatal("expected the resolution failure to abort the deploy")
	}

	if want := `resource "db": boom`; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if want := []string{"ResolveImage:db"}; !slices.Equal(calls, want) {
		t.Errorf("timeline = %v, want exactly %v", calls, want)
	}
}
