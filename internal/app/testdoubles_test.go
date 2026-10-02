package app

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/engine/local"
	"github.com/CarlosHPlata/shrine/internal/plugins/gateway/traefik"
	"github.com/CarlosHPlata/shrine/internal/plugins/secrets"
	"github.com/CarlosHPlata/shrine/internal/state"
)

var errBoom = errors.New("boom")

// fakeFileLogger stands in for the on-disk logger: bundle tests never touch
// the filesystem.
type fakeFileLogger struct {
	created  bool
	stateDir string
	events   []engine.Event
	closes   int
	closeErr error
}

func (f *fakeFileLogger) OnEvent(e engine.Event) { f.events = append(f.events, e) }

func (f *fakeFileLogger) Close() error {
	f.closes++
	return f.closeErr
}

// swapConstructor replaces a constructor variable for the duration of one
// test. Tests that swap must not run in parallel.
func swapConstructor[T any](t *testing.T, target *T, replacement T) {
	t.Helper()
	original := *target
	*target = replacement
	t.Cleanup(func() { *target = original })
}

func useInMemoryFileLogger(t *testing.T) *fakeFileLogger {
	t.Helper()
	fake := &fakeFileLogger{}
	swapConstructor(t, &newFileLogger, func(stateDir string) (closableObserver, error) {
		fake.created = true
		fake.stateDir = stateDir
		return fake, nil
	})
	return fake
}

// pinHermeticEnv removes the environment-dependent branches of bundle
// assembly: specsDir resolution and the Docker client built from env (a
// non-empty DOCKER_CERT_PATH would read certificate files).
func pinHermeticEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", "/home/test-user")
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CERT_PATH", "")
}

type bundleInputs struct {
	cfg    *config.Config
	store  *state.Store
	paths  *config.Paths
	out    *bytes.Buffer
	errOut *bytes.Buffer
}

// newBundleInputs is the minimal valid input: no registries, no gateway, no
// vault. The store is non-nil because engine construction dereferences it.
func newBundleInputs() bundleInputs {
	return bundleInputs{
		cfg:    &config.Config{SpecsDir: "/abs/specs"},
		store:  &state.Store{},
		paths:  &config.Paths{StateDir: "/nonexistent/state"},
		out:    &bytes.Buffer{},
		errOut: &bytes.Buffer{},
	}
}

// bundleBuilder erases the three bundle types so one table can drive every
// command's assembly.
type bundleBuilder struct {
	command string
	build   func(in bundleInputs) (bundleIsNil bool, cleanup func() error, err error)
}

var (
	applyBuilder = bundleBuilder{"apply", func(in bundleInputs) (bool, func() error, error) {
		bundle, cleanup, err := BuildApplyBundle(in.cfg, in.store, in.paths, in.out, in.errOut)
		return bundle == nil, cleanup, err
	}}
	deployBuilder = bundleBuilder{"deploy", func(in bundleInputs) (bool, func() error, error) {
		bundle, cleanup, err := BuildDeployBundle(in.cfg, in.store, in.paths, "", in.out, in.errOut)
		return bundle == nil, cleanup, err
	}}
	teardownBuilder = bundleBuilder{"teardown", func(in bundleInputs) (bool, func() error, error) {
		bundle, cleanup, err := BuildTeardownBundle(in.cfg, in.store, in.paths, in.out)
		return bundle == nil, cleanup, err
	}}
	allBuilders = []bundleBuilder{applyBuilder, deployBuilder, teardownBuilder}
)

func failingFileLogger(err error) func(string) (closableObserver, error) {
	return func(string) (closableObserver, error) { return nil, err }
}

func failingVault(err error) func(*config.Config) (secrets.SecretsPlugin, error) {
	return func(*config.Config) (secrets.SecretsPlugin, error) { return nil, err }
}

func failingContainerBackend(err error) func(*state.Store, []config.RegistryConfig, engine.Observer) (engine.ContainerBackend, error) {
	return func(*state.Store, []config.RegistryConfig, engine.Observer) (engine.ContainerBackend, error) {
		return nil, err
	}
}

func failingEngine(err error) func(local.EngineOptions) (*engine.Engine, error) {
	return func(local.EngineOptions) (*engine.Engine, error) { return nil, err }
}

func vaultMustNotBeCalled(t *testing.T) func(*config.Config) (secrets.SecretsPlugin, error) {
	return func(*config.Config) (secrets.SecretsPlugin, error) {
		t.Error("the secrets vault was constructed")
		return nil, errBoom
	}
}

func containerBackendMustNotBeCalled(t *testing.T) func(*state.Store, []config.RegistryConfig, engine.Observer) (engine.ContainerBackend, error) {
	return func(*state.Store, []config.RegistryConfig, engine.Observer) (engine.ContainerBackend, error) {
		t.Error("the standalone container backend was constructed")
		return nil, errBoom
	}
}

func traefikPluginMustNotBeCalled(t *testing.T) func(*config.Config, engine.ContainerBackend, string, engine.Observer) (*traefik.Plugin, error) {
	return func(*config.Config, engine.ContainerBackend, string, engine.Observer) (*traefik.Plugin, error) {
		t.Error("the gateway plugin was constructed")
		return nil, errBoom
	}
}

func requireAssembled(t *testing.T, cleanup func() error, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("assembly failed: %v", err)
	}
	if cleanup == nil {
		t.Fatal("assembly succeeded without a cleanup func")
	}
}

func assertSlotFailure(t *testing.T, err error, bundleIsNil, cleanupIsNil bool, prefix string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error beginning with %q, got nil", prefix)
	}
	if !strings.HasPrefix(err.Error(), prefix) {
		t.Errorf("error %q should begin with %q", err.Error(), prefix)
	}
	if !bundleIsNil || !cleanupIsNil {
		t.Errorf("expected no bundle and no cleanup on failure, got bundle=%v cleanup=%v", !bundleIsNil, !cleanupIsNil)
	}
}

// assertReachesLogger emits an event the terminal renders nothing for and
// checks it arrives at the file logger — observers hold slices, so they
// cannot be compared for identity.
func assertReachesLogger(t *testing.T, slot string, observer engine.Observer, logger *fakeFileLogger) {
	t.Helper()
	if observer == nil {
		t.Fatalf("%s is nil", slot)
	}
	before := len(logger.events)
	observer.OnEvent(engine.Event{Name: "routing.finalize", Status: engine.StatusInfo})
	if got := len(logger.events) - before; got != 1 {
		t.Errorf("%s delivered %d events to the file logger, want 1", slot, got)
	}
}
