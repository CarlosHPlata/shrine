package traefik

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/CarlosHPlata/shrine/internal/config"
	"github.com/CarlosHPlata/shrine/internal/engine"
)

var lstatFn = os.Lstat

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
		spec.HTTP = &httpConfig{
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
		}
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

// htpasswdEntry produces an htpasswd line in the SHA1 format that Traefik
// basicAuth accepts: user:{SHA}base64(sha1(password)).
func htpasswdEntry(user, password string) string {
	sum := sha1.Sum([]byte(password))
	return fmt.Sprintf("%s:{SHA}%s", user, base64.StdEncoding.EncodeToString(sum[:]))
}

func routeFileName(team, name string) string {
	return fmt.Sprintf("%s-%s.yml", team, name)
}
