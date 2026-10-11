package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/manifest"
	"github.com/CarlosHPlata/shrine/internal/state"
)

const runningWhoami = "docker.io/traefik/whoami:latest"

func decodeEntries(t *testing.T, doc []byte) []map[string]any {
	t.Helper()
	var listing jsonListing[map[string]any]
	if err := json.Unmarshal(doc, &listing); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, doc)
	}
	return listing.Data
}

func singleEntry(t *testing.T, deployments []teamedDeployment, pins map[string]state.ImagePin, backend engine.ContainerBackend) map[string]any {
	t.Helper()
	doc, err := formatDeploymentsJSON(deployments, pins, backend)
	if err != nil {
		t.Fatalf("formatDeploymentsJSON: %v", err)
	}
	entries := decodeEntries(t, doc)
	if len(entries) != 1 {
		t.Fatalf("expected one entry, got %d:\n%s", len(entries), doc)
	}
	return entries[0]
}

func labRecord(d state.Deployment) []teamedDeployment {
	return []teamedDeployment{{Team: "lab", Deployment: d}}
}

func TestFormatDeploymentsJSON_KeysInOrder(t *testing.T) {
	doc, err := formatDeploymentsJSON(labRecord(pinnedRecord("Application")), apiPins("Application"), nil)
	if err != nil {
		t.Fatalf("formatDeploymentsJSON: %v", err)
	}

	assertInOrder(t, string(doc),
		`"team"`, `"name"`, `"kind"`, `"image"`, `"pullPolicy"`, `"version"`,
		`"pin"`, `"runningImage"`, `"containerId"`, `"configHash"`)
}

func TestFormatDeploymentsJSON_PinnedRecordCarriesThePin(t *testing.T) {
	pins := apiPins("Application")
	pin := pins[state.ImagePinKey("lab", "api")]
	pin.PinnedAt = time.Date(2026, 10, 11, 14, 3, 22, 123_000_000, time.UTC)
	pins[state.ImagePinKey("lab", "api")] = pin

	entry := singleEntry(t, labRecord(pinnedRecord("Application")), pins, nil)

	if entry["version"] != "latest@3f2a9c1b4d7e" {
		t.Errorf("version = %#v, want the readable form", entry["version"])
	}
	got, ok := entry["pin"].(map[string]any)
	if !ok {
		t.Fatalf("pin = %#v, want an object", entry["pin"])
	}
	want := map[string]any{
		"reference": "ghcr.io/me/api@" + fullDigest,
		"version":   "latest@3f2a9c1b4d7e",
		"pinnedAt":  "2026-10-11T14:03:22Z",
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("pin.%s = %#v, want %#v", key, got[key], value)
		}
	}
	if len(got) != len(want) {
		t.Errorf("pin = %v, want exactly %v", got, want)
	}
}

func TestFormatDeploymentsJSON_PinIsNullWhenNotPinned(t *testing.T) {
	owned := pinnedRecord("Application")
	owned.Policy = "Always"
	cases := []struct {
		name string
		d    state.Deployment
		pins map[string]state.ImagePin
	}{
		{"manifest-owned record with a pin on record", owned, apiPins("Application")},
		{"pinned record without a pin", pinnedRecord("Application"), map[string]state.ImagePin{}},
		{"pinned record with a pin of the other kind", pinnedRecord("Resource"), apiPins("Application")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry := singleEntry(t, labRecord(tc.d), tc.pins, nil)

			if value, present := entry["pin"]; !present || value != nil {
				t.Errorf("pin = %#v (present %v), want null", value, present)
			}
			if entry["version"] != tc.d.Image {
				t.Errorf("version = %#v, want the recorded reference %q", entry["version"], tc.d.Image)
			}
		})
	}
}

func TestFormatDeploymentsJSON_UnknownValuesAreEmptyStrings(t *testing.T) {
	legacy := state.Deployment{Kind: "Application", Name: "api"}

	entry := singleEntry(t, labRecord(legacy), map[string]state.ImagePin{}, nil)

	for _, key := range []string{"image", "pullPolicy", "version", "containerId", "configHash"} {
		if entry[key] != "" {
			t.Errorf("%s = %#v, want \"\" (never the table's -)", key, entry[key])
		}
	}
}

func TestFormatDeploymentsJSON_FullContainerIDAndConfigHash(t *testing.T) {
	record := pinnedRecord("Application")
	record.ContainerID = strings.Repeat("a1", 32)
	record.ConfigHash = strings.Repeat("b2", 32)

	entry := singleEntry(t, labRecord(record), map[string]state.ImagePin{}, nil)

	if entry["containerId"] != record.ContainerID {
		t.Errorf("containerId = %#v, want the full %q", entry["containerId"], record.ContainerID)
	}
	if entry["configHash"] != record.ConfigHash {
		t.Errorf("configHash = %#v, want the full %q", entry["configHash"], record.ConfigHash)
	}
}

func TestFormatDeploymentsJSON_RunningImage(t *testing.T) {
	withoutContainer := pinnedRecord("Application")
	withoutContainer.ContainerID = ""
	cases := []struct {
		name    string
		d       state.Deployment
		backend engine.ContainerBackend
		want    any
	}{
		{"no backend", pinnedRecord("Application"), nil, nil},
		{"inspection fails", pinnedRecord("Application"), imageBackend{err: errors.New("no such container")}, nil},
		{"no container recorded", withoutContainer, imageBackend{info: engine.ContainerInfo{Image: runningWhoami}}, nil},
		{"image known", pinnedRecord("Application"), imageBackend{info: engine.ContainerInfo{Image: runningWhoami}}, runningWhoami},
		{"image unknown", pinnedRecord("Application"), imageBackend{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entry := singleEntry(t, labRecord(tc.d), map[string]state.ImagePin{}, tc.backend)

			value, present := entry["runningImage"]
			if !present || value != tc.want {
				t.Errorf("runningImage = %#v (present %v), want %#v", value, present, tc.want)
			}
		})
	}
}

func TestFormatDeploymentsJSON_EmptyListing(t *testing.T) {
	doc, err := formatDeploymentsJSON(nil, map[string]state.ImagePin{}, nil)
	if err != nil {
		t.Fatalf("formatDeploymentsJSON: %v", err)
	}
	if string(doc) != emptyListing {
		t.Errorf("formatDeploymentsJSON = %q, want %q", doc, emptyListing)
	}
}

func TestFormatDeploymentsJSON_KeepsListingOrder(t *testing.T) {
	deployments := []teamedDeployment{
		{Team: "media", Deployment: state.Deployment{Kind: "Application", Name: "jellyfin"}},
		{Team: "lab", Deployment: state.Deployment{Kind: "Resource", Name: "db"}},
		{Team: "lab", Deployment: state.Deployment{Kind: "Application", Name: "api"}},
	}

	doc, err := formatDeploymentsJSON(deployments, map[string]state.ImagePin{}, nil)
	if err != nil {
		t.Fatalf("formatDeploymentsJSON: %v", err)
	}

	entries := decodeEntries(t, doc)
	var got []string
	for _, entry := range entries {
		got = append(got, entry["team"].(string)+"/"+entry["name"].(string))
	}
	if want := "media/jellyfin,lab/db,lab/api"; strings.Join(got, ",") != want {
		t.Errorf("order = %v, want %s", got, want)
	}
	if !bytes.HasSuffix(doc, []byte("}\n")) {
		t.Errorf("expected one trailing newline:\n%q", doc)
	}
}

func TestListDeploymentsFiltersKind(t *testing.T) {
	store := deleteTestStore([]string{"demo"}, state.HostPortMap{}, map[string][]state.Deployment{
		"demo": {
			{Kind: manifest.ApplicationKind, Name: "api"},
			{Kind: manifest.ResourceKind, Name: "db"},
		},
	})
	cases := []struct {
		team, kind string
		want       string
	}{
		{"", manifest.ApplicationKind, "api"},
		{"demo", manifest.ResourceKind, "db"},
		{"", "", "api,db"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			deployments, err := listDeployments(tc.team, tc.kind, store)
			if err != nil {
				t.Fatalf("listDeployments: %v", err)
			}
			var names []string
			for _, td := range deployments {
				names = append(names, td.Deployment.Name)
			}
			if strings.Join(names, ",") != tc.want {
				t.Errorf("listDeployments(%q, %q) = %v, want %s", tc.team, tc.kind, names, tc.want)
			}
		})
	}
}
