package traefik

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/CarlosHPlata/shrine/internal/config"
)

func generateStaticConfig(cfg *config.TraefikPluginConfig, routingDir string) error {
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

	return os.WriteFile(filepath.Join(routingDir, "traefik.yml"), data, 0o644)
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
