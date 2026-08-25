package app

import (
	"io"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/config"
)

func TestBuildDeployBundle_FailsBeforeConstructionWhenSpecsDirUnresolvable(t *testing.T) {
	t.Setenv("HOME", "")
	cfg := &config.Config{SpecsDir: "~/manifests"}

	bundle, cleanup, err := BuildDeployBundle(cfg, nil, nil, "", io.Discard, io.Discard)

	assertUnresolvableSpecsDir(t, err)
	if bundle != nil || cleanup != nil {
		t.Fatalf("expected no bundle and no cleanup on failure, got bundle=%v cleanup=%v", bundle != nil, cleanup != nil)
	}
}

func TestBuildTeardownBundle_FailsBeforeConstructionWhenSpecsDirUnresolvable(t *testing.T) {
	t.Setenv("HOME", "")
	cfg := &config.Config{SpecsDir: "~/manifests"}

	bundle, cleanup, err := BuildTeardownBundle(cfg, nil, nil, io.Discard)

	assertUnresolvableSpecsDir(t, err)
	if bundle != nil || cleanup != nil {
		t.Fatalf("expected no bundle and no cleanup on failure, got bundle=%v cleanup=%v", bundle != nil, cleanup != nil)
	}
}

func assertUnresolvableSpecsDir(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a resolution error, got nil")
	}
	for _, want := range []string{"resolving specsDir", "expanding ~"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should contain %q", err.Error(), want)
		}
	}
}

func TestResolveOptionalSpecsDir(t *testing.T) {
	const fakeHome = "/home/test-user"

	cases := []struct {
		name     string
		specsDir string
		home     string
		want     string
		wantErr  string
	}{
		{name: "absent specsDir is not an error", home: fakeHome},
		{name: "absolute value passes through", specsDir: "/abs/specs", home: fakeHome, want: "/abs/specs"},
		{name: "tilde expands when home is known", specsDir: "~/specs", home: fakeHome, want: fakeHome + "/specs"},
		{name: "tilde without home names specsDir", specsDir: "~/specs", wantErr: "resolving specsDir"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", tc.home)
			got, err := resolveOptionalSpecsDir(&config.Config{SpecsDir: tc.specsDir})
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("resolveOptionalSpecsDir succeeded with %q, want error containing %q", got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveOptionalSpecsDir returned unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveOptionalSpecsDir(specsDir=%q) = %q, want %q", tc.specsDir, got, tc.want)
			}
		})
	}
}
