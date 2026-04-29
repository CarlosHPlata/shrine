//go:build integration

package integration_test

import (
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func TestStatusNoDocker(t *testing.T) {
	s := NewSuite(t)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		tc.Run("apply", "teams",
			"--path", fixturesPath(),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("should show error when app does not exist", func(tc *TestCase) {
		tc.Run("status", "app", "nonexistent",
			"--team", testTeam,
			"--state-dir", tc.StateDir,
		).AssertFailure().AssertStderrContains("not found")
	})

	s.Test("should show error when resource does not exist", func(tc *TestCase) {
		tc.Run("status", "resource", "nonexistent",
			"--team", testTeam,
			"--state-dir", tc.StateDir,
		).AssertFailure().AssertStderrContains("not found")
	})

	s.Test("should return success for a team with no deployments", func(tc *TestCase) {
		tc.Run("status", testTeam,
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})
}

func TestStatusDocker(t *testing.T) {
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

	s.Test("should show status of a team", func(tc *TestCase) {
		tc.Run("status", testTeam,
			"--state-dir", tc.StateDir,
		).AssertSuccess().AssertOutputContains("running")
	})

	s.Test("should show status of a deployed app", func(tc *TestCase) {
		tc.Run("status", "app", "whoami-res",
			"--team", testTeam,
			"--state-dir", tc.StateDir,
		).AssertSuccess().AssertOutputContains("running")
	})

	s.Test("should show status of a deployed resource", func(tc *TestCase) {
		tc.Run("status", "resource", "test-cache",
			"--team", testTeam,
			"--state-dir", tc.StateDir,
		).AssertSuccess().AssertOutputContains("running")
	})
}
