---
title: "Managing image versions"
description: "Pin an image at its newest version, see which version runs, and move it on purpose: upgrade, roll back, take the newest."
weight: 45
---

## What this guide covers

This guide explains how to keep an application or a resource on exactly the version it was first deployed with, and how to change that version only when you choose to. You will pin a service at its newest version, choose a database's first version in advance, rebuild the host without losing either, read which version runs, upgrade one artifact, roll it back, take the newest again, make pinning the default for every manifest, and retire an artifact.

For the field-level rules, see [Image pull policy](/reference/manifest-schema/#image-pull-policy) in the manifest reference. This guide shows how they work together.

## Concept: who owns the version

`spec.imagePullPolicy` decides who owns the version an artifact runs.

| Policy | Who owns the version | What deploy does |
|--------|----------------------|------------------|
| `Always` | the manifest | Pulls on every deploy, so a `latest` tag floats to whatever the registry serves today. |
| `IfNotPresent` | the manifest | Reuses the local image when it is present. A fixed tag such as `postgres:16` is only as fixed as the local image cache: after a prune, the tag is pulled again and may now point at something else. |
| `Pinned` | Shrine | Takes the newest version the first time, records it, and runs exactly that version from then on, until you move it. |

An artifact's effective policy is decided in this order: the manifest's own `spec.imagePullPolicy`; else the `imagePullPolicy` default in `config.yml`, when it is set; else a rule derived from the tag, `Always` for `latest` or no tag and `IfNotPresent` for any other tag. Manifests that name no policy and no default keep behaving exactly as they always have.

The words this guide uses:

- **Repository**: the image name without a version, such as `postgres` or `traefik/whoami`.
- **Version**: the readable part of a reference, the tag, such as `16` or `latest`.
- **Exact version**: the registry digest, `sha256:…`. It never changes: two pulls of the same exact version give the same bytes.
- **Manifest-owned**: the version is whatever the manifest names (`Always`, `IfNotPresent`).
- **Shrine-owned**: the manifest names only the repository, and Shrine resolves, records, and keeps the version (`Pinned`).
- **Pin**: Shrine's record of a Shrine-owned version: the exact version, the readable version it was resolved from, and the date.
- **Bump**: replacing a pin with a new one, with `shrine bump`.

Every output block starts with the command, after a `$` prompt. A line holding only `…` marks lines left out; the lines that remain are what Shrine prints. Exact versions, dates, container ids, and paths in the output are examples: yours will differ, so never copy one from this page.

## The example

One team, `shop`, runs three things:

- `api`: an application on `traefik/whoami`, pinned.
- `shop-db`: a PostgreSQL database, pinned.
- `cache`: a Redis cache on the fixed tag `7.4`, manifest-owned, for contrast.

`config.yml` points Shrine at the manifests:

```yaml
specsDir: ~/shop/manifests
```

The team is registered once with `shrine apply teams`. The three manifests:

```yaml
# ~/shop/manifests/api.yml
apiVersion: shrine/v1
kind: Application
metadata:
  name: api
  owner: shop
spec:
  image: traefik/whoami
  imagePullPolicy: Pinned
  port: 80
```

```yaml
# ~/shop/manifests/shop-db.yml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: shop-db
  owner: shop
spec:
  type: postgres
  imagePullPolicy: Pinned
  env:
    - name: POSTGRES_PASSWORD
      value: change-me
```

```yaml
# ~/shop/manifests/cache.yml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: cache
  owner: shop
spec:
  type: redis
  version: "7.4"
```

`api` and `shop-db` name only a repository and the `Pinned` policy. `cache` names a version and no policy, so the derived rule makes it `IfNotPresent`.

## Pin a service at its newest version

Under `Pinned` the manifest names only the repository: an Application image has no tag or the tag `latest`, and a Resource omits `spec.version` or sets it to `latest`. Naming a fixed version is a validation error, reported with the manifest's other errors before anything is deployed:

```text
$ shrine deploy
[shrine] Planning deployment from: /home/me/shop/manifests
Validation errors:
application "api": spec.image "traefik/whoami:v1.10.1" names a fixed version but the image pull policy is Pinned; use "traefik/whoami" or "traefik/whoami:latest"
resource "shop-db": spec.version "16" names a fixed version but the image pull policy is Pinned; omit it or use "latest"
Error: Spec validation errors
…
```

With the manifests as in [The example](#the-example), a dry run says what the first deploy would do, and writes nothing:

```text
$ shrine deploy --dry-run
[shrine] Planning deployment from: /home/me/shop/manifests
Deploy order:
  1. Application:api
  2. Resource:cache
  3. Resource:shop-db
[DOCKER] ImageResolve: name=shop.api image=traefik/whoami policy=Pinned -> would resolve newest and pin
[DOCKER] ImageResolve: name=shop.cache image=redis:7.4 policy=IfNotPresent -> manifest-owned
[DOCKER] ImageResolve: name=shop.shop-db image=postgres policy=Pinned -> would resolve newest and pin
…
```

**Choosing the first version yourself.** For a database you usually want a chosen major version, not whatever is newest on the day of the first deploy. Bump it before it is ever deployed. The bump resolves the version in the registry immediately, records the pin, and starts nothing:

```text
$ shrine bump resource shop-db -v 16
🔎 Resolving image for shop.shop-db (postgres:16)
    ✅ Pulled image postgres:16
  📌 Bumped shop.shop-db to 16@2d6f0b8e4a1c
Pinned shop/shop-db at 16@2d6f0b8e4a1c; run "shrine deploy" to apply
```

`16@2d6f0b8e4a1c` is the readable form of a pin: the version it was resolved from, `@`, and the first twelve characters of the exact version. The dry run now shows the pin it would reuse:

```text
$ shrine deploy --dry-run
…
[DOCKER] ImageResolve: name=shop.api image=traefik/whoami policy=Pinned -> would resolve newest and pin
[DOCKER] ImageResolve: name=shop.cache image=redis:7.4 policy=IfNotPresent -> manifest-owned
[DOCKER] ImageResolve: name=shop.shop-db image=postgres policy=Pinned -> pinned postgres@sha256:2d6f0b8e4a1c5e953124e2943a0ef520833a53ec00009a6c756237ef124ab460 (16, 2026-10-01)
…
```

Deploy. Before any container or network is touched, Shrine resolves the image of every artifact in the deploy set, and says what it chose:

```text
$ shrine deploy
[shrine] Planning deployment from: /home/me/shop/manifests
🔎 Resolving image for shop.api (traefik/whoami)
    ✅ Pulled image traefik/whoami
  📌 Pinned shop.api at latest@3f2a9c1b4d7e
🔎 Resolving image for shop.cache (redis:7.4)
    ✅ Pulled image redis:7.4
  🔎 Resolved shop.cache redis:7.4@e1c7a93f5b20
🔎 Resolving image for shop.shop-db (postgres)
  📌 Using pinned shop.shop-db 16@2d6f0b8e4a1c (since 2026-10-01)
…
```

`📌 Pinned` means the pin was recorded on this deploy: `api` took the newest version and that exact version is now its pin. `📌 Using pinned` means an existing pin was reused. `🔎 Resolved` means the version is manifest-owned.

A week later the upstream project publishes a new `latest`, and you redeploy to change an environment variable. `api` comes back on the same exact version:

```text
$ shrine deploy
[shrine] Planning deployment from: /home/me/shop/manifests
🔎 Resolving image for shop.api (traefik/whoami)
  📌 Using pinned shop.api latest@3f2a9c1b4d7e (since 2026-10-01)
🔎 Resolving image for shop.cache (redis:7.4)
  🔎 Resolved shop.cache redis:7.4@e1c7a93f5b20
🔎 Resolving image for shop.shop-db (postgres)
  📌 Using pinned shop.shop-db 16@2d6f0b8e4a1c (since 2026-10-01)
…
```

No tag is consulted on a pinned redeploy, and when the exact version is already on the host, no registry is contacted at all. The pin also holds when a redeploy recreates the container.

## Rebuild the host

Prune every image, reinstall Docker, or tear the team down, and deploy again: every pinned artifact comes back on the exact version it had.

```text
$ shrine teardown shop
…
$ docker image prune -a -f
…
$ shrine deploy
[shrine] Planning deployment from: /home/me/shop/manifests
🔎 Resolving image for shop.api (traefik/whoami)
    ✅ Pulled image traefik/whoami@sha256:3f2a9c1b4d7ebacb024fcc9cc3ba71306a98135816442f1b7d6817ed226ae2e2
  📌 Using pinned shop.api latest@3f2a9c1b4d7e (since 2026-10-01)
🔎 Resolving image for shop.cache (redis:7.4)
    ✅ Pulled image redis:7.4
  🔎 Resolved shop.cache redis:7.4@e1c7a93f5b20
🔎 Resolving image for shop.shop-db (postgres)
    ✅ Pulled image postgres@sha256:2d6f0b8e4a1c5e953124e2943a0ef520833a53ec00009a6c756237ef124ab460
  📌 Using pinned shop.shop-db 16@2d6f0b8e4a1c (since 2026-10-01)
…
```

When the host no longer has a pinned exact version, Shrine pulls that exact version by digest, never the tag. `cache` is manifest-owned: its tag `7.4` is pulled again and gets whatever the registry serves under it today.

Two things a pin depends on:

- **Shrine's state directory.** Pins are recorded per team in `<state-dir>/<team>/pins.txt`, by default under `~/.local/share/shrine` (or `/var/lib/shrine`). A rebuild that keeps the state directory keeps every pin. A rebuild that loses it starts every pinned artifact fresh, at the newest version. Keep or back up the state directory when you rebuild a host.
- **The registry.** A pin can be honoured only while the registry still serves the exact version. Some registries delete images that no tag points at any more; if you rely on long-lived pins, use a registry that keeps them. When a pin cannot be honoured, see [When a pin cannot be honoured](#when-a-pin-cannot-be-honoured).

## See which version runs

`shrine get deployed`, `get applications`, and `get resources` show a VERSION column. A pinned artifact shows its readable form; a manifest-owned one shows the reference its manifest named. The table is read from Shrine's state, so it works without the container runtime:

```text
$ shrine get deployed
TEAM                 NAME                           KIND            VERSION                                  CONTAINER ID
----------------------------------------------------------------------------------------------------------------------------
shop                 api                            Application     latest@3f2a9c1b4d7e                      4c7e19a2b8d0
shop                 cache                          Resource        redis:7.4                                7a3e5c9b1d2f
shop                 shop-db                        Resource        16@2d6f0b8e4a1c                          5d0a11c3b2e4
```

Each command takes `--team` to narrow it to one team:

```text
$ shrine get resources --team shop
TEAM                 NAME                           KIND            VERSION                                  CONTAINER ID
----------------------------------------------------------------------------------------------------------------------------
shop                 cache                          Resource        redis:7.4                                7a3e5c9b1d2f
shop                 shop-db                        Resource        16@2d6f0b8e4a1c                          5d0a11c3b2e4
```

`shrine describe` shows one artifact in full. `Pinned:` is the pin: the full exact version, its readable form, and the date it was pinned. `Running image:` is the image the running container was started from:

```text
$ shrine describe resource shop-db
Name:         shop-db
Team:         shop
Kind:         Resource
Image:        postgres
Pull policy:  Pinned
Pinned:       postgres@sha256:2d6f0b8e4a1c5e953124e2943a0ef520833a53ec00009a6c756237ef124ab460 (16@2d6f0b8e4a1c, 2026-10-01)
Running image: postgres@sha256:2d6f0b8e4a1c5e953124e2943a0ef520833a53ec00009a6c756237ef124ab460
Container ID: 5d0a11c3b2e4f6a8c0d2e4f6b8a0c2e4d6f8b0a2c4e6d8f0b2a4c6e8d0f2b4a6
Config Hash:  6b8d0f2a4c6e8b0d
```

When the two agree, what runs is what is pinned. When they differ, a new pin has been recorded and is waiting for the next deploy; [Upgrade one artifact](#upgrade-one-artifact) shows that state. `Image:` is the reference as the manifest wrote it, and `Pull policy:` is the effective policy it was deployed under. A manifest-owned artifact has no `Pinned:` line.

`shrine status` reads the running containers. Its IMAGE column shows, for a pinned artifact, the repository and the first twelve characters of the exact version:

```text
$ shrine status shop
NAME                      KIND            RUNNING    STATUS       IMAGE                                    IMAGE ID
------------------------------------------------------------------------------------------------------------------------------
api                       Application     true       running      traefik/whoami@3f2a9c1b4d7e              sha256:8f3c1e5a7b92
cache                     Resource        true       running      redis:7.4                                sha256:c4a9e2f7d150
shop-db                   Resource        true       running      postgres@2d6f0b8e4a1c                    sha256:a07d3b6e1f48
```

IMAGE ID is Docker's local image id, which is not the exact version.

`describe` still answers when the container runtime cannot be reached; only the running image is missing:

```text
$ shrine describe app api
Name:         api
Team:         shop
Kind:         Application
Image:        traefik/whoami
Pull policy:  Pinned
Pinned:       traefik/whoami@sha256:3f2a9c1b4d7ebacb024fcc9cc3ba71306a98135816442f1b7d6817ed226ae2e2 (latest@3f2a9c1b4d7e, 2026-10-01)
Running image: unavailable (inspecting container "4c7e19a2b8d0f3e6a5c1d9b7e2f4a8c6d0b3e5f7a9c1d3e5f7b9d1f3a5c7e9b1": Cannot connect to the Docker daemon at tcp://127.0.0.1:1. Is the docker daemon running?)
Container ID: 4c7e19a2b8d0f3e6a5c1d9b7e2f4a8c6d0b3e5f7a9c1d3e5f7b9d1f3a5c7e9b1
Config Hash:  9f2c4e6a8b0d1f3e
```

Pins of artifacts that are not deployed are never shown: after `shrine teardown shop`, none of these commands lists the team's artifacts, even though their pins are kept.

## Upgrade one artifact

PostgreSQL 17 is out. Before you move, note the full exact version on the `Pinned:` line of `shrine describe resource shop-db`: it is what a rollback to exactly this version needs, and Shrine keeps no record of a pin once it is replaced.

Bump the database to 17. The bump checks that the version exists in the registry, records the new pin, and prints the previous and the new version. Nothing restarts:

```text
$ shrine bump resource shop-db -v 17
🔎 Resolving image for shop.shop-db (postgres:17)
    ✅ Pulled image postgres:17
  📌 Bumped shop.shop-db to 17@9c1b4d7e3f2a
Bumped shop/shop-db: 16@2d6f0b8e4a1c -> 17@9c1b4d7e3f2a; run "shrine deploy" to apply
```

The repository always comes from the manifest, so `-v` takes only a version: a readable version (a tag such as `17` or `v1.4.0`) or an exact version (`sha256:` and 64 hex characters). `describe` now shows the recorded pin and the running image apart:

```text
$ shrine describe resource shop-db
…
Pull policy:  Pinned
Pinned:       postgres@sha256:9c1b4d7e3f2abafaeca130ff41ae79c7f98018fd87e177d20f90c7d5b32c63f1 (17@9c1b4d7e3f2a, 2026-10-20)
Running image: postgres@sha256:2d6f0b8e4a1c5e953124e2943a0ef520833a53ec00009a6c756237ef124ab460
…
```

Deploy when you are ready. Only `shop-db` is recreated, on the new exact version:

```text
$ shrine deploy
[shrine] Planning deployment from: /home/me/shop/manifests
🔎 Resolving image for shop.api (traefik/whoami)
  📌 Using pinned shop.api latest@3f2a9c1b4d7e (since 2026-10-01)
🔎 Resolving image for shop.cache (redis:7.4)
  🔎 Resolved shop.cache redis:7.4@e1c7a93f5b20
🔎 Resolving image for shop.shop-db (postgres)
  📌 Using pinned shop.shop-db 17@9c1b4d7e3f2a (since 2026-10-20)
…
📦 Deploying Resource: shop-db (type: postgres)
  🌐 Ensuring network: shrine.shop.private
  🏗️  Creating container: shop.shop-db
    🔄 Image changed for shop.shop-db, replacing container...
    ✨ Creating fresh container: shop.shop-db
    ✅ Container shop.shop-db is running
```

Shrine moves the image, not the data. A database whose files change format between major versions still needs its own upgrade procedure; for PostgreSQL that is `pg_upgrade` or a dump and restore. The same is true when going back.

A version the registry does not serve is rejected by the bump, so a typo never reaches a deploy. The pin on record is unchanged and no container is touched:

```text
$ shrine bump resource shop-db -v 71
…
Error: resource "shop-db": pulling image "postgres:71": Error response from daemon: manifest for postgres:71 not found: manifest unknown: manifest unknown
…
```

A value that is not a version is refused before any registry is contacted:

```text
$ shrine bump resource shop-db -v postgres:18
Error: invalid version "postgres:18": use a tag (a letter, digit, or underscore, then up to 127 letters, digits, underscores, dots, or dashes) or an exact version "sha256:<64 hex>"
…
```

`--dry-run` shows what the bump would resolve, and contacts no registry and writes nothing:

```text
$ shrine bump resource shop-db -v 17 --dry-run
[dry-run] would resolve postgres:17 and pin shop/shop-db
```

Only artifacts under `Pinned` can be bumped. The version of a manifest-owned artifact is changed by editing its manifest:

```text
$ shrine bump resource cache -v 8
Error: resource "cache": its version is manifest-owned (imagePullPolicy IfNotPresent); edit the manifest to change it
…
```

A bump looks the artifact up in the manifest directory, so it works for an artifact that was never deployed, as `shop-db` showed in [Pin a service at its newest version](#pin-a-service-at-its-newest-version). A name with no manifest is refused with the directory searched.

## Roll back

Rolling back is the same command, pointed at the earlier version. There are two ways to name it:

- **The readable version**, `shrine bump resource shop-db -v 16`. This resolves whatever tag `16` points at today. A registry often moves a major tag to each new minor release, so this may be a newer 16.x than the one you had.
- **The exact version**, `shrine bump resource shop-db -v sha256:…` with all 64 hex characters. This returns to exactly what ran before. Use the value you noted from `describe` before the upgrade; the bump's output shows only its first twelve characters (`16@2d6f0b8e4a1c` above). If you did not note it, `docker image inspect` lists it under `RepoDigests` for as long as the old image is still on the host.

```text
$ shrine bump resource shop-db -v sha256:2d6f0b8e4a1c5e953124e2943a0ef520833a53ec00009a6c756237ef124ab460
🔎 Resolving image for shop.shop-db (postgres@sha256:2d6f0b8e4a1c5e953124e2943a0ef520833a53ec00009a6c756237ef124ab460)
    ✅ Pulled image postgres@sha256:2d6f0b8e4a1c5e953124e2943a0ef520833a53ec00009a6c756237ef124ab460
  📌 Bumped shop.shop-db to 2d6f0b8e4a1c
Bumped shop/shop-db: 17@9c1b4d7e3f2a -> 2d6f0b8e4a1c; run "shrine deploy" to apply
```

A pin taken from an exact version has no readable version to show, so its readable form is the twelve characters alone. Deploy to apply it:

```text
$ shrine deploy
…
🔎 Resolving image for shop.shop-db (postgres)
  📌 Using pinned shop.shop-db 2d6f0b8e4a1c (since 2026-10-21)
…
```

## Take the newest again

Months later you want whatever `traefik/whoami` publishes now. Bump without `-v` and the newest version of the manifest's image is resolved and recorded:

```text
$ shrine bump app api --dry-run
[dry-run] would resolve traefik/whoami and pin shop/api
```

```text
$ shrine bump app api
🔎 Resolving image for shop.api (traefik/whoami)
    ✅ Pulled image traefik/whoami
  📌 Bumped shop.api to latest@5e8c2b7a1f04
Bumped shop/api: latest@3f2a9c1b4d7e -> latest@5e8c2b7a1f04; run "shrine deploy" to apply
```

The next deploy applies it. A pinned artifact takes the newest version only when you bump it like this, never as a side effect of a redeploy.

## Make pinning the house rule

To make every manifest that names no policy pinned, set the default once in `config.yml`:

```yaml
specsDir: ~/shop/manifests
imagePullPolicy: Pinned
```

Manifests that name no policy are now held to the `Pinned` rules. A manifest that names no policy but does name a fixed version now fails, and the next deploy says so before it changes anything. Every such manifest is listed, with the setting it came from and the two ways out:

```text
$ shrine deploy
[shrine] Planning deployment from: /home/me/shop/manifests
Validation errors:
resource "cache": spec.version "7.4" names a fixed version but the image pull policy is Pinned (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default
Error: Spec validation errors
…
```

Nothing was deployed. For `cache`, choose one:

- **Keep the version manifest-owned**: add `imagePullPolicy: IfNotPresent` to `cache.yml`. A manifest's own field always wins over the default.
- **Let Shrine own it**: drop the `version: "7.4"` line, and the next deploy pins the newest Redis.

After the first way out, the deploy goes ahead and `cache` stays manifest-owned:

```text
$ shrine deploy --dry-run
…
[DOCKER] ImageResolve: name=shop.cache image=redis:7.4 policy=IfNotPresent -> manifest-owned
…
```

`shrine generate` follows the default. Under `Pinned` it writes a repository with no tag and no `version` line, so the manifest it writes is valid under the default. Replace the placeholder image `web` with your own repository, still without a tag:

```text
$ shrine generate application web --team shop
Created application manifest: /home/me/shop/manifests/web.yml
```

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: web
  owner: shop
spec:
  image: web
  port: 8080
  replicas: 1
  routing:
    domain: web.shrine.lab
    pathPrefix: /web
  networking:
    exposeToPlatform: false
```

`shrine generate resource` likewise leaves out the `version` line.

Changing the default later is as consequential as editing every manifest that names no policy. If you remove it, or set it to `Always` or `IfNotPresent`, every artifact it pinned becomes manifest-owned and its pin is released on its next deploy. A resource that relied on `Pinned` to omit its `version` must name one again. The setting is described in the README's [Configuration](https://github.com/CarlosHPlata/shrine#configuration) section.

## Retire an artifact

Tearing a team down keeps its pins. Change your mind and deploy again, and every artifact comes back on the version it had, with its original pin date:

```text
$ shrine teardown shop
…
$ shrine deploy
[shrine] Planning deployment from: /home/me/shop/manifests
🔎 Resolving image for shop.api (traefik/whoami)
  📌 Using pinned shop.api latest@5e8c2b7a1f04 (since 2027-01-15)
🔎 Resolving image for shop.cache (redis:7.4)
  🔎 Resolved shop.cache redis:7.4@e1c7a93f5b20
🔎 Resolving image for shop.shop-db (postgres)
  📌 Using pinned shop.shop-db 2d6f0b8e4a1c (since 2026-10-21)
…
```

To retire one artifact for good, delete it. A delete is refused while the artifact's container exists:

```text
$ shrine delete resource shop-db
Error: resource "shop/shop-db" still has a container; run "shrine teardown shop" first
…
```

Run `shrine teardown shop` first. Then preview the delete, and run it:

```text
$ shrine delete resource shop-db --dry-run
[dry-run] would release image pin postgres@sha256:2d6f0b8e4a1c5e953124e2943a0ef520833a53ec00009a6c756237ef124ab460 for shop/shop-db
```

```text
$ shrine delete resource shop-db
Released image pin for shop/shop-db.
```

The pin goes with the delete, so a future resource named `shop-db` starts fresh: its first deploy resolves the newest version again. `shrine delete application <name>` does the same for an application, and `shrine delete team <name>` releases every pin the team still holds (it has no `--dry-run`):

```text
$ shrine delete team shop
Released 1 image pin(s) for team "shop".
Deleted team "shop" from state.
```

Exactly four things release a pin:

- `shrine delete application <name>`
- `shrine delete resource <name>`
- `shrine delete team <name>`
- a deploy of the artifact under `Always` or `IfNotPresent`, which makes its version manifest-owned again; returning it to `Pinned` later is a first deploy, which pins the newest

A redeploy, a recreated container, and a teardown never release a pin.

## When a pin cannot be honoured

If neither the host nor the registry still has a pinned exact version, the deploy stops before any container or network is touched, names the artifact and the exact version, and gives the `shrine bump` command to run. See [A deploy stops because a pinned version is no longer served](/troubleshooting/#a-deploy-stops-because-a-pinned-version-is-no-longer-served).

## What this does not do

- **No version ranges.** A bump sets a version; it never increments one or picks "the next minor".
- **No automatic moves.** A bump records the new version, and only the next deploy applies it. Nothing moves a pinned version on its own.
- **One artifact per bump.** There is no team-wide or all-artifacts bump.
- **Not the gateway.** The Traefik gateway's own image comes from configuration, not a manifest, and keeps its fixed default.
- **Registries only.** A pin records an exact version, which is a registry digest. An image that was built or loaded locally and never pushed has no registry digest and cannot be pinned.

## Where to read more

- [Image pull policy](/reference/manifest-schema/#image-pull-policy): the three values, the no-fixed-version rule, and the pin lifecycle, field by field.
- [Configuration](https://github.com/CarlosHPlata/shrine#configuration) in the README: the `imagePullPolicy` default.
- Commands: [`shrine bump`](/cli/bump/) ([application](/cli/bump_application/), [resource](/cli/bump_resource/)), [`shrine delete resource`](/cli/delete_resource/), [`shrine describe resource`](/cli/describe_resource/), [`shrine get deployed`](/cli/get_deployed/), [`shrine status`](/cli/status/), [`shrine generate application`](/cli/generate_application/).
- [Troubleshooting](/troubleshooting/#a-deploy-stops-because-a-pinned-version-is-no-longer-served): a pinned version the registry no longer serves.
