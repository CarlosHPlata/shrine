# Quickstart: Verifying the Registry Alias Fix

**Feature**: `022-fix-registry-alias-expansion`

## Reproduce the bug (on unfixed code)

1. Config with an alias (`~/.config/shrine/config.yml` or `--config-dir`):

   ```yaml
   registries:
     - host: docker.io
       alias: myregistry
   ```

2. An Application manifest for a workload with **no existing container**:

   ```yaml
   apiVersion: shrine/v1
   kind: Application
   metadata:
     name: whoami-alias
     owner: <your-team>
   spec:
     image: reg:myregistry/traefik/whoami:latest
     port: 80
   ```

3. `shrine deploy --path <dir>` → fails:
   `❌ Error [container.create]: … invalid reference format`.
   The fresh-deploy condition is essential — an existing up-to-date container
   short-circuits before the faulty path and masks the bug.

## Verify the fix

- Same deploy now succeeds; confirm what Docker received:

  ```sh
  docker inspect --format '{{.Config.Image}}' <team>.whoami-alias
  # → docker.io/traefik/whoami:latest   (never "reg:…")
  ```

- Dry-run still shows the alias as authored:

  ```sh
  shrine deploy --dry-run --path <dir> | grep 'reg:myregistry/'
  ```

- Error diagnosability: deploy a manifest with a deliberately broken image and
  confirm the failure names the image reference, not just the container name.

## Test gates

```sh
go test ./...                 # unit gate — includes the new container-spec
                              # regression test (fake dockerAPI, no filesystem,
                              # no Docker daemon)
make test-integration         # Principle V gate — real daemon: alias app
                              # deploy, alias resource deploy, form-equivalence
                              # no-recreate check, dry-run preservation
```

Fail-first proof (VR-003): the new tests were written and run red against the
unfixed code before the fix — the integration deploys fail with
`invalid reference format`, the unit test captures a `Config.Image` still
carrying the `reg:` prefix.
