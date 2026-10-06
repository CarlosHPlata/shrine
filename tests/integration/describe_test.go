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
		// Use the dedicated team/ sub-fixture so BeforeEach does not scan the
		// entire deploy testdata tree, which contains bad-kind/ and malformed-yaml/
		// that cause ScanDir to error.
		tc.Run("apply", "teams",
			"--path", fixturesPath("team"),
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
		// Use the dedicated team/ sub-fixture to avoid scanning bad-kind/ and
		// malformed-yaml/ siblings that cause ScanDir to error.
		tc.Run("apply", "teams",
			"--path", fixturesPath("team"),
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

	s.Test("should show the image reference and the pull policy of a deployed app", func(tc *TestCase) {
		tc.Run("describe", "app", "whoami", "--team", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("Image:", "traefik/whoami").
			AssertOutputLineContains("Pull policy:", "Always")

		tc.Run("describe", "app", "whoami", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("Image:", "traefik/whoami").
			AssertOutputLineContains("Pull policy:", "Always")
	})

	s.Test("should show the image reference and the pull policy of a deployed resource", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", fixturesPath("resources"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.Run("describe", "resource", "test-cache", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("Image:", "traefik/whoami").
			AssertOutputLineContains("Pull policy:", "Always")
	})

	s.Test("should describe a record from a previous release with an unknown image and policy", func(tc *TestCase) {
		SeedLegacyDeploymentRecords(tc, testTeam)

		tc.Run("describe", "app", "whoami", "--team", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("Image:", "-").
			AssertOutputLineContains("Pull policy:", "-").
			AssertOutputNotContains("traefik/whoami")
	})
}
