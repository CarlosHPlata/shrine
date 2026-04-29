//go:build integration

package integration_test

import (
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func TestDescribeNoDocker(t *testing.T) {
	s := NewSuite(t)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		tc.Run("apply", "teams",
			"--path", fixturesPath(),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("should describe a team", func(tc *TestCase) {
		tc.Run("describe", "team", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains(testTeam)
	})

	s.Test("should show error when team does not exist", func(tc *TestCase) {
		tc.Run("describe", "team", "nonexistent", "--state-dir", tc.StateDir).
			AssertFailure()
	})

	s.Test("should show error when app does not exist", func(tc *TestCase) {
		tc.Run("describe", "app", "nonexistent", "--team", testTeam, "--state-dir", tc.StateDir).
			AssertFailure().
			AssertStderrContains("not found")
	})

	s.Test("should show error when resource does not exist", func(tc *TestCase) {
		tc.Run("describe", "resource", "nonexistent", "--team", testTeam, "--state-dir", tc.StateDir).
			AssertFailure().
			AssertStderrContains("not found")
	})
}

func TestDescribeDocker(t *testing.T) {
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

	s.Test("should describe a deployed app", func(tc *TestCase) {
		tc.Run("describe", "app", "whoami", "--team", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("whoami")
	})

	s.Test("should describe a deployed app by name without --team when unambiguous", func(tc *TestCase) {
		tc.Run("describe", "app", "whoami", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("whoami")
	})
}
