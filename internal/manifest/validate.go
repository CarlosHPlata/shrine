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
	} else if meta.Kind != "Team" && meta.Kind != "Resource" && meta.Kind != "Application" {
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

	// Validate outputs: each must have a name, and names must be unique
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
