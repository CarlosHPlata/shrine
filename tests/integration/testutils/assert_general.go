//go:build integration

package testutils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (tc *TestCase) AssertSuccess() *TestCase {
	tc.t.Helper()
	if tc.result.ExitCode != 0 {
		tc.t.Fatalf("expected exit code 0, got %d\nstdout: %s\nstderr: %s",
			tc.result.ExitCode, tc.result.Stdout, tc.result.Stderr)
	}
	return tc
}

func (tc *TestCase) AssertFailure() *TestCase {
	tc.t.Helper()
	if tc.result.ExitCode == 0 {
		tc.t.Fatalf("expected non-zero exit code, got 0")
	}
	return tc
}

func (tc *TestCase) AssertOutputContains(s string) *TestCase {
	tc.t.Helper()
	if !strings.Contains(tc.result.Stdout, s) {
		tc.t.Fatalf("expected stdout to contain %q\nstdout: %s", s, tc.result.Stdout)
	}
	return tc
}

func (tc *TestCase) AssertStderrContains(s string) *TestCase {
	tc.t.Helper()
	if !strings.Contains(tc.result.Stderr, s) {
		tc.t.Fatalf("expected stderr to contain %q\nstderr: %s", s, tc.result.Stderr)
	}
	return tc
}

func (tc *TestCase) AssertFileExists(path string) *TestCase {
	tc.t.Helper()
	resolved, err := expandTilde(path)
	if err != nil {
		tc.t.Fatalf("expanding tilde for %s: %v", path, err)
	}
	if _, err := os.Stat(resolved); err != nil {
		tc.t.Fatalf("expected file to exist at %s: %v", resolved, err)
	}
	return tc
}

func (tc *TestCase) AssertFileNotExists(path string) *TestCase {
	tc.t.Helper()
	resolved, err := expandTilde(path)
	if err != nil {
		tc.t.Fatalf("expanding tilde for %s: %v", path, err)
	}
	if _, err := os.Stat(resolved); err == nil {
		tc.t.Fatalf("expected file to NOT exist at %s", resolved)
	}
	return tc
}

func (tc *TestCase) AssertFileContains(path, want string) *TestCase {
	tc.t.Helper()
	resolved, err := expandTilde(path)
	if err != nil {
		tc.t.Fatalf("expanding tilde for %s: %v", path, err)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		tc.t.Fatalf("reading %s: %v", resolved, err)
	}
	if !strings.Contains(string(data), want) {
		tc.t.Fatalf("expected file %s to contain %q\ncontent: %s", resolved, want, string(data))
	}
	return tc
}

func expandTilde(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expanding ~: %w", err)
	}
	return filepath.Join(home, path[1:]), nil
}
