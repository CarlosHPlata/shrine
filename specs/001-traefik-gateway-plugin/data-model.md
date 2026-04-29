# Data Model: Traefik Gateway Plugin

**Branch**: `001-traefik-gateway-plugin` | **Date**: 2026-04-29

## 1. Config Additions (`internal/config/config.go`)

### New types

```go
type PluginsConfig struct {
    Gateway GatewayPluginsConfig `yaml:"gateway,omitempty"`
}

type GatewayPluginsConfig struct {
    Traefik *TraefikPluginConfig `yaml:"traefik,omitempty"`
}

type TraefikPluginConfig struct {
    Image      string                  `yaml:"image,omitempty"`
    RoutingDir string                  `yaml:"routing-dir,omitempty"`
    Port       int                     `yaml:"port,omitempty"`
    Dashboard  *TraefikDashboardConfig `yaml:"dashboard,omitempty"`
}

type TraefikDashboardConfig struct {
    Port     int    `yaml:"port"`
    Username string `yaml:"username,omitempty"`
    Password string `yaml:"password,omitempty"`
}
```

### Extended `Config` struct

```go
type Config struct {
    Registries []RegistryConfig `yaml:"registries,omitempty"`
    SpecsDir   string           `yaml:"specsDir,omitempty"`
    TeamsDir   string           `yaml:"teamsDir,omitempty"`
    Plugins    PluginsConfig    `yaml:"plugins,omitempty"`  // ← new
}
```

### Validation rules

- `TraefikPluginConfig` is considered **active** when it is non-nil and at least one field is non-zero.
- `TraefikPluginConfig.Image` defaults to `"traefik:v3.7.0-rc.2"` when empty.
- `TraefikPluginConfig.RoutingDir` defaults to `{specsDir}/traefik/` when empty — shrine creates this subdirectory, not the specsDir root.
- `TraefikPluginConfig.Port` defaults to `80` when zero.
- `TraefikDashboardConfig.Port` MUST be > 0 when `Dashboard` is non-nil; enforced at validation time.
- `TraefikDashboardConfig.Username` and `Password` MUST both be non-empty when `Dashboard` is non-nil; enforced at validation time (FR-013).

---

## 2. Engine `CreateContainerOp` Extensions (`internal/engine/backends.go`)

### New value types

```go
type BindMount struct {
    Source string // absolute host path
    Target string // absolute container path
}

type PortBinding struct {
    HostPort      string // e.g., "80"
    ContainerPort string // e.g., "80"
    Protocol      string // "tcp" or "udp"
}
```

### Extended `CreateContainerOp`

```go
type CreateContainerOp struct {
    // existing fields unchanged
    Team             string
    Name             string
    Image            string
    Kind             string
    Network          string
    Env              []string
    Volumes          []VolumeMount
    ExposeToPlatform bool
    ImagePullPolicy  string
    // new optional fields
    RestartPolicy string        // e.g., "always"; empty = no restart policy
    BindMounts    []BindMount
    PortBindings  []PortBinding
}
```

---

## 3. Plugin Types (`internal/plugins/gateway/traefik/`)

### `plugin.go`

```go
// Plugin is the Traefik gateway plugin.
// It holds all state needed to validate config, generate files, and deploy Traefik.
type Plugin struct {
    cfg        *config.TraefikPluginConfig
    backend    engine.ContainerBackend
    routingDir string // resolved absolute path
}

func New(cfg *config.TraefikPluginConfig, backend engine.ContainerBackend, specsDir string) *Plugin

func (p *Plugin) isActive() bool
func (p *Plugin) Validate() error               // returns error if dashboard.port set without credentials
func (p *Plugin) RoutingBackend() engine.RoutingBackend
func (p *Plugin) Deploy() error                  // deploys Traefik container using p.backend
func (p *Plugin) resolvedImage() string          // returns cfg.Image or default
func (p *Plugin) resolvedPort() int              // returns cfg.Port or 80
func (p *Plugin) resolvedRoutingDir() string     // returns cfg.RoutingDir or {specsDir}/traefik/
func (p *Plugin) hasDashboard() bool
```

### `routing.go`

```go
// RoutingBackend implements engine.RoutingBackend by writing Traefik dynamic config files.
type RoutingBackend struct {
    routingDir string // absolute path to routing-dir/dynamic/
}

func (r *RoutingBackend) WriteRoute(op engine.WriteRouteOp) error
func (r *RoutingBackend) RemoveRoute(team string, host string) error
```

### `config_gen.go`

```go
// generateStaticConfig writes traefik.yml to routingDir.
func generateStaticConfig(cfg *config.TraefikPluginConfig, routingDir string) error

// routeFileName returns the deterministic filename for a given team+name pair.
func routeFileName(team, name string) string // → "{team}-{name}.yml"
```

---

## 4. State Interactions

The Traefik container is recorded in `DeploymentStore` via the existing `recordDeployment` call inside `DockerBackend.CreateContainer`. No new state schema is required — the existing deployment record (`Name`, `ConfigHash`, `ContainerID`) is sufficient.

**Container identity**:
- `Team`: `"platform"`
- `Name`: `"traefik"`
- Container name (Docker): `shrine.platform.traefik`
- Network: `shrine.platform` only (no team network)

---

## 5. Dry-Run Considerations

When `shrine deploy --dry-run` is run:
- Plugin validation MUST still run (fail fast on bad config, even in dry-run).
- Config file generation is skipped (dry-run produces no filesystem side effects).
- The Traefik container `Deploy()` call is skipped.
- The dry-run `RoutingBackend` (if a dry-run routing backend is added) prints route entries to stdout instead of writing files.

The existing `dryrun` engine package already has a `dry_run_routing.go` stub that will need implementing for the Traefik routing backend's dry-run counterpart.
