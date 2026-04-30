package ui

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/CarlosHPlata/shrine/internal/engine"
)

type TerminalObserver struct {
	out     io.Writer
	spinner *spinner
}

func NewTerminalObserver(out io.Writer) *TerminalObserver {
	return &TerminalObserver{out: out}
}

func (t *TerminalObserver) OnEvent(e engine.Event) {
	if e.Status == engine.StatusError {
		fmt.Fprintf(t.out, "  ❌ Error [%s]: %s\n", e.Name, e.Fields["error"])
	}

	switch e.Name {
	case "application.deploy":
		if e.Status == engine.StatusStarted {
			fmt.Fprintf(t.out, "🚀 Deploying Application: %s (owner: %s)\n", e.Fields["name"], e.Fields["owner"])
		}

	case "application.teardown":
		if e.Status == engine.StatusStarted {
			fmt.Fprintf(t.out, "🗑️  Tearing down Application: %s (team: %s)\n", e.Fields["name"], e.Fields["team"])
		}

	case "resource.deploy":
		if e.Status == engine.StatusStarted {
			fmt.Fprintf(t.out, "📦 Deploying Resource: %s (type: %s)\n", e.Fields["name"], e.Fields["type"])
		}

	case "resource.teardown":
		if e.Status == engine.StatusStarted {
			fmt.Fprintf(t.out, "🗑️  Tearing down Resource: %s (team: %s)\n", e.Fields["name"], e.Fields["team"])
		}

	case "network.ensure":
		fmt.Fprintf(t.out, "  🌐 Ensuring network: shrine.%s.private\n", e.Fields["owner"])

	case "container.create":
		fmt.Fprintf(t.out, "  🏗️  Creating container: %s.%s\n", e.Fields["team"], e.Fields["name"])

	case "routing.configure":
		fmt.Fprintf(t.out, "  🔗 Configuring routing: %s -> port %s\n", e.Fields["domain"], e.Fields["port"])

	case "gateway.config.preserved":
		fmt.Fprintf(t.out, "  📄 Preserving operator-owned traefik.yml: %s\n", e.Fields["path"])

	case "gateway.config.generated":
		fmt.Fprintf(t.out, "  📝 Generated default traefik.yml: %s\n", e.Fields["path"])

	case "dns.register":
		fmt.Fprintf(t.out, "  🌍 Registering DNS: %s\n", e.Fields["domain"])

	case "network.create":
		t.handleStep(e, engine.StatusStarted, "    ", "🔨 Creating Docker network: %s", "name",
			"✅ Network created: %s (%s)", "name", "cidr")

	case "network.remove":
		t.handleStep(e, engine.StatusStarted, "  ", "🌐 Removing network: %s", "name",
			"✅ Network removed: %s", "name")

	case "container.start":
		fmt.Fprintf(t.out, "    ▶️  Starting existing container: %s\n", e.Fields["name"])

	case "container.recreate":
		fmt.Fprintf(t.out, "    🔄 Image changed for %s, replacing container...\n", e.Fields["name"])

	case "container.fresh":
		fmt.Fprintf(t.out, "    ✨ Creating fresh container: %s\n", e.Fields["name"])

	case "container.created":
		fmt.Fprintf(t.out, "    ✅ Container %s is running\n", e.Fields["name"])

	case "container.remove":
		if e.Status == engine.StatusInfo && e.Fields["reason"] == "not found" {
			fmt.Fprintf(t.out, "    ℹ️  Container %s not found, skipping removal\n", e.Fields["name"])
		} else {
			t.handleStep(e, engine.StatusStarted, "    ", "🗑️  Removing container: %s", "name",
				"✅ Container %s removed", "name")
		}

	case "volume.create":
		// volume.create uses StatusInfo as its "started" status in docker_backend.go
		t.handleStep(e, engine.StatusInfo, "    ", "📦 Creating volume: %s", "name", "", "")

	case "volume.created":
		// volume.created is only StatusFinished, so we treat it as the finish of volume.create
		// We set startStatus to something that won't match so it only triggers the Finished case
		e.Name = "volume.create"
		t.handleStep(e, "", "    ", "", "", "✅ Volume %s is created", "name")

	case "image.pull":
		t.handleStep(e, engine.StatusStarted, "    ", "📥 Pulling image %s...", "ref", "✅ Pulled image %s", "ref")
	}
}

func (t *TerminalObserver) handleStep(e engine.Event, startStatus engine.EventStatus, prefix string, startFmt string, startFields string, finishFmt string, finishFields ...string) {
	switch e.Status {
	case startStatus:
		msg := fmt.Sprintf(startFmt, e.Fields[startFields])
		t.spinner = newSpinner(t.out, prefix+msg)
		t.spinner.start()
	case engine.StatusFinished:
		if t.spinner != nil {
			t.spinner.stop()
			t.spinner = nil
		}
		if finishFmt != "" {
			args := make([]interface{}, len(finishFields))
			for i, f := range finishFields {
				args[i] = e.Fields[f]
			}
			fmt.Fprintf(t.out, prefix+finishFmt+"\n", args...)
		}
	case engine.StatusError:
		if t.spinner != nil {
			t.spinner.stop()
			t.spinner = nil
		}
	}
}


// spinner runs a simple terminal animation in a goroutine.
type spinner struct {
	out    io.Writer
	msg    string
	stopCh chan struct{}
	doneCh chan struct{}
	once   sync.Once
}

func newSpinner(out io.Writer, msg string) *spinner {
	return &spinner{
		out:    out,
		msg:    msg,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

func (s *spinner) start() {
	go func() {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		i := 0
		for {
			select {
			case <-s.stopCh:
				fmt.Fprint(s.out, "\r\033[K")
				close(s.doneCh)
				return
			default:
				fmt.Fprintf(s.out, "\r    %s %s", frames[i%len(frames)], s.msg)
				i++
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
}

func (s *spinner) stop() {
	s.once.Do(func() {
		close(s.stopCh)
		<-s.doneCh
	})
}
