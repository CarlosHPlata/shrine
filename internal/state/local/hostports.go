package local

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

type readFileFn func(path string) ([]byte, error)
type writeFileFn func(path string, data []byte) error

type HostPortStore struct {
	baseDir   string
	reserved  map[int]struct{}
	ports     map[string]int   // "team/app" -> host port
	taken     map[int]struct{} // reserved + allocated ports
	mu        sync.Mutex
	readFile  readFileFn
	writeFile writeFileFn
}

// NewHostPortStore creates a filesystem-backed HostPortStore. The reserved
// ports (the gateway's own bindings) seed the taken set but are never
// persisted, allocated, or claimable.
func NewHostPortStore(baseDir string, reserved []int) (state.HostPortStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("creating state directory: %w", err)
	}
	return newHostPortStoreWithFileOps(baseDir, reserved, os.ReadFile, atomicWriteFile)
}

// newHostPortStoreWithFileOps is the injectable-file-ops constructor unit
// tests use to keep the filesystem out of the picture.
func newHostPortStoreWithFileOps(baseDir string, reserved []int, read readFileFn, write writeFileFn) (state.HostPortStore, error) {
	s := &HostPortStore{
		baseDir:   baseDir,
		reserved:  make(map[int]struct{}, len(reserved)),
		ports:     make(map[string]int),
		taken:     make(map[int]struct{}, len(reserved)),
		readFile:  read,
		writeFile: write,
	}
	for _, p := range reserved {
		s.reserved[p] = struct{}{}
		s.taken[p] = struct{}{}
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func atomicWriteFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "hostports-*.txt.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temporary file: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("renaming temporary file: %w", err)
	}
	return nil
}

func (s *HostPortStore) path() string {
	return filepath.Join(s.baseDir, "hostports.txt")
}

// load populates the internal maps from hostports.txt; the caller must hold
// s.mu or guarantee single-threaded access.
func (s *HostPortStore) load() error {
	data, err := s.readFile(s.path())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("reading hostports file: %w", err)
	}

	for _, raw := range strings.Split(string(data), "\n") {
		line, _, _ := strings.Cut(raw, "#")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		key, portStr, found := strings.Cut(line, "=")
		if !found || !strings.Contains(key, "/") {
			continue // be forgiving on malformed lines
		}
		port, err := strconv.Atoi(strings.TrimSpace(portStr))
		if err != nil {
			continue
		}

		s.ports[strings.TrimSpace(key)] = port
		s.taken[port] = struct{}{}
	}
	return nil
}

// save writes the allocations sorted by key; the caller must hold s.mu.
func (s *HostPortStore) save() error {
	keys := make([]string, 0, len(s.ports))
	for key := range s.ports {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "%s=%d\n", key, s.ports[key])
	}
	return s.writeFile(s.path(), []byte(b.String()))
}

func (s *HostPortStore) AllocateHostPort(team, app string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := state.HostPortKey(team, app)
	if port, ok := s.ports[key]; ok {
		return port, nil
	}

	for port := manifest.FirstAutoHostPort; port <= manifest.LastAutoHostPort; port++ {
		if _, used := s.taken[port]; used {
			continue
		}
		s.ports[key] = port
		s.taken[port] = struct{}{}
		if err := s.save(); err != nil {
			delete(s.ports, key)
			delete(s.taken, port)
			return 0, fmt.Errorf("persisting host port allocation for %q: %w", key, err)
		}
		return port, nil
	}

	return 0, state.ErrNoAvailableHostPorts
}

func (s *HostPortStore) ClaimHostPort(team, app string, port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := state.HostPortKey(team, app)
	if current, ok := s.ports[key]; ok && current == port {
		return nil
	}

	if _, isReserved := s.reserved[port]; isReserved {
		return fmt.Errorf("host port %d is reserved by the platform gateway: %w", port, state.ErrHostPortTaken)
	}
	if holder, held := s.holderOf(port); held && holder != key {
		return fmt.Errorf("host port %d is held by %q: %w", port, holder, state.ErrHostPortTaken)
	}

	previous, hadPrevious := s.ports[key]
	s.ports[key] = port
	s.taken[port] = struct{}{}
	if hadPrevious {
		delete(s.taken, previous)
	}

	if err := s.save(); err != nil {
		delete(s.taken, port)
		if hadPrevious {
			s.ports[key] = previous
			s.taken[previous] = struct{}{}
		} else {
			delete(s.ports, key)
		}
		return fmt.Errorf("persisting host port claim for %q: %w", key, err)
	}
	return nil
}

// holderOf reports which key currently holds port; the caller must hold s.mu.
func (s *HostPortStore) holderOf(port int) (string, bool) {
	for key, p := range s.ports {
		if p == port {
			return key, true
		}
	}
	return "", false
}

func (s *HostPortStore) GetHostPort(team, app string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	port, ok := s.ports[state.HostPortKey(team, app)]
	if !ok {
		return 0, state.ErrHostPortNotFound
	}
	return port, nil
}

func (s *HostPortStore) ReleaseHostPort(team, app string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := state.HostPortKey(team, app)
	port, ok := s.ports[key]
	if !ok {
		return nil // idempotent
	}

	delete(s.ports, key)
	delete(s.taken, port)
	if err := s.save(); err != nil {
		s.ports[key] = port
		s.taken[port] = struct{}{}
		return fmt.Errorf("persisting host port release for %q: %w", key, err)
	}
	return nil
}

func (s *HostPortStore) ReleaseTeamHostPorts(team string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prefix := team + "/"
	released := make(map[string]int)
	for key, port := range s.ports {
		if strings.HasPrefix(key, prefix) {
			released[key] = port
		}
	}
	if len(released) == 0 {
		return nil // idempotent
	}

	for key, port := range released {
		delete(s.ports, key)
		delete(s.taken, port)
	}
	if err := s.save(); err != nil {
		for key, port := range released {
			s.ports[key] = port
			s.taken[port] = struct{}{}
		}
		return fmt.Errorf("persisting host port release for team %q: %w", team, err)
	}
	return nil
}

func (s *HostPortStore) ListHostPorts() (state.HostPortMap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := make(state.HostPortMap, len(s.ports))
	maps.Copy(res, s.ports)
	return res, nil
}
