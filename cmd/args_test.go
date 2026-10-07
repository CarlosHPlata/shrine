package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/cmd"
)

// Pins on the "accepts 1 arg" substring so Cobra upgrades don't break the assertion.
func assertRequiresOneArg(t *testing.T, args ...string) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOutput(&out)
	cmd.SetArgs(append(args, "--state-dir", t.TempDir(), "--config-dir", t.TempDir()))

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected an arg-count error for %q", strings.Join(args, " "))
	}
	if !strings.Contains(err.Error(), "accepts 1 arg") {
		t.Errorf("expected Cobra arg-count error, got: %v", err)
	}
}
