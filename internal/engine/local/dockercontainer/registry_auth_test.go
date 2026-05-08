package dockercontainer

import (
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/config"
)

func TestHasRegistryAliasPrefix(t *testing.T) {
	cases := []struct {
		ref  string
		want bool
	}{
		{"reg:myregistry/image:tag", true},
		{"reg:myregistry/", true},
		{"ghcr.io/foo/bar:1.0", false},
		{"nginx:latest", false},
		{"192.168.1.1:3000/image:tag", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := hasRegistryAliasPrefix(tc.ref); got != tc.want {
			t.Errorf("hasRegistryAliasPrefix(%q) = %v, want %v", tc.ref, got, tc.want)
		}
	}
}

func TestExpandRegistryAlias(t *testing.T) {
	registries := []config.RegistryConfig{
		{Host: "192.168.1.1:3000", Alias: "myregistry"},
		{Host: "10.0.0.5:5000", Alias: "prod"},
	}

	cases := []struct {
		name    string
		ref     string
		want    string
		wantErr string
	}{
		{
			name: "expands known alias",
			ref:  "reg:myregistry/postgres:15",
			want: "192.168.1.1:3000/postgres:15",
		},
		{
			name: "expands second alias",
			ref:  "reg:prod/myapp:v1",
			want: "10.0.0.5:5000/myapp:v1",
		},
		{
			name: "plain image is unchanged",
			ref:  "nginx:latest",
			want: "nginx:latest",
		},
		{
			name: "ip:port image is unchanged",
			ref:  "192.168.1.1:3000/image:tag",
			want: "192.168.1.1:3000/image:tag",
		},
		{
			name:    "unknown alias returns error",
			ref:     "reg:unknown/image:tag",
			wantErr: `alias "unknown" is not defined`,
		},
		{
			name:    "empty alias returns error",
			ref:     "reg:/image:tag",
			wantErr: "alias name must not be empty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandRegistryAlias(tc.ref, registries)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expandRegistryAlias(%q) succeeded with %q, want error containing %q", tc.ref, got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("expandRegistryAlias(%q) unexpected error: %v", tc.ref, err)
			}
			if got != tc.want {
				t.Errorf("expandRegistryAlias(%q) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}
