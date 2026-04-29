# Spec: Traefik Routing Backend (Phase 9)

## Status
Pending

## Goal

When an Application declares `spec.routing.domain`, shrine writes a Traefik v3 dynamic config file to the gateway via SSH and removes it on teardown.

## Context

The `RoutingBackend` interface already exists in `internal/engine/backends.go`:

```go
type WriteRouteOp struct {
    Team        string
    Domain      string
    ServiceName string
    ServicePort int
    PathPrefix  string
}

type RoutingBackend interface {
    WriteRoute(op WriteRouteOp) error
    RemoveRoute(team string, host string) error
}
```

The dry-run backend (`internal/engine/dryrun/`) already implements it (prints ops). The real implementation is missing.

The engine holds backends as optional interfaces. Nil backends are skipped. Dry-run is just a different set of implementations wired in at startup — no special-casing in the engine itself.

## Acceptance Criteria

- [ ] Given an Application with `spec.routing.domain` set, `shrine deploy` writes a valid Traefik v3 dynamic config YAML to `/opt/traefik/config/` on the gateway (or the path from `traefik.configDir` in config.yml)
- [ ] The generated file is named `shrine-<team>-<sanitized-domain>.yml`
- [ ] `shrine teardown <team>` removes the file from the gateway
- [ ] `shrine deploy --dry-run` prints the route op without connecting to the gateway
- [ ] When gateway config is absent from `config.yml`, the engine skips routing silently (nil backend fallback)
- [ ] Connection uses key-based SSH auth; key path is read from `config.yml`

## Generated File Shape

```yaml
http:
  routers:
    shrine-team-a-hello-api:
      rule: "Host(`hello-api.home.lab`) && PathPrefix(`/hello-api`)"
      service: team-a-hello-api
      entryPoints:
        - web
  services:
    team-a-hello-api:
      loadBalancer:
        servers:
          - url: "http://team-a.hello-api:8080"
```

Naming conventions:
- Router name: `shrine-<team>-<service>`
- Service name: `<team>-<service>`
- Container URL: `http://<owner>.<name>:<port>` — Traefik reaches containers via `shrine.platform`, so the address uses the Docker DNS name

## Implementation Steps

### 1. Traefik YAML generator

Given a `WriteRouteOp`, produce the Traefik v3 dynamic config YAML. No templating engine needed — just Go structs marshaled with `gopkg.in/yaml.v3`.

File naming: `shrine-<team>-<sanitized-domain>.yml` in the config dir.

### 2. SSH executor

Connect to the gateway over SSH and write/delete files in the Traefik config dir.

- **Auth:** key-based (read private key path from `config.yml`). Agent forwarding adds a dependency on a running agent; key-based is simpler for a homelab tool.
- **File write approach:** `sftp` library (`github.com/pkg/sftp`) rather than `ssh.Session` + `cat > file`. SFTP avoids shell quoting issues and has no exec dependency. Add the dependency.
- **No host key verification needed** for homelab use — `ssh.InsecureIgnoreHostKey()` is acceptable here (document it as a known tradeoff).

### 3. Real `RoutingBackend` implementation

New package: `internal/engine/local/traefik/`.

- `WriteRoute(op)` — generates YAML, pushes via SFTP
- `RemoveRoute(team, host)` — deletes the file via SFTP

### 4. Wire into engine

`NewLocalEngine` in `internal/engine/local/local_engine.go` currently sets `Routing: nil`. Wire in the real backend when gateway config is present; fall back to nil (skip) when it isn't. The config struct already has a gateway IP field — extend it with:

```yaml
gateway:
  host: "192.168.1.208"
  user: "admin"
  sshKeyPath: "~/.ssh/id_ed25519"
traefik:
  configDir: "/opt/traefik/config/"  # default
```

### 5. Teardown

`engine.Engine.ExecuteTeardown` already calls `engine.Routing.RemoveRoute(team, host)` if `Routing != nil`. Implement `RemoveRoute` to delete the file over SFTP.

### 6. Dry-run output

The existing dry-run routing backend prints `[ROUTE] WriteRoute: domain=... → service:port`. This is sufficient for Phase 9. A `--verbose` flag that prints the full YAML is a nice-to-have, not a gate.

## Open Questions

- **TLS entrypoints:** Defer until there's a real cert manager. `web` only for now.
- **`--dry-run --verbose` full YAML output:** Nice-to-have; not the phase gate.
- **Sanitized domain naming:** Define the sanitization rule for domain → filename segment (e.g. replace `.` with `-`).

## Out of Scope for This Phase

- DNS record value (still `[IP_ADDRESS]`) — that's Phase 10 (AdGuard backend)
- TLS / HTTPS entrypoints
