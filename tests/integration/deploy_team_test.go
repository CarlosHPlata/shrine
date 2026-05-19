//go:build integration

package integration_test

import (
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func deployTeamFixturesPath(parts ...string) string {
	_, f, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(f), "..", "..", "tests", "testdata", "deploy_team")
	return filepath.Join(append([]string{base}, parts...)...)
}

const (
	teamA = "shrine-team-a"
	teamB = "shrine-team-b"
)

// TestDeployTeam exercises the new `shrine deploy team <name>` subcommand and
// the unknown-team error UX. The suite is anchored on team-a; team-b is
// cleaned up explicitly via BeforeEach/AfterEach.
func TestDeployTeam(t *testing.T) {
	s := NewDockerSuite(t, teamA)

	s.BeforeEach(func(tc *TestCase) {
		CleanupTeam(tc, teamB)
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", deployTeamFixturesPath("teams"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.AfterEach(func(tc *TestCase) {
		CleanupTeam(tc, teamB)
	})

	// Scenario A — multi-team isolation: deploying team-a does not touch team-b.
	s.Test("deploy team only mutates the requested team", func(tc *TestCase) {
		tc.Run("deploy", "team", teamA,
			"--path", deployTeamFixturesPath("multi"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertContainerRunning(teamA + ".alpha")
		tc.AssertContainerNotExists(teamB + ".beta")
	})

	// Scenario B — bare deploy still deploys everything after a team-scoped deploy.
	s.Test("bare deploy reconciles every team after a prior team-scoped deploy", func(tc *TestCase) {
		tc.Run("deploy", "team", teamA,
			"--path", deployTeamFixturesPath("multi"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
		tc.AssertContainerRunning(teamA + ".alpha")
		tc.AssertContainerNotExists(teamB + ".beta")

		tc.Run("deploy",
			"--path", deployTeamFixturesPath("multi"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertContainerRunning(teamA + ".alpha")
		tc.AssertContainerRunning(teamB + ".beta")
	})

	// Scenario C — output header names the team.
	s.Test("deploy team prints a team-aware header", func(tc *TestCase) {
		tc.Run("deploy", "team", teamA,
			"--dry-run",
			"--path", deployTeamFixturesPath("multi"),
			"--state-dir", tc.StateDir,
		).AssertSuccess().
			AssertOutputContains(`Planning deployment for team "` + teamA + `"`)
	})

	// Scenario D — dry-run produces zero side effects.
	s.Test("deploy team --dry-run does not create containers", func(tc *TestCase) {
		tc.Run("deploy", "team", teamA,
			"--dry-run",
			"--path", deployTeamFixturesPath("multi"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertContainerNotExists(teamA + ".alpha")
		tc.AssertContainerNotExists(teamB + ".beta")
	})

	// Scenario E — routing-collision detection still runs under team scope.
	s.Test("deploy team detects routing-domain collisions under --dry-run", func(tc *TestCase) {
		tc.Run("deploy", "team", teamA,
			"--dry-run",
			"--path", deployTeamFixturesPath("collision"),
			"--state-dir", tc.StateDir,
		).AssertFailure().
			AssertStderrContains("collide.test.local")
	})

	// Scenario F — typo case: error names the typo AND lists known teams.
	s.Test("deploy team <typo> exits non-zero and lists known teams", func(tc *TestCase) {
		tc.Run("deploy", "team", "markting",
			"--dry-run",
			"--path", deployTeamFixturesPath("solo"),
			"--state-dir", tc.StateDir,
		).AssertFailure().
			AssertStderrContains(`"markting"`).
			AssertStderrContains(teamA)

		tc.AssertContainerNotExists(teamA + ".alpha")
	})

	// Scenario G — empty specs dir produces the specific empty-set error.
	s.Test("deploy team in empty specs dir exits non-zero with empty-set error", func(tc *TestCase) {
		emptyDir := tc.TempDir()
		tc.Run("deploy", "team", "anything",
			"--dry-run",
			"--path", emptyDir,
			"--state-dir", tc.StateDir,
		).AssertFailure().
			AssertStderrContains("no Application or Resource manifests")
	})
}
