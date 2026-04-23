package local

import (
	"bufio"
	"fmt"
	"maps"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/CarlosHPlata/shrine/internal/state"
)

// Reservation range for team subnets (10.100.X.0/24)
const (
	firstUsableOctet uint8 = 5
	lastUsableOctet  uint8 = 255
)

type SubnetStore struct {
	baseDir string
	subnets map[string]string  // team -> CIDR
	taken   map[uint8]struct{} // third-octet set
	mu      sync.Mutex
}

// NewSubnetStore creates a new filesystem-backed SubnetStore.
func NewSubnetStore(baseDir string) (state.SubnetStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("creating state directory: %w", err)
	}

	s := &SubnetStore{
		baseDir: baseDir,
		subnets: make(map[string]string),
		taken:   make(map[uint8]struct{}),
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *SubnetStore) path() string {
	return filepath.Join(s.baseDir, "subnets.txt")
}

// load reads subnets.txt from disk and populates the internal maps.
// It is an internal helper; the caller must hold s.mu or ensure single-threaded access.
func (s *SubnetStore) load() error {
	f, err := os.Open(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("opening subnets file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// Strip comments and leading/trailing whitespace
		line, _, _ := strings.Cut(scanner.Text(), "#")
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // be forgiving on malformed lines
		}

		team, cidr := parts[0], parts[1]
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}

		ipv4 := ip.To4()
		if ipv4 == nil {
			continue
		}

		s.subnets[team] = cidr
		s.taken[ipv4[2]] = struct{}{}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading subnets file: %w", err)
	}

	return nil
}

// save writes the current subnets map to disk in a deterministic order.
// It is an internal helper; the caller must hold s.mu.
func (s *SubnetStore) save() error {
	tmpFile, err := os.CreateTemp(s.baseDir, "subnets-*.txt.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	teams := make([]string, 0, len(s.subnets))
	for team := range s.subnets {
		teams = append(teams, team)
	}
	sort.Strings(teams)

	for _, team := range teams {
		cidr := s.subnets[team]
		if _, err := fmt.Fprintf(tmpFile, "%s=%s\n", team, cidr); err != nil {
			tmpFile.Close()
			return fmt.Errorf("writing to temporary file: %w", err)
		}
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temporary file: %w", err)
	}

	if err := os.Rename(tmpFile.Name(), s.path()); err != nil {
		return fmt.Errorf("renaming temporary file: %w", err)
	}

	return nil
}

// Interface implementation stubs

func (s *SubnetStore) AllocateSubnet(team string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cidr, ok := s.subnets[team]; ok {
		return cidr, nil
	}

	for i := int(firstUsableOctet); i <= int(lastUsableOctet); i++ {
		if _, ok := s.taken[uint8(i)]; !ok {
			s.taken[uint8(i)] = struct{}{}
			s.subnets[team] = fmt.Sprintf("10.100.%d.0/24", i)
			if err := s.save(); err != nil {
				delete(s.taken, uint8(i))
				delete(s.subnets, team)
				return "", fmt.Errorf("persisting subnet allocation for %q: %w", team, err)
			}
			return s.subnets[team], nil
		}
	}

	return "", state.ErrNoAvailableSubnets
}

func (s *SubnetStore) GetSubnet(team string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cidr, ok := s.subnets[team]; ok {
		return cidr, nil
	}

	return "", state.ErrSubnetNotFound
}

func (s *SubnetStore) ReleaseSubnet(team string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cidr, ok := s.subnets[team]
	if !ok {
		return nil // idempotent
	}

	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("parsing stored CIDR %q: %w", cidr, err)
	}

	ipv4 := ip.To4()
	if ipv4 != nil {
		delete(s.taken, ipv4[2])
	}
	delete(s.subnets, team)

	return s.save()
}

func (s *SubnetStore) ListSubnets() (state.SubnetMap, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	res := make(state.SubnetMap, len(s.subnets))
	maps.Copy(res, s.subnets)

	return res, nil
}
