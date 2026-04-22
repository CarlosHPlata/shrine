package planner

import (
	"fmt"
	"path/filepath"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// ManifestSet holds indexed collections of manifests loaded from a project directory.
// We use maps keyed by metadata.name to ensure O(1) lookup during dependency resolution.
type ManifestSet struct {
	Applications map[string]*manifest.ApplicationManifest
	Resources    map[string]*manifest.ResourceManifest
}

// LoadDir scans the provided directory for .yml and .yaml files, parses them,
// and returns a ManifestSet. It skips Team manifests and errors on duplicate names.
func LoadDir(dir string) (*ManifestSet, error) {
	set := &ManifestSet{
		Applications: make(map[string]*manifest.ApplicationManifest),
		Resources:    make(map[string]*manifest.ResourceManifest),
	}

	files, err := getFiles(dir)
	if err != nil {
		return nil, err
	}

	for _, path := range files {
		m, err := manifest.Parse(path)
		if err != nil {
			// Wrapping errors with the path helps the user locate the broken file
			return nil, fmt.Errorf("parsing manifest %q: %w", path, err)
		}

		if err := manifest.Validate(m); err != nil {
			return nil, fmt.Errorf("validating manifest %q: %w", path, err)
		}

		// Route based on Kind using our private helper
		if err := set.mapKind(m, path); err != nil {
			return nil, err
		}
	}

	return set, nil
}

func getFiles(dir string) ([]string, error) {
	// 1. Discovery: Support both .yml and .yaml for flexibility,
	// maintaining consistency with the filepath.Glob pattern used in ApplyTeams.
	var files []string
	for _, ext := range []string{"*.yml", "*.yaml"} {
		matches, err := filepath.Glob(filepath.Join(dir, ext))
		if err != nil {
			return nil, fmt.Errorf("searching for %s files: %w", ext, err)
		}
		files = append(files, matches...)
	}
	return files, nil
}

// mapKind routes a single manifest into the correct map within the set.
func (set *ManifestSet) mapKind(m *manifest.Manifest, path string) error {
	switch m.Kind {
	case manifest.ApplicationKind:
		name := m.Application.Metadata.Name
		if _, exists := set.Applications[name]; exists {
			return fmt.Errorf("duplicate Application name found: %s", name)
		}
		set.Applications[name] = m.Application

	case manifest.ResourceKind:
		name := m.Resource.Metadata.Name
		if _, exists := set.Resources[name]; exists {
			return fmt.Errorf("duplicate Resource name found: %s", name)
		}
		set.Resources[name] = m.Resource

	case manifest.TeamKind:
		// Teams are "global" infra, handled by the platform sync process.
		// The deployment planner focuses on project-specific apps and resources.
		return nil

	default:
		return fmt.Errorf("unsupported manifest kind for deployment: %q (file: %s)", m.Kind, path)
	}

	return nil
}
