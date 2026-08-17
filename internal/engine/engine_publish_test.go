package engine

import (
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/planner"
)

// capturingContainerBackend records every CreateContainerOp for assertion.
type capturingContainerBackend struct {
	ops []CreateContainerOp
}

func (c *capturingContainerBackend) CreateNetwork(string) error  { return nil }
func (c *capturingContainerBackend) RemoveNetwork(string) error  { return nil }
func (c *capturingContainerBackend) CreatePlatformNetwork() error { return nil }
func (c *capturingContainerBackend) InspectContainer(string) (ContainerInfo, error) {
	return ContainerInfo{}, nil
}
func (c *capturingContainerBackend) RemoveContainer(RemoveContainerOp) error { return nil }
func (c *capturingContainerBackend) CreateContainer(op CreateContainerOp) error {
	c.ops = append(c.ops, op)
	return nil
}

func deployAppWithNetworking(t *testing.T, networking manifest.Networking) CreateContainerOp {
	t.Helper()
	app := &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind},
		Metadata: manifest.Metadata{Name: "svc", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Image:      "img",
			Port:       3000,
			Networking: networking,
		},
	}
	set := emptyManifestSet()
	set.Applications["svc"] = app

	backend := &capturingContainerBackend{}
	e := &Engine{
		Container: backend,
		Resolver:  stubResolver{},
		Observer:  &recordingObserver{},
	}
	steps := []planner.PlannedStep{{Kind: manifest.ApplicationKind, Name: "svc"}}
	if err := e.ExecuteDeploy(steps, set); err != nil {
		t.Fatalf("ExecuteDeploy failed: %v", err)
	}
	if len(backend.ops) != 1 {
		t.Fatalf("expected exactly one CreateContainer op, got %d", len(backend.ops))
	}
	return backend.ops[0]
}

func TestDeployApplication_ProjectsExplicitPublish(t *testing.T) {
	op := deployAppWithNetworking(t, manifest.Networking{
		Publish: &manifest.Publish{HostPort: 8080},
	})

	if op.Publish == nil {
		t.Fatal("op.Publish should be set for a publishing app")
	}
	if op.Publish.HostPort != 8080 {
		t.Errorf("op.Publish.HostPort = %d, want 8080", op.Publish.HostPort)
	}
	if op.Publish.ContainerPort != 3000 {
		t.Errorf("op.Publish.ContainerPort = %d, want the app's spec.port 3000", op.Publish.ContainerPort)
	}
}

func TestDeployApplication_ProjectsAutomaticPublish(t *testing.T) {
	op := deployAppWithNetworking(t, manifest.Networking{
		Publish: &manifest.Publish{},
	})

	if op.Publish == nil {
		t.Fatal("op.Publish should be set for automatic publishing")
	}
	if op.Publish.HostPort != 0 {
		t.Errorf("op.Publish.HostPort = %d, want 0 (automatic)", op.Publish.HostPort)
	}
}

func TestDeployApplication_NoPublishLeavesOpNil(t *testing.T) {
	op := deployAppWithNetworking(t, manifest.Networking{})

	if op.Publish != nil {
		t.Errorf("op.Publish should be nil without a publish declaration, got %+v", op.Publish)
	}
	if op.ExposeToPlatform {
		t.Error("op.ExposeToPlatform should be false without exposure or publish")
	}
}

// deployAppWithRouting runs one app through ExecuteDeploy with a routing
// backend attached and returns the recorded routing calls.
func deployAppWithRouting(t *testing.T, networking manifest.Networking) []string {
	t.Helper()
	app := &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind},
		Metadata: manifest.Metadata{Name: "svc", Owner: "team-a"},
		Spec: manifest.ApplicationSpec{
			Image:      "img",
			Port:       3000,
			Routing:    manifest.Routing{Domain: "svc.example.com"},
			Networking: networking,
		},
	}
	set := emptyManifestSet()
	set.Applications["svc"] = app

	var calls []string
	e := &Engine{
		Container: &capturingContainerBackend{},
		Routing:   &fakeRoutingBackend{calls: &calls},
		Resolver:  stubResolver{},
		Observer:  &recordingObserver{},
	}
	steps := []planner.PlannedStep{{Kind: manifest.ApplicationKind, Name: "svc"}}
	if err := e.ExecuteDeploy(steps, set); err != nil {
		t.Fatalf("ExecuteDeploy failed: %v", err)
	}
	return calls
}

func TestRoutingGate_RequiresRawExposeToPlatform(t *testing.T) {
	// publish-only must NOT enable routing: the implied platform attachment
	// exists for localhost access, not for gateway exposure.
	calls := deployAppWithRouting(t, manifest.Networking{Publish: &manifest.Publish{HostPort: 8080}})
	for _, c := range calls {
		if strings.HasPrefix(c, "WriteRoute:") {
			t.Fatalf("publish-only app must not get a route written, calls=%v", calls)
		}
	}

	calls = deployAppWithRouting(t, manifest.Networking{ExposeToPlatform: true})
	found := false
	for _, c := range calls {
		if strings.HasPrefix(c, "WriteRoute:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("exposeToPlatform app with a domain should get a route, calls=%v", calls)
	}
}

func TestDeployApplication_DerivesPlatformAttachment(t *testing.T) {
	cases := []struct {
		name       string
		networking manifest.Networking
		want       bool
	}{
		{"exposeOnly", manifest.Networking{ExposeToPlatform: true}, true},
		{"publishOnly", manifest.Networking{Publish: &manifest.Publish{HostPort: 8080}}, true},
		{"both", manifest.Networking{ExposeToPlatform: true, Publish: &manifest.Publish{}}, true},
		{"neither", manifest.Networking{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op := deployAppWithNetworking(t, tc.networking)
			if op.ExposeToPlatform != tc.want {
				t.Errorf("op.ExposeToPlatform = %v, want %v", op.ExposeToPlatform, tc.want)
			}
		})
	}
}
