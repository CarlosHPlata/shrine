# Research: Field-Naming Config Path Errors That Always Stop the Command

**Feature**: `026-fix-config-path-errors` | **Date**: 2026-08-24
**Purpose**: Resolve every design choice behind the plan so implementation has no open questions. The Technical Context in `plan.md` contains no `NEEDS CLARIFICATION` markers; the decisions below record the alternatives weighed for each judgment call.

## Current-state findings (what the code does today)

| Area | Location | Finding |
|------|----------|---------|
| Tilde expansion | `internal/config/utils.go:11-19` | `expandTilde` expands `~` / `~/…` via `os.UserHomeDir`; on failure returns `expanding ~: %w`. No field identity — it cannot know which field it is expanding. |
| Candidate walk | `internal/config/utils.go:22-29` | `resolvePath(candidates []string, missingErr string)` expands the **first non-empty** candidate and returns its error unwrapped; all-empty → `errors.New(missingErr)`. Positional: the caller's field names are lost here. |
| Resolvers | `internal/config/config.go:111-128`, `plugin_traefik.go:17-22` | `ResolveSpecsDir([flag, specsDir])`, `ResolveTeamsDir([flag, teamsDir, specsDir])`, `ResolveRoutingDir([routing-dir, <specsDir>/traefik])`. The `missingErr` texts already name the fields to set. |
| cmd call sites | `cmd/deploy.go:38`, `cmd/apply.go:20,41`, `cmd/generate.go:36,51,90` | `dir, err := cfg.Resolve…; if err != nil { return err }` — returned unwrapped; Cobra prints `Error: expanding ~: $HOME is not defined`. |
| Swallowed sites | `internal/app/app.go:113`, `app.go:177` | `specsDir, _ := cfg.ResolveSpecsDir(manifestDir)` and `cfg.ResolveSpecsDir("")`. The deploy site is shadowed by `cmd/deploy.go:38` resolving first with an absolute `manifestDir`; the teardown site has **no** upstream check. |
| Teardown shape | `cmd/teardown.go` | No `--path` flag. `BuildTeardownBundle` is the first thing the command calls; its first statement is the swallowed resolve, followed by `newObserverPair` → `ui.NewFileLogger` → `os.MkdirAll(<state>/logs)` + open log file. |
| Teardown fixtures | `tests/integration/teardown_test.go` | Every teardown run is `teardown <team> --state-dir <dir>` with no `--config-dir` → `specsDir` absent → `ResolveSpecsDir("")` returns the "no specs directory" error, which today is discarded. Propagating it naively breaks the suite. |
| Traefik wrap | `internal/plugins/gateway/traefik/plugin.go:119-125` | `resolvedRoutingDir` returns `traefik plugin: resolving routing directory: %w` — the only field-naming path today. |
| Cause text | `os.UserHomeDir` (Go stdlib, Unix) | `errors.New("$HOME is not defined")` when `HOME` is empty **or** unset. The issue's `$HOME is not set` is a paraphrase; contracts use the real text. |
| Reproducibility | `internal/config/paths.go` | With `--config-dir` and `--state-dir` given, `ResolvePaths` never consults `HOME`, so the failure is reproducible from the binary with `HOME` empty. |
| Deploy pre-step | `cmd/deploy.go:36,64-72` | `checkForUpdate` runs before resolution; network only, ignores its own errors, no `HOME` dependence. |
| Harness env | `tests/integration/testutils/harness.go:25`, `testsuite.go:82` | `exec.Command` without `cmd.Env` inherits the test process environment; `tc.Setenv("HOME", "")` therefore reaches the binary. `NewSuite` needs no Docker. |
| Existing unit coverage | `internal/config/{utils,config,plugin_traefik}_test.go` | `expandTilde` no-`HOME` cases assert only `expanding ~`; the three `Resolve*` tables set `HOME=fakeHome` for every case — no failure-path case exists. `internal/app/` has no tests. |
| Docs | `docs/content/troubleshooting/_index.md`, `AGENTS.md:277` | Troubleshooting page is a short catalogue of symptom → fix sections; no page documents the resolution error. AGENTS.md notes `~ is expanded` for `specsDir` — still true. |

## Decision 1 — Attach the field name inside `resolvePath`, via labelled sources

**Decision**: Replace `[]string` candidates with `[]pathSource{name, value}` and wrap in `resolvePath` at the point of failure: `fmt.Errorf("resolving %s: %w", s.name, err)`.

**Rationale**: `resolvePath` is the one place that knows *which* candidate is being expanded, so it is the only place that can name the fallback source truthfully (spec FR-002). Three resolvers share the struct (Constitution IV's ≥3 usages) and the wrap is written once (VII, DRY). `expandTilde` stays field-agnostic and its tests are unchanged.

**Alternatives considered**:
- *Wrap at each cmd call site* (`fmt.Errorf("resolving specsDir: %w", err)` ×6) — duplicates the wrap, and `apply teams` cannot know whether `teamsDir` or the `specsDir` fallback failed.
- *Wrap in each `Resolve*` method* — three near-identical wraps and the same fallback blindness.
- *A structured `PathError{Field, Err}` type* — no consumer needs structure; string wrapping with `%w` preserves `errors.Is` for any future caller (YAGNI).

## Decision 2 — Source labels: `--path`, `specsDir`, `teamsDir`, `routing-dir`

**Decision**: Labels are the literal flag or config key the operator would edit: `--path` for `flagValue`, `specsDir`/`teamsDir` for the config fields, `routing-dir` for the Traefik key; the `ResolveRoutingDir` fallback (derived from `specsDir`) is labelled `specsDir`.

**Rationale**: Spec FR-002 — the named source is the one that supplied the value. Using the exact key spelling (`routing-dir`, not "routing directory") makes the message greppable against `config.yml` and consistent with the `missingErr` texts that already say `--path/-p flag`, `specsDir`, `teamsDir`, `routing-dir`.

## Decision 3 — Message shape: `resolving <source>: expanding ~: <cause>`

**Decision**: Keep `expandTilde`'s `expanding ~: %w` and prefix it with `resolving <source>: `. Real output for the issue's reproduction: `Error: resolving specsDir: expanding ~: $HOME is not defined`.

**Rationale**: Matches the issue's expected shape and the existing Traefik phrasing (`resolving …`); the inner `expanding ~` keeps `TestExpandTilde` green and tells the operator *what* about the value failed. Exact wording is not a contract beyond "source + cause" (spec Assumptions); tests assert on `resolving specsDir` / `resolving teamsDir` / `resolving --path` / `resolving routing-dir` substrings plus `expanding ~`.

## Decision 4 — Simplify the Traefik wrap so the field is named once

**Decision**: `resolvedRoutingDir` wraps as `traefik plugin: %w` instead of `traefik plugin: resolving routing directory: %w`, giving `traefik plugin: resolving routing-dir: expanding ~: $HOME is not defined`.

**Rationale**: With Decision 1 the inner error already says `resolving routing-dir`; keeping the outer text would print "resolving routing directory: resolving routing-dir" — noise. No test or doc pins the old wording (grep: only `TestResolveRoutingDir`, which asserts `no routing directory`). The spec's edge-case line was amended to "keeps naming its field through the same rule" to reflect this.

**Alternatives considered**: keep a label-free `resolvePath` variant just for `routing-dir` — two resolver code paths to keep in step, the opposite of DRY; or drop the `traefik plugin:` prefix entirely — loses the component context that the plugin's other errors carry.

## Decision 5 — Deploy bundle: propagate unwrapped, before the observer

**Decision**: `specsDir, err := cfg.ResolveSpecsDir(manifestDir); if err != nil { return nil, nil, err }` at the current position (after `ValidateRegistries`, before `newObserverPair`).

**Rationale**: FR-005 — every stage surfaces the failure. Returning before `newObserverPair` means no `logs/` directory, no log file, no backend, plugin, vault, or engine is created (FR-004), which is also what makes the app-level unit test filesystem-free. The error is returned unwrapped because the resolver's text is already self-describing; a stage prefix would read `specs dir: resolving specsDir: …`. In production this site is reached only with an already-absolute `manifestDir` (idempotent re-resolution, spec 003 FR-006), so the guard is a safety net for call-order changes exactly as the spec's Story 2 describes.

## Decision 6 — Teardown bundle: `resolveOptionalSpecsDir` tolerates absence, not failure

**Decision**: Add `resolveOptionalSpecsDir(cfg *config.Config) (string, error)` in `internal/app/components.go`: `if cfg.SpecsDir == "" { return "", nil }; return cfg.ResolveSpecsDir("")`. `BuildTeardownBundle` calls it first and returns the error before `newObserverPair`.

**Rationale**: Spec FR-006 and Assumptions — `teardown` has no `--path`, reads no manifests, and the integration suite runs it without `specsDir`; only a configured value that cannot be resolved is a failure. Checking `cfg.SpecsDir == ""` up front is the simplest faithful expression of "absent is fine" and is pure, so its unit test needs no filesystem. The helper lives in `app` because the "optional for teardown" rule is a composition-root policy, not a config-file property. A one-line WHY comment explains the tolerance.

**Alternatives considered**:
- *Propagate literally at both sites (as the issue words it)* — makes `teardown` fail with `no specs directory: set --path/-p flag or specsDir in config.yml` (a flag teardown does not have) and breaks `TestTeardown`; a breaking change disguised as a bug fix.
- *Sentinel `config.ErrNoSpecsDir` + `errors.Is`* — new exported API and a second way to express "not configured" for one caller (YAGNI).
- *`Config.ResolveOptionalSpecsDir()` on the config type* — encodes a teardown-specific policy in the config package; rejected on placement.
- *Make `specsDir` mandatory for teardown* — out of scope per spec; would require a new flag and docs.

## Decision 7 — `cmd/` untouched; `generate` covered by construction

**Decision**: No changes to `cmd/deploy.go`, `cmd/apply.go`, `cmd/generate.go`, `cmd/teardown.go`.

**Rationale**: Constitution II — commands are thin dispatchers. They already `return err`; once the resolver names the source, every caller (including the three `generate` subcommands and `deploy --dry-run`) gets the field-naming message with no per-command code. FR-003 "exactly once" holds because the cmd layer fails first for `deploy`/`apply`/`generate`, and for `teardown` the bundle is the only resolver.

## Decision 8 — Unit tests: table `noHome` flag; `internal/app/app_test.go` in package `app`

**Decision**: Add a `noHome bool` column to `TestResolveSpecsDir`, `TestResolveTeamsDir`, and `TestResolveRoutingDir` (`home := fakeHome; if tc.noHome { home = "" }; t.Setenv("HOME", home)`) with failure cases per source (see `contracts/config-api.md`). Create `internal/app/app_test.go` (`package app`, internal so it can reach `resolveOptionalSpecsDir`) with: both bundles called with `cfg{SpecsDir: "~/manifests"}`, nil store/paths, `io.Discard` writers, `HOME=""` → `(nil, nil, err)` and `err` contains `resolving specsDir`; and a `resolveOptionalSpecsDir` table (absent → `""`, nil; absolute → unchanged; `~/x` with home → expanded; `~/x` without home → error naming `specsDir`).

**Rationale**: Project policy — unit tests never touch the filesystem. `t.Setenv("HOME", "")` is enough because `os.UserHomeDir` treats empty as undefined. Nil store/paths are safe precisely because the assertion is that the bundle returns *before* using them — a regression that moved the check later would panic, which is the right failure. The "absent `specsDir` still composes" case cannot be a unit test (the next line opens the file logger), so it is pinned by the helper's table plus the existing `TestTeardown` integration gate.

**Alternatives considered**: a fake `paths` with a temp state dir to drive `BuildTeardownBundle` to success — violates the no-filesystem rule for no coverage gain over `TestTeardown`.

## Decision 9 — Integration scenarios: one no-Docker file, one scenario per subcommand

**Decision**: New `tests/integration/config_paths_test.go` on `NewSuite`. `BeforeEach`: `tc.Setenv("HOME", "")`, `tc.StateDir = tc.TempDir()`, a config dir written with the existing `writeConfig` helper from `traefik_plugin_test.go` (same `integration_test` package, so it is already shared — no move needed) containing `specsDir: ~/manifests` and `teamsDir: ~/teams`. Scenarios per `contracts/cli-behaviour.md`: `deploy`, `deploy --dry-run`, `deploy team x`, `apply teams` (names `teamsDir`), `apply teams` with a specsDir-only config (names `specsDir`), `apply -f`, `generate team`, `teardown` (plus `AssertFileNotExists(<state>/logs)`), `deploy --path '~/manifests'` (names `--path`), once-only assertion via `strings.Count(stderr, "resolving specsDir") == 1`, and the positive case: `apply teams --path <abs team fixture>` then `deploy --dry-run --path <abs basic fixture>` succeed with the unresolvable config value never consulted.

**Rationale**: Constitution V — real binary, black-box, TDD-ordered. None of the failing scenarios reaches Docker, and `--dry-run` is Docker-free (`cmd/cmd_test.go: TestDeployDryRun`), so `NewSuite` keeps the file fast. The `logs/` absence is the observable proof that `BuildTeardownBundle` stopped before the observer — the one place where the swallowed error had no upstream guard.

## Decision 10 — Docs: one troubleshooting section; no CLI or reference drift

**Decision**: Add a section to `docs/content/troubleshooting/_index.md` for `resolving specsDir: expanding ~: $HOME is not defined` (cause: `~`-prefixed path with no `HOME` in cron/CI/systemd/containers; fix: absolute path, set `HOME`, or pass `--path`). No changes to Cobra strings, `AGENTS.md`, or the manifest reference.

**Rationale**: SC-002 is "the operator can fix it from the message alone"; a catalogue entry closes the loop for the environment-specific cause. Nothing in the CLI surface changes, so the generated `docs/content/cli/*.md` pages and `AGENTS.md` stay as they are.
