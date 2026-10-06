//go:build integration

package integration_test

import (
	"strings"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

const (
	unresolvableImageRef = "localhost:1/shrine/unresolvable:1.0.0"
	fixedTagImageRef     = "traefik/whoami:v1.10.2"
)

func newPreflightSuite(t *testing.T) *Suite {
	t.Helper()
	s := NewDockerSuite(t, testTeam)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", fixturesPath("team"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	return s
}

func preflightDeploy(tc *TestCase, fixture string, flags ...string) *TestCase {
	args := append([]string{"deploy"}, flags...)
	args = append(args, "--path", fixturesPath(fixture), "--state-dir", tc.StateDir)
	return tc.Run(args...)
}

// assertEveryLineBefore fails unless every occurrence of needle precedes the
// first occurrence of marker.
func assertEveryLineBefore(tc *TestCase, out, needle, marker string) {
	limit := strings.Index(out, marker)
	if limit < 0 {
		tc.Fatalf("expected %q in output:\n%s", marker, out)
	}
	if last := strings.LastIndex(out, needle); last < 0 || last > limit {
		tc.Fatalf("expected every %q line before %q:\n%s", needle, marker, out)
	}
}

// TestPreflightImageResolve covers spec 032: every image of the deploy set is
// resolved before any container or network operation (PRD R-14, R-16, M2, M5).
func TestPreflightImageResolve(t *testing.T) {
	s := newPreflightSuite(t)

	// US1: the broken artifact depends on both healthy ones, so it is last in
	// deploy order; before this feature the healthy containers and the team
	// network would already exist when the pull failed.
	s.Test("an unresolvable image among healthy artifacts leaves Docker untouched", func(tc *TestCase) {
		preflightDeploy(tc, "preflight-unresolvable").
			AssertFailure().
			AssertStderrContains(`application "zz-broken"`).
			AssertStderrContains(unresolvableImageRef)

		tc.AssertContainerNotExists(testTeam + ".cache-ok")
		tc.AssertContainerNotExists(testTeam + ".web-ok")
		tc.AssertContainerNotExists(testTeam + ".zz-broken")
		tc.AssertNetworkNotExists("shrine." + testTeam + ".private")
	})

	// US3: the preview lists the step, one line per artifact, before the first
	// network or container operation, and touches nothing.
	s.Test("dry run prints the resolution step before any container operation", func(tc *TestCase) {
		preflightDeploy(tc, "resources", "--dry-run").
			AssertSuccess().
			AssertOutputContains("[DOCKER] ImageResolve: name=" + testTeam + ".test-cache image=traefik/whoami policy=Always -> manifest-owned").
			AssertOutputContains("[DOCKER] ImageResolve: name=" + testTeam + ".whoami-res image=traefik/whoami policy=Always -> manifest-owned")

		out := tc.RunResult().Stdout
		assertEveryLineBefore(tc, out, "[DOCKER] ImageResolve:", "[DOCKER] CreatePlatformNetwork")
		assertEveryLineBefore(tc, out, "[DOCKER] ImageResolve:", "[DOCKER] ContainerCreate:")

		tc.AssertContainerNotExists(testTeam + ".test-cache")
		tc.AssertContainerNotExists(testTeam + ".whoami-res")
	})

	// US4: a fixed tag keeps today's IfNotPresent behaviour and the resolved
	// version line names it; the container is not recreated.
	s.Test("a fixed tag present locally is reused without a pull and the container is kept", func(tc *TestCase) {
		preflightDeploy(tc, "preflight-fixed-tag").AssertSuccess()
		firstID := containerID(tc, testTeam+".whoami-fixed")

		preflightDeploy(tc, "preflight-fixed-tag").
			AssertSuccess().
			AssertOutputNotContains("Pulling image " + fixedTagImageRef).
			AssertOutputNotContains("Pulled image " + fixedTagImageRef).
			AssertOutputContains("Resolved " + testTeam + ".whoami-fixed " + fixedTagImageRef)

		if got := containerID(tc, testTeam+".whoami-fixed"); got != firstID {
			tc.Fatalf("redeploy recreated the container: ID %q -> %q", firstID, got)
		}
	})

	// US4: an untagged image keeps today's Always behaviour.
	s.Test("an untagged image is pulled on every deploy", func(tc *TestCase) {
		preflightDeploy(tc, "basic").
			AssertSuccess().
			AssertOutputContains("Pulled image traefik/whoami")

		preflightDeploy(tc, "basic").
			AssertSuccess().
			AssertOutputContains("Pulled image traefik/whoami")
	})
}
