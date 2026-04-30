package config

import (
	"strings"
	"testing"
)

func TestResolveRoutingDir(t *testing.T) {
	const fakeHome = "/home/test-user"

	cases := []struct {
		name     string
		cfg      TraefikPluginConfig
		specsDir string
		want     string
		wantErr  string
	}{
		{
			name:    "both routingDir and specsDir empty returns error",
			wantErr: "no routing directory",
		},
		{
			name: "routingDir wins over specsDir fallback",
			cfg:  TraefikPluginConfig{RoutingDir: "/from/routing"},
			specsDir: "/from/specs",
			want:     "/from/routing",
		},
		{
			name:     "falls back to specsDir when routingDir is empty",
			specsDir: "/from/specs",
			want:     "/from/specs",
		},
		{
			name: "tilde in routingDir is expanded",
			cfg:  TraefikPluginConfig{RoutingDir: "~/traefik"},
			want: fakeHome + "/traefik",
		},
		{
			name:     "tilde in specsDir fallback is expanded",
			specsDir: "~/specs/traefik",
			want:     fakeHome + "/specs/traefik",
		},
		{
			name: "absolute routingDir passes through unchanged (idempotent)",
			cfg:  TraefikPluginConfig{RoutingDir: "/etc/shrine/traefik"},
			want: "/etc/shrine/traefik",
		},
		{
			name: "bare tilde in routingDir resolves to HOME",
			cfg:  TraefikPluginConfig{RoutingDir: "~"},
			want: fakeHome,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", fakeHome)
			got, err := tc.cfg.ResolveRoutingDir(tc.specsDir)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ResolveRoutingDir succeeded with %q, want error containing %q", got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveRoutingDir returned unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ResolveRoutingDir(cfg=%+v, specsDir=%q) = %q, want %q", tc.cfg, tc.specsDir, got, tc.want)
			}
		})
	}
}
