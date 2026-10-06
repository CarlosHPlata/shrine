package planner

import (
	"fmt"
	"strings"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

// applyEffectivePullPolicy writes the effective image pull policy back into
// every manifest of the set: the manifest's own field, else defaultPullPolicy,
// else the derived rule. Everything downstream then reads one value (TD-7).
func applyEffectivePullPolicy(set *ManifestSet, defaultPullPolicy string) {
	for _, app := range set.Applications {
		app.Spec.ImagePullPolicy = manifest.EffectivePullPolicyWithDefault(app.Spec.Image, app.Spec.ImagePullPolicy, defaultPullPolicy)
	}
	for _, res := range set.Resources {
		res.Spec.ImagePullPolicy = manifest.EffectivePullPolicyWithDefault(res.Spec.Image, res.Spec.ImagePullPolicy, defaultPullPolicy)
	}
}

// validateImagePolicies enforces the version rules that depend on the
// effective policy: under Pinned no fixed version may be named; under the
// other two a Resource version is required. defaultPullPolicy is unused until
// T4 names the configuration as the policy's source in the message.
func validateImagePolicies(set *ManifestSet, defaultPullPolicy string) []error {
	var errs []error
	for _, app := range set.Applications {
		if app.Spec.ImagePullPolicy == manifest.ImagePullPolicyPinned && namesFixedVersion(app.Spec.Image) {
			errs = append(errs, fixedVersionImageError("application", app.Metadata.Name, app.Spec.Image))
		}
	}
	for _, res := range set.Resources {
		errs = append(errs, validateResourceVersion(res)...)
	}
	return errs
}

// validateResourceVersion keys on the normalised policy: Plan fills it before
// Resolve runs, so an empty policy only occurs in hand-built sets and is
// left alone.
func validateResourceVersion(res *manifest.ResourceManifest) []error {
	name := res.Metadata.Name
	if manifest.IsManifestOwnedPolicy(res.Spec.ImagePullPolicy) && res.Spec.Version == "" {
		return []error{fmt.Errorf("resource %q: spec.version is required", name)}
	}
	if res.Spec.ImagePullPolicy != manifest.ImagePullPolicyPinned {
		return nil
	}

	var errs []error
	if res.Spec.Version != "" && res.Spec.Version != "latest" {
		errs = append(errs, fixedVersionResourceVersionError(name, res.Spec.Version))
	}
	if isResourceImageOverride(res.Spec) && namesFixedVersion(res.Spec.Image) {
		errs = append(errs, fixedVersionImageError("resource", name, res.Spec.Image))
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

func fixedVersionImageError(kind, name, image string) error {
	repository := repositoryWithoutVersion(image)
	return fmt.Errorf("%s %q: spec.image %q names a fixed version but the image pull policy is Pinned; use %q or %q",
		kind, name, image, repository, repository+":latest")
}

func fixedVersionResourceVersionError(name, version string) error {
	return fmt.Errorf("resource %q: spec.version %q names a fixed version but the image pull policy is Pinned; omit it or use \"latest\"",
		name, version)
}

func repositoryWithoutVersion(image string) string {
	repository, _, _ := strings.Cut(image, "@")
	if tag := manifest.TagOf(repository); tag != "" {
		repository = strings.TrimSuffix(repository, ":"+tag)
	}
	return repository
}
