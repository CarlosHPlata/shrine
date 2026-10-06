package handler

import (
	"fmt"
	"strings"
	"testing"
)

func appOptions(image, pullPolicy string) AppOptions {
	return AppOptions{
		Name:       "web",
		Team:       "t",
		Port:       8080,
		Replicas:   1,
		Domain:     "web.shrine.lab",
		PathPrefix: "/web",
		Image:      image,
		PullPolicy: pullPolicy,
	}
}

func TestRenderAppSkeleton(t *testing.T) {
	cases := []struct {
		name       string
		image      string
		pullPolicy string
		wantImage  string
	}{
		{"no default keeps name:latest", "", "", "  image: web:latest\n"},
		{"Always default keeps name:latest", "", "Always", "  image: web:latest\n"},
		{"IfNotPresent default keeps name:latest", "", "IfNotPresent", "  image: web:latest\n"},
		{"Pinned default writes the bare name", "", "Pinned", "  image: web\n"},
		{"an explicit image is written verbatim under Pinned", "ghcr.io/me/web:1.2", "Pinned", "  image: ghcr.io/me/web:1.2\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderAppSkeleton(appOptions(tc.image, tc.pullPolicy))

			if !strings.Contains(got, tc.wantImage) {
				t.Errorf("skeleton lacks %q:\n%s", tc.wantImage, got)
			}
			if strings.Contains(got, "imagePullPolicy") {
				t.Errorf("skeleton must not name a policy:\n%s", got)
			}
		})
	}
}

func TestRenderAppSkeleton_WithoutADefaultIsTodaysSkeleton(t *testing.T) {
	want := fmt.Sprintf(appSkeleton, "web", "t", "web:latest", 8080, 1, "web.shrine.lab", "/web", false)

	if got := renderAppSkeleton(appOptions("", "")); got != want {
		t.Errorf("skeleton changed for an installation without a default:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestDefaultAppImage(t *testing.T) {
	cases := []struct {
		pullPolicy string
		want       string
	}{
		{"Pinned", "web"},
		{"", "web:latest"},
		{"IfNotPresent", "web:latest"},
		{"Always", "web:latest"},
	}
	for _, tc := range cases {
		if got := defaultAppImage("web", tc.pullPolicy); got != tc.want {
			t.Errorf("defaultAppImage(web, %q) = %q, want %q", tc.pullPolicy, got, tc.want)
		}
	}
}
