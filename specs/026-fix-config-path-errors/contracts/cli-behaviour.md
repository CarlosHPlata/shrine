# Contract: CLI Behaviour

**Feature**: `026-fix-config-path-errors` | **Date**: 2026-08-24
**Scope**: Operator-visible contract (exit code, stderr, side effects) when a path-typed config value cannot be resolved. Every row maps to a scenario in `tests/integration/config_paths_test.go` (research Decision 9) unless marked *existing gate*.

Conventions: failures → stderr as `Error: <message>` (Cobra); exit 1 on any returned error. No flags, subcommands, or output formats are added or renamed. Fixture: `HOME` empty in the binary's environment; `--config-dir` points at a directory whose `config.yml` is

```yaml
specsDir: ~/manifests
teamsDir: ~/teams
```

(`config-specs-only` variant: `specsDir: ~/manifests` alone). `--state-dir` is a fresh temp dir. `$HOME is not defined` is Go's real `os.UserHomeDir` text.

## Failure scenarios

| # | Command | Config | Exit | stderr contains | Side effects asserted |
|---|---------|--------|------|-----------------|-----------------------|
| C1 | `deploy` | both | 1 | `resolving specsDir: expanding ~: $HOME is not defined`; `resolving specsDir` appears **exactly once** | `<state>/logs` does not exist |
| C2 | `deploy --dry-run` | both | 1 | `resolving specsDir` | `<state>/logs` does not exist |
| C3 | `deploy team any-team` | both | 1 | `resolving specsDir` | `<state>/logs` does not exist |
| C4 | `apply teams` | both | 1 | `resolving teamsDir: expanding ~` | `AssertTeamCount(0)` |
| C5 | `apply teams` | specs-only | 1 | `resolving specsDir` and **not** `resolving teamsDir` | `AssertTeamCount(0)` |
| C6 | `apply -f tests/testdata/app.yml` | both | 1 | `resolving specsDir` | `<state>/logs` does not exist |
| C7 | `generate team demo` | both | 1 | `resolving specsDir` | — (nothing written; the target dir was never resolved) |
| C8 | `teardown demo-team` | both | 1 | `resolving specsDir`; appears exactly once | `<state>/logs` does not exist (bundle stopped before the observer) |
| C9 | `deploy --path ~/manifests` (literal tilde argument) | both | 1 | `resolving --path: expanding ~` and **not** `resolving specsDir` | — |
| C10 | any command with a `~`-prefixed `plugins.gateway.traefik.routing-dir`, absolute `--path` | traefik | 1 | `traefik plugin: resolving routing-dir: expanding ~` | *existing-style scenario; covered by unit `TestResolveRoutingDir` — no new integration row required* |

The `Error: ` prefix and exit 1 come from Cobra/`main.go` as for every other shrine failure.

## Success / no-regression scenarios

| # | Command | Config | Exit | Notes |
|---|---------|--------|------|-------|
| S1 | `apply teams --path <abs>/tests/testdata/deploy/team` | both | 0 | flag wins; the unresolvable config values are never consulted (spec edge case 1) |
| S2 | `deploy --dry-run --path <abs>/tests/testdata/deploy/basic` (after S1) | both | 0 | dry-run plan printed; Docker-free |
| S3 | `teardown <team>` with **no** config dir | none | 0 | *existing gate*: `tests/integration/teardown_test.go` — absent `specsDir` remains tolerated (FR-006) |
| S4 | `deploy` with `HOME` set and `specsDir: ~/…` | both | as today | *existing gate*: `traefik_plugin_test.go` "routing-dir starts with tilde"; resolution rules unchanged (FR-007) |
| S5 | `deploy` with no `--path` and no `specsDir` | none | 1 | *unchanged*: `no specs directory: set --path/-p flag or specsDir in config.yml` |

## Message catalogue (after this feature)

| Situation | Message |
|-----------|---------|
| `specsDir` unresolvable | `resolving specsDir: expanding ~: $HOME is not defined` |
| `teamsDir` unresolvable | `resolving teamsDir: expanding ~: $HOME is not defined` |
| `--path` unresolvable | `resolving --path: expanding ~: $HOME is not defined` |
| `routing-dir` unresolvable | `traefik plugin: resolving routing-dir: expanding ~: $HOME is not defined` |
| nothing configured (`deploy`, `generate`, `apply -f`) | `no specs directory: set --path/-p flag or specsDir in config.yml` *(unchanged)* |
| nothing configured (`apply teams`) | `no specs directory: set --path/-p flag, teamsDir or specsDir in config.yml` *(unchanged)* |
| Traefik nothing configured | `traefik plugin: no routing directory: set --path/-p flag or routing-dir in config.yml` |
