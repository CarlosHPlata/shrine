package docsgen_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/CarlosHPlata/shrine/docs/tools/docsgen"
)

func TestWriteCommandProducesFrontMatterAndBanner(t *testing.T) {
	cmd := &cobra.Command{Use: "apply", Short: "apply manifests"}

	var buf bytes.Buffer
	if err := docsgen.WriteCommand(cmd, &buf); err != nil {
		t.Fatalf("WriteCommand: %v", err)
	}

	out := buf.String()
	if !strings.HasPrefix(out, "---\n") {
		t.Fatalf("expected output to start with YAML front-matter, got: %q", firstLine(out))
	}
	if !strings.Contains(out, `title: "apply"`) {
		t.Errorf("expected title front-matter for apply, got:\n%s", out)
	}
	if !strings.Contains(out, `description: "apply manifests"`) {
		t.Errorf("expected description front-matter, got:\n%s", out)
	}
	if !strings.Contains(out, "AUTO-GENERATED") {
		t.Errorf("expected AUTO-GENERATED banner in output, got:\n%s", out)
	}
}

func TestWriteCommandIncludesShortInBody(t *testing.T) {
	cmd := &cobra.Command{Use: "deploy", Short: "deploy app"}

	var buf bytes.Buffer
	if err := docsgen.WriteCommand(cmd, &buf); err != nil {
		t.Fatalf("WriteCommand: %v", err)
	}
	if !strings.Contains(buf.String(), "deploy app") {
		t.Errorf("expected Short text in body, got:\n%s", buf.String())
	}
}

func TestWriteCommandEscapesQuotesInDescription(t *testing.T) {
	cmd := &cobra.Command{Use: "noisy", Short: `talks "loudly"`}

	var buf bytes.Buffer
	if err := docsgen.WriteCommand(cmd, &buf); err != nil {
		t.Fatalf("WriteCommand: %v", err)
	}
	if !strings.Contains(buf.String(), `description: "talks \"loudly\""`) {
		t.Errorf("expected escaped quotes in description, got:\n%s", buf.String())
	}
}

func TestShouldSkipHiddenByDefault(t *testing.T) {
	hidden := &cobra.Command{Use: "secret", Hidden: true}
	visible := &cobra.Command{Use: "apply"}

	if !docsgen.ShouldSkipCommand(hidden, false) {
		t.Errorf("expected hidden command to be skipped by default")
	}
	if docsgen.ShouldSkipCommand(visible, false) {
		t.Errorf("expected visible command not to be skipped")
	}
}

func TestShouldSkipHiddenWithIncludeHidden(t *testing.T) {
	hidden := &cobra.Command{Use: "secret", Hidden: true}

	if docsgen.ShouldSkipCommand(hidden, true) {
		t.Errorf("expected hidden command to be included when includeHidden=true")
	}
}

func TestShouldSkipHelpAlways(t *testing.T) {
	help := &cobra.Command{Use: "help"}

	if !docsgen.ShouldSkipCommand(help, false) {
		t.Errorf("expected help to be skipped (default)")
	}
	if !docsgen.ShouldSkipCommand(help, true) {
		t.Errorf("expected help to be skipped even with includeHidden=true")
	}
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
