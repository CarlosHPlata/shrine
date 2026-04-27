<div align="center">
  <img src="assets/logo.webp" alt="Shrine logo" width="180" />
  <h1>Shrine</h1>
  <p>
    Shrine is a CLI tool dedicated to deploy and orchestrate your infrastructure through a single Docker agent running, inspired by kubectl it allows you to define your infrastructure in YAML manifests. It brings a declarative workflow without the complexity of running an actual cluster.
  </p>

  <a href="https://github.com/CarlosHPlata/shrine/releases/latest">
    <img alt="Latest release" src="https://img.shields.io/github/v/release/CarlosHPlata/shrine?style=flat-square&color=3fa37f" />
  </a>
  <a href="https://github.com/CarlosHPlata/shrine/releases/latest">
    <img alt="Release date" src="https://img.shields.io/github/release-date/CarlosHPlata/shrine?style=flat-square&color=1d6fa5" />
  </a>
  <a href="https://github.com/CarlosHPlata/shrine/actions/workflows/ci.yml">
    <img alt="CI" src="https://github.com/CarlosHPlata/shrine/actions/workflows/ci.yml/badge.svg" />
  </a>
  <a href="LICENSE">
    <img alt="License" src="https://img.shields.io/github/license/CarlosHPlata/shrine?style=flat-square&color=3fa37f" />
  </a>
</div>

---

## Install

**curl (recommended):**

```bash
curl -fsSL https://raw.githubusercontent.com/CarlosHPlata/shrine/main/install.sh | sh
```

Downloads the pre-built binary for your OS and architecture and places it in `/usr/local/bin`. Override the install directory:

```bash
INSTALL_DIR=~/.local/bin curl -fsSL https://raw.githubusercontent.com/CarlosHPlata/shrine/main/install.sh | sh
```

**Pin to a specific version:**

```bash
curl -fsSL https://raw.githubusercontent.com/CarlosHPlata/shrine/main/install.sh | sh -s -- --version v0.1.0
```

**Manual download:**

Pre-built binaries for every release are available on the [Releases page](https://github.com/CarlosHPlata/shrine/releases). Download the archive for your platform, extract, and place the `shrine` binary on your `$PATH`.

**Build from source:**

```bash
git clone https://github.com/CarlosHPlata/shrine.git
cd shrine
go build -o shrine .
```

Verify the installation:

```bash
shrine version
```

---

## Configuration

Shrine searches for its configuration file in the following locations, in order:

| Priority | Path |
|---|---|
| 1 | `~/.config/shrine/config.yml` |
| 2 | `~/.shrine.conf.yml` |
| 3 | `/etc/shrine/config.yml` |

The first file found is used. If none exist, Shrine starts with an empty configuration (all fields optional).

Override the search entirely with `--config-dir <dir>` (loads `<dir>/config.yml`) or the `SHRINE_CONFIG_DIR` environment variable.

```yaml
specsDir: ~/projects/myapp/manifests
teamsDir: ~/projects/myapp/teams
registries:
  - host: ghcr.io
    username: myuser
    password: mytoken
```

| Field | Description |
|---|---|
| `specsDir` | Default directory for manifest files. Used by `shrine deploy`, `shrine apply`, and `shrine generate` when `--path` is not provided. |
| `teamsDir` | Optional dedicated directory for team manifests. When set, `shrine apply teams` scans this path instead of `specsDir`. Falls back to `specsDir` if not set. |
| `registries` | List of container registry credentials used when pulling or resolving images. |

`specsDir` is the most convenient way to avoid repeating `--path` on every command. Set `teamsDir` only when your team manifests live in a separate directory from the rest of your specs.

---

## Quickstart

### 1. Define your manifests

A Shrine project is a directory of YAML manifests. Three kinds exist: **Team**, **Resource**, and **Application**. Manifests can be organised into any subdirectory structure — Shrine scans recursively.

**Team** — a namespace with quotas and permissions:

```yaml
# teams/my-team.yml
apiVersion: shrine/v1
kind: Team
metadata:
  name: my-team
spec:
  displayName: "My Team"
  contact: you@example.com
  quotas:
    maxApps: 5
    maxResources: 5
    allowedResourceTypes:
      - postgres
```

**Resource** — a managed dependency (Postgres, Redis, RabbitMQ, …) with typed outputs:

```yaml
# manifests/my-db.yml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: my-db
  owner: my-team
spec:
  type: postgres
  version: "16"
  outputs:
    - name: host
    - name: password
      generated: true
    - name: url
      template: "postgres://postgres:{{.password}}@{{.host}}:5432/app"
```

**Application** — a deployable container with routing and dependency injection:

```yaml
# manifests/my-api.yml
apiVersion: shrine/v1
kind: Application
metadata:
  name: my-api
  owner: my-team
spec:
  image: my-api:latest
  port: 8080
  routing:
    domain: my-api.home.lab
  dependencies:
    - kind: Resource
      name: my-db
      owner: my-team
  env:
    - name: DATABASE_URL
      valueFrom: resource.my-db.url
```

### 2. Deploy

```bash
# Register your teams
shrine apply teams

# Preview the full execution plan
shrine deploy --dry-run

# Deploy for real
shrine deploy

# Deploy a single manifest
shrine apply -f ./manifests/my-team/my-api.yml

# Check what's running
shrine status
shrine status app my-api
shrine status app my-api --team my-team   # disambiguate if needed

# Tear down when done
shrine teardown my-team
```

`shrine deploy` resolves manifests from `specsDir`. `shrine apply teams` resolves from `teamsDir` first, then falls back to `specsDir`. Pass `--path`/`-p` to override either for a single invocation:

```bash
shrine deploy --path ./manifests/
shrine apply teams --path ./teams/
```

---

## Command Reference

### `shrine deploy`

Deploys all manifests found under the configured specs directory (or `--path`). Scans subdirectories recursively.

```bash
shrine deploy                        # uses specsDir from config.yml
shrine deploy --path ./manifests/    # explicit path override
shrine deploy --dry-run              # preview plan without making changes
```

### `shrine apply`

```bash
# Deploy all team manifests
shrine apply teams
shrine apply teams --path ./teams/

# Deploy a single manifest file (kind is inferred from the YAML kind: field)
shrine apply -f ./manifests/my-team/my-api.yml
```

`valueFrom` references inside the target manifest are resolved relative to `specsDir` (or `--path`).

### `shrine status`

```bash
shrine status                        # overview of all running workloads
shrine status app <name>             # status for a specific application
shrine status resource <name>        # status for a specific resource
shrine status app <name> --team <t>  # disambiguate when the name exists in multiple teams
```

The `--team`/`-t` flag is optional — Shrine searches all teams automatically.

### `shrine describe`

```bash
shrine describe app <name>
shrine describe resource <name>
shrine describe app <name> --team <team>   # optional disambiguation
```

### `shrine generate`

Scaffolds new manifest files into the specs directory.

```bash
shrine generate app my-api
shrine generate resource my-db
shrine generate team my-team
shrine generate app my-api --path ./manifests/   # explicit path override
```

---

## Documentation

Full documentation is on the [Shrine Docs site](https://github.com/CarlosHPlata/shrine/wiki) *(coming soon)*.

| Topic | Link |
|---|---|
| Manifest reference | TBD |
| Networking model | TBD |
| Configuration & state directories | TBD |
| Traefik integration | TBD |
| AdGuard DNS integration | TBD |

---

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a PR — it covers the branching model, commit style, and how to run tests.

For bugs use the [bug report template](https://github.com/CarlosHPlata/shrine/issues/new?template=bug_report.md) and for new features the [feature request template](https://github.com/CarlosHPlata/shrine/issues/new?template=feature_request.md).

---

## License

MIT — see [LICENSE](LICENSE).
