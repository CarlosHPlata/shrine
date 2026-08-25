//go:build integration

package integration_test

import (
	"path/filepath"
	"strings"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

const tildeConfig = "specsDir: ~/manifests\nteamsDir: ~/teams\n"
const tildeSpecsOnlyConfig = "specsDir: ~/manifests\n"

func tildeConfigDir(t *testing.T, tc *TestCase, content string) string {
	dir := tc.Path("config")
	writeConfig(t, dir, content)
	return dir
}

func assertStderrOnce(tc *TestCase, s string) {
	if n := strings.Count(tc.RunResult().Stderr, s); n != 1 {
		tc.Fatalf("expected %q exactly once on stderr, got %d\nstderr: %s", s, n, tc.RunResult().Stderr)
	}
}

func assertNoLogsDir(tc *TestCase) {
	tc.AssertFileNotExists(filepath.Join(tc.StateDir, "logs"))
}

func TestConfigPathResolution(t *testing.T) {
	s := NewSuite(t)
	s.BeforeEach(func(tc *TestCase) {
		tc.Setenv("HOME", "")
		tc.StateDir = tc.Path("state")
	})

	s.Test("should deploy fail naming specsDir when HOME is unset", func(tc *TestCase) {
		cfgDir := tildeConfigDir(t, tc, tildeConfig)
		tc.Run("deploy", "--config-dir", cfgDir, "--state-dir", tc.StateDir).
			AssertFailure().
			AssertStderrContains("resolving specsDir: expanding ~").
			AssertStderrContains("$HOME is not defined")
		assertStderrOnce(tc, "resolving specsDir")
		assertNoLogsDir(tc)
	})

	s.Test("should deploy --dry-run fail the same way", func(tc *TestCase) {
		cfgDir := tildeConfigDir(t, tc, tildeConfig)
		tc.Run("deploy", "--dry-run", "--config-dir", cfgDir, "--state-dir", tc.StateDir).
			AssertFailure().
			AssertStderrContains("resolving specsDir: expanding ~")
		assertStderrOnce(tc, "resolving specsDir")
		assertNoLogsDir(tc)
	})

	s.Test("should deploy team fail naming specsDir", func(tc *TestCase) {
		cfgDir := tildeConfigDir(t, tc, tildeConfig)
		tc.Run("deploy", "team", "any-team", "--config-dir", cfgDir, "--state-dir", tc.StateDir).
			AssertFailure().
			AssertStderrContains("resolving specsDir: expanding ~")
		assertNoLogsDir(tc)
	})

	s.Test("should apply teams fail naming teamsDir", func(tc *TestCase) {
		cfgDir := tildeConfigDir(t, tc, tildeConfig)
		tc.Run("apply", "teams", "--config-dir", cfgDir, "--state-dir", tc.StateDir).
			AssertFailure().
			AssertStderrContains("resolving teamsDir: expanding ~")
		tc.AssertTeamCount(0)
	})

	s.Test("should apply teams name specsDir when teamsDir falls back to it", func(tc *TestCase) {
		cfgDir := tildeConfigDir(t, tc, tildeSpecsOnlyConfig)
		tc.Run("apply", "teams", "--config-dir", cfgDir, "--state-dir", tc.StateDir).
			AssertFailure().
			AssertStderrContains("resolving specsDir").
			AssertStderrNotContains("resolving teamsDir")
		tc.AssertTeamCount(0)
	})

	s.Test("should apply -f fail naming specsDir", func(tc *TestCase) {
		cfgDir := tildeConfigDir(t, tc, tildeConfig)
		manifest := filepath.Join(fixturesPath(), "..", "app.yml")
		tc.Run("apply", "-f", manifest, "--config-dir", cfgDir, "--state-dir", tc.StateDir).
			AssertFailure().
			AssertStderrContains("resolving specsDir")
		assertNoLogsDir(tc)
	})

	s.Test("should generate team fail naming specsDir", func(tc *TestCase) {
		cfgDir := tildeConfigDir(t, tc, tildeConfig)
		tc.Run("generate", "team", "demo", "--config-dir", cfgDir, "--state-dir", tc.StateDir).
			AssertFailure().
			AssertStderrContains("resolving specsDir")
	})

	s.Test("should name --path when the flag supplied the tilde value", func(tc *TestCase) {
		cfgDir := tildeConfigDir(t, tc, tildeConfig)
		tc.Run("deploy", "--path", "~/manifests", "--config-dir", cfgDir, "--state-dir", tc.StateDir).
			AssertFailure().
			AssertStderrContains("resolving --path: expanding ~").
			AssertStderrNotContains("resolving specsDir")
	})

	s.Test("should ignore an unresolvable config value when --path is absolute", func(tc *TestCase) {
		cfgDir := tildeConfigDir(t, tc, tildeConfig)
		tc.Run("apply", "teams", "--path", fixturesPath("team"), "--config-dir", cfgDir, "--state-dir", tc.StateDir).
			AssertSuccess()
		tc.Run("deploy", "--dry-run", "--path", fixturesPath("basic"), "--config-dir", cfgDir, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertStderrNotContains("resolving")
	})

	s.Test("should teardown fail naming specsDir before composing anything", func(tc *TestCase) {
		cfgDir := tildeConfigDir(t, tc, tildeConfig)
		tc.Run("teardown", "demo-team", "--config-dir", cfgDir, "--state-dir", tc.StateDir).
			AssertFailure().
			AssertStderrContains("resolving specsDir: expanding ~")
		assertStderrOnce(tc, "resolving specsDir")
		assertNoLogsDir(tc)
	})
}
