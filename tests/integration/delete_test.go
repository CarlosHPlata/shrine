//go:build integration

package integration_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func deleteFixturesPath(parts ...string) string {
	_, f, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(f), "..", "..", "tests", "testdata", "delete")
	return filepath.Join(append([]string{base}, parts...)...)
}

const deleteTestTeam = "shrine-delete-test"

func TestDeleteTeam(t *testing.T) {
	s := NewSuite(t)
	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
	})

	s.Test("should delete a team from state", func(tc *TestCase) {
		specsDir := tc.TempDir()
		tc.Run("generate", "team", "my-team", "--path", specsDir).
			AssertSuccess()
		tc.Run("apply", "teams", "--path", specsDir, "--state-dir", tc.StateDir).
			AssertSuccess()
		tc.Run("delete", "team", "my-team", "--state-dir", tc.StateDir).
			AssertSuccess()
		tc.AssertTeamNotInState("my-team")
	})

	s.Test("should show error when deleting a team that does not exist", func(tc *TestCase) {
		tc.Run("delete", "team", "nonexistent-team", "--state-dir", tc.StateDir).
			AssertFailure()
	})
}

func TestDeleteTeamWithDeployments(t *testing.T) {
	s := NewDockerSuite(t, deleteTestTeam)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", deleteFixturesPath(),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("should show error when the team has active deployments", func(tc *TestCase) {
		tc.Run("deploy",
			"--path", deleteFixturesPath(),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
		tc.Run("delete", "team", deleteTestTeam, "--state-dir", tc.StateDir).
			AssertFailure()
		tc.AssertStderrContains("active deployments")
	})
}

// stateSnapshot captures the two files a delete may write, keyed by path, so a
// refused or dry-run delete can be proven to have written nothing.
func stateSnapshot(tc *TestCase) map[string]string {
	snapshot := map[string]string{}
	for _, path := range []string{pinsPath(tc), deploymentsPath(tc)} {
		snapshot[path] = readFileOrEmpty(tc, path)
	}
	return snapshot
}

func assertStateUnchanged(tc *TestCase, before map[string]string) {
	for path, want := range before {
		if got := readFileOrEmpty(tc, path); got != want {
			tc.Fatalf("%s changed:\nbefore:\n%s\nafter:\n%s", path, want, got)
		}
	}
}

// TestDeleteResource covers spec 037: delete resource retires a torn-down
// resource's pin and record, refuses while the container exists, and every
// delete verb releases the pins it deletes.
func TestDeleteResource(t *testing.T) {
	s, worlds := newPinnedSuite(t)

	// US1 and SC-001.
	s.Test("delete of a torn-down resource releases the pin, and the next deploy pins afresh", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		newDigest := w.registry.PushAs(tc, pinnedSourceNew, pinnedRepoTag)
		pinnedTeardown(tc)

		tc.Run("delete", "resource", "cache-pinned", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("Released image pin for " + testTeam + "/cache-pinned.").
			// Teardown already dropped the record, so only the pin is held.
			AssertOutputNotContains("Removed deployment record")
		pins := readFileOrEmpty(tc, pinsPath(tc))
		if strings.Contains(pins, "cache-pinned") {
			tc.Fatalf("delete resource left cache-pinned in pins.txt:\n%s", pins)
		}
		// The application of the same team must survive the resource delete.
		if !strings.Contains(pins, "whoami-pinned") {
			tc.Fatalf("delete resource removed whoami-pinned from pins.txt:\n%s", pins)
		}

		pinnedDeploy(tc, w.specsDir).
			AssertSuccess().
			AssertOutputContains("📌 Pinned " + pinnedResource + " at latest@").
			AssertOutputContains("📌 Using pinned " + pinnedApp + " latest@")
		tc.AssertContainerImage(pinnedResource, w.registry.Host+"/shrine/whoami@"+newDigest)
		tc.AssertContainerImage(pinnedApp, w.pinnedRef())
	})

	// US2 and SC-002: Docker is authoritative, so a live container blocks the
	// delete, dry run included.
	s.Test("delete while the container exists is refused and points at teardown", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		before := stateSnapshot(tc)

		for _, flags := range [][]string{{}, {"--dry-run"}} {
			args := append([]string{"delete", "resource", "cache-pinned"}, append(flags, "--state-dir", tc.StateDir)...)
			tc.Run(args...).
				AssertFailure().
				AssertStderrContains("still has a container").
				AssertStderrContains("shrine teardown " + testTeam)
			assertStateUnchanged(tc, before)
		}
	})

	// US3 and SC-003.
	s.Test("dry run prints the pin and writes nothing", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		pinnedTeardown(tc)
		before := stateSnapshot(tc)

		tc.Run("delete", "resource", "cache-pinned", "--dry-run", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("[dry-run] would release image pin " + w.pinnedRef() + " for " + testTeam + "/cache-pinned").
			AssertOutputNotContains("would remove deployment record")
		assertStateUnchanged(tc, before)
	})

	// US4 and SC-005.
	s.Test("--team finds the resource, and so does the automatic search", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		pinnedTeardown(tc)
		before := stateSnapshot(tc)

		tc.Run("delete", "resource", "cache-pinned", "--team", "other", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains(`Nothing to delete for resource "cache-pinned" in team "other".`)
		assertStateUnchanged(tc, before)

		tc.Run("delete", "resource", "cache-pinned", "--team", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("Released image pin for " + testTeam + "/cache-pinned.")

		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		pinnedTeardown(tc)
		tc.Run("delete", "resource", "cache-pinned", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("Released image pin for " + testTeam + "/cache-pinned.")
		if pins := readFileOrEmpty(tc, pinsPath(tc)); !strings.Contains(pins, "whoami-pinned") {
			tc.Fatalf("delete resource released the application's pin:\n%s", pins)
		}
	})

	// US1 scenario 4: nothing held is not an error, so retries are safe.
	s.Test("a name nothing is held for is a soft success", func(tc *TestCase) {
		tc.Run("delete", "resource", "ghost", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains(`Nothing to delete for resource "ghost".`)
	})

	// US5 and SC-004.
	s.Test("every delete verb releases the pins it deletes", func(tc *TestCase) {
		w := worlds[tc]
		pinnedDeploy(tc, w.specsDir).AssertSuccess()
		pinnedTeardown(tc)

		tc.Run("delete", "application", "whoami-pinned", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("Released image pin for " + testTeam + "/whoami-pinned.")
		if pins := readFileOrEmpty(tc, pinsPath(tc)); !strings.Contains(pins, "cache-pinned") {
			tc.Fatalf("delete application released the resource's pin:\n%s", pins)
		}

		tc.Run("delete", "resource", "cache-pinned", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("Released image pin for " + testTeam + "/cache-pinned.")
		if lines := nonEmptyLines(readFileOrEmpty(tc, pinsPath(tc))); len(lines) != 0 {
			tc.Fatalf("delete resource left pins behind:\n%s", strings.Join(lines, "\n"))
		}

		pinnedDeploy(tc, w.specsDir).
			AssertSuccess().
			AssertOutputContains("📌 Pinned " + pinnedApp + " at latest@").
			AssertOutputContains("📌 Pinned " + pinnedResource + " at latest@")
		pinnedTeardown(tc)

		tc.Run("delete", "team", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains(`Released 2 image pin(s) for team "` + testTeam + `".`)
		if lines := nonEmptyLines(readFileOrEmpty(tc, pinsPath(tc))); len(lines) != 0 {
			tc.Fatalf("delete team left pins behind:\n%s", strings.Join(lines, "\n"))
		}

		// AfterEach cleanup expects the team to exist.
		tc.Run("apply", "teams", "--path", fixturesPath("team"), "--state-dir", tc.StateDir).AssertSuccess()
	})
}
