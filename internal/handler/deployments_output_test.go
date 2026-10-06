package handler

import (
	"strings"
	"testing"

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
	}})

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
	})

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
	out := formatDeploymentDetail("lab", state.Deployment{
		Kind:        "Application",
		Name:        "api",
		ContainerID: "9f1c0e2d4b6a8c7e5f3a",
		ConfigHash:  "0123456789abcdef0123",
		Image:       "reg:lab/hello-api:1.2.0",
		Policy:      "IfNotPresent",
	})

	want := "Name:         api\n" +
		"Team:         lab\n" +
		"Kind:         Application\n" +
		"Image:        reg:lab/hello-api:1.2.0\n" +
		"Pull policy:  IfNotPresent\n" +
		"Container ID: 9f1c0e2d4b6a8c7e5f3a\n" +
		"Config Hash:  0123456789abcdef...\n"
	if out != want {
		t.Errorf("describe output:\ngot:\n%s\nwant:\n%s", out, want)
	}
}

func TestFormatDeploymentDetail_ShowsDashForUnknownImageAndPolicy(t *testing.T) {
	out := formatDeploymentDetail("lab", state.Deployment{
		Kind:        "Resource",
		Name:        "db",
		ContainerID: "5d0a11",
		ConfigHash:  "b2e477",
	})

	for _, line := range []string{"Image:        -\n", "Pull policy:  -\n", "Config Hash:  b2e477\n"} {
		if !strings.Contains(out, line) {
			t.Errorf("expected %q in:\n%s", line, out)
		}
	}
}
