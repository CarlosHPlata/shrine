package traefik

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/CarlosHPlata/shrine/internal/engine"
)

var writeFileFn = os.WriteFile
var mkdirAllFn = os.MkdirAll
var removeFileFn = os.Remove

type RoutingBackend struct {
	routingDir string
	observer   engine.Observer
}

func (r *RoutingBackend) dynamicDir() string {
	return filepath.Join(r.routingDir, "dynamic")
}

func buildRouterRule(host, pathPrefix string) string {
	if pathPrefix == "" {
		return fmt.Sprintf("Host(`%s`)", host)
	}
	return fmt.Sprintf("Host(`%s`) && PathPrefix(`%s`)", host, pathPrefix)
}

func stripMiddlewareName(team, service string, aliasIndex int) string {
	return fmt.Sprintf("%s-%s-strip-%d", team, service, aliasIndex)
}

func (r *RoutingBackend) WriteRoute(op engine.WriteRouteOp) error {
	if err := mkdirAllFn(r.dynamicDir(), 0o755); err != nil {
		return fmt.Errorf("traefik routing: creating dynamic dir: %w", err)
	}

	path := filepath.Join(r.dynamicDir(), routeFileName(op.Team, op.ServiceName))
	present, err := isPathPresent(path)
	if err != nil {
		r.observer.OnEvent(engine.Event{
			Name:   "gateway.route.stat_error",
			Status: engine.StatusWarning,
			Fields: map[string]string{
				"team":  op.Team,
				"name":  op.ServiceName,
				"path":  path,
				"error": err.Error(),
			},
		})
		return nil
	}
	if present {
		r.observer.OnEvent(engine.Event{
			Name:   "gateway.route.preserved",
			Status: engine.StatusInfo,
			Fields: map[string]string{
				"team": op.Team,
				"name": op.ServiceName,
				"path": path,
			},
		})
		return nil
	}

	name := fmt.Sprintf("%s-%s", op.Team, op.ServiceName)

	routers := map[string]router{
		name: {
			Rule:        buildRouterRule(op.Domain, op.PathPrefix),
			Service:     name,
			EntryPoints: []string{"web"},
		},
	}

	mids := map[string]middleware{}

	for i, ar := range op.AdditionalRoutes {
		aliasKey := fmt.Sprintf("%s-alias-%d", name, i)
		r := router{
			Rule:        buildRouterRule(ar.Host, ar.PathPrefix),
			Service:     name,
			EntryPoints: []string{"web"},
		}
		if ar.StripPrefix && ar.PathPrefix != "" {
			midKey := stripMiddlewareName(op.Team, op.ServiceName, i)
			mids[midKey] = middleware{StripPrefix: &stripPrefix{Prefixes: []string{ar.PathPrefix}}}
			r.Middlewares = []string{midKey}
		}
		routers[aliasKey] = r
	}

	cfg := httpConfig{
		Routers: routers,
		Services: map[string]service{
			name: {
				LoadBalancer: loadBalancer{
					Servers: []server{
						{URL: fmt.Sprintf("http://%s.%s:%d", op.Team, op.ServiceName, op.ServicePort)},
					},
				},
			},
		},
	}
	if len(mids) > 0 {
		cfg.Middlewares = mids
	}

	doc := struct {
		HTTP httpConfig `yaml:"http"`
	}{HTTP: cfg}

	data, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal traefik route: %w", err)
	}

	if err := writeFileFn(path, data, 0o644); err != nil {
		return err
	}
	r.observer.OnEvent(engine.Event{
		Name:   "gateway.route.generated",
		Status: engine.StatusInfo,
		Fields: map[string]string{
			"team": op.Team,
			"name": op.ServiceName,
			"path": path,
		},
	})
	return nil
}

func (r *RoutingBackend) RemoveRoute(team string, host string) error {
	path := filepath.Join(r.dynamicDir(), routeFileName(team, host))
	present, err := isPathPresent(path)
	if err != nil {
		r.observer.OnEvent(engine.Event{
			Name:   "gateway.route.stat_error",
			Status: engine.StatusWarning,
			Fields: map[string]string{
				"team":  team,
				"name":  host,
				"path":  path,
				"error": err.Error(),
			},
		})
		return nil
	}
	if present {
		r.observer.OnEvent(engine.Event{
			Name:   "gateway.route.orphan",
			Status: engine.StatusWarning,
			Fields: map[string]string{
				"team": team,
				"name": host,
				"path": path,
			},
		})
		return nil
	}
	return nil
}
