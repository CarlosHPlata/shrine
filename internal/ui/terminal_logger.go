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

	case "dns.register":
		fmt.Fprintf(t.out, "  🌍 Registering DNS: %s\n", e.Fields["domain"])

	case "network.create":
		switch e.Status {
		case engine.StatusStarted:
			fmt.Fprintf(t.out, "    🔨 Creating Docker network: %s\n", e.Fields["name"])
		case engine.StatusFinished:
			fmt.Fprintf(t.out, "    ✅ Network created: %s (%s)\n", e.Fields["name"], e.Fields["cidr"])
		}

	case "network.remove":
		switch e.Status {
		case engine.StatusStarted:
			fmt.Fprintf(t.out, "  🌐 Removing network: %s\n", e.Fields["name"])
		case engine.StatusFinished:
			fmt.Fprintf(t.out, "  ✅ Network removed: %s\n", e.Fields["name"])
		}

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
		} else if e.Status == engine.StatusFinished {
			fmt.Fprintf(t.out, "    ✅ Container %s removed\n", e.Fields["name"])
		}

	case "volume.create":
		fmt.Fprintf(t.out, "    📦 Creating volume: %s\n", e.Fields["name"])

	case "volume.created":
		fmt.Fprintf(t.out, "    ✅ Volume %s is created\n", e.Fields["name"])

	case "image.pull":
		t.handleImagePull(e)
	}
}

func (t *TerminalObserver) handleImagePull(e engine.Event) {
	switch e.Status {
	case engine.StatusStarted:
		t.spinner = newSpinner(t.out, fmt.Sprintf("Pulling image %s...", e.Fields["ref"]))
		t.spinner.start()
	case engine.StatusFinished, engine.StatusError:
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
