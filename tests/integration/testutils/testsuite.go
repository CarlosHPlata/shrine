//go:build integration

package testutils

import (
	"path/filepath"
	"testing"

	"github.com/docker/docker/client"
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
	t            *testing.T
	result       *Result
	tmpDir       string
	StateDir     string
	DockerClient *client.Client
	TeamName     string
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

func (tc *TestCase) Setenv(key, value string) {
	tc.t.Helper()
	tc.t.Setenv(key, value)
}

func (tc *TestCase) Skip(reason string) {
	tc.t.Helper()
	tc.t.Skip(reason)
}
