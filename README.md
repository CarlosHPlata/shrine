# Shrine

**Declarative homelab orchestration.** Define your infrastructure as YAML manifests, and Shrine handles Docker containers, Traefik routing, and DNS — all from a single CLI.

Shrine brings a Kubernetes-inspired declarative workflow to homelabs without the complexity of running an actual cluster. Write manifests, run `shrine deploy`, and your services are live with networking, routing, and DNS configured automatically.

---

## Features

- **Declarative YAML manifests** — Kubernetes-style `Application`, `Resource`, and `Team` kinds
- **Docker orchestration** — Container lifecycle managed via the official Docker Go SDK (no shell exec)
- **Automatic networking** — Each team gets an isolated `/24` bridge network; cross-team communication is opt-in
- **Traefik integration** — Route configs generated and pushed to your gateway over SSH
- **DNS management** — AdGuard DNS entries created and cleaned up automatically
- **Team-based access control** — Resources declare who can consume them; Shrine enforces it at deploy time
- **Quotas** — Limit apps, resources, and allowed resource types per team
- **Idempotent deploys** — Re-run `deploy` safely; Shrine reconciles state instead of duplicating containers
- **Dry-run mode** — Preview the full execution plan before touching anything
- **Local state management** — Subnet allocation, secret generation, and deployment tracking

## Quick Start

### Prerequisites

- Go 1.24+
- Docker running on your app server
- Traefik v3 on your gateway (file provider enabled)
- AdGuard DNS (optional, for automatic DNS entries)

### Install

```bash
go install github.com/CarlosHPlata/shrine@latest
```

Or build from source:

```bash
git clone https://github.com/CarlosHPlata/shrine.git
cd shrine
go build -o shrine .
```

### Usage

```bash
# Deploy a project
shrine deploy ./projects/team-a/

# Preview what would happen
shrine deploy ./projects/team-a/ --dry-run

# See exact operations (Docker calls, SSH writes, HTTP requests)
shrine deploy ./projects/team-a/ --dry-run --verbose

# Tear down a team's resources
shrine teardown team-a

# Check deployment status
shrine status
shrine status team-a

# Manage teams
shrine generate team team-a                  # scaffold a manifest YAML
shrine create team -f teams/team-a.yml       # register a single team in state
shrine apply teams                           # sync all teams/*.yml to state
shrine get teams                             # list all teams from state
shrine describe team team-a                  # show team details
shrine delete team team-a                    # remove from state
```

## Manifests

Shrine uses three manifest types, all following a familiar `apiVersion` / `kind` / `metadata` / `spec` structure.

### Application

A deployable container with routing and dependency injection:

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: hello-api
  owner: team-a
spec:
  image: hello-api
  port: 8080
  replicas: 1
  routing:
    domain: hello-api.home.lab
    pathPrefix: /hello-api
  dependencies:
    - kind: Resource
      name: hello-db
      owner: team-a
  env:
    - name: DATABASE_URL
      valueFrom: dependency.hello-db.url
    - name: NODE_ENV
      value: production
```

### Resource

A managed dependency (Postgres, RabbitMQ, Redis, etc.) with access control:

```yaml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: hello-db
  owner: team-a
  access:
    - team-b
spec:
  type: postgres
  version: "16"
  networking:
    exposeToplatform: false
```

### Team

A registered team space with quotas and permissions:

```yaml
apiVersion: shrine/v1
kind: Team
metadata:
  name: team-a
spec:
  displayName: "Team Alpha"
  contact: alice@example.com
  quotas:
    maxApps: 3
    maxResources: 5
    allowedResourceTypes:
      - postgres
      - rabbitmq
```

## Networking Model

| Concept | Details |
|---|---|
| **Team network** | Each team gets an isolated Docker bridge (`shrine.<owner>.private`) with an auto-assigned `/24` from `10.100.0.0/16` |
| **Platform network** | A shared network (`shrine.platform`, `10.200.0.0/16`) for cross-team communication |
| **Isolation by default** | Databases and caches live on the team's private network only |
| **Opt-in sharing** | Resources with `exposeToplatform: true` join both networks; consumers must be on the resource's `access` list |

## Dry-Run Output

Preview deployments before they happen:

```
$ shrine deploy ./projects/team-a/ --dry-run

[PLAN] team-a
  CREATE network shrine.team-a.private (10.100.3.0/24)
  CREATE container team-a.hello-db (postgres:16) on shrine.team-a.private
  CREATE container team-a.hello-api (hello-api:latest)
    ENV DATABASE_URL=postgres://postgres:<generated>@team-a.hello-db:5432/hello
  WRITE  /opt/traefik/config/team-a-hello-api.yml (via SSH)
  DNS    hello-api.home.lab → gateway
```

Add `--verbose` to see exact Docker API calls, SSH commands, and HTTP requests.

## Project Structure

```
shrine/
├── cmd/                   # CLI verb commands (cobra)
│   ├── root.go            # Root command, global flags
│   ├── generate.go        # shrine generate <resource> <name>
│   ├── create.go          # shrine create <resource> -f <path>
│   ├── apply.go           # shrine apply <resource>
│   ├── get.go             # shrine get <resource>
│   ├── describe.go        # shrine describe <resource> <name>
│   ├── delete.go          # shrine delete <resource> <name>
│   ├── deploy.go          # shrine deploy <path>
│   ├── teardown.go        # shrine teardown <team>
│   └── status.go          # shrine status [team]
├── internal/
│   ├── handler/           # Resource-specific business logic
│   ├── manifest/          # YAML parsing, validation, schema types
│   ├── config/            # Path resolution (XDG, FHS, .env)
│   ├── planner/           # Dependency graph, access checks, ordering
│   ├── executor/          # Executor interface (real + dry-run)
│   ├── docker/            # Docker SDK wrapper
│   ├── traefik/           # Traefik route config generation + deployment
│   ├── dns/               # AdGuard HTTP API client
│   └── state/             # Store interface, FileStore, subnet allocation
├── teams/                 # Team manifests (Git is source of truth)
├── main.go
├── go.mod
└── go.sum
```

## Configuration

Shrine resolves configuration and state directories dynamically:

| Directory | User (XDG) | Root (System) | Env Var | Flag |
|---|---|---|---|---|
| **Config** | `~/.config/shrine/` | `/etc/shrine/` | `SHRINE_CONFIG_DIR` | `--config-dir` |
| **State** | `~/.local/share/shrine/` | `/var/lib/shrine/` | `SHRINE_STATE_DIR` | `--state-dir` |

State tracks subnet allocations, team cache, generated secrets, and deployed resources.

## Contributing

Contributions are welcome! Please open an issue to discuss what you'd like to change before submitting a PR.

```bash
# Clone and build
git clone https://github.com/CarlosHPlata/shrine.git
cd shrine
go build ./...

# Run tests
go test ./...
```

## License

MIT
