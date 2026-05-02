---
title: "Traefik gateway"
description: "Expose Shrine apps publicly via the Traefik plugin."
weight: 10
---

## What this guide covers

How to enable the Shrine Traefik gateway plugin, configure HTTP and HTTPS entrypoints, route external traffic to your applications, and reach the Traefik dashboard.

## Concept

By design, Shrine never publishes host ports on application containers. External access is always mediated by a gateway plugin. The Traefik plugin deploys Traefik as a managed container on the shared platform network, generates its static and dynamic configuration from your application manifests, and gives you HTTPS, path-based routing, and the dashboard without manual Traefik setup.

The plugin is activated by adding a `plugins.gateway.traefik` block to `~/.config/shrine/config.yml`. When that block is absent or empty, no Traefik container is deployed and no routing files are generated.

## Enable the gateway

Add the plugin section to your config file:

```yaml
plugins:
  gateway:
    traefik:
      port: 80
```

Then run:

```bash
shrine deploy --path ./manifests
```

Shrine deploys Traefik as a container on the `shrine.platform` network, generates `traefik.yml` (static config) and per-app routing files in `{specsDir}/traefik/` (or the path set by `routing-dir`), and mounts that directory into the container.

A minimal working config uses all defaults — only `port` is required to pin the HTTP entrypoint. The default image is `v3.7.0-rc.2`.

## Configure entrypoints (HTTP / HTTPS / dashboard)

All fields are optional except `port`:

```yaml
plugins:
  gateway:
    traefik:
      image: v3.7.0-rc.2          # Traefik image tag; default v3.7.0-rc.2
      port: 80                    # HTTP entrypoint — host port bound to container :80
      tlsPort: 443                # HTTPS entrypoint — host port bound to container :443
      routing-dir: ./traefik      # where Shrine writes config files; default {specsDir}/traefik/
      dashboard:
        port: 8080                # enables the Traefik dashboard on this host port
        username: admin           # basic-auth credentials (required when dashboard.port is set)
        password: secret
```

When `tlsPort` is set, Shrine adds a `websecure` entrypoint at `:443` to the generated static config and publishes `<tlsPort>:443/tcp` on the Traefik container. Shrine does not manage TLS certificates; certificate provisioning (Let's Encrypt, file-based certs, etc.) is configured directly in Traefik via its own mechanisms.

The three host ports (`port`, `tlsPort`, `dashboard.port`) must be distinct; Shrine validates this at deploy time.

## Per-app routing

For an application to appear in Traefik's routing table it must have `networking.exposeToPlatform: true` and a non-empty `routing.domain`:

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: hello
  owner: my-team
spec:
  image: nginx:alpine
  port: 80
  routing:
    domain: hello.home.lab
    pathPrefix: /hello
  networking:
    exposeToPlatform: true
```

Shrine writes a dynamic config file at `{routing-dir}/dynamic/my-team-hello.yml` containing a Traefik router that matches `Host(\`hello.home.lab\`) && PathPrefix(\`/hello\`)` and forwards to the container.

Applications without `exposeToPlatform: true` are reachable inside their team network but not through Traefik.

## Dashboard access

When `dashboard.port` is set, Shrine writes a router and basic-auth middleware into the dynamic config so Traefik's own dashboard is reachable at `http://<host>:<dashboard.port>/dashboard/`.

`dashboard.port` without credentials fails validation before any container is started.

## Common pitfalls

- **App not reachable through Traefik**: check that the application's manifest sets `networking.exposeToPlatform: true`. Applications without this flag are excluded from Traefik routing generation.
- **Dashboard returns 404**: the dynamic config router for the dashboard is written to `{routing-dir}/dynamic/`. If the directory was empty at deploy time, check that Traefik is mounting it as a volume and that the `providers.file.directory` in `traefik.yml` points to the same path.
- **HTTPS port unreachable**: verify that `tlsPort` is set in the config, that the host firewall allows the port, and that the `websecure` entrypoint appears in `traefik.yml`. See [TLS / HTTPS](/guides/tls/) for full details.
- **Traefik container not recreated after config change**: Shrine detects drift in port bindings and image tags and recreates the container automatically on the next `shrine deploy`.

## See also

- [Routing & aliases](/guides/routing-and-aliases/)
- [TLS / HTTPS](/guides/tls/)
