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

## Quickstart

### 1. Define your manifests

A Shrine project is a directory of YAML manifests. Three kinds exist: **Team**, **Resource**, and **Application**.

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
# Register your teams from the teams/ directory
shrine apply teams

# Preview the full execution plan — nothing is touched
shrine deploy ./manifests/ --dry-run

# Deploy for real
shrine deploy ./manifests/

# Check what's running
shrine status my-team

# Tear down when done
shrine teardown my-team
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
