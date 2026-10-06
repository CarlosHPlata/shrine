package planner

import (
	"fmt"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// pullPolicySource names the precedence layer that supplied an artifact's
// effective image pull policy.
type pullPolicySource int

const (
	policyFromManifest pullPolicySource = iota
	policyFromDefault
	policyFromDerivedRule
)

const configSourcedRemedy = " (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default"

func pullPolicySourceOf(declared, defaultPullPolicy string) pullPolicySource {
	switch {
	case declared != "":
		return policyFromManifest
	case defaultPullPolicy != "":
		return policyFromDefault
	default:
		return policyFromDerivedRule
	}
}

func policySourceKey(kind, name string) string {
	return kind + "/" + name
}

// recordPullPolicySource remembers the layer a policy came from, because
// normalisation overwrites the field and the validator must still tell a
// configured Pinned from a declared one.
func (s *ManifestSet) recordPullPolicySource(kind, name string, src pullPolicySource) {
	if s.pullPolicySources == nil {
		s.pullPolicySources = map[string]pullPolicySource{}
	}
	s.pullPolicySources[policySourceKey(kind, name)] = src
}

// isPolicyFromDefault reads as manifest-sourced for a set that was never
// normalised: whoever built it set the field.
func (s *ManifestSet) isPolicyFromDefault(kind, name string) bool {
	return s.pullPolicySources[policySourceKey(kind, name)] == policyFromDefault
}

// applyEffectivePullPolicy writes the effective image pull policy back into
// every manifest of the set: the manifest's own field, else defaultPullPolicy,
// else the derived rule. Everything downstream then reads one value (TD-7).
func applyEffectivePullPolicy(set *ManifestSet, defaultPullPolicy string) {
	for _, app := range set.Applications {
		set.recordPullPolicySource("application", app.Metadata.Name, pullPolicySourceOf(app.Spec.ImagePullPolicy, defaultPullPolicy))
		app.Spec.ImagePullPolicy = manifest.EffectivePullPolicyWithDefault(app.Spec.Image, app.Spec.ImagePullPolicy, defaultPullPolicy)
	}
	for _, res := range set.Resources {
		set.recordPullPolicySource("resource", res.Metadata.Name, pullPolicySourceOf(res.Spec.ImagePullPolicy, defaultPullPolicy))
		res.Spec.ImagePullPolicy = manifest.EffectivePullPolicyWithDefault(res.Spec.Image, res.Spec.ImagePullPolicy, defaultPullPolicy)
	}
}

// validateImagePolicies enforces the version rules that depend on the
// effective policy: under Pinned no fixed version may be named; under the
// other two a Resource version is required. A violation names the
// configuration setting when the policy came from it.
func validateImagePolicies(set *ManifestSet) []error {
	var errs []error
	for _, app := range set.Applications {
		if app.Spec.ImagePullPolicy == manifest.ImagePullPolicyPinned && namesFixedVersion(app.Spec.Image) {
			fromDefault := set.isPolicyFromDefault("application", app.Metadata.Name)
			errs = append(errs, fixedVersionImageError("application", app.Metadata.Name, app.Spec.Image, fromDefault))
		}
	}
	for _, res := range set.Resources {
		errs = append(errs, validateResourceVersion(res, set.isPolicyFromDefault("resource", res.Metadata.Name))...)
	}
	return errs
}

// validateResourceVersion keys on the normalised policy: Plan fills it before
// Resolve runs, so an empty policy only occurs in hand-built sets and is
// left alone.
func validateResourceVersion(res *manifest.ResourceManifest, fromDefault bool) []error {
	name := res.Metadata.Name
	if manifest.IsManifestOwnedPolicy(res.Spec.ImagePullPolicy) && res.Spec.Version == "" {
		return []error{fmt.Errorf("resource %q: spec.version is required", name)}
	}
	if res.Spec.ImagePullPolicy != manifest.ImagePullPolicyPinned {
		return nil
	}

	var errs []error
	if res.Spec.Version != "" && res.Spec.Version != "latest" {
		errs = append(errs, fixedVersionResourceVersionError(name, res.Spec.Version, fromDefault))
	}
	if isResourceImageOverride(res.Spec) && namesFixedVersion(res.Spec.Image) {
		errs = append(errs, fixedVersionImageError("resource", name, res.Spec.Image, fromDefault))
	}
	return errs
}

// isResourceImageOverride tells an operator-written spec.image from the
// <type>:<version> the parser derives, so one mistake is reported once.
func isResourceImageOverride(spec manifest.ResourceSpec) bool {
	if spec.Image == "" || spec.Image == spec.Type {
		return false
	}
	return spec.Version == "" || spec.Image != spec.Type+":"+spec.Version
}

func namesFixedVersion(image string) bool {
	if manifest.IsDigestReference(image) {
		return true
	}
	tag := manifest.TagOf(image)
	return tag != "" && tag != "latest"
}

func fixedVersionImageError(kind, name, image string, fromDefault bool) error {
	repository := repositoryWithoutVersion(image)
	hint := fmt.Sprintf("use %q or %q", repository, repository+":latest")
	return fmt.Errorf("%s %q: spec.image %q names a fixed version but the image pull policy is Pinned%s",
		kind, name, image, fixedVersionRemedy(fromDefault, hint))
}

func fixedVersionResourceVersionError(name, version string, fromDefault bool) error {
	return fmt.Errorf("resource %q: spec.version %q names a fixed version but the image pull policy is Pinned%s",
		name, version, fixedVersionRemedy(fromDefault, `omit it or use "latest"`))
}

// fixedVersionRemedy ends a fixed-version error with the two ways out of
// R-07 when the policy came from the configuration, and with the field fix
// when the manifest named Pinned itself.
func fixedVersionRemedy(fromDefault bool, manifestHint string) string {
	if fromDefault {
		return configSourcedRemedy
	}
	return "; " + manifestHint
}

func repositoryWithoutVersion(image string) string {
	repository, _, _ := strings.Cut(image, "@")
	if tag := manifest.TagOf(repository); tag != "" {
		repository = strings.TrimSuffix(repository, ":"+tag)
	}
	return repository
}
