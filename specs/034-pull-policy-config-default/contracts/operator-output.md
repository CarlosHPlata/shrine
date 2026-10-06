# Contract: operator-visible output, errors, and generated text

**Feature**: 034-pull-policy-config-default

## Configuration error (every command, before anything runs)

stderr, exit non-zero:

```text
Error: loading config: imagePullPolicy: must be one of Always, IfNotPresent, Pinned
```

## Validation errors (planning, before any change)

Printed under `Validation errors:` on stderr with the manifest set's other errors, exit non-zero. The stem is T3's; the ending depends on where the policy came from.

Configuration-sourced (the manifest names no policy and `config.yml` sets `imagePullPolicy: Pinned`):

```text
application "app-fixed": spec.image "traefik/whoami:v1.10.2" names a fixed version but the image pull policy is Pinned (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default
resource "res-fixed": spec.version "16" names a fixed version but the image pull policy is Pinned (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default
resource "res-override": spec.image "postgres:16" names a fixed version but the image pull policy is Pinned (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default
```

Manifest-sourced (the manifest names `Pinned` itself), unchanged from T3 under every default:

```text
application "app-own-pinned-fixed": spec.image "traefik/whoami:v1.10.2" names a fixed version but the image pull policy is Pinned; use "traefik/whoami" or "traefik/whoami:latest"
resource "cache-fixed": spec.version "16" names a fixed version but the image pull policy is Pinned; omit it or use "latest"
```

Version required (effective policy `Always` or `IfNotPresent`, from any source, and no `version`), unchanged:

```text
resource "res-noversion": spec.version is required
```

Exactly one message per offending field; a Resource whose derived `<type>` image is not an override is reported once, on `spec.version`.

## Dry-run lines (`deploy --dry-run`)

Unchanged shapes from T2 and T3; the `policy=` value now reflects the configuration default when the manifest names none:

```text
[DOCKER] ImageResolve: name=shrine-deploy-test.app-latest image=traefik/whoami:latest policy=IfNotPresent -> manifest-owned
[DOCKER] ImageResolve: name=shrine-deploy-test.app-untagged image=traefik/whoami policy=Pinned -> would resolve newest and pin
[DOCKER] ImageResolve: name=shrine-deploy-test.res-noversion image=traefik/whoami policy=Pinned -> would resolve newest and pin
```

Dry run writes nothing: no `pins.txt` appears and recorded state is byte for byte unchanged.

## Deploy output

Unchanged from T3. An artifact pinned because of the configuration default prints the same `📌 Pinned <team>.<name> at latest@<12 hex>` and `📌 Using pinned …` lines as one that names `Pinned` itself; a manifest-owned artifact prints T2's lines. Nothing in the output names the source of the policy.

## Generated manifests

`shrine generate application web` with no `--image`:

| Default | `spec.image` line |
|---|---|
| absent, `Always`, `IfNotPresent` | `  image: web:latest` |
| `Pinned` | `  image: web` |

`shrine generate resource db` with no `--version`:

| Default | after `  type: postgres` |
|---|---|
| absent, `Always`, `IfNotPresent` | `  version: "16"` then `  networking:` |
| `Pinned` | `  networking:` directly; no `version:` line |

With `--image` or `--version` given, the value appears verbatim regardless of the default. No generated manifest contains `imagePullPolicy:`. Apart from the two lines above, both skeletons are byte for byte today's text, including the commented `env` and `outputs` guidance in the resource skeleton.

## Flag help (`--help`, and the regenerated CLI pages)

```text
      --image string        Docker image to run (defaults to [name]:latest, or [name] when imagePullPolicy in config.yml is Pinned)
      --version string      Version of the resource (defaults to 16; omitted when imagePullPolicy in config.yml is Pinned)
```

The `--version` line no longer ends in `(default "16")`.
