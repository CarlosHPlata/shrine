package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Manifest struct {
	TypeMeta    `yaml:",inline"`
	Resource    *ResourceManifest
	Application *ApplicationManifest
	Team        *TeamManifest
}

func Parse(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest file: %w", err)
	}

	meta, err := probeKind(data)
	if err != nil {
		return nil, err
	}

	return parseManifest(meta, data)
}

func probeKind(data []byte) (*Manifest, error) {
	var meta TypeMeta
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing manifest metadata: %w", err)
	}

	m := &Manifest{TypeMeta: meta}
	return m, nil
}

func parseManifest(meta *Manifest, data []byte) (*Manifest, error) {
	switch meta.TypeMeta.Kind {
	case "Application":
		var app ApplicationManifest
		if err := yaml.Unmarshal(data, &app); err != nil {
			return nil, fmt.Errorf("parsing Application manifest: %w", err)
		}
		meta.Application = &app
	case "Resource":
		var res ResourceManifest
		if err := yaml.Unmarshal(data, &res); err != nil {
			return nil, fmt.Errorf("parsing Resource manifest: %w", err)
		}
		meta.Resource = &res
	case "Team":
		var team TeamManifest
		if err := yaml.Unmarshal(data, &team); err != nil {
			return nil, fmt.Errorf("parsing Team manifest: %w", err)
		}
		meta.Team = &team
	default:
		return nil, fmt.Errorf("unknown manifest kind: %q", meta.Kind)
	}
	return meta, nil
}
