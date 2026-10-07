//go:build integration

package integration_test

import (
	"strings"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func bump(tc *TestCase, specsDir string, args ...string) *TestCase {
	args = append([]string{"bump"}, args...)
	args = append(args, "--path", specsDir, "--state-dir", tc.StateDir)
	return tc.Run(args...)
}

func pushVersion(tc *TestCase, w *pinnedWorld, tag string) string {
	return w.registry.PushAs(tc, pinnedSourceNew, "shrine/whoami:"+tag)
}

func pinLineFor(tc *TestCase, name string) string {
	for _, line := range nonEmptyLines(readFileOrEmpty(tc, pinsPath(tc))) {
		if strings.Contains(line, " "+name+" ") {
			return line
		}
	}
	return ""
}

// TestBump covers spec 036: bump records a chosen or the newest version as
// the new pin without touching a container, and the next deploy applies it
// (PRD R-21 to R-25; design 4.6).
func TestBump(t *testing.T) {
	s, worlds := newPinnedSuite(t)

	// US1, US2, SC-001, SC-003.
	s.Test("bump to a chosen version records it, deploy applies it, and bump back rolls it back", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		newDigest := pushVersion(tc, w, "v2")
		before := pinnedContainerID(tc, pinnedApp)
		resBefore := pinLineFor(tc, "cache-pinned")

		bump(tc, w.specsDir, "app", "whoami-pinned", "-v", "v2").
			AssertSuccess().
			AssertOutputContains("🔎 Resolving image for " + pinnedApp + " (" + w.registry.Host + "/shrine/whoami:v2)").
			AssertOutputContains("📌 Bumped " + pinnedApp + " to v2@" + shortHex(newDigest)).
			AssertOutputContains("Bumped " + testTeam + "/whoami-pinned: latest@" + shortHex(w.oldDigest) + " -> v2@" + shortHex(newDigest) + `; run "shrine deploy" to apply`)

		appLine := pinLineFor(tc, "whoami-pinned")
		for _, want := range []string{w.registry.Host + "/shrine/whoami:v2", w.registry.Host + "/shrine/whoami@" + newDigest} {
			if !strings.Contains(appLine, want) {
				tc.Fatalf("pin line %q does not hold %s", appLine, want)
			}
		}
		if got := pinLineFor(tc, "cache-pinned"); got != resBefore {
			tc.Fatalf("bumping the application changed the resource's pin:\nbefore: %s\nafter:  %s", resBefore, got)
		}
		if after := pinnedContainerID(tc, pinnedApp); after != before {
			tc.Fatalf("bump touched the container (%s -> %s)", before, after)
		}
		tc.AssertContainerImage(pinnedApp, w.pinnedRef())

		pinnedDeploy(tc, w.specsDir).
			AssertSuccess().
			AssertOutputContains("📌 Using pinned " + pinnedApp + " v2@" + shortHex(newDigest))
		if after := pinnedContainerID(tc, pinnedApp); after == before {
			tc.Fatalf("deploy after a bump should recreate the container, id stayed %s", before)
		}
		tc.AssertContainerImage(pinnedApp, w.registry.Host+"/shrine/whoami@"+newDigest)

		bump(tc, w.specsDir, "application", "whoami-pinned", "-v", w.oldDigest).
			AssertSuccess().
			AssertOutputContains("Bumped " + testTeam + "/whoami-pinned: v2@" + shortHex(newDigest) + " -> " + shortHex(w.oldDigest) + `; run "shrine deploy" to apply`)
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		tc.AssertContainerImage(pinnedApp, w.pinnedRef())
	})

	// US1 scenarios 4 and 7, SC-002.
	s.Test("bump to a version the registry does not serve fails and records nothing", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		pinsBefore := readFileOrEmpty(tc, pinsPath(tc))
		before := pinnedContainerID(tc, pinnedApp)

		bump(tc, w.specsDir, "app", "whoami-pinned", "-v", "v9").
			AssertFailure().
			AssertStderrContains(`application "whoami-pinned"`).
			AssertStderrContains(`pulling image "` + w.registry.Host + `/shrine/whoami:v9"`)
		if got := readFileOrEmpty(tc, pinsPath(tc)); got != pinsBefore {
			tc.Fatalf("a failed bump changed pins.txt:\nbefore:\n%s\nafter:\n%s", pinsBefore, got)
		}
		if after := pinnedContainerID(tc, pinnedApp); after != before {
			tc.Fatalf("a failed bump touched the container (%s -> %s)", before, after)
		}

		bump(tc, w.specsDir, "app", "whoami-pinned", "-v", "not valid!").
			AssertFailure().
			AssertStderrContains(`invalid version "not valid!"`).
			AssertOutputNotContains("Resolving image")
		if got := readFileOrEmpty(tc, pinsPath(tc)); got != pinsBefore {
			tc.Fatalf("an invalid version changed pins.txt:\nbefore:\n%s\nafter:\n%s", pinsBefore, got)
		}
	})

	// US3, SC-004.
	s.Test("bump without a version records the current newest", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		newDigest := pushVersion(tc, w, "latest")
		before := pinnedContainerID(tc, pinnedResource)

		bump(tc, w.specsDir, "res", "cache-pinned").
			AssertSuccess().
			AssertOutputContains("Bumped " + testTeam + "/cache-pinned: latest@" + shortHex(w.oldDigest) + " -> latest@" + shortHex(newDigest))
		if line := pinLineFor(tc, "cache-pinned"); !strings.Contains(line, w.registry.Host+"/shrine/whoami@"+newDigest) {
			tc.Fatalf("pin line %q does not hold the newest exact version %s", line, newDigest)
		}
		if after := pinnedContainerID(tc, pinnedResource); after != before {
			tc.Fatalf("bump touched the container (%s -> %s)", before, after)
		}
		tc.AssertContainerImage(pinnedResource, w.pinnedRef())

		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		tc.AssertContainerImage(pinnedResource, w.registry.Host+"/shrine/whoami@"+newDigest)
	})

	// US4, SC-005.
	s.Test("manifest-owned, unknown, and wrong-team artifacts are refused and nothing is recorded", func(tc *TestCase) {
		w := worlds[tc]
		WriteManifestOwnedFixture(tc, w.specsDir, w.registry.Host, "v1.10.1")

		for _, flags := range [][]string{{}, {"--dry-run"}} {
			bump(tc, w.specsDir, append([]string{"app", "whoami-pinned", "-v", "v2"}, flags...)...).
				AssertFailure().
				AssertStderrContains(`application "whoami-pinned": its version is manifest-owned (imagePullPolicy IfNotPresent); edit the manifest to change it`)
		}
		bump(tc, w.specsDir, "app", "nope", "-v", "v2").
			AssertFailure().
			AssertStderrContains(`application "nope": no manifest found in ` + w.specsDir)
		bump(tc, w.specsDir, "res", "whoami-pinned", "-v", "v2").
			AssertFailure().
			AssertStderrContains(`resource "whoami-pinned": no manifest found in `)
		bump(tc, w.specsDir, "res", "cache-pinned", "--team", "other", "-v", "v2").
			AssertFailure().
			AssertStderrContains(`resource "cache-pinned" not found in team "other"`)
		tc.AssertFileNotExists(pinsPath(tc))
	})

	// US4 scenario 3, SC-006.
	s.Test("bump before the first deploy chooses the version that deploy runs", func(tc *TestCase) {
		w := worlds[tc]
		newDigest := pushVersion(tc, w, "v2")

		bump(tc, w.specsDir, "app", "whoami-pinned", "-v", "v2").
			AssertSuccess().
			AssertOutputContains("Pinned " + testTeam + "/whoami-pinned at v2@" + shortHex(newDigest) + `; run "shrine deploy" to apply`).
			AssertOutputNotContains("->")
		if lines := nonEmptyLines(readFileOrEmpty(tc, pinsPath(tc))); len(lines) != 1 {
			tc.Fatalf("a bump before the first deploy should record exactly one pin, got %d:\n%s", len(lines), strings.Join(lines, "\n"))
		}
		tc.AssertContainerNotExists(pinnedApp)

		pinnedDeploy(tc, w.specsDir).
			AssertSuccess().
			AssertOutputContains("📌 Using pinned " + pinnedApp + " v2@").
			AssertOutputContains("📌 Pinned " + pinnedResource + " at latest@")
		tc.AssertContainerImage(pinnedApp, w.registry.Host+"/shrine/whoami@"+newDigest)
	})

	// US5, SC-007.
	s.Test("dry run prints what it would resolve and writes nothing", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		pushVersion(tc, w, "v2")
		pinsBefore := readFileOrEmpty(tc, pinsPath(tc))

		bump(tc, w.specsDir, "app", "whoami-pinned", "-v", "v2", "--dry-run").
			AssertSuccess().
			AssertOutputContains("[dry-run] would resolve " + w.registry.Host + "/shrine/whoami:v2 and pin " + testTeam + "/whoami-pinned").
			AssertOutputNotContains("Resolving image").
			AssertOutputNotContains("📌")
		bump(tc, w.specsDir, "res", "cache-pinned", "--dry-run").
			AssertSuccess().
			AssertOutputContains("[dry-run] would resolve " + w.registry.Host + "/shrine/whoami and pin " + testTeam + "/cache-pinned")
		if got := readFileOrEmpty(tc, pinsPath(tc)); got != pinsBefore {
			tc.Fatalf("a dry-run bump changed pins.txt:\nbefore:\n%s\nafter:\n%s", pinsBefore, got)
		}
	})
}
