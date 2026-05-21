//go:build integration

package integration_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

func deployTeamInferFixturesPath(parts ...string) string {
	_, f, _, _ := runtime.Caller(0)
	base := filepath.Join(filepath.Dir(f), "..", "..", "tests", "testdata", "deploy_team_infer")
	return filepath.Join(append([]string{base}, parts...)...)
}

const (
	inferTeamA = "shrine-team-a"
	inferTeamB = "shrine-team-b"
)

// TestDeployTeam_Inference covers US1, US2, and US3: same-team valueFrom
// inference and the cross-team / absent-target fail-fast contract.
func TestDeployTeam_Inference(t *testing.T) {
	s := NewDockerSuite(t, inferTeamA)

	s.BeforeEach(func(tc *TestCase) {
		CleanupTeam(tc, inferTeamB)
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", deployTeamInferFixturesPath("teams"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.AfterEach(func(tc *TestCase) {
		CleanupTeam(tc, inferTeamB)
	})

	// ---- US1 ----

	s.Test("US1: dry-run orders Resource before Application via inferred edge", func(tc *TestCase) {
		tc.Run("deploy", "team", inferTeamA,
			"--dry-run",
			"--path", deployTeamInferFixturesPath("us1"),
			"--state-dir", tc.StateDir,
		).AssertSuccess().
			AssertOutputContains("Deploy order:").
			AssertOutputContains("Resource:ops-bot-db").
			AssertOutputContains("Application:ops-bot").
			AssertOutputContains("Resource:ops-bot-db (inferred from env DB_CONNECTION_URL)")

		out := tc.RunResult().Stdout
		idxRes := strings.Index(out, "Resource:ops-bot-db")
		idxApp := strings.Index(out, "Application:ops-bot")
		if idxRes < 0 || idxApp < 0 || idxRes >= idxApp {
			tc.Fatalf("expected Resource before Application in plan summary\n%s", out)
		}
	})

	s.Test("US1: real deploy succeeds and creates containers in inferred order", func(tc *TestCase) {
		tc.Run("deploy", "team", inferTeamA,
			"--path", deployTeamInferFixturesPath("us1"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.AssertContainerRunning(inferTeamA + ".ops-bot-db")
		tc.AssertContainerRunning(inferTeamA + ".ops-bot")
	})

	s.Test("US1: explicit dep produces no duplicate edge and no inferred tag", func(tc *TestCase) {
		tc.Run("deploy", "team", inferTeamA,
			"--dry-run",
			"--path", deployTeamInferFixturesPath("us1_explicit"),
			"--state-dir", tc.StateDir,
		).AssertSuccess().
			AssertOutputContains("Resource:ops-bot-db")

		out := tc.RunResult().Stdout
		// Explicit dep means the line for the dep does NOT carry an inferred tag.
		appBlock := afterSubstring(out, "Application:ops-bot")
		if strings.Contains(appBlock, "inferred from env") {
			tc.Fatalf("explicit dep must not be tagged as inferred:\n%s", appBlock)
		}
		if strings.Count(appBlock, "Resource:ops-bot-db") > 1 {
			tc.Fatalf("expected exactly one Resource:ops-bot-db line under Application:ops-bot, got:\n%s", appBlock)
		}
	})

	// ---- US2 ----

	s.Test("US2: dry-run orders producer Application before consumer Application", func(tc *TestCase) {
		tc.Run("deploy", "team", inferTeamA,
			"--dry-run",
			"--path", deployTeamInferFixturesPath("us2"),
			"--state-dir", tc.StateDir,
		).AssertSuccess().
			AssertOutputContains("Application:producer").
			AssertOutputContains("Application:consumer").
			AssertOutputContains("Application:producer (inferred from env PROD_HOST)")

		out := tc.RunResult().Stdout
		// The step lines look like "  1. Application:producer" / "  2. Application:consumer".
		// We want producer's STEP line to come before consumer's STEP line.
		idxProducerStep := strings.Index(out, ". Application:producer")
		idxConsumerStep := strings.Index(out, ". Application:consumer")
		if idxProducerStep < 0 || idxConsumerStep < 0 || idxProducerStep >= idxConsumerStep {
			tc.Fatalf("expected producer step before consumer step in plan summary\n%s", out)
		}
	})

	// ---- US3 ----

	s.Test("US3: cross-team reference without explicit dep fails with enrichment error", func(tc *TestCase) {
		tc.Run("deploy", "team", inferTeamA,
			"--dry-run",
			"--path", deployTeamInferFixturesPath("us3_crossteam"),
			"--state-dir", tc.StateDir,
		).AssertFailure().
			AssertStderrContains(`enrichment: app "ops-bot" env "CACHE_HOST" references resource "shared-cache.HOST" which is not owned by team "shrine-team-a"`).
			AssertStderrContains(`add an explicit spec.dependencies entry (kind: Resource, name: shared-cache)`)

		// No Docker side-effects.
		tc.AssertContainerNotExists(inferTeamA + ".ops-bot")
	})

	s.Test("US3: cross-team reference WITH explicit dep succeeds and is not tagged inferred", func(tc *TestCase) {
		tc.Run("deploy", "team", inferTeamA,
			"--dry-run",
			"--path", deployTeamInferFixturesPath("us3_crossteam_explicit"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		out := tc.RunResult().Stdout
		if strings.Contains(out, "enrichment:") {
			tc.Fatalf("expected no enrichment error, got stderr/out:\n%s\n%s", tc.RunResult().Stdout, tc.RunResult().Stderr)
		}
		appBlock := afterSubstring(out, "Application:ops-bot")
		if !strings.Contains(appBlock, "Resource:shared-cache") {
			tc.Fatalf("expected explicit Resource:shared-cache dep, got:\n%s", appBlock)
		}
		if strings.Contains(appBlock, "Resource:shared-cache (inferred") {
			tc.Fatalf("explicit dep must not be tagged inferred:\n%s", appBlock)
		}
	})

	s.Test("US3: absent target reference fails with enrichment error naming the missing target", func(tc *TestCase) {
		tc.Run("deploy", "team", inferTeamA,
			"--dry-run",
			"--path", deployTeamInferFixturesPath("us3_absent"),
			"--state-dir", tc.StateDir,
		).AssertFailure().
			AssertStderrContains(`enrichment:`).
			AssertStderrContains(`does-not-exist`)
	})

	s.Test("US3: fail-fast reports first offending ref deterministically across runs", func(tc *TestCase) {
		first := tc.Run("deploy", "team", inferTeamA,
			"--dry-run",
			"--path", deployTeamInferFixturesPath("us3_failfast"),
			"--state-dir", tc.StateDir,
		).AssertFailure().RunResult().Stderr

		second := tc.Run("deploy", "team", inferTeamA,
			"--dry-run",
			"--path", deployTeamInferFixturesPath("us3_failfast"),
			"--state-dir", tc.StateDir,
		).AssertFailure().RunResult().Stderr

		firstLine := firstEnrichmentLine(first)
		secondLine := firstEnrichmentLine(second)
		if firstLine == "" {
			tc.Fatalf("no enrichment line in stderr:\n%s", first)
		}
		if firstLine != secondLine {
			tc.Fatalf("non-deterministic first error\nrun 1: %s\nrun 2: %s", firstLine, secondLine)
		}
		// Sorted-by-name: alpha < beta, declaration order: A1 first.
		if !strings.Contains(firstLine, `app "alpha"`) || !strings.Contains(firstLine, `env "A1"`) {
			tc.Fatalf("expected first error on alpha/A1, got: %s", firstLine)
		}
		// Fail-fast: exactly one enrichment line per run.
		if strings.Count(first, "enrichment:") != 1 {
			tc.Fatalf("expected exactly one enrichment line, got %d:\n%s",
				strings.Count(first, "enrichment:"), first)
		}
	})
}

// afterSubstring returns the slice of s starting right after the FIRST
// occurrence of needle, up to the next "  N. " step ordinal or end of string.
// Used to scope assertions to one step's "depends on:" block.
func afterSubstring(s, needle string) string {
	idx := strings.Index(s, needle)
	if idx < 0 {
		return ""
	}
	tail := s[idx:]
	// Find the next "\n  " followed by a digit and ". " — i.e. next step.
	for i := 1; i < len(tail)-4; i++ {
		if tail[i] == '\n' && tail[i+1] == ' ' && tail[i+2] == ' ' &&
			tail[i+3] >= '0' && tail[i+3] <= '9' {
			return tail[:i]
		}
	}
	return tail
}

func firstEnrichmentLine(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		if strings.Contains(line, "enrichment:") {
			return line
		}
	}
	return ""
}
