//go:build integration

package integration_test

import (
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func deleteFixturesPath(parts ...string) string {
	_, f, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(f), "..", "..", "tests", "testdata", "delete")
	return filepath.Join(append([]string{base}, parts...)...)
}

const deleteTestTeam = "shrine-delete-test"

func TestDeleteTeam(t *testing.T) {
	s := NewSuite(t)
	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
	})

	s.Test("should delete a team from state", func(tc *TestCase) {
		specsDir := tc.TempDir()
		tc.Run("generate", "team", "my-team", "--path", specsDir).
			AssertSuccess()
		tc.Run("apply", "teams", "--path", specsDir, "--state-dir", tc.StateDir).
			AssertSuccess()
		tc.Run("delete", "team", "my-team", "--state-dir", tc.StateDir).
			AssertSuccess()
		tc.AssertTeamNotInState("my-team")
	})

	s.Test("should show error when deleting a team that does not exist", func(tc *TestCase) {
		tc.Run("delete", "team", "nonexistent-team", "--state-dir", tc.StateDir).
			AssertFailure()
	})
}

func TestDeleteTeamWithDeployments(t *testing.T) {
	s := NewDockerSuite(t, deleteTestTeam)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", deleteFixturesPath(),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("should show error when the team has active deployments", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", deleteFixturesPath(),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
		tc.Run("delete", "team", deleteTestTeam, "--state-dir", tc.StateDir).
			AssertFailure()
		tc.AssertStderrContains("active deployments")
	})
}
