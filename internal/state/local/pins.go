package local

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CarlosHPlata/shrine/internal/state"
)

const pinsFileName = "pins.txt"

type listDirsFn func(path string) ([]string, error)

// ImagePinStore keeps one pins.txt per team under the state directory.
type ImagePinStore struct {
	mu        sync.Mutex
	baseDir   string
	readFile  readFileFn
	writeFile writeFileFn
	listDirs  listDirsFn
}

// NewImagePinStore creates a filesystem-backed ImagePinStore.
func NewImagePinStore(baseDir string) (state.ImagePinStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("creating state directory: %w", err)
	}
	return newImagePinStoreWithFileOps(baseDir, os.ReadFile, writeTeamFile, listSubdirectories), nil
}

// newImagePinStoreWithFileOps is the injectable-file-ops constructor unit
// tests use to keep the filesystem out of the picture.
func newImagePinStoreWithFileOps(baseDir string, read readFileFn, write writeFileFn, listDirs listDirsFn) *ImagePinStore {
	return &ImagePinStore{baseDir: baseDir, readFile: read, writeFile: write, listDirs: listDirs}
}

func listSubdirectories(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

func (s *ImagePinStore) teamPath(team string) string {
	return filepath.Join(s.baseDir, team, pinsFileName)
}

func (s *ImagePinStore) Get(team, name string) (state.ImagePin, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pins, err := s.loadTeam(team)
	if err != nil {
		return state.ImagePin{}, err
	}
	pin, ok := pins[name]
	if !ok {
		return state.ImagePin{}, state.ErrImagePinNotFound
	}
	return pin, nil
}

func (s *ImagePinStore) Put(team string, pin state.ImagePin) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pins, err := s.loadTeam(team)
	if err != nil {
		return err
	}
	pins[pin.Name] = pin
	return s.saveTeam(team, pins)
}

func (s *ImagePinStore) Release(team, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pins, err := s.loadTeam(team)
	if err != nil {
		return err
	}
	if _, ok := pins[name]; !ok {
		return nil // idempotent
	}
	delete(pins, name)
	return s.saveTeam(team, pins)
}

func (s *ImagePinStore) ReleaseTeam(team string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pins, err := s.loadTeam(team)
	if err != nil {
		return err
	}
	if len(pins) == 0 {
		return nil // idempotent
	}
	return s.saveTeam(team, map[string]state.ImagePin{})
}

func (s *ImagePinStore) List(team string) ([]state.ImagePin, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	pins, err := s.loadTeam(team)
	if err != nil {
		return nil, err
	}
	return sortedPins(pins), nil
}

func (s *ImagePinStore) ListAll() (map[string]state.ImagePin, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	teams, err := s.listDirs(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("listing state directory: %w", err)
	}

	all := make(map[string]state.ImagePin)
	for _, team := range teams {
		pins, err := s.loadTeam(team)
		if err != nil {
			return nil, err
		}
		for name, pin := range pins {
			all[state.ImagePinKey(team, name)] = pin
		}
	}
	return all, nil
}

// loadTeam reads a team's pins.txt, skipping comments and malformed lines;
// the caller must hold s.mu.
func (s *ImagePinStore) loadTeam(team string) (map[string]state.ImagePin, error) {
	pins := make(map[string]state.ImagePin)

	data, err := s.readFile(s.teamPath(team))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return pins, nil
		}
		return nil, fmt.Errorf("reading image pins for team %q: %w", team, err)
	}

	for _, raw := range strings.Split(string(data), "\n") {
		line, _, _ := strings.Cut(raw, "#")
		pin, ok := parsePinLine(line)
		if ok {
			pins[pin.Name] = pin
		}
	}
	return pins, nil
}

func parsePinLine(line string) (state.ImagePin, bool) {
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return state.ImagePin{}, false
	}
	if !strings.Contains(fields[3], "@sha256:") {
		return state.ImagePin{}, false
	}
	pinnedAt, err := time.Parse(time.RFC3339, fields[4])
	if err != nil {
		return state.ImagePin{}, false
	}
	return state.ImagePin{
		Kind:      fields[0],
		Name:      fields[1],
		Requested: fields[2],
		Pinned:    fields[3],
		PinnedAt:  pinnedAt.UTC(),
	}, true
}

// saveTeam writes the pins sorted by name; the caller must hold s.mu.
func (s *ImagePinStore) saveTeam(team string, pins map[string]state.ImagePin) error {
	var b strings.Builder
	for _, pin := range sortedPins(pins) {
		fmt.Fprintf(&b, "%s %s %s %s %s\n",
			pin.Kind, pin.Name, pin.Requested, pin.Pinned, pin.PinnedAt.UTC().Format(time.RFC3339))
	}
	if err := s.writeFile(s.teamPath(team), []byte(b.String())); err != nil {
		return fmt.Errorf("writing image pins for team %q: %w", team, err)
	}
	return nil
}

func sortedPins(pins map[string]state.ImagePin) []state.ImagePin {
	sorted := make([]state.ImagePin, 0, len(pins))
	for _, pin := range pins {
		sorted = append(sorted, pin)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	return sorted
}
