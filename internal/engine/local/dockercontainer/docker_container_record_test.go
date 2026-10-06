package dockercontainer

import (
	"context"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/state"
	"github.com/docker/docker/api/types/container"
)

func recordTestBackend(api dockerAPI, deployments *fakeDeploymentStore) *DockerBackend {
	return &DockerBackend{
		client:     api,
		state:      &state.Store{Deployments: deployments},
		registries: testRegistries,
		observer:   engine.NoopObserver{},
	}
}

// upToDateFakeDockerAPI reports the container as already present and running
// so CreateContainer takes the ensureRunning path instead of creating one.
type upToDateFakeDockerAPI struct {
	startCapableFakeDockerAPI
}

func (f *upToDateFakeDockerAPI) ContainerInspect(context.Context, string) (container.InspectResponse, error) {
	return container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID:    "existing-id",
			State: &container.State{Running: true},
		},
	}, nil
}

func TestCreateContainer_RecordsManifestImageAndPolicy(t *testing.T) {
	deployments := &fakeDeploymentStore{}
	backend := recordTestBackend(&startCapableFakeDockerAPI{}, deployments)

	op := aliasTestOp("reg:myregistry/traefik/whoami:latest")
	if err := backend.CreateContainer(op); err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}

	if len(deployments.records) != 1 {
		t.Fatalf("expected one deployment record, got %d", len(deployments.records))
	}
	got := deployments.records[0]
	if want := "reg:myregistry/traefik/whoami:latest"; got.Image != want {
		t.Errorf("recorded image = %q, want the unexpanded manifest reference %q", got.Image, want)
	}
	if got.Policy != op.ImagePullPolicy {
		t.Errorf("recorded policy = %q, want %q", got.Policy, op.ImagePullPolicy)
	}
	if got.Kind != op.Kind || got.Name != op.Name {
		t.Errorf("recorded identity = %q/%q, want %s/%s", got.Kind, got.Name, op.Kind, op.Name)
	}
	if got.ConfigHash == "" {
		t.Error("recorded config hash must not be empty")
	}
}

func TestCreateContainer_UpToDateRedeployFillsLegacyRecord(t *testing.T) {
	op := aliasTestOp("nginx:1.27")
	deployments := &fakeDeploymentStore{}

	fresh := recordTestBackend(&startCapableFakeDockerAPI{}, deployments)
	if err := fresh.CreateContainer(op); err != nil {
		t.Fatalf("first CreateContainer failed: %v", err)
	}
	hash := deployments.records[0].ConfigHash
	deployments.records = []state.Deployment{{Kind: op.Kind, Name: op.Name, ContainerID: "existing-id", ConfigHash: hash}}

	fake := &upToDateFakeDockerAPI{}
	redeploy := recordTestBackend(fake, deployments)
	if err := redeploy.CreateContainer(op); err != nil {
		t.Fatalf("redeploy CreateContainer failed: %v", err)
	}

	if fake.createdConfig != nil {
		t.Error("an up-to-date container must not be recreated")
	}
	last := deployments.records[len(deployments.records)-1]
	if last.Image != "nginx:1.27" || last.Policy != op.ImagePullPolicy {
		t.Errorf("redeploy must fill the legacy record, got image %q policy %q", last.Image, last.Policy)
	}
	if last.ContainerID != "existing-id" || last.ConfigHash != hash {
		t.Errorf("redeploy must keep the container id and hash, got %+v", last)
	}
}
