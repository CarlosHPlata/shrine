# Contract: Wiring Guide

The outline, manifests, and output for `docs/content/guides/wiring-env-and-outputs.md`. Every manifest and every quoted message below was produced by running a binary built from `main` @ `64c93bc` with `shrine deploy --dry-run` ([research.md](../research.md) F12). The guide may quote them; it may not show output that is not here without re-running the preview to obtain it.

Front matter and listings are in [docs-pages.md](docs-pages.md) §5.

## 1. Outline

| # | Section | Covers | FR |
|---|---|---|---|
| 1 | What this guide covers | One paragraph; what the reader will have at the end | FR-023 |
| 2 | Concept | `env` is the container's private runtime config. `outputs` is the allowlist other manifests may read. Nothing is exported unless listed. An output is never injected into the resource's own container. | FR-024 |
| 3 | The example | The cast (§2) and the team manifest | FR-025 |
| 4 | Keep a generated secret private | `POSTGRES_PASSWORD` with `generated: true`, not listed under `outputs`. Generated once, stable across redeploys. | FR-026a |
| 5 | Export a connection string | `url` output template. State plainly: a consumer of `url` receives a string that contains the password; what stays private is the password as a separately readable key, and every other env var. | FR-026b |
| 6 | Give exports stable names | `database` output with template `{{.POSTGRES_DB}}`; the internal env name can change without breaking consumers | FR-026c |
| 7 | Read an output from an application | `api` reads `resource.shop-db.url` | FR-026d |
| 8 | Point one application at another | `web` reads `application.api.host` and `.port` and composes `API_URL` with a `template`. Applications expose `host` and `port` only; there is no `url` built-in. | FR-026e |
| 9 | Chain a resource to a resource | `shop-db-metrics` reads `resource.shop-db.url` in its own `env` | FR-026f |
| 10 | Deploy order | Same-team references order themselves (preview output, §4). Cross-team references need three things (§5): an explicit `dependencies` entry on the consumer, `metadata.access` on the producer, `networking.exposeToPlatform: true` on the producer. | FR-026g |
| 11 | Preview and verify | Real preview output (§4). One sentence: the preview does not print environment values. After a real deploy: `docker inspect shop.api --format '{{range .Config.Env}}{{println .}}{{end}}'` | FR-027 |
| 12 | Common pitfalls | The messages in §6, each with its fix | — |
| 13 | See also | Manifest reference (field detail), Secrets vault (vault refs in `env`), Team-scoped deploy | FR-028 |

Sections 4–9 each show the manifest fragment that introduces the pattern and one sentence on when to use it. The complete manifests appear once, in section 3 or at the end of the step that completes them. Field-by-field tables stay in the manifest reference.

`traefik/whoami` stands in for the reader's own application images; the guide says so once.

## 2. Manifests (verified)

`team.yml` — `allowedResourceTypes` is enforced; omitting `postgres-exporter` fails with `team "shop": resource type "postgres-exporter" (on "shop-db-metrics") is not allowed by quota`.

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

`shop-db.yml`

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

`api.yml`

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

`web.yml`

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

`shop-db-metrics.yml`

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

## 3. Commands

```bash
shrine apply teams --path .
shrine deploy --dry-run --path .
shrine deploy --path .
```

## 4. Preview output (same-team, verified, exit 0)

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

The first line of real output is `[shrine] Planning deployment from: <absolute path>`; the guide shows it with the reader's path elided or omits it.

## 5. Cross-team step (verified)

Additions: a second team and a consumer in it.

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

The guide walks the three failures in the order a reader meets them, then the fix for each.

| State | Result |
|---|---|
| As above, no `dependencies` | `Error: enrichment: app "reporter" env "SHOP_DATABASE_URL" references resource "shop-db.url" which is not owned by team "ops"; add an explicit spec.dependencies entry (kind: Resource, name: shop-db) to declare this dependency` |
| + `dependencies` entry on `reporter` | `app "reporter" (team "ops") does not have access to resource "shop-db" (owned by "shop")` and the reachability error below, reported together |
| + `access: [ops]` on `shop-db` | `app "reporter" (team "ops"): resource "shop-db" (team "shop") is not reachable cross-team — set networking.exposeToPlatform: true on the resource` |
| + `networking.exposeToPlatform: true` on `shop-db` | Exit 0 |

Consumer, final form:

```yaml
  dependencies:
    - kind: Resource
      name: shop-db
      owner: shop
```

Producer, final additions:

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

Preview lines that differ from §4 once all three are in place: the explicit edge prints with no "inferred" tag, and the producer joins the platform network.

```text
  3. Application:reporter
       depends on:
         - Resource:shop-db
…
[DOCKER] ContainerCreate: name=shop.shop-db image=postgres:16
  attach to platform network=shrine.platform
…
[DOCKER] NetworkCreate: name=ops
[DOCKER] ContainerCreate: name=ops.reporter image=traefik/whoami:v1.10.1
```

## 6. Messages for "Common pitfalls" (verified)

| Mistake | Message |
|---|---|
| A consumer reads a key the resource does not export (`resource.shop-db.POSTGRES_PASSWORD`) | `app "api": env "DATABASE_URL" references non-existent output "POSTGRES_PASSWORD" on resource "shop-db"` |
| An output sets `value`, `valueFrom`, or `generated` | `spec.outputs[0] "legacy" must not set value/valueFrom/generated — those fields are deprecated on outputs; declare it under spec.env and list its name under spec.outputs to export it` |
| An output names something that is neither an env var nor a built-in, and has no template | `spec.outputs[2] "nosuchvar" has no template and matches no env var or built-in (host, port)` |
| A resource type the team's quota does not allow | `team "shop": resource type "postgres-exporter" (on "shop-db-metrics") is not allowed by quota` |
| The three cross-team messages | §5 |

## 7. What the guide must not say

- That the preview shows environment values, placeholders, or a resource's exported outputs (descoped; research F7).
- That an output template hides a secret from the consumer of that output (outline row 5).
- That Applications expose a `url` built-in.
- That cross-team references are inferred.
