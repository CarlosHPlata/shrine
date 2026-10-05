---
title: "Wiring env and outputs"
description: "Connect resources and applications: private config, exported outputs, and deploy order."
weight: 15
---

## What this guide covers

How to connect a Resource to the Applications and Resources that use it. You will build a Postgres database with a generated password, publish a connection string from it, read that string from an application, point a second application at the first, and hang a metrics exporter off the database. Along the way you will see how Shrine works out the deploy order, and what changes when the consumer belongs to another team.

For the meaning of every individual field, see the [manifest reference](/reference/manifest-schema/). This guide shows how the fields fit together.

## Concept

A Resource declares two blocks, and they go to different places.

| Block | Goes to | Purpose |
|-------|---------|---------|
| `spec.env` | The resource's own container | Its private runtime configuration |
| `spec.outputs` | Other manifests that ask for it | The list of values the resource publishes |

Three rules follow from that split:

- **Nothing is published unless it is listed.** An env var that does not appear under `outputs` cannot be read by any other manifest.
- **An output is never injected into the resource's own container.** `outputs` describes what consumers may read; it adds nothing to the container's environment.
- **Consumers read by name.** An Application or another Resource refers to `resource.<name>.<output>` in its own `env`, and receives the value at deploy time.

Applications have only `spec.env`. They publish two built-in values, `host` and `port`, without declaring anything.

## The example

A team called `shop` runs four things:

- `shop-db` — a Postgres database
- `api` — an application that needs the database's connection string
- `web` — an application that needs to reach `api`
- `shop-db-metrics` — a metrics exporter that also needs the connection string

`api` and `web` use the `traefik/whoami` image as a stand-in for your own applications.

Start with the team. A resource's `type` must be listed under `allowedResourceTypes`, so both types used here are included. Save this as `team.yml`:

```yaml
apiVersion: shrine/v1
kind: Team
metadata:
  name: shop
spec:
  displayName: "Shop"
  contact: shop@example.com
  quotas:
    maxApps: 10
    maxResources: 5
    allowedResourceTypes:
      - postgres
      - postgres-exporter
```

## Keep a generated secret private

Postgres reads its password from `POSTGRES_PASSWORD`. Mark the env var `generated: true` and Shrine creates a random value on the first deploy and keeps the same value on every later one:

```yaml
  env:
    - name: POSTGRES_USER
      value: shop
    - name: POSTGRES_DB
      value: shop
    - name: POSTGRES_PASSWORD
      generated: true
```

None of these is published yet. As long as `POSTGRES_PASSWORD` is not listed under `outputs`, no other manifest can read it.

Use this for any secret that only the resource itself needs to know as a standalone value.

## Export a connection string

Consumers rarely want the password on its own; they want something they can connect with. An output with a `template` composes one from the resource's env vars and the built-ins `host` and `port`:

```yaml
  outputs:
    - name: host
    - name: port
    - name: url
      template: "postgres://{{.POSTGRES_USER}}:{{.POSTGRES_PASSWORD}}@{{.host}}:{{.port}}/{{.POSTGRES_DB}}"
```

A bare `name` re-exports an env var of that name, or the built-in `host` or `port`. `host` is the container's address on the team network, `shop.shop-db` here. A `template` may use any of the resource's env vars, exported or not.

Be clear about what this protects. A consumer that reads `url` receives a string with the password inside it — that is the point of a connection string. What stays private is the password as a separately readable value, and every other env var you did not list. A consumer that is given `url` cannot ask for `POSTGRES_PASSWORD`.

Use this when consumers need one ready-made value rather than the parts.

## Give exports stable names

An output's name does not have to match an env var. Publishing the database name as `database` lets you rename or restructure the internal variable later without touching any consumer:

```yaml
    - name: database
      template: "{{.POSTGRES_DB}}"
```

Use this whenever the internal name is dictated by the image (`POSTGRES_DB`) and you would rather consumers depend on a name you chose.

The database manifest is now complete. Save it as `shop-db.yml`:

```yaml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: shop-db
  owner: shop
spec:
  type: postgres
  version: "16"
  port: 5432
  env:
    - name: POSTGRES_USER
      value: shop
    - name: POSTGRES_DB
      value: shop
    - name: POSTGRES_PASSWORD
      generated: true
  outputs:
    - name: host
    - name: port
    - name: database
      template: "{{.POSTGRES_DB}}"
    - name: url
      template: "postgres://{{.POSTGRES_USER}}:{{.POSTGRES_PASSWORD}}@{{.host}}:{{.port}}/{{.POSTGRES_DB}}"
```

## Read an output from an application

An Application reads a published value with `valueFrom: resource.<name>.<output>`. Save this as `api.yml`:

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: api
  owner: shop
spec:
  image: traefik/whoami:v1.10.1
  port: 80
  env:
    - name: DATABASE_URL
      valueFrom: resource.shop-db.url
```

The container starts with `DATABASE_URL` set to the rendered connection string. The application's env var can have any name; only the part after `resource.shop-db.` has to match an output.

Use this for every value an application takes from a resource.

## Point one application at another

Every Application publishes `host` and `port` without declaring them. There is no `url` built-in, because Shrine does not assume a scheme; compose one with a `template` over the application's own env vars. Save this as `web.yml`:

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: web
  owner: shop
spec:
  image: traefik/whoami:v1.10.1
  port: 80
  env:
    - name: API_HOST
      valueFrom: application.api.host
    - name: API_PORT
      valueFrom: application.api.port
    - name: API_URL
      template: "http://{{.API_HOST}}:{{.API_PORT}}"
```

`API_URL` resolves to `http://shop.api:80`.

Use this when one application calls another inside the platform, instead of hard-coding the address.

## Chain a resource to a resource

A Resource can consume another Resource's outputs in exactly the same way, through its own `env`. Here a Postgres exporter takes the database's connection string. Save this as `shop-db-metrics.yml`:

```yaml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: shop-db-metrics
  owner: shop
spec:
  type: postgres-exporter
  version: "0.15.0"
  image: quay.io/prometheuscommunity/postgres-exporter:v0.15.0
  port: 9187
  env:
    - name: DATA_SOURCE_NAME
      valueFrom: resource.shop-db.url
  outputs:
    - name: host
    - name: port
```

Use this for sidecar-style services — exporters, poolers, backup jobs — that sit beside a resource and need its details.

## Deploy order

### Within one team

You did not declare a single dependency, and you do not need to. When a `valueFrom` points at a manifest owned by the same team, Shrine adds the dependency itself and deploys the producer first. The preview in the next section shows each inferred edge and the env var that caused it.

You can still write `spec.dependencies` entries by hand, for ordering that no `valueFrom` expresses. An explicit entry and an inferred one for the same target count once.

### Across teams

Cross-team references are never inferred. Suppose an `ops` team wants a `reporter` application to read the shop database. Add the team:

```yaml
apiVersion: shrine/v1
kind: Team
metadata:
  name: ops
spec:
  displayName: "Ops"
  contact: ops@example.com
  quotas:
    maxApps: 5
    maxResources: 5
```

and the application:

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: reporter
  owner: ops
spec:
  image: traefik/whoami:v1.10.1
  port: 80
  env:
    - name: SHOP_DATABASE_URL
      valueFrom: resource.shop-db.url
```

Three things are missing, and Shrine names each one in turn.

**1. The consumer must declare the dependency.** As written, planning fails:

```text
Error: enrichment: app "reporter" env "SHOP_DATABASE_URL" references resource "shop-db.url" which is not owned by team "ops"; add an explicit spec.dependencies entry (kind: Resource, name: shop-db) to declare this dependency
```

Add it to `reporter`, under `spec`:

```yaml
  dependencies:
    - kind: Resource
      name: shop-db
      owner: shop
```

The next run reports the two remaining problems together.

**2. The producer must grant access.**

```text
app "reporter" (team "ops") does not have access to resource "shop-db" (owned by "shop")
```

List the consuming team in the producer's metadata.

**3. The producer must be reachable.** Teams have separate networks, so the producer has to join the shared platform network:

```text
app "reporter" (team "ops"): resource "shop-db" (team "shop") is not reachable cross-team — set networking.exposeToPlatform: true on the resource
```

Both fixes go on `shop-db`:

```yaml
metadata:
  name: shop-db
  owner: shop
  access: [ops]
spec:
  # …unchanged…
  networking:
    exposeToPlatform: true
```

With all three in place the plan succeeds. The explicit edge is listed without an "inferred" tag, and `shop-db` joins the platform network:

```text
  3. Application:reporter
       depends on:
         - Resource:shop-db
```

```text
[DOCKER] ContainerCreate: name=shop.shop-db image=postgres:16
  attach to platform network=shrine.platform
```

## Preview and verify

Register the team, then preview:

```bash
shrine apply teams --path .
shrine deploy --dry-run --path .
```

For the `shop` team's manifests the preview prints the order it worked out, then the operations it would perform:

```text
Deploy order:
  1. Resource:shop-db
  2. Application:api
       depends on:
         - Resource:shop-db (inferred from env DATABASE_URL)
  3. Resource:shop-db-metrics
       depends on:
         - Resource:shop-db (inferred from env DATA_SOURCE_NAME)
  4. Application:web
       depends on:
         - Application:api (inferred from env API_HOST)
[DOCKER] CreatePlatformNetwork name=shrine.platform
[DOCKER] NetworkCreate: name=shop
[DOCKER] ContainerCreate: name=shop.shop-db image=postgres:16
[DOCKER] ContainerCreate: name=shop.api image=traefik/whoami:v1.10.1
[DOCKER] ContainerCreate: name=shop.shop-db-metrics image=quay.io/prometheuscommunity/postgres-exporter:v0.15.0
[DOCKER] ContainerCreate: name=shop.web image=traefik/whoami:v1.10.1
[ROUTE]  Finalize
```

The preview checks every reference and shows the order, but it does not print environment values. To see what a container actually received, deploy for real and inspect it:

```bash
shrine deploy --path .
docker inspect shop.api --format '{{range .Config.Env}}{{println .}}{{end}}'
```

## Common pitfalls

| Message | Cause and fix |
|---------|---------------|
| `app "api": env "DATABASE_URL" references non-existent output "POSTGRES_PASSWORD" on resource "shop-db"` | The consumer asked for an env var the resource does not publish. List the name under the resource's `outputs`, or read an output that already exists. |
| `spec.outputs[0] "legacy" must not set value/valueFrom/generated — those fields are deprecated on outputs; declare it under spec.env and list its name under spec.outputs to export it` | An output carries a value of its own. Move the value to an `env` entry and keep only the name under `outputs`. |
| `spec.outputs[2] "nosuchvar" has no template and matches no env var or built-in (host, port)` | A bare output name must match an env var, `host`, or `port`. Fix the name or give the output a `template`. |
| `team "shop": resource type "postgres-exporter" (on "shop-db-metrics") is not allowed by quota` | The resource's `type` is missing from the team's `allowedResourceTypes`. Add it and run `shrine apply teams` again. |
| `… which is not owned by team "ops"; add an explicit spec.dependencies entry …` | A cross-team reference with no declared dependency. See [Across teams](#across-teams). |

## See also

- [Manifest reference](/reference/manifest-schema/) — every field of `env`, `outputs`, and `dependencies`.
- [Secrets vault](/guides/secrets-vault/) — take an env var's value from an external vault instead of generating it.
- [Team-scoped deploy](/guides/team-scoped-deploy/) — deploy one team's manifests when several teams share a directory.
