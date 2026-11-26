//go:build integration

package integration_test

import (
	"testing"

	. "github.com/CarlosHPlata/shrine/tests/integration/testutils"
)

// traefik/whoami's entrypoint is /whoami and it declares no CMD, so a
// manifest command reaches /whoami as its arguments.
const (
	appWithCommand      = testTeam + ".whoami-cmd"
	resourceWithCommand = testTeam + ".cache-cmd"
	appWithoutCommand   = testTeam + ".whoami"
)

func TestDeployCommand(t *testing.T) {
	s := NewDockerSuite(t, testTeam)

	s.BeforeEach(func(tc *TestCase) {
		tc.StateDir = tc.TempDir()
		SeedSubnetState(tc)
		tc.Run("apply", "teams",
			"--path", fixturesPath("team"),
			"--state-dir", tc.StateDir,
		).AssertSuccess()
	})

	s.Test("an Application command replaces the image's default command and keeps its entrypoint", func(tc *TestCase) {
		tc.Deploy(fixturesPath("command-app")).AssertSuccess()

		// Running proves the entrypoint is still /whoami: a replaced
		// entrypoint would try to execute "--name" and exit.
		tc.AssertContainerRunning(appWithCommand)
		tc.AssertContainerCmd(appWithCommand, []string{"--name", "from-command"})
	})

	s.Test("a Resource command replaces the image's default command and keeps its entrypoint", func(tc *TestCase) {
		tc.Deploy(fixturesPath("command-resource")).AssertSuccess()

		tc.AssertContainerRunning(resourceWithCommand)
		tc.AssertContainerCmd(resourceWithCommand, []string{"--name", "resource-command"})
	})

	s.Test("a manifest without command keeps the image's default command", func(tc *TestCase) {
		tc.Deploy(fixturesPath("basic")).AssertSuccess()

		tc.AssertContainerRunning(appWithoutCommand)
		tc.AssertContainerCmd(appWithoutCommand, nil)
	})

	s.Test("adding, changing or removing command recreates the container; an unchanged command leaves it alone", func(tc *TestCase) {
		app := tc.CopyManifest(fixturesPath("basic", "app.yml"))
		tc.Deploy(app.Dir()).AssertSuccess()
		container := tc.WatchContainer(appWithoutCommand)

		// Adding a command recreates the container.
		app.Set("spec.command", []string{"--name", "first"})
		tc.Deploy(app.Dir()).AssertSuccess()
		container.AssertRecreated()
		tc.AssertContainerCmd(appWithoutCommand, []string{"--name", "first"})

		// Deploying the same command again leaves it alone.
		tc.Deploy(app.Dir()).AssertSuccess()
		container.AssertNotRecreated()

		// Changing the command recreates it.
		app.Set("spec.command", []string{"--name", "second"})
		tc.Deploy(app.Dir()).AssertSuccess()
		container.AssertRecreated()
		tc.AssertContainerCmd(appWithoutCommand, []string{"--name", "second"})

		// Removing the command recreates it with the image's default.
		app.Remove("spec.command")
		tc.Deploy(app.Dir()).AssertSuccess()
		container.AssertRecreated()
		tc.AssertContainerCmd(appWithoutCommand, nil)
	})

	s.Test("a command written as a plain string is rejected before anything is deployed", func(tc *TestCase) {
		app := tc.CopyManifest(fixturesPath("basic", "app.yml"))
		app.Set("spec.command", "npm start")

		tc.Deploy(app.Dir()).
			AssertFailure().
			AssertStderrContains(app.Path()).
			AssertStderrContains("spec.command must be a list of strings")
		tc.AssertContainerNotExists(appWithoutCommand)
	})

	s.Test("an empty command list is rejected before anything is deployed", func(tc *TestCase) {
		app := tc.CopyManifest(fixturesPath("basic", "app.yml"))
		app.Set("spec.command", []string{})

		tc.Deploy(app.Dir()).
			AssertFailure().
			AssertStderrContains(app.Path()).
			AssertStderrContains("spec.command must not be empty")
		tc.AssertContainerNotExists(appWithoutCommand)
	})

	s.Test("a command with an empty item is rejected before anything is deployed", func(tc *TestCase) {
		app := tc.CopyManifest(fixturesPath("basic", "app.yml"))
		app.Set("spec.command", []string{"npm", ""})

		tc.Deploy(app.Dir()).
			AssertFailure().
			AssertStderrContains(app.Path()).
			AssertStderrContains("spec.command[1] must not be empty")
		tc.AssertContainerNotExists(appWithoutCommand)
	})
}
