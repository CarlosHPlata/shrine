//go:build integration

package integration_test

import (
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func fixturesPath(parts ...string) string {
	_, f, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(f), "..", "..", "tests", "testdata", "deploy")
	return filepath.Join(append([]string{base}, parts...)...)
}

const testTeam = "shrine-deploy-test"

func TestDeploy(t *testing.T) {
	s := NewDockerSuite(t, testTeam)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", fixturesPath(),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("should deploy a basic app and create its container and network", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", fixturesPath("basic"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertContainerRunning(testTeam + ".whoami")
		tc.AssertNetworkExists("shrine." + testTeam + ".private")
	})

	s.Test("should deploy an app with static env vars and inject them into the container", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", fixturesPath("envvars"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertContainerRunning(testTeam + ".whoami-env")
		tc.AssertContainerEnvVar(testTeam+".whoami-env", "APP_ENV", "test")
		tc.AssertContainerEnvVar(testTeam+".whoami-env", "APP_VERSION", "1.0")
	})

	s.Test("should resolve resource output env vars and inject them into the dependent app", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", fixturesPath("resources"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertContainerRunning(testTeam + ".test-cache")
		tc.AssertContainerRunning(testTeam + ".whoami-res")
		tc.AssertContainerEnvVar(testTeam+".whoami-res", "CACHE_URL", "http://test-cache:80")
	})

	s.Test("should attach container to platform network when exposeToPlatform is true", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", fixturesPath("platform"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertContainerRunning(testTeam + ".whoami-platform")
		tc.AssertContainerInNetwork(testTeam+".whoami-platform", "shrine.platform")
	})

	s.Test("should persist generated secret to secrets.env after deploy", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", fixturesPath("secrets"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertSecretInState(testTeam, "secret-store", "password")
	})

	s.Test("should resolve template output and inject into container", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", fixturesPath("secrets"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertContainerEnvVar(testTeam+".whoami-secrets", "SECRET_CONNECTION", "redis://"+testTeam+".secret-store:6379")
	})

	s.Test("should inject generated secret value into container env var", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", fixturesPath("secrets"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertContainerEnvVarNotEmpty(testTeam+".whoami-secrets", "SECRET_PASSWORD")
	})

	s.Test("should reuse same generated secret on re-deploy", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", fixturesPath("secrets"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		first := tc.SecretFromState(testTeam, "secret-store", "password")

		tc.Run("deploy",
			"--path", fixturesPath("secrets"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertSecretValueInState(testTeam, "secret-store", "password", first)
	})
}
