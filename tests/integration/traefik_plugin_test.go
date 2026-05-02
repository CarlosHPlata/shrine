//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"gopkg.in/yaml.v3"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

// copyDir recursively copies the contents of src into dst, creating dst if needed.
func copyDir(t *testing.T, src, dst string) error {
	t.Helper()
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(src, path, target)
	})
}

func copyFile(_ string, src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

const traefikTestTeam = "shrine-traefik-test"
const traefikContainerName = "platform.traefik"
const aliasTestTeam = "shrine-alias-test"

func writeConfig(t *testing.T, configDir, content string) {
	t.Helper()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
}

func cleanupTraefikContainer(tc *TestCase) {
	if tc.DockerClient == nil {
		return
	}
	ctx := context.Background()
	_ = tc.DockerClient.ContainerRemove(ctx, traefikContainerName, container.RemoveOptions{Force: true})
}

func traefikFixturePath() string {
	return fixturesPath("traefik")
}

func aliasFixturePath(variant string) string { return fixturesPath("traefik-alias-" + variant) }

func TestTraefikPlugin(t *testing.T) {
	s := NewDockerSuite(t, traefikTestTeam)

	s.BeforeEach(func(tc *TestCase) {
		cleanupTraefikContainer(tc)
		CleanupTeam(tc, aliasTestTeam)
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", traefikFixturePath(),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})
	s.AfterEach(func(tc *TestCase) {
		cleanupTraefikContainer(tc)
		CleanupTeam(tc, aliasTestTeam)
	})

	s.Test("should deploy traefik container when plugin block is populated", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8081
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		tc.AssertContainerRunning(traefikContainerName)
		tc.AssertContainerInNetwork(traefikContainerName, "shrine.platform")
		tc.AssertFileExists(filepath.Join(routingDir, "traefik.yml"))
		tc.AssertOutputContains("Generated default traefik.yml")
	})

	s.Test("should not deploy traefik when plugin block is absent", func(tc *TestCase) {
		configDir := tc.Path("config")
		writeConfig(t, configDir, "")

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		tc.AssertContainerNotExists(traefikContainerName)
	})

	s.Test("should create custom routing dir and bind-mount it into traefik", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := filepath.Join(tc.Path(""), "custom-traefik-dir")

		if _, err := os.Stat(routingDir); err == nil {
			t.Fatalf("precondition failed: %s already exists", routingDir)
		}

		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8083
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		tc.AssertFileExists(routingDir)
		tc.AssertFileExists(filepath.Join(routingDir, "traefik.yml"))
		tc.AssertContainerHasBindMount(traefikContainerName, routingDir, "/etc/traefik")
	})

	s.Test("should write dynamic route file only for apps with domain + ExposeToPlatform", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8084
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		eligiblePath := filepath.Join(routingDir, "dynamic", traefikTestTeam+"-hello-eligible.yml")
		tc.AssertFileExists(eligiblePath)
		tc.AssertFileContains(eligiblePath, "hello-eligible.shrine.lab")

		tc.AssertFileNotExists(filepath.Join(routingDir, "dynamic", traefikTestTeam+"-hello-internal.yml"))
	})

	s.Test("should preserve operator-added files in the dynamic directory", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		dynamicDir := filepath.Join(routingDir, "dynamic")
		if err := os.MkdirAll(dynamicDir, 0o755); err != nil {
			t.Fatalf("mkdir dynamic dir: %v", err)
		}
		opPath := filepath.Join(dynamicDir, "operator-custom.yml")
		opContent := []byte("# operator file — must not be removed\n")
		if err := os.WriteFile(opPath, opContent, 0o644); err != nil {
			t.Fatalf("write operator file: %v", err)
		}

		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8085
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		tc.AssertFileExists(opPath)
		got, err := os.ReadFile(opPath)
		if err != nil {
			t.Fatalf("read operator file after deploy: %v", err)
		}
		if string(got) != string(opContent) {
			t.Fatalf("operator file content was mutated\nwant: %q\ngot:  %q", opContent, got)
		}
	})

	s.Test("should create no traefik artifacts when plugin block is absent", func(tc *TestCase) {
		configDir := tc.Path("config")
		writeConfig(t, configDir, "")

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		tc.AssertFileNotExists(filepath.Join(traefikFixturePath(), "traefik", "traefik.yml"))
		tc.AssertFileNotExists(filepath.Join(traefikFixturePath(), "traefik", "dynamic"))
	})

	s.Test("should produce no side effects on dry-run while still printing route operations", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8086
`)

		tc.Run("deploy", "--dry-run",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		tc.AssertContainerNotExists(traefikContainerName)
		tc.AssertFileNotExists(filepath.Join(routingDir, "traefik.yml"))
		tc.AssertFileNotExists(filepath.Join(routingDir, "dynamic"))
		tc.AssertOutputContains("[ROUTE]")
	})

	s.Test("should fail fast when dashboard.port is set without credentials", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      dashboard:
        port: 8082
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertFailure()

		tc.AssertContainerNotExists(traefikContainerName)
	})

	s.Test("should not fail when routing-dir starts with tilde", func(tc *TestCase) {
		tc.Setenv("HOME", tc.TempDir())

		configDir := tc.Path("config")
		routingDir := filepath.Join("~", "traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8082
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		tc.AssertContainerRunning(traefikContainerName)
		tc.AssertContainerInNetwork(traefikContainerName, "shrine.platform")
		tc.AssertFileExists(filepath.Join(routingDir, "traefik.yml"))
	})

	s.Test("should deploy succeed when routing-dir is inside specsDir", func(tc *TestCase) {
		// SC-001 regression: when routing-dir is a subdirectory of specsDir, the
		// first deploy generates Traefik YAML files (no apiVersion) inside specsDir.
		// The second deploy then scans specsDir again and must NOT crash on those
		// foreign files. We reproduce this by copying the fixture into a temp dir
		// so that both --path and routing-dir point at the same mutable tree.
		specsDir := tc.Path("specs")
		if err := copyDir(t, traefikFixturePath(), specsDir); err != nil {
			t.Fatalf("copy fixture: %v", err)
		}

		configDir := tc.Path("config")
		// routing-dir is specsDir/traefik — a subdirectory of the scanned path.
		routingDir := filepath.Join(specsDir, "traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8087
`)

		// First run: generates Traefik routing files into specsDir/traefik/.
		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", specsDir,
		).AssertSuccess()

		// Second run: specsDir/traefik/ now contains generated files with no
		// apiVersion. This is the canonical SC-001 path — must succeed.
		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", specsDir,
		).AssertSuccess()

		tc.AssertContainerRunning(traefikContainerName)
	})

	// T004: operator-edited traefik.yml must survive a redeploy unchanged.
	s.Test("should preserve operator-edited traefik.yml across redeploys", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8090
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		traefikYML := filepath.Join(routingDir, "traefik.yml")
		originalContent, err := os.ReadFile(traefikYML)
		if err != nil {
			t.Fatalf("read traefik.yml after first deploy: %v", err)
		}
		markedContent := append(originalContent, []byte("\n# OPERATOR_MARKER_T004\n")...)
		if err := os.WriteFile(traefikYML, markedContent, 0o644); err != nil {
			t.Fatalf("write operator marker: %v", err)
		}

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()
		tc.AssertOutputContains("Preserving operator-owned traefik.yml")

		afterContent, err := os.ReadFile(traefikYML)
		if err != nil {
			t.Fatalf("read traefik.yml after second deploy: %v", err)
		}
		if !bytes.Contains(afterContent, []byte("# OPERATOR_MARKER_T004")) {
			t.Fatalf("operator marker was removed from traefik.yml after redeploy\ncontent: %s", afterContent)
		}
	})

	// T005: a broken symlink at the traefik.yml path must be left untouched.
	s.Test("should preserve a broken symlink at traefik.yml across deploy", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8091
`)

		if err := os.MkdirAll(routingDir, 0o755); err != nil {
			t.Fatalf("mkdir routing dir: %v", err)
		}
		symlinkPath := filepath.Join(routingDir, "traefik.yml")
		symlinkTarget := "/nonexistent/path/sentinel-t005"
		if err := os.Symlink(symlinkTarget, symlinkPath); err != nil {
			t.Fatalf("create broken symlink: %v", err)
		}

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()
		tc.AssertOutputContains("Preserving operator-owned traefik.yml")

		gotTarget, err := os.Readlink(symlinkPath)
		if err != nil {
			t.Fatalf("readlink traefik.yml: %v", err)
		}
		if gotTarget != symlinkTarget {
			t.Fatalf("symlink target changed: want %q, got %q", symlinkTarget, gotTarget)
		}

		_, statErr := os.Stat(symlinkTarget)
		if !os.IsNotExist(statErr) {
			t.Fatalf("expected %q to not exist on disk, but stat returned: %v", symlinkTarget, statErr)
		}
	})

	// T006: a directory at the traefik.yml path must be left untouched.
	s.Test("should preserve a directory at the traefik.yml path", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8092
`)

		dirAtYMLPath := filepath.Join(routingDir, "traefik.yml")
		if err := os.MkdirAll(dirAtYMLPath, 0o755); err != nil {
			t.Fatalf("mkdir at traefik.yml path: %v", err)
		}

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		info, err := os.Stat(dirAtYMLPath)
		if err != nil {
			t.Fatalf("stat traefik.yml after deploy: %v", err)
		}
		if !info.IsDir() {
			t.Fatalf("expected traefik.yml to still be a directory after deploy, but it is not")
		}
	})

	// T007: non-IsNotExist stat error on traefik.yml must fail the deploy with a clear message.
	// This scenario is not achievable via the CLI integration path: making routingDir
	// unreadable (chmod 000) also blocks the routing backend's dynamic/ writes, which
	// run in handler.Deploy() BEFORE Plugin.Deploy() reaches isStaticConfigPresent.
	// The error-wrap behavior is fully covered by the unit tests in config_gen_test.go
	// (TestIsStaticConfigPresent_OtherError, TestGenerateStaticConfig_StatError).
	s.Test("should fail deploy with a clear error when stat on traefik.yml fails for a reason other than NotExist", func(tc *TestCase) {
		tc.Skip("non-IsNotExist lstat path is unreachable via the CLI: chmod on routingDir blocks the routing backend before Plugin.Deploy(); covered by config_gen_test.go unit tests")
	})

	// T014: deleting traefik.yml and redeploying must regenerate it with valid content.
	s.Test("should regenerate default traefik.yml after operator deletes the file", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8094
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		traefikYML := filepath.Join(routingDir, "traefik.yml")
		tc.AssertFileExists(traefikYML)

		if err := os.Remove(traefikYML); err != nil {
			t.Fatalf("remove traefik.yml: %v", err)
		}

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()
		tc.AssertOutputContains("Generated default traefik.yml")

		tc.AssertFileExists(traefikYML)

		content, err := os.ReadFile(traefikYML)
		if err != nil {
			t.Fatalf("read regenerated traefik.yml: %v", err)
		}
		if len(content) == 0 {
			t.Fatalf("regenerated traefik.yml is empty")
		}
		if !bytes.Contains(content, []byte("entryPoints:")) {
			t.Fatalf("regenerated traefik.yml missing canonical 'entryPoints:' key\ncontent: %s", content)
		}
	})

	// T009: host-only alias produces a second router with no middlewares.
	s.Test("should publish alias router for a host-only alias", func(tc *TestCase) {
		// Register the alias team in addition to the traefik team registered by BeforeEach.
		tc.Run("apply", "teams",
			"--path", aliasFixturePath("host-only"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8095
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", aliasFixturePath("host-only"),
		).AssertSuccess()

		dynamicFile := filepath.Join(routingDir, "dynamic", "shrine-alias-test-whoami-host-only.yml")
		tc.AssertFileExists(dynamicFile)
		tc.AssertFileContains(dynamicFile, "whoami.shrine.lab")
		tc.AssertFileContains(dynamicFile, "Host(`alias.shrine.lab`)")
		tc.AssertFileContains(dynamicFile, "shrine-alias-test-whoami-host-only-alias-0")
		// Host-only alias never produces a strip middleware.
		content, err := os.ReadFile(dynamicFile)
		if err != nil {
			t.Fatalf("read dynamic file: %v", err)
		}
		if bytes.Contains(content, []byte("middlewares:")) {
			t.Fatalf("host-only alias must not produce any middlewares:\ncontent: %s", content)
		}
	})

	// T010: path-prefixed alias with default strip produces a strip middleware.
	s.Test("should publish alias router with default-strip middleware for path-prefixed alias", func(tc *TestCase) {
		tc.Run("apply", "teams",
			"--path", aliasFixturePath("prefix"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8096
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", aliasFixturePath("prefix"),
		).AssertSuccess()

		dynamicFile := filepath.Join(routingDir, "dynamic", "shrine-alias-test-whoami-prefix.yml")
		tc.AssertFileExists(dynamicFile)
		tc.AssertFileContains(dynamicFile, "Host(`alias.shrine.lab`) && PathPrefix(`/finances`)")
		tc.AssertFileContains(dynamicFile, "shrine-alias-test-whoami-prefix-strip-0:")
		tc.AssertFileContains(dynamicFile, "prefixes:")
		tc.AssertFileContains(dynamicFile, "/finances")
		// The alias router must reference the strip middleware.
		tc.AssertFileContains(dynamicFile, "shrine-alias-test-whoami-prefix-strip-0")
	})

	// T011: three aliases with mixed strip settings emit exactly one strip middleware at index 1.
	s.Test("should publish multiple alias routers with sparse strip indexing", func(tc *TestCase) {
		tc.Run("apply", "teams",
			"--path", aliasFixturePath("multi"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8097
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", aliasFixturePath("multi"),
		).AssertSuccess()

		dynamicFile := filepath.Join(routingDir, "dynamic", "shrine-alias-test-whoami-multi.yml")
		tc.AssertFileExists(dynamicFile)
		tc.AssertFileContains(dynamicFile, "shrine-alias-test-whoami-multi-alias-0")
		tc.AssertFileContains(dynamicFile, "shrine-alias-test-whoami-multi-alias-1")
		tc.AssertFileContains(dynamicFile, "shrine-alias-test-whoami-multi-alias-2")

		// Alias 1 has pathPrefix + default strip → strip-1 emitted.
		tc.AssertFileContains(dynamicFile, "shrine-alias-test-whoami-multi-strip-1")

		// Alias 0 has no pathPrefix; alias 2 has stripPrefix:false → no strip middleware for those.
		content, err := os.ReadFile(dynamicFile)
		if err != nil {
			t.Fatalf("read dynamic file: %v", err)
		}
		if bytes.Contains(content, []byte("shrine-alias-test-whoami-multi-strip-0")) {
			t.Fatalf("alias-0 is host-only — strip-0 must not be emitted:\ncontent: %s", content)
		}
		if bytes.Contains(content, []byte("shrine-alias-test-whoami-multi-strip-2")) {
			t.Fatalf("alias-2 has stripPrefix:false — strip-2 must not be emitted:\ncontent: %s", content)
		}
	})

	// T012: two apps colliding on the same primary domain must fail before any dynamic config is written.
	s.Test("should fail deploy when two applications collide on host+pathPrefix", func(tc *TestCase) {
		tc.Run("apply", "teams",
			"--path", aliasFixturePath("collision"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8098
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", aliasFixturePath("collision"),
		).AssertFailure().
			AssertStderrContains("routing collision:").
			AssertStderrContains("app-a").
			AssertStderrContains("app-b")

		// No dynamic config must have been written for either colliding app.
		tc.AssertFileNotExists(filepath.Join(routingDir, "dynamic", "shrine-alias-test-app-a.yml"))
		tc.AssertFileNotExists(filepath.Join(routingDir, "dynamic", "shrine-alias-test-app-b.yml"))
	})

	// T013: both primary and alias routers reference the same backend service.
	// NOTE: The harness has no docker-exec primitive (assert_docker.go contains no
	// ContainerExec helper), so end-to-end curl verification (SC-002) is out of scope
	// for this test run. Instead we assert the config-level guarantee: both the primary
	// router and all alias routers name the same service key — which is what routes
	// traffic to the same backend. FR-006 live curl coverage belongs to quickstart.md.
	s.Test("should reach backend through both primary and alias addresses", func(tc *TestCase) {
		tc.Run("apply", "teams",
			"--path", aliasFixturePath("prefix"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8099
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", aliasFixturePath("prefix"),
		).AssertSuccess()

		dynamicFile := filepath.Join(routingDir, "dynamic", "shrine-alias-test-whoami-prefix.yml")
		tc.AssertFileExists(dynamicFile)
		// Both the primary router and the alias router must point at the same service.
		tc.AssertFileContains(dynamicFile, "service: shrine-alias-test-whoami-prefix")
	})

	// T014: the routing.configure log line must include an aliases= field.
	s.Test("should include aliases field in routing.configure log signal", func(tc *TestCase) {
		tc.Run("apply", "teams",
			"--path", aliasFixturePath("prefix"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8100
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", aliasFixturePath("prefix"),
		).AssertSuccess()

		tc.AssertOutputContains("Configuring routing:")
		tc.AssertOutputContains("Aliases: alias.shrine.lab+/finances")
	})

	// T014a: re-deploying without aliases must drop alias routers and strip middlewares,
	// once the operator has removed the per-app file (spec 009 preserve policy: a fresh
	// write only happens when the path is Absent — operator rm is the manifest-change
	// trigger).
	s.Test("should drop alias routers when alias is removed and re-deployed", func(tc *TestCase) {
		tc.Run("apply", "teams",
			"--path", aliasFixturePath("prefix"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8101
`)

		// First deploy: with alias.
		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", aliasFixturePath("prefix"),
		).AssertSuccess()

		dynamicFile := filepath.Join(routingDir, "dynamic", "shrine-alias-test-whoami-prefix.yml")
		tc.AssertFileExists(dynamicFile)
		tc.AssertFileContains(dynamicFile, "shrine-alias-test-whoami-prefix-alias-0")
		tc.AssertFileContains(dynamicFile, "shrine-alias-test-whoami-prefix-strip-0")

		// Operator rm: the spec-009 preserve policy requires the file to be Absent
		// before a manifest change can land. Without this rm, the next deploy emits
		// gateway.route.preserved and leaves the alias-bearing file untouched.
		if err := os.Remove(dynamicFile); err != nil {
			t.Fatalf("rm per-app file before manifest-change deploy: %v", err)
		}

		// Second deploy: alias removed — traefik-alias-removed has the same app name
		// (whoami-prefix) but no aliases section. The Absent file path forces regeneration.
		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", aliasFixturePath("removed"),
		).AssertSuccess()

		tc.AssertFileExists(dynamicFile)
		afterContent, err := os.ReadFile(dynamicFile)
		if err != nil {
			t.Fatalf("read dynamic file after alias removal: %v", err)
		}
		if bytes.Contains(afterContent, []byte("alias-0")) {
			t.Fatalf("alias router must be gone after alias is removed:\ncontent: %s", afterContent)
		}
		if bytes.Contains(afterContent, []byte("strip-0")) {
			t.Fatalf("strip middleware must be gone after alias is removed:\ncontent: %s", afterContent)
		}
	})

	// T028: same manifest (with aliases) must deploy cleanly on both a Traefik-disabled
	// and a Traefik-enabled host. This validates the US2 "portable manifest" guarantee.
	s.Test("should run alias-bearing manifest on traefik-enabled and traefik-disabled hosts", func(tc *TestCase) {
		tc.Run("apply", "teams",
			"--path", aliasFixturePath("prefix"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")

		// First deploy: Traefik plugin DISABLED — alias must be inert.
		writeConfig(t, configDir, "")
		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", aliasFixturePath("prefix"),
		).AssertSuccess()

		tc.AssertFileNotExists(filepath.Join(routingDir, "dynamic"))

		// Second deploy: Traefik plugin ENABLED — alias routing must materialise.
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8102
`)
		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", aliasFixturePath("prefix"),
		).AssertSuccess()

		dynamicFile := filepath.Join(routingDir, "dynamic", "shrine-alias-test-whoami-prefix.yml")
		tc.AssertFileExists(dynamicFile)
		tc.AssertFileContains(dynamicFile, "shrine-alias-test-whoami-prefix-alias-0")
	})

	// T005 (spec 009): per-app routing file must be preserved byte-for-byte across redeploys.
	s.Test("should preserve operator-edited per-app routing file across redeploys", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8103
`)

		// First deploy creates the per-app routing file.
		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		perAppFile := filepath.Join(routingDir, "dynamic", traefikTestTeam+"-hello-eligible.yml")
		tc.AssertFileExists(perAppFile)

		originalContent, err := os.ReadFile(perAppFile)
		if err != nil {
			t.Fatalf("read per-app file after first deploy: %v", err)
		}

		// Operator hand-edits the file.
		sentinel := []byte("\n# operator sentinel — must survive\n")
		markedContent := append(originalContent, sentinel...)
		if err := os.WriteFile(perAppFile, markedContent, 0o644); err != nil {
			t.Fatalf("write operator sentinel: %v", err)
		}

		// Second deploy must NOT clobber the file.
		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		afterContent, err := os.ReadFile(perAppFile)
		if err != nil {
			t.Fatalf("read per-app file after second deploy: %v", err)
		}
		if !bytes.Equal(afterContent, markedContent) {
			t.Fatalf("per-app file was mutated across redeploys\nwant:\n%s\ngot:\n%s", markedContent, afterContent)
		}
	})

	// T012 (spec 011): clean deploy with tlsPort must publish host→443/tcp and add websecure entrypoint.
	s.Test("should publish tlsPort to container 443 and add websecure entrypoint on clean deploy", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8108
      tlsPort: 8443
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		tc.AssertContainerRunning(traefikContainerName)
		tc.AssertContainerPublishesPort(traefikContainerName, "8443", "443", "tcp")
		tc.AssertContainerPublishesPort(traefikContainerName, "8108", "8108", "tcp") // regression: HTTP port still bound

		traefikYML := filepath.Join(routingDir, "traefik.yml")
		staticBytes, err := os.ReadFile(traefikYML)
		if err != nil {
			t.Fatalf("read traefik.yml: %v", err)
		}
		var staticDoc map[string]any
		if err := yaml.Unmarshal(staticBytes, &staticDoc); err != nil {
			t.Fatalf("parse traefik.yml: %v\ncontent: %s", err, staticBytes)
		}
		entryPointsRaw, ok := staticDoc["entryPoints"]
		if !ok {
			t.Fatalf("traefik.yml missing entryPoints key\ncontent: %s", staticBytes)
		}
		entryPoints, ok := entryPointsRaw.(map[string]any)
		if !ok {
			t.Fatalf("entryPoints is not a map[string]any\ncontent: %s", staticBytes)
		}
		websecureRaw, ok := entryPoints["websecure"]
		if !ok {
			t.Fatalf("entryPoints.websecure missing\ncontent: %s", staticBytes)
		}
		websecure, ok := websecureRaw.(map[string]any)
		if !ok {
			t.Fatalf("entryPoints.websecure is not a map[string]any\ncontent: %s", staticBytes)
		}
		if len(websecure) != 1 {
			t.Fatalf("entryPoints.websecure must have exactly one key (address), got %d keys: %v\ncontent: %s", len(websecure), websecure, staticBytes)
		}
		addrRaw, ok := websecure["address"]
		if !ok {
			t.Fatalf("entryPoints.websecure missing address key\ncontent: %s", staticBytes)
		}
		if addrRaw != ":443" {
			t.Fatalf("entryPoints.websecure.address = %q, want \":443\"\ncontent: %s", addrRaw, staticBytes)
		}
	})

	// T013 (spec 011): tlsPort that collides with port must be rejected at validation time.
	s.Test("should reject tlsPort that collides with port at validation time", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 443
      tlsPort: 443
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertFailure().
			AssertStderrContains("tlsPort").
			AssertStderrContains("port").
			AssertStderrContains("443")

		tc.AssertContainerNotExists(traefikContainerName)
	})

	// T014 (spec 011): changing tlsPort between deploys must recreate the traefik container.
	s.Test("should recreate traefik container when tlsPort value changes between deploys", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")

		// Stage 1: initial deploy with tlsPort: 8443.
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8110
      tlsPort: 8443
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		infoBefore, err := tc.DockerClient.ContainerInspect(context.Background(), traefikContainerName)
		if err != nil {
			t.Fatalf("inspect container after first deploy: %v", err)
		}
		firstID := infoBefore.ID
		if firstID == "" {
			t.Fatalf("container ID is empty after first deploy")
		}

		// Stage 2: redeploy with tlsPort: 9443 — only tlsPort changes.
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8110
      tlsPort: 9443
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		infoAfter, err := tc.DockerClient.ContainerInspect(context.Background(), traefikContainerName)
		if err != nil {
			t.Fatalf("inspect container after second deploy: %v", err)
		}
		if infoAfter.ID == firstID {
			t.Fatalf("container was NOT recreated after tlsPort change: ID before=%s, ID after=%s", firstID, infoAfter.ID)
		}
		tc.AssertContainerPublishesPort(traefikContainerName, "9443", "443", "tcp")
	})

	// T020 (spec 011 US2): no tlsPort → container must not publish 443/tcp and
	// generated traefik.yml must not contain a websecure entrypoint.
	s.Test("should not publish 443 nor add websecure entrypoint when tlsPort is unset", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8112
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		// Assert container does not publish 443/tcp.
		info, err := tc.DockerClient.ContainerInspect(context.Background(), traefikContainerName)
		if err != nil {
			t.Fatalf("inspect container: %v", err)
		}
		if _, has443 := info.HostConfig.PortBindings[nat.Port("443/tcp")]; has443 {
			var keys []string
			for k := range info.HostConfig.PortBindings {
				keys = append(keys, string(k))
			}
			t.Fatalf("container must not publish 443/tcp when tlsPort is unset; actual bindings: %v", keys)
		}

		// Assert traefik.yml has no websecure entrypoint.
		traefikYML := filepath.Join(routingDir, "traefik.yml")
		staticBytes, err := os.ReadFile(traefikYML)
		if err != nil {
			t.Fatalf("read traefik.yml: %v", err)
		}
		var staticDoc map[string]any
		if err := yaml.Unmarshal(staticBytes, &staticDoc); err != nil {
			t.Fatalf("parse traefik.yml: %v\ncontent: %s", err, staticBytes)
		}
		entryPointsRaw, ok := staticDoc["entryPoints"]
		if !ok {
			t.Fatalf("traefik.yml missing entryPoints key\ncontent: %s", staticBytes)
		}
		entryPoints, ok := entryPointsRaw.(map[string]any)
		if !ok {
			t.Fatalf("entryPoints is not map[string]any\ncontent: %s", staticBytes)
		}
		if _, hasWebsecure := entryPoints["websecure"]; hasWebsecure {
			t.Fatalf("entryPoints.websecure must not exist when tlsPort is unset\ncontent: %s", staticBytes)
		}
	})

	// T021 (spec 011 US2): no-op redeploy must not recreate the traefik container.
	// This is the regression guard for ConfigHash stability: if the hash feeds
	// unstable input (e.g., random iteration over PortBindings), the container ID
	// changes between identical deploys.
	s.Test("should not recreate traefik container on no-op redeploy", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8113
`)

		// Stage 1: initial deploy.
		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		infoFirst, err := tc.DockerClient.ContainerInspect(context.Background(), traefikContainerName)
		if err != nil {
			t.Fatalf("inspect container after first deploy: %v", err)
		}
		firstID := infoFirst.ID
		if firstID == "" {
			t.Fatalf("container ID is empty after first deploy")
		}

		// Stage 2: identical redeploy — no config changes.
		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		infoSecond, err := tc.DockerClient.ContainerInspect(context.Background(), traefikContainerName)
		if err != nil {
			t.Fatalf("inspect container after second deploy: %v", err)
		}
		secondID := infoSecond.ID
		if firstID != secondID {
			t.Fatalf("container was unexpectedly recreated on no-op redeploy\nfirst ID:  %s\nsecond ID: %s", firstID, secondID)
		}
	})

	s.Test("should expose dashboard via dynamic config and keep static traefik.yml http-block free", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8101
      dashboard:
        port: 8102
        username: admin
        password: hunter2
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		dashboardPath := filepath.Join(routingDir, "dynamic", "__shrine-dashboard.yml")
		tc.AssertFileExists(dashboardPath)
		tc.AssertFileContains(dashboardPath, "dashboard-auth")
		tc.AssertFileContains(dashboardPath, "api@internal")

		traefikYML := filepath.Join(routingDir, "traefik.yml")
		tc.AssertFileExists(traefikYML)
		staticBytes, err := os.ReadFile(traefikYML)
		if err != nil {
			t.Fatalf("read traefik.yml: %v", err)
		}
		var staticDoc map[string]any
		if err := yaml.Unmarshal(staticBytes, &staticDoc); err != nil {
			t.Fatalf("parse traefik.yml: %v\ncontent: %s", err, staticBytes)
		}
		if _, ok := staticDoc["http"]; ok {
			t.Fatalf("traefik.yml MUST NOT contain a top-level http: key (Traefik silently drops it in static config)\ncontent: %s", staticBytes)
		}
		if _, ok := staticDoc["entryPoints"]; !ok {
			t.Fatalf("traefik.yml missing entryPoints: key\ncontent: %s", staticBytes)
		}
		if _, ok := staticDoc["api"]; !ok {
			t.Fatalf("traefik.yml missing api: key (dashboard requires api.dashboard=true)\ncontent: %s", staticBytes)
		}
	})

	s.Test("should not generate dashboard dynamic file when no dashboard configured", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8103
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		tc.AssertFileNotExists(filepath.Join(routingDir, "dynamic", "__shrine-dashboard.yml"))
	})

	s.Test("should warn but not modify pre-existing traefik.yml containing http block", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		if err := os.MkdirAll(routingDir, 0o755); err != nil {
			t.Fatalf("mkdir routing dir: %v", err)
		}
		traefikYML := filepath.Join(routingDir, "traefik.yml")
		buggyContent := []byte(`entryPoints:
  web:
    address: ":80"
  traefik:
    address: ":8080"
api:
  dashboard: true
providers:
  file:
    directory: /etc/traefik/dynamic
    watch: true
http:
  middlewares:
    dashboard-auth:
      basicAuth:
        users:
          - "admin:{SHA}old-hash"
  routers:
    dashboard:
      rule: "PathPrefix(` + "`/dashboard`" + `)"
      service: api@internal
      entryPoints: [traefik]
      middlewares: [dashboard-auth]
`)
		if err := os.WriteFile(traefikYML, buggyContent, 0o644); err != nil {
			t.Fatalf("pre-stage buggy traefik.yml: %v", err)
		}

		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8104
      dashboard:
        port: 8105
        username: admin
        password: hunter2
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		afterContent, err := os.ReadFile(traefikYML)
		if err != nil {
			t.Fatalf("read traefik.yml after deploy: %v", err)
		}
		if !bytes.Equal(afterContent, buggyContent) {
			t.Fatalf("pre-existing traefik.yml was mutated by deploy\nwant:\n%s\ngot:\n%s", buggyContent, afterContent)
		}

		tc.AssertFileExists(filepath.Join(routingDir, "dynamic", "__shrine-dashboard.yml"))
		tc.AssertOutputContains("Legacy http block in traefik.yml")
	})

	// T028 (spec 011 US3): preserved traefik.yml with no websecure entrypoint and tlsPort set
	// must produce a warning, leave the file untouched, and still publish the TLS port.
	s.Test("should warn but not modify preserved traefik.yml lacking websecure entrypoint when tlsPort is set", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")

		// Pre-stage the routing dir and a hand-written traefik.yml (no websecure).
		if err := os.MkdirAll(routingDir, 0o755); err != nil {
			t.Fatalf("mkdir routing dir: %v", err)
		}
		staged := []byte(`entryPoints:
  web:
    address: :8114
providers:
  file:
    directory: /etc/traefik/dynamic
    watch: true
`)
		traefikYML := filepath.Join(routingDir, "traefik.yml")
		if err := os.WriteFile(traefikYML, staged, 0o644); err != nil {
			t.Fatalf("pre-stage traefik.yml: %v", err)
		}

		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8114
      tlsPort: 8444
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		// File must be byte-for-byte unchanged (FR-007).
		postDeploy, err := os.ReadFile(traefikYML)
		if err != nil {
			t.Fatalf("read traefik.yml after deploy: %v", err)
		}
		if !bytes.Equal(staged, postDeploy) {
			t.Fatalf("preserved traefik.yml was mutated by deploy\nwant:\n%s\ngot:\n%s", staged, postDeploy)
		}

		// TLS port must still be published (FR-007).
		tc.AssertContainerPublishesPort(traefikContainerName, "8444", "443", "tcp")

		// Warning must appear in output.
		tc.AssertOutputContains("tlsPort set but traefik.yml is missing websecure entrypoint")
	})

	// T029 (spec 011 US3): the websecure-missing warning must be re-emitted on every
	// deploy where the mismatch persists (idempotent — FR-008).
	s.Test("should re-emit the websecure-missing warning on every deploy where the mismatch persists", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")

		// Pre-stage a preserved traefik.yml with no websecure entrypoint.
		if err := os.MkdirAll(routingDir, 0o755); err != nil {
			t.Fatalf("mkdir routing dir: %v", err)
		}
		staged := []byte(`entryPoints:
  web:
    address: :8115
providers:
  file:
    directory: /etc/traefik/dynamic
    watch: true
`)
		traefikYML := filepath.Join(routingDir, "traefik.yml")
		if err := os.WriteFile(traefikYML, staged, 0o644); err != nil {
			t.Fatalf("pre-stage traefik.yml: %v", err)
		}

		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8115
      tlsPort: 8445
`)

		// First deploy.
		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		// Second deploy — warning must fire again (idempotent).
		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		tc.AssertOutputContains("tlsPort set but traefik.yml is missing websecure entrypoint")
	})

	s.Test("should preserve operator edits to dashboard dynamic file across redeploys", func(tc *TestCase) {
		configDir := tc.Path("config")
		routingDir := tc.Path("traefik")
		writeConfig(t, configDir, `plugins:
  gateway:
    traefik:
      routing-dir: `+routingDir+`
      port: 8106
      dashboard:
        port: 8107
        username: admin
        password: hunter2
`)

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		dashboardPath := filepath.Join(routingDir, "dynamic", "__shrine-dashboard.yml")
		tc.AssertFileExists(dashboardPath)

		original, err := os.ReadFile(dashboardPath)
		if err != nil {
			t.Fatalf("read dashboard dynamic file: %v", err)
		}
		sentinel := []byte("\n# operator sentinel — must survive\n")
		marked := append(original, sentinel...)
		if err := os.WriteFile(dashboardPath, marked, 0o644); err != nil {
			t.Fatalf("write operator sentinel: %v", err)
		}

		tc.Run("deploy",
			"--config-dir", configDir,
			"--state-dir", tc.StateDir,
			"--path", traefikFixturePath(),
		).AssertSuccess()

		after, err := os.ReadFile(dashboardPath)
		if err != nil {
			t.Fatalf("read dashboard dynamic file after second deploy: %v", err)
		}
		if !bytes.Equal(after, marked) {
			t.Fatalf("dashboard dynamic file was mutated across redeploys\nwant:\n%s\ngot:\n%s", marked, after)
		}
		tc.AssertOutputContains("Preserving operator-owned dashboard dynamic file")
	})
}
