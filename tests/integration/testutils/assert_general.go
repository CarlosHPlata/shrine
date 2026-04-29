//go:build integration

package testutils

import (
	"os"
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
	if _, err := os.Stat(path); err != nil {
		tc.t.Fatalf("expected file to exist at %s: %v", path, err)
	}
	return tc
}
