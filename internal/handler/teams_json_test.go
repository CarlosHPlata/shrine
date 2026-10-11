package handler

import (
	"bytes"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/manifest"
)

func teamManifest(name, displayName string, maxApps, maxResources int) *manifest.TeamManifest {
	team := &manifest.TeamManifest{}
	team.Metadata.Name = name
	team.Spec.DisplayName = displayName
	team.Spec.Quotas.MaxApps = maxApps
	team.Spec.Quotas.MaxResources = maxResources
	return team
}

func TestFormatTeamsJSON_FieldsAndNumbers(t *testing.T) {
	doc, err := formatTeamsJSON([]*manifest.TeamManifest{
		teamManifest("media", "Media & Co", 5, 3),
		teamManifest("lab", "", 0, 0),
	})
	if err != nil {
		t.Fatalf("formatTeamsJSON: %v", err)
	}

	entries := decodeEntries(t, doc)
	want := []map[string]any{
		{"name": "media", "displayName": "Media & Co", "maxApps": float64(5), "maxResources": float64(3)},
		{"name": "lab", "displayName": "", "maxApps": float64(0), "maxResources": float64(0)},
	}
	if len(entries) != len(want) {
		t.Fatalf("expected %d entries, got %d:\n%s", len(want), len(entries), doc)
	}
	for i, entry := range entries {
		if len(entry) != len(want[i]) {
			t.Errorf("entry %d = %v, want exactly %v", i, entry, want[i])
		}
		for key, value := range want[i] {
			if entry[key] != value {
				t.Errorf("entry %d %s = %#v, want %#v", i, key, entry[key], value)
			}
		}
	}
	if !bytes.Contains(doc, []byte("Media & Co")) {
		t.Errorf("expected the display name written literally:\n%s", doc)
	}
}

func TestFormatTeamsJSON_EmptyListing(t *testing.T) {
	doc, err := formatTeamsJSON(nil)
	if err != nil {
		t.Fatalf("formatTeamsJSON: %v", err)
	}
	if string(doc) != emptyListing {
		t.Errorf("formatTeamsJSON = %q, want %q", doc, emptyListing)
	}
}
