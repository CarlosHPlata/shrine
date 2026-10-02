//go:build integration

package integration_test

import (
	"path/filepath"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func TestFileLogger(t *testing.T) {
	s := NewDockerSuite(t, testTeam)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", fixturesPath("team"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
		tc.Run("deploy",
			"--path", fixturesPath("basic"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("should create the log on first use and append across runs", func(tc *TestCase) {
		logFile := filepath.Join(tc.StateDir, "logs", "shrine.log")
		deployEntry := `[started] application.deploy name="whoami" owner="` + testTeam + `"`
		// Only teardown removes the team network, so this entry can only come
		// from the second run.
		teardownEntry := `[finished] network.remove name="shrine.` + testTeam + `.private"`

		tc.AssertFileExists(logFile)
		tc.AssertFileContains(logFile, deployEntry)

		tc.Run("teardown", testTeam, "--state-dir", tc.StateDir).AssertSuccess()

		tc.AssertFileContains(logFile, deployEntry)
		tc.AssertFileContains(logFile, teardownEntry)
	})
}
