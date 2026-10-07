package ui

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/engine"
)

// safeBuffer is a concurrency-safe destination: the spinner goroutine writes
// while OnEvent does, which os.Stdout tolerates in production.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// newObserverWithBuffer stops any indicator still running when the test
// ends, so no spinner goroutine outlives it.
func newObserverWithBuffer(t *testing.T) (*TerminalObserver, *safeBuffer) {
	t.Helper()
	buf := &safeBuffer{}
	obs := NewTerminalObserver(buf)
	t.Cleanup(func() { stopSpinner(obs) })
	return obs, buf
}

func stopSpinner(obs *TerminalObserver) {
	if obs.spinner != nil {
		obs.spinner.stop()
		obs.spinner = nil
	}
}

// ev builds an event from alternating field keys and values.
func ev(name string, status engine.EventStatus, kv ...string) engine.Event {
	fields := make(map[string]string, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		fields[kv[i]] = kv[i+1]
	}
	return engine.Event{Name: name, Status: status, Fields: fields}
}

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

func TestTerminalObserver_RendersEachKind(t *testing.T) {
	const (
		traefikYml = "/routes/traefik.yml"
		routeFile  = "/routes/dynamic/team-a-web.yml"
		dashboard  = "/routes/dynamic/__shrine-dashboard.yml"
		hint       = "add a websecure entrypoint"
	)

	cases := []struct {
		event engine.Event
		want  string
	}{
		{ev("application.deploy", engine.StatusStarted, "name", "web", "owner", "team-a"),
			"🚀 Deploying Application: web (owner: team-a)\n"},
		{ev("application.teardown", engine.StatusStarted, "name", "web", "team", "team-a"),
			"🗑️  Tearing down Application: web (team: team-a)\n"},
		{ev("resource.deploy", engine.StatusStarted, "name", "db", "type", "postgres"),
			"📦 Deploying Resource: db (type: postgres)\n"},
		{ev("resource.teardown", engine.StatusStarted, "name", "db", "team", "team-a"),
			"🗑️  Tearing down Resource: db (team: team-a)\n"},
		{ev("network.ensure", engine.StatusInfo, "owner", "team-a"),
			"  🌐 Ensuring network: shrine.team-a.private\n"},
		{ev("container.create", engine.StatusInfo, "team", "team-a", "name", "web"),
			"  🏗️  Creating container: team-a.web\n"},
		{ev("routing.configure", engine.StatusInfo, "domain", "web.example.com", "port", "8080"),
			"  🔗 Configuring routing: web.example.com -> port 8080\n"},
		{ev("gateway.config.preserved", engine.StatusInfo, "path", traefikYml),
			"  📄 Preserving operator-owned traefik.yml: /routes/traefik.yml\n"},
		{ev("gateway.config.generated", engine.StatusInfo, "path", traefikYml),
			"  📝 Generated default traefik.yml: /routes/traefik.yml\n"},
		{ev("gateway.config.legacy_http_block", engine.StatusWarning, "path", traefikYml, "hint", hint),
			"  ⚠️  Legacy http block in traefik.yml at /routes/traefik.yml — add a websecure entrypoint\n"},
		{ev("gateway.config.tls_port_no_websecure", engine.StatusWarning, "path", traefikYml, "hint", hint),
			"  ⚠️  tlsPort set but traefik.yml is missing websecure entrypoint at /routes/traefik.yml — add a websecure entrypoint\n"},
		{ev("gateway.alias.tls_no_websecure", engine.StatusWarning, "path", traefikYml, "team", "team-a", "name", "web", "tls_aliases", "alias.example.com", "hint", hint),
			"  ⚠️  alias tls: true but websecure entrypoint missing in /routes/traefik.yml for team-a.web (alias.example.com) — add a websecure entrypoint\n"},
		{ev("gateway.config.legacy_probe_error", engine.StatusWarning, "path", traefikYml, "error", "permission denied"),
			"  ⚠️  Could not probe traefik.yml for legacy http block (deploy continues): /routes/traefik.yml (permission denied)\n"},
		{ev("gateway.config.tls_port_probe_error", engine.StatusWarning, "path", traefikYml, "error", "permission denied"),
			"  ⚠️  Could not probe traefik.yml for websecure entrypoint (deploy continues): /routes/traefik.yml (permission denied)\n"},
		{ev("gateway.dashboard.generated", engine.StatusInfo, "path", dashboard),
			"  📝 Generated dashboard dynamic file: /routes/dynamic/__shrine-dashboard.yml\n"},
		{ev("gateway.dashboard.preserved", engine.StatusInfo, "path", dashboard),
			"  📄 Preserving operator-owned dashboard dynamic file: /routes/dynamic/__shrine-dashboard.yml\n"},
		{ev("gateway.dashboard.removed", engine.StatusInfo, "path", dashboard),
			"  🗑️  Removed stale dashboard dynamic file: /routes/dynamic/__shrine-dashboard.yml\n"},
		{ev("gateway.route.generated", engine.StatusInfo, "path", routeFile),
			"  📝 Generated route file: /routes/dynamic/team-a-web.yml\n"},
		{ev("gateway.route.preserved", engine.StatusInfo, "path", routeFile),
			"  📄 Preserving operator-owned route file: /routes/dynamic/team-a-web.yml\n"},
		{ev("gateway.route.stat_error", engine.StatusWarning, "path", routeFile, "error", "permission denied"),
			"  ⚠️  Could not stat route file (deploy continues): /routes/dynamic/team-a-web.yml (permission denied)\n"},
		{ev("gateway.route.orphan", engine.StatusWarning, "path", routeFile),
			"  ⚠️  Orphan route file left on disk; remove with: rm /routes/dynamic/team-a-web.yml\n"},
		{ev("dns.register", engine.StatusInfo, "domain", "web.example.com"),
			"  🌍 Registering DNS: web.example.com\n"},
		{ev("container.start", engine.StatusInfo, "name", "team-a.web"),
			"    ▶️  Starting existing container: team-a.web\n"},
		{ev("container.recreate", engine.StatusInfo, "name", "team-a.web"),
			"    🔄 Image changed for team-a.web, replacing container...\n"},
		{ev("container.fresh", engine.StatusInfo, "name", "team-a.web"),
			"    ✨ Creating fresh container: team-a.web\n"},
		{ev("container.created", engine.StatusFinished, "name", "team-a.web"),
			"    ✅ Container team-a.web is running\n"},
		{ev("hostport.published", engine.StatusFinished, "team", "team-a", "name", "web", "hostPort", "30000", "containerPort", "8080", "proto", "tcp"),
			"    📡 Published team-a/web on 127.0.0.1:30000 -> 8080/tcp\n"},
		{ev("container.remove", engine.StatusInfo, "name", "team-a.web", "reason", "not found"),
			"    ℹ️  Container team-a.web not found, skipping removal\n"},
		{ev("image.resolve", engine.StatusStarted, "team", "team-a", "name", "web", "ref", "nginx:1.27"),
			"🔎 Resolving image for team-a.web (nginx:1.27)\n"},
		{ev("image.resolve", engine.StatusFinished, "team", "team-a", "name", "web", "ref", "nginx:1.27",
			"digest", "sha256:a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2", "source", "manifest"),
			"  🔎 Resolved team-a.web nginx:1.27@a1b2c3d4e5f6\n"},
		{ev("image.resolve", engine.StatusFinished, "team", "team-a", "name", "web", "ref", "ghcr.io/me/web@sha256:3f2a9c1b4d7e3f2a",
			"digest", "sha256:3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a", "requested", "ghcr.io/me/web:latest", "source", "resolved"),
			"  📌 Pinned team-a.web at latest@3f2a9c1b4d7e\n"},
		{ev("image.resolve", engine.StatusFinished, "team", "team-a", "name", "db", "ref", "postgres@sha256:9c1b4d7e3f2a9c1b",
			"digest", "sha256:9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b", "requested", "postgres:17", "source", "pinned", "pinned_at", "2026-10-06"),
			"  📌 Using pinned team-a.db 17@9c1b4d7e3f2a (since 2026-10-06)\n"},
	}

	for _, tc := range cases {
		t.Run(tc.event.Name, func(t *testing.T) {
			obs, buf := newObserverWithBuffer(t)

			obs.OnEvent(tc.event)

			if got := buf.String(); got != tc.want {
				t.Errorf("rendered line changed:\ngot  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestTerminalObserver_ImageResolveFinishedWithoutDigestShowsTheReferenceAlone(t *testing.T) {
	obs, buf := newObserverWithBuffer(t)

	obs.OnEvent(ev("image.resolve", engine.StatusFinished, "team", "team-a", "name", "web", "ref", "nginx:1.27", "digest", "", "source", "manifest"))

	want := "  🔎 Resolved team-a.web nginx:1.27\n"
	if got := buf.String(); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// The resolving line is a plain line: image.pull opens its own indicator
// inside the step, and the renderer holds one indicator at a time.
func TestTerminalObserver_ImageResolveStartsNoIndicatorAndErrorsGenerically(t *testing.T) {
	obs, buf := newObserverWithBuffer(t)

	obs.OnEvent(ev("image.resolve", engine.StatusStarted, "team", "team-a", "name", "web", "ref", "nginx:1.27"))
	if obs.spinner != nil {
		t.Fatal("the resolving line must not start an indicator")
	}
	obs.OnEvent(ev("image.resolve", engine.StatusError, "team", "team-a", "name", "web", "ref", "nginx:1.27", "error", "boom"))

	want := "🔎 Resolving image for team-a.web (nginx:1.27)\n  ❌ Error [image.resolve]: boom\n"
	if got := buf.String(); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestTerminalObserver_RoutingConfigureAliases(t *testing.T) {
	const routingLine = "  🔗 Configuring routing: web.example.com -> port 8080\n"

	t.Run("aliases add a sub-line", func(t *testing.T) {
		obs, buf := newObserverWithBuffer(t)

		obs.OnEvent(ev("routing.configure", engine.StatusInfo,
			"domain", "web.example.com", "port", "8080", "aliases", "a.example.com, b.example.com/api"))

		want := routingLine + "    ↳ Aliases: a.example.com, b.example.com/api\n"
		if got := buf.String(); got != want {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("no aliases, no sub-line", func(t *testing.T) {
		obs, buf := newObserverWithBuffer(t)

		obs.OnEvent(ev("routing.configure", engine.StatusInfo,
			"domain", "web.example.com", "port", "8080", "aliases", ""))

		if got := buf.String(); got != routingLine {
			t.Errorf("got  %q\nwant %q", got, routingLine)
		}
	})
}

func TestTerminalObserver_GenericErrorLine(t *testing.T) {
	t.Run("kind with no dedicated rendering", func(t *testing.T) {
		obs, buf := newObserverWithBuffer(t)

		obs.OnEvent(ev("routing.finalize", engine.StatusError, "error", "boom"))

		want := "  ❌ Error [routing.finalize]: boom\n"
		if got := buf.String(); got != want {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})

	t.Run("kind rendered at every status keeps its own line too", func(t *testing.T) {
		obs, buf := newObserverWithBuffer(t)

		obs.OnEvent(ev("dns.register", engine.StatusError, "domain", "web.example.com", "error", "boom"))

		want := "  ❌ Error [dns.register]: boom\n  🌍 Registering DNS: web.example.com\n"
		if got := buf.String(); got != want {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	})
}

func TestTerminalObserver_PinnedLinesForUntaggedAndDigestRequests(t *testing.T) {
	const digest = "sha256:9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b4d7e3f2a9c1b"
	obs, buf := newObserverWithBuffer(t)

	obs.OnEvent(ev("image.resolve", engine.StatusFinished, "team", "team-a", "name", "db", "ref", "postgres@"+digest,
		"digest", digest, "requested", "postgres", "source", "pinned", "pinned_at", "2026-10-06"))
	obs.OnEvent(ev("image.resolve", engine.StatusFinished, "team", "team-a", "name", "db", "ref", "postgres@"+digest,
		"digest", digest, "requested", "postgres@"+digest, "source", "pinned", "pinned_at", "2026-10-06"))

	want := "  📌 Using pinned team-a.db latest@9c1b4d7e3f2a (since 2026-10-06)\n" +
		"  📌 Using pinned team-a.db 9c1b4d7e3f2a (since 2026-10-06)\n"
	if got := buf.String(); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
