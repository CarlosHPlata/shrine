package local

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CarlosHPlata/shrine/internal/state"
)

// fakePinFiles is an in-memory stand-in for every team's pins.txt so unit
// tests never touch the real filesystem.
type fakePinFiles struct {
	files    map[string][]byte
	writes   int
	writeErr error
}

func newFakePinFiles() *fakePinFiles {
	return &fakePinFiles{files: map[string][]byte{}}
}

func (f *fakePinFiles) read(path string) ([]byte, error) {
	data, ok := f.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (f *fakePinFiles) write(path string, data []byte) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.files[path] = append([]byte(nil), data...)
	f.writes++
	return nil
}

func (f *fakePinFiles) seed(team, content string) {
	f.files[pinsPath(team)] = []byte(content)
}

func (f *fakePinFiles) content(team string) string {
	return string(f.files[pinsPath(team)])
}

func pinsPath(team string) string {
	return filepath.Join(testStateDir, team, "pins.txt")
}

// teams derives the team directories from the files held, as ReadDir would.
func (f *fakePinFiles) teams(string) ([]string, error) {
	var teams []string
	for path := range f.files {
		teams = append(teams, filepath.Base(filepath.Dir(path)))
	}
	return teams, nil
}

func newTestImagePinStore(files *fakePinFiles) state.ImagePinStore {
	return newImagePinStoreWithFileOps(testStateDir, files.read, files.write, files.teams)
}

var pinnedAt = time.Date(2026, 10, 6, 10, 42, 17, 0, time.UTC)

func webPin() state.ImagePin {
	return state.ImagePin{
		Kind:      "Application",
		Name:      "web",
		Requested: "ghcr.io/me/web:latest",
		Pinned:    "ghcr.io/me/web@sha256:3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a",
		PinnedAt:  pinnedAt,
	}
}

func dbPin() state.ImagePin {
	return state.ImagePin{
		Kind:      "Resource",
		Name:      "db",
		Requested: "postgres",
		Pinned:    "postgres@sha256:9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b",
		PinnedAt:  pinnedAt.Add(-2 * time.Minute),
	}
}

func TestImagePinStore_MissingFileIsEmptyTeam(t *testing.T) {
	s := newTestImagePinStore(newFakePinFiles())

	pins, err := s.List("team-a")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(pins) != 0 {
		t.Errorf("missing file should mean no pins, got %v", pins)
	}
	if _, err := s.Get("team-a", "web"); !errors.Is(err, state.ErrImagePinNotFound) {
		t.Errorf("Get on an empty team = %v, want ErrImagePinNotFound", err)
	}
}

func TestImagePinStore_LoadForgivingFormat(t *testing.T) {
	files := newFakePinFiles()
	files.seed("team-a", `
# image pins
Application web ghcr.io/me/web:latest ghcr.io/me/web@sha256:aaa 2026-10-06T10:42:17Z

Resource short postgres postgres@sha256:bbb
Resource baddate postgres postgres@sha256:ccc yesterday
Resource nodigest postgres postgres:17 2026-10-06T10:40:03Z
Resource db postgres postgres@sha256:ddd 2026-10-06T10:40:03Z # inline comment
`)
	s := newTestImagePinStore(files)

	pins, err := s.List("team-a")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(pins) != 2 {
		t.Fatalf("got %d pins, want the two well-formed lines: %+v", len(pins), pins)
	}
	if pins[0].Name != "db" || pins[1].Name != "web" {
		t.Errorf("List must be sorted by name, got %q then %q", pins[0].Name, pins[1].Name)
	}
	web := pins[1]
	if web.Kind != "Application" || web.Requested != "ghcr.io/me/web:latest" || web.Pinned != "ghcr.io/me/web@sha256:aaa" || !web.PinnedAt.Equal(pinnedAt) {
		t.Errorf("web pin parsed as %+v", web)
	}
}

func TestImagePinStore_PutWritesSortedFiveFieldLines(t *testing.T) {
	files := newFakePinFiles()
	s := newTestImagePinStore(files)

	if err := s.Put("team-a", webPin()); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if err := s.Put("team-a", dbPin()); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	want := "Resource db postgres " + dbPin().Pinned + " 2026-10-06T10:40:17Z\n" +
		"Application web ghcr.io/me/web:latest " + webPin().Pinned + " 2026-10-06T10:42:17Z\n"
	if got := files.content("team-a"); got != want {
		t.Errorf("pins.txt:\ngot  %q\nwant %q", got, want)
	}
}

func TestImagePinStore_PutReplacesByName(t *testing.T) {
	files := newFakePinFiles()
	s := newTestImagePinStore(files)
	if err := s.Put("team-a", webPin()); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	replaced := webPin()
	replaced.Pinned = "ghcr.io/me/web@sha256:ffff"
	if err := s.Put("team-a", replaced); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	pins, _ := s.List("team-a")
	if len(pins) != 1 || pins[0].Pinned != replaced.Pinned {
		t.Errorf("Put must replace the pin of the same name, got %+v", pins)
	}
	got, err := s.Get("team-a", "web")
	if err != nil || got.Pinned != replaced.Pinned {
		t.Errorf("Get = %+v, %v; want the replaced pin", got, err)
	}
}

func TestImagePinStore_ReleaseIsIdempotentAndWritesOnlyOnChange(t *testing.T) {
	files := newFakePinFiles()
	s := newTestImagePinStore(files)
	if err := s.Put("team-a", webPin()); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	writesAfterPut := files.writes

	if err := s.Release("team-a", "ghost"); err != nil {
		t.Fatalf("Release of an absent pin must succeed, got %v", err)
	}
	if files.writes != writesAfterPut {
		t.Error("releasing an absent pin must not rewrite the file")
	}

	if err := s.Release("team-a", "web"); err != nil {
		t.Fatalf("Release failed: %v", err)
	}
	if strings.Contains(files.content("team-a"), "web") {
		t.Errorf("released pin still on disk:\n%s", files.content("team-a"))
	}
	if _, err := s.Get("team-a", "web"); !errors.Is(err, state.ErrImagePinNotFound) {
		t.Errorf("Get after Release = %v, want ErrImagePinNotFound", err)
	}
}

func TestImagePinStore_ReleaseTeamEmptiesTheTeamAndIsIdempotent(t *testing.T) {
	files := newFakePinFiles()
	s := newTestImagePinStore(files)
	_ = s.Put("team-a", webPin())
	_ = s.Put("team-a", dbPin())
	_ = s.Put("team-b", webPin())

	if err := s.ReleaseTeam("team-a"); err != nil {
		t.Fatalf("ReleaseTeam failed: %v", err)
	}
	if pins, _ := s.List("team-a"); len(pins) != 0 {
		t.Errorf("team-a still has pins: %+v", pins)
	}
	if pins, _ := s.List("team-b"); len(pins) != 1 {
		t.Errorf("team-b must be untouched, got %+v", pins)
	}
	writes := files.writes
	if err := s.ReleaseTeam("team-a"); err != nil {
		t.Fatalf("second ReleaseTeam failed: %v", err)
	}
	if files.writes != writes {
		t.Error("releasing an already empty team must not rewrite the file")
	}
}

func TestImagePinStore_ListAllIsKeyedByTeamAndName(t *testing.T) {
	files := newFakePinFiles()
	s := newTestImagePinStore(files)
	_ = s.Put("team-a", webPin())
	_ = s.Put("team-b", dbPin())

	all, err := s.ListAll()
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ListAll = %v, want two entries", all)
	}
	if all["team-a/web"].Pinned != webPin().Pinned || all["team-b/db"].Pinned != dbPin().Pinned {
		t.Errorf("ListAll keys or values wrong: %v", all)
	}
}

func TestImagePinStore_WriteFailureLeavesStateUnchanged(t *testing.T) {
	files := newFakePinFiles()
	s := newTestImagePinStore(files)
	_ = s.Put("team-a", webPin())
	files.writeErr = errors.New("disk full")

	if err := s.Put("team-a", dbPin()); err == nil {
		t.Fatal("Put must surface the write failure")
	}
	if _, err := s.Get("team-a", "db"); !errors.Is(err, state.ErrImagePinNotFound) {
		t.Error("a failed Put must not leave the new pin visible")
	}
	if err := s.Release("team-a", "web"); err == nil {
		t.Fatal("Release must surface the write failure")
	}
	if _, err := s.Get("team-a", "web"); err != nil {
		t.Error("a failed Release must keep the pin")
	}
}
