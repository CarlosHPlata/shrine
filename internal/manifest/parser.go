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
	case ApplicationKind:
		var app ApplicationManifest
		if err := yaml.Unmarshal(data, &app); err != nil {
			return nil, fmt.Errorf("parsing Application manifest: %w", err)
		}
		if err := rejectTLSOutsideAliasEntries(data); err != nil {
			return nil, err
		}
		meta.Application = &app
	case ResourceKind:
		var res ResourceManifest
		if err := yaml.Unmarshal(data, &res); err != nil {
			return nil, fmt.Errorf("parsing Resource manifest: %w", err)
		}
		// Default image to type:version when not explicitly set
		if res.Spec.Image == "" && res.Spec.Type != "" && res.Spec.Version != "" {
			res.Spec.Image = res.Spec.Type + ":" + res.Spec.Version
		}
		meta.Resource = &res
	case TeamKind:
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

// rejectTLSOutsideAliasEntries enforces FR-005: the `tls` field is only valid
// inside a spec.routing.aliases[] entry, never at spec.routing top level.
// The strongly-typed unmarshal silently drops unknown fields, so we re-walk
// the raw YAML to catch this one specific misuse.
func rejectTLSOutsideAliasEntries(data []byte) error {
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil
	}
	spec, ok := doc["spec"].(map[string]any)
	if !ok {
		return nil
	}
	routing, ok := spec["routing"].(map[string]any)
	if !ok {
		return nil
	}
	if _, hasTLS := routing["tls"]; hasTLS {
		return fmt.Errorf("parsing Application manifest %q: field tls is not valid at spec.routing; tls is only valid inside an alias entry under spec.routing.aliases[]", applicationName(doc))
	}
	return nil
}

func applicationName(doc map[string]any) string {
	meta, ok := doc["metadata"].(map[string]any)
	if !ok {
		return "<unknown>"
	}
	name, ok := meta["name"].(string)
	if !ok || name == "" {
		return "<unknown>"
	}
	return name
}
