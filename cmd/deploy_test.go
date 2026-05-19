package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/cmd"
)

func TestDeployTeam_RequiresArg(t *testing.T) {
	var out bytes.Buffer
	cmd.SetOutput(&out)
	cmd.SetArgs([]string{"deploy", "team"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when 'deploy team' is invoked without a team name")
	}
	// Cobra emits its standard "accepts 1 arg(s), received 0" message — pin on
	// the substring rather than the full text so cobra upgrades don't break us.
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}
