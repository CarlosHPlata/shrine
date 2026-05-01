package traefik

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
	"github.com/CarlosHPlata/shrine/internal/manifest"
)

const (
	defaultImage = "traefik:v3.7.0-rc.2"
	defaultPort  = 80

	containerTeam = "platform"
	containerName = "traefik"

	platformNetwork = "shrine.platform"
)

type Plugin struct {
	cfg      *config.TraefikPluginConfig
	backend  engine.ContainerBackend
	specsDir string
	observer engine.Observer
}

// New constructs and validates a Traefik plugin. It returns an error if the
// supplied config is invalid (e.g. dashboard.port without credentials), so
// callers don't need a separate Validate step.
func New(cfg *config.TraefikPluginConfig, backend engine.ContainerBackend, specsDir string, observer engine.Observer) (*Plugin, error) {
	if observer == nil {
		observer = engine.NoopObserver{}
	}
	p := &Plugin{cfg: cfg, backend: backend, specsDir: specsDir, observer: observer}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Plugin) isActive() bool {
	if p == nil || p.cfg == nil {
		return false
	}
	c := p.cfg
	if c.Image != "" || c.RoutingDir != "" || c.Port != 0 {
		return true
	}
	if c.Dashboard != nil {
		return true
	}
	return false
}

// IsActive exposes the active state to callers in other packages.
func (p *Plugin) IsActive() bool { return p.isActive() }

func (p *Plugin) hasDashboard() bool {
	return p.cfg != nil && p.cfg.Dashboard != nil && p.cfg.Dashboard.Port > 0
}

func (p *Plugin) hasCredentials() bool {
	if p.cfg == nil || p.cfg.Dashboard == nil {
		return false
	}
	return p.cfg.Dashboard.Username != "" && p.cfg.Dashboard.Password != ""
}

func (p *Plugin) validate() error {
	if !p.isActive() {
		return nil
	}
	if p.hasDashboard() && !p.hasCredentials() {
		return fmt.Errorf("traefik plugin: dashboard.port is set but username and password are required")
	}
	return nil
}

func (p *Plugin) resolvedImage() string {
	if p.cfg == nil || p.cfg.Image == "" {
		return defaultImage
	}
	return p.cfg.Image
}

func (p *Plugin) resolvedPort() int {
	if p.cfg == nil || p.cfg.Port == 0 {
		return defaultPort
	}
	return p.cfg.Port
}

func (p *Plugin) resolvedRoutingDir() (string, error) {
	routingDir, err := p.cfg.ResolveRoutingDir(filepath.Join(p.specsDir, "traefik"))
	if err != nil {
		return "", fmt.Errorf("traefik plugin: resolving routing directory: %w", err)
	}
	return routingDir, nil
}

func (p *Plugin) Deploy() error {
	if !p.isActive() {
		return nil
	}

	routingDir, err := p.resolvedRoutingDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(routingDir, 0o755); err != nil {
		return fmt.Errorf("traefik plugin: creating routing dir %q: %w", routingDir, err)
	}
	if err := os.MkdirAll(filepath.Join(routingDir, "dynamic"), 0o755); err != nil {
		return fmt.Errorf("traefik plugin: creating dynamic dir: %w", err)
	}

	if err := generateStaticConfig(p.cfg, routingDir, p.observer); err != nil {
		return err
	}

	op := engine.CreateContainerOp{
		Team:             containerTeam,
		Name:             containerName,
		Kind:             manifest.ApplicationKind,
		Image:            p.resolvedImage(),
		Network:          platformNetwork,
		ExposeToPlatform: false,
		ImagePullPolicy:  "IfNotPresent",
		RestartPolicy:    "always",
		BindMounts: []engine.BindMount{
			{Source: routingDir, Target: "/etc/traefik"},
		},
		PortBindings: p.portBindings(),
	}

	if err := p.backend.CreateContainer(op); err != nil {
		return fmt.Errorf("traefik plugin: starting traefik container: %w", err)
	}
	return nil
}

func (p *Plugin) portBindings() []engine.PortBinding {
	port := strconv.Itoa(p.resolvedPort())
	bindings := []engine.PortBinding{
		{HostPort: port, ContainerPort: port, Protocol: "tcp"},
	}
	if p.hasDashboard() {
		dp := strconv.Itoa(p.cfg.Dashboard.Port)
		bindings = append(bindings, engine.PortBinding{HostPort: dp, ContainerPort: dp, Protocol: "tcp"})
	}
	return bindings
}

func (p *Plugin) RoutingBackend() (engine.RoutingBackend, error) {
	routingDir, err := p.resolvedRoutingDir()
	if err != nil {
		return nil, err
	}
	return &RoutingBackend{routingDir: routingDir, observer: p.observer}, nil
}
