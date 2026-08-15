package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/engine"
)

// failedCreateEvents is the exact sequence one failed container creation
// produces: the engine's informational progress event, the backend's error
// event (whose only identity field is the full container name), and the
// engine's error re-emit.
func failedCreateEvents() []engine.Event {
	return []engine.Event{
		{Name: "container.create", Status: engine.StatusInfo,
			Fields: map[string]string{"team": "shrine-deploy-test", "name": "alias-app"}},
		{Name: "container.create", Status: engine.StatusError,
			Fields: map[string]string{
				"name":  "shrine-deploy-test.alias-app",
				"error": `creating container "shrine-deploy-test.alias-app" (image "docker.io/traefik/whoami:latest"): invalid reference format`,
			}},
		{Name: "container.create", Status: engine.StatusError,
			Fields: map[string]string{
				"team":  "shrine-deploy-test",
				"name":  "alias-app",
				"error": `application "alias-app": creating container "shrine-deploy-test.alias-app" (image "docker.io/traefik/whoami:latest"): invalid reference format`,
			}},
	}
}

func TestTerminalObserver_ContainerCreate(t *testing.T) {
	t.Run("failed creation prints exactly one progress line and keeps both error lines", func(t *testing.T) {
		var buf bytes.Buffer
		obs := NewTerminalObserver(&buf)

		for _, e := range failedCreateEvents() {
			obs.OnEvent(e)
		}
		out := buf.String()

		if got := strings.Count(out, "Creating container:"); got != 1 {
			t.Errorf("got %d \"Creating container\" lines, want exactly 1\noutput:\n%s", got, out)
		}
		if !strings.Contains(out, "Creating container: shrine-deploy-test.alias-app") {
			t.Errorf("progress line does not carry the full <team>.<name>\noutput:\n%s", out)
		}
		if strings.Contains(out, "Creating container: .") {
			t.Errorf("output invents a container name with a leading separator\noutput:\n%s", out)
		}
		if got := strings.Count(out, "❌ Error [container.create]"); got != 2 {
			t.Errorf("got %d error lines, want 2 (backend + engine re-emit)\noutput:\n%s", got, out)
		}
	})

	t.Run("successful creation output is unchanged", func(t *testing.T) {
		var buf bytes.Buffer
		obs := NewTerminalObserver(&buf)

		obs.OnEvent(engine.Event{Name: "container.create", Status: engine.StatusInfo,
			Fields: map[string]string{"team": "shrine-deploy-test", "name": "alias-app"}})

		want := "  🏗️  Creating container: shrine-deploy-test.alias-app\n"
		if got := buf.String(); got != want {
			t.Errorf("success output changed:\ngot  %q\nwant %q", got, want)
		}
	})
}
