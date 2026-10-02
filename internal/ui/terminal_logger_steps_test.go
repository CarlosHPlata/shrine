package ui

import (
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/engine"
)

const clearLine = "\r\033[K"

// stepKind describes one event kind that opens a progress indicator. Tests
// assert on the indicator's state and on line counts, never on animation
// frames: whether a frame is written before the stop is nondeterministic.
type stepKind struct {
	name         string
	startStatus  engine.EventStatus
	fields       []string
	wantMessage  string
	wantFinished string
}

func (s stepKind) event(status engine.EventStatus, extra ...string) engine.Event {
	return ev(s.name, status, append(append([]string{}, s.fields...), extra...)...)
}

var stepKinds = []stepKind{
	{
		name:         "network.create",
		startStatus:  engine.StatusStarted,
		fields:       []string{"name", "shrine.team-a.private", "cidr", "10.100.1.0/24"},
		wantMessage:  "    🔨 Creating Docker network: shrine.team-a.private",
		wantFinished: "    ✅ Network created: shrine.team-a.private (10.100.1.0/24)\n",
	},
	{
		name:         "network.remove",
		startStatus:  engine.StatusStarted,
		fields:       []string{"name", "shrine.team-a.private"},
		wantMessage:  "  🌐 Removing network: shrine.team-a.private",
		wantFinished: "  ✅ Network removed: shrine.team-a.private\n",
	},
	{
		name:         "container.remove",
		startStatus:  engine.StatusStarted,
		fields:       []string{"name", "team-a.web"},
		wantMessage:  "    🗑️  Removing container: team-a.web",
		wantFinished: "    ✅ Container team-a.web removed\n",
	},
	{
		// The backend announces a volume with an info event and completes it
		// with volume.created, so volume.create has no finished line of its own.
		name:        "volume.create",
		startStatus: engine.StatusInfo,
		fields:      []string{"name", "team-a.db-data"},
		wantMessage: "    📦 Creating volume: team-a.db-data",
	},
	{
		name:         "image.pull",
		startStatus:  engine.StatusStarted,
		fields:       []string{"ref", "docker.io/traefik/whoami:latest"},
		wantMessage:  "    📥 Pulling image docker.io/traefik/whoami:latest...",
		wantFinished: "    ✅ Pulled image docker.io/traefik/whoami:latest\n",
	},
}

func assertNoCompletionLine(t *testing.T, out string) {
	t.Helper()
	if strings.Contains(out, "✅") {
		t.Errorf("unexpected completion line\noutput: %q", out)
	}
}

func TestTerminalObserver_StepStartsIndicator(t *testing.T) {
	for _, step := range stepKinds {
		t.Run(step.name, func(t *testing.T) {
			obs, _ := newObserverWithBuffer(t)

			obs.OnEvent(step.event(step.startStatus))

			if obs.spinner == nil {
				t.Fatal("no indicator started")
			}
			if obs.spinner.msg != step.wantMessage {
				t.Errorf("indicator message = %q, want %q", obs.spinner.msg, step.wantMessage)
			}
		})
	}
}

func TestTerminalObserver_StepFinishedStopsIndicatorAndPrintsOnce(t *testing.T) {
	for _, step := range stepKinds {
		t.Run(step.name, func(t *testing.T) {
			obs, buf := newObserverWithBuffer(t)

			obs.OnEvent(step.event(step.startStatus))
			obs.OnEvent(step.event(engine.StatusFinished))
			out := buf.String()

			if obs.spinner != nil {
				t.Error("indicator still running after the finished event")
			}
			if got := strings.Count(out, clearLine); got != 1 {
				t.Errorf("indicator line cleared %d times, want 1\noutput: %q", got, out)
			}
			if !strings.HasSuffix(out, clearLine+step.wantFinished) {
				t.Errorf("output should end with the cleared line then %q\noutput: %q", step.wantFinished, out)
			}
			if step.wantFinished == "" {
				assertNoCompletionLine(t, out)
				return
			}
			if got := strings.Count(out, step.wantFinished); got != 1 {
				t.Errorf("completion line written %d times, want 1\noutput: %q", got, out)
			}
		})
	}
}

func TestTerminalObserver_StepErrorStopsIndicatorWithoutCompletion(t *testing.T) {
	for _, step := range stepKinds {
		t.Run(step.name, func(t *testing.T) {
			obs, buf := newObserverWithBuffer(t)

			obs.OnEvent(step.event(step.startStatus))
			obs.OnEvent(step.event(engine.StatusError, "error", "boom"))
			out := buf.String()

			if obs.spinner != nil {
				t.Error("indicator still running after the error event")
			}
			if want := "  ❌ Error [" + step.name + "]: boom\n"; strings.Count(out, want) != 1 {
				t.Errorf("want exactly one %q\noutput: %q", want, out)
			}
			if got := strings.Count(out, clearLine); got != 1 {
				t.Errorf("indicator line cleared %d times, want 1\noutput: %q", got, out)
			}
			assertNoCompletionLine(t, out)
		})
	}
}

func TestTerminalObserver_FinishedWithoutStart(t *testing.T) {
	cases := []struct {
		event engine.Event
		want  string
	}{
		{event: ev("volume.created", engine.StatusFinished, "name", "team-a.db-data"),
			want: "    ✅ Volume team-a.db-data is created\n"},
	}
	for _, step := range stepKinds {
		cases = append(cases, struct {
			event engine.Event
			want  string
		}{step.event(engine.StatusFinished), step.wantFinished})
	}

	for _, tc := range cases {
		t.Run(tc.event.Name, func(t *testing.T) {
			obs, buf := newObserverWithBuffer(t)

			obs.OnEvent(tc.event)

			if got := buf.String(); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestTerminalObserver_VolumeCreatedCompletesVolumeIndicator(t *testing.T) {
	const created = "    ✅ Volume team-a.db-data is created\n"
	obs, buf := newObserverWithBuffer(t)

	obs.OnEvent(ev("volume.create", engine.StatusInfo, "name", "team-a.db-data", "mount", "/var/lib/data"))
	if obs.spinner == nil {
		t.Fatal("volume.create did not start an indicator")
	}
	obs.OnEvent(ev("volume.created", engine.StatusFinished, "name", "team-a.db-data", "mount", "/var/lib/data"))
	out := buf.String()

	if obs.spinner != nil {
		t.Error("volume indicator still running after volume.created")
	}
	if !strings.HasSuffix(out, clearLine+created) || strings.Count(out, created) != 1 {
		t.Errorf("output should end with the cleared line then exactly one %q\noutput: %q", created, out)
	}
}

func TestTerminalObserver_ContainerRemoveNotFoundSkipsIndicator(t *testing.T) {
	obs, buf := newObserverWithBuffer(t)

	obs.OnEvent(ev("container.remove", engine.StatusInfo, "name", "team-a.web", "reason", "not found"))

	if obs.spinner != nil {
		t.Error("a skipped removal must not start an indicator")
	}
	want := "    ℹ️  Container team-a.web not found, skipping removal\n"
	if got := buf.String(); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestTerminalObserver_WarningLeavesIndicatorRunning(t *testing.T) {
	obs, buf := newObserverWithBuffer(t)
	step := stepKinds[0]

	obs.OnEvent(step.event(step.startStatus))
	running := obs.spinner
	obs.OnEvent(step.event(engine.StatusWarning))

	if obs.spinner == nil || obs.spinner != running {
		t.Error("a warning must leave the running indicator untouched")
	}
	out := buf.String()
	if strings.Contains(out, clearLine) {
		t.Errorf("a warning must not clear the indicator line\noutput: %q", out)
	}
	assertNoCompletionLine(t, out)
}

func TestTerminalObserver_SilentKinds(t *testing.T) {
	cases := []struct {
		name  string
		event engine.Event
	}{
		{"routing.finalize started has no rendering", ev("routing.finalize", engine.StatusStarted)},
		{"routing.finalize info has no rendering", ev("routing.finalize", engine.StatusInfo)},
		{"application.deploy renders only when started", ev("application.deploy", engine.StatusFinished, "name", "web", "owner", "team-a")},
		{"application.teardown renders only when started", ev("application.teardown", engine.StatusFinished, "name", "web", "team", "team-a")},
		{"resource.deploy renders only when started", ev("resource.deploy", engine.StatusFinished, "name", "db", "type", "postgres")},
		{"resource.teardown renders only when started", ev("resource.teardown", engine.StatusFinished, "name", "db", "team", "team-a")},
		{"container.create renders only as info", ev("container.create", engine.StatusStarted, "team", "team-a", "name", "web")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs, buf := newObserverWithBuffer(t)

			obs.OnEvent(tc.event)

			if got := buf.String(); got != "" {
				t.Errorf("expected no output, got %q", got)
			}
		})
	}
}
