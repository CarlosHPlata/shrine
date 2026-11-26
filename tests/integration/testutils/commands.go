//go:build integration

package testutils

// Deploy runs `shrine deploy` on the manifests in dir against the test's
// state dir, so a test reads as tc.Deploy(dir).AssertSuccess().
func (tc *TestCase) Deploy(dir string) *TestCase {
	tc.t.Helper()
	return tc.Run("deploy", "--path", dir, "--state-dir", tc.StateDir)
}
