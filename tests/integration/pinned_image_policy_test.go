//go:build integration

package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

const (
	pinnedSourceOld = "traefik/whoami:v1.10.1"
	pinnedSourceNew = "traefik/whoami:v1.10.2"
	pinnedRepoTag   = "shrine/whoami:latest"
	pinnedApp       = testTeam + ".whoami-pinned"
	pinnedResource  = testTeam + ".cache-pinned"
	zeroDigest      = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
)

func pinnedFixturesPath(parts ...string) string {
	_, f, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(f), "..", "..", "tests", "testdata", "pinned")
	return filepath.Join(append([]string{base}, parts...)...)
}

// pinnedWorld is what BeforeEach prepares for every scenario: a registry
// whose latest is the old whoami, the digest it assigned, and a manifest
// directory naming it under Pinned.
type pinnedWorld struct {
	registry  *LocalRegistry
	oldDigest string
	oldID     string
	specsDir  string
}

func (w *pinnedWorld) pinnedRef() string {
	return w.registry.Host + "/shrine/whoami@" + w.oldDigest
}

func newPinnedSuite(t *testing.T) (*Suite, map[*TestCase]*pinnedWorld) {
	t.Helper()
	s := NewDockerSuite(t, testTeam)
	worlds := map[*TestCase]*pinnedWorld{}

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", fixturesPath("team"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		registry := StartLocalRegistry(tc)
		digest := registry.PushAs(tc, pinnedSourceOld, pinnedRepoTag)
		specsDir := filepath.Join(tc.TempDir(), "specs")
		WritePinnedFixture(tc, specsDir, registry.Host, "1")
		worlds[tc] = &pinnedWorld{
			registry:  registry,
			oldDigest: digest,
			oldID:     tc.ImageIDOf(pinnedSourceOld),
			specsDir:  specsDir,
		}
	})

	return s, worlds
}

func pinnedDeploy(tc *TestCase, specsDir string, flags ...string) *TestCase {
	args := append([]string{"deploy"}, flags...)
	args = append(args, "--path", specsDir, "--state-dir", tc.StateDir)
	return tc.Run(args...)
}

func pinnedTeardown(tc *TestCase) {
	tc.Run("teardown", testTeam, "--state-dir", tc.StateDir).AssertSuccess()
}

func pinsPath(tc *TestCase) string {
	return filepath.Join(tc.StateDir, testTeam, "pins.txt")
}

func deploymentsPath(tc *TestCase) string {
	return filepath.Join(tc.StateDir, testTeam, "deployments.txt")
}

func readFileOrEmpty(tc *TestCase, path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		tc.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

func nonEmptyLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func pinnedContainerID(tc *TestCase, name string) string {
	info, err := tc.DockerClient.ContainerInspect(context.Background(), name)
	if err != nil {
		tc.Fatalf("inspecting container %s: %v", name, err)
	}
	return info.ID
}

func containerImageID(tc *TestCase, name string) string {
	info, err := tc.DockerClient.ContainerInspect(context.Background(), name)
	if err != nil {
		tc.Fatalf("inspecting container %s: %v", name, err)
	}
	return info.Image
}

// assertPinnedWorld checks both containers run exactly the pinned digest and
// were created from the digest reference (FR-006, SC-001).
func assertPinnedWorld(tc *TestCase, w *pinnedWorld) {
	for _, name := range []string{pinnedApp, pinnedResource} {
		tc.AssertContainerRunning(name)
		tc.AssertContainerImage(name, w.pinnedRef())
		if got := containerImageID(tc, name); got != w.oldID {
			tc.Fatalf("container %s runs image %s, want the pinned image %s", name, got, w.oldID)
		}
	}
}

// TestPinnedImagePolicy covers spec 033: the Pinned policy pins on the first
// deploy and keeps the exact version across every later deploy (PRD G1, G5,
// R-09 to R-13, R-15, R-16; metrics M1, M5, M6).
func TestPinnedImagePolicy(t *testing.T) {
	s, worlds := newPinnedSuite(t)

	// US2: a fixed version under Pinned is rejected at planning time with the
	// field named, before anything is resolved or written.
	s.Test("a manifest naming a fixed version under Pinned is rejected naming the field", func(tc *TestCase) {
		pinnedDeploy(tc, pinnedFixturesPath("fixed-version"), "--dry-run").
			AssertFailure().
			AssertStderrContains(`application "whoami-fixed": spec.image "127.0.0.1:5000/shrine/whoami:v1.10.2" names a fixed version but the image pull policy is Pinned`).
			AssertStderrContains(`resource "cache-fixed": spec.version "16" names a fixed version but the image pull policy is Pinned`)
		tc.AssertFileNotExists(pinsPath(tc))
	})

	// US2: an unknown policy value is rejected at parse time (FR-001).
	s.Test("an unknown image pull policy value is rejected", func(tc *TestCase) {
		pinnedDeploy(tc, pinnedFixturesPath("unknown-policy"), "--dry-run").
			AssertFailure().
			AssertStderrContains("spec.imagePullPolicy must be one of Always, IfNotPresent, Pinned")
	})

	// US1 and SC-001: first deploy pins; ten cycles mixing redeploy, recreate,
	// teardown and deploy, and a wiped image cache all run the same exact
	// version while the registry's latest has moved on.
	s.Test("first deploy pins and ten mixed cycles keep the exact version while latest moves", func(tc *TestCase) {
		w := worlds[tc]

		pinnedDeploy(tc, w.specsDir).
			AssertSuccess().
			AssertOutputContains("📌 Pinned " + pinnedApp + " at latest@").
			AssertOutputContains("📌 Pinned " + pinnedResource + " at latest@")
		pins := nonEmptyLines(readFileOrEmpty(tc, pinsPath(tc)))
		if len(pins) != 2 {
			tc.Fatalf("pins.txt should hold one line per pinned artifact, got %d:\n%s", len(pins), strings.Join(pins, "\n"))
		}
		for _, line := range pins {
			if !strings.Contains(line, w.pinnedRef()) {
				tc.Fatalf("pin line %q does not record the exact version %s", line, w.pinnedRef())
			}
		}
		assertPinnedWorld(tc, w)

		// latest moves; the pin must not follow.
		w.registry.PushAs(tc, pinnedSourceNew, pinnedRepoTag)

		round := 1
		redeploy := func() {
			before := pinnedContainerID(tc, pinnedApp)
			pinnedDeploy(tc, w.specsDir).
				AssertSuccess().
				AssertOutputContains("📌 Using pinned " + pinnedApp + " latest@").
				AssertOutputNotContains("Pulling image")
			if after := pinnedContainerID(tc, pinnedApp); after != before {
				tc.Fatalf("a plain redeploy with a reused pin recreated the container (%s -> %s)", before, after)
			}
		}
		recreate := func() {
			round++
			before := pinnedContainerID(tc, pinnedApp)
			WritePinnedFixture(tc, w.specsDir, w.registry.Host, string(rune('0'+round)))
			pinnedDeploy(tc, w.specsDir).
				AssertSuccess().
				AssertOutputContains("📌 Using pinned " + pinnedApp + " latest@")
			if after := pinnedContainerID(tc, pinnedApp); after == before {
				tc.Fatalf("an env change should recreate the container, id stayed %s", before)
			}
		}
		teardownAndDeploy := func() {
			pinnedTeardown(tc)
			pinnedDeploy(tc, w.specsDir).
				AssertSuccess().
				AssertOutputContains("📌 Using pinned " + pinnedApp + " latest@")
		}
		wipeAndDeploy := func() {
			pinnedTeardown(tc)
			tc.RemoveImage(w.pinnedRef())
			pinnedDeploy(tc, w.specsDir).
				AssertSuccess().
				AssertOutputContains("Pulling image " + w.pinnedRef()).
				AssertOutputContains("📌 Using pinned " + pinnedApp + " latest@")
		}

		cycles := []func(){
			redeploy, recreate, teardownAndDeploy, wipeAndDeploy, redeploy,
			recreate, teardownAndDeploy, wipeAndDeploy, redeploy, redeploy,
		}
		for i, cycle := range cycles {
			cycle()
			assertPinnedWorld(tc, w)
			if lines := nonEmptyLines(readFileOrEmpty(tc, pinsPath(tc))); len(lines) != 2 {
				tc.Fatalf("cycle %d changed pins.txt line count to %d", i+1, len(lines))
			}
		}
	})

	// US3 and SC-006: delete application, delete team, and a manifest-owned
	// deploy release the pin; teardown and redeploy do not.
	s.Test("delete application releases the pin and the next deploy pins afresh", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		newDigest := w.registry.PushAs(tc, pinnedSourceNew, pinnedRepoTag)
		pinnedTeardown(tc)

		before := readFileOrEmpty(tc, pinsPath(tc))
		tc.Run("delete", "application", "whoami-pinned", "--dry-run", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("[dry-run] would release image pin " + w.pinnedRef() + " for " + testTeam + "/whoami-pinned")
		if after := readFileOrEmpty(tc, pinsPath(tc)); after != before {
			tc.Fatalf("delete --dry-run changed pins.txt:\nbefore:\n%s\nafter:\n%s", before, after)
		}

		tc.Run("delete", "application", "whoami-pinned", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("Released image pin for " + testTeam + "/whoami-pinned.")
		if strings.Contains(readFileOrEmpty(tc, pinsPath(tc)), "whoami-pinned") {
			tc.Fatalf("delete application left the pin in place:\n%s", readFileOrEmpty(tc, pinsPath(tc)))
		}

		pinnedDeploy(tc, w.specsDir).
			AssertSuccess().
			AssertOutputContains("📌 Pinned " + pinnedApp + " at latest@").
			AssertOutputContains("📌 Using pinned " + pinnedResource + " latest@")
		tc.AssertContainerImage(pinnedApp, w.registry.Host+"/shrine/whoami@"+newDigest)
		tc.AssertContainerImage(pinnedResource, w.pinnedRef())
	})

	s.Test("a deploy under a manifest-owned policy releases the pin and returning to Pinned pins afresh", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		newDigest := w.registry.PushAs(tc, pinnedSourceNew, pinnedRepoTag)
		w.registry.PushAs(tc, pinnedSourceNew, "shrine/whoami:v1.10.2")
		pinnedTeardown(tc)

		WriteManifestOwnedFixture(tc, w.specsDir, w.registry.Host, "v1.10.2")
		pinnedDeploy(tc, w.specsDir).
			AssertSuccess().
			AssertOutputContains("🔎 Resolved " + pinnedApp + " ").
			AssertOutputNotContains("📌 Pinned " + pinnedApp)
		if strings.Contains(readFileOrEmpty(tc, pinsPath(tc)), "whoami-pinned") {
			tc.Fatalf("a manifest-owned deploy left the pin in place:\n%s", readFileOrEmpty(tc, pinsPath(tc)))
		}
		tc.AssertContainerImage(pinnedApp, w.registry.Host+"/shrine/whoami:v1.10.2")

		pinnedTeardown(tc)
		WritePinnedFixture(tc, w.specsDir, w.registry.Host, "1")
		pinnedDeploy(tc, w.specsDir).
			AssertSuccess().
			AssertOutputContains("📌 Pinned " + pinnedApp + " at latest@")
		tc.AssertContainerImage(pinnedApp, w.registry.Host+"/shrine/whoami@"+newDigest)
	})

	s.Test("delete team releases every pin the team held", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		pinnedTeardown(tc)

		tc.Run("delete", "team", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains(`Released 2 image pin(s) for team "` + testTeam + `".`)
		if lines := nonEmptyLines(readFileOrEmpty(tc, pinsPath(tc))); len(lines) != 0 {
			tc.Fatalf("delete team left pins behind:\n%s", strings.Join(lines, "\n"))
		}

		tc.Run("apply", "teams", "--path", fixturesPath("team"), "--state-dir", tc.StateDir).AssertSuccess()
	})

	// US4 and SC-004: dry run previews would-pin and reuse, writes nothing,
	// and repeated runs leave recorded state byte for byte unchanged.
	s.Test("dry run previews the pin decision and writes nothing", func(tc *TestCase) {
		w := worlds[tc]

		pinnedDeploy(tc, w.specsDir, "--dry-run").
			AssertSuccess().
			AssertOutputContains("[DOCKER] ImageResolve: name=" + pinnedApp + " image=" + w.registry.Host + "/shrine/whoami policy=Pinned -> would resolve newest and pin").
			AssertOutputContains("[DOCKER] ImageResolve: name=" + pinnedResource + " image=" + w.registry.Host + "/shrine/whoami policy=Pinned -> would resolve newest and pin")
		tc.AssertFileNotExists(pinsPath(tc))

		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		pinsBefore := readFileOrEmpty(tc, pinsPath(tc))
		deploymentsBefore := readFileOrEmpty(tc, deploymentsPath(tc))

		for i := 0; i < 2; i++ {
			pinnedDeploy(tc, w.specsDir, "--dry-run").
				AssertSuccess().
				AssertOutputContains("policy=Pinned -> pinned " + w.pinnedRef() + " (latest, ").
				AssertOutputNotContains("📌")
		}
		if got := readFileOrEmpty(tc, pinsPath(tc)); got != pinsBefore {
			tc.Fatalf("dry run changed pins.txt:\nbefore:\n%s\nafter:\n%s", pinsBefore, got)
		}
		if got := readFileOrEmpty(tc, deploymentsPath(tc)); got != deploymentsBefore {
			tc.Fatalf("dry run changed deployments.txt:\nbefore:\n%s\nafter:\n%s", deploymentsBefore, got)
		}
	})

	// US5 and SC-005: a pin the registry no longer serves fails in the
	// resolution step with zero changes and a message naming the way out.
	s.Test("a pin the registry no longer serves stops the deploy before any change", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		pinnedTeardown(tc)

		rewritten := strings.ReplaceAll(readFileOrEmpty(tc, pinsPath(tc)), w.oldDigest, zeroDigest)
		if err := os.WriteFile(pinsPath(tc), []byte(rewritten), 0o600); err != nil {
			tc.Fatalf("rewriting pins.txt: %v", err)
		}

		pinnedDeploy(tc, w.specsDir).
			AssertFailure().
			AssertStderrContains("is no longer served by the registry").
			AssertStderrContains(`"` + w.registry.Host + "/shrine/whoami@" + zeroDigest + `"`).
			AssertStderrContains(`run "shrine bump application whoami-pinned" to choose another version`)
		tc.AssertContainerNotExists(pinnedApp)
		tc.AssertContainerNotExists(pinnedResource)
		tc.AssertNetworkNotExists("shrine." + testTeam + ".private")
	})

	// FR-015: manifests that do not name Pinned behave exactly as before.
	s.Test("manifests without the value behave exactly as before", func(tc *TestCase) {
		pinnedDeploy(tc, fixturesPath("resources")).
			AssertSuccess().
			AssertOutputContains("🔎 Resolved " + testTeam + ".test-cache traefik/whoami").
			AssertOutputNotContains("📌")
		tc.AssertFileNotExists(pinsPath(tc))
	})
}
