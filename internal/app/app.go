// Package app is the composition root for Shrine's CLI commands.
//
// Each command-shaped scenario (deploy, apply, teardown) gets a Build*Bundle
// constructor that returns a fully-wired dependency set plus a cleanup func.
// Handlers in internal/handler/ consume these bundles and contain only
// request-shaped logic — no plugin / engine / observer construction.
package app

import (
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/engine/local"
	"github.com/CarlosHPlata/shrine/internal/plugins/secrets"
	"github.com/CarlosHPlata/shrine/internal/state"
)

// DeployBundle is the dependency set passed to handler.Deploy.
type DeployBundle struct {
	Out              io.Writer
	Cfg              *config.Config
	Store            *state.Store
	Paths            *config.Paths
	SpecsDir         string
	Observer         engine.Observer
	Vault            secrets.SecretsPlugin
	ContainerBackend engine.ContainerBackend
	Routing          engine.RoutingBackend
	Engine           *engine.Engine
}

// TeardownBundle is the dependency set passed to handler.Teardown.
type TeardownBundle struct {
	Out      io.Writer
	Cfg      *config.Config
	Store    *state.Store
	Paths    *config.Paths
	SpecsDir string
	Observer engine.Observer
	Routing  engine.RoutingBackend
	Engine   *engine.Engine
}

// ApplyBundle is the dependency set passed to handler.ApplySingle.
type ApplyBundle struct {
	Out      io.Writer
	Cfg      *config.Config
	Store    *state.Store
	Paths    *config.Paths
	Observer engine.Observer
	Vault    secrets.SecretsPlugin
	Engine   *engine.Engine
}

// BuildApplyBundle composes the dependency graph for `shrine apply --file`.
//
// On success the returned cleanup func is non-nil and idempotent — callers
// MUST defer it. On failure all three return values are zero; partial state
// has been unwound internally.
func BuildApplyBundle(cfg *config.Config, store *state.Store, paths *config.Paths, out io.Writer) (*ApplyBundle, func() error, error) {
	if err := cfg.ValidateRegistries(); err != nil {
		return nil, nil, fmt.Errorf("validating registries: %w", err)
	}

	observer, closeObserver, err := newObserverPair(out, paths)
	if err != nil {
		return nil, nil, fmt.Errorf("observer: %w", err)
	}

	vault, err := newVault(cfg)
	if err != nil {
		_ = closeObserver()
		return nil, nil, fmt.Errorf("vault: %w", err)
	}

	deployEngine, err := newLocalEngine(local.EngineOptions{
		Store:      store,
		Registries: cfg.Registries,
		Observer:   observer,
		Vault:      vault,
	})
	if err != nil {
		_ = closeObserver()
		return nil, nil, fmt.Errorf("engine: %w", err)
	}

	return &ApplyBundle{
		Out:      out,
		Cfg:      cfg,
		Store:    store,
		Paths:    paths,
		Observer: observer,
		Vault:    vault,
		Engine:   deployEngine,
	}, joinCleanup(closeObserver), nil
}

// BuildDeployBundle composes the dependency graph for `shrine deploy`.
//
// Sequence mirrors the historical handler.Deploy wiring: validate registries,
// resolve specsDir, observers, container backend, Traefik plugin, vault,
// routing backend, local engine.
func BuildDeployBundle(cfg *config.Config, store *state.Store, paths *config.Paths, manifestDir string, out io.Writer) (*DeployBundle, func() error, error) {
	if err := cfg.ValidateRegistries(); err != nil {
		return nil, nil, fmt.Errorf("validating registries: %w", err)
	}

	specsDir, _ := cfg.ResolveSpecsDir(manifestDir)

	observer, closeObserver, err := newObserverPair(out, paths)
	if err != nil {
		return nil, nil, fmt.Errorf("observer: %w", err)
	}

	containerBackend, err := newContainerBackend(store, cfg.Registries, observer)
	if err != nil {
		_ = closeObserver()
		return nil, nil, fmt.Errorf("container backend: %w", err)
	}

	plugin, err := newTraefikPlugin(cfg, containerBackend, specsDir, observer)
	if err != nil {
		_ = closeObserver()
		return nil, nil, fmt.Errorf("traefik: %w", err)
	}

	vault, err := newVault(cfg)
	if err != nil {
		_ = closeObserver()
		return nil, nil, fmt.Errorf("vault: %w", err)
	}

	routing, err := routingFromPlugin(plugin)
	if err != nil {
		_ = closeObserver()
		return nil, nil, fmt.Errorf("routing: %w", err)
	}

	deployEngine, err := newLocalEngine(local.EngineOptions{
		Store:      store,
		Registries: cfg.Registries,
		Observer:   observer,
		Routing:    routing,
		Vault:      vault,
	})
	if err != nil {
		_ = closeObserver()
		return nil, nil, fmt.Errorf("engine: %w", err)
	}

	return &DeployBundle{
		Out:              out,
		Cfg:              cfg,
		Store:            store,
		Paths:            paths,
		SpecsDir:         specsDir,
		Observer:         observer,
		Vault:            vault,
		ContainerBackend: containerBackend,
		Routing:          routing,
		Engine:           deployEngine,
	}, joinCleanup(closeObserver), nil
}

// BuildTeardownBundle composes the dependency graph for `shrine teardown`.
//
// Sequence mirrors handler.Teardown: observers, Traefik plugin (with nil
// container backend — teardown does not push images), routing backend,
// local engine. No vault: teardown does not resolve secrets.
func BuildTeardownBundle(cfg *config.Config, store *state.Store, paths *config.Paths, out io.Writer) (*TeardownBundle, func() error, error) {
	specsDir, _ := cfg.ResolveSpecsDir("")

	observer, closeObserver, err := newObserverPair(out, paths)
	if err != nil {
		return nil, nil, fmt.Errorf("observer: %w", err)
	}

	plugin, err := newTraefikPlugin(cfg, nil, specsDir, observer)
	if err != nil {
		_ = closeObserver()
		return nil, nil, fmt.Errorf("traefik: %w", err)
	}

	routing, err := routingFromPlugin(plugin)
	if err != nil {
		_ = closeObserver()
		return nil, nil, fmt.Errorf("routing: %w", err)
	}

	deployEngine, err := newLocalEngine(local.EngineOptions{
		Store:      store,
		Registries: cfg.Registries,
		Observer:   observer,
		Routing:    routing,
	})
	if err != nil {
		_ = closeObserver()
		return nil, nil, fmt.Errorf("engine: %w", err)
	}

	return &TeardownBundle{
		Out:      out,
		Cfg:      cfg,
		Store:    store,
		Paths:    paths,
		SpecsDir: specsDir,
		Observer: observer,
		Routing:  routing,
		Engine:   deployEngine,
	}, joinCleanup(closeObserver), nil
}

// ValidateTraefikConfig is a validation-only entry point for the Traefik
// plugin config, used by handler.DryRun. It constructs the plugin with nil
// dependencies purely to exercise its config validation, then discards it.
func ValidateTraefikConfig(cfg *config.Config) error {
	_, err := newTraefikPlugin(cfg, nil, "", nil)
	return err
}
