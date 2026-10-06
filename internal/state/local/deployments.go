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

	"github.com/CarlosHPlata/shrine/internal/state"
)

type DeploymentStore struct {
	mu        sync.Mutex
	baseDir   string
	readFile  readFileFn
	writeFile writeFileFn
}

func NewDeploymentStore(baseDir string) (state.DeploymentStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("creating state directory: %w", err)
	}
	return newDeploymentStoreWithFileOps(baseDir, os.ReadFile, writeTeamFile), nil
}

// newDeploymentStoreWithFileOps is the injectable-file-ops constructor unit
// tests use to keep the filesystem out of the picture.
func newDeploymentStoreWithFileOps(baseDir string, read readFileFn, write writeFileFn) *DeploymentStore {
	return &DeploymentStore{
		baseDir:   baseDir,
		readFile:  read,
		writeFile: write,
	}
}

func writeTeamFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("creating team directory: %w", err)
	}
	return atomicWriteFile(path, data)
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

	deploymentsSlice := make([]state.Deployment, 0, len(deployments))
	for _, deployment := range deployments {
		deploymentsSlice = append(deploymentsSlice, deployment)
	}

	return deploymentsSlice, nil
}

func (s *DeploymentStore) teamPath(team string) string {
	return filepath.Join(s.baseDir, team, "deployments.txt")
}

func (s *DeploymentStore) loadTeam(team string) (map[string]state.Deployment, error) {
	data, err := s.readFile(s.teamPath(team))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return make(map[string]state.Deployment), nil
		}
		return nil, fmt.Errorf("reading deployments for team %q: %w", team, err)
	}

	deployments := make(map[string]state.Deployment)
	for _, raw := range strings.Split(string(data), "\n") {
		line, _, _ := strings.Cut(raw, "#")
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		deployments[fields[1]] = state.Deployment{
			Kind:        fields[0],
			Name:        fields[1],
			ContainerID: fields[2],
			ConfigHash:  fieldAt(fields, 3),
			Image:       fieldAt(fields, 4),
			Policy:      fieldAt(fields, 5),
		}
	}

	return deployments, nil
}

// emptyFieldPlaceholder keeps later fields in position on disk when an
// optional value is empty; it never reaches the in-memory record.
const emptyFieldPlaceholder = "-"

// fieldAt reads an optional field as empty when the line predates it or
// holds the placeholder, so records written by earlier releases keep loading.
func fieldAt(fields []string, index int) string {
	if index >= len(fields) || fields[index] == emptyFieldPlaceholder {
		return ""
	}
	return fields[index]
}

func fieldOrPlaceholder(value string) string {
	if value == "" {
		return emptyFieldPlaceholder
	}
	return value
}

func (s *DeploymentStore) saveTeam(team string, deployments map[string]state.Deployment) error {
	names := make([]string, 0, len(deployments))
	for name := range deployments {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		d := deployments[name]
		fmt.Fprintf(&b, "%s %s %s %s %s %s\n",
			d.Kind, d.Name, d.ContainerID,
			fieldOrPlaceholder(d.ConfigHash), fieldOrPlaceholder(d.Image), fieldOrPlaceholder(d.Policy))
	}

	if err := s.writeFile(s.teamPath(team), []byte(b.String())); err != nil {
		return fmt.Errorf("writing deployments for team %q: %w", team, err)
	}
	return nil
}
