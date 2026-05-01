package traefik

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
)

// stubLstat replaces lstatFn for the duration of the test and restores it on cleanup.
func stubLstat(t *testing.T, fn func(string) (os.FileInfo, error)) {
	t.Helper()
	orig := lstatFn
	t.Cleanup(func() { lstatFn = orig })
	lstatFn = fn
}

// --- isStaticConfigPresent tests ---

func TestIsStaticConfigPresent_Present(t *testing.T) {
	stubLstat(t, func(path string) (os.FileInfo, error) {
		// Return nil FileInfo with nil error — helper only checks err.
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
	// Stub lstat to report the file as present (no error).
	stubLstat(t, func(path string) (os.FileInfo, error) {
		return nil, nil
	})

	obs := &recordingObserver{}
	err := generateStaticConfig(&config.TraefikPluginConfig{Port: 8080}, "/fake/dir", obs)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(obs.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(obs.events))
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

// TestGenerateStaticConfig_Write is intentionally omitted.
// The write branch (lstatFn returns NotExist → os.WriteFile) cannot be
// exercised without touching the filesystem, which violates the unit-test
// constraint (no TempDir / no disk writes). The write path is covered by the
// integration scenarios in tests/integration/traefik_plugin_test.go.

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
