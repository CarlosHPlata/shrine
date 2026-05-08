---
title: "Custom registries"
description: "Pull images from private registries and reference them with short aliases in manifests."
weight: 25
---

## What custom registries are

By default Shrine pulls images from Docker Hub. A **custom registry** is any private or self-hosted image registry — a Gitea Container Registry, Harbor instance, AWS ECR, or any other OCI-compatible host — that requires a non-default hostname and, optionally, credentials.

Custom registries are declared in `config.yml`. Shrine passes the configured credentials to Docker when pulling images, so manifests themselves never need to include auth details.

## Configuring a custom registry

Add a `registries` list to `config.yml`. Each entry targets one registry host:

```yaml
registries:
  - host: registry.home.lab:5000
    username: admin
    password: s3cr3t
```

| Field | Required | Description |
|-------|----------|-------------|
| `host` | yes | Registry hostname (and port if non-default). |
| `username` | no | Pull credential username. |
| `password` | no | Pull credential password or token. |

Shrine looks up the host from the image reference on every pull. Entries without credentials enable authenticated lookups where the registry allows anonymous access.

You can configure multiple registries:

```yaml
registries:
  - host: registry.home.lab:5000
    username: admin
    password: s3cr3t
  - host: ghcr.io
    username: github-user
    password: ghp_xxxxxxxxxxxx
```

## Using an alias for a registry

Embedding raw hostnames like `registry.home.lab:5000` directly in every manifest leaks your internal network layout to every team author. **Registry aliases** solve this: give a registry a short, memorable name in `config.yml`, then use that name in manifests via the `reg:` prefix.

Add an `alias` field to any registry entry:

```yaml
registries:
  - host: registry.home.lab:5000
    alias: homelab
    username: admin
    password: s3cr3t
```

Alias rules:
- Must contain only alphanumeric characters, hyphens (`-`), and underscores (`_`).
- Dots are not allowed (to avoid ambiguity with hostnames).
- Each alias must be unique across all registry entries.
- An entry without an `alias` field is valid and behaves as before.

Shrine validates aliases at startup. A duplicate or malformed alias causes the command to exit with a clear error before any manifest is processed.

## Referencing an alias in a manifest

Use `reg:<alias>/` as the image prefix in any Application or Resource manifest:

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: my-api
  owner: my-team
spec:
  image: reg:homelab/my-api:v1.2.0
  port: 8080
```

```yaml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: app-db
  owner: my-team
spec:
  type: postgres
  version: "16"
  image: reg:homelab/postgres:16
  port: 5432
  outputs:
    - name: host
    - name: port
```

At **dry-run time** Shrine validates that the alias exists in config and reports an error if it does not — the raw `reg:homelab/` form is preserved in dry-run output so you can confirm the alias before deploying:

```
[DOCKER] ContainerCreate: name=my-team.my-api image=reg:homelab/my-api:v1.2.0
```

At **live deployment time** Shrine expands `reg:homelab/` to the configured host before passing the reference to Docker:

```
Docker pulls → registry.home.lab:5000/my-api:v1.2.0
```

The raw hostname never appears in your manifest files.

## Verifying the setup

Run a dry-run to confirm alias resolution before deploying:

```bash
shrine deploy --dry-run --path ./specs/my-team
```

If the alias is not defined in config, the error will name the alias and the manifest:

```
app "my-api": image "reg:homelab/my-api:v1.2.0": alias "homelab" is not defined in config registries
```

Fix: add or correct the alias in `config.yml`, then re-run.

## Full example

`config.yml`:
```yaml
registries:
  - host: registry.home.lab:5000
    alias: homelab
    username: admin
    password: s3cr3t
```

`specs/my-team/app.yml`:
```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: my-api
  owner: my-team
spec:
  image: reg:homelab/my-api:latest
  port: 8080
  networking:
    exposeToPlatform: true
```

Deploy:
```bash
shrine deploy --path ./specs/my-team
```

Shrine pulls `registry.home.lab:5000/my-api:latest` using the configured credentials. Team authors see only `reg:homelab/my-api:latest` in the manifest — internal hostnames stay private to the operator.
