//go:build integration

package testutils

import (
	"os"
	"path/filepath"
	"strings"
)

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
