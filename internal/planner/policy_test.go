package planner

import (
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

func policySet(appImage, appPolicy, resType, resVersion, resImage, resPolicy string) *ManifestSet {
	set := NewManifestSet()
	set.Applications["web"] = &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "web", Owner: "team-a"},
		Spec:     manifest.ApplicationSpec{Image: appImage, Port: 80, ImagePullPolicy: appPolicy},
	}
	set.Resources["db"] = &manifest.ResourceManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ResourceKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "db", Owner: "team-a"},
		Spec:     manifest.ResourceSpec{Type: resType, Version: resVersion, Image: resImage, ImagePullPolicy: resPolicy},
	}
	return set
}

func TestApplyEffectivePullPolicy(t *testing.T) {
	cases := []struct {
		name            string
		appImage, appIn string
		resImage, resIn string
		dflt            string
		wantApp         string
		wantRes         string
	}{
		{"declared policy is kept", "web:1.2", "Always", "postgres:16", "Pinned", "IfNotPresent", "Always", "Pinned"},
		{"default fills empty fields", "web:1.2", "", "postgres:16", "", "Pinned", "Pinned", "Pinned"},
		{"derived rule when nothing is set: latest", "web:latest", "", "postgres", "", "", "Always", "Always"},
		{"derived rule when nothing is set: fixed tag", "web:1.2", "", "postgres:16", "", "", "IfNotPresent", "IfNotPresent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := policySet(tc.appImage, tc.appIn, "postgres", "", tc.resImage, tc.resIn)

			applyEffectivePullPolicy(set, tc.dflt)

			if got := set.Applications["web"].Spec.ImagePullPolicy; got != tc.wantApp {
				t.Errorf("application policy = %q, want %q", got, tc.wantApp)
			}
			if got := set.Resources["db"].Spec.ImagePullPolicy; got != tc.wantRes {
				t.Errorf("resource policy = %q, want %q", got, tc.wantRes)
			}
		})
	}
}

func TestPlan_NormalisesThePolicyIntoTheReturnedSet(t *testing.T) {
	set := policySet("web", "Pinned", "postgres", "16", "postgres:16", "")

	result := Plan(set, stubTeamStore{}, nil, PortContext{}, NoFilter(), "")

	if result.Error != nil || len(result.ValidationErr) > 0 {
		t.Fatalf("Plan failed: %v %v", result.Error, result.ValidationErr)
	}
	if got := result.ManifestSet.Applications["web"].Spec.ImagePullPolicy; got != manifest.ImagePullPolicyPinned {
		t.Errorf("application policy on the planned set = %q, want Pinned", got)
	}
	if got := result.ManifestSet.Resources["db"].Spec.ImagePullPolicy; got != manifest.ImagePullPolicyIfNotPresent {
		t.Errorf("resource policy on the planned set = %q, want the derived IfNotPresent", got)
	}
}
