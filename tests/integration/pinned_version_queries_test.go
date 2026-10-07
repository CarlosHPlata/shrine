//go:build integration

package integration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

const pinnedOwnedApp = "whoami-owned"

// shortHex is the table form of an exact version: the first twelve hex
// characters after sha256: (design TD-11).
func shortHex(digest string) string {
	return strings.TrimPrefix(digest, "sha256:")[:12]
}

// writeManifestOwnedSibling adds a manifest-owned application beside the
// pinned pair so one listing holds both kinds of row.
func writeManifestOwnedSibling(tc *TestCase, dir string) {
	manifest := fmt.Sprintf(`apiVersion: shrine/v1
kind: Application
metadata:
  name: %s
  owner: %s
spec:
  image: %s
  port: 80
`, pinnedOwnedApp, testTeam, pinnedSourceOld)
	if err := os.WriteFile(filepath.Join(dir, "owned.yml"), []byte(manifest), 0o644); err != nil {
		tc.Fatalf("writing manifest-owned sibling: %v", err)
	}
}

// replacePinDigest rewrites the pin on record the way a bump will once T6
// lands, leaving the running container on the old exact version.
func replacePinDigest(tc *TestCase, old, new string) {
	data := readFileOrEmpty(tc, pinsPath(tc))
	if !strings.Contains(data, old) {
		tc.Fatalf("pins.txt does not hold %s:\n%s", old, data)
	}
	if err := os.WriteFile(pinsPath(tc), []byte(strings.ReplaceAll(data, old, new)), 0o644); err != nil {
		tc.Fatalf("rewriting pins.txt: %v", err)
	}
}

// pinnedDateOf reads the YYYY-MM-DD part of an artifact's pin date from state,
// which is what describe must print.
func pinnedDateOf(tc *TestCase, name string) string {
	for _, line := range nonEmptyLines(readFileOrEmpty(tc, pinsPath(tc))) {
		fields := strings.Fields(line)
		if len(fields) == 5 && fields[1] == name {
			return fields[4][:len("2006-01-02")]
		}
	}
	tc.Fatalf("no pin on record for %s:\n%s", name, readFileOrEmpty(tc, pinsPath(tc)))
	return ""
}

func stdoutLineContaining(tc *TestCase, anchor string) string {
	for _, line := range strings.Split(tc.RunResult().Stdout, "\n") {
		if strings.Contains(line, anchor) {
			return line
		}
	}
	tc.Fatalf("expected a stdout line containing %q\nstdout: %s", anchor, tc.RunResult().Stdout)
	return ""
}

// TestPinnedVersionQueries covers spec 035: get, describe, and status show a
// pinned artifact's version, the pin, and the running image, and nothing shows
// a pin without a deployment record (PRD J3, R-17 to R-20; metric M3).
func TestPinnedVersionQueries(t *testing.T) {
	s, worlds := newPinnedSuite(t)

	// US1, FR-001, FR-002, SC-001: the readable form in VERSION beside the
	// manifest reference of a manifest-owned row.
	s.Test("get shows the readable form for pinned rows and the manifest reference for others", func(tc *TestCase) {
		w := worlds[tc]
		writeManifestOwnedSibling(tc, w.specsDir)
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		readable := "latest@" + shortHex(w.oldDigest)

		tc.Run("get", "deployed", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-pinned", readable).
			AssertOutputLineContains("cache-pinned", readable).
			AssertOutputLineContains(pinnedOwnedApp, pinnedSourceOld).
			AssertOutputNotContains(w.oldDigest)
		assertColumnsInOrder(tc, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")

		tc.Run("get", "apps", "--team", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-pinned", readable).
			AssertOutputLineContains(pinnedOwnedApp, pinnedSourceOld)
		assertColumnsInOrder(tc, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")

		tc.Run("get", "resources", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("cache-pinned", readable).
			AssertOutputNotContains(pinnedOwnedApp)
		assertColumnsInOrder(tc, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")
	})

	// US2, FR-004, FR-005, SC-002: the pin and the running image agree right
	// after a deploy.
	s.Test("describe shows the pin and the running image in agreement", func(tc *TestCase) {
		w := worlds[tc]
		writeManifestOwnedSibling(tc, w.specsDir)
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		readable := "latest@" + shortHex(w.oldDigest)

		tc.Run("describe", "app", "whoami-pinned", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("Pull policy:", "Pinned").
			AssertOutputLineContains("Pinned:", w.pinnedRef()).
			AssertOutputLineContains("Pinned:", readable).
			AssertOutputLineContains("Pinned:", pinnedDateOf(tc, "whoami-pinned")).
			AssertOutputLineContains("Running image:", w.pinnedRef())

		tc.Run("describe", "resource", "cache-pinned", "--team", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("Pull policy:", "Pinned").
			AssertOutputLineContains("Pinned:", w.pinnedRef()).
			AssertOutputLineContains("Running image:", w.pinnedRef())

		tc.Run("describe", "app", pinnedOwnedApp, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("Running image:", pinnedSourceOld).
			AssertOutputNotContains("Pinned:")
	})

	// US2, FR-005, SC-002: a pin recorded after the last deploy is visible as
	// the two lines disagreeing; the table follows the pin, not the container.
	s.Test("a pin replaced without a deploy shows as a difference", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		newDigest := w.registry.PushAs(tc, pinnedSourceNew, pinnedRepoTag)
		replacePinDigest(tc, w.oldDigest, newDigest)

		tc.Run("describe", "app", "whoami-pinned", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("Pinned:", newDigest).
			AssertOutputLineContains("Running image:", w.oldDigest)
		if line := stdoutLineContaining(tc, "Pinned:"); strings.Contains(line, w.oldDigest) {
			tc.Fatalf("the Pinned: line still shows the old exact version: %s", line)
		}

		tc.Run("get", "deployed", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-pinned", "latest@"+shortHex(newDigest))
	})

	// US3, FR-007, FR-008, SC-004: the live view carries the running image,
	// a digest reference shortened to twelve hex characters.
	s.Test("status shows the running image in an IMAGE column", func(tc *TestCase) {
		w := worlds[tc]
		writeManifestOwnedSibling(tc, w.specsDir)
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		pinnedCell := w.registry.Host + "/shrine/whoami@" + shortHex(w.oldDigest)

		tc.Run("status", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-pinned", pinnedCell).
			AssertOutputLineContains("cache-pinned", pinnedCell).
			AssertOutputLineContains(pinnedOwnedApp, pinnedSourceOld).
			AssertOutputNotContains(w.oldDigest)
		assertColumnsInOrder(tc, "NAME", "KIND", "RUNNING", "STATUS", "IMAGE", "IMAGE ID")

		tc.Run("status", "app", "whoami-pinned", "--team", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-pinned", pinnedCell)
		assertColumnsInOrder(tc, "NAME", "KIND", "RUNNING", "STATUS", "IMAGE", "IMAGE ID")
	})

	// US4, FR-009, FR-010, SC-005: a torn-down artifact is absent everywhere,
	// its pin is neither shown nor touched, and it returns on redeploy.
	s.Test("after teardown nothing shows the artifact or its pin", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		readable := "latest@" + shortHex(w.oldDigest)
		dateBefore := pinnedDateOf(tc, "whoami-pinned")
		pinsBefore := readFileOrEmpty(tc, pinsPath(tc))

		pinnedTeardown(tc)

		tc.Run("get", "deployed", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputNotContains("whoami-pinned").
			AssertOutputNotContains("latest@")
		tc.Run("describe", "app", "whoami-pinned", "--state-dir", tc.StateDir).
			AssertFailure().
			AssertStderrContains("not found").
			AssertOutputNotContains("Pinned:")
		tc.Run("status", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputNotContains("whoami-pinned")
		if after := readFileOrEmpty(tc, pinsPath(tc)); after != pinsBefore {
			tc.Fatalf("a query changed pins.txt:\nbefore:\n%s\nafter:\n%s", pinsBefore, after)
		}

		pinnedDeploy(tc, w.specsDir).
			AssertSuccess().
			AssertOutputContains("📌 Using pinned " + pinnedApp)
		tc.Run("get", "deployed", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-pinned", readable)
		tc.Run("describe", "app", "whoami-pinned", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("Pinned:", dateBefore)
	})
}
