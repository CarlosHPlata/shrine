package cmd_test

import "testing"

func TestDeployTeam_RequiresArg(t *testing.T) {
	assertRequiresOneArg(t, "deploy", "team")
}
