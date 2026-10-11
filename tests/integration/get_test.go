//go:build integration

package integration_test

import (
	"encoding/json"
	"strings"
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

// assertColumnsInOrder checks that the header line of the last run's stdout
// names the columns left to right in the given order.
func assertColumnsInOrder(tc *TestCase, columns ...string) {
	header, _, _ := strings.Cut(tc.RunResult().Stdout, "\n")
	rest := header
	for _, column := range columns {
		idx := strings.Index(rest, column)
		if idx < 0 {
			tc.Fatalf("expected column %q in order %v\nheader: %s", column, columns, header)
		}
		rest = rest[idx+len(column):]
	}
}

var workloadEntryKeys = []string{
	"team", "name", "kind", "image", "pullPolicy", "version",
	"pin", "runningImage", "containerId", "configHash",
}

// decodeListing parses the last run's stdout as a --json listing; a document
// whose data is null fails, because scripts rely on it always being an array.
func decodeListing(tc *TestCase) []map[string]any {
	var doc struct {
		Data []map[string]any `json:"data"`
	}
	stdout := tc.RunResult().Stdout
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		tc.Fatalf("stdout is not a JSON document: %v\nstdout: %s", err, stdout)
	}
	if doc.Data == nil {
		tc.Fatalf("expected a data array, got none or null\nstdout: %s", stdout)
	}
	return doc.Data
}

func entryNamed(tc *TestCase, entries []map[string]any, name string) map[string]any {
	for _, entry := range entries {
		if entry["name"] == name {
			return entry
		}
	}
	tc.Fatalf("no entry named %q in %v", name, namesOf(entries))
	return nil
}

func namesOf(entries []map[string]any) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name, _ := entry["name"].(string)
		names = append(names, name)
	}
	return names
}

// tableNames reads the NAME column of every row below the table's separator.
func tableNames(tc *TestCase) []string {
	var names []string
	pastSeparator := false
	for _, line := range strings.Split(tc.RunResult().Stdout, "\n") {
		if strings.HasPrefix(line, "---") {
			pastSeparator = true
			continue
		}
		fields := strings.Fields(line)
		if pastSeparator && len(fields) >= 2 {
			names = append(names, fields[1])
		}
	}
	return names
}

func stringField(tc *TestCase, entry map[string]any, key string) string {
	value, ok := entry[key].(string)
	if !ok {
		tc.Fatalf("expected %q to be a string, got %#v in %v", key, entry[key], entry)
	}
	return value
}

func assertField(tc *TestCase, entry map[string]any, key, want string) {
	if got := stringField(tc, entry, key); got != want {
		tc.Fatalf("expected %s %q, got %q in %v", key, want, got, entry)
	}
}

func assertNullField(tc *TestCase, entry map[string]any, key string) {
	value, present := entry[key]
	if !present || value != nil {
		tc.Fatalf("expected %q to be present and null, got %#v in %v", key, value, entry)
	}
}

func assertExactKeys(tc *TestCase, entry map[string]any, keys []string) {
	if len(entry) != len(keys) {
		tc.Fatalf("expected exactly the keys %v, got %v", keys, entry)
	}
	for _, key := range keys {
		if _, present := entry[key]; !present {
			tc.Fatalf("expected key %q in %v", key, entry)
		}
	}
}

func assertSameNames(tc *TestCase, got, want []string) {
	if strings.Join(got, ",") != strings.Join(want, ",") {
		tc.Fatalf("expected names %v in order, got %v", want, got)
	}
}

func TestGetNoDocker(t *testing.T) {
	s := NewSuite(t)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		// Use the dedicated team/ sub-fixture to avoid scanning bad-kind/ and
		// malformed-yaml/ siblings that cause ScanDir to error.
		tc.Run("apply", "teams",
			"--path", fixturesPath("team"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("should list registered teams", func(tc *TestCase) {
		tc.Run("get", "teams", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains(testTeam)
	})

	// US3, FR-004: quotas are JSON numbers, and team entries carry only the
	// table's four fields.
	s.Test("get teams --json lists each team with numeric quotas", func(tc *TestCase) {
		tc.Run("get", "teams", "--json", "--state-dir", tc.StateDir).AssertSuccess()

		team := entryNamed(tc, decodeListing(tc), testTeam)
		assertExactKeys(tc, team, []string{"name", "displayName", "maxApps", "maxResources"})
		assertField(tc, team, "displayName", "Shrine Test")
		if team["maxApps"] != float64(10) || team["maxResources"] != float64(10) {
			tc.Fatalf("expected numeric quotas 10 and 10, got %#v and %#v", team["maxApps"], team["maxResources"])
		}
	})

	// US1, US2, FR-008: an empty listing is an empty array, never a sentence.
	s.Test("get --json with nothing deployed prints an empty data array", func(tc *TestCase) {
		listings := map[string]string{
			"deployed":  "No deployments found",
			"apps":      "No applications deployed",
			"resources": "No resources deployed",
		}
		for command, sentence := range listings {
			tc.Run("get", command, "--json", "--state-dir", tc.StateDir).
				AssertSuccess().
				AssertOutputNotContains(sentence)
			if entries := decodeListing(tc); len(entries) != 0 {
				tc.Fatalf("get %s --json: expected no entries, got %v", command, entries)
			}
		}
	})

	// US1, FR-003a, FR-007, SC-002a: without a reachable runtime the listing
	// still succeeds, with a null running image and "" where the table has "-".
	s.Test("get deployed --json without a container runtime still succeeds with a null running image", func(tc *TestCase) {
		SeedDeploymentRecord(tc, testTeam, "Application whoami 0123456789abcdef 3a7b - -")
		tc.Setenv("DOCKER_HOST", "tcp://127.0.0.1:1")

		tc.Run("get", "deployed", "--json", "--state-dir", tc.StateDir).AssertSuccess()
		if stderr := tc.RunResult().Stderr; stderr != "" {
			tc.Fatalf("expected empty stderr, got: %s", stderr)
		}

		entry := entryNamed(tc, decodeListing(tc), "whoami")
		assertExactKeys(tc, entry, workloadEntryKeys)
		assertField(tc, entry, "team", testTeam)
		assertField(tc, entry, "kind", "Application")
		assertField(tc, entry, "image", "")
		assertField(tc, entry, "pullPolicy", "")
		assertField(tc, entry, "version", "")
		assertNullField(tc, entry, "pin")
		assertNullField(tc, entry, "runningImage")
		assertField(tc, entry, "containerId", "0123456789abcdef")
		assertField(tc, entry, "configHash", "3a7b")
	})

	// FR-010: only the four listings accept --json.
	s.Test("get config-dir and get state-dir reject --json", func(tc *TestCase) {
		for _, command := range []string{"config-dir", "state-dir"} {
			tc.Run("get", command, "--json", "--state-dir", tc.StateDir).
				AssertFailure().
				AssertStderrContains("unknown flag: --json")
		}
	})

	// FR-011: without --json the output is still the table.
	s.Test("get deployed without --json is still the table", func(tc *TestCase) {
		SeedDeploymentRecord(tc, testTeam, "Application whoami 0123456789abcdef 3a7b - -")

		tc.Run("get", "deployed", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami", "0123456789ab").
			AssertOutputNotContains(`"data"`)
		assertColumnsInOrder(tc, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")
	})
}

func TestGetDocker(t *testing.T) {
	s := NewDockerSuite(t, testTeam)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		// Use the dedicated team/ sub-fixture to avoid scanning bad-kind/ and
		// malformed-yaml/ siblings that cause ScanDir to error.
		tc.Run("apply", "teams",
			"--path", fixturesPath("team"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
		tc.Run("deploy",
			"--path", fixturesPath("resources"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("should list deployed apps", func(tc *TestCase) {
		tc.Run("get", "apps", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("whoami-res")
	})

	s.Test("should list deployed resources", func(tc *TestCase) {
		tc.Run("get", "resources", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("test-cache")
	})

	s.Test("should list all deployed items", func(tc *TestCase) {
		tc.Run("get", "deployed", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputContains("whoami-res").
			AssertOutputContains("test-cache")
	})

	s.Test("should show the manifest image reference in a VERSION column after KIND", func(tc *TestCase) {
		tc.Run("get", "deployed", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-res", "traefik/whoami").
			AssertOutputLineContains("test-cache", "traefik/whoami")
		assertColumnsInOrder(tc, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")
	})

	s.Test("should show the VERSION column when filtered by team", func(tc *TestCase) {
		tc.Run("get", "deployed", "--team", testTeam, "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-res", "traefik/whoami").
			AssertOutputLineContains("test-cache", "traefik/whoami")
		assertColumnsInOrder(tc, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")
	})

	s.Test("should show the VERSION column for applications and for resources", func(tc *TestCase) {
		tc.Run("get", "apps", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-res", "traefik/whoami")
		assertColumnsInOrder(tc, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")

		tc.Run("get", "resources", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("test-cache", "traefik/whoami")
		assertColumnsInOrder(tc, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")
	})

	s.Test("should list records from a previous release with an unknown version and fill it on the next deploy", func(tc *TestCase) {
		SeedLegacyDeploymentRecords(tc, testTeam)

		tc.Run("get", "deployed", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-res", " - ").
			AssertOutputLineContains("test-cache", " - ").
			AssertOutputNotContains("traefik/whoami")

		tc.Run("deploy",
			"--path", fixturesPath("resources"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()

		tc.Run("get", "deployed", "--state-dir", tc.StateDir).
			AssertSuccess().
			AssertOutputLineContains("whoami-res", "traefik/whoami").
			AssertOutputLineContains("test-cache", "traefik/whoami")
	})

	// US1, FR-003, FR-005, SC-002: one entry per table row, in the table's
	// order, with the live running image and the full container ID and hash.
	s.Test("get deployed --json matches the table row for row", func(tc *TestCase) {
		tc.Run("get", "deployed", "--state-dir", tc.StateDir).AssertSuccess()
		want := tableNames(tc)

		tc.Run("get", "deployed", "--json", "--state-dir", tc.StateDir).AssertSuccess()
		entries := decodeListing(tc)
		assertSameNames(tc, namesOf(entries), want)
		for _, entry := range entries {
			assertExactKeys(tc, entry, workloadEntryKeys)
			assertNullField(tc, entry, "pin")
			image := stringField(tc, entry, "image")
			if !strings.Contains(image, "traefik/whoami") {
				tc.Fatalf("expected image to name traefik/whoami, got %q", image)
			}
			assertField(tc, entry, "version", image)
			if running := stringField(tc, entry, "runningImage"); !strings.Contains(running, "traefik/whoami") {
				tc.Fatalf("expected a live running image naming traefik/whoami, got %q", running)
			}
			if id := stringField(tc, entry, "containerId"); len(id) != 64 {
				tc.Fatalf("expected the full 64-character container ID, got %q", id)
			}
			if hash := stringField(tc, entry, "configHash"); hash == "" || strings.HasSuffix(hash, "...") {
				tc.Fatalf("expected the full config hash, got %q", hash)
			}
		}
	})

	// US1, FR-006: --team filters the JSON exactly as it filters the table.
	s.Test("get deployed --json honours --team", func(tc *TestCase) {
		tc.Run("get", "deployed", "--json", "--team", testTeam, "--state-dir", tc.StateDir).AssertSuccess()
		entries := decodeListing(tc)
		entryNamed(tc, entries, "whoami-res")
		entryNamed(tc, entries, "test-cache")

		tc.Run("get", "deployed", "--json", "--team", "nobody", "--state-dir", tc.StateDir).AssertSuccess()
		if entries := decodeListing(tc); len(entries) != 0 {
			tc.Fatalf("expected no entries for an unknown team, got %v", entries)
		}
	})

	// US2: each kind-filtered listing holds only its kind, under every alias.
	s.Test("get apps and get resources --json list only their kind", func(tc *TestCase) {
		listings := []struct {
			args []string
			kind string
			name string
		}{
			{[]string{"apps"}, "Application", "whoami-res"},
			{[]string{"applications", "--team", testTeam}, "Application", "whoami-res"},
			{[]string{"resources"}, "Resource", "test-cache"},
			{[]string{"res"}, "Resource", "test-cache"},
		}
		for _, listing := range listings {
			tableArgs := append(append([]string{"get"}, listing.args...), "--state-dir", tc.StateDir)
			tc.Run(tableArgs...).AssertSuccess()
			want := tableNames(tc)

			tc.Run(append(tableArgs, "--json")...).AssertSuccess()
			entries := decodeListing(tc)
			assertSameNames(tc, namesOf(entries), want)
			assertSameNames(tc, namesOf(entries), []string{listing.name})
			for _, entry := range entries {
				assertField(tc, entry, "kind", listing.kind)
			}
		}
	})
}
