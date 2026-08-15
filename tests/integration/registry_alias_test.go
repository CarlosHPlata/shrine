//go:build integration

package integration_test

import (
	"context"
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

// newRegistryAliasDockerSuite prepares real (non-dry-run) alias deploys:
// docker client, team applied, subnet state seeded.
func newRegistryAliasDockerSuite(t *testing.T) *Suite {
	t.Helper()
	s := NewDockerSuite(t, testTeam)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", registryAliasFixturesPath("team"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	return s
}

func containerID(tc *TestCase, name string) string {
	info, err := tc.DockerClient.ContainerInspect(context.Background(), name)
	if err != nil {
		tc.Fatalf("inspecting container %q: %v", name, err)
	}
	return info.ID
}

// deployAliasFixture runs a real deploy of the named fixture dir, which
// carries both the manifests and the registry alias config.
func deployAliasFixture(tc *TestCase, fixture string) *TestCase {
	return tc.Run("deploy",
		"--path", registryAliasFixturesPath(fixture),
		"--state-dir", tc.StateDir,
		"--config-dir", registryAliasFixturesPath(fixture),
	)
}

// Spec 022 US1 (FR-001, FR-003, FR-011): a fresh alias-form application
// deploys for real, and the created container carries the expanded reference.
func TestRegistryAliasRealDeploy_Application(t *testing.T) {
	s := newRegistryAliasDockerSuite(t)

	s.Test("fresh alias-form app deploy creates the container with the expanded image", func(tc *TestCase) {
		deployAliasFixture(tc, "registry-alias").AssertSuccess()

		tc.AssertContainerRunning(testTeam + ".alias-app")
		tc.AssertContainerImage(testTeam+".alias-app", "docker.io/traefik/whoami:latest")

		firstID := containerID(tc, testTeam+".alias-app")

		// An existing up-to-date container must keep taking the early-return
		// path — the exact condition that masked issue #33 (FR-011).
		deployAliasFixture(tc, "registry-alias").AssertSuccess()

		if got := containerID(tc, testTeam+".alias-app"); got != firstID {
			tc.Fatalf("redeploy recreated the container: ID %q -> %q", firstID, got)
		}
	})
}

// Spec 022 US2 (FR-002): a fresh alias-form resource deploys for real.
func TestRegistryAliasRealDeploy_Resource(t *testing.T) {
	s := newRegistryAliasDockerSuite(t)

	s.Test("fresh alias-form resource deploy creates the container with the expanded image", func(tc *TestCase) {
		deployAliasFixture(tc, "registry-alias-resource-live").AssertSuccess()

		tc.AssertContainerRunning(testTeam + ".alias-cache")
		tc.AssertContainerImage(testTeam+".alias-cache", "docker.io/traefik/whoami:latest")
	})
}

// Spec 022 US2 acceptance scenario 2: one deploy of a manifest set holding an
// alias-form application AND an alias-form resource — neither may carry an
// unexpanded reg: reference into the runtime.
func TestRegistryAliasRealDeploy_Mixed(t *testing.T) {
	s := newRegistryAliasDockerSuite(t)

	s.Test("app and resource in one alias-form deploy both get expanded images", func(tc *TestCase) {
		deployAliasFixture(tc, "registry-alias-mixed").AssertSuccess()

		tc.AssertContainerRunning(testTeam + ".alias-mixed-app")
		tc.AssertContainerImage(testTeam+".alias-mixed-app", "docker.io/traefik/whoami:latest")
		tc.AssertContainerRunning(testTeam + ".alias-mixed-cache")
		tc.AssertContainerImage(testTeam+".alias-mixed-cache", "docker.io/traefik/whoami:latest")
	})
}

// Spec 022 FR-004/FR-010: the alias form and its fully-qualified equivalent
// denote the same image — editing a manifest between the two forms must never
// recreate the workload by itself.
func TestRegistryAliasFormEquivalence_NoRecreate(t *testing.T) {
	s := newRegistryAliasDockerSuite(t)

	s.Test("swapping between alias and fully-qualified forms never recreates", func(tc *TestCase) {
		deployAliasFixture(tc, "registry-alias-eq-alias").AssertSuccess()

		tc.AssertContainerImage(testTeam+".alias-eq", "docker.io/traefik/whoami:latest")
		firstID := containerID(tc, testTeam+".alias-eq")

		deployAliasFixture(tc, "registry-alias-eq-full").AssertSuccess()

		if got := containerID(tc, testTeam+".alias-eq"); got != firstID {
			tc.Fatalf("form swap recreated the container: ID %q -> %q", firstID, got)
		}
	})
}

// Spec 022 FR-006 under VR-001: an undeclared alias must fail a REAL deploy at
// plan time, before anything is created — the pre-existing coverage for this
// was dry-run only.
func TestRegistryAliasUnknown_RealDeployFailsBeforeCreate(t *testing.T) {
	s := newRegistryAliasDockerSuite(t)

	s.Test("unknown alias fails a real deploy before any container exists", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", registryAliasFixturesPath("registry-alias-unknown"),
			"--state-dir", tc.StateDir,
			"--config-dir", registryAliasFixturesPath("registry-alias"),
		).AssertFailure().
			AssertStderrContains("unknown")

		tc.AssertContainerNotExists(testTeam + ".unknown-alias-app")
	})
}
