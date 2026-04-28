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
}
