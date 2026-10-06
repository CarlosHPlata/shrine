//go:build integration

package integration_test

import (
	"strings"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

// assertColumnsInOrder checks that the header line of the last run's stdout
// names the columns left to right in the given order.
func assertColumnsInOrder(tc *TestCase, columns ...string) {
	header, _, _ := strings.Cut(tc.RunResult().Stdout, "\n")
	rest := header
	for _, column := range columns {
		idx := strings.Index(rest, column)
		if idx < 0 {
			tc.Fatalf("expected column %q in order %v\nheader: %s", column, columns, header)
		}
		rest = rest[idx+len(column):]
	}
}

func TestGetNoDocker(t *testing.T) {
	s := NewSuite(t)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		// Use the dedicated team/ sub-fixture to avoid scanning bad-kind/ and
		// malformed-yaml/ siblings that cause ScanDir to error.
		tc.Run("apply", "teams",
			"--path", fixturesPath("team"),
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
		// Use the dedicated team/ sub-fixture to avoid scanning bad-kind/ and
		// malformed-yaml/ siblings that cause ScanDir to error.
		tc.Run("apply", "teams",
			"--path", fixturesPath("team"),
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

	s.Test("should show the manifest image reference in a VERSION column after KIND", func(tc *TestCase) {
		tc.Run("get", "deployed", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-res", "traefik/whoami").
			AssertOutputLineContains("test-cache", "traefik/whoami")
		assertColumnsInOrder(tc, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")
	})

	s.Test("should show the VERSION column when filtered by team", func(tc *TestCase) {
		tc.Run("get", "deployed", "--team", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-res", "traefik/whoami").
			AssertOutputLineContains("test-cache", "traefik/whoami")
		assertColumnsInOrder(tc, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")
	})

	s.Test("should show the VERSION column for applications and for resources", func(tc *TestCase) {
		tc.Run("get", "apps", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-res", "traefik/whoami")
		assertColumnsInOrder(tc, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")

		tc.Run("get", "resources", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("test-cache", "traefik/whoami")
		assertColumnsInOrder(tc, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")
	})

	s.Test("should list records from a previous release with an unknown version and fill it on the next deploy", func(tc *TestCase) {
		SeedLegacyDeploymentRecords(tc, testTeam)

		tc.Run("get", "deployed", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-res", " - ").
			AssertOutputLineContains("test-cache", " - ").
			AssertOutputNotContains("traefik/whoami")

		tc.Run("deploy",
			"--path", fixturesPath("resources"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.Run("get", "deployed", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-res", "traefik/whoami").
			AssertOutputLineContains("test-cache", "traefik/whoami")
	})
}
