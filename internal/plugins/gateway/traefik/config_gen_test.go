package traefik

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
)

var yamlMarshal = yaml.Marshal

// stubLstat replaces lstatFn for the duration of the test and restores it on cleanup.
func stubLstat(t *testing.T, fn func(string) (os.FileInfo, error)) {
	t.Helper()
	orig := lstatFn
	t.Cleanup(func() { lstatFn = orig })
	lstatFn = fn
}

// stubReadFile replaces readFileFn for the duration of the test and restores it on cleanup.
func stubReadFile(t *testing.T, fn func(string) ([]byte, error)) {
	t.Helper()
	orig := readFileFn
	t.Cleanup(func() { readFileFn = orig })
	readFileFn = fn
}

// --- isStaticConfigPresent tests ---

func TestIsStaticConfigPresent_Present(t *testing.T) {
	stubLstat(t, func(path string) (os.FileInfo, error) {
		return nil, nil
	})

	got, err := isStaticConfigPresent("/some/routing/dir")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !got {
		t.Fatal("expected true (file present), got false")
	}
}

func TestIsStaticConfigPresent_NotExist(t *testing.T) {
	stubLstat(t, func(path string) (os.FileInfo, error) {
		return nil, &fs.PathError{Op: "lstat", Path: path, Err: fs.ErrNotExist}
	})

	got, err := isStaticConfigPresent("/some/routing/dir")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got {
		t.Fatal("expected false (file absent), got true")
	}
}

func TestIsStaticConfigPresent_OtherError(t *testing.T) {
	stubLstat(t, func(path string) (os.FileInfo, error) {
		return nil, errors.New("permission denied")
	})

	got, err := isStaticConfigPresent("/some/routing/dir")
	if err == nil {
		t.Fatal("expected non-nil error, got nil")
	}
	if got {
		t.Fatal("expected false on error, got true")
	}
	if !strings.Contains(err.Error(), "checking traefik.yml") {
		t.Errorf("error message missing 'checking traefik.yml': %v", err)
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error message missing original error: %v", err)
	}
	if !strings.Contains(err.Error(), "/some/routing/dir") {
		t.Errorf("error message missing path: %v", err)
	}
}

// --- generateStaticConfig tests ---

func TestGenerateStaticConfig_Skip_WhenPresent(t *testing.T) {
	stubLstat(t, func(path string) (os.FileInfo, error) {
		return nil, nil
	})
	stubReadFile(t, func(path string) ([]byte, error) {
		return []byte("entryPoints:\n  web:\n    address: ':80'\nproviders:\n  file:\n    directory: /etc/traefik/dynamic\n    watch: true\n"), nil
	})

	obs := &recordingObserver{}
	err := generateStaticConfig(&config.TraefikPluginConfig{Port: 8080}, "/fake/dir", obs)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(obs.events) != 1 {
		t.Fatalf("expected 1 event (preserved, no legacy block), got %d: %+v", len(obs.events), obs.events)
	}
	ev := obs.events[0]
	if ev.Name != "gateway.config.preserved" {
		t.Errorf("expected event name 'gateway.config.preserved', got %q", ev.Name)
	}
	if ev.Status != engine.StatusInfo {
		t.Errorf("expected StatusInfo, got %q", ev.Status)
	}
	if ev.Fields["path"] != "/fake/dir/traefik.yml" {
		t.Errorf("expected path '/fake/dir/traefik.yml', got %q", ev.Fields["path"])
	}
}

func TestGenerateStaticConfig_LegacyHTTPBlock_EmitsWarning(t *testing.T) {
	stubLstat(t, func(path string) (os.FileInfo, error) {
		return nil, nil
	})
	stubReadFile(t, func(path string) ([]byte, error) {
		return []byte("entryPoints:\n  web:\n    address: ':80'\nhttp:\n  routers:\n    dashboard:\n      rule: PathPrefix(`/dashboard`)\n      service: api@internal\n"), nil
	})

	obs := &recordingObserver{}
	err := generateStaticConfig(&config.TraefikPluginConfig{Port: 8080}, "/fake/dir", obs)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(obs.events) != 2 {
		t.Fatalf("expected 2 events (legacy_http_block + preserved), got %d: %+v", len(obs.events), obs.events)
	}
	if obs.events[0].Name != "gateway.config.legacy_http_block" {
		t.Errorf("expected first event 'gateway.config.legacy_http_block', got %q", obs.events[0].Name)
	}
	if obs.events[0].Status != engine.StatusWarning {
		t.Errorf("expected first event status Warning, got %q", obs.events[0].Status)
	}
	if !strings.Contains(obs.events[0].Fields["hint"], "__shrine-dashboard.yml") {
		t.Errorf("hint should mention the new dashboard dynamic file, got %q", obs.events[0].Fields["hint"])
	}
	if obs.events[1].Name != "gateway.config.preserved" {
		t.Errorf("expected second event 'gateway.config.preserved', got %q", obs.events[1].Name)
	}
}

func TestGenerateStaticConfig_StatError(t *testing.T) {
	stubLstat(t, func(path string) (os.FileInfo, error) {
		return nil, errors.New("perm denied")
	})

	obs := &recordingObserver{}
	err := generateStaticConfig(&config.TraefikPluginConfig{Port: 8080}, "/fake/dir", obs)
	if err == nil {
		t.Fatal("expected non-nil error, got nil")
	}
	if !strings.Contains(err.Error(), "checking traefik.yml") {
		t.Errorf("error missing 'checking traefik.yml': %v", err)
	}
	if !strings.Contains(err.Error(), "perm denied") {
		t.Errorf("error missing 'perm denied': %v", err)
	}
	if len(obs.events) != 0 {
		t.Errorf("expected 0 events on error, got %d", len(obs.events))
	}
}

// TestGenerateStaticConfig_Write is intentionally omitted.
// The write branch (lstatFn returns NotExist → os.WriteFile) cannot be
// exercised without touching the filesystem, which violates the unit-test
// constraint. The write path is covered by integration scenarios in
// tests/integration/traefik_plugin_test.go.

// --- staticConfig YAML shape tests ---

// staticConfig must never marshal a top-level `http:` key, regardless of
// configuration: dynamic-only sections in static config are silently dropped
// by Traefik and therefore mask bugs. The dashboard surface lives in the
// dynamic file, not here.
func TestStaticConfigMarshal_HasNoHTTPKey(t *testing.T) {
	spec := staticConfig{
		EntryPoints: map[string]entryPoint{
			"web":     {Address: ":80"},
			"traefik": {Address: ":8080"},
		},
		API: &apiConfig{Dashboard: true},
		Providers: providersConfig{
			File: fileProvider{Directory: "/etc/traefik/dynamic", Watch: true},
		},
	}

	data, err := yamlMarshal(spec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), "\nhttp:") || strings.HasPrefix(string(data), "http:") {
		t.Fatalf("staticConfig marshalled with a top-level http: key; this is the bug class fixed by spec 010\noutput:\n%s", data)
	}
}

// --- generateDashboardDynamicConfig tests ---

func TestGenerateDashboardDynamicConfig_Skip_WhenPresent(t *testing.T) {
	stubLstat(t, func(path string) (os.FileInfo, error) {
		return nil, nil
	})

	obs := &recordingObserver{}
	cfg := &config.TraefikPluginConfig{
		Dashboard: &config.TraefikDashboardConfig{Port: 8080, Username: "admin", Password: "hunter2"},
	}
	err := generateDashboardDynamicConfig(cfg, "/fake/dir", obs)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(obs.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(obs.events))
	}
	ev := obs.events[0]
	if ev.Name != "gateway.dashboard.preserved" {
		t.Errorf("expected event name 'gateway.dashboard.preserved', got %q", ev.Name)
	}
	if ev.Status != engine.StatusInfo {
		t.Errorf("expected StatusInfo, got %q", ev.Status)
	}
	wantPath := "/fake/dir/dynamic/__shrine-dashboard.yml"
	if ev.Fields["path"] != wantPath {
		t.Errorf("expected path %q, got %q", wantPath, ev.Fields["path"])
	}
}

func TestGenerateDashboardDynamicConfig_StatError(t *testing.T) {
	stubLstat(t, func(path string) (os.FileInfo, error) {
		return nil, errors.New("perm denied")
	})

	obs := &recordingObserver{}
	cfg := &config.TraefikPluginConfig{
		Dashboard: &config.TraefikDashboardConfig{Port: 8080, Username: "admin", Password: "hunter2"},
	}
	err := generateDashboardDynamicConfig(cfg, "/fake/dir", obs)
	if err == nil {
		t.Fatal("expected non-nil error, got nil")
	}
	if !strings.Contains(err.Error(), "__shrine-dashboard.yml") {
		t.Errorf("error should reference the dashboard file path: %v", err)
	}
	if !strings.Contains(err.Error(), "perm denied") {
		t.Errorf("error missing 'perm denied': %v", err)
	}
	if len(obs.events) != 0 {
		t.Errorf("expected 0 events on error, got %d", len(obs.events))
	}
}

// --- hasLegacyDashboardHTTPBlock tests ---

func TestHasLegacyDashboardHTTPBlock_Detected(t *testing.T) {
	stubReadFile(t, func(path string) ([]byte, error) {
		return []byte("entryPoints:\n  web:\n    address: ':80'\nhttp:\n  routers:\n    dashboard:\n      rule: PathPrefix(`/dashboard`)\n"), nil
	})

	hit, err := hasLegacyDashboardHTTPBlock("/fake/dir/traefik.yml")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !hit {
		t.Fatal("expected legacy http block detected, got false")
	}
}

func TestHasLegacyDashboardHTTPBlock_NoBlock(t *testing.T) {
	stubReadFile(t, func(path string) ([]byte, error) {
		return []byte("entryPoints:\n  web:\n    address: ':80'\napi:\n  dashboard: true\nproviders:\n  file:\n    directory: /etc/traefik/dynamic\n"), nil
	})

	hit, err := hasLegacyDashboardHTTPBlock("/fake/dir/traefik.yml")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if hit {
		t.Fatal("expected no legacy http block, got true")
	}
}

func TestHasLegacyDashboardHTTPBlock_FileMissing(t *testing.T) {
	stubReadFile(t, func(path string) ([]byte, error) {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	})

	hit, err := hasLegacyDashboardHTTPBlock("/fake/dir/traefik.yml")
	if err != nil {
		t.Fatalf("expected nil error on missing file, got %v", err)
	}
	if hit {
		t.Fatal("expected false on missing file, got true")
	}
}

func TestHasLegacyDashboardHTTPBlock_ParseError(t *testing.T) {
	stubReadFile(t, func(path string) ([]byte, error) {
		return []byte("not: valid: yaml: structure: [unbalanced"), nil
	})

	_, err := hasLegacyDashboardHTTPBlock("/fake/dir/traefik.yml")
	if err == nil {
		t.Fatal("expected non-nil error on malformed yaml, got nil")
	}
	if !strings.Contains(err.Error(), "/fake/dir/traefik.yml") {
		t.Errorf("error should include path: %v", err)
	}
	if !strings.Contains(err.Error(), "probing legacy http block") {
		t.Errorf("error should include 'probing legacy http block': %v", err)
	}
}
