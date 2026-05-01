# Quickstart — Preserve Operator-Edited Per-App Routing Files

**Date**: 2026-05-01
**Audience**: a homelab operator running `shrine deploy` against a single host.

A 5-minute walkthrough framed as a real session. By the end you will have:

1. Deployed a new app and seen Shrine write its per-app routing file.
2. Hand-edited that file to add a custom middleware Shrine doesn't expose at the manifest level.
3. Redeployed and confirmed your edit survived.
4. Removed the app from the manifest, run teardown, and seen Shrine warn you about the orphan file.
5. Cleaned up the orphan with a single `rm`.

## 0. Pre-flight

You have:

- A working `shrine` binary on `PATH`.
- A homelab host with the traefik plugin enabled (per the gateway-routing-dir config).
- One Application manifest you can `shrine apply`.

The walkthrough uses `team=marketing`, `name=blog` for clarity. Substitute your own.

## 1. First deploy — Shrine writes the per-app file

```bash
shrine deploy
```

In the deploy log you will see (among other lines):

```text
gateway.route.generated  team=marketing  name=blog  path=/var/lib/shrine/gateway/dynamic/marketing-blog.yml
```

That `path` is the per-app routing file Shrine just wrote from your manifest. From this moment on, Shrine treats the file as **operator-owned** — every subsequent deploy will leave it alone.

## 2. Hand-edit the file

Open it:

```bash
$EDITOR /var/lib/shrine/gateway/dynamic/marketing-blog.yml
```

Add whatever Shrine doesn't expose. For example, a Traefik header-stripping middleware that the manifest field set doesn't yet cover:

```yaml
http:
  routers:
    marketing-blog:
      rule: Host(`blog.home.lab`)
      service: marketing-blog
      entryPoints: [web]
      middlewares: [strip-tracking-headers]   # <-- you added this
  services:
    marketing-blog:
      loadBalancer:
        servers:
          - url: http://marketing.blog:80
  middlewares:                                 # <-- you added this section
    strip-tracking-headers:
      headers:
        customRequestHeaders:
          X-Forwarded-For: ""
          X-Real-IP: ""
```

Save and exit. Traefik will pick up the change automatically (file-watch).

## 3. Redeploy — your edit survives

Run deploy again:

```bash
shrine deploy
```

This time the log will show:

```text
gateway.route.preserved  team=marketing  name=blog  path=/var/lib/shrine/gateway/dynamic/marketing-blog.yml
```

The bytes on disk are unchanged. You can verify with:

```bash
diff <(shasum /var/lib/shrine/gateway/dynamic/marketing-blog.yml) <(shasum /var/lib/shrine/gateway/dynamic/marketing-blog.yml.before-deploy)
```

(if you snapshotted before — but the policy guarantees the bytes are identical, so the `diff` is empty.)

## 4. Optional sidetrack — manifest changes do NOT propagate while the file exists

If you change the manifest's `port` or `aliases` and redeploy, you'll still see `gateway.route.preserved` and the on-disk file will still have the old values. **This is intentional.** The deploy log preserve-signal is your reminder that the manifest change did not land.

To apply a manifest change:

```bash
rm /var/lib/shrine/gateway/dynamic/marketing-blog.yml
shrine deploy
```

The next deploy will see the file Absent and write a fresh one from the current manifest. The log will switch back to `gateway.route.generated`. Your hand-edits are gone — that's the deliberate trade-off (re-apply them by editing again, or merge them into a fresh edit before redeploying).

## 5. Tear down the app — see the orphan warning

Remove the app from your manifest, then:

```bash
shrine teardown marketing
```

The log will include:

```text
application.teardown   team=marketing  name=blog
container.remove       team=marketing  name=blog
gateway.route.orphan   team=marketing  name=blog  path=/var/lib/shrine/gateway/dynamic/marketing-blog.yml
```

The container is gone. The per-app routing file is **still on disk** — Shrine refuses to delete operator-owned state without your explicit consent. Until you remove the file, Traefik will keep serving (or attempting to serve) `blog.home.lab` with the now-orphaned route, returning 502s once the backend is gone.

## 6. Final cleanup — single `rm` per orphan

The warning told you exactly which path to remove. Run it:

```bash
rm /var/lib/shrine/gateway/dynamic/marketing-blog.yml
```

That's it. The route is fully torn down; on the next teardown of any app, the warning won't reappear for `marketing-blog` (file is now Absent).

---

## Cheat-sheet

| You want to… | Run |
|---|---|
| Apply manifest changes for a specific app whose per-app file you have edited | `rm <path-from-deploy-log>` then `shrine deploy` |
| Discover every orphan you have left behind | Run `shrine teardown <team>` for each torn-down team — every orphan emits one warning per teardown |
| Find the per-app file path for a given app | Look in the deploy log; `path=` is in every `gateway.route.*` event |

## What changed vs. spec 004

Spec 004 introduced the same policy for the gateway-wide `traefik.yml` static config. This feature extends that policy down to per-app routing files. The mental model is identical: **anything Shrine writes once is yours from then on.** The only operator action that returns ownership to Shrine is deleting the file.
