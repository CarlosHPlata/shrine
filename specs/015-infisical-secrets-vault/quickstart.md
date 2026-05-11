# Quickstart: Secrets Vault Plugin (Infisical)

## Prerequisites

- A running self-hosted Infisical instance (see [Testing Setup](spec.md#testing-setup-infisical-docker-compose))
- A Machine Identity created in Infisical with read access to the relevant project and environment
- Shrine CLI installed

## 1. Add the plugin to shrine.yml

```yaml
plugins:
  secrets:
    infisical:
      url: http://infisical:8080          # your self-hosted Infisical URL
      client-id: "your-client-id"
      client-secret: "your-client-secret"
```

## 2. Reference vault secrets in an Application manifest

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: myapp
  owner: myteam
spec:
  image: myapp:latest
  port: 8080
  env:
    - name: DB_PASSWORD
      valueFrom: vault:myproject/production/DB_PASSWORD
    - name: API_KEY
      valueFrom: vault:myproject/production/API_KEY
    - name: APP_ENV
      value: production
```

Path format: `vault:<project-slug>/<environment-slug>/<secret-name>`

## 3. Validate with dry-run (no vault connection needed)

```bash
shrine dry-run --path ./manifests
```

Vault refs render as placeholders — no network call is made:
```
DB_PASSWORD=[VAULT:myproject/production/DB_PASSWORD]
API_KEY=[VAULT:myproject/production/API_KEY]
APP_ENV=production
```

## 4. Deploy

```bash
shrine deploy --path ./manifests
```

Shrine fetches all vault secrets upfront, then starts containers. If any secret is missing or credentials are invalid, the deploy aborts before any container is touched.

## Common errors

| Error | Cause | Fix |
|---|---|---|
| `vault ref "vault:foo": no secrets plugin configured` | Missing `plugins.secrets.infisical` in shrine.yml | Add the plugin block |
| `vault: path must have 3 components` | Malformed `valueFrom: vault:foo/bar` (only 2 parts) | Use `vault:project/env/key` format |
| `failed to authenticate with Infisical` | Wrong client-id / client-secret, or vault unreachable | Check credentials and URL |
| `secret "DB_PASSWORD" not found` | Secret does not exist in the specified project/environment | Create it in Infisical UI |

## See also

- [Secrets vault guide](../../docs/content/guides/secrets-vault.md)
- [Secrets config contract](contracts/secrets-config.md)
- [Data model](data-model.md)
