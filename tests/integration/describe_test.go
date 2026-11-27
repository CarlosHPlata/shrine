//go:build integration

package integration_test

import (
	"strings"
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

	s.Test("should still describe a seeded record when the container cannot be inspected", func(tc *TestCase) {
		SeedDeploymentRecord(tc, testTeam, "Application whoami 0123456789abcdef 3a7b - -")

		tc.Run("describe", "app", "whoami", "--team", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("Image:", "-").
			AssertOutputLineContains("Running image:", "unavailable").
			AssertOutputNotContains("Pinned:")
	})
}

func TestDescribeDocker(t *testing.T) {
	s := NewSharedDockerSuite(t, testTeam)

	s.BeforeAll(func(tc *TestCase) {
		tc.ApplyTeams(fixturesPath("team")).AssertSuccess()
		tc.Deploy(fixturesPath("basic")).AssertSuccess()
		tc.Deploy(fixturesPath("resources")).AssertSuccess()
	})

	s.Test("should describe a deployed app", func(tc *TestCase) {
		tc.Describe("app", "whoami", "--team", testTeam).
			AssertSuccess().
			AssertOutputContains("whoami")
	})

	s.Test("should describe a deployed app by name without --team when unambiguous", func(tc *TestCase) {
		tc.Describe("app", "whoami").
			AssertSuccess().
			AssertOutputContains("whoami")
	})

	s.Test("should show the image reference and the pull policy of a deployed app", func(tc *TestCase) {
		tc.Describe("app", "whoami", "--team", testTeam).
			AssertSuccess().
			AssertOutputLineContains("Image:", "traefik/whoami").
			AssertOutputLineContains("Pull policy:", "IfNotPresent")

		tc.Describe("app", "whoami").
			AssertSuccess().
			AssertOutputLineContains("Image:", "traefik/whoami").
			AssertOutputLineContains("Pull policy:", "IfNotPresent")
	})

	s.Test("should show the image reference and the pull policy of a deployed resource", func(tc *TestCase) {
		tc.Describe("resource", "test-cache").
			AssertSuccess().
			AssertOutputLineContains("Image:", "traefik/whoami").
			AssertOutputLineContains("Pull policy:", "Always")
	})

	s.Test("should describe a record from a previous release with an unknown image and policy", func(tc *TestCase) {
		tc.ForkState()
		SeedLegacyDeploymentRecords(tc, testTeam)

		tc.Describe("app", "whoami", "--team", testTeam).
			AssertSuccess().
			AssertOutputLineContains("Image:", "-").
			AssertOutputLineContains("Pull policy:", "-").
			AssertOutputLineContains("Running image:", "traefik/whoami")
		// The record knows nothing about the image; only the live container does.
		for _, line := range strings.Split(tc.RunResult().Stdout, "\n") {
			if strings.Contains(line, "traefik/whoami") && !strings.HasPrefix(line, "Running image:") {
				tc.Fatalf("a legacy record must not print the image on a record line: %s", line)
			}
		}
	})

	s.Test("should show the image the running container was started from for a manifest-owned app", func(tc *TestCase) {
		tc.Describe("app", "whoami", "--team", testTeam).
			AssertSuccess().
			AssertOutputLineContains("Running image:", "traefik/whoami").
			AssertOutputNotContains("Pinned:")
	})
}
