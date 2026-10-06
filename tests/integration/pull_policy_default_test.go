//go:build integration

package integration_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

const (
	configSourcedRemedy = `(from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default`
	invalidPolicyError  = "loading config: imagePullPolicy: must be one of Always, IfNotPresent, Pinned"
)

func policyFixturesPath(parts ...string) string {
	_, f, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(f), "..", "..", "tests", "testdata", "pull-policy-default")
	return filepath.Join(append([]string{base}, parts...)...)
}

// configDirWithPolicy writes a config.yml holding only the default pull
// policy; an empty value writes an empty file, the "no setting" case.
func configDirWithPolicy(t *testing.T, tc *TestCase, value string) string {
	t.Helper()
	dir := tc.Path("config")
	content := ""
	if value != "" {
		content = "imagePullPolicy: " + value + "\n"
	}
	writeConfig(t, dir, content)
	return dir
}

func dryRunWithDefault(tc *TestCase, cfgDir, fixtureDir string) *TestCase {
	return tc.Run("deploy", "--dry-run",
		"--config-dir", cfgDir,
		"--path", fixtureDir,
		"--state-dir", tc.StateDir,
	)
}

func resolveLine(name, image, policy, decision string) string {
	return "[DOCKER] ImageResolve: name=" + testTeam + "." + name + " image=" + image + " policy=" + policy + " -> " + decision
}

// TestPullPolicyDefaultPrecedence covers spec 034 US1 to US3: the
// configuration default is the middle layer between the manifest field and
// the derived rule, a Pinned default fails fixed-version manifests with a
// message naming the setting, and an invalid value stops every command.
func TestPullPolicyDefaultPrecedence(t *testing.T) {
	s := NewSuite(t)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.Path("state")
		tc.Run("apply", "teams",
			"--path", fixturesPath("team"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("no setting keeps the derived rule for every manifest", func(tc *TestCase) {
		cfg := configDirWithPolicy(t, tc, "")
		dryRunWithDefault(tc, cfg, policyFixturesPath("versioned")).
			AssertSuccess().
			AssertOutputContains(resolveLine("app-latest", "traefik/whoami:latest", "Always", "manifest-owned")).
			AssertOutputContains(resolveLine("app-fixed", "traefik/whoami:v1.10.2", "IfNotPresent", "manifest-owned")).
			AssertOutputContains(resolveLine("res-fixed", "traefik/whoami", "Always", "manifest-owned")).
			AssertOutputContains(resolveLine("app-own-ifnotpresent", "traefik/whoami:v1.10.1", "IfNotPresent", "manifest-owned"))
	})

	s.Test("a Pinned default pins every manifest that names no policy", func(tc *TestCase) {
		cfg := configDirWithPolicy(t, tc, "Pinned")
		dryRunWithDefault(tc, cfg, policyFixturesPath("pinned-shape")).
			AssertSuccess().
			AssertOutputContains(resolveLine("app-untagged", "traefik/whoami", "Pinned", "would resolve newest and pin")).
			AssertOutputContains(resolveLine("res-noversion", "traefik/whoami", "Pinned", "would resolve newest and pin")).
			AssertOutputContains(resolveLine("res-own-pinned", "traefik/whoami", "Pinned", "would resolve newest and pin"))
		tc.AssertFileNotExists(filepath.Join(tc.StateDir, testTeam, "pins.txt"))
	})

	s.Test("a Pinned default fails fixed-version manifests naming the setting and the two ways out", func(tc *TestCase) {
		cfg := configDirWithPolicy(t, tc, "Pinned")
		dryRunWithDefault(tc, cfg, policyFixturesPath("versioned")).
			AssertFailure().
			AssertStderrContains(`application "app-fixed": spec.image "traefik/whoami:v1.10.2" names a fixed version but the image pull policy is Pinned ` + configSourcedRemedy).
			AssertStderrContains(`resource "res-fixed": spec.version "16" names a fixed version but the image pull policy is Pinned ` + configSourcedRemedy).
			AssertStderrNotContains(`"app-latest"`).
			AssertStderrNotContains(`"app-own-ifnotpresent"`)
	})

	s.Test("a manifest that names Pinned itself keeps the manifest-sourced message", func(tc *TestCase) {
		cfg := configDirWithPolicy(t, tc, "Pinned")
		dryRunWithDefault(tc, cfg, policyFixturesPath("own-pinned-fixed")).
			AssertFailure().
			AssertStderrContains(`application "app-own-pinned-fixed": spec.image "traefik/whoami:v1.10.2" names a fixed version but the image pull policy is Pinned; use "traefik/whoami" or "traefik/whoami:latest"`).
			AssertStderrNotContains("from config.yml")
	})

	s.Test("an IfNotPresent default applies where the derived rule would say Always", func(tc *TestCase) {
		cfg := configDirWithPolicy(t, tc, "IfNotPresent")
		dryRunWithDefault(tc, cfg, policyFixturesPath("versioned")).
			AssertSuccess().
			AssertOutputContains(resolveLine("app-latest", "traefik/whoami:latest", "IfNotPresent", "manifest-owned"))
	})

	s.Test("an Always default applies where the derived rule would say IfNotPresent", func(tc *TestCase) {
		cfg := configDirWithPolicy(t, tc, "Always")
		dryRunWithDefault(tc, cfg, policyFixturesPath("versioned")).
			AssertSuccess().
			AssertOutputContains(resolveLine("app-fixed", "traefik/whoami:v1.10.2", "Always", "manifest-owned"))
	})

	s.Test("a manifest-owned default still requires a Resource version", func(tc *TestCase) {
		cfg := configDirWithPolicy(t, tc, "IfNotPresent")
		dryRunWithDefault(tc, cfg, policyFixturesPath("pinned-shape")).
			AssertFailure().
			AssertStderrContains(`resource "res-noversion": spec.version is required`).
			AssertStderrNotContains(`"res-own-pinned"`)
	})

	s.Test("an invalid value stops every command before it acts", func(tc *TestCase) {
		cfg := configDirWithPolicy(t, tc, "pinned")
		tc.Run("get", "deployed", "--config-dir", cfg, "--state-dir", tc.StateDir).
			AssertFailure().
			AssertStderrContains(invalidPolicyError).
			AssertStderrNotContains("Validation errors")
	})
}

// generatedWorld is what BeforeEach prepares for the generate scenarios: a
// registry serving shrine/whoami:latest, a config dir with the Pinned
// default, and an empty specs dir.
type generatedWorld struct {
	registry *LocalRegistry
	cfgDir   string
	specsDir string
}

func generateWithDefault(tc *TestCase, w *generatedWorld, args ...string) *TestCase {
	full := append([]string{"generate"}, args...)
	full = append(full, "--config-dir", w.cfgDir, "--path", w.specsDir, "--team", testTeam)
	return tc.Run(full...)
}

func deployGenerated(tc *TestCase, w *generatedWorld) *TestCase {
	return tc.Run("deploy", "--config-dir", w.cfgDir, "--path", w.specsDir, "--state-dir", tc.StateDir)
}

func readGenerated(tc *TestCase, w *generatedWorld, name string) string {
	return readFileOrEmpty(tc, filepath.Join(w.specsDir, name+".yml"))
}

func assertNoPolicyOrVersion(tc *TestCase, content, file string) {
	if strings.Contains(content, "imagePullPolicy") {
		tc.Fatalf("%s names a policy; generated manifests must follow the default:\n%s", file, content)
	}
	if strings.Contains(content, "version:") {
		tc.Fatalf("%s names a version under a Pinned default:\n%s", file, content)
	}
}

// TestPullPolicyDefaultGenerateThenDeploy covers spec 034 US4 together with
// US1: under a Pinned default the generate commands write manifests that
// deploy and pin without edits (R-08), and changing the default later
// releases the pins (FR-007).
func TestPullPolicyDefaultGenerateThenDeploy(t *testing.T) {
	s := NewDockerSuite(t, testTeam)
	worlds := map[*TestCase]*generatedWorld{}

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", fixturesPath("team"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		registry := StartLocalRegistry(tc)
		registry.PushAs(tc, pinnedSourceOld, pinnedRepoTag)
		specsDir := filepath.Join(tc.TempDir(), "specs")
		if err := os.MkdirAll(specsDir, 0o755); err != nil {
			tc.Fatalf("creating specs dir: %v", err)
		}
		worlds[tc] = &generatedWorld{
			registry: registry,
			cfgDir:   configDirWithPolicy(t, tc, "Pinned"),
			specsDir: specsDir,
		}
	})

	s.AfterEach(func(tc *TestCase) {
		tc.Run("teardown", testTeam, "--state-dir", tc.StateDir)
	})

	s.Test("generated manifests deploy and pin under a Pinned default, and a changed default releases the pin", func(tc *TestCase) {
		w := worlds[tc]
		repo := w.registry.Host + "/shrine/whoami"

		generateWithDefault(tc, w, "application", "whoami-gen", "--image", repo, "--port", "80").AssertSuccess()
		generateWithDefault(tc, w, "resource", "cache-gen", "--type", repo).AssertSuccess()

		app := readGenerated(tc, w, "whoami-gen")
		if !strings.Contains(app, "  image: "+repo+"\n") {
			tc.Fatalf("generated application does not name the bare repository:\n%s", app)
		}
		assertNoPolicyOrVersion(tc, app, "whoami-gen.yml")
		assertNoPolicyOrVersion(tc, readGenerated(tc, w, "cache-gen"), "cache-gen.yml")

		deployGenerated(tc, w).
			AssertSuccess().
			AssertOutputContains("📌 Pinned " + testTeam + ".whoami-gen at latest@").
			AssertOutputContains("📌 Pinned " + testTeam + ".cache-gen at latest@")
		if lines := nonEmptyLines(readFileOrEmpty(tc, pinsPath(tc))); len(lines) != 2 {
			tc.Fatalf("expected two pins after the generated deploy, got %d:\n%s", len(lines), strings.Join(lines, "\n"))
		}

		writeConfig(t, w.cfgDir, "imagePullPolicy: IfNotPresent\n")
		if err := os.Remove(filepath.Join(w.specsDir, "cache-gen.yml")); err != nil {
			tc.Fatalf("removing generated resource: %v", err)
		}
		deployGenerated(tc, w).
			AssertSuccess().
			AssertOutputNotContains("📌 Using pinned " + testTeam + ".whoami-gen")
		if pins := readFileOrEmpty(tc, pinsPath(tc)); strings.Contains(pins, "whoami-gen") {
			tc.Fatalf("the application's pin survived a manifest-owned default:\n%s", pins)
		}
	})

	s.Test("apply -f holds a single manifest to the configured default", func(tc *TestCase) {
		w := worlds[tc]
		fixed := policyFixturesPath("versioned", "app-fixed.yml")

		tc.Run("apply", "-f", fixed,
			"--path", policyFixturesPath("versioned"),
			"--config-dir", w.cfgDir,
			"--state-dir", tc.StateDir,
		).
			AssertFailure().
			AssertStderrContains(`application "app-fixed": spec.image "traefik/whoami:v1.10.2" names a fixed version but the image pull policy is Pinned ` + configSourcedRemedy)
		tc.AssertFileNotExists(pinsPath(tc))
	})

	s.Test("generate with no image or version flag writes the Pinned defaults", func(tc *TestCase) {
		w := worlds[tc]

		generateWithDefault(tc, w, "application", "web").AssertSuccess()
		generateWithDefault(tc, w, "resource", "db").AssertSuccess()

		web := readGenerated(tc, w, "web")
		if !strings.Contains(web, "  image: web\n") {
			tc.Fatalf("generated application should default to the bare name under Pinned:\n%s", web)
		}
		assertNoPolicyOrVersion(tc, web, "web.yml")

		db := readGenerated(tc, w, "db")
		if !strings.Contains(db, "  type: postgres\n  networking:\n") {
			tc.Fatalf("generated resource should omit the version line under Pinned:\n%s", db)
		}
		assertNoPolicyOrVersion(tc, db, "db.yml")
	})
}
