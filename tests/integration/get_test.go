//go:build integration

package integration_test

import (
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func TestGetNoDocker(t *testing.T) {
	s := NewSuite(t)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		tc.Run("apply", "teams",
			"--path", fixturesPath(),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("should list registered teams", func(tc *TestCase) {
		tc.Run("get", "teams", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains(testTeam)
	})
}

func TestGetDocker(t *testing.T) {
	s := NewDockerSuite(t, testTeam)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", fixturesPath(),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
		tc.Run("deploy",
			"--path", fixturesPath("resources"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("should list deployed apps", func(tc *TestCase) {
		tc.Run("get", "apps", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("whoami-res")
	})

	s.Test("should list deployed resources", func(tc *TestCase) {
		tc.Run("get", "resources", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("test-cache")
	})

	s.Test("should list all deployed items", func(tc *TestCase) {
		tc.Run("get", "deployed", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("whoami-res").
			AssertOutputContains("test-cache")
	})
}
