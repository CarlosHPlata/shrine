package planner

import (
	"strings"
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

func TestPlan_AppliesTheConfigurationDefault(t *testing.T) {
	cases := []struct {
		name             string
		appImage, appIn  string
		resImage, resIn  string
		dflt             string
		wantApp, wantRes string
	}{
		{"a Pinned default fills manifests that name no policy", "web", "", "traefik/whoami", "", "Pinned", "Pinned", "Pinned"},
		{"the manifest field wins over the default", "web:1.2", "IfNotPresent", "postgres:16", "Always", "Pinned", "IfNotPresent", "Always"},
		{"no default keeps the derived rule", "web", "", "postgres:16", "", "", "Always", "IfNotPresent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resVersion := ""
			if tc.resImage == "postgres:16" {
				resVersion = "16"
			}
			set := policySet(tc.appImage, tc.appIn, "postgres", resVersion, tc.resImage, tc.resIn)

			result := Plan(set, stubTeamStore{}, nil, PortContext{}, NoFilter(), tc.dflt)

			if result.Error != nil || len(result.ValidationErr) > 0 {
				t.Fatalf("Plan failed: %v %v", result.Error, result.ValidationErr)
			}
			if got := result.ManifestSet.Applications["web"].Spec.ImagePullPolicy; got != tc.wantApp {
				t.Errorf("application policy = %q, want %q", got, tc.wantApp)
			}
			if got := result.ManifestSet.Resources["db"].Spec.ImagePullPolicy; got != tc.wantRes {
				t.Errorf("resource policy = %q, want %q", got, tc.wantRes)
			}
		})
	}
}

func assertErrorStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d errors %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("error %d:\ngot  %q\nwant %q", i, got[i], want[i])
		}
	}
}

func errorStrings(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, err := range errs {
		out = append(out, err.Error())
	}
	return out
}

func TestValidateImagePolicies(t *testing.T) {
	cases := []struct {
		name                 string
		appImage, appPolicy  string
		resVersion, resImage string
		resPolicy            string
		want                 []string
	}{
		{"pinned application with a fixed tag", "repo:1.2", "Pinned", "latest", "postgres", "Pinned",
			[]string{`application "web": spec.image "repo:1.2" names a fixed version but the image pull policy is Pinned; use "repo" or "repo:latest"`}},
		{"pinned application with a digest reference", "repo@sha256:abc", "Pinned", "latest", "postgres", "Pinned",
			[]string{`application "web": spec.image "repo@sha256:abc" names a fixed version but the image pull policy is Pinned; use "repo" or "repo:latest"`}},
		{"pinned application without a tag", "repo", "Pinned", "latest", "postgres", "Pinned", nil},
		{"pinned application with latest", "repo:latest", "Pinned", "latest", "postgres", "Pinned", nil},
		{"pinned application on a registry with a port", "127.0.0.1:5000/app", "Pinned", "", "postgres", "Pinned", nil},
		{"pinned resource with a version", "repo", "Pinned", "16", "postgres:16", "Pinned",
			[]string{`resource "db": spec.version "16" names a fixed version but the image pull policy is Pinned; omit it or use "latest"`}},
		{"pinned resource with an empty version", "repo", "Pinned", "", "postgres", "Pinned", nil},
		{"pinned resource with an image override naming a tag", "repo", "Pinned", "", "postgres:16", "Pinned",
			[]string{`resource "db": spec.image "postgres:16" names a fixed version but the image pull policy is Pinned; use "postgres" or "postgres:latest"`}},
		{"manifest-owned resource without a version", "repo:1.2", "IfNotPresent", "", "postgres", "IfNotPresent",
			[]string{`resource "db": spec.version is required`}},
		{"manifest-owned resource with a version", "repo:1.2", "IfNotPresent", "16", "postgres:16", "IfNotPresent", nil},
		{"two violations are both reported", "repo:1.2", "Pinned", "16", "postgres:16", "Pinned",
			[]string{
				`application "web": spec.image "repo:1.2" names a fixed version but the image pull policy is Pinned; use "repo" or "repo:latest"`,
				`resource "db": spec.version "16" names a fixed version but the image pull policy is Pinned; omit it or use "latest"`,
			}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := policySet(tc.appImage, tc.appPolicy, "postgres", tc.resVersion, tc.resImage, tc.resPolicy)

			got := errorStrings(validateImagePolicies(set))

			assertErrorStrings(t, got, tc.want)
		})
	}
}

func TestApplyEffectivePullPolicy_RecordsWhereThePolicyCameFrom(t *testing.T) {
	set := policySet("web", "IfNotPresent", "postgres", "", "traefik/whoami", "")
	set.Applications["api"] = &manifest.ApplicationManifest{
		TypeMeta: manifest.TypeMeta{Kind: manifest.ApplicationKind, APIVersion: "shrine/v1"},
		Metadata: manifest.Metadata{Name: "api", Owner: "team-a"},
		Spec:     manifest.ApplicationSpec{Image: "api", Port: 80},
	}

	if set.isPolicyFromDefault("application", "api") {
		t.Fatal("a set that was never normalised must not report a default-sourced policy")
	}

	applyEffectivePullPolicy(set, "Pinned")

	if set.isPolicyFromDefault("application", "web") {
		t.Error("a declared policy must not be reported as default-sourced")
	}
	if !set.isPolicyFromDefault("application", "api") {
		t.Error("an undeclared application policy filled from the default must be reported as default-sourced")
	}
	if !set.isPolicyFromDefault("resource", "db") {
		t.Error("an undeclared resource policy filled from the default must be reported as default-sourced")
	}

	derived := policySet("web", "", "postgres", "16", "postgres:16", "")
	applyEffectivePullPolicy(derived, "")
	if derived.isPolicyFromDefault("application", "web") || derived.isPolicyFromDefault("resource", "db") {
		t.Error("with no default, the derived rule must not be reported as default-sourced")
	}
}

func TestValidateImagePolicies_NamesTheConfigurationWhenThePolicyCameFromIt(t *testing.T) {
	cases := []struct {
		name                 string
		appImage, appPolicy  string
		resVersion, resImage string
		resPolicy            string
		dflt                 string
		want                 []string
	}{
		{"undeclared application with a fixed tag under a Pinned default", "repo:1.2", "", "", "postgres", "", "Pinned",
			[]string{`application "web": spec.image "repo:1.2" names a fixed version but the image pull policy is Pinned` + configSourcedRemedy}},
		{"undeclared resource with a version under a Pinned default", "repo", "", "16", "postgres:16", "", "Pinned",
			[]string{`resource "db": spec.version "16" names a fixed version but the image pull policy is Pinned` + configSourcedRemedy}},
		{"undeclared resource with a tagged image override under a Pinned default", "repo", "", "", "postgres:16", "", "Pinned",
			[]string{`resource "db": spec.image "postgres:16" names a fixed version but the image pull policy is Pinned` + configSourcedRemedy}},
		{"declared Pinned application keeps the manifest-sourced message under a Pinned default", "repo:1.2", "Pinned", "", "postgres", "", "Pinned",
			[]string{`application "web": spec.image "repo:1.2" names a fixed version but the image pull policy is Pinned; use "repo" or "repo:latest"`}},
		{"declared Pinned resource keeps the manifest-sourced message under a Pinned default", "repo", "", "16", "postgres:16", "Pinned", "Pinned",
			[]string{`resource "db": spec.version "16" names a fixed version but the image pull policy is Pinned; omit it or use "latest"`}},
		{"undeclared manifests that break no rule under a Pinned default", "repo", "", "", "postgres", "", "Pinned", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := policySet(tc.appImage, tc.appPolicy, "postgres", tc.resVersion, tc.resImage, tc.resPolicy)
			applyEffectivePullPolicy(set, tc.dflt)

			got := errorStrings(validateImagePolicies(set))

			assertErrorStrings(t, got, tc.want)
		})
	}
}

func TestApplyEffectivePullPolicy_ManifestOwnedDefaults(t *testing.T) {
	cases := []struct {
		name     string
		appImage string
		dflt     string
		wantApp  string
	}{
		{"IfNotPresent default where the derived rule says Always", "web:latest", "IfNotPresent", "IfNotPresent"},
		{"Always default where the derived rule says IfNotPresent", "web:1.2", "Always", "Always"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := policySet(tc.appImage, "", "postgres", "16", "postgres:16", "")

			applyEffectivePullPolicy(set, tc.dflt)

			if got := set.Applications["web"].Spec.ImagePullPolicy; got != tc.wantApp {
				t.Errorf("application policy = %q, want %q", got, tc.wantApp)
			}
			if !set.isPolicyFromDefault("application", "web") {
				t.Error("a manifest-owned default must be recorded as default-sourced")
			}
		})
	}
}

func TestValidateImagePolicies_UnderManifestOwnedDefaults(t *testing.T) {
	cases := []struct {
		name       string
		resVersion string
		resPolicy  string
		dflt       string
		want       []string
	}{
		{"declared Pinned resource without a version under an IfNotPresent default", "", "Pinned", "IfNotPresent", nil},
		{"undeclared resource without a version under an IfNotPresent default", "", "", "IfNotPresent",
			[]string{`resource "db": spec.version is required`}},
		{"undeclared resource without a version under an Always default", "", "", "Always",
			[]string{`resource "db": spec.version is required`}},
		{"undeclared resource with a version under an Always default", "16", "", "Always", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := policySet("web:latest", "", "postgres", tc.resVersion, "traefik/whoami", tc.resPolicy)
			applyEffectivePullPolicy(set, tc.dflt)

			got := errorStrings(validateImagePolicies(set))

			assertErrorStrings(t, got, tc.want)
		})
	}
}

func TestResolve_ReportsPolicyAndAliasErrorsTogether(t *testing.T) {
	set := policySet("reg:ghost/web:1.2", "Pinned", "postgres", "latest", "postgres", "Pinned")

	errs := errorStrings(Resolve(set, stubTeamStore{}, nil))

	var alias, fixed bool
	for _, err := range errs {
		alias = alias || strings.Contains(err, "ghost")
		fixed = fixed || strings.Contains(err, "names a fixed version")
	}
	if !alias || !fixed {
		t.Errorf("expected the alias error and the fixed-version error in one report, got %v", errs)
	}
}
