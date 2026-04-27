# Shrine

Shrine is a Go CLI that orchestrates homelab services via declarative YAML manifests. It manages Docker containers — all driven by `kind: Application` and `kind: Resource` files that intentionally mirror Kubernetes manifest conventions.

## Quick Start

```bash
go build -o shrine .

shrine deploy                              # deploy all manifests (uses specsDir from config)
shrine deploy --path ./manifests/          # override specsDir
shrine deploy --dry-run                    # print plan, no side effects
shrine apply -f ./manifests/my-app.yml    # deploy a single manifest
shrine apply teams                         # sync team manifests to state
shrine teardown team-a                     # remove all containers and network for a team
shrine status                              # show all deployed resources
shrine status app my-api                  # show status for a specific app
```

## CLI Reference

### shrine deploy
No positional path argument. Uses --path/-p flag or specsDir from config.yml. Examples: shrine deploy, shrine deploy --path ./manifests/, shrine deploy --dry-run

### shrine apply -f <file>
New command. Deploys a single manifest file. Kind is inferred from the YAML kind: field. Uses specsDir (or --path) as resolution context for valueFrom dependencies.

### shrine apply teams
Syncs team manifests to state. Uses --path/-p flag or specsDir from config (no longer defaults to .).

### shrine status app/resource <name>
Team is now an optional --team/-t flag, not a required positional argument. Shrine auto-searches all teams; use --team to disambiguate. Examples: shrine status app my-api, shrine status app my-api --team team-a, shrine status resource my-db

### shrine describe app/resource <name>
Same as status: team is now an optional --team flag, not required. Examples: shrine describe app my-api, shrine describe app my-api --team team-a

## Manifest Kinds

### Application

A deployable container. Declares its image, port, dependencies, environment, and routing.

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: hello-api
  owner: team-a
  access:
    - team-b          # teams allowed to consume this app's built-in outputs
spec:
  image: 192.168.1.206:8080/hello-api:latest
  port: 8080
  routing:
    domain: hello-api.home.lab
    pathPrefix: /hello-api
  dependencies:
    - kind: Resource
      name: hello-db
      owner: team-a
    - kind: Application
      name: auth-service
      owner: team-b    # cross-team: producer must set exposeToPlatform: true
  env:
    - name: DATABASE_URL
      valueFrom: resource.hello-db.url        # pull a resolved Resource output
    - name: AUTH_HOST
      valueFrom: application.auth-service.host  # built-in: <owner>.<name>
    - name: AUTH_PORT
      valueFrom: application.auth-service.port  # built-in: spec.port
    - name: AUTH_BASE
      template: "http://{{.AUTH_HOST}}:{{.AUTH_PORT}}"  # composed from sibling env names
    - name: NODE_ENV
      value: production                        # literal
  networking:
    exposeToPlatform: false   # true → joins shrine.platform for cross-team reachability
  volumes:
    - name: uploads
      mountPath: /app/uploads
```

Each `env` entry uses exactly one of `value` / `valueFrom` / `template`. `template` is Go `text/template`; it resolves in topological order so a template can reference a sibling env that was itself resolved from `valueFrom`.

Applications expose exactly two built-in outputs to other manifests: `host` (`<owner>.<name>`, the container DNS name) and `port` (`spec.port`). There is no `url` built-in — scheme composition is the consumer's job via `template`.

### Resource

A managed dependency (postgres, redis, rabbitmq, …). Declares an image and named outputs that apps can reference via `valueFrom`.

```yaml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: hello-db
  owner: team-a
  access:
    - team-b          # teams allowed to consume this resource
spec:
  type: postgres
  version: "16"
  image: postgres:16  # optional, defaults to type:version
  outputs:
    - name: host      # infrastructure-synthesized → team-a.hello-db (Docker container DNS name)
    - name: port
      value: "5432"
    - name: password
      generated: true # random secret, persisted across redeploys via SecretStore
    - name: url
      template: "postgres://postgres:{{.password}}@{{.host}}:{{.port}}/hello"
  networking:
    exposeToPlatform: false
  volumes:
    - name: data
      mountPath: /var/lib/postgresql/data
```

Output kinds: `value` (literal), `generated` (random secret), `template` (Go `text/template` referencing sibling outputs and built-ins `{{.team}}`, `{{.name}}`). The `host` output is always infrastructure-synthesized and must be declared bare (no `value`/`generated`/`template`).

### Team

Registers a team space and sets quotas.

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

## Project Structure

```
shrine/
├── cmd/                        # Cobra commands (thin dispatchers)
│   ├── root.go                 # Global flags: --config-dir, --state-dir
│   ├── deploy.go               # shrine deploy [--path] [--dry-run]
│   ├── teardown.go             # shrine teardown <team>
│   ├── generate.go             # shrine generate team|app|resource <name>
│   └── ...
├── internal/
│   ├── manifest/               # YAML structs, parser, validator, template helpers
│   │   ├── types.go            # ApplicationManifest, ResourceManifest, TeamManifest
│   │   ├── parser.go           # two-pass YAML loader (probe kind → unmarshal)
│   │   ├── validate.go         # multi-error structural validation
│   │   └── template.go         # ExtractFieldRefs: walks text/template parse trees
│   ├── topo/                   # Standalone Kahn's algorithm (shared by planner + resolver)
│   │   └── topo.go             # Sort(deps map[string]map[string]struct{}) ([]string, error)
│   ├── planner/                # Dependency graph, access checks, ordering
│   │   ├── loader.go           # LoadDir → ManifestSet (duplicate detection, recursive scan)
│   │   ├── resolve.go          # validateDependencies, access checks, quota enforcement
│   │   ├── templates.go        # Plan-time template ref validation (unknown refs rejected)
│   │   ├── order.go            # Topo sort over Resource+Application graph → []PlannedStep
│   │   └── plan.go             # Plan(), PlanSingle() entry points: load → resolve → order/single-step
│   ├── resolver/               # Materializes outputs and env at deploy time
│   │   ├── resolver.go         # LiveResolver: secrets, templates, valueFrom lookup
│   │   └── dry_run_resolver.go # DryRunResolver: same API, placeholder values
│   ├── engine/                 # Orchestrator: dispatches PlannedSteps to backends
│   │   ├── engine.go           # ExecuteDeploy, ExecuteTeardown
│   │   ├── backends.go         # ContainerBackend, RoutingBackend, DNSBackend interfaces
│   │   ├── dryrun/             # Print-only implementations of all three backends
│   │   └── local/              # Real Docker backend
│   │       └── dockercontainer/
│   ├── config/                 # Path resolution (Flag > Env > XDG/FHS) + config.yml loader
│   ├── handler/                # Business logic handlers called by cmd/ (teams, deploy, etc.)
│   │   └── apply.go
│   └── state/                  # Store interfaces + local filesystem implementations
│       └── local/              # SubnetStore, SecretStore, DeploymentStore
├── agents/
│   ├── path.md                 # Session guide for AI build assistants
│   ├── progress.md             # Phase checklist, decisions, known gaps
│   └── prompt.md               # Copy-paste kickstart for new sessions
└── test/
    └── smock/                  # Integration fixture: aterrizar + backendredis + externaldeps
```

## Networking Model

- Every team gets a private bridge network: `shrine.<team>.private` with an auto-assigned `/24` from `10.100.0.0/16`
- A single shared platform network (`shrine.platform`, `10.200.0.0/24`) exists for cross-team communication
- `networking.exposeToPlatform: true` attaches a container to **both** its team network and `shrine.platform`
- Cross-team dependencies require the producer to set `exposeToPlatform: true` (reachability) **and** list the consuming team in `access:` (authorization) — two separate checks at plan time
- `shrine.platform` is never torn down by `shrine teardown` — it is global, not team-owned
- External access is via Traefik only (no host-port publishing). Traefik reaches containers by DNS name over the Docker bridge network

## Deploy Pipeline

```
shrine deploy
     │
     ├── manifest.LoadDir()          → ManifestSet (all Applications + Resources, recursive)
     ├── planner.Resolve()           → validates deps, access, quotas, template refs
     ├── planner.Order()             → topo-sorted []PlannedStep (Kahn's algorithm)
     ├── resolver.ResolveResource()  → materializes each Resource's outputs (literals, secrets, templates)
     ├── engine.ExecuteDeploy()
     │     ├── Container.CreatePlatformNetwork()
     │     ├── for each step (topo order):
     │     │     ├── Container.CreateNetwork(team)
     │     │     ├── Container.CreateContainer(op)   ← image pull, reconcile-by-name, multi-network attach
     │     │     ├── Routing.WriteRoute(op)           ← Traefik dynamic config via SSH
     │     │     └── DNS.WriteRecord(op)              ← AdGuard API call
     │     └── ...
     └── done
```

`--dry-run` swaps all three backends for print-only implementations wired at startup. No special-casing in the engine.

## Key Design Decisions

**Explicit outputs (Option B).** Resources declare exactly what they expose. The planner validates `valueFrom` references against declared output names. No type-specific knowledge in the resolver — any resource type works without code changes.

**No `url` built-in.** Applications expose `host` and `port` only. Scheme composition (`http://`, `grpc://`, `ws://`) is the consumer's job via `template` env entries. Avoids baking HTTP assumptions into the engine.

**Level-triggered reconciliation.** `deployed.txt` is a belief cache; Docker is the source of truth. Every operation inspects real container state by name (`<team>.<name>`) before acting. "Not found" during teardown is a soft success.

**Pluggable backends.** `Engine` holds three optional interfaces (`ContainerBackend`, `RoutingBackend`, `DNSBackend`). Nil backends are skipped silently. Dry-run is just a different set of implementations wired in at startup.

**Topological ordering.** Resources and Applications form a DAG. Kahn's algorithm in `internal/topo/` is shared by the planner (deploy ordering) and the resolver (template resolution within a resource). Cycles are detected and rejected at plan time.

## State Directory Layout

```
<state-dir>/                     # default: ~/.local/share/shrine/
├── subnets.txt                  # allocated /24 subnets (one per team)
├── <team>/
│   ├── secrets.env              # generated secrets (KEY=VALUE, 0600)
│   └── deployments.txt          # deployed resource records (<kind> <name> <container-id>)
└── teams/                       # synced Team manifests (JSON)
```

## Config Directory Layout

```
<config-dir>/                    # default: ~/.config/shrine/
└── config.yml                   # registry credentials, specsDir, gateway IP
```

```yaml
specsDir: ~/projects/myapp/manifests   # default specs directory (~ is expanded)
registries:
  - host: ghcr.io
    username: myuser
    password: mytoken
```

## Infrastructure (Homelab Reference)

| Component    | Details                                              |
|--------------|------------------------------------------------------|
| App server   | 192.168.1.207 — Docker, Watchtower                   |
| Gateway      | 192.168.1.208 — Traefik v3.3, Tailscale              |
| Registry     | 192.168.1.206:8080 — unauthenticated mirror          |
| Traefik dir  | `/opt/traefik/config/` on gateway (file provider)    |
| AdGuard DNS  | Container 104                                        |

## Testing

```bash
go test ./...                        # run all tests
go test ./internal/planner/...       # single package
go run . deploy --path test/smock/ --dry-run  # integration smoke test (no Docker needed)
go run . deploy --path test/smock/          # real Docker round-trip
go run . teardown backend
go run . teardown external
```

The `test/smock/` fixture is the integration gate: three manifests across two teams (`backend`, `external`) with cross-team app→app and app→resource dependencies.
