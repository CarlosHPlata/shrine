package manifest

import "testing"

func TestTagOf(t *testing.T) {
	cases := map[string]string{
		"nginx:1.27":                   "1.27",
		"nginx":                        "",
		"127.0.0.1:5000/app":           "",
		"127.0.0.1:5000/app:v1":        "v1",
		"repo@sha256:abc":              "",
		"repo:1.2@sha256:abc":          "1.2",
		"ghcr.io/me/app:latest":        "latest",
		"localhost:5000/shrine/whoami": "",
	}
	for ref, want := range cases {
		t.Run(ref, func(t *testing.T) {
			if got := TagOf(ref); got != want {
				t.Errorf("TagOf(%q) = %q, want %q", ref, got, want)
			}
		})
	}
}

func TestIsDigestReference(t *testing.T) {
	if !IsDigestReference("repo@sha256:abc") {
		t.Error("a reference with @ is a digest reference")
	}
	if IsDigestReference("repo:1.2") {
		t.Error("a tag reference is not a digest reference")
	}
}

func TestIsKnownPullPolicy(t *testing.T) {
	for _, policy := range []string{ImagePullPolicyAlways, ImagePullPolicyIfNotPresent, ImagePullPolicyPinned} {
		if !IsKnownPullPolicy(policy) {
			t.Errorf("%q must be a known policy", policy)
		}
	}
	for _, policy := range []string{"", "Sometimes", "pinned"} {
		if IsKnownPullPolicy(policy) {
			t.Errorf("%q must not be a known policy", policy)
		}
	}
}

func TestIsManifestOwnedPolicy(t *testing.T) {
	if !IsManifestOwnedPolicy(ImagePullPolicyAlways) || !IsManifestOwnedPolicy(ImagePullPolicyIfNotPresent) {
		t.Error("Always and IfNotPresent are manifest-owned")
	}
	if IsManifestOwnedPolicy(ImagePullPolicyPinned) || IsManifestOwnedPolicy("") {
		t.Error("Pinned and an empty policy are not manifest-owned")
	}
}

func TestEffectivePullPolicy_DerivedRule(t *testing.T) {
	cases := map[string]string{
		"nginx":                       ImagePullPolicyAlways,
		"nginx:latest":                ImagePullPolicyAlways,
		"nginx:1.27":                  ImagePullPolicyIfNotPresent,
		"127.0.0.1:5000/app":          ImagePullPolicyAlways,
		"127.0.0.1:5000/app:latest":   ImagePullPolicyAlways,
		"127.0.0.1:5000/app:v1":       ImagePullPolicyIfNotPresent,
		"ghcr.io/me/app@sha256:abcde": ImagePullPolicyIfNotPresent,
	}
	for image, want := range cases {
		t.Run(image, func(t *testing.T) {
			if got := EffectivePullPolicy(image, ""); got != want {
				t.Errorf("EffectivePullPolicy(%q, \"\") = %q, want %q", image, got, want)
			}
		})
	}
	if got := EffectivePullPolicy("nginx", ImagePullPolicyPinned); got != ImagePullPolicyPinned {
		t.Errorf("a declared policy must win, got %q", got)
	}
}

func TestEffectivePullPolicyWithDefault_Precedence(t *testing.T) {
	if got := EffectivePullPolicyWithDefault("nginx:1.27", ImagePullPolicyAlways, ImagePullPolicyPinned); got != ImagePullPolicyAlways {
		t.Errorf("declared must win over the default, got %q", got)
	}
	if got := EffectivePullPolicyWithDefault("nginx:1.27", "", ImagePullPolicyPinned); got != ImagePullPolicyPinned {
		t.Errorf("the default must win over the derived rule, got %q", got)
	}
	if got := EffectivePullPolicyWithDefault("nginx:1.27", "", ""); got != ImagePullPolicyIfNotPresent {
		t.Errorf("with nothing declared the derived rule applies, got %q", got)
	}
}
