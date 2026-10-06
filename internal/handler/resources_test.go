package handler

import (
	"fmt"
	"strings"
	"testing"
)

// legacyResourceSkeleton is the skeleton as it was before the configuration
// default existed; an installation without the key must still get it.
const legacyResourceSkeleton = `apiVersion: shrine/v1
kind: Resource
metadata:
  name: %s
  owner: %s
spec:
  type: %s
  version: "%s"
  networking:
    exposeToPlatform: %v
  # env declares the container's runtime configuration (same shape as an
  # Application's env, plus generated secrets).
  # env:
  #   - name: POSTGRES_DB
  #     value: app
  #   - name: POSTGRES_PASSWORD
  #     generated: true          # auto-minted secret
  # outputs declares the export allowlist consumers may read: a name (re-exporting
  # an env var or the built-in host/port) plus an optional template. No values.
  # Outputs are export-only — they are NEVER set as this container's own env vars.
  # If the container itself needs a value, declare it under env (above).
  # outputs:
  #   - name: POSTGRES_DB        # re-export an env var
  #   - name: host               # built-in
  #   - name: DB_URL
  #     template: "postgres://app:{{.POSTGRES_PASSWORD}}@{{.host}}:{{.port}}/{{.POSTGRES_DB}}"
`

func resourceOptions(version, pullPolicy string) ResourceOptions {
	return ResourceOptions{
		Name:       "db",
		Team:       "t",
		Type:       "postgres",
		Version:    version,
		PullPolicy: pullPolicy,
	}
}

func TestRenderResourceSkeleton(t *testing.T) {
	cases := []struct {
		name          string
		version       string
		pullPolicy    string
		want          string
		wantNoVersion bool
	}{
		{"no default writes version 16", "", "", "  type: postgres\n  version: \"16\"\n  networking:\n", false},
		{"Always default writes version 16", "", "Always", "  type: postgres\n  version: \"16\"\n  networking:\n", false},
		{"IfNotPresent default writes version 16", "", "IfNotPresent", "  type: postgres\n  version: \"16\"\n  networking:\n", false},
		{"Pinned default omits the version line", "", "Pinned", "  type: postgres\n  networking:\n", true},
		{"an explicit version is written verbatim under Pinned", "16", "Pinned", "  version: \"16\"\n", false},
		{"an explicit latest is written verbatim under Pinned", "latest", "Pinned", "  version: \"latest\"\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderResourceSkeleton(resourceOptions(tc.version, tc.pullPolicy))

			if !strings.Contains(got, tc.want) {
				t.Errorf("skeleton lacks %q:\n%s", tc.want, got)
			}
			if tc.wantNoVersion && strings.Contains(got, "version:") {
				t.Errorf("skeleton must omit the version line under Pinned:\n%s", got)
			}
			if strings.Contains(got, "imagePullPolicy") {
				t.Errorf("skeleton must not name a policy:\n%s", got)
			}
			if !strings.Contains(got, "# outputs declares the export allowlist") {
				t.Errorf("skeleton lost the env/outputs guidance:\n%s", got)
			}
		})
	}
}

func TestRenderResourceSkeleton_WithoutADefaultIsTodaysSkeleton(t *testing.T) {
	want := fmt.Sprintf(legacyResourceSkeleton, "db", "t", "postgres", "16", false)

	if got := renderResourceSkeleton(resourceOptions("", "")); got != want {
		t.Errorf("skeleton changed for an installation without a default:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestDefaultResourceVersion(t *testing.T) {
	cases := []struct {
		pullPolicy string
		want       string
	}{
		{"Pinned", ""},
		{"", "16"},
		{"Always", "16"},
		{"IfNotPresent", "16"},
	}
	for _, tc := range cases {
		if got := defaultResourceVersion(tc.pullPolicy); got != tc.want {
			t.Errorf("defaultResourceVersion(%q) = %q, want %q", tc.pullPolicy, got, tc.want)
		}
	}
}

func TestVersionLine(t *testing.T) {
	if got := versionLine(""); got != "" {
		t.Errorf("versionLine(\"\") = %q, want empty", got)
	}
	if got, want := versionLine("16"), "  version: \"16\"\n"; got != want {
		t.Errorf("versionLine(16) = %q, want %q", got, want)
	}
}
