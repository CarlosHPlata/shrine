package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/CarlosHPlata/shrine/cmd"
)

func TestBump_RequiresArg(t *testing.T) {
	for _, sub := range []string{"application", "app", "resource", "res"} {
		t.Run(sub, func(t *testing.T) { assertRequiresOneArg(t, "bump", sub) })
	}
}

func TestBump_NoSubcommandPrintsHelp(t *testing.T) {
	var out bytes.Buffer
	cmd.SetOutput(&out)
	cmd.SetArgs([]string{"bump", "--state-dir", t.TempDir(), "--config-dir", t.TempDir()})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v\nOutput: %s", err, out.String())
	}
	got := out.String()
	for _, want := range []string{"application", "resource"} {
		if !strings.Contains(got, want) {
			t.Errorf("help output missing %q:\n%s", want, got)
		}
	}
}

// findBumpSub resolves a bump subcommand through the Cobra tree, so an alias
// is looked up the way the operator types it.
func findBumpSub(t *testing.T, name string) *cobra.Command {
	t.Helper()
	sub, _, err := cmd.RootCmd().Find([]string{"bump", name})
	if err != nil {
		t.Fatalf("finding bump %s: %v", name, err)
	}
	return sub
}

func TestBump_LongStatesRollback(t *testing.T) {
	const want = "Rolling back is a bump to an earlier version."
	for _, sub := range []string{"application", "resource"} {
		t.Run(sub, func(t *testing.T) {
			long := strings.Join(strings.Fields(findBumpSub(t, sub).Long), " ")
			if !strings.Contains(long, want) {
				t.Errorf("bump %s Long missing %q:\n%s", sub, want, long)
			}
		})
	}
}

func TestBump_PathFlagMatchesDeploy(t *testing.T) {
	const want = "Directory containing manifest files (overrides specsDir in config.yml)"
	for _, sub := range []string{"application", "resource"} {
		t.Run(sub, func(t *testing.T) {
			flag := findBumpSub(t, sub).InheritedFlags().Lookup("path")
			if flag == nil {
				t.Fatalf("bump %s has no inherited --path flag", sub)
			}
			if flag.Shorthand != "p" {
				t.Errorf("--path shorthand = %q, want %q", flag.Shorthand, "p")
			}
			if flag.Usage != want {
				t.Errorf("--path usage = %q, want %q", flag.Usage, want)
			}
		})
	}
}

func TestBump_UnknownArtifactNamesResolvedDir(t *testing.T) {
	specsDir := t.TempDir()
	var out bytes.Buffer
	cmd.SetOutput(&out)
	cmd.SetArgs([]string{"bump", "app", "nope", "-v", "1", "--dry-run=false",
		"--path", specsDir, "--state-dir", t.TempDir(), "--config-dir", t.TempDir()})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an artifact with no manifest")
	}
	if !strings.Contains(err.Error(), "no manifest found in ") {
		t.Errorf("expected the unknown-artifact message, got: %v", err)
	}
	if !strings.Contains(err.Error(), specsDir) {
		t.Errorf("expected the message to name %q, got: %v", specsDir, err)
	}
}

func TestBump_DryRunBuildsNoBundle(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:1")
	specsDir := t.TempDir()
	var out bytes.Buffer
	cmd.SetOutput(&out)
	cmd.SetArgs([]string{"bump", "app", "nope", "-v", "1", "--dry-run",
		"--path", specsDir, "--state-dir", t.TempDir(), "--config-dir", t.TempDir()})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error for an artifact with no manifest")
	}
	if !strings.Contains(err.Error(), "no manifest found") {
		t.Errorf("expected the unknown-artifact message, got: %v", err)
	}
	if lower := strings.ToLower(err.Error()); strings.Contains(lower, "docker") || strings.Contains(lower, "127.0.0.1:1") {
		t.Errorf("dry run reached the Docker client: %v", err)
	}
}
