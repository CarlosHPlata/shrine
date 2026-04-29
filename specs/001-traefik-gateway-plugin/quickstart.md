# Quickstart: Traefik Gateway Plugin

## Minimal configuration

Add a `plugins.gateway.traefik` section to your shrine config file (`~/.config/shrine/config.yml` or wherever `--config` points):

```yaml
specsDir: /path/to/your/specs

plugins:
  gateway:
    traefik:
      routing-dir: /path/to/your/traefik
```

Run a normal deploy:

```bash
shrine deploy --state-dir /path/to/your/state --config-dir /path/to/your/config
```

Shrine will:
1. Validate the plugin config.
2. Create `/path/to/your/specs/traefik` if it does not exist based on the config, either by using the default specs directory or the one specified in the plugin traefik config.
3. Generate `traefik.yml` (static config) and `dynamic/{team}-{name}.yml` files for each app with both `Routing.Domain` set and `ExposeToPlatform: true`.
4. Deploy all apps and resources as usual.
5. Start the Traefik container (`shrine.platform.traefik`) on the platform network.

---

## Full configuration (all options)

```yaml
specsDir: /path/to/your/specs # Where the specs for shrine team, app and resources live
plugins:
  gateway:
    traefik:
      image: traefik:v3.7.0-rc.2   # optional — this is the default
      routing-dir: /path/to/your/traefik          # optional — defaults to {specsDir}/traefik/
      port: 80                       # optional — Traefik HTTP entrypoint port, default 80

      dashboard:
        port: 8080                   # enables the dashboard on this port
        username: admin              # required when dashboard.port is set
        password: s3cr3t             # required when dashboard.port is set
```

Deploying without `dashboard.port` credentials set will fail immediately with:

```
[shrine] Error: traefik plugin: dashboard.port is set but username and password are required
```

---

## Example app manifest that gets a Traefik route

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: my-api
  owner: my-team
spec:
  image: my-api:1.0.0
  port: 8080
  routing:
    domain: my-api.shrine.lab
    pathPrefix: /api
  networking:
    exposeToPlatform: true    # ← both this AND routing.domain must be set
```

The generated dynamic config at `{routing-dir}/dynamic/my-team-my-api.yml`:

```yaml
http:
  routers:
    my-team-my-api:
      rule: "Host(`my-api.shrine.lab`) && PathPrefix(`/api`)"
      service: my-team-my-api
      entryPoints:
        - web
  services:
    my-team-my-api:
      loadBalancer:
        servers:
          - url: "http://my-team.my-api:8080"
```

---

## Opting out

Remove or leave empty the `plugins.gateway.traefik` section. Shrine skips all plugin steps silently — no Traefik container is started, no config files are generated.

---

## Dry run

```bash
shrine deploy --dry-run --config-dir /path/to/your/config --state-dir /path/to/your/state
```

Plugin validation still runs (invalid config fails even in dry-run). No config files are written and no Traefik container is started.
