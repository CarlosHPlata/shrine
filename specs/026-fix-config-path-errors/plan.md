# Implementation Plan: Field-Naming Config Path Errors That Always Stop the Command

**Branch**: `026-fix-config-path-errors` | **Date**: 2026-08-24 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/026-fix-config-path-errors/spec.md` (GitHub issue #37)

## Summary

Two error-handling defects in config path resolution break spec 003 FR-005. (1) `config.resolvePath` expands the first non-empty positional candidate and `expandTilde` returns `expanding ~: %w` with no field identity; `cmd/deploy.go`, `cmd/apply.go`, and `cmd/generate.go` return that error unwrapped, so the operator sees `expanding ~: $HOME is not defined` and cannot tell whether `--path`, `specsDir`, or `teamsDir` needs fixing — only Traefik's `routing-dir` path wraps with a field name. Fix: `resolvePath` takes labelled sources (`pathSource{name, value}`) and wraps at the point of failure — `resolving <source>: expanding ~: $HOME is not defined` — where `<source>` is the flag or config key that actually supplied the value (`--path`, `specsDir`, `teamsDir`, `routing-dir`); the three `Resolve*` methods only pass labels, `cmd/` stays untouched, and the Traefik plugin's outer wrap is simplified to `traefik plugin: %w` so the field is named exactly once. (2) `app.BuildDeployBundle` and `app.BuildTeardownBundle` discard the `ResolveSpecsDir` error (`specsDir, _ :=`). Fix: deploy propagates it before the observer/file-logger is created; teardown resolves through a new `resolveOptionalSpecsDir(cfg)` helper so an *absent* `specsDir` stays tolerated (teardown has no `--path` and `TestTeardown` runs it without one) while a *configured-but-unresolvable* one fails before anything is constructed. Coverage: the config resolver test tables gain no-`HOME` cases per source; a new `internal/app/app_test.go` drives both bundles and the helper with `t.Setenv("HOME", "")` and no filesystem; a new no-Docker integration file runs every affected subcommand against `specsDir: ~/manifests` with `HOME` empty and asserts a `resolving specsDir` failure with no side effects — authored red-first, executed by CI.

## Technical Context

**Language/Version**: Go 1.25 (`go.mod`: `go 1.25.0`; module `github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: Cobra (existing `RunE` error path: `Error: …` on stderr, `main.go` exits 1); stdlib `os.UserHomeDir` — the only resolution-failure source today, returning `$HOME is not defined` on Unix when `HOME` is empty or unset (the issue's `$HOME is not set` is a paraphrase); no new dependencies
**Storage**: N/A — no state or config schema change; the fix changes *when* a command stops and *what* the error says
**Testing**: `go test ./...` for units (package policy: unit tests never touch the filesystem — the missing home directory is simulated with `t.Setenv("HOME", "")`; app-level tests pass a nil store/paths because both bundles must return before touching them); integration via a new `tests/integration/config_paths_test.go` on `NewSuite` (no Docker — every failing scenario stops before any Docker call or state write, and the one positive scenario is a `--dry-run`) with `-tags integration`, compile-checked locally (`go vet -tags integration ./tests/integration/...`) and executed by CI; the existing `TestTeardown` (runs `teardown` with no `specsDir`) is the gate for FR-006
**Target Platform**: Linux server (Docker host); the cause text comes from `os.UserHomeDir` and is Unix-specific
**Project Type**: Single Go CLI
**Performance Goals**: N/A — one string wrap on an error path
**Constraints**: The error must carry the value's source and the cause, exactly once per run (FR-001–FR-003); the "not configured" messages (`no specs directory: …`, `no routing directory: …`) are unchanged; `teardown` without `specsDir` is unchanged (FR-006); spec 003 resolution rules are unchanged (FR-007); Cobra `Long`/flag strings are unchanged so the generated `docs/content/cli/*.md` pages do not drift
**Scale/Scope**: 6 production files (`internal/config/{utils,config,plugin_traefik}.go`, `internal/app/{app,components}.go`, one wrap string in `internal/plugins/gateway/traefik/plugin.go`; ~40 net lines), 3 modified + 1 new unit-test file, 1 new integration-test file, 1 troubleshooting doc section, 2 spec bookkeeping files

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | Pass — no new capability surface, flags, or manifest fields; error reporting only |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | Pass — no new commands; `cmd/` is untouched (the thin dispatchers already return the error and Cobra prints it on stderr with exit 1); `deploy --dry-run` fails at the same point as the real deploy (research D7) |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | N/A — no engine or backend logic; the Traefik plugin change is a one-line wrap string (research D4) |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | Pass — one two-field struct (`pathSource`) consumed by all three resolvers (≥3 usages, research D1); one private helper for the single teardown rule, extracted for naming per VII rather than as an abstraction (research D6); no sentinel errors, no new exported config API |
| V. Integration-Test Gate | Does this phase map to an integration test scenario using the real binary? | Pass — new no-Docker scenarios in `tests/integration/config_paths_test.go` on `NewSuite`, authored red-first (research D9); FR-006 is gated by the existing `TestTeardown`; `specs/features/integration-tests.md` updated |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | Pass — the failing path performs no Docker or state operation at all; the fix moves the stop *earlier* (before the file logger opens) |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | Pass — labelled `pathSource` values replace positional candidates so the wrap lives in one place; `resolveOptionalSpecsDir` names the teardown rule instead of an inline `if`; the only new comment is a one-line WHY (why teardown tolerates an unset `specsDir`) |

**Post-Phase-1 re-check**: all gates unchanged — Pass. No Complexity Tracking entries required.

## Project Structure

### Documentation (this feature)

```text
specs/026-fix-config-path-errors/
├── plan.md              # This file
├── research.md          # Phase 0 — current-state findings + 10 decisions (label placement, message shape, teardown rule, tests, docs)
├── data-model.md        # Phase 1 — pathSource, resolution-outcome tables per resolver, teardown decision table, side-effect boundary
├── quickstart.md        # Phase 1 — manual verification walkthrough per user story (env -u HOME)
├── contracts/
│   ├── cli-behaviour.md # Phase 1 — exit code / stderr / side effects per subcommand and scenario
│   └── config-api.md    # Phase 1 — resolvePath(sources), Resolve* guarantees, resolveOptionalSpecsDir, Build*Bundle guarantees
└── tasks.md             # Phase 2 (/speckit-tasks — NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
internal/config/
├── utils.go                 # MODIFIED: type pathSource{name, value}; resolvePath(sources []pathSource, missingErr) wraps
│                            #           the expansion failure as "resolving <name>: %w" (utils.go:22-29); expandTilde unchanged
├── utils_test.go            # UNCHANGED: expandTilde cases still assert "expanding ~"
├── config.go                # MODIFIED: ResolveSpecsDir → sources {"--path", flag}, {"specsDir", c.SpecsDir};
│                            #           ResolveTeamsDir → {"--path", flag}, {"teamsDir", c.TeamsDir}, {"specsDir", c.SpecsDir}
├── config_test.go           # MODIFIED: TestResolveSpecsDir / TestResolveTeamsDir tables gain a noHome flag and
│                            #           failure cases asserting the named source (--path, specsDir, teamsDir, fallback)
├── plugin_traefik.go        # MODIFIED: ResolveRoutingDir → {"routing-dir", p.RoutingDir}, {"specsDir", specsDir}
└── plugin_traefik_test.go   # MODIFIED: no-HOME cases asserting "resolving routing-dir" / "resolving specsDir"

internal/app/
├── app.go                   # MODIFIED: BuildDeployBundle propagates ResolveSpecsDir error (app.go:113) before newObserverPair;
│                            #           BuildTeardownBundle uses resolveOptionalSpecsDir(cfg) (app.go:177) and propagates
├── components.go            # MODIFIED: func resolveOptionalSpecsDir(cfg *config.Config) (string, error)
└── app_test.go              # NEW (package app): both bundles fail before construction on an unresolvable specsDir;
                             #      resolveOptionalSpecsDir table (absent → "", nil; resolvable; unresolvable → names specsDir)

internal/plugins/gateway/traefik/
└── plugin.go                # MODIFIED: resolvedRoutingDir wrap "traefik plugin: resolving routing directory: %w" → "traefik plugin: %w"

cmd/                         # UNCHANGED — deploy.go, apply.go, generate.go, teardown.go already return the error

tests/integration/
└── config_paths_test.go     # NEW: NewSuite (no Docker); HOME="" + config with ~-prefixed specsDir/teamsDir;
                             #      one scenario per subcommand (contracts/cli-behaviour.md), plus --path naming,
                             #      fallback naming, once-only, and the absolute --path dry-run positive case

docs/content/troubleshooting/_index.md   # MODIFIED: section for "resolving specsDir: expanding ~: $HOME is not defined"
specs/progress.md                        # MODIFIED: fix entry (issue #37)
specs/features/integration-tests.md      # MODIFIED: config-path scenarios listed
```

**Structure Decision**: Single-project Go CLI; production changes stay inside the two packages that own the defects — `internal/config/` (where the value's source is known, so the label is attached exactly where the failure occurs) and `internal/app/` (the composition root that swallowed the error). `cmd/` is deliberately untouched: the dispatchers already return the resolver's error, and Cobra's existing `RunE` path delivers stderr + exit 1. No `Long`/flag strings change, so the generated CLI reference does not drift (research D10).

## Design Outline

1. **Labelled sources** (`internal/config/utils.go`): `type pathSource struct{ name, value string }`; `resolvePath(sources []pathSource, missingErr string)` walks the sources, skips empty values, expands the first non-empty one, and on failure returns `fmt.Errorf("resolving %s: %w", s.name, err)`; when every value is empty it returns `errors.New(missingErr)` exactly as today (FR-001, FR-002, FR-007). `expandTilde` is unchanged.
2. **Resolvers pass labels** (`config.go`, `plugin_traefik.go`): `ResolveSpecsDir` → `{"--path", flagValue}, {"specsDir", c.SpecsDir}`; `ResolveTeamsDir` → `{"--path", flagValue}, {"teamsDir", c.TeamsDir}, {"specsDir", c.SpecsDir}`; `ResolveRoutingDir` → `{"routing-dir", p.RoutingDir}, {"specsDir", specsDir}`. The `missingErr` strings are untouched. Because the label travels with the value, the fallback case names `specsDir` and the flag case names `--path` by construction.
3. **Name the field once** (`traefik/plugin.go`): `resolvedRoutingDir` wraps as `traefik plugin: %w`, yielding `traefik plugin: resolving routing-dir: expanding ~: $HOME is not defined` instead of a doubled "resolving …" (research D4).
4. **Deploy bundle propagates** (`app.go`): `specsDir, err := cfg.ResolveSpecsDir(manifestDir); if err != nil { return nil, nil, err }` — placed where the discard was, i.e. after `ValidateRegistries` and before `newObserverPair`, so nothing (no file logger, backend, plugin, vault, engine) is constructed (FR-004, FR-005). The error is returned unwrapped because it is already self-describing (research D5).
5. **Teardown bundle tolerates absence, not failure** (`components.go`, `app.go`): `resolveOptionalSpecsDir(cfg)` returns `("", nil)` when `cfg.SpecsDir == ""` and otherwise `cfg.ResolveSpecsDir("")`; `BuildTeardownBundle` calls it first and returns the error before `newObserverPair` (FR-005, FR-006; research D6).
6. **Tests** (TDD per Constitution V): the integration file and the unit cases are written red-first — config tables gain `noHome` failure cases per source; `internal/app/app_test.go` asserts both bundles return `(nil, nil, err)` with `resolving specsDir` in `err` and that `resolveOptionalSpecsDir` distinguishes absent from unresolvable; the integration scenarios in `contracts/cli-behaviour.md` run each subcommand with `HOME=""` and assert failure, the named source, once-only reporting, and no `<state>/logs` directory (proof the observer never opened) (FR-008).
7. **Docs and bookkeeping**: add a troubleshooting section for the new message shape; record the fix in `specs/progress.md`; list the scenarios in `specs/features/integration-tests.md`; run `graphify update .` after the code change.

## Complexity Tracking

No Constitution violations — table intentionally empty.
