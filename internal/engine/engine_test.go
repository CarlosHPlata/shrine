package engine

import (
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/planner"
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

// fakeRoutingBackend records RemoveRoute calls via a shared calls slice.
type fakeRoutingBackend struct {
	calls *[]string
}

func (f *fakeRoutingBackend) WriteRoute(WriteRouteOp) error { return nil }
func (f *fakeRoutingBackend) RemoveRoute(team string, host string) error {
	*f.calls = append(*f.calls, "RemoveRoute:"+team+"/"+host)
	return nil
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
