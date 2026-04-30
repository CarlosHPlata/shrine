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

func TestTraefikPlugin(t *testing.T) {
	s := NewDockerSuite(t, traefikTestTeam)

	s.BeforeEach(func(tc *TestCase) {
		cleanupTraefikContainer(tc)
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", traefikFixturePath(),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})
	s.AfterEach(func(tc *TestCase) {
		cleanupTraefikContainer(tc)
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
}
