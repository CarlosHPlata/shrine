//go:build integration

package testutils

import (
	"os"
	"path/filepath"
	"strings"
)

// SeedLegacyDeploymentRecords rewrites a team's deployments.txt to the
// four-field shape written before the image and pull policy were recorded, so
// a test can exercise records left by an earlier release.
func SeedLegacyDeploymentRecords(tc *TestCase, teamName string) {
	tc.t.Helper()
	path := filepath.Join(tc.StateDir, teamName, "deployments.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		tc.t.Fatalf("reading deployment records for team %q: %v", teamName, err)
	}
	var legacy []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) > 4 {
			fields = fields[:4]
		}
		legacy = append(legacy, strings.Join(fields, " "))
	}
	if len(legacy) == 0 {
		tc.t.Fatalf("no deployment records to rewrite for team %q", teamName)
	}
	if err := os.WriteFile(path, []byte(strings.Join(legacy, "\n")+"\n"), 0600); err != nil {
		tc.t.Fatalf("writing legacy deployment records for team %q: %v", teamName, err)
	}
}

// SeedDeploymentRecord writes one deployments.txt line for a team that has
// nothing deployed, so a query can be exercised without a container runtime.
func SeedDeploymentRecord(tc *TestCase, teamName, line string) {
	tc.t.Helper()
	dir := filepath.Join(tc.StateDir, teamName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tc.t.Fatalf("creating state directory for team %q: %v", teamName, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deployments.txt"), []byte(line+"\n"), 0600); err != nil {
		tc.t.Fatalf("writing deployment record for team %q: %v", teamName, err)
	}
}
