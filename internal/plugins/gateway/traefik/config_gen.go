package traefik

import (
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
)

var (
	lstatFn    = os.Lstat
	readFileFn = os.ReadFile
)

// isPathPresent treats any directory entry at path as present, including
// symlinks and non-regular files. Lstat (not Stat) is used so a broken
// symlink still counts as present and is left untouched.
func isPathPresent(path string) (bool, error) {
	if _, err := lstatFn(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking path %q: %w", path, err)
	}
	return true, nil
}

func isStaticConfigPresent(routingDir string) (bool, error) {
	path := filepath.Join(routingDir, "traefik.yml")
	present, err := isPathPresent(path)
	if err != nil {
		return false, fmt.Errorf("traefik plugin: checking traefik.yml at %q: %w", path, err)
	}
	return present, nil
}

func generateStaticConfig(cfg *config.TraefikPluginConfig, routingDir string, observer engine.Observer) error {
	path := filepath.Join(routingDir, "traefik.yml")
	present, err := isStaticConfigPresent(routingDir)
	if err != nil {
		return err
	}
	if present {
		emitLegacyHTTPBlockSignal(path, routingDir, observer)
		emitTLSPortNoWebsecureSignal(path, cfg, observer)
		observer.OnEvent(engine.Event{
			Name:   "gateway.config.preserved",
			Status: engine.StatusInfo,
			Fields: map[string]string{"path": path},
		})
		return nil
	}

	port := cfg.Port
	if port == 0 {
		port = defaultPort
	}

	spec := staticConfig{
		EntryPoints: map[string]entryPoint{
			"web": {Address: fmt.Sprintf(":%d", port)},
		},
		Providers: providersConfig{
			File: fileProvider{Directory: "/etc/traefik/dynamic", Watch: true},
		},
	}

	if cfg.Dashboard != nil && cfg.Dashboard.Port > 0 {
		spec.EntryPoints["traefik"] = entryPoint{Address: fmt.Sprintf(":%d", cfg.Dashboard.Port)}
		spec.API = &apiConfig{Dashboard: true}
	}
	if cfg.TLSPort > 0 {
		spec.EntryPoints["websecure"] = entryPoint{Address: ":443"}
	}

	data, err := yaml.Marshal(spec)
	if err != nil {
		return fmt.Errorf("marshal traefik static config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("traefik plugin: writing traefik.yml: %w", err)
	}
	observer.OnEvent(engine.Event{
		Name:   "gateway.config.generated",
		Status: engine.StatusInfo,
		Fields: map[string]string{"path": path},
	})
	return nil
}

func generateDashboardDynamicConfig(cfg *config.TraefikPluginConfig, routingDir string, observer engine.Observer) error {
	path := filepath.Join(routingDir, "dynamic", dashboardDynamicFileName())
	present, err := isPathPresent(path)
	if err != nil {
		return fmt.Errorf("traefik plugin: checking dashboard dynamic file at %q: %w", path, err)
	}
	if present {
		observer.OnEvent(engine.Event{
			Name:   "gateway.dashboard.preserved",
			Status: engine.StatusInfo,
			Fields: map[string]string{"path": path},
		})
		return nil
	}

	doc := dashboardDynamicDoc{
		HTTP: httpConfig{
			Middlewares: map[string]middleware{
				"dashboard-auth": {
					BasicAuth: &basicAuth{
						Users: []string{htpasswdEntry(cfg.Dashboard.Username, cfg.Dashboard.Password)},
					},
				},
			},
			Routers: map[string]router{
				"dashboard": {
					Rule:        "PathPrefix(`/dashboard`) || PathPrefix(`/api`)",
					Service:     "api@internal",
					EntryPoints: []string{"traefik"},
					Middlewares: []string{"dashboard-auth"},
				},
			},
		},
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("traefik plugin: marshal dashboard dynamic config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("traefik plugin: writing dashboard dynamic file: %w", err)
	}
	observer.OnEvent(engine.Event{
		Name:   "gateway.dashboard.generated",
		Status: engine.StatusInfo,
		Fields: map[string]string{"path": path},
	})
	return nil
}

// hasLegacyDashboardHTTPBlock returns whether path contains a top-level `http:`
// section. It is meant to flag the artefact left behind by an earlier buggy
// version of this plugin, which emitted the dashboard router into the static
// config where Traefik silently drops it.
func hasLegacyDashboardHTTPBlock(path string) (bool, error) {
	data, err := readFileFn(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("traefik plugin: probing legacy http block at %q: %w", path, err)
	}
	var probe legacyHTTPProbe
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return false, fmt.Errorf("traefik plugin: probing legacy http block at %q: %w", path, err)
	}
	return probe.HTTP != nil, nil
}

func emitLegacyHTTPBlockSignal(staticPath, routingDir string, observer engine.Observer) {
	hit, err := hasLegacyDashboardHTTPBlock(staticPath)
	if err != nil {
		observer.OnEvent(engine.Event{
			Name:   "gateway.config.legacy_probe_error",
			Status: engine.StatusWarning,
			Fields: map[string]string{
				"path":  staticPath,
				"error": err.Error(),
			},
		})
		return
	}
	if !hit {
		return
	}
	dynamicPath := filepath.Join(routingDir, "dynamic", dashboardDynamicFileName())
	observer.OnEvent(engine.Event{
		Name:   "gateway.config.legacy_http_block",
		Status: engine.StatusWarning,
		Fields: map[string]string{
			"path": staticPath,
			"hint": fmt.Sprintf("Remove the top-level http: block from this file; the dashboard now lives in %s.", dynamicPath),
		},
	})
}

// htpasswdEntry produces an htpasswd line in the SHA1 format that Traefik
// basicAuth accepts: user:{SHA}base64(sha1(password)).
func htpasswdEntry(user, password string) string {
	sum := sha1.Sum([]byte(password))
	return fmt.Sprintf("%s:{SHA}%s", user, base64.StdEncoding.EncodeToString(sum[:]))
}

func routeFileName(team, name string) string {
	return fmt.Sprintf("%s-%s.yml", team, name)
}

func dashboardDynamicFileName() string {
	return "__shrine-dashboard.yml"
}

type dashboardDynamicDoc struct {
	HTTP httpConfig `yaml:"http"`
}

type legacyHTTPProbe struct {
	HTTP *yaml.Node `yaml:"http"`
}

// hasWebsecureEntrypoint returns whether path contains an entryPoints.websecure
// key. It is used to detect when a preserved traefik.yml is missing the
// websecure entrypoint while tlsPort is set (FR-008).
func hasWebsecureEntrypoint(path string) (bool, error) {
	data, err := readFileFn(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("traefik plugin: probing websecure entrypoint at %q: %w", path, err)
	}
	var probe websecureProbe
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return false, fmt.Errorf("traefik plugin: probing websecure entrypoint at %q: %w", path, err)
	}
	_, ok := probe.EntryPoints["websecure"]
	return ok, nil
}

func emitTLSPortNoWebsecureSignal(staticPath string, cfg *config.TraefikPluginConfig, observer engine.Observer) {
	if cfg.TLSPort == 0 {
		return
	}
	ok, err := hasWebsecureEntrypoint(staticPath)
	if err != nil {
		observer.OnEvent(engine.Event{
			Name:   "gateway.config.legacy_probe_error",
			Status: engine.StatusWarning,
			Fields: map[string]string{
				"path":  staticPath,
				"error": err.Error(),
			},
		})
		return
	}
	if ok {
		return
	}
	observer.OnEvent(engine.Event{
		Name:   "gateway.config.tls_port_no_websecure",
		Status: engine.StatusWarning,
		Fields: map[string]string{
			"path": staticPath,
			"hint": fmt.Sprintf(
				"tlsPort=%d publishes host port %d→443/tcp on the Traefik container, but this preserved traefik.yml has no entryPoints.websecure listening on :443. Add the entrypoint, or delete the file so Shrine regenerates it.",
				cfg.TLSPort, cfg.TLSPort,
			),
		},
	})
}

type websecureProbe struct {
	EntryPoints map[string]*yaml.Node `yaml:"entryPoints"`
}
