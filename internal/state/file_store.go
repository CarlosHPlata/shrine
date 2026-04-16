package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/google/uuid"
)

type FileStore struct {
	baseDir string
}

// NewFileStore creates a new filesystem-based store in the target directory.
// It ensures that the required directory structure exists.
func NewFileStore(baseDir string) (*FileStore, error) {
	fs := &FileStore{baseDir: baseDir}
	if err := os.MkdirAll(fs.teamsDir(), 0755); err != nil {
		return nil, fmt.Errorf("creating teams directory: %w", err)
	}
	return fs, nil
}

func (fs *FileStore) teamsDir() string {
	return filepath.Join(fs.baseDir, "teams")
}

func (fs *FileStore) teamPath(name string) string {
	return filepath.Join(fs.teamsDir(), strings.ToLower(name)+".json")
}

func (fs *FileStore) SaveTeam(team *manifest.TeamManifest) error {
	if team.Metadata.ResourceID == "" {
		team.Metadata.ResourceID = uuid.New().String()
	}

	path := fs.teamPath(team.Metadata.Name)

	// Create temp file for atomic write
	tmpFile, err := os.CreateTemp(fs.teamsDir(), "team-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temporary file: %w", err)
	}
	defer os.Remove(tmpFile.Name()) // Clean up if rename fails

	encoder := json.NewEncoder(tmpFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(team); err != nil {
		tmpFile.Close()
		return fmt.Errorf("encoding team to JSON: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temporary file: %w", err)
	}

	if err := os.Rename(tmpFile.Name(), path); err != nil {
		return fmt.Errorf("renaming temporary file to %q: %w", path, err)
	}

	return nil
}

func (fs *FileStore) LoadTeam(name string) (*manifest.TeamManifest, error) {
	path := fs.teamPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("team %q not found %w", name, err)
		}
		return nil, fmt.Errorf("reading team file: %w", err)
	}

	var team manifest.TeamManifest
	if err := json.Unmarshal(data, &team); err != nil {
		return nil, fmt.Errorf("unmarshaling team JSON: %w", err)
	}

	return &team, nil
}

func (fs *FileStore) ListTeams() ([]*manifest.TeamManifest, error) {
	entries, err := os.ReadDir(fs.teamsDir())
	if err != nil {
		return nil, fmt.Errorf("reading teams directory: %w", err)
	}

	var teams []*manifest.TeamManifest
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		name := entry.Name()[:len(entry.Name())-5] // strip .json
		team, err := fs.LoadTeam(name)
		if err != nil {
			fmt.Printf("Warning: failed to load team file %q: %v\n", entry.Name(), err)
			continue
		}
		teams = append(teams, team)
	}

	return teams, nil
}

func (fs *FileStore) DeleteTeam(name string) error {
	path := fs.teamPath(name)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("team %q not found", name)
		}
		return fmt.Errorf("deleting team file: %w", err)
	}
	return nil
}
