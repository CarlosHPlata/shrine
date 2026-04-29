# Research: Traefik Gateway Plugin

**Branch**: `001-traefik-gateway-plugin` | **Date**: 2026-04-29

## Decision 1: Traefik Configuration Strategy

**Decision**: Use the Traefik **file provider** with two generated files per deployment: one static config (`traefik.yml`) enabling entrypoints + file provider + optional dashboard, and one dynamic config file per app (`{team}-{name}.yml`) containing the HTTP router and service rule.

**Rationale**: The file provider is the simplest Traefik dynamic config mechanism that requires no sidecar or API polling. Shrine writes YAML files; Traefik watches the directory. This is fully decoupled — Traefik doesn't need to know about shrine internals.

**Alternatives considered**:
- Docker labels provider: requires Docker socket access inside Traefik and tightly couples routing config to container lifecycle. Rejected — shrine already manages container lifecycle separately.
- Consul/etcd provider: requires additional infrastructure. Rejected — YAGNI.

---

## Decision 2: Routing Directory Layout

**Decision**: Shrine generates two categories of files in `routing-dir` (default `{specsDir}/traefik/`):
- `traefik.yml` — static config (generated once per deploy; overwritten each time)
- `dynamic/` subdirectory — one YAML file per routable app (`{team}-{name}.yml`), overwritten on each deploy

Operator-added files anywhere in `routing-dir` outside of `dynamic/` and `traefik.yml` are preserved (FR-009). Operator files inside `dynamic/` with names not matching shrine's `{team}-{name}.yml` pattern are also preserved.

**Generated static config shape** (`traefik.yml`):
```yaml
entryPoints:
  web:
    address: ":{port}"        # from TraefikPluginConfig.Port, default 80

api:                          # only when dashboard.port is set
  dashboard: true

providers:
  file:
    directory: /etc/traefik/dynamic
    watch: true

# dashboard entrypoint added only when dashboard.port is set
entryPoints:
  traefik:
    address: ":{dashboard.port}"
```

**Generated dynamic route shape** (`dynamic/{team}-{name}.yml`):
```yaml
http:
  routers:
    {team}-{name}:
      rule: "Host(`{domain}`) && PathPrefix(`{pathPrefix}`)"
      service: {team}-{name}
      entryPoints:
        - web
  services:
    {team}-{name}:
      loadBalancer:
        servers:
          - url: "http://{team}.{name}:{port}"
```

**Rationale**: Per-app files allow shrine to add/update/remove individual app routes without touching other apps' configs. The `dynamic/` subdirectory cleanly separates generated from operator files.

---

## Decision 3: Traefik Dashboard Basic Auth

**Decision**: Basic auth for the dashboard is configured via Traefik middleware in the static config using the `htpasswd` format for the password (`htpasswd -nb username password`). Shrine generates the hashed password at config generation time using `golang.org/x/crypto/bcrypt` (already available in Go standard-adjacent packages) formatted as a Traefik-compatible htpasswd entry.

**Generated static config addition** (when dashboard is enabled):
```yaml
http:
  middlewares:
    dashboard-auth:
      basicAuth:
        users:
          - "{username}:{htpasswd-hashed-password}"
  routers:
    dashboard:
      rule: "Host(`traefik.shrine.local`) || PathPrefix(`/dashboard`) || PathPrefix(`/api`)"
      service: api@internal
      middlewares:
        - dashboard-auth
      entryPoints:
        - traefik
```

**Rationale**: Traefik's built-in basicAuth middleware is the standard approach; no external auth proxy needed. `bcrypt` for password hashing is the Traefik-recommended format.

**Alternatives considered**:
- Storing plain-text password in static config: insecure. Rejected.
- Digest auth: not supported by Traefik basicAuth middleware. N/A.

---

## Decision 4: Docker Restart Policy

**Decision**: Set `RestartPolicy` to `"always"` in `container.HostConfig.RestartPolicy` using the Docker SDK type `container.RestartPolicy{Name: container.RestartPolicyAlways}`.

**Implementation**: Add `RestartPolicy string` to `engine.CreateContainerOp`. In `dockercontainer.createFreshContainer`, map the string to `container.RestartPolicy{Name: container.RestartPolicyMode(op.RestartPolicy)}`. Empty string means no restart policy (existing behaviour).

**Rationale**: `always` matches FR-015. Using a string field keeps `CreateContainerOp` backend-agnostic; the Docker backend does the translation.

---

## Decision 5: Config-Dir as Bind Mount

**Decision**: Mount `config-dir` as a Docker **bind mount** (not a named volume) to `/etc/traefik` inside the Traefik container. Bind mount lets shrine write files to the host path and have Traefik see them immediately via the file provider's `watch: true`.

**Implementation**: Add `BindMounts []BindMount` to `engine.CreateContainerOp` where `BindMount{Source string, Target string}`. In `dockercontainer.buildMounts`, append `mount.Mount{Type: mount.TypeBind, Source: v.Source, Target: v.Target}` for each bind mount. Existing `Volumes` (named Docker volumes) are unaffected.

**Rationale**: A named Docker volume can't be pre-populated by shrine before container creation. A bind mount maps a host directory directly, allowing shrine to write config files before starting Traefik.

---

## Decision 6: Engine Routing Gate Change

**Decision**: Change `engine.go` line 164 from:
```go
if application.Spec.Routing.Domain != "" && engine.Routing != nil {
```
to:
```go
if application.Spec.Routing.Domain != "" && application.Spec.Networking.ExposeToPlatform && engine.Routing != nil {
```

**Rationale**: FR-006 requires that only apps with BOTH a non-empty domain AND `ExposeToPlatform: true` get routing rules generated. This is a one-line correctness fix in the engine — not a new backend.

**Impact**: Existing deployments with `Routing.Domain` set but `ExposeToPlatform: false` will stop receiving routing config writes. This is the correct behaviour per spec and was previously an undefined/no-op case (since `engine.Routing` was always `nil`).

---

## Decision 7: Traefik Container Identity

**Decision**: The Traefik container is named `shrine.platform.traefik` and is associated with team `platform` and name `traefik` in the `DeploymentStore`. It attaches only to the platform network (`shrine.platform`), not to any team network.

**Rationale**: Traefik is a platform-level service, not team-scoped. Using `platform` as the team pseudo-namespace fits the existing `containerName(team, name)` → `shrine.{team}.{name}` naming convention without needing special cases.

---

## Decision 8: Port Bindings

**Decision**: Add `PortBindings []PortBinding` to `CreateContainerOp` where `PortBinding{HostPort string, ContainerPort string, Protocol string}`. The Traefik plugin sets:
- `{HostPort: strconv.Itoa(cfg.Port), ContainerPort: strconv.Itoa(cfg.Port), Protocol: "tcp"}`
- If dashboard: `{HostPort: strconv.Itoa(cfg.Dashboard.Port), ContainerPort: strconv.Itoa(cfg.Dashboard.Port), Protocol: "tcp"}`

In `dockercontainer.createFreshContainer`, populate `HostConfig.PortBindings` and `ContainerConfig.ExposedPorts` from `op.PortBindings`.

**Rationale**: Traefik must bind host ports to receive external traffic. Existing containers don't use port bindings (they communicate via Docker networks), so this is a purely additive field.
