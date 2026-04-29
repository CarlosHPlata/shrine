package traefik

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/CarlosHPlata/shrine/internal/engine"
)

type RoutingBackend struct {
	routingDir string
}

func (r *RoutingBackend) dynamicDir() string {
	return filepath.Join(r.routingDir, "dynamic")
}

func (r *RoutingBackend) WriteRoute(op engine.WriteRouteOp) error {
	if err := os.MkdirAll(r.dynamicDir(), 0o755); err != nil {
		return fmt.Errorf("traefik routing: creating dynamic dir: %w", err)
	}

	name := fmt.Sprintf("%s-%s", op.Team, op.ServiceName)
	rule := fmt.Sprintf("Host(`%s`)", op.Domain)
	if op.PathPrefix != "" {
		rule = fmt.Sprintf("Host(`%s`) && PathPrefix(`%s`)", op.Domain, op.PathPrefix)
	}

	doc := struct {
		HTTP httpConfig `yaml:"http"`
	}{
		HTTP: httpConfig{
			Routers: map[string]router{
				name: {
					Rule:        rule,
					Service:     name,
					EntryPoints: []string{"web"},
				},
			},
			Services: map[string]service{
				name: {
					LoadBalancer: loadBalancer{
						Servers: []server{
							{URL: fmt.Sprintf("http://%s.%s:%d", op.Team, op.ServiceName, op.ServicePort)},
						},
					},
				},
			},
		},
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal traefik route: %w", err)
	}

	return os.WriteFile(filepath.Join(r.dynamicDir(), routeFileName(op.Team, op.ServiceName)), data, 0o644)
}

func (r *RoutingBackend) RemoveRoute(team string, host string) error {
	path := filepath.Join(r.dynamicDir(), routeFileName(team, host))
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("traefik routing: removing %s: %w", path, err)
	}
	return nil
}
