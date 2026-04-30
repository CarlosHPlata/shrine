package config

import (
	"strings"
	"testing"
)

func TestResolveSpecsDir(t *testing.T) {
	const fakeHome = "/home/test-user"

	cases := []struct {
		name      string
		flagValue string
		cfg       Config
		want      string
		wantErr   string
	}{
		{
			name:    "both flag and config empty returns error naming both options",
			wantErr: "no specs directory",
		},
		{
			name:      "flag value is returned when config is empty",
			flagValue: "/abs/from/flag",
			want:      "/abs/from/flag",
		},
		{
			name: "config value is returned when flag is empty",
			cfg:  Config{SpecsDir: "/abs/from/config"},
			want: "/abs/from/config",
		},
		{
			name:      "flag wins over config",
			flagValue: "/from/flag",
			cfg:       Config{SpecsDir: "/from/config"},
			want:      "/from/flag",
		},
		{
			name:      "tilde in flag value is expanded",
			flagValue: "~/specs",
			want:      fakeHome + "/specs",
		},
		{
			name: "tilde in config value is expanded",
			cfg:  Config{SpecsDir: "~/specs"},
			want: fakeHome + "/specs",
		},
		{
			name:      "absolute flag value passes through unchanged (idempotent)",
			flagValue: "/etc/shrine/specs",
			want:      "/etc/shrine/specs",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", fakeHome)
			got, err := tc.cfg.ResolveSpecsDir(tc.flagValue)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ResolveSpecsDir succeeded with %q, want error containing %q", got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveSpecsDir returned unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ResolveSpecsDir(flag=%q, cfg=%+v) = %q, want %q", tc.flagValue, tc.cfg, got, tc.want)
			}
		})
	}
}

func TestResolveTeamsDir(t *testing.T) {
	const fakeHome = "/home/test-user"

	cases := []struct {
		name      string
		flagValue string
		cfg       Config
		want      string
		wantErr   string
	}{
		{
			name:    "all three sources empty returns error",
			wantErr: "no specs directory",
		},
		{
			name:      "flag wins over teamsDir and specsDir",
			flagValue: "/from/flag",
			cfg:       Config{TeamsDir: "/from/teams", SpecsDir: "/from/specs"},
			want:      "/from/flag",
		},
		{
			name: "teamsDir wins over specsDir when flag is empty",
			cfg:  Config{TeamsDir: "/from/teams", SpecsDir: "/from/specs"},
			want: "/from/teams",
		},
		{
			name: "falls back to specsDir when flag and teamsDir are empty",
			cfg:  Config{SpecsDir: "/from/specs"},
			want: "/from/specs",
		},
		{
			name:      "tilde expansion applies to flag",
			flagValue: "~/teams",
			want:      fakeHome + "/teams",
		},
		{
			name: "tilde expansion applies to teamsDir",
			cfg:  Config{TeamsDir: "~/teams"},
			want: fakeHome + "/teams",
		},
		{
			name: "tilde expansion applies to specsDir fallback",
			cfg:  Config{SpecsDir: "~/specs"},
			want: fakeHome + "/specs",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", fakeHome)
			got, err := tc.cfg.ResolveTeamsDir(tc.flagValue)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ResolveTeamsDir succeeded with %q, want error containing %q", got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveTeamsDir returned unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ResolveTeamsDir(flag=%q, cfg=%+v) = %q, want %q", tc.flagValue, tc.cfg, got, tc.want)
			}
		})
	}
}
