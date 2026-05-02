# Quickstart: Per-Alias TLS Opt-In for Routing Aliases

**Feature**: 012-tls-alias-routers
**Audience**: Operator (homelab, multi-network deploy)
**Prerequisites**: Shrine release containing this feature; Traefik gateway plugin enabled; the `websecure` entrypoint already wired (via spec 011's `tlsPort: 443` config, or via an operator-edited preserved `traefik.yml`); a working application manifest with at least one `routing.aliases` entry.

This quickstart walks through the manual end-to-end flow for declaring an alias as HTTPS-published. It is the operator-facing companion to the integration scenarios in `tests/integration/traefik_plugin_test.go`.

## 1. Pre-flight: confirm the websecure entrypoint is wired

Before adding `tls: true` to any alias, confirm Traefik has a `websecure` entrypoint listening. Either:

a. **Shrine-generated path (recommended)**: confirm `~/.config/shrine/config.yml` declares `tlsPort: 443` (or another host port) under `plugins.gateway.traefik`. Run `shrine deploy` on a deploy that touches Traefik (e.g., a dummy app re-deploy). Verify `<routing-dir>/traefik.yml` now contains:

   ```yaml
   entryPoints:
     web:
       address: ":80"
     websecure:
       address: ":443"
   ```

b. **Operator-preserved path**: if you've hand-edited `<routing-dir>/traefik.yml` to add a `websecure` entrypoint, confirm the file still contains it. Shrine treats this file as preserved per spec 004 and will not modify it.

If neither is true, this quickstart still works — Shrine will write the alias router with the TLS shape regardless, but inbound HTTPS traffic will not land on it until the entrypoint is wired. You will see a `gateway.alias.tls_no_websecure` warning in the deploy log (see step 5).

## 2. Add `tls: true` to the alias entry

Edit your application manifest. For an existing alias:

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: personal-finances
  owner: shrine-team
spec:
  image: example/finances:1.0
  port: 8080
  routing:
    domain: finances.home.lab
    aliases:
      - host: gateway.tail9a6ddb.ts.net
        pathPrefix: /finances
        stripPrefix: false
        tls: true                # NEW — single line opt-in
```

`tls: true` is the only edit required. The other alias fields (`host`, `pathPrefix`, `stripPrefix`) keep their existing semantics from spec 006.

## 3. Apply the manifest

```sh
shrine apply application -f path/to/personal-finances.yml
```

Or if your workflow re-deploys all manifests in a directory:

```sh
shrine deploy
```

The deploy log should show a line for the application's routing configuration. With `tls: true` set, you'll see the new per-alias log marker:

```text
  🔗 Configuring routing: finances.home.lab -> port 8080
    ↳ Aliases: gateway.tail9a6ddb.ts.net+/finances (no strip) (tls)
```

The `(tls)` marker is the FR-010 signal that Shrine recognized the field. If you don't see it, recheck the manifest indentation — `tls: true` must be a sibling of `host` and `pathPrefix`, not nested under another key.

## 4. Inspect the generated dynamic config

The per-app dynamic config lives at:

```text
<routing-dir>/dynamic/<team>-<service>.yml
```

For our example: `<routing-dir>/dynamic/shrine-team-personal-finances.yml`. Open it. The alias router should now look like:

```yaml
http:
  routers:
    shrine-team-personal-finances:
      rule: Host(`finances.home.lab`)
      service: shrine-team-personal-finances
      entryPoints:
        - web
    shrine-team-personal-finances-alias-0:
      rule: Host(`gateway.tail9a6ddb.ts.net`) && PathPrefix(`/finances`)
      service: shrine-team-personal-finances
      entryPoints:
        - web
        - websecure
      tls: {}
  services:
    shrine-team-personal-finances:
      loadBalancer:
        servers:
          - url: http://shrine-team.personal-finances:8080
```

Confirm:

- The primary-domain router (`shrine-team-personal-finances`) declares `entryPoints: [web]` only and has no `tls` field.
- The alias router (`shrine-team-personal-finances-alias-0`) declares `entryPoints: [web, websecure]` and has `tls: {}`.

If the file does NOT show this shape, check whether the file has been marked operator-owned per spec 009 (you'll see a `Preserving operator-owned route file:` line in the deploy log). When the file is preserved, Shrine does not rewrite it in response to manifest changes — delete the file and re-deploy to regenerate it with the new shape.

## 5. Confirm Traefik is serving HTTPS on the alias

Send an HTTPS request to the alias address. With Traefik's TLS configuration (separate from this feature — operator-owned per FR-012):

```sh
curl -k https://gateway.tail9a6ddb.ts.net/finances
```

The `-k` flag skips cert verification — fine for a homelab confirmation. The response should match what `curl http://finances.home.lab` returns: same backend, same body.

If the connection fails at the TLS layer (handshake error, no cert), the issue is in your Traefik TLS configuration (cert resolver, default cert, ACME setup), not in this feature. Per FR-012, Shrine emits `tls: {}` and nothing else — TLS termination is configured entirely outside Shrine. Consult Traefik's docs for cert resolvers and the static-config TLS surface.

## 6. Mixing TLS-on and TLS-off aliases

For applications that need different protocols on different aliases, declare each alias entry independently:

```yaml
spec:
  routing:
    domain: finances.home.lab
    aliases:
      - host: lan-only.shrine.lab
        pathPrefix: /finances
        # no tls field — internal HTTP only
      - host: gateway.tail9a6ddb.ts.net
        pathPrefix: /finances
        stripPrefix: false
        tls: true                 # external HTTPS via Tailscale
```

After `shrine deploy`, the dynamic config will contain two alias routers — the first attaching only to `web`, the second attaching to both `web` and `websecure` with `tls: {}`. The deploy log will show:

```text
    ↳ Aliases: gateway.tail9a6ddb.ts.net+/finances (no strip) (tls),lan-only.shrine.lab+/finances
```

(The list is alphabetically sorted.)

## Reverting

To revert an alias from HTTPS to HTTP-only, remove the `tls: true` line (or set `tls: false` explicitly — they behave identically). Re-deploy:

```sh
shrine deploy
```

The alias router is regenerated with `entryPoints: [web]` only and no `tls` block. Operator-preserved dynamic config files (per spec 009) are NOT rewritten — delete the file first if you need the regenerate-from-manifest behavior.

## What this feature does NOT do

- **Certificate provisioning**: Shrine does not generate, validate, distribute, or reference TLS certificates. Configure cert resolvers, ACME, or default certs directly in Traefik (preserved `traefik.yml` or external cert management). FR-012 makes this explicit.
- **HTTP→HTTPS redirects**: not configured by this feature. If you want redirect-from-HTTP behavior, configure it in Traefik's static or dynamic config directly.
- **Primary-domain HTTPS**: the `tls` field is only valid inside `routing.aliases[]`. To publish the primary `routing.domain` over HTTPS, the existing pre-spec-012 workaround applies — edit the preserved per-app dynamic config file directly. Per-primary-domain TLS opt-in is a deferred conversation (see spec assumptions).
- **Per-route cert overrides**: Shrine emits an empty `tls: {}` block. Adding `certResolver`, `domains`, or `options` to the block requires hand-editing the dynamic config file (which then becomes operator-preserved per spec 009). FR-012 prohibits Shrine from injecting these keys.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---------|--------------|-----|
| Deploy log shows `⚠️ alias tls: true but websecure entrypoint missing in <path>` | Manifest declares `tls: true` but the active static config has no `websecure` entrypoint. | Set `tlsPort` per spec 011, OR add `entryPoints.websecure: { address: ":443" }` to a preserved `traefik.yml`, OR delete the preserved static config so Shrine regenerates it. |
| Generated dynamic config does not show `tls: {}` after re-deploy | The file is operator-preserved per spec 009. Manifest changes do not rewrite preserved files. | Delete the preserved file at `<routing-dir>/dynamic/<team>-<service>.yml` and re-deploy. |
| Manifest deploy fails with `cannot unmarshal !!str into bool` (or similar) | `tls` field has a non-boolean value (e.g., `"yes"`, `1`, an object). | Use bare YAML `true` or `false`. Quoting (`tls: "true"`) is rejected by design. |
| Manifest deploy fails with `field tls not found in type manifest.Routing` | `tls` field placed at `spec.routing.tls` (top level) instead of inside an alias entry. | Move `tls: true` to be a sibling of `host` and `pathPrefix` inside the relevant `routing.aliases[]` entry. |
| HTTPS connection succeeds at TCP but fails at TLS handshake | Traefik's `websecure` entrypoint has no cert resolver / default cert configured. | Configure cert resolvers in Traefik directly — out of scope for this feature per FR-012. |
