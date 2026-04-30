# Quickstart: Routing Aliases

**Feature**: 006-routing-aliases
**Audience**: operators with an existing Shrine deployment using the Traefik gateway plugin

This walk-through adds a second hostname to an already-running application — no second manifest, no extra commands.

---

## Prerequisites

- A working Shrine deployment with the Traefik gateway plugin enabled (`config.yml` has a `traefik:` block with at least `routingDir`).
- An existing Application manifest already reachable at its primary `routing.domain`.
- DNS or `/etc/hosts` entries for any new alias hostnames you plan to add — Shrine does not provision these for you.

---

## 1. Edit the manifest

Open the manifest for the application you want to expose at a second address. Add an `aliases` list under `spec.routing`:

```yaml
# before
spec:
  routing:
    domain: finances.home.lab
```

```yaml
# after
spec:
  routing:
    domain: finances.home.lab
    aliases:
      - host: gateway.tail9a6ddb.ts.net
        pathPrefix: /finances
```

The default `stripPrefix: true` means requests to `gateway.tail9a6ddb.ts.net/finances/api/balance` reach the backend as `/api/balance` — the same way `finances.home.lab/api/balance` already does. No backend changes needed.

---

## 2. Deploy

```sh
shrine deploy
```

In the deploy output, look for the `routing.configure` info event for your app:

```text
routing.configure status=info domain=finances.home.lab port=8080 aliases=gateway.tail9a6ddb.ts.net+/finances
```

The `aliases` field confirms which alias addresses are now published.

---

## 3. Verify

```sh
curl -s -o /dev/null -w '%{http_code}\n' http://finances.home.lab/api/health
# 200

curl -s -o /dev/null -w '%{http_code}\n' http://gateway.tail9a6ddb.ts.net/finances/api/health
# 200
```

Both addresses hit the same backend container.

You can also inspect the generated dynamic config:

```sh
cat <routingDir>/dynamic/<team>-<app>.yml
```

You should see two routers (`<team>-<app>` and `<team>-<app>-alias-0`), one shared `services.<team>-<app>` block, and one `middlewares.<team>-<app>-strip-0` block with `prefixes: [/finances]`.

---

## 4. Add more aliases

Append more entries to the same list. Each one becomes its own router. To omit prefix stripping:

```yaml
spec:
  routing:
    domain: notes.home.lab
    aliases:
      - host: lan.home.lab
        # no pathPrefix → matches all paths on lan.home.lab
      - host: gateway.tail9a6ddb.ts.net
        pathPrefix: /notes-raw
        stripPrefix: false
        # backend receives /notes-raw/... unchanged
```

Re-run `shrine deploy`. The dynamic config file is rewritten with the new routers; Traefik's file watcher picks the change up automatically.

---

## 5. Remove an alias

Delete the entry from the manifest and `shrine deploy` again. The alias's router (and strip middleware, if any) disappears from the dynamic config; the address stops resolving on the next Traefik reload — typically within a second.

---

## Common errors

| You wrote | You'll see |
|---|---|
| `aliases:` without a `host:` field | `spec.routing.aliases[0].host is required` |
| `pathPrefix: finances` (no leading `/`) | `spec.routing.aliases[0].pathPrefix "finances" must start with "/"` |
| `aliases:` but `domain:` is empty | `spec.routing.aliases is set but spec.routing.domain is empty` |
| Same `host`+`pathPrefix` declared by two different apps in your fleet | `routing collision: host="..." pathPrefix="..." declared by "team-a/x" and "team-b/y"` — fix one of the manifests; the deploy is aborted before any gateway config is touched |

---

## Rolling back

To revert to the pre-aliases state, delete the `aliases:` field and re-deploy. Shrine writes a dynamic config file containing only the primary-domain router — byte-identical to what pre-feature releases produced for the same manifest.

---

## What this feature does *not* do

- Provision DNS for alias hostnames.
- Issue TLS certificates per-alias. Aliases inherit the entry-point and TLS configuration of the primary domain; if you need a different cert for a different host, that is a separate concern.
- Apply per-alias middleware (auth, rate limit, etc.). Aliases share the primary domain's middleware chain.
- Affect any non-Traefik gateway plugin. Aliases are parsed but silently ignored on hosts where the Traefik plugin is not active.
