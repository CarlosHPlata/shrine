//go:build integration

package testutils

import (
	"path/filepath"
	"testing"

	"github.com/docker/docker/client"
)

type Suite struct {
	t          *testing.T
	beforeAll  []func(*TestCase)
	afterAll   []func(*TestCase)
	beforeEach []func(*TestCase)
	afterEach  []func(*TestCase)
	shared     *TestCase
}

func NewSuite(t *testing.T) *Suite {
	t.Helper()
	return &Suite{t: t}
}

// BeforeAll registers a step that runs once, before the suite's first test.
// The state dir, Docker client and team name it leaves on the TestCase are
// handed to every test, so tests that only read can share one setup. Each
// test also starts with the result of the last command the steps ran, so it
// can assert on that command's output.
func (s *Suite) BeforeAll(fn func(*TestCase)) {
	s.beforeAll = append(s.beforeAll, fn)
}

// AfterAll registers a step that runs once, after the suite's last test.
// Steps run in reverse order of registration.
func (s *Suite) AfterAll(fn func(*TestCase)) {
	s.afterAll = append(s.afterAll, fn)
}

func (s *Suite) BeforeEach(fn func(*TestCase)) {
	s.beforeEach = append(s.beforeEach, fn)
}

func (s *Suite) AfterEach(fn func(*TestCase)) {
	s.afterEach = append(s.afterEach, fn)
}

func (s *Suite) Test(name string, fn func(*TestCase)) {
	s.t.Helper()
	shared := s.sharedSetup()
	s.t.Run(name, func(t *testing.T) {
		t.Helper()
		tc := &TestCase{t: t, tmpDir: t.TempDir()}
		if shared != nil {
			tc.StateDir = shared.StateDir
			tc.DockerClient = shared.DockerClient
			tc.TeamName = shared.TeamName
			tc.result = shared.result
		}
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

// sharedSetup runs the BeforeAll steps the first time a test is declared and
// returns the TestCase they ran on; it is nil for a suite without them.
func (s *Suite) sharedSetup() *TestCase {
	s.t.Helper()
	if s.shared != nil || (len(s.beforeAll) == 0 && len(s.afterAll) == 0) {
		return s.shared
	}
	shared := &TestCase{t: s.t, tmpDir: s.t.TempDir()}
	s.shared = shared
	s.t.Cleanup(func() {
		for i := len(s.afterAll) - 1; i >= 0; i-- {
			s.afterAll[i](shared)
		}
	})
	for _, before := range s.beforeAll {
		before(shared)
	}
	return shared
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

// RunResult returns the captured Result from the most recent Run call.
func (tc *TestCase) RunResult() *Result { return tc.result }

// Fatalf delegates to the underlying *testing.T's Fatalf, marking this test
// as a helper. Use it when an assertion needs custom formatting that the
// built-in Assert* helpers don't cover.
func (tc *TestCase) Fatalf(format string, args ...interface{}) {
	tc.t.Helper()
	tc.t.Fatalf(format, args...)
}

func (tc *TestCase) Skip(reason string) {
	tc.t.Helper()
	tc.t.Skip(reason)
}
