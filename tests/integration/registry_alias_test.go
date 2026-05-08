//go:build integration

package integration_test

import (
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func registryAliasFixturesPath(parts ...string) string {
	_, f, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(f), "..", "..", "tests", "testdata", "deploy")
	return filepath.Join(append([]string{base}, parts...)...)
}


func TestRegistryAliasConfig(t *testing.T) {
	s := NewSuite(t)

	s.Test("valid alias loads without error on dry-run", func(tc *TestCase) {
		stateDir := tc.TempDir()
		cfgDir := registryAliasFixturesPath("registry-alias")

		tc.Run("apply", "teams",
			"--path", registryAliasFixturesPath("team"),
			"--state-dir", stateDir,
		).AssertSuccess()

		tc.Run("deploy", "--dry-run",
			"--path", registryAliasFixturesPath("registry-alias"),
			"--state-dir", stateDir,
			"--config-dir", cfgDir,
		).AssertSuccess()
	})

	s.Test("duplicate alias causes config validation error", func(tc *TestCase) {
		stateDir := tc.TempDir()
		cfgDir := registryAliasFixturesPath("registry-alias-dup")

		tc.Run("deploy", "--dry-run",
			"--path", registryAliasFixturesPath("registry-alias"),
			"--state-dir", stateDir,
			"--config-dir", cfgDir,
		).AssertFailure().
			AssertStderrContains("myregistry")
	})

	s.Test("alias with invalid characters causes config validation error", func(tc *TestCase) {
		stateDir := tc.TempDir()
		cfgDir := registryAliasFixturesPath("registry-alias-badformat")

		tc.Run("deploy", "--dry-run",
			"--path", registryAliasFixturesPath("registry-alias"),
			"--state-dir", stateDir,
			"--config-dir", cfgDir,
		).AssertFailure().
			AssertStderrContains("my.registry")
	})
}

func TestRegistryAliasAppImage(t *testing.T) {
	s := NewSuite(t)

	s.Test("dry-run preserves reg: alias in output for application", func(tc *TestCase) {
		stateDir := tc.TempDir()
		cfgDir := registryAliasFixturesPath("registry-alias")

		tc.Run("apply", "teams",
			"--path", registryAliasFixturesPath("team"),
			"--state-dir", stateDir,
		).AssertSuccess()

		tc.Run("deploy", "--dry-run",
			"--path", registryAliasFixturesPath("registry-alias"),
			"--state-dir", stateDir,
			"--config-dir", cfgDir,
		).AssertSuccess().
			AssertOutputContains("reg:myregistry/")
	})

	s.Test("unknown alias in application image fails at dry-run time", func(tc *TestCase) {
		stateDir := tc.TempDir()
		cfgDir := registryAliasFixturesPath("registry-alias")

		tc.Run("apply", "teams",
			"--path", registryAliasFixturesPath("team"),
			"--state-dir", stateDir,
		).AssertSuccess()

		tc.Run("deploy", "--dry-run",
			"--path", registryAliasFixturesPath("registry-alias-unknown"),
			"--state-dir", stateDir,
			"--config-dir", cfgDir,
		).AssertFailure().
			AssertStderrContains("unknown")
	})

	s.Test("plain image reference is unaffected by alias feature", func(tc *TestCase) {
		stateDir := tc.TempDir()
		cfgDir := registryAliasFixturesPath("registry-alias")

		tc.Run("apply", "teams",
			"--path", registryAliasFixturesPath("team"),
			"--state-dir", stateDir,
		).AssertSuccess()

		tc.Run("deploy", "--dry-run",
			"--path", registryAliasFixturesPath("basic"),
			"--state-dir", stateDir,
			"--config-dir", cfgDir,
		).AssertSuccess()
	})
}

func TestRegistryAliasResourceImage(t *testing.T) {
	s := NewSuite(t)

	s.Test("dry-run preserves reg: alias in output for resource", func(tc *TestCase) {
		stateDir := tc.TempDir()
		cfgDir := registryAliasFixturesPath("registry-alias")

		tc.Run("apply", "teams",
			"--path", registryAliasFixturesPath("team"),
			"--state-dir", stateDir,
		).AssertSuccess()

		tc.Run("deploy", "--dry-run",
			"--path", registryAliasFixturesPath("registry-alias-resource"),
			"--state-dir", stateDir,
			"--config-dir", cfgDir,
		).AssertSuccess().
			AssertOutputContains("reg:myregistry/")
	})

	s.Test("unknown alias in resource image fails at dry-run time", func(tc *TestCase) {
		stateDir := tc.TempDir()
		cfgDir := registryAliasFixturesPath("registry-alias")

		tc.Run("apply", "teams",
			"--path", registryAliasFixturesPath("team"),
			"--state-dir", stateDir,
		).AssertSuccess()

		tc.Run("deploy", "--dry-run",
			"--path", registryAliasFixturesPath("registry-alias-resource-unknown"),
			"--state-dir", stateDir,
			"--config-dir", cfgDir,
		).AssertFailure().
			AssertStderrContains("unknown")
	})
}
