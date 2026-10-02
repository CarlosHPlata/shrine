package app

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/state"
)

func TestBuildDeployBundle_FailsBeforeConstructionWhenSpecsDirUnresolvable(t *testing.T) {
	t.Setenv("HOME", "")
	cfg := &config.Config{SpecsDir: "~/manifests"}

	bundle, cleanup, err := BuildDeployBundle(cfg, nil, nil, "", io.Discard, io.Discard)

	assertUnresolvableSpecsDir(t, err)
	if bundle != nil || cleanup != nil {
		t.Fatalf("expected no bundle and no cleanup on failure, got bundle=%v cleanup=%v", bundle != nil, cleanup != nil)
	}
}

func TestBuildTeardownBundle_FailsBeforeConstructionWhenSpecsDirUnresolvable(t *testing.T) {
	t.Setenv("HOME", "")
	cfg := &config.Config{SpecsDir: "~/manifests"}

	bundle, cleanup, err := BuildTeardownBundle(cfg, nil, nil, io.Discard)

	assertUnresolvableSpecsDir(t, err)
	if bundle != nil || cleanup != nil {
		t.Fatalf("expected no bundle and no cleanup on failure, got bundle=%v cleanup=%v", bundle != nil, cleanup != nil)
	}
}

func assertUnresolvableSpecsDir(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected a resolution error, got nil")
	}
	for _, want := range []string{"resolving specsDir", "expanding ~"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should contain %q", err.Error(), want)
		}
	}
}

func TestResolveOptionalSpecsDir(t *testing.T) {
	const fakeHome = "/home/test-user"

	cases := []struct {
		name     string
		specsDir string
		home     string
		want     string
		wantErr  string
	}{
		{name: "absent specsDir is not an error", home: fakeHome},
		{name: "absolute value passes through", specsDir: "/abs/specs", home: fakeHome, want: "/abs/specs"},
		{name: "tilde expands when home is known", specsDir: "~/specs", home: fakeHome, want: fakeHome + "/specs"},
		{name: "tilde without home names specsDir", specsDir: "~/specs", wantErr: "resolving specsDir"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", tc.home)
			got, err := resolveOptionalSpecsDir(&config.Config{SpecsDir: tc.specsDir})
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("resolveOptionalSpecsDir succeeded with %q, want error containing %q", got, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveOptionalSpecsDir returned unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveOptionalSpecsDir(specsDir=%q) = %q, want %q", tc.specsDir, got, tc.want)
			}
		})
	}
}

// sharedSlots are the collaborators every bundle carries.
type sharedSlots struct {
	out      io.Writer
	cfg      *config.Config
	store    *state.Store
	paths    *config.Paths
	observer engine.Observer
	engine   *engine.Engine
}

func assertSharedSlots(t *testing.T, got sharedSlots, in bundleInputs, logger *fakeFileLogger) {
	t.Helper()
	if got.out != in.out {
		t.Error("Out is not the writer passed in")
	}
	if got.cfg != in.cfg {
		t.Error("Cfg is not the config passed in")
	}
	if got.store != in.store {
		t.Error("Store is not the store passed in")
	}
	if got.paths != in.paths {
		t.Error("Paths is not the paths passed in")
	}
	if logger.stateDir != in.paths.StateDir {
		t.Errorf("file logger opened under %q, want the state dir %q", logger.stateDir, in.paths.StateDir)
	}
	if pair, ok := got.observer.(engine.MultiObserver); !ok || len(pair) != 2 {
		t.Errorf("Observer = %T, want the terminal + file-logger pair", got.observer)
	}
	assertReachesLogger(t, "Observer", got.observer, logger)
	if got.engine == nil {
		t.Fatal("Engine is nil")
	}
	assertReachesLogger(t, "Engine.Observer", got.engine.Observer, logger)
	if in.out.Len() != 0 || in.errOut.Len() != 0 {
		t.Errorf("assembly wrote output: out=%q errOut=%q", in.out.String(), in.errOut.String())
	}
}

func TestBuildApplyBundle_ComposesMinimalConfig(t *testing.T) {
	pinHermeticEnv(t)
	logger := useInMemoryFileLogger(t)
	in := newBundleInputs()

	bundle, cleanup, err := BuildApplyBundle(in.cfg, in.store, in.paths, in.out, in.errOut)

	requireAssembled(t, cleanup, err)
	assertSharedSlots(t, sharedSlots{bundle.Out, bundle.Cfg, bundle.Store, bundle.Paths, bundle.Observer, bundle.Engine}, in, logger)
	if bundle.ErrOut != in.errOut {
		t.Error("ErrOut is not the writer passed in")
	}
	if bundle.Vault.IsActive() {
		t.Error("Vault should be inactive when no secrets plugin is configured")
	}
}

func TestBuildDeployBundle_ComposesMinimalConfig(t *testing.T) {
	pinHermeticEnv(t)
	logger := useInMemoryFileLogger(t)
	in := newBundleInputs()

	bundle, cleanup, err := BuildDeployBundle(in.cfg, in.store, in.paths, "", in.out, in.errOut)

	requireAssembled(t, cleanup, err)
	assertSharedSlots(t, sharedSlots{bundle.Out, bundle.Cfg, bundle.Store, bundle.Paths, bundle.Observer, bundle.Engine}, in, logger)
	if bundle.ErrOut != in.errOut {
		t.Error("ErrOut is not the writer passed in")
	}
	if bundle.SpecsDir != "/abs/specs" {
		t.Errorf("SpecsDir = %q, want %q", bundle.SpecsDir, "/abs/specs")
	}
	if bundle.Vault.IsActive() {
		t.Error("Vault should be inactive when no secrets plugin is configured")
	}
	if bundle.ContainerBackend == nil {
		t.Error("ContainerBackend is nil")
	}
	if bundle.Routing != nil || bundle.Engine.Routing != nil {
		t.Error("Routing should be absent when no gateway plugin is configured")
	}
}

func TestBuildTeardownBundle_ComposesMinimalConfig(t *testing.T) {
	cases := []struct {
		name     string
		specsDir string
	}{
		{name: "configured specsDir is resolved", specsDir: "/abs/specs"},
		{name: "absent specsDir is tolerated"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pinHermeticEnv(t)
			logger := useInMemoryFileLogger(t)
			in := newBundleInputs()
			in.cfg.SpecsDir = tc.specsDir

			bundle, cleanup, err := BuildTeardownBundle(in.cfg, in.store, in.paths, in.out)

			requireAssembled(t, cleanup, err)
			assertSharedSlots(t, sharedSlots{bundle.Out, bundle.Cfg, bundle.Store, bundle.Paths, bundle.Observer, bundle.Engine}, in, logger)
			if bundle.SpecsDir != tc.specsDir {
				t.Errorf("SpecsDir = %q, want %q", bundle.SpecsDir, tc.specsDir)
			}
			if bundle.Routing != nil || bundle.Engine.Routing != nil {
				t.Error("Routing should be absent when no gateway plugin is configured")
			}
		})
	}
}

func TestBuildTeardownBundle_NeverConstructsVault(t *testing.T) {
	pinHermeticEnv(t)
	useInMemoryFileLogger(t)
	swapConstructor(t, &newVault, vaultMustNotBeCalled(t))
	in := newBundleInputs()

	_, cleanup, err := BuildTeardownBundle(in.cfg, in.store, in.paths, in.out)

	requireAssembled(t, cleanup, err)
}

func TestBuildApplyBundle_NeverConstructsGatewayOrContainerBackend(t *testing.T) {
	t.Run("neither constructor is invoked", func(t *testing.T) {
		pinHermeticEnv(t)
		useInMemoryFileLogger(t)
		swapConstructor(t, &newTraefikPlugin, traefikPluginMustNotBeCalled(t))
		swapConstructor(t, &newContainerBackend, containerBackendMustNotBeCalled(t))
		in := newBundleInputs()

		_, cleanup, err := BuildApplyBundle(in.cfg, in.store, in.paths, in.out, in.errOut)

		requireAssembled(t, cleanup, err)
	})

	t.Run("an invalid gateway config cannot fail apply", func(t *testing.T) {
		pinHermeticEnv(t)
		useInMemoryFileLogger(t)
		in := newBundleInputs()
		in.cfg.Plugins.Gateway.Traefik = dashboardWithoutCredentials()

		_, cleanup, err := BuildApplyBundle(in.cfg, in.store, in.paths, in.out, in.errOut)

		requireAssembled(t, cleanup, err)
	})
}

func dashboardWithoutCredentials() *config.TraefikPluginConfig {
	return &config.TraefikPluginConfig{Dashboard: &config.TraefikDashboardConfig{Port: 8080}}
}

func TestBuildBundles_SlotFailures(t *testing.T) {
	cases := []struct {
		prefix  string
		bundles []bundleBuilder
		arrange func(t *testing.T, cfg *config.Config)
		// wantCause is the injected error for stand-in failures; real
		// constructors are matched on their message instead.
		wantCause    error
		wantContains string
		// opensLogger is true for every slot built after the observer pair.
		opensLogger bool
	}{
		{
			prefix:  "validating registries: ",
			bundles: []bundleBuilder{applyBuilder, deployBuilder},
			arrange: func(t *testing.T, cfg *config.Config) {
				cfg.Registries = []config.RegistryConfig{{Alias: "bad alias!"}}
			},
			wantContains: `registries: alias "bad alias!" contains invalid characters`,
		},
		{
			prefix:  "observer: ",
			bundles: allBuilders,
			arrange: func(t *testing.T, cfg *config.Config) {
				swapConstructor(t, &newFileLogger, failingFileLogger(errBoom))
			},
			wantCause:    errBoom,
			wantContains: "initializing file logger: ",
		},
		{
			prefix:  "container backend: ",
			bundles: []bundleBuilder{deployBuilder},
			arrange: func(t *testing.T, cfg *config.Config) {
				swapConstructor(t, &newContainerBackend, failingContainerBackend(errBoom))
			},
			wantCause:   errBoom,
			opensLogger: true,
		},
		{
			prefix:  "traefik: ",
			bundles: []bundleBuilder{deployBuilder, teardownBuilder},
			arrange: func(t *testing.T, cfg *config.Config) {
				cfg.Plugins.Gateway.Traefik = dashboardWithoutCredentials()
			},
			wantContains: "dashboard.port is set but username and password are required",
			opensLogger:  true,
		},
		{
			prefix:  "vault: ",
			bundles: []bundleBuilder{applyBuilder, deployBuilder},
			arrange: func(t *testing.T, cfg *config.Config) {
				swapConstructor(t, &newVault, failingVault(errBoom))
			},
			wantCause:   errBoom,
			opensLogger: true,
		},
		{
			prefix:  "routing: ",
			bundles: []bundleBuilder{deployBuilder, teardownBuilder},
			arrange: func(t *testing.T, cfg *config.Config) {
				t.Setenv("HOME", "")
				cfg.Plugins.Gateway.Traefik = &config.TraefikPluginConfig{RoutingDir: "~/routes"}
			},
			wantContains: "resolving routing-dir: expanding ~",
			opensLogger:  true,
		},
		{
			prefix:  "engine: ",
			bundles: allBuilders,
			arrange: func(t *testing.T, cfg *config.Config) {
				swapConstructor(t, &newLocalEngine, failingEngine(errBoom))
			},
			wantCause:   errBoom,
			opensLogger: true,
		},
	}

	for _, tc := range cases {
		for _, builder := range tc.bundles {
			t.Run(tc.prefix+builder.command, func(t *testing.T) {
				pinHermeticEnv(t)
				logger := useInMemoryFileLogger(t)
				in := newBundleInputs()
				tc.arrange(t, in.cfg)

				bundleIsNil, cleanup, err := builder.build(in)

				assertSlotFailure(t, err, bundleIsNil, cleanup == nil, tc.prefix)
				if tc.wantCause != nil && !errors.Is(err, tc.wantCause) {
					t.Errorf("error %q lost its cause %q", err, tc.wantCause)
				}
				if !strings.Contains(err.Error(), tc.wantContains) {
					t.Errorf("error %q should contain %q", err.Error(), tc.wantContains)
				}
				if tc.opensLogger && logger.closes != 1 {
					t.Errorf("log writer closed %d times after a late failure, want 1", logger.closes)
				}
				if !tc.opensLogger && (logger.created || logger.closes != 0) {
					t.Errorf("log writer touched before it could be opened: created=%v closes=%d", logger.created, logger.closes)
				}
			})
		}
	}
}

func TestBundleCleanup_ClosesLogWriterOnce(t *testing.T) {
	for _, builder := range allBuilders {
		t.Run(builder.command, func(t *testing.T) {
			pinHermeticEnv(t)
			logger := useInMemoryFileLogger(t)

			_, cleanup, err := builder.build(newBundleInputs())
			requireAssembled(t, cleanup, err)

			if err := cleanup(); err != nil {
				t.Errorf("cleanup returned %v, want nil", err)
			}
			if logger.closes != 1 {
				t.Errorf("log writer closed %d times, want 1", logger.closes)
			}
		})
	}
}

func TestBundleCleanup_ReportsCloseError(t *testing.T) {
	pinHermeticEnv(t)
	logger := useInMemoryFileLogger(t)
	logger.closeErr = errBoom

	_, cleanup, err := applyBuilder.build(newBundleInputs())
	requireAssembled(t, cleanup, err)

	if err := cleanup(); !errors.Is(err, errBoom) {
		t.Errorf("cleanup returned %v, want the close error", err)
	}
}

func TestBundleCleanup_SecondCallDoesNotPanic(t *testing.T) {
	pinHermeticEnv(t)
	logger := useInMemoryFileLogger(t)

	_, cleanup, err := applyBuilder.build(newBundleInputs())
	requireAssembled(t, cleanup, err)

	_ = cleanup()
	_ = cleanup()

	// Not idempotent: each call reaches the writer again, which with a real
	// file reports "already closed".
	if logger.closes != 2 {
		t.Errorf("log writer closed %d times across two cleanup calls, want 2", logger.closes)
	}
}
