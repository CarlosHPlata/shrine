package dockercontainer

import (
	"context"
	"errors"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
)

// fakeHostPortStore is an in-memory HostPortStore that records calls; unit
// tests never touch the real filesystem-backed store.
type fakeHostPortStore struct {
	ports    map[string]int
	next     int
	claimErr error
	allocErr error
	claims   []string
}

func newFakeHostPortStore() *fakeHostPortStore {
	return &fakeHostPortStore{ports: map[string]int{}, next: 30000}
}

func (f *fakeHostPortStore) AllocateHostPort(team, app string) (int, error) {
	if f.allocErr != nil {
		return 0, f.allocErr
	}
	key := state.HostPortKey(team, app)
	if p, ok := f.ports[key]; ok {
		return p, nil
	}
	p := f.next
	f.next++
	f.ports[key] = p
	return p, nil
}

func (f *fakeHostPortStore) ClaimHostPort(team, app string, port int) error {
	if f.claimErr != nil {
		return f.claimErr
	}
	key := state.HostPortKey(team, app)
	f.claims = append(f.claims, key)
	f.ports[key] = port
	return nil
}

func (f *fakeHostPortStore) GetHostPort(team, app string) (int, error) {
	p, ok := f.ports[state.HostPortKey(team, app)]
	if !ok {
		return 0, state.ErrHostPortNotFound
	}
	return p, nil
}

func (f *fakeHostPortStore) ReleaseHostPort(team, app string) error { return nil }
func (f *fakeHostPortStore) ReleaseTeamHostPorts(team string) error { return nil }
func (f *fakeHostPortStore) ListHostPorts() (state.HostPortMap, error) {
	out := make(state.HostPortMap, len(f.ports))
	for k, v := range f.ports {
		out[k] = v
	}
	return out, nil
}

func publishTestOp(publish *engine.PublishPort) engine.CreateContainerOp {
	return engine.CreateContainerOp{
		Team:            "demo",
		Name:            "web",
		Kind:            "Application",
		Image:           "nginx:alpine",
		ImagePullPolicy: "IfNotPresent",
		Publish:         publish,
	}
}

func publishTestBackend(fake *fakeDockerAPI, hostPorts *fakeHostPortStore) *DockerBackend {
	return &DockerBackend{
		client:   fake,
		state:    &state.Store{HostPorts: hostPorts},
		observer: engine.NoopObserver{},
	}
}

func TestCreateContainer_ExplicitPublishClaimsAndBindsLoopback(t *testing.T) {
	fake := &fakeDockerAPI{createErr: errors.New("stop after create")}
	hostPorts := newFakeHostPortStore()
	backend := publishTestBackend(fake, hostPorts)

	op := publishTestOp(&engine.PublishPort{HostPort: 18080, ContainerPort: 80})
	if err := backend.CreateContainer(op); err == nil {
		t.Fatal("expected the fake's create error to surface")
	}

	if got := hostPorts.ports["demo/web"]; got != 18080 {
		t.Errorf("explicit port should be claimed in the store: got %d, want 18080", got)
	}

	if fake.createdHost == nil {
		t.Fatal("ContainerCreate never received a HostConfig")
	}
	bindings := fake.createdHost.PortBindings[nat.Port("80/tcp")]
	if len(bindings) != 1 {
		t.Fatalf("expected one binding for 80/tcp, got %+v", fake.createdHost.PortBindings)
	}
	if bindings[0].HostIP != "127.0.0.1" {
		t.Errorf("binding HostIP = %q, want \"127.0.0.1\"", bindings[0].HostIP)
	}
	if bindings[0].HostPort != "18080" {
		t.Errorf("binding HostPort = %q, want \"18080\"", bindings[0].HostPort)
	}
	if _, exposed := fake.createdConfig.ExposedPorts[nat.Port("80/tcp")]; !exposed {
		t.Errorf("80/tcp should be exposed, got %+v", fake.createdConfig.ExposedPorts)
	}
}

func TestCreateContainer_ClaimErrorAbortsBeforeCreate(t *testing.T) {
	fake := &fakeDockerAPI{}
	hostPorts := newFakeHostPortStore()
	hostPorts.claimErr = state.ErrHostPortTaken
	backend := publishTestBackend(fake, hostPorts)

	op := publishTestOp(&engine.PublishPort{HostPort: 18080, ContainerPort: 80})
	err := backend.CreateContainer(op)
	if !errors.Is(err, state.ErrHostPortTaken) {
		t.Fatalf("expected ErrHostPortTaken, got %v", err)
	}
	if fake.createdConfig != nil {
		t.Error("ContainerCreate must not run when the port claim fails")
	}
}

// startCapableFakeDockerAPI lets the create flow run to completion so the
// post-start behavior (published event, deployment record) can be asserted.
type startCapableFakeDockerAPI struct {
	fakeDockerAPI
	started bool
}

func (f *startCapableFakeDockerAPI) ContainerStart(context.Context, string, container.StartOptions) error {
	f.started = true
	return nil
}

// fakeDeploymentStore records deployments in memory.
type fakeDeploymentStore struct {
	records []state.Deployment
}

func (f *fakeDeploymentStore) Record(team string, d state.Deployment) error {
	f.records = append(f.records, d)
	return nil
}
func (f *fakeDeploymentStore) Remove(team, name string) error { return nil }
func (f *fakeDeploymentStore) List(team string) ([]state.Deployment, error) {
	return f.records, nil
}

func TestCreateContainer_AutomaticPublishAllocates(t *testing.T) {
	fake := &fakeDockerAPI{createErr: errors.New("stop after create")}
	hostPorts := newFakeHostPortStore()
	backend := publishTestBackend(fake, hostPorts)

	op := publishTestOp(&engine.PublishPort{HostPort: 0, ContainerPort: 80})
	if err := backend.CreateContainer(op); err == nil {
		t.Fatal("expected the fake's create error to surface")
	}

	if got := hostPorts.ports["demo/web"]; got != 30000 {
		t.Errorf("automatic port should be allocated and recorded: got %d, want 30000", got)
	}
	bindings := fake.createdHost.PortBindings[nat.Port("80/tcp")]
	if len(bindings) != 1 || bindings[0].HostPort != "30000" || bindings[0].HostIP != "127.0.0.1" {
		t.Errorf("expected loopback binding on allocated port 30000, got %+v", bindings)
	}
}

func TestCreateContainer_AutomaticPublishReusesPersistedPort(t *testing.T) {
	fake := &fakeDockerAPI{createErr: errors.New("stop after create")}
	hostPorts := newFakeHostPortStore()
	hostPorts.ports["demo/web"] = 30005
	backend := publishTestBackend(fake, hostPorts)

	op := publishTestOp(&engine.PublishPort{HostPort: 0, ContainerPort: 80})
	if err := backend.CreateContainer(op); err == nil {
		t.Fatal("expected the fake's create error to surface")
	}

	bindings := fake.createdHost.PortBindings[nat.Port("80/tcp")]
	if len(bindings) != 1 || bindings[0].HostPort != "30005" {
		t.Errorf("persisted port must be reused across redeploys, got %+v", bindings)
	}
}

func TestCreateContainer_AllocationErrorAbortsWithoutRecord(t *testing.T) {
	fake := &fakeDockerAPI{}
	hostPorts := newFakeHostPortStore()
	hostPorts.allocErr = state.ErrNoAvailableHostPorts
	deployments := &fakeDeploymentStore{}
	backend := &DockerBackend{
		client:   fake,
		state:    &state.Store{HostPorts: hostPorts, Deployments: deployments},
		observer: engine.NoopObserver{},
	}

	op := publishTestOp(&engine.PublishPort{HostPort: 0, ContainerPort: 80})
	err := backend.CreateContainer(op)
	if !errors.Is(err, state.ErrNoAvailableHostPorts) {
		t.Fatalf("expected ErrNoAvailableHostPorts, got %v", err)
	}
	if fake.createdConfig != nil {
		t.Error("ContainerCreate must not run when allocation fails")
	}
	if len(deployments.records) != 0 {
		t.Error("no deployment record may be written when allocation fails")
	}
}

func TestCreateContainer_EmitsPublishedEventAfterStart(t *testing.T) {
	fake := &startCapableFakeDockerAPI{}
	hostPorts := newFakeHostPortStore()
	deployments := &fakeDeploymentStore{}
	obs := &recordingObserver{}
	backend := &DockerBackend{
		client:   fake,
		state:    &state.Store{HostPorts: hostPorts, Deployments: deployments},
		observer: obs,
	}

	op := publishTestOp(&engine.PublishPort{HostPort: 18080, ContainerPort: 80})
	if err := backend.CreateContainer(op); err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}
	if !fake.started {
		t.Fatal("container was never started")
	}

	event, ok := obs.find("hostport.published", engine.StatusFinished)
	if !ok {
		t.Fatal("expected a hostport.published event after start")
	}
	if event.Fields["hostPort"] != "18080" || event.Fields["containerPort"] != "80" {
		t.Errorf("published event fields = %v, want hostPort=18080 containerPort=80", event.Fields)
	}
	if len(deployments.records) != 1 {
		t.Errorf("expected one deployment record after start, got %d", len(deployments.records))
	}
}

func TestBuildPortBindings_PassesHostIPThrough(t *testing.T) {
	exposed, pmap := buildPortBindings([]PortBinding{
		{HostIP: "127.0.0.1", HostPort: "8080", ContainerPort: "80"},
		{HostPort: "9000", ContainerPort: "9000"},
	})

	if _, ok := exposed[nat.Port("80/tcp")]; !ok {
		t.Errorf("80/tcp missing from exposed set: %+v", exposed)
	}
	loopback := pmap[nat.Port("80/tcp")]
	if len(loopback) != 1 || loopback[0].HostIP != "127.0.0.1" || loopback[0].HostPort != "8080" {
		t.Errorf("loopback binding not passed through: %+v", loopback)
	}
	wildcard := pmap[nat.Port("9000/tcp")]
	if len(wildcard) != 1 || wildcard[0].HostIP != "" {
		t.Errorf("empty HostIP must stay empty (0.0.0.0 publish): %+v", wildcard)
	}
}

func TestConfigHash_HostIPOnlyChangesHashWhenSet(t *testing.T) {
	base := engine.CreateContainerOp{
		Team:  "demo",
		Name:  "web",
		Image: "nginx:alpine",
		PortBindings: []engine.PortBinding{
			{HostPort: "8080", ContainerPort: "80"},
		},
	}
	same := base
	same.PortBindings = []engine.PortBinding{{HostPort: "8080", ContainerPort: "80"}}

	if configHash(base, "digest") != configHash(same, "digest") {
		t.Error("identical ops must produce identical hashes")
	}

	withIP := base
	withIP.PortBindings = []engine.PortBinding{{HostIP: "127.0.0.1", HostPort: "8080", ContainerPort: "80"}}
	if configHash(base, "digest") == configHash(withIP, "digest") {
		t.Error("adding a HostIP must change the config hash")
	}
}
