package planner

import (
	"errors"
	"fmt"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// ManifestSet holds indexed collections of manifests loaded from a project directory.
// We use maps keyed by metadata.name to ensure O(1) lookup during dependency resolution.
type ManifestSet struct {
	Applications map[string]*manifest.ApplicationManifest
	Resources    map[string]*manifest.ResourceManifest
}

// ErrDuplicateManifest is returned by MergeManifest when the named manifest is
// already present in the set. Callers (notably handler.ApplySingle) use
// errors.Is to detect the duplicate and continue without re-adding.
var ErrDuplicateManifest = errors.New("manifest already present in set")

// NewManifestSet returns an empty set with both maps allocated.
func NewManifestSet() *ManifestSet {
	return &ManifestSet{
		Applications: make(map[string]*manifest.ApplicationManifest),
		Resources:    make(map[string]*manifest.ResourceManifest),
	}
}

func LoadDir(dir string) (*ManifestSet, error) {
	set := NewManifestSet()

	result, err := manifest.ScanDir(dir)
	if err != nil {
		return nil, err
	}

	for _, candidate := range result.Shrine {
		path := candidate.Path
		m, err := manifest.Parse(path)
		if err != nil {
			return nil, fmt.Errorf("parsing manifest %q: %w", path, err)
		}

		if err := manifest.Validate(m); err != nil {
			return nil, fmt.Errorf("validating manifest %q: %w", path, err)
		}

		if err := set.MergeManifest(m, path); err != nil {
			return nil, err
		}
	}

	if len(result.Foreign) > 0 {
		manifest.ReportForeignFiles(dir, result.Foreign)
	}

	return set, nil
}

// MergeManifest adds m to the set under its kind-specific map. Team-kind
// manifests are silent no-ops (handled by the platform sync, not the
// deployment planner). Returns ErrDuplicateManifest wrapped with the name
// when an Application or Resource of the same name is already present.
func (set *ManifestSet) MergeManifest(m *manifest.Manifest, path string) error {
	switch m.Kind {
	case manifest.ApplicationKind:
		name := m.Application.Metadata.Name
		if _, exists := set.Applications[name]; exists {
			return fmt.Errorf("application %q: %w", name, ErrDuplicateManifest)
		}
		set.Applications[name] = m.Application

	case manifest.ResourceKind:
		name := m.Resource.Metadata.Name
		if _, exists := set.Resources[name]; exists {
			return fmt.Errorf("resource %q: %w", name, ErrDuplicateManifest)
		}
		set.Resources[name] = m.Resource

	case manifest.TeamKind:
		return nil

	default:
		return fmt.Errorf("unsupported manifest kind for deployment: %q (file: %s)", m.Kind, path)
	}

	return nil
}
