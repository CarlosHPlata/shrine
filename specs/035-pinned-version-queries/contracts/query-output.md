# Contract: operator-visible output of `get`, `describe`, and `status`

**Feature**: 035-pinned-version-queries

All on stdout, exit zero, unless stated. `<12hex>` is the first twelve hexadecimal characters of the exact version. Examples use the loopback registry fixture (`127.0.0.1:<port>/shrine/whoami`) and the `basic` fixture (`traefik/whoami`).

## `shrine get deployed`, `get applications`, `get resources` (with or without `--team`)

Header and widths unchanged from T1:

```text
TEAM                 NAME                           KIND            VERSION                                  CONTAINER ID
--------------------------------------------------------------------------------------------------------------------------
shrine-deploy-test   whoami-pinned                  Application     latest@3f2a9c1b4d7e                      9f1c0e2d4b6a
shrine-deploy-test   cache-pinned                   Resource        latest@3f2a9c1b4d7e                      5d0a11c3b2e4
shrine-deploy-test   whoami                         Application     traefik/whoami                           a1b2c3d4e5f6
```

| Row | VERSION |
|---|---|
| record policy `Pinned`, pin present, resolved from a tag or untagged reference | `<tag>@<12hex>`; `latest` when the reference had no tag |
| record policy `Pinned`, pin resolved from a digest reference | `<12hex>` |
| record policy `Pinned`, no pin on record | the recorded manifest reference, as a manifest-owned row |
| record policy `Always` or `IfNotPresent` | the recorded manifest reference, alias unexpanded (T1) |
| legacy record without image and policy | `-` (T1) |

No Docker call is made; the commands succeed with the daemon stopped. `No deployments found.`, `No applications deployed.`, and `No resources deployed.` are unchanged.

## `shrine describe app <name>`, `describe resource <name>` (with or without `--team`)

Pinned artifact, deployed and unchanged since:

```text
Name:         whoami-pinned
Team:         shrine-deploy-test
Kind:         Application
Image:        127.0.0.1:5000/shrine/whoami
Pull policy:  Pinned
Pinned:       127.0.0.1:5000/shrine/whoami@sha256:3f2a9c1b4d7e<52 more hex> (latest@3f2a9c1b4d7e, 2026-10-07)
Running image: 127.0.0.1:5000/shrine/whoami@sha256:3f2a9c1b4d7e<52 more hex>
Container ID: 9f1c0e2d4b6a8c7e5f3a…
Config Hash:  3a7b…
```

Same artifact after its pin was replaced without a deploy: `Pinned:` carries the new digest and date, `Running image:` still the old digest. The difference is the two lines disagreeing; no marker is printed.

Manifest-owned artifact (no `Pinned:` line; the running image is the expanded reference):

```text
Image:        traefik/whoami
Pull policy:  Always
Running image: docker.io/traefik/whoami:latest
```

Note: the `Running image:` label is fourteen characters and overruns the twelve-character label column by two; the value follows one space after the colon. The exact spelling is `Running image: ` and tests anchor on `Running image:`.

| Case | Line |
|---|---|
| `Pinned` record, pin present | `Pinned:       <pin.Pinned> (<readable>, <YYYY-MM-DD>)` |
| `Pinned` record, no pin | `Pinned:       -` |
| `Always` / `IfNotPresent` / legacy record | no `Pinned:` line |
| container inspected, image known | `Running image: <reference as created from>` |
| container inspected, image empty (dry-run backend) | `Running image: -` |
| container runtime unreachable or container gone | `Running image: unavailable (<error text>)`; exit zero |
| no container runtime handed to the handler | `Running image: unavailable (no container runtime)`; exit zero |

Error cases unchanged (stderr, exit non-zero): `application "x" not found in team "t"`, `application "x" not found in any team`, `ambiguous: application "x" found in teams [a, b], use --team to disambiguate`. A torn-down artifact hits the not-found case and no pin is printed.

## `shrine status [team]`, `status application <name>`, `status resource <name>`

```text
NAME                      KIND            RUNNING    STATUS       IMAGE                                    IMAGE ID
-----------------------------------------------------------------------------------------------------------------------------
whoami-pinned             Application     true       running      127.0.0.1:5000/shrine/whoami@3f2a9c1b4d7e sha256:3f2a9c1b4d7e
whoami                    Application     true       running      docker.io/traefik/whoami:latest          sha256:a1b2c3d4e5f6
```

| Container created from | IMAGE |
|---|---|
| a digest reference (pinned) | `<repository>@<12hex>` |
| a tag reference (manifest-owned) | the reference as is |
| unknown (empty) | `-` |

IMAGE sits between STATUS and IMAGE ID; the five existing columns keep their headers, order, widths, and values; the separator grows to the header length. `status` still fails when the runtime is unreachable, as today. `Team: <name>` headers and `No deployments found for team "x".` are unchanged.

## What never appears

No command prints a pin for an artifact without a deployment record, and no command lists `pins.txt` on its own. After `shrine teardown <team>`, `get` shows `No deployments found.` for that team, `describe` reports not found, and `status <team>` reports no deployments, while `pins.txt` keeps its lines for the next deploy.

## Deploy, dry run, apply, delete, teardown

Unchanged. The terminal's `📌 Pinned …` and `📌 Using pinned …` lines keep their exact text; the readable helper moved packages without changing output.

## Flag help and CLI pages (regenerated by `make docs-gen-cli`)

`describe app` / `describe resource` `Long` gains, after the existing text:

```text
The record shows the image the manifest named and the effective pull policy.
Under the Pinned policy it also shows the pin (exact version, readable version,
and date) and, when the container runtime can be reached, the image the running
container was started from; a pin that differs from the running image has been
recorded but not yet deployed.
```

`status`, `status application`, `status resource` `Long` gains:

```text
The IMAGE column shows the image each container was started from: the exact
version for a Pinned artifact, the tag reference otherwise.
```

No flag is added or changed.
