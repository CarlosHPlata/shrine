# Implementation Plan: Secrets Vault Plugin (Infisical)

**Branch**: `015-infisical-secrets-vault` | **Date**: 2026-05-11 | **Spec**: [spec.md](./spec.md)

## Summary

Add a `SecretsPlugin` interface and an Infisical implementation so application manifests
can reference external vault secrets via `valueFrom: vault:<project>/<environment>/<key>`.
Shrine authenticates to a self-hosted Infisical instance using Machine Identity Universal
Auth (configured in shrine.yml under `plugins.secrets.infisical`). All vault secrets are
resolved upfront before any container operation; dry-run returns `[VAULT:<path>]`
placeholders without contacting the vault. The `SecretsPlugin` interface is designed for
future alternative providers without changes to the manifest syntax or resolver.

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: Cobra CLI, Docker SDK, `gopkg.in/yaml.v3`, `github.com/infisical/go-sdk`
**Storage**: N/A — secrets are fetched live on each apply; no local caching
**Testing**: `go test ./...`; integration via `make test-integration` / `go test -tags integration ./tests/integration/...`
**Target Platform**: Linux server (homelab Docker daemon)
**Project Type**: CLI tool
**Performance Goals**: ≤20 vault fetches per apply with no perceptible latency on a local network
**Constraints**: Zero regressions for manifests that do not use `vault:` refs; secret values never appear in any log or error output
**Scale/Scope**: Single self-hosted Infisical instance; ≤10 vault refs per manifest

## Constitution Check

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | ✅ Pass — `valueFrom: vault:` in existing manifest `valueFrom` field; config in shrine.yml under `plugins.secrets` |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | N/A — no new commands |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | ✅ Pass — `SecretsPlugin` interface; Infisical impl is injected into `LiveResolver`; dry-run path returns placeholders without interface call |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | ✅ Pass — `SecretsPlugin` used by `LiveResolver.lookupValueFrom`, `DryRunResolver` placeholder branch, and `handler/deploy.go` IsActive check |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | ✅ Pass — integration test scenario added to `tests/integration/` before implementation (TDD) |
| VI. Docker-Authoritative State | Does state update happen after Docker operations complete? | N/A — feature does not change state management |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | ✅ Pass — `isVaultRef`, `parseVaultPath`, `lookupVaultSecret`; boolean method `IsActive()` follows naming rules |

## Project Structure

### Documentation (this feature)

```text
specs/015-infisical-secrets-vault/
├── plan.md              ← this file
├── research.md          ← Phase 0 (generated above)
├── data-model.md        ← Phase 1 (generated above)
├── contracts/
│   └── secrets-config.md   ← Phase 1 (generated above)
└── tasks.md             ← Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/config/
  config.go                          ← add SecretsPluginsConfig; add Secrets field to PluginsConfig;
                                        add validateSecretsPlugins() (error if >1 block declared)
  plugin_infisical.go                ← InfisicalPluginConfig{URL, ClientID, ClientSecret}

internal/plugins/secrets/
  plugin.go                          ← SecretsPlugin interface {IsActive() bool; GetSecret(path string) (string, error)}

internal/plugins/secrets/infisical/
  plugin.go                          ← InfisicalPlugin struct; New(), IsActive(), GetSecret()
  plugin_test.go                     ← unit tests (mock SDK client)

internal/resolver/
  resolver.go                        ← extend LiveResolver: add Vault SecretsPlugin field;
                                        extend lookupValueFrom for vault: prefix;
                                        update NewLiveResolver(store, vault) signature;
                                        add isVaultRef(), parseVaultPath() helpers
  dry_run_resolver.go                ← extend placeholder logic: vault: → [VAULT:<path>]
  resolver_test.go                   ← extend unit tests for vault: resolution and placeholder

internal/planner/
  resolve.go                         ← extend validateValueFrom: accept vault: prefix;
                                        validate exactly 3 non-empty path components

internal/engine/local/
  local_engine.go                    ← pass vault plugin to NewLiveResolver

internal/handler/
  deploy.go                          ← construct SecretsPlugin from cfg.Plugins.Secrets;
                                        validate at startup (same pattern as Traefik plugin)

tests/integration/
  deploy_test.go                     ← new vault secret resolution scenario (TDD-first)
  testdata/deploy/vault-secrets/     ← shrine.yml + manifest fixtures for integration test
```

**Structure Decision**: All changes are additive extensions to existing files within the
established package layout. One new package (`internal/plugins/secrets/`) for the interface,
one new package (`internal/plugins/secrets/infisical/`) for the implementation, and one new
config file — consistent with how the Traefik plugin is structured under
`internal/plugins/gateway/traefik/`.

## Implementation Notes

### Config Loading

`config.go` adds `validateSecretsPlugins()` which counts non-nil fields in `SecretsPluginsConfig`
and returns an error if count > 1. Called from `Load()` after YAML unmarshalling.

### Handler Wiring (`handler/deploy.go`)

Mirrors the Traefik plugin pattern:
```go
vaultPlugin, err := infisical.New(cfg.Plugins.Secrets.Infisical)
// err is nil when cfg.Plugins.Secrets.Infisical is nil (plugin inactive)
```
The plugin is then passed into `NewLocalEngineWithRouting` → `NewLocalEngine` → `NewLiveResolver`.

### Plan-Time Validation (`planner/resolve.go`)

`validateValueFrom` is extended to recognise the `vault:` prefix. A `vault:` ref is valid
at plan time if its path has exactly 3 non-empty `/`-separated components. The actual secret
existence is NOT validated at plan time (would require vault connectivity during `dry-run`).

### Resolver Extension (`resolver.go`)

`lookupValueFrom` switch gains a `vault:` case. If `r.Vault` is nil or inactive when a
`vault:` ref is encountered, an error is returned (not a silent fallthrough). This makes
misconfiguration (missing `plugins.secrets` block) loud and immediate.

### No-Value-Logging Invariant

`GetSecret` in `InfisicalPlugin` MUST NOT include the returned value in any error message.
Error messages include only the path. `lookupValueFrom` propagates errors without wrapping
the value. This is enforced by code review (no automated check available).
