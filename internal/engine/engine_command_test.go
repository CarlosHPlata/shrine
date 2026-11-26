package engine

import (
	"reflect"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/planner"
)

// deployOne runs a single Application or Resource through ExecuteDeploy and
// returns the CreateContainerOp the backend received.
func deployOne(t *testing.T, set *planner.ManifestSet, kind, name string) CreateContainerOp {
	t.Helper()
	backend := &capturingContainerBackend{}
	e := &Engine{
		Container: backend,
		Resolver:  stubResolver{},
		Observer:  &recordingObserver{},
	}
	if err := e.ExecuteDeploy([]planner.PlannedStep{{Kind: kind, Name: name}}, set); err != nil {
		t.Fatalf("ExecuteDeploy failed: %v", err)
	}
	if len(backend.ops) != 1 {
		t.Fatalf("expected exactly one CreateContainer op, got %d", len(backend.ops))
	}
	return backend.ops[0]
}

func appSet(command manifest.Command) *planner.ManifestSet {
	set := emptyManifestSet()
	set.Applications["web"] = &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind},
		Metadata: manifest.Metadata{Name: "web", Owner: "team-a"},
		Spec:     manifest.ApplicationSpec{ContainerSpec: manifest.ContainerSpec{Image: "node:22", Port: 3000, Command: command}},
	}
	return set
}

func resourceSet(command manifest.Command) *planner.ManifestSet {
	set := emptyManifestSet()
	set.Resources["db"] = &manifest.ResourceManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ResourceKind},
		Metadata: manifest.Metadata{Name: "db", Owner: "team-a"},
		Spec:     manifest.ResourceSpec{ContainerSpec: manifest.ContainerSpec{Image: "redis:7", Port: 6379, Command: command}, Type: "redis"},
	}
	return set
}

func TestDeploy_SpecCommandReachesTheCreateOp(t *testing.T) {
	want := []string{"npm", "run", "worker"}

	t.Run("Application", func(t *testing.T) {
		op := deployOne(t, appSet(manifest.Command{"npm", "run", "worker"}), manifest.ApplicationKind, "web")
		if !reflect.DeepEqual(op.Command, want) {
			t.Errorf("op.Command = %q, want %q", op.Command, want)
		}
	})
	t.Run("Resource", func(t *testing.T) {
		op := deployOne(t, resourceSet(manifest.Command{"npm", "run", "worker"}), manifest.ResourceKind, "db")
		if !reflect.DeepEqual(op.Command, want) {
			t.Errorf("op.Command = %q, want %q", op.Command, want)
		}
	})
}

func TestDeploy_NoSpecCommandLeavesTheCreateOpEmpty(t *testing.T) {
	t.Run("Application", func(t *testing.T) {
		if op := deployOne(t, appSet(nil), manifest.ApplicationKind, "web"); op.Command != nil {
			t.Errorf("op.Command = %q, want nil so the image default runs", op.Command)
		}
	})
	t.Run("Resource", func(t *testing.T) {
		if op := deployOne(t, resourceSet(nil), manifest.ResourceKind, "db"); op.Command != nil {
			t.Errorf("op.Command = %q, want nil so the image default runs", op.Command)
		}
	})
}
