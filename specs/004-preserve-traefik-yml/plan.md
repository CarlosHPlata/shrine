# Implementation Plan: Preserve Operator-Edited traefik.yml

**Branch**: `004-preserve-traefik-yml` | **Date**: 2026-04-30 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/004-preserve-traefik-yml/spec.md`

## Summary

On every deploy today, [`generateStaticConfig`](../../internal/plugins/gateway/traefik/config_gen.go#L15) at `internal/plugins/gateway/traefik/config_gen.go:57` unconditionally rewrites `traefik.yml`, destroying any operator edits. The fix gates that single `os.WriteFile` call on a pre-write existence probe of the target path: if any entry exists at the path (regular file, symlink — broken or not — directory, or any other type), Shrine leaves it alone; otherwise it generates the default exactly as today. The `dynamic/` subtree is unaffected — the parallel "preserve operator-added files" policy already lives there. To satisfy FR-006/SC-004, the plugin emits a `gateway.config.preserved` or `gateway.config.generated` info event through the existing `engine.Observer`; the terminal logger renders it like the other deploy-step signals.

## Technical Context

**Language/Version**: Go 1.24+
**Primary Dependencies**: `gopkg.in/yaml.v3`, Docker SDK (unchanged for this feature), `github.com/CarlosHPlata/shrine/internal/engine` (Observer/Event)
**Storage**: Filesystem only — `traefik.yml` at the gateway routing dir; no state-store or DB writes
**Testing**: `go test ./...` for units; `go test -tags integration ./tests/integration/...` for the canonical gate (Principle V)
**Target Platform**: Linux server (homelab gateway host)
**Project Type**: CLI tool (single Go module)
**Performance Goals**: Negligible — adds one `os.Lstat` syscall per deploy
**Constraints**: No new flags, env vars, or config fields (FR-005, SC-003); must not regress first-deploy bootstrap (User Story 2); must not follow symlinks when probing (FR-009)
**Scale/Scope**: One file (`traefik.yml`) at one path per deploy; no fan-out

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | Pass — no new fields; behavior change only |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | N/A — no new commands; existing `shrine deploy --dry-run` path is preserved by the same skip-on-exists guard |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | Pass — change lives entirely in the Traefik gateway plugin (`internal/plugins/gateway/traefik/`); `internal/engine/` is untouched |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | Pass — single guarded write site, no new abstractions or interfaces; helper extracted only because the existence probe is reused by tests via a stable plugin entry point |
| V. Integration-Test Gate | Does this phase map to an integration test in `tests/integration/` using `NewDockerSuite` against a real binary? | Pass — new scenarios added to `tests/integration/traefik_plugin_test.go` (preserve-on-redeploy, regenerate-after-delete, symlink, broken symlink, non-regular file); existing first-deploy scenario already covers the bootstrap path |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | N/A — no state-store changes; the file write is gated on filesystem state, not Docker state |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | Pass — existence probe extracted as `isStaticConfigPresent(routingDir)` (boolean prefix per the constitution's naming rule); event names are self-describing (`gateway.config.preserved`, `gateway.config.generated`) |

> Violations MUST be documented in the Complexity Tracking table below.

## Project Structure

### Documentation (this feature)

```text
specs/004-preserve-traefik-yml/
├── spec.md              # Feature spec (already written + clarified)
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (event contract)
└── tasks.md             # Phase 2 output (created by /speckit-tasks)
```

### Source Code (repository root)

```text
internal/plugins/gateway/traefik/
├── plugin.go            # Deploy() — unchanged structurally; receives observer via constructor
├── config_gen.go        # generateStaticConfig() — gated on existence probe; emits info event
├── routing.go           # unchanged (dynamic/ policy already correct)
└── spec.go              # unchanged

internal/ui/
└── terminal_logger.go   # add cases for gateway.config.preserved / gateway.config.generated

internal/handler/
└── deploy.go            # pass the existing observer through to traefik.New() (1-line wiring change)

tests/integration/
└── traefik_plugin_test.go  # new scenarios (US1, US3 + edge cases)
```

**Structure Decision**: Single Go module, no new packages. The change is confined to the Traefik gateway plugin (`internal/plugins/gateway/traefik/`) and a small wiring update in `internal/handler/deploy.go` plus the matching observer rendering in `internal/ui/terminal_logger.go`. The integration suite gains scenarios in the existing `tests/integration/traefik_plugin_test.go` — no new test file or harness.

## Complexity Tracking

> No Constitution violations. Table left empty intentionally.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| (none)    | —          | —                                    |
