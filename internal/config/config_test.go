package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidateRegistries(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "no registries is valid",
			cfg:  Config{},
		},
		{
			name: "registry without alias is valid",
			cfg:  Config{Registries: []RegistryConfig{{Host: "ghcr.io"}}},
		},
		{
			name: "valid alias passes",
			cfg:  Config{Registries: []RegistryConfig{{Host: "192.168.1.1:3000", Alias: "myregistry"}}},
		},
		{
			name: "hyphens and underscores in alias are valid",
			cfg:  Config{Registries: []RegistryConfig{{Host: "192.168.1.1:3000", Alias: "my-registry_prod"}}},
		},
		{
			name: "multiple unique aliases pass",
			cfg: Config{Registries: []RegistryConfig{
				{Host: "192.168.1.1:3000", Alias: "reg-a"},
				{Host: "192.168.1.2:3000", Alias: "reg-b"},
			}},
		},
		{
			name: "duplicate alias returns error",
			cfg: Config{Registries: []RegistryConfig{
				{Host: "192.168.1.1:3000", Alias: "myregistry"},
				{Host: "192.168.1.2:3000", Alias: "myregistry"},
			}},
			wantErr: `alias "myregistry" is defined more than once`,
		},
		{
			name:    "dot in alias returns invalid characters error",
			cfg:     Config{Registries: []RegistryConfig{{Host: "192.168.1.1:3000", Alias: "my.registry"}}},
			wantErr: `alias "my.registry" contains invalid characters`,
		},
		{
			name:    "space in alias returns invalid characters error",
			cfg:     Config{Registries: []RegistryConfig{{Host: "192.168.1.1:3000", Alias: "my registry"}}},
			wantErr: `alias "my registry" contains invalid characters`,
		},
		{
			name:    "slash in alias returns invalid characters error",
			cfg:     Config{Registries: []RegistryConfig{{Host: "192.168.1.1:3000", Alias: "my/registry"}}},
			wantErr: `alias "my/registry" contains invalid characters`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.ValidateRegistries()
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ValidateRegistries succeeded, want error containing %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateRegistries returned unexpected error: %v", err)
			}
		})
	}
}

func TestResolveSpecsDir(t *testing.T) {
	const fakeHome = "/home/test-user"

	cases := []struct {
		name      string
		flagValue string
		cfg       Config
		want      string
		wantErr   string
		noHome    bool
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
		{
			name:    "tilde in config value with no home names specsDir",
			cfg:     Config{SpecsDir: "~/specs"},
			noHome:  true,
			wantErr: "resolving specsDir: expanding ~",
		},
		{
			name:      "tilde in flag value with no home names --path",
			flagValue: "~/specs",
			noHome:    true,
			wantErr:   "resolving --path: expanding ~",
		},
		{
			name:   "absolute config value with no home succeeds",
			cfg:    Config{SpecsDir: "/abs/specs"},
			noHome: true,
			want:   "/abs/specs",
		},
		{
			name:      "absolute flag with tilde config and no home returns the flag",
			flagValue: "/abs/from/flag",
			cfg:       Config{SpecsDir: "~/specs"},
			noHome:    true,
			want:      "/abs/from/flag",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := fakeHome
			if tc.noHome {
				home = ""
			}
			t.Setenv("HOME", home)
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
		noHome    bool
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
		{
			name:    "tilde teamsDir with no home names teamsDir",
			cfg:     Config{TeamsDir: "~/teams"},
			noHome:  true,
			wantErr: "resolving teamsDir: expanding ~",
		},
		{
			name:    "fallback to tilde specsDir with no home names specsDir",
			cfg:     Config{SpecsDir: "~/specs"},
			noHome:  true,
			wantErr: "resolving specsDir: expanding ~",
		},
		{
			name:      "tilde flag with no home names --path",
			flagValue: "~/teams",
			noHome:    true,
			wantErr:   "resolving --path: expanding ~",
		},
		{
			name:   "absolute teamsDir with tilde specsDir and no home returns teamsDir",
			cfg:    Config{TeamsDir: "/from/teams", SpecsDir: "~/specs"},
			noHome: true,
			want:   "/from/teams",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := fakeHome
			if tc.noHome {
				home = ""
			}
			t.Setenv("HOME", home)
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

func TestResolveTeamsDir_FallbackNamesSpecsDirNotTeamsDir(t *testing.T) {
	t.Setenv("HOME", "")
	_, err := (&Config{SpecsDir: "~/specs"}).ResolveTeamsDir("")
	if err == nil {
		t.Fatal("ResolveTeamsDir succeeded, want a resolution error")
	}
	if !strings.Contains(err.Error(), "resolving specsDir") {
		t.Errorf("error %q should name specsDir", err.Error())
	}
	if strings.Contains(err.Error(), "teamsDir") {
		t.Errorf("error %q must not blame teamsDir when specsDir supplied the value", err.Error())
	}
}

func TestValidateImagePullPolicy(t *testing.T) {
	const want = "imagePullPolicy: must be one of Always, IfNotPresent, Pinned"
	cases := []struct {
		value   string
		wantErr bool
	}{
		{"", false},
		{"Always", false},
		{"IfNotPresent", false},
		{"Pinned", false},
		{"pinned", true},
		{"Never", true},
		{"Pinned ", true},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%q", tc.value), func(t *testing.T) {
			cfg := &Config{ImagePullPolicy: tc.value}
			err := cfg.validateImagePullPolicy()
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("expected %q to be accepted, got %v", tc.value, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected %q to be rejected", tc.value)
			}
			if err.Error() != want {
				t.Fatalf("error = %q, want %q", err.Error(), want)
			}
		})
	}
}
