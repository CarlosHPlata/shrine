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
| `<project>` | path[0] | Infisical project **name** (e.g. `shrine-test`), slug, or UUID. Plugin resolves any of the three via a one-time `/api/v1/workspace` lookup; the result is cached for the process lifetime. UUIDs skip the lookup entirely. |
| `<environment>` | path[1] | Infisical environment slug (e.g. `production`, `staging`) |
| `<secret-name>` | path[2] | Infisical secret key (e.g. `DB_PASSWORD`) |

**Full example**: `valueFrom: vault:myapp/production/DB_PASSWORD`

### Constraints

- Path must contain exactly 3 `/`-separated non-empty components. Validated at plan time.
- `valueFrom: vault:...` is mutually exclusive with `value:`, `template:`, and `generated:` on the same key (env var or output). Enforced by manifest validation.
- `valueFrom: vault:` is valid in both Application `spec.env[]` and Resource `spec.outputs[]`.

---

## Resource Manifest — `spec.outputs[*].valueFrom`

Resource outputs can also reference vault secrets. The resolved value is stored as a named output and available to downstream Applications via `valueFrom: resource.<name>.<output>`.

```yaml
apiVersion: shrine/v1
kind: Resource
metadata:
  name: mydb
  owner: myteam
spec:
  type: postgres
  version: "16"
  outputs:
    - name: password
      valueFrom: vault:myproject/production/DB_PASSWORD   # fetched from vault

    - name: api-key
      valueFrom: vault:myproject/production/API_KEY       # fetched from vault

    - name: host                                          # built-in output
    - name: port
      value: "5432"                                       # composable with vault outputs
```

Applications consume vault-backed resource outputs exactly as they would any other output:

```yaml
env:
  - name: DATABASE_URL
    valueFrom: resource.mydb.password   # resolved value came from vault
```

### Resource Output Constraints

- `valueFrom: vault:` on a resource output is mutually exclusive with `value:`, `generated:`, and `template:` on the same output name. Validated at manifest parse time.
- Path format and plan-time validation rules are identical to Application env vars.

### Dry-run output

When `shrine dry-run` is used, vault refs render as placeholders:

```
DB_PASSWORD=[VAULT:myapp/production/DB_PASSWORD]
API_KEY=[VAULT:myapp/production/API_KEY]
```

No network connection to the vault is made.

---

## Published Documentation

This contract is the source of truth for implementation. The following docs files expose these contracts to operators:

- **`docs/content/guides/secrets-vault.md`** — user-facing guide covering all config fields, manifest syntax, and dry-run behaviour described in this contract.
- **`docs/content/reference/manifest-schema.md`** — the `spec.env[].valueFrom` table is updated to document `vault:<path>` as a valid form alongside `resource.<name>.<output>`.
