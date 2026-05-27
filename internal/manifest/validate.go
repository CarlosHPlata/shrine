package manifest

import (
	"fmt"
	"strings"
)

func Validate(m *Manifest) error {
	var errs []string

	errs = append(errs, validateTypeMeta(m.TypeMeta)...)
	errs = append(errs, validateMetadata(m)...)
	errs = append(errs, validateSpec(m)...)

	if len(errs) > 0 {
		return fmt.Errorf("validation failed:\n- %s", strings.Join(errs, "\n- "))
	}
	return nil
}

func validateTypeMeta(meta TypeMeta) []string {
	var errs []string
	if meta.Kind == "" {
		errs = append(errs, "kind is required")
	} else if meta.Kind != TeamKind && meta.Kind != ResourceKind && meta.Kind != ApplicationKind {
		errs = append(errs, "kind must be one of: Team, Resource, Application")
	}
	if meta.APIVersion == "" {
		errs = append(errs, "apiVersion is required")
	}
	return errs
}

func validateMetadata(m *Manifest) []string {
	switch {
	case m.Application != nil:
		return validateMetadataWithOwner(m.Application.Metadata)
	case m.Resource != nil:
		return validateMetadataWithOwner(m.Resource.Metadata)
	case m.Team != nil:
		return validateMetadataName(m.Team.Metadata)
	default:
		return nil
	}
}

func validateMetadataName(meta Metadata) []string {
	var errs []string
	if meta.Name == "" {
		errs = append(errs, "metadata.name is required")
	}
	return errs
}

func validateMetadataWithOwner(meta Metadata) []string {
	errs := validateMetadataName(meta)
	if meta.Owner == "" {
		errs = append(errs, "metadata.owner is required")
	}
	return errs
}

func validateSpec(m *Manifest) []string {
	switch {
	case m.Application != nil:
		return validateApplicationSpec(m.Application.Spec)
	case m.Resource != nil:
		return validateResourceSpec(m.Resource.Spec)
	case m.Team != nil:
		return validateTeamSpec(m.Team.Spec)
	default:
		return nil
	}
}

func validateApplicationSpec(spec ApplicationSpec) []string {
	var errs []string
	if spec.Image == "" {
		errs = append(errs, "spec.image is required")
	}
	if spec.Port <= 0 {
		errs = append(errs, "spec.port must be greater than 0")
	}
	for i, e := range spec.Env {
		if e.Name == "" {
			errs = append(errs, fmt.Sprintf("spec.env[%d].name is required", i))
			continue
		}
		if e.Generated {
			errs = append(errs, fmt.Sprintf("spec.env[%d] %q: generated is only valid on Resource env", i, e.Name))
		}
		kinds := countSet(e.Value != "", e.ValueFrom != "", e.Template != "")
		errs = append(errs, validateExclusiveFields("spec.env", i, e.Name, "value/valueFrom/template", kinds)...)
	}

	errs = append(errs, validateRoutingAliases(spec.Routing)...)

	// validate volumes
	if spec.Volumes != nil {
		errs = append(errs, validateVolumeMounts(spec.Volumes)...)
	}
	return errs
}

// reservedBuiltinNames are resolved from CLI built-ins or metadata; a resource
// env var must not shadow them so that output names and templates stay
// unambiguous.
var reservedBuiltinNames = map[string]bool{"host": true, "port": true, "team": true, "name": true}

func validateResourceSpec(spec ResourceSpec) []string {
	var errs []string
	if spec.Type == "" {
		errs = append(errs, "spec.type is required")
	}
	if spec.Version == "" {
		errs = append(errs, "spec.version is required")
	}

	envNames := make(map[string]bool, len(spec.Env))
	errs = append(errs, validateResourceEnv(spec.Env, envNames)...)
	errs = append(errs, validateResourceOutputs(spec.Outputs, envNames)...)

	// validate volumes
	if spec.Volumes != nil {
		errs = append(errs, validateVolumeMounts(spec.Volumes)...)
	}

	return errs
}

// validateResourceEnv enforces the runtime-config block: a name (not a reserved
// built-in, unique) plus exactly one of value/valueFrom/template/generated. It
// records every valid name in names for output cross-referencing.
func validateResourceEnv(env []EnvVar, names map[string]bool) []string {
	var errs []string
	for i, e := range env {
		if e.Name == "" {
			errs = append(errs, fmt.Sprintf("spec.env[%d].name is required", i))
			continue
		}
		if reservedBuiltinNames[e.Name] {
			errs = append(errs, fmt.Sprintf("spec.env[%d] %q uses a reserved built-in name (host, port, team, name)", i, e.Name))
		}
		if names[e.Name] {
			errs = append(errs, fmt.Sprintf("spec.env has duplicate name %q", e.Name))
		}
		names[e.Name] = true

		kinds := countSet(e.Value != "", e.ValueFrom != "", e.Template != "", e.Generated)
		errs = append(errs, validateExclusiveFields("spec.env", i, e.Name, "value/valueFrom/template/generated", kinds)...)
	}
	return errs
}

// validateResourceOutputs enforces the export allowlist: a name (unique) plus an
// optional template only. A pre-split output that still sets value/valueFrom/
// generated is rejected with migration guidance; a non-template name must match
// a declared env var or the host/port built-in.
func validateResourceOutputs(outputs []Output, envNames map[string]bool) []string {
	var errs []string
	seen := make(map[string]bool)
	for i, o := range outputs {
		if o.Name == "" {
			errs = append(errs, fmt.Sprintf("spec.outputs[%d].name is required", i))
			continue
		}
		if seen[o.Name] {
			errs = append(errs, fmt.Sprintf("spec.outputs has duplicate name %q", o.Name))
		}
		seen[o.Name] = true

		if o.Value != "" || o.Generated || o.ValueFrom != "" {
			errs = append(errs, fmt.Sprintf("spec.outputs[%d] %q must not set value/valueFrom/generated — those fields are deprecated on outputs; declare it under spec.env and list its name under spec.outputs to export it", i, o.Name))
			continue
		}

		if o.Template != "" {
			continue
		}

		if envNames[o.Name] || o.Name == "host" || o.Name == "port" {
			continue
		}
		errs = append(errs, fmt.Sprintf("spec.outputs[%d] %q has no template and matches no env var or built-in (host, port)", i, o.Name))
	}
	return errs
}

func hasInvalidChars(s string) bool {
	for _, r := range s {
		if r == ' ' || r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func normalizePathPrefix(p string) string {
	return strings.TrimRight(p, "/")
}

func validateRoutingAliases(routing Routing) []string {
	if len(routing.Aliases) == 0 {
		return nil
	}

	var errs []string

	if routing.Domain == "" {
		errs = append(errs, "spec.routing.aliases is set but spec.routing.domain is empty")
	}

	type routeKey struct{ host, path string }
	seen := make(map[routeKey]bool)
	seen[routeKey{routing.Domain, normalizePathPrefix(routing.PathPrefix)}] = true

	for i, a := range routing.Aliases {
		if a.Host == "" {
			errs = append(errs, fmt.Sprintf("spec.routing.aliases[%d].host is required", i))
			continue
		}

		if hasInvalidChars(a.Host) {
			errs = append(errs, fmt.Sprintf("spec.routing.aliases[%d].host %q contains invalid characters", i, a.Host))
		}

		if a.PathPrefix != "" {
			normPath := normalizePathPrefix(a.PathPrefix)
			if !strings.HasPrefix(a.PathPrefix, "/") {
				errs = append(errs, fmt.Sprintf("spec.routing.aliases[%d].pathPrefix %q must start with \"/\"", i, a.PathPrefix))
			} else if normPath == "" {
				errs = append(errs, fmt.Sprintf("spec.routing.aliases[%d].pathPrefix must not be just \"/\"", i))
			} else if hasInvalidChars(a.PathPrefix) {
				errs = append(errs, fmt.Sprintf("spec.routing.aliases[%d].pathPrefix %q contains invalid characters", i, a.PathPrefix))
			}
		}

		normPath := normalizePathPrefix(a.PathPrefix)
		key := routeKey{a.Host, normPath}
		if seen[key] {
			errs = append(errs, fmt.Sprintf("spec.routing: duplicate route %s%s declared on alias[%d]", a.Host, normPath, i))
		}
		seen[key] = true
	}

	return errs
}

func validateVolumeMounts(mounts []VolumeMount) []string {
	var errs []string
	names := make(map[string]bool)
	paths := make(map[string]bool)

	for i, m := range mounts {
		if m.Name == "" {
			errs = append(errs, fmt.Sprintf("spec.volumes[%d].name is required", i))
			continue
		} else if names[m.Name] {
			errs = append(errs, fmt.Sprintf("spec.volumes has duplicate name %q", m.Name))
		}
		names[m.Name] = true

		if m.MountPath == "" {
			errs = append(errs, fmt.Sprintf("spec.volumes[%d].mountPath is required", i))
		} else if !strings.HasPrefix(m.MountPath, "/") {
			errs = append(errs, fmt.Sprintf("spec.volumes[%d].mountPath %q must be absolute (starts with /)", i, m.MountPath))
		} else if paths[m.MountPath] {
			errs = append(errs, fmt.Sprintf("spec.volumes has duplicate mountPath %q", m.MountPath))
		}
		paths[m.MountPath] = true
	}
	return errs
}

func validateTeamSpec(spec TeamSpec) []string {
	var errs []string
	if spec.DisplayName == "" {
		errs = append(errs, "spec.displayName is required")
	}
	if spec.Contact == "" {
		errs = append(errs, "spec.contact is required")
	}
	return errs
}

func countSet(conds ...bool) int {
	n := 0
	for _, c := range conds {
		if c {
			n++
		}
	}
	return n
}

func validateExclusiveFields(path string, index int, name string, labels string, count int) []string {
	var errs []string
	switch {
	case count == 0:
		errs = append(errs, fmt.Sprintf("%s[%d] %q: must set one of %s", path, index, name, labels))
	case count > 1:
		errs = append(errs, fmt.Sprintf("%s[%d] %q: %s are mutually exclusive", path, index, name, labels))
	}
	return errs
}
