package engine

import (
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

func boolPtr(b bool) *bool { return &b }

func TestResolveAliasRoutes(t *testing.T) {
	tests := []struct {
		name        string
		input       manifest.RoutingAlias
		wantPrefix  string
		wantStrip   bool
	}{
		{
			name:      "nil StripPrefix, empty PathPrefix",
			input:     manifest.RoutingAlias{Host: "h", PathPrefix: "", StripPrefix: nil},
			wantPrefix: "",
			wantStrip:  false,
		},
		{
			name:      "nil StripPrefix, non-empty PathPrefix",
			input:     manifest.RoutingAlias{Host: "h", PathPrefix: "/x", StripPrefix: nil},
			wantPrefix: "/x",
			wantStrip:  true,
		},
		{
			name:      "explicit true StripPrefix",
			input:     manifest.RoutingAlias{Host: "h", PathPrefix: "/x", StripPrefix: boolPtr(true)},
			wantPrefix: "/x",
			wantStrip:  true,
		},
		{
			name:      "explicit false StripPrefix",
			input:     manifest.RoutingAlias{Host: "h", PathPrefix: "/x", StripPrefix: boolPtr(false)},
			wantPrefix: "/x",
			wantStrip:  false,
		},
		{
			name:      "trailing slash normalization",
			input:     manifest.RoutingAlias{Host: "h", PathPrefix: "/x/", StripPrefix: nil},
			wantPrefix: "/x",
			wantStrip:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			routes := resolveAliasRoutes([]manifest.RoutingAlias{tc.input})
			if len(routes) != 1 {
				t.Fatalf("expected 1 route, got %d", len(routes))
			}
			r := routes[0]
			if r.Host != tc.input.Host {
				t.Errorf("Host: got %q, want %q", r.Host, tc.input.Host)
			}
			if r.PathPrefix != tc.wantPrefix {
				t.Errorf("PathPrefix: got %q, want %q", r.PathPrefix, tc.wantPrefix)
			}
			if r.StripPrefix != tc.wantStrip {
				t.Errorf("StripPrefix: got %v, want %v", r.StripPrefix, tc.wantStrip)
			}
		})
	}
}

func TestFormatAliasesForLog(t *testing.T) {
	tests := []struct {
		name   string
		routes []AliasRoute
		want   string
	}{
		{
			name:   "one alias no prefix",
			routes: []AliasRoute{{Host: "lan.home.lab", PathPrefix: ""}},
			want:   "lan.home.lab",
		},
		{
			name:   "one alias with prefix",
			routes: []AliasRoute{{Host: "gateway.x.y", PathPrefix: "/finances"}},
			want:   "gateway.x.y+/finances",
		},
		{
			name: "multiple sorted",
			routes: []AliasRoute{
				{Host: "z.example.com", PathPrefix: "/z"},
				{Host: "a.example.com", PathPrefix: ""},
				{Host: "m.example.com", PathPrefix: "/m"},
			},
			want: "a.example.com,m.example.com+/m,z.example.com+/z",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatAliasesForLog(tc.routes)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
