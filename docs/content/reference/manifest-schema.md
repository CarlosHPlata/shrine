---
title: "Manifest schema"
description: "Field-by-field reference for Team, Resource, and Application manifests."
weight: 10
---

## Overview

Every Shrine manifest must begin with `apiVersion: shrine/v1` and a `kind` field. Three kinds exist: **Team** (a namespace with quotas), **Resource** (a managed dependency with typed outputs), and **Application** (a deployable container with routing and dependency injection).

All manifests carry `metadata.name` (required on all kinds) and `metadata.owner` (required on Resource and Application; the owning Team name).

Validation is multi-error: all field errors are collected and reported together before any deployment begins.


## Team

A Team defines a namespace with resource quotas. Register teams with `shrine apply teams` before deploying Applications or Resources that reference them.

```yaml
apiVersion: shrine/v1
kind: Team
metadata:
  name: <string>         # required
spec:
  displayName: <string>  # required
  contact: <string>      # required — e.g. admin@example.com
  quotas:
    maxApps: <int>
    maxResources: <int>
    allowedResourceTypes:
      - <string>
  registryUser: <string>
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `metadata.name` | yes | — | Unique identifier for the team. Used as the Docker network suffix. |
| `spec.displayName` | yes | — | Human-readable name shown in status output. |
| `spec.contact` | yes | — | Contact address for the team (for documentation purposes). |
| `spec.quotas.maxApps` | no | 0 (unlimited) | Maximum number of Application manifests the team may deploy. |
| `spec.quotas.maxResources` | no | 0 (unlimited) | Maximum number of Resource manifests the team may deploy. |
| `spec.quotas.allowedResourceTypes` | no | any | Restricts which resource types (e.g. `postgres`) are permitted. |
| `spec.registryUser` | no | — | Docker registry username associated with this team. |

## Resource

A Resource is a managed dependency container (Postgres, Redis, etc.). It declares two distinct blocks: **`env`** is the container's runtime configuration (the same shape as an Application's `env`, plus `generated` secrets); **`outputs`** is a pure export allowlist of what other manifests may consume via `valueFrom`.

The two blocks flow to different places and never overlap: `env` is injected into the resource's own container; `outputs` is published only to consumers. **An output is never set as an environment variable on the resource's own container** — even a template output like `url` exists solely for consumers. If the resource container itself needs a value, declare it under `env`; to also expose it, list its name under `outputs`.

```yaml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: <string>    # required
  owner: <string>   # required — team name
  access:
    - <team-name>   # teams allowed to consume outputs
spec:
  type: <string>    # required — e.g. postgres
  version: <string> # required — e.g. "16"
  port: <int>
  image: <string>
  env:                         # runtime configuration → the container's environment
    - name: <string>
      value: <string>         # static value
      valueFrom: <string>     # vault:<project>/<env>/<key>, or another manifest's exported output
      template: <string>      # Go text/template over sibling env + {{.team}}/{{.name}}/{{.host}}/{{.port}}
      generated: <bool>       # generate a random secret at deploy time
  outputs:                     # export allowlist → what consumers may read
    - name: <string>          # re-export an env var by name, or the built-in host/port
      template: <string>      # OR a derived value over env vars + {{.host}}/{{.port}}/{{.team}}/{{.name}}
  networking:
    exposeToPlatform: <bool>
  volumes:
    - name: <string>
      mountPath: <string>
  imagePullPolicy: <Always|IfNotPresent>
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `metadata.name` | yes | — | Unique identifier within the owning team. |
| `metadata.owner` | yes | — | Team that owns this resource. |
| `metadata.access[]` | no | — | Additional teams that may reference this resource's outputs. |
| `spec.type` | yes | — | Resource type string (e.g. `postgres`, `redis`). |
| `spec.version` | yes | — | Version tag passed to the resource image. |
| `spec.port` | no | — | Override the default port for this resource type. Required when `port` is exported. |
| `spec.image` | no | — | Override the default image for this resource type. |
| `spec.env[].name` | yes | — | Env var name. Must not be a reserved built-in (`host`, `port`, `team`, `name`); unique within the resource. |
| `spec.env[].value` | no† | — | Static string value. |
| `spec.env[].valueFrom` | no† | — | `vault:<project>/<environment>/<secret-name>` (see [Secrets vault guide](/guides/secrets-vault/#how-the-project-component-is-resolved)), or another manifest's exported output (`resource.<name>.<exportedKey>` / `application.<name>.<host\|port>`). |
| `spec.env[].template` | no† | — | Go `text/template` referencing sibling env names plus the built-ins `{{.team}}`/`{{.name}}`/`{{.host}}`/`{{.port}}` (`{{.port}}` requires `spec.port`). e.g. `redis://{{.host}}:{{.port}}` to give the container its own connection string. |
| `spec.env[].generated` | no† | — | Generate a random secret at deploy time, persisted across redeploys. |
| `spec.outputs[].name` | yes | — | Exported key. A bare name re-exports a declared env var, or the built-in `host`/`port`. |
| `spec.outputs[].template` | no | — | Go `text/template` for a derived export, referencing any declared env var (exported or not) plus `{{.host}}`/`{{.port}}`/`{{.team}}`/`{{.name}}`, e.g. `postgres://postgres:{{.POSTGRES_PASSWORD}}@{{.host}}:{{.port}}/{{.POSTGRES_DB}}`. |
| `spec.dependencies[]` | no | — | Explicit deploy-order dependencies (same shape as an Application's). Same-team `valueFrom` references are inferred automatically. |
| `spec.networking.exposeToPlatform` | no | `false` | Attach the resource to the shared platform network so gateway plugins can reach it. |
| `spec.volumes[].name` | yes (per entry) | — | Logical volume name; must be unique within the manifest. |
| `spec.volumes[].mountPath` | yes (per entry) | — | Absolute path inside the container. |
| `spec.imagePullPolicy` | no | `Always` for `:latest`, `IfNotPresent` otherwise | Docker image pull policy. |

† Each `env` entry must set exactly one of `value`, `valueFrom`, `template`, or `generated`.

**Strict allowlist.** Other manifests may consume only keys listed in `outputs`. An env var that is not exported is private to the resource — though an operator may deliberately fold a private value into an exported `template`. An `outputs` entry may **not** set `value`/`valueFrom`/`generated` — those fields are deprecated on outputs; declare them under `env` and list the name under `outputs` to export it. (Pre-split manifests that set those on an output are rejected at plan time with a migration error.)

## Application

An Application is a deployable container with routing, env injection, and dependency wiring.

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: <string>   # required
  owner: <string>  # required — team name
spec:
  image: <string>  # required
  port: <int>      # required
  replicas: <int>
  routing:
    domain: <string>
    pathPrefix: <string>
    aliases:
      - host: <string>
        pathPrefix: <string>
        stripPrefix: <bool>
        tls: <bool>
  dependencies:
    - kind: Resource
      name: <string>
      owner: <string>
  env:
    - name: <string>
      value: <string>
      valueFrom: resource.<name>.<output>
      template: <string>
  networking:
    exposeToPlatform: <bool>
  volumes:
    - name: <string>
      mountPath: <string>
  imagePullPolicy: <Always|IfNotPresent>
```

### Application top-level fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `metadata.name` | yes | — | Unique identifier within the owning team. Used as the container name suffix. |
| `metadata.owner` | yes | — | Team that owns this application. |
| `spec.image` | yes | — | Docker image reference (e.g. `nginx:alpine`). |
| `spec.port` | yes | — | Port the container listens on. |
| `spec.replicas` | no | 1 | Number of container instances to run. |
| `spec.networking.exposeToPlatform` | no | `false` | Attach the container to the platform network and include it in Traefik routing generation. |
| `spec.imagePullPolicy` | no | `Always` for `:latest`, `IfNotPresent` otherwise | Docker image pull policy. |

### `spec.routing`

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `routing.domain` | no | — | Primary hostname. Required when `routing.aliases` is set. |
| `routing.pathPrefix` | no | — | URL path prefix for the primary-domain router. |
| `routing.aliases[]` | no | — | Additional hostnames / path prefixes (see below). Requires `routing.domain`. |

### `spec.routing.aliases[]`

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `host` | yes | — | Hostname the alias router matches. Must be non-empty. |
| `pathPrefix` | no | — | URL path prefix; router matches paths at or below it. |
| `stripPrefix` | no | `true` (when `pathPrefix` is set) | Remove the prefix before forwarding. Set `false` for backends that own their basePath (e.g. Next.js). No-op when `pathPrefix` is absent. |
| `tls` | no | `false` | Attach this alias router to the `websecure` entrypoint and emit `tls: {}`. Only valid inside alias entries. |

### `spec.env[]`

Each env var must set exactly one of `value`, `valueFrom`, or `template`.

| Field | Description |
|-------|-------------|
| `name` | Environment variable name passed to the container. |
| `value` | Static string value. |
| `valueFrom` | Reference to a Resource output (`resource.<resource-name>.<output-name>`) or a vault secret (`vault:<project>/<environment>/<secret-name>` — project may be a name, slug, or UUID; see the [Secrets vault guide](/guides/secrets-vault/)). |
| `template` | Go `text/template` expression; can reference other env vars or resource outputs by name. |

## Templating

Shrine resolves `valueFrom` references, vault-sourced values, and `template` expressions at deploy time using Go `text/template`. Vault secrets (any `valueFrom` field whose value starts with the `vault:` prefix) are fetched from the configured secrets plugin and treated as resolved string values before template expressions are evaluated. The dependency graph is topologically sorted (Kahn's algorithm) so all three resolution mechanisms — `valueFrom`, vault fetch, and `template` — resolve in the correct order. Circular references are a validation error.

## Examples

### Team

```yaml
apiVersion: shrine/v1
kind: Team
metadata:
  name: platform
spec:
  displayName: "Platform Team"
  contact: platform@example.com
  quotas:
    maxApps: 10
    maxResources: 5
    allowedResourceTypes:
      - postgres
      - redis
```

### Resource

```yaml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: app-db
  owner: platform
spec:
  type: postgres
  version: "16"
  port: 5432
  env:
    - name: POSTGRES_DB
      value: app
    - name: password
      generated: true        # private to the resource container
  outputs:
    - name: host
    - name: port
    - name: url
      template: "postgres://postgres:{{.password}}@{{.host}}:{{.port}}/{{.POSTGRES_DB}}"
```

### Application

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: api
  owner: platform
spec:
  image: my-api:1.2.3
  port: 8080
  routing:
    domain: api.home.lab
    aliases:
      - host: gateway.tailnet.ts.net
        pathPrefix: /api
        stripPrefix: true
  dependencies:
    - kind: Resource
      name: app-db
      owner: platform
  env:
    - name: DATABASE_URL
      valueFrom: resource.app-db.url
  networking:
    exposeToPlatform: true
```
