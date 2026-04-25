package local

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/CarlosHPlata/shrine/internal/state"
)

type DeploymentStore struct {
	mu      sync.Mutex
	baseDir string
}

func NewDeploymentStore(baseDir string) (state.DeploymentStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("creating state directory: %w", err)
	}

	s := &DeploymentStore{
		baseDir: baseDir,
	}

	return s, nil
}

func (s *DeploymentStore) Record(team string, deployment state.Deployment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	deployments, err := s.loadTeam(team)
	if err != nil {
		return err
	}

	deployments[deployment.Name] = deployment

	return s.saveTeam(team, deployments)
}

func (s *DeploymentStore) Remove(team string, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	deployments, err := s.loadTeam(team)
	if err != nil {
		return err
	}

	delete(deployments, name)
	return s.saveTeam(team, deployments)
}

func (s *DeploymentStore) List(team string) ([]state.Deployment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	deployments, err := s.loadTeam(team)
	if err != nil {
		return nil, err
	}

	// convert map to slice
	deploymentsSlice := make([]state.Deployment, 0, len(deployments))
	for _, deployment := range deployments {
		deploymentsSlice = append(deploymentsSlice, deployment)
	}

	return deploymentsSlice, nil
}

func (s *DeploymentStore) loadTeam(team string) (map[string]state.Deployment, error) {
	path := filepath.Join(s.baseDir, team, "deployments.txt")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]state.Deployment), nil
		}
		return nil, fmt.Errorf("opening deployments for team %q: %w", team, err)
	}
	defer f.Close()

	deployments := make(map[string]state.Deployment)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line, _, _ := strings.Cut(scanner.Text(), "#")
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		parts := strings.SplitN(line, " ", 4)
		if len(parts) >= 3 {
			d := state.Deployment{
				Kind:        parts[0],
				Name:        parts[1],
				ContainerID: parts[2],
			}
			if len(parts) == 4 {
				d.ConfigHash = parts[3]
			}
			deployments[parts[1]] = d
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading deployments for team %q: %w", team, err)
	}

	return deployments, nil
}

func (s *DeploymentStore) saveTeam(team string, deployments map[string]state.Deployment) error {
	teamDir := filepath.Join(s.baseDir, team)
	if err := os.MkdirAll(teamDir, 0700); err != nil {
		return fmt.Errorf("creating team directory for %q: %w", team, err)
	}

	// creating temp file
	tmp, err := os.CreateTemp(teamDir, "deployments-*.txt.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary deployments file: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	// Sort deployment by name
	keys := make([]string, 0, len(deployments))
	for k := range deployments {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if _, err := fmt.Fprintf(tmp, "%s %s %s %s\n", deployments[k].Kind, deployments[k].Name, deployments[k].ContainerID, deployments[k].ConfigHash); err != nil {
			return fmt.Errorf("writing to temporary deployments file: %w", err)
		}
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temporary deployments file: %w", err)
	}

	destPath := filepath.Join(teamDir, "deployments.txt")
	if err := os.Rename(tmp.Name(), destPath); err != nil {
		return fmt.Errorf("finalizing deployments file for %q: %w", team, err)
	}

	return nil
}
