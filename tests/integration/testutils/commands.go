//go:build integration

package testutils

// Deploy runs `shrine deploy` on the manifests in dir against the test's
// state dir, so a test reads as tc.Deploy(dir).AssertSuccess().
func (tc *TestCase) Deploy(dir string) *TestCase {
	tc.t.Helper()
	return tc.Run("deploy", "--path", dir, "--state-dir", tc.StateDir)
}

// ApplyTeams runs `shrine apply teams` on the team manifests in dir.
func (tc *TestCase) ApplyTeams(dir string) *TestCase {
	tc.t.Helper()
	return tc.Run("apply", "teams", "--path", dir, "--state-dir", tc.StateDir)
}

// Get runs `shrine get` with the given arguments against the test's state
// dir, so a test reads as tc.Get("apps").AssertSuccess().
func (tc *TestCase) Get(args ...string) *TestCase {
	tc.t.Helper()
	return tc.runOnState("get", args)
}

// Describe runs `shrine describe` with the given arguments against the
// test's state dir.
func (tc *TestCase) Describe(args ...string) *TestCase {
	tc.t.Helper()
	return tc.runOnState("describe", args)
}

// Status runs `shrine status` with the given arguments against the test's
// state dir.
func (tc *TestCase) Status(args ...string) *TestCase {
	tc.t.Helper()
	return tc.runOnState("status", args)
}

func (tc *TestCase) runOnState(command string, args []string) *TestCase {
	tc.t.Helper()
	full := append([]string{command}, args...)
	return tc.Run(append(full, "--state-dir", tc.StateDir)...)
}
