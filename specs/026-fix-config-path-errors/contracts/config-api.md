# Contract: Config Resolver and Composition-Root API

**Feature**: `026-fix-config-path-errors` | **Date**: 2026-08-24
**Packages**: `internal/config` (resolvers), `internal/app` (composition root). Internal APIs; consumers are `cmd/`, `internal/handler`, `internal/plugins/gateway/traefik`, and the packages' own tests.

## `resolvePath` (unexported, `internal/config/utils.go`)

```go
// Before
func resolvePath(candidates []string, missingErr string) (string, error)

// After
type pathSource struct {
	name  string // "--path" | "specsDir" | "teamsDir" | "routing-dir"
	value string
}

func resolvePath(sources []pathSource, missingErr string) (string, error)
```

**Guarantees**

1. Walks `sources` in order; the first source with a non-empty `value` is the only one expanded. Later sources are never consulted.
2. Non-`~` values pass through unchanged (absolute or relative); `~` / `~/…` are expanded through `expandTilde` exactly as today (spec 003 rules, idempotent).
3. On expansion failure returns `("", fmt.Errorf("resolving %s: %w", name, err))` — the wrapped chain is `resolving <name>: expanding ~: <os.UserHomeDir error>`; `errors.Is`/`errors.As` on the inner error still work.
4. When every value is empty returns `("", errors.New(missingErr))` — byte-identical to today.
5. Pure; no I/O beyond `os.UserHomeDir`.

## Public resolvers (signatures unchanged)

```go
func (c *Config) ResolveSpecsDir(flagValue string) (string, error)          // sources: --path, specsDir
func (c *Config) ResolveTeamsDir(flagValue string) (string, error)          // sources: --path, teamsDir, specsDir
func (p *TraefikPluginConfig) ResolveRoutingDir(specsDir string) (string, error) // sources: routing-dir, specsDir
```

| Input | `ResolveSpecsDir` | `ResolveTeamsDir` | `ResolveRoutingDir` |
|-------|-------------------|-------------------|---------------------|
| Winning value resolvable | same result as today | same result as today | same result as today |
| Winning value is the flag and unresolvable | `resolving --path: expanding ~: …` | `resolving --path: expanding ~: …` | n/a |
| Winning value is the primary field and unresolvable | `resolving specsDir: …` | `resolving teamsDir: …` | `resolving routing-dir: …` |
| Winning value is a fallback field and unresolvable | n/a | `resolving specsDir: …` | `resolving specsDir: …` |
| Nothing configured | `no specs directory: set --path/-p flag or specsDir in config.yml` | `no specs directory: set --path/-p flag, teamsDir or specsDir in config.yml` | `no routing directory: set --path/-p flag or routing-dir in config.yml` |

**Unit cases to add** (`noHome` column; `HOME=""`):

- `TestResolveSpecsDir`: tilde config value → `resolving specsDir: expanding ~`; tilde flag → `resolving --path`; absolute config value with no home → succeeds; absolute flag + tilde config → returns the flag value.
- `TestResolveTeamsDir`: tilde `teamsDir` → `resolving teamsDir`; `teamsDir` empty + tilde `specsDir` → `resolving specsDir` (and **not** `teamsDir`); tilde flag → `resolving --path`; absolute `teamsDir` + tilde `specsDir` → returns `teamsDir`.
- `TestResolveRoutingDir`: tilde `routing-dir` → `resolving routing-dir`; empty `routing-dir` + tilde specsDir fallback → `resolving specsDir`.
- `TestExpandTilde`: unchanged (still asserts `expanding ~`).

## Traefik plugin wrap (`internal/plugins/gateway/traefik/plugin.go`)

```go
// Before
return "", fmt.Errorf("traefik plugin: resolving routing directory: %w", err)
// After
return "", fmt.Errorf("traefik plugin: %w", err)
```

Resulting text: `traefik plugin: resolving routing-dir: expanding ~: $HOME is not defined`. The not-configured text becomes `traefik plugin: no routing directory: set --path/-p flag or routing-dir in config.yml`.

## `resolveOptionalSpecsDir` (new, unexported, `internal/app/components.go`)

```go
func resolveOptionalSpecsDir(cfg *config.Config) (string, error)
```

| `cfg.SpecsDir` | Result |
|----------------|--------|
| `""` | `("", nil)` — absence is not an error (teardown reads no manifests) |
| non-empty | `cfg.ResolveSpecsDir("")` — resolved path, or `resolving specsDir: …` |

Pure; safe to call with any non-nil `cfg`.

## `BuildDeployBundle` / `BuildTeardownBundle` (signatures unchanged)

```go
func BuildDeployBundle(cfg *config.Config, store *state.Store, paths *config.Paths, manifestDir string, out, errOut io.Writer) (*DeployBundle, func() error, error)
func BuildTeardownBundle(cfg *config.Config, store *state.Store, paths *config.Paths, out io.Writer) (*TeardownBundle, func() error, error)
```

**Guarantees (new)**

1. `BuildDeployBundle`: after `ValidateRegistries`, `cfg.ResolveSpecsDir(manifestDir)` failure returns `(nil, nil, err)` with `err` unwrapped from the resolver. No observer, file logger, backend, plugin, vault, routing backend, or engine is constructed; `store` and `paths` are not dereferenced.
2. `BuildTeardownBundle`: `resolveOptionalSpecsDir(cfg)` runs first; failure returns `(nil, nil, err)` with the same properties. With `cfg.SpecsDir == ""` the bundle composes exactly as today with `SpecsDir: ""`.
3. Existing success paths and cleanup semantics are unchanged.

**Unit cases to add** (`internal/app/app_test.go`, `package app`, `HOME=""`, `cfg = &config.Config{SpecsDir: "~/manifests"}`, nil store/paths, `io.Discard` writers):

- `BuildDeployBundle(cfg, nil, nil, "", …)` → bundle nil, cleanup nil, `err` contains `resolving specsDir` and `expanding ~`.
- `BuildTeardownBundle(cfg, nil, nil, …)` → same.
- `resolveOptionalSpecsDir` table: `""` → `""`/nil; `/abs` → `/abs`; `~/x` with `HOME=/home/test-user` → `/home/test-user/x`; `~/x` with `HOME=""` → error containing `resolving specsDir`.

## `cmd/` (no change)

`cmd/deploy.go`, `cmd/apply.go`, `cmd/generate.go` keep `dir, err := cfg.Resolve…; if err != nil { return err }`; `cmd/teardown.go` keeps returning the bundle error. Cobra prints `Error: <message>` on stderr and `main.go` exits 1.
