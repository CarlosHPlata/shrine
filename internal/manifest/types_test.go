package manifest

import (
	"strings"
	"testing"
)

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

func TestReadableVersion(t *testing.T) {
	const digest = "sha256:9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b"
	cases := map[string]string{
		"postgres:17":                     "17@9c1b4d7e3f2a",
		"postgres":                        "latest@9c1b4d7e3f2a",
		"127.0.0.1:5000/shrine/whoami":    "latest@9c1b4d7e3f2a",
		"ghcr.io/me/web:latest":           "latest@9c1b4d7e3f2a",
		"postgres@sha256:aaaaaaaaaaaaaaa": "9c1b4d7e3f2a",
	}
	for requested, want := range cases {
		t.Run(requested, func(t *testing.T) {
			if got := ReadableVersion(requested, digest); got != want {
				t.Errorf("ReadableVersion(%q) = %q, want %q", requested, got, want)
			}
		})
	}
	if got := ReadableVersion("postgres:17", "sha256:abc"); got != "17@abc" {
		t.Errorf("a short exact version is kept as is, got %q", got)
	}
}

func TestShortDigest(t *testing.T) {
	cases := map[string]string{
		"sha256:a1b2c3d4e5f6a7b8c9d0": "a1b2c3d4e5f6",
		"a1b2c3d4e5f6a7b8":            "a1b2c3d4e5f6",
		"sha256:abc":                  "abc",
		"":                            "",
	}
	for digest, want := range cases {
		t.Run(digest, func(t *testing.T) {
			if got := ShortDigest(digest); got != want {
				t.Errorf("ShortDigest(%q) = %q, want %q", digest, got, want)
			}
		})
	}
}

func TestDigestOf(t *testing.T) {
	cases := map[string]string{
		"ghcr.io/me/api@sha256:abc": "sha256:abc",
		"ghcr.io/me/api:1.2":        "",
		"":                          "",
	}
	for ref, want := range cases {
		t.Run(ref, func(t *testing.T) {
			if got := DigestOf(ref); got != want {
				t.Errorf("DigestOf(%q) = %q, want %q", ref, got, want)
			}
		})
	}
}

func TestRepositoryOf(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	cases := map[string]string{
		"postgres:17":                     "postgres",
		"postgres":                        "postgres",
		"127.0.0.1:5000/shrine/whoami:v2": "127.0.0.1:5000/shrine/whoami",
		"127.0.0.1:5000/shrine/whoami":    "127.0.0.1:5000/shrine/whoami",
		"ghcr.io/me/app@" + digest:        "ghcr.io/me/app",
		"reg:lab/hello-api:1.2":           "reg:lab/hello-api",
		"":                                "",
	}
	for ref, want := range cases {
		t.Run(ref, func(t *testing.T) {
			if got := RepositoryOf(ref); got != want {
				t.Errorf("RepositoryOf(%q) = %q, want %q", ref, got, want)
			}
		})
	}
}
