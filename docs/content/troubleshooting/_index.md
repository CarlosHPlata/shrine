---
title: "Troubleshooting"
description: "Diagnose common issues with Shrine deployments."
weight: 50
---

Common symptoms, their causes, and how to fix them.

## Container fails to start

Check `docker logs <container-id>` to see the actual error. Verify the manifest's `spec.image` exists locally or is pullable from your configured registries. If the image doesn't exist, update `spec.image` and re-run `shrine apply`.

## Traefik does not route to my app

Verify the gateway plugin is active, the app's `spec.routing.domain` matches the incoming request's `Host` header, and the per-team network is connected to the platform bridge. Run `docker network ls` and `docker network inspect <network>` to confirm connectivity.

## State drift after manual `docker rm`

Shrine treats Docker as the source of truth and reconciles on every `apply`. If you manually remove a container, running `shrine apply <dir>` or `shrine deploy` will redeploy it. This is expected behavior.

## `shrine apply` reports validation errors

Fix every error listed in the multi-error report. Shrine surfaces all issues in a single pass, not one at a time. Re-run after fixing to ensure all errors are resolved.

## `--dry-run` shows the right plan but apply fails

The plan succeeded but the actual Docker operation failed. Check Docker's stderr for errors like missing images, port conflicts, or network issues. Verify the image exists, ports are available, and your registries are configured correctly.

## `resolving specsDir: expanding ~: $HOME is not defined`

A `~`-prefixed path in `config.yml` (`specsDir`, `teamsDir`, or the Traefik `routing-dir`) or in `--path` could not be expanded because the process has no home directory — typical under cron, CI runners, systemd units, and containers. The error names the field or flag that supplied the value: replace it with an absolute path, export `HOME` for that environment, or pass an absolute `--path`. Shrine stops before any deployment side effect, so there is nothing to clean up.

## A routing change in the manifest did not take effect

You changed `routing.domain`, an alias, `stripPrefix`, `tls`, or one of the gateway's ports, redeployed, and Traefik still serves the old route. The deploy output shows `Preserving operator-owned route file` or `Preserving operator-owned traefik.yml`: Shrine writes generated gateway files once and never rewrites them. Delete the file named in that line and redeploy, or edit it by hand. See [Generated gateway files are written once](/guides/traefik/#generated-gateway-files-are-written-once).

## A deploy stops because a pinned version is no longer served

The deploy fails before it creates, changes, or removes anything, with a message like this:

```text
Error: application "api": pinned exact version "traefik/whoami@sha256:3f2a9c1b4d7ebacb024fcc9cc3ba71306a98135816442f1b7d6817ed226ae2e2" for shop/api is no longer served by the registry; run "shrine bump application api" to choose another version: pulling image "traefik/whoami@sha256:3f2a9c1b4d7ebacb024fcc9cc3ba71306a98135816442f1b7d6817ed226ae2e2": Error response from daemon: manifest for traefik/whoami@sha256:3f2a9c1b4d7ebacb024fcc9cc3ba71306a98135816442f1b7d6817ed226ae2e2 not found: manifest unknown: manifest unknown
```

The artifact is pinned (`imagePullPolicy: Pinned`), the host no longer has its pinned exact version, and the registry did not serve it either. Registries that delete images no tag points at any more are the usual cause. No container or network was touched, so there is nothing to clean up.

Run the command the message names. `shrine bump application api -v <version>` moves the pin to a version you choose; `shrine bump application api` without `-v` moves it to the newest. Then deploy again. For a resource the command is `shrine bump resource <name>`.

Read the text after the last colon first. If it is an authentication or connection error rather than `manifest unknown`, Shrine could not reach the registry with the configured credentials, and the pinned version may still be there. Fix the `registries` entry in `config.yml` or the network, and deploy again before you bump.

See [Managing image versions](/guides/image-versions/) for how pins work and how to keep them.

## See also

- [`shrine apply`](/cli/apply/) — Deploy manifests
- [`shrine status`](/cli/status/) — Check running workloads
