---
title: "Secrets vault"
description: "Store secrets in an external vault and reference them from manifests."
weight: 20
---

## What this guide covers

How to enable the Shrine secrets vault plugin, configure Infisical as the provider, reference vault-stored secrets in Resource outputs and Application env vars, and understand dry-run behaviour.

## Concept

Shrine manifests are designed to be checked into version control, which means plain-text passwords must never appear inside them. The secrets vault integration solves this with a provider-agnostic `vault:` prefix: any field that supports dynamic values can instead name a secret path, and Shrine fetches the real value at deploy time.

The design separates concerns cleanly:

- **Manifests** carry only the logical path (`vault:<project>/<environment>/<secret-name>`). They are provider-agnostic and safe to commit.
- **Config** (`~/.config/shrine/config.yml` or the path passed via `--config`) selects the provider and supplies the credentials. Currently the only supported provider is Infisical.

When the `plugins.secrets.infisical` block is absent, any `vault:` reference in a deployed manifest produces the error "no secrets plugin configured" at plan time, before any container starts.

## Enable the vault

Add the plugin section to your config file:

```yaml
plugins:
  secrets:
    infisical:
      url: https://app.infisical.com   # Infisical instance URL (self-hosted or cloud)
      client-id: <machine-identity-id>
      client-secret: <machine-identity-secret>
```

All three fields are required. `url` must point to the Infisical instance (cloud or self-hosted). `client-id` and `client-secret` are the credentials for a [Machine Identity](https://infisical.com/docs/documentation/platform/identities/machine-identities) that has read access to the relevant projects and environments.

Then run:

```bash
shrine deploy --path ./manifests
```

Shrine authenticates against Infisical once per deploy, fetches each referenced secret, and injects the value into the container environment.

## Reference a vault secret in an Application manifest

Use `valueFrom: vault:<project>/<environment>/<secret-name>` inside `spec.env[]`:

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: api
  owner: platform
spec:
  image: my-api:1.2.3
  port: 8080
  env:
    - name: API_KEY
      valueFrom: vault:my-project/production/API_KEY
    - name: STRIPE_SECRET
      valueFrom: vault:my-project/production/STRIPE_SECRET_KEY
```

The path has exactly three slash-separated components: project slug, environment slug, and secret name. A path with a different number of components is rejected at plan time with an explicit error.

`value`, `valueFrom`, and `template` are mutually exclusive on a single env entry. Combining `vault:` with any other source is a plan-time validation error.

## Reference a vault secret in a Resource output

A Resource output can also carry a `vault:` reference so that downstream Applications receive the resolved value via the normal `resource.<name>.<output>` mechanism:

```yaml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: app-db
  owner: platform
spec:
  type: postgres
  version: "16"
  outputs:
    - name: host
    - name: port
      value: "5432"
    - name: password
      valueFrom: vault:my-project/production/DB_PASSWORD
```

A dependent Application consumes the output with the standard `resource.` reference — no vault path appears in the Application manifest at all:

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: api
  owner: platform
spec:
  image: my-api:1.2.3
  port: 8080
  dependencies:
    - kind: Resource
      name: app-db
      owner: platform
  env:
    - name: DB_PASSWORD
      valueFrom: resource.app-db.password
```

At deploy time Shrine resolves the Resource output from the vault, stores it in the deployment state, and then resolves the Application's `resource.app-db.password` reference from that state — the Application never contacts the vault directly.

`valueFrom` on a Resource output is mutually exclusive with `value`, `generated`, and `template`.

## Dry-run behaviour

When you run `shrine deploy --dry-run`, Shrine does not contact the vault. Any `vault:` reference is rendered as the placeholder `[VAULT:<path>]` in the plan output so you can review the deployment plan without requiring network connectivity or valid credentials.

Example dry-run output:

```
env DB_PASSWORD=[VAULT:my-project/production/DB_PASSWORD]
env API_KEY=[VAULT:my-project/production/API_KEY]
```

This lets you validate manifest structure and dependency wiring in CI lint jobs or on developer laptops that have no vault access.

## Common pitfalls

| Situation | Error |
|-----------|-------|
| `vault:` ref in manifest but no `plugins.secrets.infisical` block in config | `no secrets plugin configured` (plan-time) |
| Malformed path — not exactly three `/`-separated components | Plan-time validation error naming the offending field |
| Wrong `client-id` or `client-secret` | Authentication error at deploy startup, before any container is started |
| Project slug or environment slug does not exist in Infisical | Explicit error at deploy time: path not found, with the full path printed |
| `value:` and `valueFrom: vault:` on the same output or env entry | Mutual-exclusion validation error at plan time |

## See also

- [Manifest schema reference](/reference/manifest-schema/)
