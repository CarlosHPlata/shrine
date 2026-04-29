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
		kinds := countSet(e.Value != "", e.ValueFrom != "", e.Template != "")
		errs = append(errs, validateExclusiveFields("spec.env", i, e.Name, "value/valueFrom/template", kinds)...)
	}

	// validate volumes
	if spec.Volumes != nil {
		errs = append(errs, validateVolumeMounts(spec.Volumes)...)
	}
	return errs
}

func validateResourceSpec(spec ResourceSpec) []string {
	var errs []string
	if spec.Type == "" {
		errs = append(errs, "spec.type is required")
	}
	if spec.Version == "" {
		errs = append(errs, "spec.version is required")
	}

	// Validate outputs: each must have a name, names must be unique. Every
	// output must declare exactly one of value/generated/template, except the
	// built-ins `host` and `port` which must declare none (the CLI fills them in).
	seen := make(map[string]bool)
	for i, o := range spec.Outputs {
		if o.Name == "" {
			errs = append(errs, fmt.Sprintf("spec.outputs[%d].name is required", i))
			continue
		}
		if seen[o.Name] {
			errs = append(errs, fmt.Sprintf("spec.outputs has duplicate name %q", o.Name))
		}
		seen[o.Name] = true

		kinds := countSet(o.Value != "", o.Generated, o.Template != "")
		if o.Name == "host" || o.Name == "port" {
			if kinds > 0 {
				errs = append(errs, fmt.Sprintf("spec.outputs[%d]: %q is a CLI built-in and must not set value/generated/template", i, o.Name))
			}
			continue
		}

		errs = append(errs, validateExclusiveFields("spec.outputs", i, o.Name, "value/generated/template", kinds)...)
	}

	// validate volumes
	if spec.Volumes != nil {
		errs = append(errs, validateVolumeMounts(spec.Volumes)...)
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
