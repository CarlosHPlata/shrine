//go:build integration

package integration_test

import (
	"path/filepath"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func TestApplyTeams(t *testing.T) {
	s := NewSuite(t)
	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
	})

	s.Test("should apply a team manifest from the specs directory", func(tc *TestCase) {
		specsDir := tc.TempDir()
		tc.Run("generate", "team", "my-team", "--path", specsDir).
			AssertSuccess()
		tc.Run("apply", "teams", "--path", specsDir, "--state-dir", tc.StateDir).
			AssertSuccess()
		tc.AssertTeamInState("my-team")
	})

	s.Test("should apply a team manifest from a subdirectory", func(tc *TestCase) {
		specsDir := tc.TempDir()
		subDir := filepath.Join(specsDir, "nested")
		tc.Run("generate", "team", "nested-team", "--path", subDir).
			AssertSuccess()
		tc.Run("apply", "teams", "--path", specsDir, "--state-dir", tc.StateDir).
			AssertSuccess()
		tc.AssertTeamInState("nested-team")
	})

	s.Test("should apply multiple team manifests", func(tc *TestCase) {
		specsDir := tc.TempDir()
		tc.Run("generate", "team", "team-alpha", "--path", specsDir).
			AssertSuccess()
		tc.Run("generate", "team", "team-beta", "--path", specsDir).
			AssertSuccess()
		tc.Run("apply", "teams", "--path", specsDir, "--state-dir", tc.StateDir).
			AssertSuccess()
		tc.AssertTeamInState("team-alpha")
		tc.AssertTeamInState("team-beta")
		tc.AssertTeamCount(2)
	})
}
