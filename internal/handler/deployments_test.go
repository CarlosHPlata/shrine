package handler

import (
	"bytes"
	"errors"
	"io"
	"os"
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

type memDeploymentStore struct {
	byTeam  map[string][]state.Deployment
	listErr error
}

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
	if m.listErr != nil {
		return nil, m.listErr
	}
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

func (s *stubContainerBackend) CreateNetwork(string) error                     { return nil }
func (s *stubContainerBackend) RemoveNetwork(string) error                     { return nil }
func (s *stubContainerBackend) CreateContainer(engine.CreateContainerOp) error { return nil }
func (s *stubContainerBackend) RemoveContainer(engine.RemoveContainerOp) error { return nil }
func (s *stubContainerBackend) CreatePlatformNetwork() error                   { return nil }
func (s *stubContainerBackend) InspectContainer(name string) (engine.ContainerInfo, error) {
	if s.existing[name] {
		return engine.ContainerInfo{Running: true, Status: "running"}, nil
	}
	return engine.ContainerInfo{}, errors.New("no such container")
}
func (s *stubContainerBackend) ResolveImage(op engine.ResolveImageOp) (engine.ResolvedImage, error) {
	return engine.ResolvedImage{Ref: op.Image, Source: engine.ImageSourceManifest}, nil
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

	err := DeleteApplication(store, backend, DeleteOptions{Name: "api"})
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

	if err := DeleteApplication(store, backend, DeleteOptions{Name: "api"}); err != nil {
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

	if err := DeleteApplication(store, backend, DeleteOptions{Name: "ghost"}); err != nil {
		t.Errorf("deleting nothing should be a soft success, got: %v", err)
	}
	if err := DeleteApplication(store, backend, DeleteOptions{Name: "ghost", Team: "demo"}); err != nil {
		t.Errorf("deleting nothing with an explicit team should be a soft success, got: %v", err)
	}
}

func TestDeleteApplication_DryRunWritesNothing(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, state.HostPortMap{"demo/api": 30000},
		map[string][]state.Deployment{"demo": {{Kind: manifest.ApplicationKind, Name: "api"}}},
	)
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteApplication(store, backend, DeleteOptions{Name: "api", DryRun: true}); err != nil {
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

	err := DeleteApplication(store, backend, DeleteOptions{Name: "api"})
	if err == nil {
		t.Fatal("expected an ambiguity error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "demo") || !strings.Contains(msg, "media") {
		t.Errorf("ambiguity error should list the candidate teams, got: %v", err)
	}

	// Disambiguated with --team it proceeds.
	if err := DeleteApplication(store, backend, DeleteOptions{Name: "api", Team: "demo"}); err != nil {
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

// memImagePinStore keeps every Put in puts as well, so a test can prove a
// command recorded nothing.
type memImagePinStore struct {
	pins map[string]state.ImagePin
	puts []state.ImagePin
}

func newMemImagePinStore(pins ...state.ImagePin) *memImagePinStore {
	m := &memImagePinStore{pins: map[string]state.ImagePin{}}
	for _, pin := range pins {
		m.pins[state.ImagePinKey("demo", pin.Name)] = pin
	}
	return m
}

func (m *memImagePinStore) Get(team, name string) (state.ImagePin, error) {
	pin, ok := m.pins[state.ImagePinKey(team, name)]
	if !ok {
		return state.ImagePin{}, state.ErrImagePinNotFound
	}
	return pin, nil
}
func (m *memImagePinStore) Put(team string, pin state.ImagePin) error {
	m.pins[state.ImagePinKey(team, pin.Name)] = pin
	m.puts = append(m.puts, pin)
	return nil
}
func (m *memImagePinStore) Release(team, name string) error {
	delete(m.pins, state.ImagePinKey(team, name))
	return nil
}
func (m *memImagePinStore) ReleaseTeam(team string) error {
	for key := range m.pins {
		if strings.HasPrefix(key, team+"/") {
			delete(m.pins, key)
		}
	}
	return nil
}
func (m *memImagePinStore) List(team string) ([]state.ImagePin, error) {
	var pins []state.ImagePin
	for key, pin := range m.pins {
		if strings.HasPrefix(key, team+"/") {
			pins = append(pins, pin)
		}
	}
	return pins, nil
}
func (m *memImagePinStore) ListAll() (map[string]state.ImagePin, error) { return m.pins, nil }

func apiPin() state.ImagePin {
	return state.ImagePin{Kind: manifest.ApplicationKind, Name: "api", Requested: "ghcr.io/me/api:latest", Pinned: "ghcr.io/me/api@sha256:abc"}
}

func TestDeleteApplication_ReleasesThePin(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, state.HostPortMap{},
		map[string][]state.Deployment{"demo": {{Kind: manifest.ApplicationKind, Name: "api"}}})
	store.ImagePins = newMemImagePinStore(apiPin())
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteApplication(store, backend, DeleteOptions{Name: "api"}); err != nil {
		t.Fatalf("DeleteApplication failed: %v", err)
	}

	if _, err := store.ImagePins.Get("demo", "api"); !errors.Is(err, state.ErrImagePinNotFound) {
		t.Errorf("the pin should be released, got %v", err)
	}
}

func TestDeleteApplication_DryRunKeepsThePin(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, state.HostPortMap{},
		map[string][]state.Deployment{"demo": {{Kind: manifest.ApplicationKind, Name: "api"}}})
	store.ImagePins = newMemImagePinStore(apiPin())
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteApplication(store, backend, DeleteOptions{Name: "api", DryRun: true}); err != nil {
		t.Fatalf("dry-run DeleteApplication failed: %v", err)
	}

	if _, err := store.ImagePins.Get("demo", "api"); err != nil {
		t.Error("dry-run must not release the pin")
	}
}

// A pin alone, with no port and no record, still locates the team and is
// released: a torn-down pinned application keeps only its pin.
func TestDeleteApplication_PinAloneIsFoundAndReleased(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, state.HostPortMap{}, nil)
	store.ImagePins = newMemImagePinStore(apiPin())
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteApplication(store, backend, DeleteOptions{Name: "api"}); err != nil {
		t.Fatalf("DeleteApplication failed: %v", err)
	}

	if _, err := store.ImagePins.Get("demo", "api"); !errors.Is(err, state.ErrImagePinNotFound) {
		t.Errorf("the pin should be released, got %v", err)
	}
}

func TestDeleteApplication_ToleratesAStoreWithoutPins(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, state.HostPortMap{"demo/api": 30000}, nil)
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteApplication(store, backend, DeleteOptions{Name: "api"}); err != nil {
		t.Fatalf("DeleteApplication must tolerate a nil ImagePins store, got %v", err)
	}
}

func cachePin() state.ImagePin {
	return state.ImagePin{Kind: manifest.ResourceKind, Name: "cache", Requested: "ghcr.io/me/cache:latest", Pinned: "ghcr.io/me/cache@sha256:abc"}
}

// cacheDeleteStore holds a torn-down resource: its record and its pin, with
// no host-port store at all so the resource path is proven never to consult one.
func cacheDeleteStore() *state.Store {
	store := deleteTestStore([]string{"demo"}, nil,
		map[string][]state.Deployment{"demo": {{Kind: manifest.ResourceKind, Name: "cache"}}})
	store.HostPorts = nil
	store.ImagePins = newMemImagePinStore(cachePin())
	return store
}

// captureStdout runs fn with os.Stdout redirected through an in-memory pipe
// and returns what it printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	defer func() { os.Stdout = orig }()
	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}

func TestDeleteResource_ReleasesPinAndRecord(t *testing.T) {
	store := cacheDeleteStore()
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteResource(store, backend, DeleteOptions{Name: "cache"}); err != nil {
		t.Fatalf("DeleteResource failed: %v", err)
	}

	if _, err := store.ImagePins.Get("demo", "cache"); !errors.Is(err, state.ErrImagePinNotFound) {
		t.Errorf("the pin should be released, got %v", err)
	}
	records, _ := store.Deployments.List("demo")
	if len(records) != 0 {
		t.Errorf("the stale deployment record should be removed, got %v", records)
	}
}

// A pin alone, with no record, still locates the team and is released: a
// torn-down pinned resource may keep only its pin.
func TestDeleteResource_PinAloneIsFoundAndReleased(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, nil, nil)
	store.HostPorts = nil
	store.ImagePins = newMemImagePinStore(cachePin())
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteResource(store, backend, DeleteOptions{Name: "cache"}); err != nil {
		t.Fatalf("DeleteResource failed: %v", err)
	}

	if _, err := store.ImagePins.Get("demo", "cache"); !errors.Is(err, state.ErrImagePinNotFound) {
		t.Errorf("the pin should be released, got %v", err)
	}
}

func TestDeleteResource_IdempotentWhenNothingHeld(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, nil, nil)
	store.ImagePins = newMemImagePinStore()
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteResource(store, backend, DeleteOptions{Name: "ghost"}); err != nil {
		t.Errorf("deleting nothing should be a soft success, got: %v", err)
	}
	if err := DeleteResource(store, backend, DeleteOptions{Name: "ghost", Team: "demo"}); err != nil {
		t.Errorf("deleting nothing with an explicit team should be a soft success, got: %v", err)
	}
}

func TestDeleteResource_ToleratesAStoreWithoutPins(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, nil,
		map[string][]state.Deployment{"demo": {{Kind: manifest.ResourceKind, Name: "cache"}}})
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteResource(store, backend, DeleteOptions{Name: "cache"}); err != nil {
		t.Fatalf("DeleteResource must tolerate a nil ImagePins store, got %v", err)
	}
}

// The kind guard keeps delete resource from touching an application that
// shares the name.
func TestDeleteResource_IgnoresAnApplicationOfTheSameName(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, nil,
		map[string][]state.Deployment{"demo": {{Kind: manifest.ApplicationKind, Name: "api"}}})
	store.ImagePins = newMemImagePinStore(apiPin())
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteResource(store, backend, DeleteOptions{Name: "api"}); err != nil {
		t.Fatalf("DeleteResource should be a soft success, got %v", err)
	}

	if _, err := store.ImagePins.Get("demo", "api"); err != nil {
		t.Errorf("the application's pin must remain, got %v", err)
	}
	records, _ := store.Deployments.List("demo")
	if len(records) != 1 {
		t.Errorf("the application's record must remain, got %v", records)
	}
}

func TestDeleteApplication_IgnoresAResourceOfTheSameName(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, state.HostPortMap{},
		map[string][]state.Deployment{"demo": {{Kind: manifest.ResourceKind, Name: "cache"}}})
	store.ImagePins = newMemImagePinStore(cachePin())
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteApplication(store, backend, DeleteOptions{Name: "cache"}); err != nil {
		t.Fatalf("DeleteApplication should be a soft success, got %v", err)
	}

	if _, err := store.ImagePins.Get("demo", "cache"); err != nil {
		t.Errorf("the resource's pin must remain, got %v", err)
	}
	records, _ := store.Deployments.List("demo")
	if len(records) != 1 {
		t.Errorf("the resource's record must remain, got %v", records)
	}
}

func TestDeleteResource_RefusesWhileContainerExists(t *testing.T) {
	backend := &stubContainerBackend{existing: map[string]bool{"demo.cache": true}}

	for _, dryRun := range []bool{false, true} {
		store := cacheDeleteStore()
		err := DeleteResource(store, backend, DeleteOptions{Name: "cache", DryRun: dryRun})
		if err == nil {
			t.Fatalf("dryRun=%v: expected a refusal while the container exists", dryRun)
		}
		for _, want := range []string{`resource "demo/cache" still has a container`, "shrine teardown demo"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("dryRun=%v: refusal should contain %q, got: %v", dryRun, want, err)
			}
		}
		if _, getErr := store.ImagePins.Get("demo", "cache"); getErr != nil {
			t.Errorf("dryRun=%v: the pin must NOT be released when the delete is refused", dryRun)
		}
		records, _ := store.Deployments.List("demo")
		if len(records) != 1 {
			t.Errorf("dryRun=%v: the record must NOT be removed when the delete is refused, got %v", dryRun, records)
		}
	}
}

func TestDeleteResource_DryRunWritesNothing(t *testing.T) {
	store := cacheDeleteStore()
	backend := &stubContainerBackend{existing: map[string]bool{}}

	var err error
	out := captureStdout(t, func() {
		err = DeleteResource(store, backend, DeleteOptions{Name: "cache", DryRun: true})
	})
	if err != nil {
		t.Fatalf("dry-run DeleteResource failed: %v", err)
	}

	if _, err := store.ImagePins.Get("demo", "cache"); err != nil {
		t.Error("dry-run must not release the pin")
	}
	records, _ := store.Deployments.List("demo")
	if len(records) != 1 {
		t.Error("dry-run must not remove the deployment record")
	}
	for _, want := range []string{
		"[dry-run] would release image pin ghcr.io/me/cache@sha256:abc for demo/cache",
		"[dry-run] would remove deployment record for demo/cache",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "host port") {
		t.Errorf("a resource dry run must not mention a host port:\n%s", out)
	}
}

func TestDeleteResource_AmbiguousAcrossTeams(t *testing.T) {
	store := deleteTestStore([]string{"demo", "media"}, nil, map[string][]state.Deployment{
		"demo":  {{Kind: manifest.ResourceKind, Name: "cache"}},
		"media": {{Kind: manifest.ResourceKind, Name: "cache"}},
	})
	backend := &stubContainerBackend{existing: map[string]bool{}}

	err := DeleteResource(store, backend, DeleteOptions{Name: "cache"})
	if err == nil {
		t.Fatal("expected an ambiguity error")
	}
	if want := `ambiguous: resource "cache" found in teams [demo, media]`; !strings.Contains(err.Error(), want) {
		t.Errorf("ambiguity error should contain %q, got: %v", want, err)
	}

	if err := DeleteResource(store, backend, DeleteOptions{Name: "cache", Team: "demo"}); err != nil {
		t.Fatalf("explicit --team should disambiguate, got: %v", err)
	}
	if records, _ := store.Deployments.List("demo"); len(records) != 0 {
		t.Errorf("demo's record should be removed, got %v", records)
	}
	if records, _ := store.Deployments.List("media"); len(records) != 1 {
		t.Errorf("media's record must be untouched, got %v", records)
	}
}

// Only resource state counts as a candidate: an application of the same name
// in another team does not make the resource ambiguous.
func TestDeleteResource_ApplicationInAnotherTeamIsNotACandidate(t *testing.T) {
	store := deleteTestStore([]string{"demo", "media"}, nil, map[string][]state.Deployment{
		"demo":  {{Kind: manifest.ApplicationKind, Name: "api"}},
		"media": {{Kind: manifest.ResourceKind, Name: "api"}},
	})
	backend := &stubContainerBackend{existing: map[string]bool{}}

	if err := DeleteResource(store, backend, DeleteOptions{Name: "api"}); err != nil {
		t.Fatalf("DeleteResource should find media's resource without ambiguity, got: %v", err)
	}
	if records, _ := store.Deployments.List("media"); len(records) != 0 {
		t.Errorf("media's resource record should be removed, got %v", records)
	}
	if records, _ := store.Deployments.List("demo"); len(records) != 1 {
		t.Errorf("demo's application record must remain, got %v", records)
	}
}

func describeTestStore(records map[string][]state.Deployment) *state.Store {
	teams := make([]string, 0, len(records))
	for team := range records {
		teams = append(teams, team)
	}
	return deleteTestStore(teams, nil, records)
}

func pinnedApiRecord() state.Deployment {
	return state.Deployment{Name: "api", Kind: manifest.ApplicationKind, ContainerID: "c1", Image: "ghcr.io/me/api", Policy: manifest.ImagePullPolicyPinned}
}

func TestDescribeApplication_SucceedsWhenTheBackendCannotInspect(t *testing.T) {
	store := describeTestStore(map[string][]state.Deployment{"demo": {pinnedApiRecord()}})
	store.ImagePins = newMemImagePinStore(apiPin())

	if err := DescribeApplication("demo", "api", store, &stubContainerBackend{}); err != nil {
		t.Fatalf("describe must succeed when the container cannot be inspected, got %v", err)
	}
}

func TestDescribeApplication_ToleratesAStoreWithoutPins(t *testing.T) {
	store := describeTestStore(map[string][]state.Deployment{"demo": {pinnedApiRecord()}})

	if err := DescribeApplication("", "api", store, &stubContainerBackend{existing: map[string]bool{"c1": true}}); err != nil {
		t.Fatalf("describe must tolerate a nil ImagePins store, got %v", err)
	}
}

func TestDescribeApplication_ToleratesANilBackend(t *testing.T) {
	store := describeTestStore(map[string][]state.Deployment{"demo": {pinnedApiRecord()}})
	store.ImagePins = newMemImagePinStore(apiPin())

	if err := DescribeApplication("demo", "api", store, nil); err != nil {
		t.Fatalf("describe must tolerate a nil backend, got %v", err)
	}
}

func TestDescribeApplication_NotFoundAndAmbiguousAreUnchanged(t *testing.T) {
	store := describeTestStore(map[string][]state.Deployment{"demo": {pinnedApiRecord()}, "other": {pinnedApiRecord()}})

	err := DescribeApplication("", "missing", store, nil)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a not-found error, got %v", err)
	}
	err = DescribeApplication("", "api", store, nil)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected an ambiguity error, got %v", err)
	}
}

func TestDescribeApplication_APinWithoutARecordIsNotFound(t *testing.T) {
	store := describeTestStore(map[string][]state.Deployment{"demo": {}})
	store.ImagePins = newMemImagePinStore(apiPin())

	err := DescribeApplication("", "api", store, nil)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("a pin without a deployment record must stay invisible, got %v", err)
	}
}
