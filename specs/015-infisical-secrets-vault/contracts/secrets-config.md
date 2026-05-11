# Contract: Secrets Plugin Configuration

## shrine.yml — `plugins.secrets`

```yaml
plugins:
  secrets:
    infisical:              # only one secrets plugin block allowed
      url: <string>         # required — self-hosted Infisical base URL (e.g. http://infisical:8080)
      client-id: <string>   # required — Machine Identity Universal Auth client ID
      client-secret: <string> # required — Machine Identity Universal Auth client secret
```

### Constraints

- At most one key under `plugins.secrets` is allowed. Declaring multiple (e.g. both `infisical` and a future `other`) is a config load error.
- All three fields (`url`, `client-id`, `client-secret`) are required when the `infisical` block is present. Missing fields are a config load error.
- `url` is accepted as-is with no protocol enforcement. Both `http://` and `https://` are valid.

---

## Application Manifest — `spec.env[*].valueFrom`

```yaml
apiVersion: shrine/v1
kind: Application
metadata:
  name: myapp
  team: myteam
spec:
  env:
    - name: DB_PASSWORD
      valueFrom: vault:myproject/production/DB_PASSWORD

    - name: API_KEY
      valueFrom: vault:myproject/production/API_KEY

    - name: STATIC_VALUE
      value: "hello"              # composable with vault refs on other keys

    - name: GENERATED_TOKEN
      generated: true             # composable with vault refs on other keys
```

### `valueFrom: vault:<path>` syntax

| Component | Position | Description |
|---|---|---|
| `vault:` | prefix | Required literal prefix identifying a vault reference |
| `<project>` | path[0] | Infisical project slug (e.g. `myapp`) |
| `<environment>` | path[1] | Infisical environment slug (e.g. `production`, `staging`) |
| `<secret-name>` | path[2] | Infisical secret key (e.g. `DB_PASSWORD`) |

**Full example**: `valueFrom: vault:myapp/production/DB_PASSWORD`

### Constraints

- Path must contain exactly 3 `/`-separated non-empty components. Validated at plan time.
- `valueFrom: vault:...` is mutually exclusive with `value:`, `template:`, and `generated:` on the same env key. Enforced by existing manifest validation (no new logic required).
- `vault:` refs in Resource manifests are not supported in v1.

### Dry-run output

When `shrine dry-run` is used, vault refs render as placeholders:

```
DB_PASSWORD=[VAULT:myapp/production/DB_PASSWORD]
API_KEY=[VAULT:myapp/production/API_KEY]
```

No network connection to the vault is made.
