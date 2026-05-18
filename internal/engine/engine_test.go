package engine

import (
	"errors"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/planner"
	"github.com/CarlosHPlata/shrine/internal/resolver"
)

// fakeContainerBackend records RemoveContainer calls via a shared calls slice.
type fakeContainerBackend struct {
	calls *[]string
}

func (f *fakeContainerBackend) CreateNetwork(string) error               { return nil }
func (f *fakeContainerBackend) RemoveNetwork(string) error               { return nil }
func (f *fakeContainerBackend) CreateContainer(CreateContainerOp) error  { return nil }
func (f *fakeContainerBackend) CreatePlatformNetwork() error             { return nil }
func (f *fakeContainerBackend) InspectContainer(string) (ContainerInfo, error) {
	return ContainerInfo{}, nil
}
func (f *fakeContainerBackend) RemoveContainer(op RemoveContainerOp) error {
	*f.calls = append(*f.calls, "RemoveContainer:"+op.Team+"/"+op.Name)
	return nil
}

// fakeRoutingBackend records WriteRoute, RemoveRoute and Finalize calls via a
// shared calls slice. finalizeErr is returned from Finalize when non-nil.
type fakeRoutingBackend struct {
	calls       *[]string
	finalizeErr error
}

func (f *fakeRoutingBackend) WriteRoute(op WriteRouteOp) error {
	*f.calls = append(*f.calls, "WriteRoute:"+op.Team+"/"+op.ServiceName)
	return nil
}
func (f *fakeRoutingBackend) RemoveRoute(team string, host string) error {
	*f.calls = append(*f.calls, "RemoveRoute:"+team+"/"+host)
	return nil
}
func (f *fakeRoutingBackend) Finalize() error {
	*f.calls = append(*f.calls, "Finalize")
	return f.finalizeErr
}

// recordingObserver captures events for assertion.
type recordingObserver struct {
	events []Event
}

func (r *recordingObserver) OnEvent(e Event) { r.events = append(r.events, e) }

// stubResolver returns empty resolved values for any resource/application.
type stubResolver struct{}

func (stubResolver) ResolveResource(*manifest.ResourceManifest) (map[string]string, error) {
	return map[string]string{}, nil
}
func (stubResolver) ResolveApplication(*manifest.ApplicationManifest, resolver.ResolvedDependencies) (map[string]string, error) {
	return map[string]string{}, nil
}

// failingContainerBackend returns an error from CreateContainer to simulate a
// step-loop failure that should skip Finalize.
type failingContainerBackend struct {
	fakeContainerBackend
}

func (f *failingContainerBackend) CreateContainer(CreateContainerOp) error {
	return errors.New("create container failed")
}

func TestEngine_TeardownApplication_CallsRoutingRemove(t *testing.T) {
	t.Run("ApplicationKind triggers RemoveRoute", func(t *testing.T) {
		var calls []string
		e := &Engine{
			Container: &fakeContainerBackend{calls: &calls},
			Routing:   &fakeRoutingBackend{calls: &calls},
		}

		steps := []planner.PlannedStep{
			{Kind: manifest.ApplicationKind, Name: "svc-y"},
		}
		err := e.ExecuteTeardown("team-x", steps)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{
			"RemoveContainer:team-x/svc-y",
			"RemoveRoute:team-x/svc-y",
			"Finalize",
		}
		if len(calls) != len(want) {
			t.Fatalf("got calls %v, want %v", calls, want)
		}
		for i, c := range calls {
			if c != want[i] {
				t.Errorf("calls[%d]: got %q, want %q", i, c, want[i])
			}
		}
	})

	t.Run("nil routing backend skips RemoveRoute", func(t *testing.T) {
		var calls []string
		e := &Engine{
			Container: &fakeContainerBackend{calls: &calls},
			Routing:   nil,
		}

		steps := []planner.PlannedStep{
			{Kind: manifest.ApplicationKind, Name: "svc-y"},
		}
		err := e.ExecuteTeardown("team-x", steps)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []string{"RemoveContainer:team-x/svc-y"}
		if len(calls) != len(want) {
			t.Fatalf("got calls %v, want %v", calls, want)
		}
		if calls[0] != want[0] {
			t.Errorf("calls[0]: got %q, want %q", calls[0], want[0])
		}
	})
}

func emptyManifestSet() *planner.ManifestSet {
	return &planner.ManifestSet{
		Applications: map[string]*manifest.ApplicationManifest{},
		Resources:    map[string]*manifest.ResourceManifest{},
	}
}

func TestEngine_ExecuteDeploy_CallsFinalizeOnceAfterStepLoop(t *testing.T) {
	var calls []string
	obs := &recordingObserver{}
	e := &Engine{
		Container: &fakeContainerBackend{calls: &calls},
		Routing:   &fakeRoutingBackend{calls: &calls},
		Resolver:  stubResolver{},
		Observer:  obs,
	}

	if err := e.ExecuteDeploy(nil, emptyManifestSet()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	finalizeCount := 0
	for _, c := range calls {
		if c == "Finalize" {
			finalizeCount++
		}
	}
	if finalizeCount != 1 {
		t.Fatalf("expected Finalize to be invoked exactly once, got %d (calls=%v)", finalizeCount, calls)
	}
	if calls[len(calls)-1] != "Finalize" {
		t.Errorf("expected Finalize to be the last call, got %v", calls)
	}
}

func TestEngine_ExecuteDeploy_StepLoopFailureSkipsFinalize(t *testing.T) {
	var calls []string
	app := &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind},
		Metadata: manifest.Metadata{Name: "svc-a", Owner: "team-a"},
		Spec:     manifest.ApplicationSpec{Image: "img"},
	}
	set := emptyManifestSet()
	set.Applications["svc-a"] = app
	steps := []planner.PlannedStep{{Kind: manifest.ApplicationKind, Name: "svc-a"}}

	routing := &fakeRoutingBackend{calls: &calls}
	e := &Engine{
		Container: &failingContainerBackend{fakeContainerBackend: fakeContainerBackend{calls: &calls}},
		Routing:   routing,
		Resolver:  stubResolver{},
		Observer:  &recordingObserver{},
	}

	err := e.ExecuteDeploy(steps, set)
	if err == nil {
		t.Fatalf("expected an error from failing CreateContainer, got nil")
	}
	for _, c := range calls {
		if c == "Finalize" {
			t.Fatalf("Finalize must NOT be invoked when a step fails; calls=%v", calls)
		}
	}
}

func TestEngine_ExecuteTeardown_CallsFinalizeOnceAfterStepLoop(t *testing.T) {
	var calls []string
	e := &Engine{
		Container: &fakeContainerBackend{calls: &calls},
		Routing:   &fakeRoutingBackend{calls: &calls},
		Resolver:  stubResolver{},
		Observer:  &recordingObserver{},
	}

	steps := []planner.PlannedStep{{Kind: manifest.ApplicationKind, Name: "svc-y"}}
	if err := e.ExecuteTeardown("team-x", steps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	finalizeCount := 0
	for _, c := range calls {
		if c == "Finalize" {
			finalizeCount++
		}
	}
	if finalizeCount != 1 {
		t.Fatalf("expected Finalize to be invoked exactly once during teardown, got %d (calls=%v)", finalizeCount, calls)
	}
}

func TestEngine_ExecuteTeardown_StepLoopFailureSkipsFinalize(t *testing.T) {
	var calls []string
	failingContainer := &teardownFailingContainerBackend{calls: &calls}
	e := &Engine{
		Container: failingContainer,
		Routing:   &fakeRoutingBackend{calls: &calls},
		Resolver:  stubResolver{},
		Observer:  &recordingObserver{},
	}

	steps := []planner.PlannedStep{{Kind: manifest.ApplicationKind, Name: "svc-y"}}
	if err := e.ExecuteTeardown("team-x", steps); err == nil {
		t.Fatalf("expected error from failing RemoveContainer")
	}

	for _, c := range calls {
		if c == "Finalize" {
			t.Fatalf("Finalize must NOT be invoked when a teardown step fails; calls=%v", calls)
		}
	}
}

func TestEngine_ExecuteDeploy_FinalizeErrorWrapsAndEmitsEvent(t *testing.T) {
	var calls []string
	obs := &recordingObserver{}
	routing := &fakeRoutingBackend{calls: &calls, finalizeErr: errors.New("boom")}
	e := &Engine{
		Container: &fakeContainerBackend{calls: &calls},
		Routing:   routing,
		Resolver:  stubResolver{},
		Observer:  obs,
	}

	err := e.ExecuteDeploy(nil, emptyManifestSet())
	if err == nil {
		t.Fatalf("expected wrapped error, got nil")
	}
	if got, want := err.Error(), "routing finalize: boom"; got != want {
		t.Errorf("wrapped error: got %q, want %q", got, want)
	}
	if !errors.Is(err, routing.finalizeErr) {
		t.Errorf("expected errors.Is to match underlying finalize error")
	}

	var errEvent *Event
	for i := range obs.events {
		if obs.events[i].Name == "routing.finalize" && obs.events[i].Status == StatusError {
			errEvent = &obs.events[i]
			break
		}
	}
	if errEvent == nil {
		t.Fatalf("expected a routing.finalize event with status=error in %v", obs.events)
	}
	if got := errEvent.Fields["error"]; got == "" {
		t.Errorf("expected error field on routing.finalize event, fields=%v", errEvent.Fields)
	}
}

func TestEngine_NilRoutingSkipsFinalize(t *testing.T) {
	t.Run("ExecuteDeploy", func(t *testing.T) {
		obs := &recordingObserver{}
		e := &Engine{
			Container: &fakeContainerBackend{calls: new([]string)},
			Routing:   nil,
			Resolver:  stubResolver{},
			Observer:  obs,
		}
		if err := e.ExecuteDeploy(nil, emptyManifestSet()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, ev := range obs.events {
			if ev.Name == "routing.finalize" {
				t.Errorf("expected no routing.finalize event when Routing is nil, got %v", ev)
			}
		}
	})
	t.Run("ExecuteTeardown", func(t *testing.T) {
		obs := &recordingObserver{}
		var calls []string
		e := &Engine{
			Container: &fakeContainerBackend{calls: &calls},
			Routing:   nil,
			Resolver:  stubResolver{},
			Observer:  obs,
		}
		steps := []planner.PlannedStep{{Kind: manifest.ApplicationKind, Name: "svc-y"}}
		if err := e.ExecuteTeardown("team-x", steps); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, ev := range obs.events {
			if ev.Name == "routing.finalize" {
				t.Errorf("expected no routing.finalize event when Routing is nil, got %v", ev)
			}
		}
	})
}

// teardownFailingContainerBackend fails RemoveContainer to simulate a teardown
// step-loop failure that should skip Finalize.
type teardownFailingContainerBackend struct {
	calls *[]string
}

func (f *teardownFailingContainerBackend) CreateNetwork(string) error              { return nil }
func (f *teardownFailingContainerBackend) RemoveNetwork(string) error              { return nil }
func (f *teardownFailingContainerBackend) CreateContainer(CreateContainerOp) error { return nil }
func (f *teardownFailingContainerBackend) CreatePlatformNetwork() error            { return nil }
func (f *teardownFailingContainerBackend) InspectContainer(string) (ContainerInfo, error) {
	return ContainerInfo{}, nil
}
func (f *teardownFailingContainerBackend) RemoveContainer(RemoveContainerOp) error {
	return errors.New("remove container failed")
}
