package planner

import (
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
// effective policy. Filled in by spec 033 US2.
func validateImagePolicies(set *ManifestSet, defaultPullPolicy string) []error {
	return nil
}
