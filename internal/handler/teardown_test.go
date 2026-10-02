package handler

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/app"
	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

const teardownTeam = "team-a"

// recordingContainerBackend records the operations the engine sends it, in
// call order.
type recordingContainerBackend struct {
	stubContainerBackend
	removed   []engine.RemoveContainerOp
	networks  []string
	removeErr error
}

func (r *recordingContainerBackend) RemoveContainer(op engine.RemoveContainerOp) error {
	r.removed = append(r.removed, op)
	return r.removeErr
}

func (r *recordingContainerBackend) RemoveNetwork(team string) error {
	r.networks = append(r.networks, team)
	return nil
}

type recordingObserver struct{ events []engine.Event }

func (r *recordingObserver) OnEvent(e engine.Event) { r.events = append(r.events, e) }

func (r *recordingObserver) saw(name string, status engine.EventStatus, deployment string) bool {
	return slices.ContainsFunc(r.events, func(e engine.Event) bool {
		return e.Name == name && e.Status == status &&
			e.Fields["team"] == teardownTeam && e.Fields["name"] == deployment
	})
}

type teardownStandIns struct {
	deployments *memDeploymentStore
	backend     *recordingContainerBackend
	observer    *recordingObserver
}

// newTeardownStandIns lists a resource before an application so the test can
// tell the planner's ordering from the store's.
func newTeardownStandIns() teardownStandIns {
	return teardownStandIns{
		deployments: &memDeploymentStore{byTeam: map[string][]state.Deployment{
			teardownTeam: {
				{Kind: manifest.ResourceKind, Name: "db"},
				{Kind: manifest.ApplicationKind, Name: "web"},
			},
		}},
		backend:  &recordingContainerBackend{},
		observer: &recordingObserver{},
	}
}

// bundle is assembled by hand: no composition-root, engine, or plugin
// constructor runs, so the handler is exercised in isolation.
func (s teardownStandIns) bundle() *app.TeardownBundle {
	return &app.TeardownBundle{
		Out:    &bytes.Buffer{},
		Cfg:    &config.Config{},
		Store:  &state.Store{Deployments: s.deployments},
		Engine: &engine.Engine{Container: s.backend, Observer: s.observer},
	}
}

func TestTeardown_RemovesPlannedDeploymentsThenNetwork(t *testing.T) {
	standIns := newTeardownStandIns()

	if err := Teardown(standIns.bundle(), teardownTeam); err != nil {
		t.Fatalf("Teardown failed: %v", err)
	}

	wantRemoved := []engine.RemoveContainerOp{
		{Team: teardownTeam, Name: "web"},
		{Team: teardownTeam, Name: "db"},
	}
	if !slices.Equal(standIns.backend.removed, wantRemoved) {
		t.Errorf("removals = %v, want the application before the resource: %v", standIns.backend.removed, wantRemoved)
	}
	if !slices.Equal(standIns.backend.networks, []string{teardownTeam}) {
		t.Errorf("networks removed = %v, want exactly the team's", standIns.backend.networks)
	}
	// Teardown events are named after the manifest kind as it was recorded.
	if !standIns.observer.saw(manifest.ApplicationKind+".teardown", engine.StatusStarted, "web") {
		t.Errorf("observer did not see the application teardown start, got %v", standIns.observer.events)
	}
	if !standIns.observer.saw(manifest.ResourceKind+".teardown", engine.StatusStarted, "db") {
		t.Errorf("observer did not see the resource teardown start, got %v", standIns.observer.events)
	}
}

func TestTeardown_ReturnsListErrorWithoutTouchingBackend(t *testing.T) {
	errList := errors.New("deployments unreadable")
	standIns := newTeardownStandIns()
	standIns.deployments.listErr = errList

	err := Teardown(standIns.bundle(), teardownTeam)

	if !errors.Is(err, errList) {
		t.Errorf("Teardown returned %v, want the listing error", err)
	}
	if len(standIns.backend.removed) != 0 || len(standIns.backend.networks) != 0 {
		t.Errorf("backend was touched after a listing failure: removed=%v networks=%v",
			standIns.backend.removed, standIns.backend.networks)
	}
}

func TestTeardown_StopsAtFirstRemovalFailure(t *testing.T) {
	errRemove := errors.New("daemon unreachable")
	standIns := newTeardownStandIns()
	standIns.backend.removeErr = errRemove

	err := Teardown(standIns.bundle(), teardownTeam)

	if !errors.Is(err, errRemove) {
		t.Errorf("Teardown returned %v, want it to wrap the removal error", err)
	}
	if len(standIns.backend.removed) != 1 || len(standIns.backend.networks) != 0 {
		t.Errorf("expected one attempted removal and no network removal, got removed=%v networks=%v",
			standIns.backend.removed, standIns.backend.networks)
	}
	if !standIns.observer.saw(manifest.ApplicationKind+".remove", engine.StatusError, "web") {
		t.Errorf("observer did not see the removal error, got %v", standIns.observer.events)
	}
}
