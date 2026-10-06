//go:build integration

package testutils

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func (tc *TestCase) AssertTeamNotInState(name string) *TestCase {
	tc.t.Helper()
	path := filepath.Join(tc.StateDir, "teams", strings.ToLower(name)+".json")
	if _, err := os.Stat(path); err == nil {
		tc.t.Errorf("expected team %q to not be in state, but file exists at %s", name, path)
	}
	return tc
}

func (tc *TestCase) AssertTeamInState(name string) *TestCase {
	tc.t.Helper()
	path := filepath.Join(tc.StateDir, "teams", strings.ToLower(name)+".json")
	if _, err := os.Stat(path); err != nil {
		tc.t.Errorf("expected team %q in state: %v", name, err)
	}
	return tc
}

func (tc *TestCase) AssertTeamCount(n int) *TestCase {
	tc.t.Helper()
	entries, err := os.ReadDir(filepath.Join(tc.StateDir, "teams"))
	count := 0
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
				count++
			}
		}
	}
	if count != n {
		tc.t.Errorf("expected %d team(s) in state, got %d", n, count)
	}
	return tc
}

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

func (tc *TestCase) readSecretsFile(teamName string) map[string]string {
	path := filepath.Join(tc.StateDir, teamName, "secrets.env")
	f, err := os.Open(path)
	if err != nil {
		return map[string]string{}
	}
	defer f.Close()
	result := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

func (tc *TestCase) AssertSecretInState(teamName, resourceName, outputName string) *TestCase {
	tc.t.Helper()
	secrets := tc.readSecretsFile(teamName)
	key := resourceName + "." + outputName
	v, ok := secrets[key]
	if !ok || v == "" {
		tc.t.Errorf("expected non-empty secret %q for team %q", key, teamName)
	}
	return tc
}

func (tc *TestCase) AssertSecretValueInState(teamName, resourceName, outputName, expected string) *TestCase {
	tc.t.Helper()
	secrets := tc.readSecretsFile(teamName)
	key := resourceName + "." + outputName
	v, ok := secrets[key]
	if !ok || v == "" {
		tc.t.Errorf("expected non-empty secret %q for team %q", key, teamName)
		return tc
	}
	if v != expected {
		tc.t.Errorf("secret %q for team %q = %q, want %q", key, teamName, v, expected)
	}
	return tc
}

func (tc *TestCase) SecretFromState(teamName, resourceName, outputName string) string {
	tc.t.Helper()
	secrets := tc.readSecretsFile(teamName)
	key := resourceName + "." + outputName
	v, ok := secrets[key]
	if !ok || v == "" {
		tc.t.Fatalf("secret %q for team %q is missing or empty", key, teamName)
	}
	return v
}
