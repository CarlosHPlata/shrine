//go:build integration

package testutils

import (
	"os/exec"
	"strings"
	"testing"
)

type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

var binaryPath string

func Setup(path string) {
	binaryPath = path
}

func Execute(t *testing.T, args ...string) *Result {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error running %s: %v", binaryPath, err)
		}
	}

	return &Result{
		ExitCode: exitCode,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
}
