package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/cmd"
)

func TestDeleteResource_RequiresArg(t *testing.T) {
	cases := map[string][]string{
		"no argument":   {},
		"two arguments": {"cache", "web"},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			cmd.SetOutput(&out)
			args := append([]string{"delete", "resource"}, extra...)
			args = append(args, "--state-dir", t.TempDir(), "--config-dir", t.TempDir())
			cmd.SetArgs(args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error when 'delete resource' is invoked with %s", name)
			}
			if !strings.Contains(err.Error(), "accepts 1 arg(s)") {
				t.Errorf("expected Cobra arg-count error, got: %v", err)
			}
		})
	}
}
