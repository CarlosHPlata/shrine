# Data Model: Field-Naming Config Path Errors That Always Stop the Command

**Feature**: `026-fix-config-path-errors` | **Date**: 2026-08-24

No persisted schema changes. This document captures the in-memory concepts the feature manipulates and the rules that govern them.

## Entities

### pathSource (new — `internal/config/utils.go`, unexported)

| Field | Type | Meaning |
|-------|------|---------|
| `name` | `string` | The flag or config key the operator would edit: `--path`, `specsDir`, `teamsDir`, `routing-dir` |
| `value` | `string` | The raw configured value (may be empty, absolute, `~`-prefixed, or relative) |

`resolvePath` walks an ordered `[]pathSource`, skips empty values, expands the first non-empty one, and wraps any expansion failure with `resolving <name>: `. The **label travels with the value**, so whichever source wins the priority walk is the one named.

### Resolver source order (existing priority, now labelled)

| Resolver | Sources in priority order | Not-configured message (unchanged) |
|----------|---------------------------|------------------------------------|
| `Config.ResolveSpecsDir(flag)` | `--path`=flag → `specsDir` | `no specs directory: set --path/-p flag or specsDir in config.yml` |
| `Config.ResolveTeamsDir(flag)` | `--path`=flag → `teamsDir` → `specsDir` | `no specs directory: set --path/-p flag, teamsDir or specsDir in config.yml` |
| `TraefikPluginConfig.ResolveRoutingDir(specsDir)` | `routing-dir` → `specsDir` (value is `<specsDir>/traefik`) | `no routing directory: set --path/-p flag or routing-dir in config.yml` |

### Resolution outcome

For a resolver walk over sources `s₁ … sₙ`, let `w` be the first source with a non-empty value.

| Condition | Result | Error text |
|-----------|--------|------------|
| no `w` (all empty) | **not configured** | `<missingErr>` (unchanged) |
| `w.value` absolute or relative (no leading `~`) | resolved = `w.value` | — |
| `w.value` is `~` or `~/…` and home directory known | resolved = home-joined path | — |
| `w.value` is `~` or `~/…` and home directory unknown | **resolution failure** | `resolving <w.name>: expanding ~: $HOME is not defined` |

"Not configured" and "resolution failure" are distinct outcomes with distinct messages; only the second is new in shape. Sources *after* `w` are never consulted — an unresolvable `specsDir` is invisible when `--path` is absolute.

### Worked examples

| Command | `--path` | `teamsDir` | `specsDir` | `HOME` | Outcome |
|---------|----------|------------|------------|--------|---------|
| `deploy` | — | — | `~/manifests` | unset | `resolving specsDir: expanding ~: $HOME is not defined` |
| `deploy --path ~/manifests` (literal) | `~/manifests` | — | `~/manifests` | unset | `resolving --path: expanding ~: $HOME is not defined` |
| `deploy --path /abs` | `/abs` | — | `~/manifests` | unset | `/abs` — config value never consulted |
| `apply teams` | — | `~/teams` | `~/manifests` | unset | `resolving teamsDir: …` |
| `apply teams` | — | — | `~/manifests` | unset | `resolving specsDir: …` (fallback names the supplier) |
| `apply teams` | — | `/abs/teams` | `~/manifests` | unset | `/abs/teams` |
| `deploy` | — | — | — | unset | `no specs directory: set --path/-p flag or specsDir in config.yml` (unchanged) |
| Traefik with `routing-dir: ~/traefik` | — | — | `/abs` | unset | `traefik plugin: resolving routing-dir: expanding ~: $HOME is not defined` |

## Composition-root rules (`internal/app`)

### Deploy bundle

`BuildDeployBundle(cfg, store, paths, manifestDir, …)`:

| Step | On failure |
|------|------------|
| 1. `cfg.ValidateRegistries()` | `validating registries: %w` (unchanged) |
| 2. **`cfg.ResolveSpecsDir(manifestDir)`** | **return the resolver error unwrapped — nothing constructed** (was: discarded) |
| 3. `newObserverPair` (opens `<state>/logs/…`) | `observer: %w` |
| 4 … | container backend, Traefik plugin, vault, routing, engine (unchanged) |

### Teardown bundle and `resolveOptionalSpecsDir`

| `cfg.SpecsDir` | `HOME` | `resolveOptionalSpecsDir(cfg)` | `BuildTeardownBundle` |
|----------------|--------|-------------------------------|-----------------------|
| `""` (absent) | any | `("", nil)` | proceeds as today (FR-006) |
| `/abs` | any | `("/abs", nil)` | proceeds |
| `~/x` | set | `("<home>/x", nil)` | proceeds |
| `~/x` | unset | `("", resolving specsDir: expanding ~: …)` | **returns the error before `newObserverPair`** (was: proceeded with `""`) |

### Side-effect boundary (what exists after a failed run)

| Command | Where it stops | State dir | Docker | Files |
|---------|----------------|-----------|--------|-------|
| `deploy` / `--dry-run` / `deploy team` | `cmd/deploy.go` resolver | store initialised (pre-existing, empty) — no `logs/` | untouched | none |
| `apply teams` | `cmd/apply.go` resolver | no team written (`AssertTeamCount(0)`) | untouched | none |
| `apply -f` | `cmd/apply.go` resolver | untouched | untouched | none |
| `generate *` | `cmd/generate.go` resolver | untouched | — | no manifest written |
| `teardown` | `BuildTeardownBundle` → `resolveOptionalSpecsDir` | no `logs/` | untouched — nothing removed | none |

`<state>/logs/` is created only by `ui.NewFileLogger` inside `newObserverPair`; its absence after a failed run is the observable proof that no bundle was composed.
