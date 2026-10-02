package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/plugins/gateway/traefik"
)

func TestJoinCleanup(t *testing.T) {
	errFirst := errors.New("first closer failed")
	errSecond := errors.New("second closer failed")

	t.Run("runs every closer and skips nil entries", func(t *testing.T) {
		calls := 0
		closer := func() error { calls++; return nil }

		if err := joinCleanup(closer, nil, closer)(); err != nil {
			t.Errorf("joinCleanup returned %v, want nil", err)
		}
		if calls != 2 {
			t.Errorf("ran %d closers, want 2", calls)
		}
	})

	t.Run("joins every failure and keeps running after one", func(t *testing.T) {
		ranLast := false
		err := joinCleanup(
			func() error { return errFirst },
			func() error { return errSecond },
			func() error { ranLast = true; return nil },
		)()

		if !errors.Is(err, errFirst) || !errors.Is(err, errSecond) {
			t.Errorf("joined error %v should carry both failures", err)
		}
		if !ranLast {
			t.Error("a closer after a failing one was skipped")
		}
	})

	t.Run("no closers is not an error", func(t *testing.T) {
		if err := joinCleanup()(); err != nil {
			t.Errorf("joinCleanup returned %v, want nil", err)
		}
	})
}

func TestRoutingFromPlugin(t *testing.T) {
	cases := []struct {
		name        string
		cfg         *config.TraefikPluginConfig
		home        string
		wantRouting bool
		wantErr     string
	}{
		{name: "inactive plugin means routing is disabled", home: "/home/test-user"},
		{name: "active plugin yields a routing backend", cfg: &config.TraefikPluginConfig{RoutingDir: "/abs/routes"}, home: "/home/test-user", wantRouting: true},
		{name: "unresolvable routing-dir is reported", cfg: &config.TraefikPluginConfig{RoutingDir: "~/routes"}, wantErr: "resolving routing-dir"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", tc.home)
			plugin, err := traefik.New(tc.cfg, nil, "/abs/specs", nil)
			if err != nil {
				t.Fatalf("traefik.New failed: %v", err)
			}

			routing, err := routingFromPlugin(plugin)

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("routingFromPlugin error = %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("routingFromPlugin returned unexpected error: %v", err)
			}
			if (routing != nil) != tc.wantRouting {
				t.Errorf("routing present = %v, want %v", routing != nil, tc.wantRouting)
			}
		})
	}
}
