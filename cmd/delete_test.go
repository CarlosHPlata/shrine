package cmd_test

import "testing"

func TestDeleteResource_RequiresArg(t *testing.T) {
	t.Run("no argument", func(t *testing.T) { assertRequiresOneArg(t, "delete", "resource") })
	t.Run("two arguments", func(t *testing.T) { assertRequiresOneArg(t, "delete", "resource", "cache", "web") })
}
