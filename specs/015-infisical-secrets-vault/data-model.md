# Data Model: Secrets Vault Plugin (Infisical)

## New Types

### `SecretsPlugin` interface — `internal/plugins/secrets/plugin.go`

```go
type SecretsPlugin interface {
    IsActive() bool
    GetSecret(path string) (string, error)
}
```

- `IsActive()` returns false when the plugin is nil or its config is absent; used by handlers to skip vault wiring entirely.
- `GetSecret(path string)` fetches the secret at the given opaque path string. For Infisical, `path` is `"project/environment/secret-name"`. Returns the plaintext value or a descriptive error (path included, value never logged).

---

### `InfisicalPlugin` — `internal/plugins/secrets/infisical/plugin.go`

```go
type InfisicalPlugin struct {
    cfg    *config.InfisicalPluginConfig
    client infisical.InfisicalClientInterface
}

func New(cfg *config.InfisicalPluginConfig) (*InfisicalPlugin, error)
func (p *InfisicalPlugin) IsActive() bool
func (p *InfisicalPlugin) GetSecret(path string) (string, error)
```

`New()` initialises and authenticates the Infisical SDK client. Returns an error if authentication fails. `GetSecret` splits `path` on `/` into `[project, environment, secretKey]` and calls the SDK `Retrieve` method.

---

### `InfisicalPluginConfig` — `internal/config/plugin_infisical.go`

```go
type InfisicalPluginConfig struct {
    URL          string `yaml:"url"`
    ClientID     string `yaml:"client-id"`
    ClientSecret string `yaml:"client-secret"`
}
```

Maps directly to the shrine.yml block:
```yaml
plugins:
  secrets:
    infisical:
      url: http://infisical:8080
      client-id: "abc123"
      client-secret: "xyz789"
```

---

### Updated `PluginsConfig` — `internal/config/config.go`

```go
type SecretsPluginsConfig struct {
    Infisical *InfisicalPluginConfig `yaml:"infisical,omitempty"`
}

type PluginsConfig struct {
    Gateway GatewayPluginsConfig `yaml:"gateway,omitempty"`
    Secrets SecretsPluginsConfig `yaml:"secrets,omitempty"`  // new
}
```

Config validation (in `config.Load()` or a new `ValidateSecretsPlugins()`) counts non-nil secrets plugin fields and errors if count > 1.

---

### Updated `LiveResolver` — `internal/resolver/resolver.go`

```go
type LiveResolver struct {
    Secrets state.SecretStore
    Vault   secrets.SecretsPlugin  // nil when no vault configured
}

func NewLiveResolver(store state.SecretStore, vault secrets.SecretsPlugin) Resolver
```

`lookupValueFrom` is extended with a `vault:` branch:
```go
case isVaultRef(valueFrom):
    if r.Vault == nil || !r.Vault.IsActive() {
        return "", fmt.Errorf("vault ref %q: no secrets plugin configured", valueFrom)
    }
    return r.Vault.GetSecret(parseVaultPath(valueFrom))
```

Helpers (private, in `resolver.go`):
- `isVaultRef(s string) bool` — `strings.HasPrefix(s, "vault:")`
- `parseVaultPath(s string) string` — `strings.TrimPrefix(s, "vault:")`

---

### Updated `DryRunResolver` — `internal/resolver/dry_run_resolver.go`

`lookupValueFrom` (or equivalent placeholder logic) extended:
```go
case isVaultRef(valueFrom):
    return fmt.Sprintf("[VAULT:%s]", parseVaultPath(valueFrom)), nil
```

No vault call is made; `isVaultRef` and `parseVaultPath` are shared helpers (defined once in `resolver.go`, accessible to `dry_run_resolver.go` in the same package).

---

### `VaultSecretRef` (logical concept, not a standalone type)

A vault reference is a `valueFrom` string with the `vault:` prefix, e.g. `vault:myproject/production/DB_PASSWORD`. It is:

- **Parsed** in `resolver.go` via `isVaultRef` / `parseVaultPath`
- **Validated** at plan time in `planner/resolve.go`: the path after `vault:` must split into exactly 3 non-empty components; otherwise a plan-time error is returned
- **Never stored** as a struct — it remains a plain string throughout the pipeline

---

## Validation Rules

| Location | Check | Error |
|---|---|---|
| `manifest/validate.go` | No change needed — `valueFrom` accepts any non-empty string | — |
| `planner/resolve.go` | `vault:` path must have exactly 3 `/`-separated non-empty parts | plan-time error |
| `config.Load()` | At most one `plugins.secrets.*` block | config load error |
| `InfisicalPlugin.New()` | Auth must succeed at construction | startup error |
| `LiveResolver.lookupValueFrom` | Vault plugin must be active if `vault:` ref present | resolve error |

## State & Caching

No secret values are cached. Each `shrine apply` invocation fetches all vault-referenced secrets fresh from the vault. This is consistent with the constitution rule: "No in-memory caching of secret values — always read from disk" (here: always read from vault).
