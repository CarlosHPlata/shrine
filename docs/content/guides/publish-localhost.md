---
title: "Publish on localhost"
description: "Expose an application's port at localhost:<port> — fixed or automatically assigned — without going through the gateway."
weight: 40
---

## When to use this

Traefik routing gives applications hostnames. Sometimes you just want
`http://localhost:8080` — a debug UI, a dev instance, a protocol that doesn't
fit behind the gateway. `networking.publish` binds the application's
`spec.port` to the host's **loopback interface only** (`127.0.0.1`), so the
port answers on the machine itself and nowhere else.

## Fixed port

Name the port and shrine claims it for the application:

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: dashboard
  owner: ops
spec:
  image: my-dashboard:1.2.0
  port: 3000
  networking:
    publish:
      hostPort: 8080
```

```bash
shrine deploy --path ./specs
curl http://localhost:8080/        # → your app
```

Valid fixed ports are **1024–65535, excluding 30000–32767** (the block below
1024 collides with system services; 30000–32767 is reserved for automatic
assignment).

## Automatic port

Don't care which port? Let shrine pick a free one from **30000–32767**:

```yaml
  networking:
    publish: true
```

The deploy output tells you what was assigned:

```text
    📡 Published ops/dashboard on 127.0.0.1:30000 -> 3000/tcp
```

### Your port survives redeploys

The assigned port is persisted per application. Redeploying — including
deploys that recreate the container, and even a full `shrine teardown`
followed by a redeploy — brings the application back **on the same port**.
Bookmarks and scripts keep working.

The port is given up only when you explicitly forget the application:

```bash
shrine delete application dashboard      # releases the port (teardown first)
shrine delete team ops                   # releases every port the team held
```

`delete application` refuses while the container is still running — tear it
down first. Removing the `publish` option from the manifest stops exposing the
port but keeps the allocation reserved, so re-enabling publishing later
returns the same port.

## What stops a deploy

Fixed ports are conflict-checked at both `shrine deploy --dry-run` and
`shrine deploy`, **before anything changes**. A deploy is rejected when a
fixed port:

- is claimed by two applications in the same manifest set,
- matches a port the Traefik gateway occupies (its entrypoints or dashboard),
- is already allocated to a different application from an earlier deploy.

All conflicts are reported in one run:

```text
host port validation failed:
- host port collision: port 8080 declared by "media/jellyfin" and "ops/dashboard"
- host port reserved: port 18085 declared by "ops/edge" is reserved by the platform gateway
```

An application re-claiming the port it already holds is fine. Ports occupied
by processes outside shrine (say, a service you started by hand) can't be
seen at plan time — those fail when the container starts.

## Do I also need `exposeToPlatform`?

No. `publish` alone is complete — shrine attaches the application to the
platform network automatically. The two options stay independent in every
other way:

| `exposeToPlatform` | `publish` | On platform network | Host port published | Other teams may depend on it |
|---|---|---|---|---|
| off | off | no | no | no |
| on | off | yes | no | yes |
| off | on | yes (implied) | yes | no |
| on | on | yes | yes | yes |

Two things publishing deliberately does **not** do:

- It grants no cross-team access — other teams still need the target to set
  `exposeToPlatform: true` before they can depend on it.
- It enables no gateway routing — `spec.routing` still requires
  `exposeToPlatform: true`.

## Previewing with dry-run

`shrine deploy --dry-run` shows exactly what would be published without
allocating anything:

```text
[DOCKER] ContainerCreate: name=ops.dashboard image=my-dashboard:1.2.0
  attach to platform network=shrine.platform
  publish=127.0.0.1:(auto)->3000/tcp
```

`(auto)` means no port has been assigned yet; once the application holds one,
the dry-run shows the real port. Dry-runs never allocate, write, or release
anything.

## Reference

The full field contract — forms, ranges, and validation rules — lives in the
[manifest schema reference](/reference/manifest-schema/#specnetworkingpublish).
