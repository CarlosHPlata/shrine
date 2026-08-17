//go:build integration

package integration_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func publishFixturesPath(parts ...string) string {
	_, f, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(f), "..", "..", "tests", "testdata", "publish")
	return filepath.Join(append([]string{base}, parts...)...)
}

const publishTeam = "shrine-publish-test"

func publishStateFile(tc *TestCase) string {
	return filepath.Join(tc.StateDir, "hostports.txt")
}

// waitHTTPOK polls url until it answers 200 or the deadline passes.
func waitHTTPOK(tc *TestCase, url string) {
	client := &http.Client{Timeout: time.Second}
	var lastErr error
	for i := 0; i < 30; i++ {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(300 * time.Millisecond)
	}
	tc.Fatalf("url %s never answered 200: %v", url, lastErr)
}

func assertHostPortsFile(tc *TestCase, wantLines []string, forbidden []string) {
	data, err := os.ReadFile(publishStateFile(tc))
	if err != nil {
		tc.Fatalf("reading hostports.txt: %v", err)
	}
	content := string(data)
	for _, want := range wantLines {
		if !strings.Contains(content, want) {
			tc.Fatalf("hostports.txt should contain %q, got:\n%s", want, content)
		}
	}
	for _, bad := range forbidden {
		if strings.Contains(content, bad) {
			tc.Fatalf("hostports.txt should NOT contain %q, got:\n%s", bad, content)
		}
	}
}

func newPublishSuite(t *testing.T) *Suite {
	s := NewDockerSuite(t, publishTeam)
	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", publishFixturesPath("team"),
			"--state-dir", tc.StateDir,
			"--config-dir", publishFixturesPath("config-plain"),
		).AssertSuccess()
	})
	return s
}

func publishRun(tc *TestCase, fixture string, extra ...string) *TestCase {
	args := append([]string{"deploy"}, extra...)
	args = append(args,
		"--path", publishFixturesPath(fixture),
		"--state-dir", tc.StateDir,
		"--config-dir", publishFixturesPath("config-plain"),
	)
	return tc.Run(args...)
}

func publishRunWithConfig(tc *TestCase, fixture, configDir string, extra ...string) *TestCase {
	args := append([]string{"deploy"}, extra...)
	args = append(args,
		"--path", publishFixturesPath(fixture),
		"--state-dir", tc.StateDir,
		"--config-dir", publishFixturesPath(configDir),
	)
	return tc.Run(args...)
}

func TestPublishConflicts(t *testing.T) {
	s := newPublishSuite(t)

	s.Test("duplicate explicit ports fail dry-run and deploy before any change", func(tc *TestCase) {
		publishRun(tc, "duplicate", "--dry-run").
			AssertFailure().
			AssertStderrContains("port 18081").
			AssertStderrContains(publishTeam + "/whoami-a").
			AssertStderrContains(publishTeam + "/whoami-b")

		publishRun(tc, "duplicate").
			AssertFailure().
			AssertStderrContains("port 18081")

		tc.AssertContainerNotExists(publishTeam + ".whoami-a")
		tc.AssertContainerNotExists(publishTeam + ".whoami-b")
	})

	s.Test("claiming a gateway-reserved port fails dry-run and deploy", func(tc *TestCase) {
		publishRunWithConfig(tc, "reserved", "config-traefik", "--dry-run").
			AssertFailure().
			AssertStderrContains("port 18085").
			AssertStderrContains("reserved")

		publishRunWithConfig(tc, "reserved", "config-traefik").
			AssertFailure().
			AssertStderrContains("reserved")

		tc.AssertContainerNotExists(publishTeam + ".whoami-reserved")
	})

	s.Test("claiming another app's persisted port fails dry-run and deploy", func(tc *TestCase) {
		publishRun(tc, "explicit").AssertSuccess()

		publishRun(tc, "steal", "--dry-run").
			AssertFailure().
			AssertStderrContains("port 18080").
			AssertStderrContains("already allocated").
			AssertStderrContains(publishTeam + "/whoami-pub")

		publishRun(tc, "steal").
			AssertFailure().
			AssertStderrContains("already allocated")

		tc.AssertContainerNotExists(publishTeam + ".whoami-thief")
	})

	s.Test("all conflicts are reported in one invocation", func(tc *TestCase) {
		publishRunWithConfig(tc, "multi", "config-traefik", "--dry-run").
			AssertFailure().
			AssertStderrContains("host port collision: port 18081").
			AssertStderrContains("host port reserved: port 18085")
	})

	s.Test("an app re-claiming its own persisted port is not a conflict", func(tc *TestCase) {
		publishRun(tc, "explicit").AssertSuccess()
		publishRun(tc, "explicit", "--dry-run").AssertSuccess()
		publishRun(tc, "explicit").AssertSuccess()
	})
}

func TestPublishAutomatic(t *testing.T) {
	s := newPublishSuite(t)
	const autoApp = publishTeam + ".whoami-auto"
	const autoKey = publishTeam + "/whoami-auto=30000"

	s.Test("allocates from the automatic range, reports it, and serves HTTP", func(tc *TestCase) {
		publishRun(tc, "auto").AssertSuccess().
			AssertOutputContains("127.0.0.1:30000")

		tc.AssertContainerRunning(autoApp)
		tc.AssertContainerPublishesOnLoopback(autoApp, "30000", "80", "tcp")
		waitHTTPOK(tc, "http://127.0.0.1:30000/")
		assertHostPortsFile(tc, []string{autoKey}, nil)
	})

	s.Test("keeps the port across plain redeploy and forced recreation", func(tc *TestCase) {
		publishRun(tc, "auto").AssertSuccess()
		publishRun(tc, "auto").AssertSuccess().
			AssertOutputContains("127.0.0.1:30000")
		publishRun(tc, "auto-changed").AssertSuccess().
			AssertOutputContains("127.0.0.1:30000")

		tc.AssertContainerPublishesOnLoopback(autoApp, "30000", "80", "tcp")
		tc.AssertContainerEnvVar(autoApp, "FORCE_RECREATE", "1")
		assertHostPortsFile(tc, []string{autoKey}, nil)
	})

	s.Test("keeps the port across teardown and redeploy", func(tc *TestCase) {
		publishRun(tc, "auto").AssertSuccess()

		tc.Run("teardown", publishTeam,
			"--state-dir", tc.StateDir,
			"--config-dir", publishFixturesPath("config-plain"),
		).AssertSuccess()
		tc.AssertContainerNotExists(autoApp)
		assertHostPortsFile(tc, []string{autoKey}, nil)

		publishRun(tc, "auto").AssertSuccess().
			AssertOutputContains("127.0.0.1:30000")
		tc.AssertContainerPublishesOnLoopback(autoApp, "30000", "80", "tcp")
	})

	s.Test("dry-run before the first deploy previews (auto) and writes nothing", func(tc *TestCase) {
		publishRun(tc, "auto", "--dry-run").
			AssertSuccess().
			AssertOutputContains("publish=127.0.0.1:(auto)->80/tcp")

		tc.AssertFileNotExists(publishStateFile(tc))
	})

	s.Test("dry-run after deploy shows the held port without touching state", func(tc *TestCase) {
		publishRun(tc, "auto").AssertSuccess()
		before, err := os.ReadFile(publishStateFile(tc))
		if err != nil {
			tc.Fatalf("reading hostports.txt: %v", err)
		}

		publishRun(tc, "auto", "--dry-run").
			AssertSuccess().
			AssertOutputContains("publish=127.0.0.1:30000->80/tcp")

		after, err := os.ReadFile(publishStateFile(tc))
		if err != nil {
			tc.Fatalf("re-reading hostports.txt: %v", err)
		}
		if string(before) != string(after) {
			tc.Fatalf("dry-run must leave hostports.txt byte-identical:\nbefore: %q\nafter:  %q", before, after)
		}
	})

	s.Test("delete application refuses while the container exists, releases after teardown", func(tc *TestCase) {
		publishRun(tc, "auto").AssertSuccess()

		tc.Run("delete", "application", "whoami-auto",
			"--state-dir", tc.StateDir,
			"--config-dir", publishFixturesPath("config-plain"),
		).AssertFailure().
			AssertStderrContains("teardown")
		assertHostPortsFile(tc, []string{autoKey}, nil)

		tc.Run("teardown", publishTeam,
			"--state-dir", tc.StateDir,
			"--config-dir", publishFixturesPath("config-plain"),
		).AssertSuccess()

		tc.Run("delete", "application", "whoami-auto",
			"--state-dir", tc.StateDir,
			"--config-dir", publishFixturesPath("config-plain"),
		).AssertSuccess().
			AssertOutputContains("30000")
		assertHostPortsFile(tc, nil, []string{"whoami-auto"})
	})

	s.Test("delete team releases the team's remaining allocations", func(tc *TestCase) {
		publishRun(tc, "auto").AssertSuccess()
		tc.Run("teardown", publishTeam,
			"--state-dir", tc.StateDir,
			"--config-dir", publishFixturesPath("config-plain"),
		).AssertSuccess()

		tc.Run("delete", "team", publishTeam,
			"--state-dir", tc.StateDir,
			"--config-dir", publishFixturesPath("config-plain"),
		).AssertSuccess()

		assertHostPortsFile(tc, nil, []string{publishTeam})
	})
}

func TestPublishSemantics(t *testing.T) {
	s := newPublishSuite(t)

	s.Test("the three exposure combinations attach and publish correctly", func(tc *TestCase) {
		publishRun(tc, "semantics").AssertSuccess()

		// publish-only: implied platform attachment + published port
		tc.AssertContainerInNetwork(publishTeam+".whoami-sem-pub", "shrine.platform")
		tc.AssertContainerPublishesOnLoopback(publishTeam+".whoami-sem-pub", "18082", "80", "tcp")

		// exposure-only: attached but publishes nothing
		tc.AssertContainerInNetwork(publishTeam+".whoami-sem-exp", "shrine.platform")
		tc.AssertContainerHasNoPortBindings(publishTeam + ".whoami-sem-exp")

		// both: attached and published, no warning, no error
		tc.AssertContainerInNetwork(publishTeam+".whoami-sem-both", "shrine.platform")
		tc.AssertContainerPublishesOnLoopback(publishTeam+".whoami-sem-both", "18083", "80", "tcp")
	})

	s.Test("dry-run shows the implied attachment next to the publish line", func(tc *TestCase) {
		publishRun(tc, "semantics", "--dry-run").
			AssertSuccess().
			AssertOutputContains("attach to platform network=shrine.platform").
			AssertOutputContains("publish=127.0.0.1:18082->80/tcp")
	})

	s.Test("publish grants no cross-team dependency access", func(tc *TestCase) {
		publishRun(tc, "semantics-crossteam", "--dry-run").
			AssertFailure().
			AssertStderrContains("not reachable cross-team")

		publishRun(tc, "semantics-crossteam").
			AssertFailure().
			AssertStderrContains("not reachable cross-team")

		tc.AssertContainerNotExists(publishTeam + ".whoami-target")
	})
}

func TestPublishExplicit(t *testing.T) {
	s := newPublishSuite(t)

	s.Test("dry-run previews the explicit mapping without touching state", func(tc *TestCase) {
		publishRun(tc, "explicit", "--dry-run").
			AssertSuccess().
			AssertOutputContains("publish=127.0.0.1:18080->80/tcp")

		tc.AssertFileNotExists(publishStateFile(tc))
	})

	s.Test("publishes the explicit port on loopback and serves HTTP", func(tc *TestCase) {
		publishRun(tc, "explicit").AssertSuccess().
			AssertOutputContains("127.0.0.1:18080")

		tc.AssertContainerRunning(publishTeam + ".whoami-pub")
		tc.AssertContainerPublishesOnLoopback(publishTeam+".whoami-pub", "18080", "80", "tcp")
		waitHTTPOK(tc, "http://127.0.0.1:18080/")
		assertHostPortsFile(tc, []string{publishTeam + "/whoami-pub=18080"}, nil)
	})

	s.Test("changing the explicit port recreates the container on the new port", func(tc *TestCase) {
		publishRun(tc, "explicit").AssertSuccess()
		publishRun(tc, "explicit-changed").AssertSuccess()

		tc.AssertContainerRunning(publishTeam + ".whoami-pub")
		tc.AssertContainerPublishesOnLoopback(publishTeam+".whoami-pub", "19090", "80", "tcp")
		tc.AssertContainerDoesNotPublishPort(publishTeam+".whoami-pub", "18080")
		waitHTTPOK(tc, "http://127.0.0.1:19090/")
		assertHostPortsFile(tc, []string{publishTeam + "/whoami-pub=19090"}, []string{"=18080"})
	})

	s.Test("an app without publish exposes no host ports", func(tc *TestCase) {
		publishRun(tc, "plain").AssertSuccess()

		tc.AssertContainerRunning(publishTeam + ".whoami-plain")
		tc.AssertContainerHasNoPortBindings(publishTeam + ".whoami-plain")
		tc.AssertFileNotExists(publishStateFile(tc))
	})
}
