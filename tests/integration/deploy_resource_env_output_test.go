//go:build integration

package integration_test

import (
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func reoFixturesPath(parts ...string) string {
	_, f, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(f), "..", "..", "tests", "testdata", "resource_env_output")
	return filepath.Join(append([]string{base}, parts...)...)
}

const reoTeam = "shrine-reo"

// TestDeployResourceEnvOutput covers the env/output split: US1 (env feeds the
// container, output is the curated interface), US2 (un-exported keys are not
// consumable), US3 (pre-split output schema is rejected), and FR-014 (a
// resource consumes another resource's exported output).
func TestDeployResourceEnvOutput(t *testing.T) {
	s := NewDockerSuite(t, reoTeam)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", reoFixturesPath("teams"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	// ---- US1 ----
	s.Test("US1: resource env reaches the container; only exported keys are consumable", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", reoFixturesPath("us1"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertContainerRunning(reoTeam + ".pg")
		tc.AssertContainerRunning(reoTeam + ".api")

		// Full env (incl. the private secret) reaches the resource container.
		tc.AssertContainerEnvVar(reoTeam+".pg", "POSTGRES_DB", "app")
		tc.AssertContainerEnvVarNotEmpty(reoTeam+".pg", "POSTGRES_PASSWORD")

		// Consumer resolves the exported re-export and the derived template.
		tc.AssertContainerEnvVar(reoTeam+".api", "DB_NAME", "app")
		tc.AssertContainerEnvVarNotEmpty(reoTeam+".api", "DB_URL")
	})

	// ---- US2 ----
	s.Test("US2: referencing an un-exported env var fails deploy (strict allowlist)", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", reoFixturesPath("us2"),
			"--state-dir", tc.StateDir,
		).
			AssertFailure().
			AssertStderrContains(`references non-existent output "POSTGRES_PASSWORD"`)

		tc.AssertContainerNotExists(reoTeam + ".api")
	})

	// ---- US3 ----
	s.Test("US3: pre-split output schema is rejected with migration guidance", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", reoFixturesPath("us3"),
			"--state-dir", tc.StateDir,
		).
			AssertFailure().
			AssertStderrContains("must not set value/valueFrom/generated")
	})

	// ---- FR-014 ----
	s.Test("FR-014: a resource consumes another resource's exported output in order", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", reoFixturesPath("fr014"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertContainerRunning(reoTeam + ".pg")
		tc.AssertContainerRunning(reoTeam + ".cache")
		tc.AssertContainerEnvVar(reoTeam+".cache", "UPSTREAM_DB_HOST", reoTeam+".pg")
	})
}
