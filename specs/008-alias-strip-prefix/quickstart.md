# Quickstart: Publishing a Next.js App with `basePath` Through a Shrine Alias

**Feature**: 008-alias-strip-prefix
**Audience**: operators hitting the asset-404 / redirect-loop failure mode reported in [issue #9](https://github.com/CarlosHPlata/shrine/issues/9)

This quickstart is the operator-UX integration test for SC-001 ("an operator who is hitting the redirect-loop / asset-404 failure mode can resolve it by adding `stripPrefix: false` ..."). Time to fix: under 5 minutes once you've identified the symptom.

---

## You'll know you're hitting this if...

You deployed a Next.js (or Grafana, JupyterLab, etc.) app behind a `routing.aliases[]` entry with a `pathPrefix`, and any of:

- Loading the page at `gateway.example.com/finances` redirects to `gateway.example.com/_next/static/...` (no `/finances` prefix), then 404s.
- Static assets (`/_next/static/chunks/main-*.js`, `/_next/image/*`, etc.) all 404 even though the page HTML loads.
- Server logs on the backend show requests with the prefix already stripped (e.g., `/api/balance` instead of `/finances/api/balance`).

What's happening: Shrine's Traefik plugin attaches a `stripPrefix` middleware to the alias router by default, removing `/finances` before forwarding. Next.js sees a request to `/` and emits asset URLs *with* its `basePath: '/finances'` re-prepended. Those asset URLs hit the alias router again, get stripped again, and the backend has no route at the asset path. Loop or 404.

The fix is one line in your manifest.

---

## The fix

In your application manifest, find the alias entry that has a `pathPrefix` and add `stripPrefix: false`:

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: personal-finances
  owner: home
spec:
  image: registry.home.lab/personal-finances:latest
  port: 3000
  routing:
    domain: finances.home.lab
    aliases:
      - host: gateway.tail9a6ddb.ts.net
        pathPrefix: /finances
        stripPrefix: false   # ← this line
```

Re-deploy:

```sh
shrine deploy
```

---

## Verify the fix took effect

The deploy log emits a `routing.configure` event for each application that exposes a domain. With `stripPrefix: false` set, the `aliases` field in that event annotates the affected alias with `(no strip)`:

```text
[INFO] routing.configure  domain=finances.home.lab  port=3000  aliases=gateway.tail9a6ddb.ts.net+/finances (no strip)
```

If you see the `(no strip)` marker, the gateway is forwarding the prefix unchanged.

If you don't see the marker but you set `stripPrefix: false`:

- Check that the alias has a non-empty `pathPrefix`. The marker is suppressed when `pathPrefix` is absent because there's no prefix to strip — `stripPrefix: false` is a no-op in that case.
- Check that you re-deployed after editing the manifest (`shrine deploy`).

---

## Optional: inspect the generated dynamic config

For deeper verification, inspect the dynamic Traefik config Shrine writes:

```sh
cat /var/lib/shrine/traefik/dynamic/<owner>-<app-name>.yml
```

(Substitute the actual routing-dir from your `shrine` config; `/var/lib/shrine/traefik/` is the typical default.)

Before the fix:

```yaml
http:
  middlewares:
    home-personal-finances-strip-0:
      stripPrefix:
        prefixes:
          - /finances
  routers:
    home-personal-finances-alias-0:
      rule: Host(`gateway.tail9a6ddb.ts.net`) && PathPrefix(`/finances`)
      service: home-personal-finances
      entryPoints:
        - web
      middlewares:
        - home-personal-finances-strip-0
```

After the fix:

```yaml
http:
  routers:
    home-personal-finances-alias-0:
      rule: Host(`gateway.tail9a6ddb.ts.net`) && PathPrefix(`/finances`)
      service: home-personal-finances
      entryPoints:
        - web
```

No `middlewares:` block, no `strip-0` middleware, no `middlewares:` list on the router. The request reaches your backend with the prefix intact.

---

## Confirm the app loads

Hit the alias URL in a browser or with `curl`:

```sh
curl -sI http://gateway.tail9a6ddb.ts.net/finances/
# expect: 200 OK (or 304/302 to the login page if auth-gated)

curl -s http://gateway.tail9a6ddb.ts.net/finances/_next/static/...some-chunk.js
# expect: 200 OK with JS content, not 404
```

If both succeed, the fix is in. The Next.js app receives `/finances/...` exactly as the browser sent it; its asset URLs (which include `basePath: '/finances'`) resolve through the same alias router on subsequent requests.

---

## When NOT to use `stripPrefix: false`

Use the default (`stripPrefix: true` or omitted) when:

- Your backend serves at root (`/`) and doesn't know about the prefix. This is the common case for stateless APIs and most container images that didn't pre-bake a basePath.
- You're publishing the same backend at both `<domain>/` (primary) and `<host>/<prefix>/` (alias) and you want the alias to "feel like" the root of the app. The strip middleware makes this work without backend changes.

Use `stripPrefix: false` when:

- The backend has a basePath / sub-path / context-path configured internally (Next.js `basePath`, Grafana `[server] root_url`, JupyterLab `--ServerApp.base_url`, etc.).
- The backend's redirects, asset URLs, or self-links include the prefix and would 404 if the prefix were stripped.
- The backend explicitly documents that it must be reverse-proxied with the path intact.

If you're not sure: deploy with the default first. If you see the redirect-loop / asset-404 symptom above, switch to `stripPrefix: false` and re-deploy.
