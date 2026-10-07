package handler

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CarlosHPlata/shrine/internal/engine"

	"github.com/CarlosHPlata/shrine/internal/state"
)

// assertInOrder checks that line contains every part, left to right, each one
// found after the previous match.
func assertInOrder(t *testing.T, line string, parts ...string) {
	t.Helper()
	rest := line
	for _, part := range parts {
		idx := strings.Index(rest, part)
		if idx < 0 {
			t.Fatalf("expected %q in order %v\nline: %q", part, parts, line)
		}
		rest = rest[idx+len(part):]
	}
}

func tableLines(t *testing.T, out string) (header, separator string, rows []string) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected a header and a separator, got:\n%s", out)
	}
	return lines[0], lines[1], lines[2:]
}

func TestFormatDeploymentsTable_AddsVersionAfterKind(t *testing.T) {
	out := formatDeploymentsTable([]teamedDeployment{{
		Team: "lab",
		Deployment: state.Deployment{
			Kind:        "Application",
			Name:        "api",
			ContainerID: "9f1c0e2d4b6a8c7e5f3a",
			ConfigHash:  "3a7b",
			Image:       "reg:lab/hello-api:1.2.0",
			Policy:      "Always",
		},
	}}, nil)

	header, separator, rows := tableLines(t, out)
	assertInOrder(t, header, "TEAM", "NAME", "KIND", "VERSION", "CONTAINER ID")
	if len(separator) != len(header) {
		t.Errorf("separator length %d, want the header length %d", len(separator), len(header))
	}
	if strings.Trim(separator, "-") != "" {
		t.Errorf("separator should be dashes only, got %q", separator)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d:\n%s", len(rows), out)
	}
	assertInOrder(t, rows[0], "lab", "api", "Application", "reg:lab/hello-api:1.2.0", "9f1c0e2d4b6a")
	if strings.Contains(rows[0], "9f1c0e2d4b6a8c7e5f3a") {
		t.Errorf("container id should stay shortened to twelve characters: %q", rows[0])
	}
}

func TestFormatDeploymentsTable_ShowsDashForUnknownVersion(t *testing.T) {
	out := formatDeploymentsTable([]teamedDeployment{
		{Team: "lab", Deployment: state.Deployment{Kind: "Resource", Name: "db", ContainerID: "5d0a11"}},
		{Team: "lab", Deployment: state.Deployment{Kind: "Application", Name: "api", ContainerID: "9f1ce2", Image: "nginx:1.27"}},
	}, nil)

	_, _, rows := tableLines(t, out)
	if len(rows) != 2 {
		t.Fatalf("expected two rows, got %d:\n%s", len(rows), out)
	}
	assertInOrder(t, rows[0], "lab", "db", "Resource", "-", "5d0a11")
	assertInOrder(t, rows[1], "lab", "api", "Application", "nginx:1.27", "9f1ce2")
	if strings.Contains(rows[1], " - ") {
		t.Errorf("a known version must not print the placeholder: %q", rows[1])
	}
}

func TestFormatDeploymentDetail_PrintsImageAndPolicyAfterKind(t *testing.T) {
	out := formatDeploymentDetail(deploymentDetail{Team: "lab", Deployment: state.Deployment{
		Kind:        "Application",
		Name:        "api",
		ContainerID: "9f1c0e2d4b6a8c7e5f3a",
		ConfigHash:  "0123456789abcdef0123",
		Image:       "reg:lab/hello-api:1.2.0",
		Policy:      "IfNotPresent",
	}, RunningImage: "-"})

	want := "Name:         api\n" +
		"Team:         lab\n" +
		"Kind:         Application\n" +
		"Image:        reg:lab/hello-api:1.2.0\n" +
		"Pull policy:  IfNotPresent\n" +
		"Running image: -\n" +
		"Container ID: 9f1c0e2d4b6a8c7e5f3a\n" +
		"Config Hash:  0123456789abcdef...\n"
	if out != want {
		t.Errorf("describe output:\ngot:\n%s\nwant:\n%s", out, want)
	}
}

func TestFormatDeploymentDetail_ShowsDashForUnknownImageAndPolicy(t *testing.T) {
	out := formatDeploymentDetail(deploymentDetail{Team: "lab", Deployment: state.Deployment{
		Kind:        "Resource",
		Name:        "db",
		ContainerID: "5d0a11",
		ConfigHash:  "b2e477",
	}, RunningImage: "-"})

	for _, line := range []string{"Image:        -\n", "Pull policy:  -\n", "Config Hash:  b2e477\n"} {
		if !strings.Contains(out, line) {
			t.Errorf("expected %q in:\n%s", line, out)
		}
	}
}

const fullDigest = "sha256:3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a"

func pinnedRecord(kind string) state.Deployment {
	return state.Deployment{
		Kind:        kind,
		Name:        "api",
		ContainerID: "9f1c0e2d4b6a8c7e5f3a",
		ConfigHash:  "3a7b",
		Image:       "ghcr.io/me/api",
		Policy:      "Pinned",
	}
}

func apiPins(kind string) map[string]state.ImagePin {
	return map[string]state.ImagePin{
		state.ImagePinKey("lab", "api"): {
			Kind:      kind,
			Name:      "api",
			Requested: "ghcr.io/me/api:latest",
			Pinned:    "ghcr.io/me/api@" + fullDigest,
		},
	}
}

func singleRow(t *testing.T, out string) string {
	t.Helper()
	_, _, rows := tableLines(t, out)
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d:\n%s", len(rows), out)
	}
	return rows[0]
}

func TestFormatDeploymentsTable_PinnedRowShowsTheReadableForm(t *testing.T) {
	row := singleRow(t, formatDeploymentsTable(
		[]teamedDeployment{{Team: "lab", Deployment: pinnedRecord("Application")}},
		apiPins("Application")))

	assertInOrder(t, row, "lab", "api", "Application", "latest@3f2a9c1b4d7e", "9f1c0e2d4b6a")
	if strings.Contains(row, "ghcr.io/me/api") || strings.Contains(row, fullDigest) {
		t.Errorf("a pinned row shows the readable form only: %q", row)
	}
}

func TestFormatDeploymentsTable_PinnedRowWithoutAPinShowsTheRecordedReference(t *testing.T) {
	row := singleRow(t, formatDeploymentsTable(
		[]teamedDeployment{{Team: "lab", Deployment: pinnedRecord("Application")}},
		map[string]state.ImagePin{}))

	assertInOrder(t, row, "lab", "api", "Application", "ghcr.io/me/api", "9f1c0e2d4b6a")
}

func TestFormatDeploymentsTable_ManifestOwnedRowIgnoresAPin(t *testing.T) {
	record := pinnedRecord("Application")
	record.Policy = "Always"
	row := singleRow(t, formatDeploymentsTable(
		[]teamedDeployment{{Team: "lab", Deployment: record}},
		apiPins("Application")))

	assertInOrder(t, row, "lab", "api", "Application", "ghcr.io/me/api", "9f1c0e2d4b6a")
	if strings.Contains(row, "latest@") {
		t.Errorf("a manifest-owned row never shows a pin: %q", row)
	}
}

func TestFormatDeploymentsTable_PinOfTheOtherKindIsIgnored(t *testing.T) {
	row := singleRow(t, formatDeploymentsTable(
		[]teamedDeployment{{Team: "lab", Deployment: pinnedRecord("Resource")}},
		apiPins("Application")))

	assertInOrder(t, row, "lab", "api", "Resource", "ghcr.io/me/api", "9f1c0e2d4b6a")
}

func TestVersionCell(t *testing.T) {
	owned := pinnedRecord("Application")
	owned.Policy = "Always"
	cases := []struct {
		name string
		d    state.Deployment
		pins map[string]state.ImagePin
		want string
	}{
		{"pinned with a pin", pinnedRecord("Application"), apiPins("Application"), "latest@3f2a9c1b4d7e"},
		{"pinned without a pin", pinnedRecord("Application"), nil, "ghcr.io/me/api"},
		{"manifest-owned ignores a pin", owned, apiPins("Application"), "ghcr.io/me/api"},
		{"pin of the other kind", pinnedRecord("Resource"), apiPins("Application"), "ghcr.io/me/api"},
		{"legacy record", state.Deployment{Kind: "Application", Name: "api"}, nil, "-"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := versionCell("lab", tc.d, tc.pins); got != tc.want {
				t.Errorf("versionCell = %q, want %q", got, tc.want)
			}
		})
	}
}

func pinnedDetail() deploymentDetail {
	return deploymentDetail{
		Team:       "lab",
		Deployment: pinnedRecord("Application"),
		Pin: state.ImagePin{
			Kind:      "Application",
			Name:      "api",
			Requested: "ghcr.io/me/api:latest",
			Pinned:    "ghcr.io/me/api@" + fullDigest,
			PinnedAt:  time.Date(2026, 10, 7, 10, 42, 17, 0, time.UTC),
		},
		HasPin:       true,
		RunningImage: "ghcr.io/me/api@" + fullDigest,
	}
}

func TestFormatDeploymentDetail_PinnedRecordShowsThePinAndTheRunningImage(t *testing.T) {
	out := formatDeploymentDetail(pinnedDetail())

	for _, line := range []string{
		"Pull policy:  Pinned\n",
		"Pinned:       ghcr.io/me/api@" + fullDigest + " (latest@3f2a9c1b4d7e, 2026-10-07)\n",
		"Running image: ghcr.io/me/api@" + fullDigest + "\n",
	} {
		if !strings.Contains(out, line) {
			t.Errorf("expected %q in:\n%s", line, out)
		}
	}
	assertInOrder(t, out, "Pull policy:", "Pinned:", "Running image:", "Container ID:")
}

func TestFormatDeploymentDetail_PinnedRecordWithoutAPinShowsADash(t *testing.T) {
	d := pinnedDetail()
	d.HasPin = false
	d.Pin = state.ImagePin{}

	if out := formatDeploymentDetail(d); !strings.Contains(out, "Pinned:       -\n") {
		t.Errorf("expected a dash on the Pinned: line in:\n%s", out)
	}
}

func TestFormatDeploymentDetail_ManifestOwnedRecordHasNoPinnedLine(t *testing.T) {
	d := pinnedDetail()
	d.Deployment.Policy = "Always"
	d.RunningImage = "docker.io/traefik/whoami:latest"

	out := formatDeploymentDetail(d)
	if strings.Contains(out, "Pinned:") {
		t.Errorf("a manifest-owned record has no Pinned: line:\n%s", out)
	}
	if !strings.Contains(out, "Running image: docker.io/traefik/whoami:latest\n") {
		t.Errorf("expected the running image in:\n%s", out)
	}
}

func TestFormatDeploymentDetail_LegacyRecordHasNoPinnedLine(t *testing.T) {
	out := formatDeploymentDetail(deploymentDetail{
		Team:         "lab",
		Deployment:   state.Deployment{Kind: "Application", Name: "api", ContainerID: "9f1c"},
		RunningImage: "unavailable (no such container)",
	})

	for _, line := range []string{"Image:        -\n", "Pull policy:  -\n", "Running image: unavailable (no such container)\n"} {
		if !strings.Contains(out, line) {
			t.Errorf("expected %q in:\n%s", line, out)
		}
	}
	if strings.Contains(out, "Pinned:") {
		t.Errorf("a legacy record has no Pinned: line:\n%s", out)
	}
}

type imageBackend struct {
	engine.ContainerBackend
	info engine.ContainerInfo
	err  error
}

func (b imageBackend) InspectContainer(string) (engine.ContainerInfo, error) { return b.info, b.err }

func TestRunningImage(t *testing.T) {
	cases := []struct {
		name    string
		backend engine.ContainerBackend
		want    string
	}{
		{"no backend", nil, "unavailable (no container runtime)"},
		{"inspection fails", imageBackend{err: errors.New("no such container")}, "unavailable (no such container)"},
		{"image unknown", imageBackend{}, "-"},
		{"image known", imageBackend{info: engine.ContainerInfo{Image: "docker.io/traefik/whoami:latest"}}, "docker.io/traefik/whoami:latest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runningImage(tc.backend, "9f1c"); got != tc.want {
				t.Errorf("runningImage = %q, want %q", got, tc.want)
			}
		})
	}
}
