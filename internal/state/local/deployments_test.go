package local

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/state"
)

// fakeDeploymentFiles is an in-memory stand-in for the per-team
// deployments.txt files so unit tests never touch the real filesystem.
type fakeDeploymentFiles struct {
	files map[string][]byte
}

func newFakeDeploymentFiles() *fakeDeploymentFiles {
	return &fakeDeploymentFiles{files: map[string][]byte{}}
}

func (f *fakeDeploymentFiles) read(path string) ([]byte, error) {
	data, ok := f.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (f *fakeDeploymentFiles) write(path string, data []byte) error {
	f.files[path] = append([]byte(nil), data...)
	return nil
}

func (f *fakeDeploymentFiles) seed(team, content string) {
	f.files[deploymentsPath(team)] = []byte(content)
}

func (f *fakeDeploymentFiles) content(team string) string {
	return string(f.files[deploymentsPath(team)])
}

const testStateDir = "/state"

func deploymentsPath(team string) string {
	return filepath.Join(testStateDir, team, "deployments.txt")
}

func newTestDeploymentStore(files *fakeDeploymentFiles) *DeploymentStore {
	return newDeploymentStoreWithFileOps(testStateDir, files.read, files.write)
}

func TestDeploymentStore_LoadTeam(t *testing.T) {
	files := newFakeDeploymentFiles()
	files.seed("team-a", `
# Team deployments
container web abc123
  # indented comment
container api def456 # inline comment

invalid-line
service db ghi789
container svc cid999 deadbeef
`)
	s := newTestDeploymentStore(files)

	deployments, err := s.loadTeam("team-a")
	if err != nil {
		t.Fatalf("loadTeam failed: %v", err)
	}

	expected := map[string]state.Deployment{
		"web": {Kind: "container", Name: "web", ContainerID: "abc123"},
		"api": {Kind: "container", Name: "api", ContainerID: "def456"},
		"db":  {Kind: "service", Name: "db", ContainerID: "ghi789"},
		"svc": {Kind: "container", Name: "svc", ContainerID: "cid999", ConfigHash: "deadbeef"},
	}

	if len(deployments) != len(expected) {
		t.Errorf("got %d deployments, want %d", len(deployments), len(expected))
	}

	for name, want := range expected {
		got, ok := deployments[name]
		if !ok {
			t.Errorf("deployment %q missing", name)
			continue
		}
		if got != want {
			t.Errorf("deployment %q: got %+v, want %+v", name, got, want)
		}
	}
}

func TestDeploymentStore_LoadsLegacyLinesWithEmptyImageAndPolicy(t *testing.T) {
	files := newFakeDeploymentFiles()
	files.seed("team-a", "Application api cid123 hash123\nResource db cid456\n")
	s := newTestDeploymentStore(files)

	deployments, err := s.loadTeam("team-a")
	if err != nil {
		t.Fatalf("loadTeam failed: %v", err)
	}

	want := map[string]state.Deployment{
		"api": {Kind: "Application", Name: "api", ContainerID: "cid123", ConfigHash: "hash123"},
		"db":  {Kind: "Resource", Name: "db", ContainerID: "cid456"},
	}
	for name, wantDeployment := range want {
		got := deployments[name]
		if got != wantDeployment {
			t.Errorf("legacy %q: got %+v, want %+v", name, got, wantDeployment)
		}
		if got.Image != "" || got.Policy != "" {
			t.Errorf("legacy %q must load with empty Image and Policy, got image %q policy %q", name, got.Image, got.Policy)
		}
	}
}

func TestDeploymentStore_LoadsSixFieldLines(t *testing.T) {
	files := newFakeDeploymentFiles()
	files.seed("team-a", "Application api cid123 hash123 reg:lab/api:1.2.0 IfNotPresent\nResource db cid456 hash456 postgres:16 Always\n")
	s := newTestDeploymentStore(files)

	deployments, err := s.loadTeam("team-a")
	if err != nil {
		t.Fatalf("loadTeam failed: %v", err)
	}

	want := map[string]state.Deployment{
		"api": {Kind: "Application", Name: "api", ContainerID: "cid123", ConfigHash: "hash123", Image: "reg:lab/api:1.2.0", Policy: "IfNotPresent"},
		"db":  {Kind: "Resource", Name: "db", ContainerID: "cid456", ConfigHash: "hash456", Image: "postgres:16", Policy: "Always"},
	}
	for name, wantDeployment := range want {
		if got := deployments[name]; got != wantDeployment {
			t.Errorf("%q: got %+v, want %+v", name, got, wantDeployment)
		}
	}
}

func TestDeploymentStore_RecordWritesSixFields(t *testing.T) {
	files := newFakeDeploymentFiles()
	s := newTestDeploymentStore(files)

	dep := state.Deployment{Kind: "Application", Name: "api", ContainerID: "cid123", ConfigHash: "hash123", Image: "reg:lab/api:1.2.0", Policy: "IfNotPresent"}
	if err := s.Record("team-a", dep); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	want := "Application api cid123 hash123 reg:lab/api:1.2.0 IfNotPresent\n"
	if got := files.content("team-a"); got != want {
		t.Errorf("written file:\ngot  %q\nwant %q", got, want)
	}
}

func TestDeploymentStore_LegacyLineSurvivesAnotherRecord(t *testing.T) {
	files := newFakeDeploymentFiles()
	files.seed("team-a", "Application legacy cid000 hash000\n")
	s := newTestDeploymentStore(files)

	fresh := state.Deployment{Kind: "Resource", Name: "db", ContainerID: "cid456", ConfigHash: "hash456", Image: "postgres:16", Policy: "Always"}
	if err := s.Record("team-a", fresh); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	got, err := s.List("team-a")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })
	want := []state.Deployment{
		fresh,
		{Kind: "Application", Name: "legacy", ContainerID: "cid000", ConfigHash: "hash000"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d deployments, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("List[%d]: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDeploymentStore_Persistence(t *testing.T) {
	files := newFakeDeploymentFiles()
	team := "team-x"

	store1 := newTestDeploymentStore(files)
	dep := state.Deployment{Kind: "container", Name: "web", ContainerID: "abc123", ConfigHash: "aabbcc", Image: "nginx:1.27", Policy: "IfNotPresent"}
	if err := store1.Record(team, dep); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	store2 := newTestDeploymentStore(files)
	deployments, err := store2.List(team)
	if err != nil {
		t.Fatalf("List on re-loaded store failed: %v", err)
	}

	if len(deployments) != 1 || deployments[0] != dep {
		t.Errorf("persistence failed: got %+v, want [%+v]", deployments, dep)
	}
}

func TestDeploymentStore_Interface(t *testing.T) {
	files := newFakeDeploymentFiles()
	store := newTestDeploymentStore(files)
	team := "team-a"

	web := state.Deployment{Kind: "container", Name: "web", ContainerID: "abc123"}
	api := state.Deployment{Kind: "container", Name: "api", ContainerID: "def456"}

	if err := store.Record(team, web); err != nil {
		t.Fatalf("Record web failed: %v", err)
	}
	if err := store.Record(team, api); err != nil {
		t.Fatalf("Record api failed: %v", err)
	}

	webUpdated := state.Deployment{Kind: "container", Name: "web", ContainerID: "newid"}
	if err := store.Record(team, webUpdated); err != nil {
		t.Fatalf("Record web update failed: %v", err)
	}

	got, err := store.List(team)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d deployments, want 2", len(got))
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })
	want := []state.Deployment{api, webUpdated}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("List[%d]: got %+v, want %+v", i, got[i], want[i])
		}
	}

	if err := store.Remove(team, "web"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	got, err = store.List(team)
	if err != nil {
		t.Fatalf("List after Remove failed: %v", err)
	}
	if len(got) != 1 || got[0] != api {
		t.Errorf("after Remove: got %+v, want [%+v]", got, api)
	}

	if err := store.Remove(team, "non-existent"); err != nil {
		t.Errorf("Remove non-existent should not error: %v", err)
	}
}

func TestDeploymentStore_EmptyTeam(t *testing.T) {
	store := newTestDeploymentStore(newFakeDeploymentFiles())

	got, err := store.List("unknown-team")
	if err != nil {
		t.Fatalf("List on empty team failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d deployments for unknown team, want 0", len(got))
	}
}

func TestDeploymentStore_TeamIsolation(t *testing.T) {
	files := newFakeDeploymentFiles()
	store := newTestDeploymentStore(files)

	depA := state.Deployment{Kind: "container", Name: "web", ContainerID: "aaa"}
	depB := state.Deployment{Kind: "container", Name: "web", ContainerID: "bbb"}

	if err := store.Record("team-a", depA); err != nil {
		t.Fatalf("Record team-a failed: %v", err)
	}
	if err := store.Record("team-b", depB); err != nil {
		t.Fatalf("Record team-b failed: %v", err)
	}

	gotA, _ := store.List("team-a")
	if len(gotA) != 1 || gotA[0] != depA {
		t.Errorf("team-a: got %+v, want [%+v]", gotA, depA)
	}

	gotB, _ := store.List("team-b")
	if len(gotB) != 1 || gotB[0] != depB {
		t.Errorf("team-b: got %+v, want [%+v]", gotB, depB)
	}
	if _, exists := files.files[deploymentsPath("team-a")]; !exists {
		t.Error("team-a records should live in team-a's own file")
	}
}
