//go:build integration

package integration_test

import (
	"os"
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
		// Use the dedicated team/ sub-fixture so BeforeEach does not scan the
		// entire deploy testdata tree, which now contains bad-kind/ and
		// malformed-yaml/ subdirectories that cause ScanDir to error.
		tc.Run("apply", "teams",
			"--path", fixturesPath("team"),
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

	s.Test("should resolve host and port built-ins and inject into container", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", fixturesPath("secrets"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertContainerEnvVar(testTeam+".whoami-secrets", "SECRET_HOST", testTeam+".secret-store")
		tc.AssertContainerEnvVar(testTeam+".whoami-secrets", "SECRET_PORT", "6379")
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

	s.Test("should deploy succeed when specsDir contains foreign YAML files", func(tc *TestCase) {
		// foreign-yaml fixture contains team.yaml + app.yaml (valid shrine) plus
		// traefik/traefik.yml and traefik/dynamic/team-foo-app.yml (no apiVersion)
		// and notes.json. The scanner MUST skip the foreign files silently.
		tc.Run("deploy",
			"--path", fixturesPath("foreign-yaml"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		// The shrine Application named "whoami" should be running.
		tc.AssertContainerRunning(testTeam + ".whoami")

		// No container must have been created from the foreign Traefik YAML.
		tc.AssertContainerNotExists(testTeam + ".traefik")
	})

	// T022: intentionally the same scenario as T016 above — covered by the existing
	// "should deploy succeed when specsDir contains foreign YAML files" test (T016).
	// T022 would assert deploy exits 0 and whoami is running, which is a strict subset
	// of T016's assertions. Adding a duplicate would provide no additional coverage.
	// STATUS: already-covered-by-T016.

	s.Test("should deploy fail loudly when shrine manifest has bad kind", func(tc *TestCase) {
		// T031: bad-kind fixture contains team.yaml + typo.yaml (apiVersion: shrine/v1,
		// kind: Aplication — typo). LoadDir parses the shrine-classified file and
		// manifest.Parse returns "unknown manifest kind: \"Aplication\"". The command
		// must exit non-zero and the error message must name both the file and the kind.
		tc.Run("deploy",
			"--path", fixturesPath("bad-kind"),
			"--state-dir", tc.StateDir,
		).AssertFailure().
			AssertStderrContains("typo.yaml").
			AssertStderrContains("Aplication")
	})

	s.Test("should deploy fail loudly when yaml is malformed", func(tc *TestCase) {
		// T032: malformed-yaml fixture contains team.yaml + app.yaml (valid) plus
		// broken.yaml (apiVersion: shrine/v1, kind: [unclosed — unparseable YAML).
		// ScanDir returns an error wrapping the file path; deploy must exit non-zero
		// and the error message must name the offending file.
		tc.Run("deploy",
			"--path", fixturesPath("malformed-yaml"),
			"--state-dir", tc.StateDir,
		).AssertFailure().
			AssertStderrContains("broken.yaml")
	})

	s.Test("should succeed when specsDir contains non-YAML files", func(tc *TestCase) {
		// T023: proves the extension filter never opens .json files (FR-001a).
		// Copy the basic fixture into a writable temp dir so we can add siblings.
		specsDir := tc.Path("specs")
		if err := copyDir(t, fixturesPath("basic"), specsDir); err != nil {
			t.Fatalf("copy fixture: %v", err)
		}

		// Add a plain JSON file — must be silently ignored by the scanner.
		if err := os.WriteFile(filepath.Join(specsDir, "notes.json"),
			[]byte(`{"note": "scratchpad"}`), 0o644); err != nil {
			t.Fatalf("write notes.json: %v", err)
		}

		// Add a chmod-000 JSON file — the extension filter must never open it.
		configPath := filepath.Join(specsDir, "config.json")
		if err := os.WriteFile(configPath, []byte(`{}`), 0o000); err != nil {
			t.Fatalf("write config.json: %v", err)
		}

		tc.Run("deploy",
			"--path", specsDir,
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertContainerRunning(testTeam + ".whoami")
	})
}
