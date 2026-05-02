---
title: "TLS / HTTPS"
description: "Terminate HTTPS at Traefik for any aliased route."
weight: 30
---

## Concept

Shrine applications always communicate over plain HTTP inside the platform network. TLS termination happens at Traefik. There is one `tlsPort` per Traefik gateway instance; it maps a host port to container port 443 where Traefik's `websecure` entrypoint listens.

Shrine is responsible for two things:

1. Publishing the host-to-container port mapping `<tlsPort>:443/tcp` on the Traefik container.
2. Declaring a `websecure` entrypoint at `:443` in the generated static configuration.

Everything else — certificates, resolvers, ACME / Let's Encrypt, mTLS, HTTPS redirects — is configured directly in Traefik and is outside Shrine's scope.

## Configure the gateway

Add `tlsPort` to the Traefik plugin block in `~/.config/shrine/config.yml`:

```yaml
plugins:
  gateway:
    traefik:
      port: 80
      tlsPort: 443
```

Run `shrine deploy` once. Shrine recreates the Traefik container with the new port binding and, when the static config (`traefik.yml`) is Shrine-generated (not operator-preserved), adds the `websecure` entrypoint:

```yaml
# generated traefik.yml (relevant section)
entryPoints:
  web:
    address: ":80"
  websecure:
    address: ":443"
```

The `websecure` entrypoint contains only its `address`. Shrine never injects `tls`, `certResolver`, or any TLS-termination keys — those are operator-owned.

If your `traefik.yml` was preserved from a prior operator edit (per Shrine's preservation regime), Shrine will not overwrite it. You must add the `websecure` entrypoint yourself, or delete the file so Shrine regenerates it.

## Mark an alias as TLS

Per-route HTTPS is opt-in at the alias level. Add `tls: true` to the alias entry in your Application manifest:

```yaml
routing:
  domain: finances.home.lab
  aliases:
    - host: gateway.tailnet.ts.net
      pathPrefix: /finances
      stripPrefix: false
      tls: true
```

Shrine generates that alias router with `entryPoints: [web, websecure]` and an empty `tls: {}` block. Traefik terminates TLS on that route using whatever certificate configuration is already active on the `websecure` entrypoint. The primary-domain router and any aliases without `tls: true` stay HTTP-only.

The `tls` field is only valid inside a `routing.aliases[]` entry. Declaring it on the primary `routing` block is a validation error.

## Certificates

Shrine does not generate, distribute, or reference TLS certificates. Wire certificates into Traefik using standard Traefik mechanisms:

- **Let's Encrypt / ACME** — configure a certificate resolver in your preserved `traefik.yml` and reference it on the router via a Traefik-native dynamic config file in the routing directory.
- **File-based certs** — add a `tls.certificates` block to a dynamic config file in `{routing-dir}/` pointing to your cert and key paths.

Both approaches are outside Shrine's surface and are unaffected by Shrine redeploys as long as the files remain in the routing directory (operator-added files are preserved across redeploys).

## Mixed HTTP and HTTPS

One application can expose both HTTP-only and HTTPS aliases:

```yaml
routing:
  domain: finances.home.lab       # HTTP only — internal LAN access
  aliases:
    - host: a.internal.net        # HTTP only
      pathPrefix: /finances
    - host: b.external.net        # HTTPS — exposed over Tailscale or public internet
      pathPrefix: /finances
      stripPrefix: false
      tls: true
```

After `shrine deploy`, the generated dynamic config contains:
- The primary-domain router: `entryPoints: [web]`, no `tls` block.
- The first alias router: `entryPoints: [web]`, no `tls` block.
- The second alias router: `entryPoints: [web, websecure]`, `tls: {}`.

All three routes point to the same backend service.

## See also

- [Traefik gateway](/guides/traefik/)
- [Routing & aliases](/guides/routing-and-aliases/)
