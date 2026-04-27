package cmd_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/cmd"
)

func TestStatus(t *testing.T) {
	// 1. Setup a temporary state directory so we don't mess with real data
	tmpDir := t.TempDir()

	// 2. Capture output
	var out bytes.Buffer
	cmd.SetOutput(&out)

	// 3. Set arguments
	cmd.SetArgs([]string{"status", "--state-dir", tmpDir})

	// 4. Execute the command in-process
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 5. Assert output matches our expectations
	got := out.String()
	want := "[shrine] Showing platform status..."
	if !strings.Contains(got, want) {
		t.Errorf("got %q, want it to contain %q", got, want)
	}
}

func TestDeployDryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Setup sample team state
	teamsDir := filepath.Join(tmpDir, "teams")
	if err := os.MkdirAll(teamsDir, 0755); err != nil {
		t.Fatal(err)
	}
	teamData := `{
		"apiVersion": "shrine/v1",
		"kind": "Team",
		"metadata": { "name": "team-a" },
		"spec": { "quotas": { "maxApps": 10, "maxResources": 10 } }
	}`
	if err := os.WriteFile(filepath.Join(teamsDir, "team-a.json"), []byte(teamData), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Setup output capture
	var out bytes.Buffer
	cmd.SetOutput(&out)

	// 3. Execute deploy dry-run
	cmd.SetArgs([]string{"deploy", "--path", "./testdata/basic-app", "--dry-run", "--state-dir", tmpDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v\nOutput: %s", err, out.String())
	}

	// 4. Verify the plan contains expected steps
	got := out.String()
	expectedSteps := []string{
		"[DOCKER] NetworkCreate: name=team-a",
		"[DOCKER] ContainerCreate: name=team-a.test-app image=nginx:latest",
		"[ROUTE]  WriteRoute: domain=test.home.lab → test-app:80",
		"[DNS]    AddRecord: test.home.lab → [IP_ADDRESS]",
	}

	for _, step := range expectedSteps {
		if !strings.Contains(got, step) {
			t.Errorf("missing expected step: %q\nFull output:\n%s", step, got)
		}
	}
}
