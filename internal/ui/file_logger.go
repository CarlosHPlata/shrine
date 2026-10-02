package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CarlosHPlata/shrine/internal/engine"
)

const logFileName = "shrine.log"

// FileLogger appends every event (including errors) to {stateDir}/logs/shrine.log
// in a simple human-readable plain-text format.
type FileLogger struct {
	out io.WriteCloser
	mu  sync.Mutex
}

func NewFileLogger(stateDir string) (*FileLogger, error) {
	logsDir := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("creating logs dir %q: %w", logsDir, err)
	}

	path := filepath.Join(logsDir, logFileName)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening log file %q: %w", path, err)
	}

	return newFileLogger(f), nil
}

// newFileLogger takes the destination as a writer so the line format can be
// tested without a file.
func newFileLogger(out io.WriteCloser) *FileLogger {
	return &FileLogger{out: out}
}

func (l *FileLogger) OnEvent(e engine.Event) {
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(l.out, "%s [%s] %s%s\n", ts, e.Status, e.Name, formatFields(e.Fields))
}

func (l *FileLogger) Close() error {
	return l.out.Close()
}

func formatFields(fields map[string]string) string {
	if len(fields) == 0 {
		return ""
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, " %s=%q", k, fields[k])
	}
	return b.String()
}
