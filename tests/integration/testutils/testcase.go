//go:build integration

package testutils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type Suite struct {
	t          *testing.T
	beforeEach []func(*TestCase)
	afterEach  []func(*TestCase)
}

func NewSuite(t *testing.T) *Suite {
	t.Helper()
	return &Suite{t: t}
}

func (s *Suite) BeforeEach(fn func(*TestCase)) {
	s.beforeEach = append(s.beforeEach, fn)
}

func (s *Suite) AfterEach(fn func(*TestCase)) {
	s.afterEach = append(s.afterEach, fn)
}

func (s *Suite) Test(name string, fn func(*TestCase)) {
	s.t.Helper()
	s.t.Run(name, func(t *testing.T) {
		t.Helper()
		tc := &TestCase{t: t, tmpDir: t.TempDir()}
		for _, before := range s.beforeEach {
			before(tc)
		}
		t.Cleanup(func() {
			for _, after := range s.afterEach {
				after(tc)
			}
		})
		fn(tc)
	})
}

type TestCase struct {
	t      *testing.T
	result *Result
	tmpDir string
}

func Test(t *testing.T, name string, fn func(*TestCase)) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		tc := &TestCase{
			t:      t,
			tmpDir: t.TempDir(),
		}
		fn(tc)
	})
}

func (tc *TestCase) TempDir() string {
	return tc.tmpDir
}

func (tc *TestCase) Path(name string) string {
	return filepath.Join(tc.tmpDir, name)
}

func (tc *TestCase) Run(args ...string) *TestCase {
	tc.t.Helper()
	tc.result = Execute(tc.t, args...)
	return tc
}

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

func (tc *TestCase) AssertSpecHas(filePath, key, expectedValue string) *TestCase {
	tc.t.Helper()
	data, err := os.ReadFile(filePath)
	if err != nil {
		tc.t.Fatalf("failed to read file %s: %v", filePath, err)
	}
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		tc.t.Fatalf("failed to parse YAML at %s: %v", filePath, err)
	}
	value, ok := navigateDotPath(parsed, key)
	if !ok {
		tc.t.Fatalf("key %q not found in %s", key, filePath)
	}
	actual := fmt.Sprintf("%v", value)
	if actual != expectedValue {
		tc.t.Fatalf("key %q: expected %q, got %q", key, expectedValue, actual)
	}
	return tc
}

func navigateDotPath(data map[string]interface{}, path string) (interface{}, bool) {
	parts := strings.SplitN(path, ".", 2)
	val, ok := data[parts[0]]
	if !ok {
		return nil, false
	}
	if len(parts) == 1 {
		return val, true
	}
	nested, ok := val.(map[string]interface{})
	if !ok {
		return nil, false
	}
	return navigateDotPath(nested, parts[1])
}
