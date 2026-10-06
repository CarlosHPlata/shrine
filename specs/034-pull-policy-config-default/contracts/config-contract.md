# Contract: the configuration key and how it reaches the code

**Feature**: 034-pull-policy-config-default

## `config.yml`

```yaml
imagePullPolicy: Pinned       # optional; one of Always, IfNotPresent, Pinned; absent means the derived rule
```

| Rule | Behaviour |
|---|---|
| key absent, or present with an empty value | `Config.ImagePullPolicy == ""`; no default; identical to before this feature |
| `Always`, `IfNotPresent`, `Pinned` | the default for every manifest that names no `spec.imagePullPolicy` |
| any other string, including `pinned`, `always`, `Never`, ` Pinned` | `config.Load` returns `imagePullPolicy: must be one of Always, IfNotPresent, Pinned` |
| file absent or empty | as today: empty `Config`, no error |

`Load` validates after unmarshalling and after `validateSecretsPlugins`. Because `cmd/root.go` calls `Load` in `PersistentPreRunE`, every subcommand fails with `Error: loading config: imagePullPolicy: must be one of Always, IfNotPresent, Pinned` and does nothing else. `--config-dir` and `SHRINE_CONFIG_DIR` select which file is read, as for every other key.

## Precedence (unchanged rule; the middle layer is now live)

For every Application and Resource at plan time, in `applyEffectivePullPolicy`:

1. `spec.imagePullPolicy` when non-empty;
2. else `Config.ImagePullPolicy` when non-empty;
3. else `Always` for `latest` or no tag, `IfNotPresent` for any other tag or a digest reference.

The same value is used by `deploy`, `deploy team`, `deploy --dry-run`, and `apply -f`, because all three handlers pass `cfg.ImagePullPolicy` to `planner.Plan`. The engine, the backends, the pin store, the deployment record, and the terminal see only the effective value.

## Source record

`applyEffectivePullPolicy` records which layer supplied the policy for each artifact on the `ManifestSet` (unexported). `validateImagePolicies` reads it to choose the message ending. Nothing else reads it; it is never written to disk.

## Pin lifecycle under a changed default (no new code)

| Default before | Default after | Manifest names no policy | Next deploy |
|---|---|---|---|
| `Pinned` | `IfNotPresent`, `Always`, or absent | yes | the artifact is manifest-owned; `resolveManifestOwned` releases its pin (TD-6); a Resource without `version` now fails `spec.version is required` |
| `IfNotPresent`, `Always`, or absent | `Pinned` | yes | the artifact is `Pinned`; `pinNewest` records a pin as on a first deploy, after validation of the no-fixed-version rule |
| any | any | no (the manifest sets the field) | unchanged |

## Generate

`cmd/generate.go` passes `PullPolicy: cfg.ImagePullPolicy` on `AppOptions` and `ResourceOptions`, and passes `Image` and `Version` exactly as typed (empty when the flag is absent). The handler fills the defaults:

| `PullPolicy` | `Image` when empty | `Version` when empty |
|---|---|---|
| `Pinned` | `<name>` | `""` (the `version:` line is omitted) |
| anything else | `<name>:latest` | `16` |

A non-empty `Image` or `Version` is written verbatim. No skeleton writes `imagePullPolicy:`.

## Signatures

```go
// internal/config
type Config struct { /* … */ ImagePullPolicy string `yaml:"imagePullPolicy,omitempty"` /* … */ }
func (c *Config) validateImagePullPolicy() error

// internal/planner
func Plan(set *ManifestSet, store state.TeamStore, registries []config.RegistryConfig, ports PortContext, filter Filter, defaultPullPolicy string) PlanResult
func Resolve(set *ManifestSet, store state.TeamStore, registries []config.RegistryConfig) []error   // defaultPullPolicy removed
func validateImagePolicies(set *ManifestSet) []error                                                // defaultPullPolicy removed

// internal/handler
type AppOptions struct { /* … */ PullPolicy string }
type ResourceOptions struct { /* … */ PullPolicy string }
func renderAppSkeleton(opts AppOptions) string
func renderResourceSkeleton(opts ResourceOptions) string
```
