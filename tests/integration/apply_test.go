//go:build integration

package integration_test

import (
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func applyFixturesPath(parts ...string) string {
	_, f, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(f), "..", "..", "tests", "testdata", "apply")
	return filepath.Join(append([]string{base}, parts...)...)
}

const applyTestTeam = "shrine-apply-test"

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

func TestApplyFileErrors(t *testing.T) {
	s := NewSuite(t)

	s.Test("should show error when the target file does not exist", func(tc *TestCase) {
		tc.Run("apply", "-f", "/tmp/shrine-nonexistent-99999.yml",
			"--state-dir", tc.TempDir(),
			"--path", applyFixturesPath(),
		).AssertFailure()
	})

	s.Test("should show error when the file contains malformed YAML", func(tc *TestCase) {
		tc.Run("apply", "-f", applyFixturesPath("errors", "malformed.yml"),
			"--state-dir", tc.TempDir(),
			"--path", applyFixturesPath(),
		).AssertFailure()
	})

	s.Test("should show error when the manifest kind is unknown", func(tc *TestCase) {
		tc.Run("apply", "-f", applyFixturesPath("errors", "unknown-kind.yml"),
			"--state-dir", tc.TempDir(),
			"--path", applyFixturesPath(),
		).AssertFailure()
	})

	s.Test("should show error when applying a Team manifest via -f", func(tc *TestCase) {
		tc.Run("apply", "-f", applyFixturesPath("errors", "team.yml"),
			"--state-dir", tc.TempDir(),
			"--path", applyFixturesPath("errors"),
		).AssertFailure().AssertStderrContains("apply teams")
	})

	s.Test("should show error when the file extension is not .yml or .yaml", func(tc *TestCase) {
		tc.Run("apply", "-f", applyFixturesPath("errors", "file.txt"),
			"--state-dir", tc.TempDir(),
			"--path", applyFixturesPath(),
		).AssertFailure()
	})
}

func TestApplyFile(t *testing.T) {
	s := NewDockerSuite(t, applyTestTeam)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", applyFixturesPath("success"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("should deploy an application spec via apply -f", func(tc *TestCase) {
		tc.Run("apply", "-f", applyFixturesPath("success", "app.yml"),
			"--state-dir", tc.StateDir,
			"--path", applyFixturesPath("success"),
		).AssertSuccess()
		tc.AssertContainerRunning(applyTestTeam + ".whoami-apply")
	})

	s.Test("should deploy a resource spec via apply -f", func(tc *TestCase) {
		tc.Run("apply", "-f", applyFixturesPath("success", "resource.yml"),
			"--state-dir", tc.StateDir,
			"--path", applyFixturesPath("success"),
		).AssertSuccess()
		tc.AssertContainerRunning(applyTestTeam + ".apply-cache")
	})

	s.Test("should accept a .yaml file extension", func(tc *TestCase) {
		tc.Run("apply", "-f", applyFixturesPath("success", "app.yaml"),
			"--state-dir", tc.StateDir,
			"--path", applyFixturesPath("success"),
		).AssertSuccess()
		tc.AssertContainerRunning(applyTestTeam + ".whoami-apply-yaml")
	})
}
