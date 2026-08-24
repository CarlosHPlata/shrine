package traefik

import (
	"fmt"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
)

// fakeBackend is a minimal in-test stub for engine.ContainerBackend.
type fakeBackend struct{}

func (fakeBackend) CreateNetwork(string) error                     { return nil }
func (fakeBackend) RemoveNetwork(string) error                     { return nil }
func (fakeBackend) CreateContainer(engine.CreateContainerOp) error { return nil }
func (fakeBackend) RemoveContainer(engine.RemoveContainerOp) error { return nil }
func (fakeBackend) CreatePlatformNetwork() error                   { return nil }
func (fakeBackend) InspectContainer(string) (engine.ContainerInfo, error) {
	return engine.ContainerInfo{}, nil
}

// TestPlugin_Validate_AcceptsValidTLSPort asserts that a config with a valid
// TLSPort (443) alongside Port 80 passes validation without error.
func TestPlugin_Validate_AcceptsValidTLSPort(t *testing.T) {
	cfg := config.TraefikPluginConfig{Port: 80, TLSPort: 443}
	_, err := New(&cfg, fakeBackend{}, "/tmp", nil)
	if err != nil {
		t.Fatalf("expected nil error for valid TLSPort=443, got: %v", err)
	}
}

// TestPlugin_Validate_RejectsTLSPortOutOfRange asserts that out-of-range
// TLSPort values are rejected with an error naming "tlsPort" and the value.
func TestPlugin_Validate_RejectsTLSPortOutOfRange(t *testing.T) {
	cases := []int{-1, 65536, 100000}
	for _, tlsPort := range cases {
		tlsPort := tlsPort
		t.Run(fmt.Sprintf("TLSPort=%d", tlsPort), func(t *testing.T) {
			cfg := config.TraefikPluginConfig{Port: 80, TLSPort: tlsPort}
			_, err := New(&cfg, fakeBackend{}, "/tmp", nil)
			if err == nil {
				t.Fatalf("expected error for TLSPort=%d, got nil", tlsPort)
			}
			msg := err.Error()
			if !strings.Contains(msg, "tlsPort") {
				t.Errorf("error message %q does not contain %q", msg, "tlsPort")
			}
			valStr := fmt.Sprintf("%d", tlsPort)
			if !strings.Contains(msg, valStr) {
				t.Errorf("error message %q does not contain the value %q", msg, valStr)
			}
		})
	}
}

// TestPlugin_Validate_RejectsTLSPortCollidesWithPort asserts that TLSPort
// colliding with the (possibly defaulted) Port is rejected, and that the
// error message names both "tlsPort" and "port".
func TestPlugin_Validate_RejectsTLSPortCollidesWithPort(t *testing.T) {
	cases := []struct {
		name    string
		port    int
		tlsPort int
	}{
		{"explicit-443-collision", 443, 443},
		{"default-80-collision", 0, 80},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.TraefikPluginConfig{Port: tc.port, TLSPort: tc.tlsPort}
			_, err := New(&cfg, fakeBackend{}, "/tmp", nil)
			if err == nil {
				t.Fatalf("expected error for Port=%d TLSPort=%d collision, got nil", tc.port, tc.tlsPort)
			}
			msg := err.Error()
			if !strings.Contains(msg, "tlsPort") {
				t.Errorf("error message %q does not contain %q", msg, "tlsPort")
			}
			if !strings.Contains(msg, "port") {
				t.Errorf("error message %q does not contain %q", msg, "port")
			}
		})
	}
}

// TestPlugin_Validate_RejectsTLSPortCollidesWithDashboardPort asserts that
// TLSPort colliding with dashboard.port is rejected, naming both fields.
func TestPlugin_Validate_RejectsTLSPortCollidesWithDashboardPort(t *testing.T) {
	cfg := config.TraefikPluginConfig{
		Port:    80,
		TLSPort: 8080,
		Dashboard: &config.TraefikDashboardConfig{
			Port:     8080,
			Username: "u",
			Password: "p",
		},
	}
	_, err := New(&cfg, fakeBackend{}, "/tmp", nil)
	if err == nil {
		t.Fatal("expected error for TLSPort=8080 colliding with dashboard.port=8080, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "tlsPort") {
		t.Errorf("error message %q does not contain %q", msg, "tlsPort")
	}
	if !strings.Contains(msg, "dashboard.port") {
		t.Errorf("error message %q does not contain %q", msg, "dashboard.port")
	}
}

