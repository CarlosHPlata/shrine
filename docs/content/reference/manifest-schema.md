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

A Resource is a managed dependency container (Postgres, Redis, etc.) with named outputs that Applications consume via `valueFrom`.

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
  outputs:
    - name: <string>
      value: <string>        # static value (mutually exclusive with generated/template)
      generated: <bool>      # generate a random secret at deploy time
      template: <string>     # Go text/template referencing other output names
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
| `spec.port` | no | — | Override the default port for this resource type. |
| `spec.image` | no | — | Override the default image for this resource type. |
| `spec.outputs[].name` | yes | — | Output name. `host` and `port` are CLI built-ins filled automatically. |
| `spec.outputs[].value` | no* | — | Static string value. Mutually exclusive with `generated` and `template`. |
| `spec.outputs[].generated` | no* | — | Generate a random value at deploy time. |
| `spec.outputs[].template` | no* | — | Go `text/template` expression referencing sibling output names, e.g. `postgres://postgres:{{.password}}@{{.host}}:{{.port}}/app`. |
| `spec.networking.exposeToPlatform` | no | `false` | Attach the resource to the shared platform network so gateway plugins can reach it. |
| `spec.volumes[].name` | yes (per entry) | — | Logical volume name; must be unique within the manifest. |
| `spec.volumes[].mountPath` | yes (per entry) | — | Absolute path inside the container. |
| `spec.imagePullPolicy` | no | `Always` for `:latest`, `IfNotPresent` otherwise | Docker image pull policy. |

\* Each output (except the built-ins `host` and `port`) must set exactly one of `value`, `generated`, or `template`.

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
| `valueFrom` | Reference to a Resource output: `resource.<resource-name>.<output-name>`. |
| `template` | Go `text/template` expression; can reference other env vars or resource outputs by name. |

## Templating

Shrine resolves `valueFrom` references and `template` expressions at deploy time using Go `text/template`. The dependency graph is topologically sorted (Kahn's algorithm) so references resolve in the correct order. Circular references are a validation error.

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
  outputs:
    - name: host
    - name: port
      value: "5432"
    - name: password
      generated: true
    - name: url
      template: "postgres://postgres:{{.password}}@{{.host}}:{{.port}}/app"
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
