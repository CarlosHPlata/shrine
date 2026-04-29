//go:build integration

package integration_test

import (
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func teardownFixturesPath(parts ...string) string {
	_, f, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(f), "..", "..", "tests", "testdata", "teardown")
	return filepath.Join(append([]string{base}, parts...)...)
}

const teardownTeamA = "shrine-teardown-a"
const teardownTeamB = "shrine-teardown-b"

func TestTeardown(t *testing.T) {
	s := NewDockerSuite(t, testTeam)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", fixturesPath(),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
		tc.Run("deploy",
			"--path", fixturesPath("basic"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("should teardown a deployed team and remove its containers and network", func(tc *TestCase) {
		tc.Run("teardown", testTeam, "--state-dir", tc.StateDir).AssertSuccess()
		tc.AssertContainerNotRunning(testTeam + ".whoami")
		tc.AssertNetworkNotExists("shrine." + testTeam + ".private")
	})
}

func TestTeardownMultiTeam(t *testing.T) {
	s := NewDockerSuite(t, teardownTeamA)

	s.BeforeEach(func(tc *TestCase) {
		CleanupTeam(tc, teardownTeamB)
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", teardownFixturesPath(),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("should only teardown the target team leaving the other running", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", teardownFixturesPath(""),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.Run("teardown", teardownTeamA, "--state-dir", tc.StateDir).AssertSuccess()

		tc.AssertContainerNotRunning(teardownTeamA + ".shared-cache")
		tc.AssertContainerRunning(teardownTeamB + ".dependent-app")
	})

	s.Test("should succeed tearing down team-a even when team-b has a valueFrom dependency on it", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", teardownFixturesPath(),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.Run("teardown", teardownTeamA, "--state-dir", tc.StateDir).AssertSuccess()

		tc.AssertContainerNotRunning(teardownTeamA + ".shared-cache")
		tc.AssertContainerRunning(teardownTeamB + ".dependent-app")
	})
}
