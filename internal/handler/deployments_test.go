package handler

import (
	"errors"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

// In-memory store fakes: handler unit tests never touch the filesystem.

type memTeamStore struct{ teams []string }

func (m *memTeamStore) SaveTeam(*manifest.TeamManifest) error { return nil }
func (m *memTeamStore) LoadTeam(name string) (*manifest.TeamManifest, error) {
	return &manifest.TeamManifest{Metadata: manifest.Metadata{Name: name}}, nil
}
func (m *memTeamStore) ListTeams() ([]*manifest.TeamManifest, error) {
	out := make([]*manifest.TeamManifest, len(m.teams))
	for i, name := range m.teams {
		out[i] = &manifest.TeamManifest{Metadata: manifest.Metadata{Name: name}}
	}
	return out, nil
}
func (m *memTeamStore) DeleteTeam(string) error { return nil }

type memDeploymentStore struct{ byTeam map[string][]state.Deployment }

func (m *memDeploymentStore) Record(team string, d state.Deployment) error {
	m.byTeam[team] = append(m.byTeam[team], d)
	return nil
}
func (m *memDeploymentStore) Remove(team, name string) error {
	kept := m.byTeam[team][:0]
	for _, d := range m.byTeam[team] {
		if d.Name != name {
			kept = append(kept, d)
		}
	}
	m.byTeam[team] = kept
	return nil
}
func (m *memDeploymentStore) List(team string) ([]state.Deployment, error) {
	return m.byTeam[team], nil
}

type memHostPortStore struct{ ports state.HostPortMap }

func (m *memHostPortStore) AllocateHostPort(team, app string) (int, error) { return 0, nil }
func (m *memHostPortStore) ClaimHostPort(team, app string, port int) error { return nil }
func (m *memHostPortStore) GetHostPort(team, app string) (int, error) {
	p, ok := m.ports[state.HostPortKey(team, app)]
	if !ok {
		return 0, state.ErrHostPortNotFound
	}
	return p, nil
}
func (m *memHostPortStore) ReleaseHostPort(team, app string) error {
	delete(m.ports, state.HostPortKey(team, app))
	return nil
}
func (m *memHostPortStore) ReleaseTeamHostPorts(team string) error {
	for key := range m.ports {
		if strings.HasPrefix(key, team+"/") {
			delete(m.ports, key)
		}
	}
	return nil
}
func (m *memHostPortStore) ListHostPorts() (state.HostPortMap, error) {
	out := make(state.HostPortMap, len(m.ports))
	for k, v := range m.ports {
		out[k] = v
	}
	return out, nil
}

type memSubnetStore struct{}

func (memSubnetStore) AllocateSubnet(string) (string, error) { return "", nil }
func (memSubnetStore) GetSubnet(string) (string, error)      { return "", state.ErrSubnetNotFound }
func (memSubnetStore) ReleaseSubnet(string) error            { return nil }
func (memSubnetStore) ListSubnets() (state.SubnetMap, error) { return state.SubnetMap{}, nil }

// stubContainerBackend reports a container as present or absent by name.
type stubContainerBackend struct{ existing map[string]bool }

func (s *stubContainerBackend) CreateNetwork(string) error                { return nil }
func (s *stubContainerBackend) RemoveNetwork(string) error                { return nil }
func (s *stubContainerBackend) CreateContainer(engine.CreateContainerOp) error { return nil }
func (s *stubContainerBackend) RemoveContainer(engine.RemoveContainerOp) error { return nil }
func (s *stubContainerBackend) CreatePlatformNetwork() error              { return nil }
func (s *stubContainerBackend) InspectContainer(name string) (engine.ContainerInfo, error) {
	if s.existing[name] {
		return engine.ContainerInfo{Running: true, Status: "running"}, nil
	}
	return engine.ContainerInfo{}, errors.New("no such container")
}

func deleteTestStore(teams []string, ports state.HostPortMap, deployments map[string][]state.Deployment) *state.Store {
	if deployments == nil {
		deployments = map[string][]state.Deployment{}
	}
	return &state.Store{
		Teams:       &memTeamStore{teams: teams},
		Deployments: &memDeploymentStore{byTeam: deployments},
		HostPorts:   &memHostPortStore{ports: ports},
		Subnets:     memSubnetStore{},
	}
}

func TestDeleteApplication_RefusesWhileContainerExists(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, state.HostPortMap{"demo/api": 30000}, nil)
	backend := &stubContainerBackend{existing: map[string]bool{"demo.api": true}}

	err := DeleteApplication(store, backend, DeleteApplicationOptions{Name: "api"})
	if err == nil {
		t.Fatal("expected a refusal while the container exists")
	}
	if !strings.Contains(err.Error(), "teardown") {
		t.Errorf("refusal should point at teardown, got: %v", err)
	}
	if _, getErr := store.HostPorts.GetHostPort("demo", "api"); getErr != nil {
		t.Error("the port must NOT be released when the delete is refused")
	}
}

func TestDeleteApplication_ReleasesPortAndRecord(t *testing.T) {
	store := deleteTestStore([]string{"demo"},
		state.HostPortMap{"demo/api": 30000},
		map[string][]state.Deployment{"demo": {{Kind: manifest.ApplicationKind, Name: "api"}}},
	)
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteApplication(store, backend, DeleteApplicationOptions{Name: "api"}); err != nil {
		t.Fatalf("DeleteApplication failed: %v", err)
	}

	if _, err := store.HostPorts.GetHostPort("demo", "api"); !errors.Is(err, state.ErrHostPortNotFound) {
		t.Error("the port should be released")
	}
	records, _ := store.Deployments.List("demo")
	if len(records) != 0 {
		t.Errorf("the stale deployment record should be removed, got %v", records)
	}
}

func TestDeleteApplication_IdempotentWhenNothingHeld(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, state.HostPortMap{}, nil)
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteApplication(store, backend, DeleteApplicationOptions{Name: "ghost"}); err != nil {
		t.Errorf("deleting nothing should be a soft success, got: %v", err)
	}
	if err := DeleteApplication(store, backend, DeleteApplicationOptions{Name: "ghost", Team: "demo"}); err != nil {
		t.Errorf("deleting nothing with an explicit team should be a soft success, got: %v", err)
	}
}

func TestDeleteApplication_DryRunWritesNothing(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, state.HostPortMap{"demo/api": 30000},
		map[string][]state.Deployment{"demo": {{Kind: manifest.ApplicationKind, Name: "api"}}},
	)
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteApplication(store, backend, DeleteApplicationOptions{Name: "api", DryRun: true}); err != nil {
		t.Fatalf("dry-run DeleteApplication failed: %v", err)
	}

	if _, err := store.HostPorts.GetHostPort("demo", "api"); err != nil {
		t.Error("dry-run must not release the port")
	}
	records, _ := store.Deployments.List("demo")
	if len(records) != 1 {
		t.Error("dry-run must not remove the deployment record")
	}
}

func TestDeleteApplication_AmbiguousAcrossTeams(t *testing.T) {
	store := deleteTestStore([]string{"demo", "media"},
		state.HostPortMap{"demo/api": 30000, "media/api": 30001}, nil)
	backend := &stubContainerBackend{existing: map[string]bool{}}

	err := DeleteApplication(store, backend, DeleteApplicationOptions{Name: "api"})
	if err == nil {
		t.Fatal("expected an ambiguity error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "demo") || !strings.Contains(msg, "media") {
		t.Errorf("ambiguity error should list the candidate teams, got: %v", err)
	}

	// Disambiguated with --team it proceeds.
	if err := DeleteApplication(store, backend, DeleteApplicationOptions{Name: "api", Team: "demo"}); err != nil {
		t.Fatalf("explicit --team should disambiguate, got: %v", err)
	}
	if _, err := store.HostPorts.GetHostPort("media", "api"); err != nil {
		t.Error("the other team's allocation must be untouched")
	}
}

func TestDeleteTeam_ReleasesTeamHostPorts(t *testing.T) {
	store := deleteTestStore([]string{"demo"},
		state.HostPortMap{"demo/api": 30000, "demo/web": 8080, "media/jellyfin": 30001}, nil)

	if err := DeleteTeam("demo", store); err != nil {
		t.Fatalf("DeleteTeam failed: %v", err)
	}

	ports, _ := store.HostPorts.ListHostPorts()
	if len(ports) != 1 || ports["media/jellyfin"] != 30001 {
		t.Errorf("only the other team's allocation should remain, got %v", ports)
	}
}
