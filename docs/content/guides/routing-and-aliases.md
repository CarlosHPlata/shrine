---
title: "Routing & aliases"
description: "Multiple hostnames and path prefixes for one app via routing.aliases."
weight: 20
---

## What this is

`routing.aliases` lets a single Application manifest be reachable at multiple hostnames and path prefixes simultaneously. Each alias entry generates an additional Traefik router that forwards to the same backend container — no duplicate images, no second manifest.

Aliases are parsed and validated on every host; they are silently ignored when no gateway plugin is active, keeping manifests portable across environments with different gateway setups.

## Minimal example

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: finances
  owner: my-team
spec:
  image: my-finances:1.0
  port: 8080
  routing:
    domain: finances.home.lab        # primary domain (required)
    aliases:
      - host: gateway.tailnet.ts.net # additional hostname
        pathPrefix: /finances        # match only this path and below
        stripPrefix: true            # strip the prefix before forwarding (default)
  networking:
    exposeToPlatform: true
```

Each alias entry accepts these fields:

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `host` | yes | — | Hostname the alias router matches. |
| `pathPrefix` | no | — | URL path prefix; when set, the router matches only paths at or below it. |
| `stripPrefix` | no | `true` (when `pathPrefix` is set) | Whether to remove the prefix before forwarding to the backend. |
| `tls` | no | `false` | Attach this alias router to the `websecure` entrypoint and emit `tls: {}`. |

`routing.aliases` requires a non-empty `routing.domain`. An alias with `host` but no `pathPrefix` matches the alias hostname on all paths, mirroring the primary-domain behavior.

## `stripPrefix` semantics

When `pathPrefix` is set and `stripPrefix` is `true` (the default), Traefik removes the prefix from the request path before forwarding. A backend that serves at root receives `/` regardless of which alias path triggered the route.

When `stripPrefix: false`, the full original path is forwarded. Use this when the backend owns its path prefix internally — for example, a Next.js app with `basePath: '/finances'` in `next.config.ts` expects every request to arrive with `/finances` intact. Omitting `stripPrefix: false` for such a backend causes a redirect loop where the app's asset URLs strip the prefix and 404.

```yaml
aliases:
  - host: gateway.tailnet.ts.net
    pathPrefix: /finances
    stripPrefix: false   # backend handles basePath internally
```

`stripPrefix` is a no-op when `pathPrefix` is absent — no middleware is generated.

## `tls: true` on an alias

Setting `tls: true` on an alias entry tells Shrine to generate that alias router with `entryPoints: [web, websecure]` and an empty `tls: {}` block, causing Traefik to terminate TLS on that route:

```yaml
aliases:
  - host: gateway.tailnet.ts.net
    pathPrefix: /finances
    stripPrefix: false
    tls: true
```

This requires the Traefik gateway to have a `websecure` entrypoint configured — either via `tlsPort` in the Shrine config (see [TLS / HTTPS](/guides/tls/)) or by adding the entrypoint to a preserved `traefik.yml`. Shrine writes the alias router regardless and emits a warning if no `websecure` entrypoint is detected, so you can land both changes in any order.

Shrine does not provision TLS certificates. Certificate setup (Let's Encrypt, file-based certs, etc.) is configured directly in Traefik.

## Combining multiple aliases

One Application can carry any number of aliases with independent `stripPrefix` and `tls` settings:

```yaml
routing:
  domain: finances.home.lab
  aliases:
    - host: gateway.tailnet.ts.net   # LAN — HTTP only, strip prefix
      pathPrefix: /finances
      stripPrefix: true
    - host: ext.example.com          # external — HTTPS, no strip
      pathPrefix: /finances
      stripPrefix: false
      tls: true
```

Each alias produces its own router. The primary-domain router is never affected by alias `tls` values.

Two different applications may not declare a router for the same `host` + `pathPrefix` combination. Shrine fails the deploy with a clear error naming both applications and the colliding route.

## Logging

The deploy log includes a line per alias listing the published address and, when `pathPrefix` is set with `stripPrefix: false`, a `(no strip)` marker:

```text
[shrine] alias finances.home.lab -> gateway.tailnet.ts.net/finances (no strip)
[shrine] alias finances.home.lab -> ext.example.com/finances (tls)
```

<!-- TODO: verify exact log format tokens against current code in internal/plugins/gateway/traefik/routing.go -->

## See also

- [Traefik gateway](/guides/traefik/)
- [TLS / HTTPS](/guides/tls/)
