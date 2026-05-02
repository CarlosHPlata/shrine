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

## See also

- [`shrine apply`](/cli/apply/) — Deploy manifests
- [`shrine status`](/cli/status/) — Check running workloads
