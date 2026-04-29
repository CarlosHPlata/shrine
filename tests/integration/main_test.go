//go:build integration

package integration_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func TestMain(m *testing.M) {
	bin, cleanup, err := buildBinary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build binary: %v\n", err)
		os.Exit(1)
	}
	testutils.Setup(bin)
	defer cleanup()
	os.Exit(m.Run())
}

func buildBinary() (binPath string, cleanup func(), err error) {
	tmp, err := os.MkdirTemp("", "shrine-integration-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup = func() { os.RemoveAll(tmp) }

	projectRoot, err := findProjectRoot()
	if err != nil {
		cleanup()
		return "", func() {}, err
	}

	binPath = filepath.Join(tmp, "shrine")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("go build failed: %w", err)
	}

	return binPath, cleanup, nil
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found: reached filesystem root")
		}
		dir = parent
	}
}
