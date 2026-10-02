package ui

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CarlosHPlata/shrine/internal/engine"
)

// countingCloser is the in-memory log destination: file-logger tests never
// open a file.
type countingCloser struct {
	bytes.Buffer
	closes   int
	closeErr error
}

func (c *countingCloser) Close() error {
	c.closes++
	return c.closeErr
}

func (c *countingCloser) lines() []string {
	return strings.Split(strings.TrimSuffix(c.String(), "\n"), "\n")
}

// assertLogLine checks the timestamp's format and compares everything after
// it exactly, so the test does not depend on the wall clock.
func assertLogLine(t *testing.T, line, wantRest string) {
	t.Helper()
	const timestampLen = len("2006-01-02T15:04:05Z")
	if len(line) < timestampLen {
		t.Fatalf("line %q is too short to carry a timestamp", line)
	}
	timestamp, rest := line[:timestampLen], line[timestampLen:]
	if _, err := time.Parse(time.RFC3339, timestamp); err != nil || !strings.HasSuffix(timestamp, "Z") {
		t.Errorf("timestamp %q is not UTC RFC3339 (parse error: %v)", timestamp, err)
	}
	if rest != wantRest {
		t.Errorf("log line changed after the timestamp:\ngot  %q\nwant %q", rest, wantRest)
	}
}

func TestFileLogger_FormatsFieldsSortedAndQuoted(t *testing.T) {
	cases := []struct {
		name  string
		event engine.Event
		want  string
	}{
		{
			name: "fields are written in ascending key order",
			event: engine.Event{Name: "container.create", Status: engine.StatusStarted,
				Fields: map[string]string{"team": "shrine-deploy-test", "name": "alias-app"}},
			want: ` [started] container.create name="alias-app" team="shrine-deploy-test"`,
		},
		{
			name: "values with spaces and quotes are escaped",
			event: engine.Event{Name: "container.create", Status: engine.StatusError,
				Fields: map[string]string{"error": `creating "x": bad "ref"`}},
			want: ` [error] container.create error="creating \"x\": bad \"ref\""`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dest := &countingCloser{}

			newFileLogger(dest).OnEvent(tc.event)

			if !strings.HasSuffix(dest.String(), "\n") {
				t.Errorf("line is not newline-terminated: %q", dest.String())
			}
			assertLogLine(t, dest.lines()[0], tc.want)
		})
	}
}

func TestFileLogger_FieldlessLineEndsAfterName(t *testing.T) {
	dest := &countingCloser{}

	newFileLogger(dest).OnEvent(engine.Event{Name: "routing.finalize", Status: engine.StatusInfo})

	assertLogLine(t, dest.lines()[0], " [info] routing.finalize")
}

func TestFileLogger_WritesOneLinePerEventForEveryStatus(t *testing.T) {
	statuses := []engine.EventStatus{
		engine.StatusStarted, engine.StatusFinished, engine.StatusInfo, engine.StatusWarning, engine.StatusError,
	}
	dest := &countingCloser{}
	logger := newFileLogger(dest)

	for _, status := range statuses {
		logger.OnEvent(engine.Event{Name: "routing.finalize", Status: status})
	}

	lines := dest.lines()
	if len(lines) != len(statuses) {
		t.Fatalf("wrote %d lines for %d events:\n%s", len(lines), len(statuses), dest.String())
	}
	for i, status := range statuses {
		assertLogLine(t, lines[i], " ["+string(status)+"] routing.finalize")
	}
}

func TestFileLogger_ConcurrentEventsProduceWholeLines(t *testing.T) {
	const writers, eventsPerWriter = 50, 20
	dest := &countingCloser{}
	logger := newFileLogger(dest)

	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range eventsPerWriter {
				logger.OnEvent(engine.Event{Name: "container.create", Status: engine.StatusInfo,
					Fields: map[string]string{"team": "team-a", "name": "web"}})
			}
		}()
	}
	wg.Wait()

	lines := dest.lines()
	if len(lines) != writers*eventsPerWriter {
		t.Fatalf("wrote %d lines, want %d", len(lines), writers*eventsPerWriter)
	}
	for _, line := range lines {
		assertLogLine(t, line, ` [info] container.create name="web" team="team-a"`)
	}
}

func TestFileLogger_CloseClosesDestination(t *testing.T) {
	errClose := errors.New("disk full")
	dest := &countingCloser{closeErr: errClose}

	err := newFileLogger(dest).Close()

	if !errors.Is(err, errClose) {
		t.Errorf("Close returned %v, want the destination's error", err)
	}
	if dest.closes != 1 {
		t.Errorf("destination closed %d times, want 1", dest.closes)
	}
}
