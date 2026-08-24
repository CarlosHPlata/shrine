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

	s.Test("should succeed when specsDir contains foreign yaml", func(tc *TestCase) {
		// T024: apply/foreign-yaml contains team.yaml (shrine-apply-test) plus
		// traefik/traefik.yml (no apiVersion). ApplyTeams must skip the foreign file.
		tc.Run("apply", "teams",
			"--path", applyFixturesPath("foreign-yaml"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
		tc.AssertTeamInState("shrine-apply-test")
	})

	s.Test("should apply teams fail loudly when shrine manifest has bad kind", func(tc *TestCase) {
		// bad-kind/ holds a valid team.yaml beside typo.yaml (kind: Aplication). The
		// run must exit non-zero, name the file and kind on stderr, and write nothing.
		tc.Run("apply", "teams",
			"--path", applyFixturesPath("bad-kind"),
			"--state-dir", tc.StateDir,
		).AssertFailure().
			AssertStderrContains("typo.yaml").
			AssertStderrContains("Aplication")
		tc.AssertTeamNotInState("shrine-apply-test")
	})

	s.Test("should report every broken manifest and write no team", func(tc *TestCase) {
		// multi-broken/ holds typo.yaml (bad kind), noname.yaml (Team without
		// metadata.name) and a valid team.yaml: both defects in one run, zero writes.
		tc.Run("apply", "teams",
			"--path", applyFixturesPath("multi-broken"),
			"--state-dir", tc.StateDir,
		).AssertFailure().
			AssertStderrContains("apply teams failed").
			AssertStderrContains("typo.yaml").
			AssertStderrContains("noname.yaml")
		tc.AssertTeamCount(0)
	})

	s.Test("should reject a Team manifest that fails validation", func(tc *TestCase) {
		tc.Run("apply", "teams",
			"--path", applyFixturesPath("invalid-team"),
			"--state-dir", tc.StateDir,
		).AssertFailure().
			AssertStderrContains("validating manifest").
			AssertStderrContains("noname.yaml").
			AssertStderrContains("metadata.name is required")
		tc.AssertTeamCount(0)
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

	s.Test("should apply file fail loudly when target is shrine manifest with bad kind", func(tc *TestCase) {
		// T034: apply -f pointing at typo.yaml (apiVersion: shrine/v1, kind: Aplication).
		// LoadDir parses the shrine-classified file via manifest.Parse, which returns
		// "unknown manifest kind: \"Aplication\"". The command must exit non-zero and
		// stderr must contain both the file name and the offending kind value.
		// Note: the existing "should show error when the manifest kind is unknown" case
		// uses unknown-kind.yml (kind: Unknown) and only asserts AssertFailure() without
		// checking the specific file name or kind value in stderr. T034 adds those
		// targeted assertions on a separate fixture to explicitly guard FR-003.
		tc.Run("apply", "-f", applyFixturesPath("bad-kind", "typo.yaml"),
			"--path", applyFixturesPath("bad-kind"),
			"--state-dir", tc.TempDir(),
		).AssertFailure().
			AssertStderrContains("typo").
			AssertStderrContains("Aplication")
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

	s.Test("should apply file succeed when specsDir contains foreign yaml", func(tc *TestCase) {
		// T025: apply/foreign-yaml contains app.yaml (whoami-apply-foreign, owner
		// shrine-apply-test) plus traefik/traefik.yml (no apiVersion). The team is
		// already seeded by BeforeEach (from the success fixture). LoadDir is called
		// with --path applyFixturesPath("foreign-yaml") and must skip the foreign file.
		tc.Run("apply", "-f", applyFixturesPath("foreign-yaml", "app.yaml"),
			"--path", applyFixturesPath("foreign-yaml"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
		tc.AssertContainerRunning(applyTestTeam + ".whoami-apply-foreign")
	})

	s.Test("should reject apply -f when the manifest collides with an existing route", func(tc *TestCase) {
		// routing-collision/ holds app-a and app-b both claiming collision.apply.local.
		// Applying app-b alone must fail with the same diagnostic deploy emits and
		// leave no container behind.
		tc.Run("apply", "-f", applyFixturesPath("routing-collision", "app-b.yml"),
			"--path", applyFixturesPath("routing-collision"),
			"--state-dir", tc.StateDir,
		).AssertFailure().
			AssertStderrContains("routing validation failed").
			AssertStderrContains("routing collision").
			AssertStderrContains("shrine-apply-test/app-a").
			AssertStderrContains("shrine-apply-test/app-b")
		tc.AssertContainerNotExists(applyTestTeam + ".app-b")
	})

	s.Test("should apply -f succeed when the only collision does not involve the applied app", func(tc *TestCase) {
		tc.Run("apply", "-f", applyFixturesPath("routing-collision", "app-c.yml"),
			"--path", applyFixturesPath("routing-collision"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
		tc.AssertContainerRunning(applyTestTeam + ".app-c")
	})
}
