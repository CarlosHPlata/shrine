package updater

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		// Baseline: stable vs stable.
		{"stable patch bump is newer", "v0.0.1", "v0.0.2", true},
		{"stable patch downgrade is not newer", "v0.0.2", "v0.0.1", false},
		{"identical stable is not newer", "v0.0.1", "v0.0.1", false},
		{"minor bump beats large patch", "v0.0.99", "v0.1.0", true},
		{"major bump beats large minor", "v0.9.9", "v1.0.0", true},

		// The reported bug: a pre-release of a higher patch must not be
		// treated as older than a lower stable.
		{"prerelease ahead of stable patch", "v0.0.2-beta.1.5", "v0.0.1", false},
		{"stable lower than current prerelease", "v0.0.2-beta.1.5", "v0.0.2", true},
		{"newer prerelease of same patch", "v0.0.2-beta.1", "v0.0.2-beta.2", true},
		{"older prerelease of same patch", "v0.0.2-beta.2", "v0.0.2-beta.1", false},
		{"alpha < beta on same patch", "v0.0.2-alpha.1", "v0.0.2-beta.1", true},
		{"beta is older than final on same patch", "v0.0.2-beta.1", "v0.0.2", true},
		{"final is not older than its own beta", "v0.0.2", "v0.0.2-beta.1", false},
		{"longer prerelease chain wins when prefix equal", "v0.0.2-beta.1", "v0.0.2-beta.1.5", true},

		// Special cases preserved from the original contract.
		{"dev current is never updated", "dev", "v0.0.2", false},
		{"empty latest yields no update", "v0.0.1", "", false},

		// Unparseable inputs must not falsely trigger an update.
		{"garbage latest yields no update", "v0.0.1", "not-a-version", false},
		{"garbage current yields no update", "not-a-version", "v0.0.1", false},

		// Build metadata is ignored per SemVer 2.0.0.
		{"build metadata does not affect equality", "v0.0.1+abc", "v0.0.1+def", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsNewer(tc.current, tc.latest); got != tc.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}
