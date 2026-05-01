# Implementation Plan: Fix Traefik Dashboard Generated in Static Config

**Branch**: `010-fix-dashboard-static-config` | **Date**: 2026-05-01 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/010-fix-dashboard-static-config/spec.md`

## Summary

Move the Traefik dashboard router and basic-auth middleware out of the generated static `traefik.yml` (where Traefik silently drops them) and into a dedicated dynamic file in the file provider's watched directory, so the dashboard works on a clean `shrine deploy`.

Two narrow code changes inside `internal/plugins/gateway/traefik/`:

1. `generateStaticConfig` stops populating `staticConfig.HTTP`. The dashboard surface stays advertised via `entryPoints.traefik` and `api.dashboard: true`, both of which are valid static keys.
2. A new `generateDashboardDynamicConfig` writes the dashboard router and `dashboard-auth` middleware to `<routing-dir>/dynamic/__shrine-dashboard.yml`, mirroring the "if file exists, preserve it" regime already implemented by `RoutingBackend.WriteRoute` and the existing `generateStaticConfig` preservation branch.

A separate detection pass on the pre-existing static file emits a `gateway.config.legacy_http_block` warning event when an `http:` block is present (the artefact of an earlier buggy version), per FR-010/FR-011. The warning is purely advisory — the file is never modified.

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: `gopkg.in/yaml.v3` (already used by the plugin); `internal/engine` Observer for event emission; `internal/config.TraefikPluginConfig` for inputs.
**Storage**: Filesystem only — `<routing-dir>/traefik.yml` (static) and `<routing-dir>/dynamic/*.yml` (dynamic) are bind-mounted into the Traefik container at `/etc/traefik`.
**Testing**: `go test ./...` for unit tests (no filesystem touches per project convention); `make test-integration` (build tag `integration`) for the end-to-end gate using `NewDockerSuite` against a real Docker daemon.
**Target Platform**: Linux server (homelab operator host running Docker).
**Project Type**: CLI (single Go module under `cmd/`, internal packages under `internal/`).
**Performance Goals**: N/A — config generation runs once per `shrine deploy` and is bounded by a few small file writes.
**Constraints**: Must remain idempotent across redeploys; must not modify operator-edited files (FR-006, FR-010); must not break the existing per-app routing file regime; must not collide with per-app routing filenames (FR-009).
**Scale/Scope**: One plugin package, ~1 new file (~80 LOC), ~30 LOC delta in `config_gen.go` and `spec.go`, plus unit and integration tests.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | [x] N/A — pure bug fix; no new operator-facing surface. Existing `plugins.gateway.traefik.dashboard` field is unchanged. |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | [x] N/A — no new commands; existing `shrine deploy --dry-run` continues to work because the new file write happens inside the same `Deploy()` path that already short-circuits when the dry-run container backend is used (no traefik.yml written today on dry-run; same applies to the new dynamic file). |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | [x] Pass — change is fully contained inside `internal/plugins/gateway/traefik/` (the plugin package); no engine core edits. |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | [x] Pass — re-uses existing `httpConfig`, `router`, `middleware`, `basicAuth` types; no new types beyond a tiny `dashboardDynamicDoc` wrapper. No factories, no interfaces. |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | [x] Pass — three new scenarios in `tests/integration/traefik_plugin_test.go`: dashboard-on-clean-deploy, dashboard-dynamic-file-preserved, and legacy-http-block-warning. |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | [x] N/A — feature touches only filesystem config files; no `DeploymentStore` or container-id state involved. |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | [x] Pass — `isPathPresent` is reused for the new file's preservation check; new helpers `generateDashboardDynamicConfig`, `dashboardDynamicFileName`, `hasLegacyDashboardHTTPBlock` follow the project's `is/has/should` boolean and verb-noun naming. |

> Violations MUST be documented in the Complexity Tracking table below.

**Result**: All seven principles pass with no violations. Complexity Tracking section is therefore empty.

## Project Structure

### Documentation (this feature)

```text
specs/010-fix-dashboard-static-config/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── observer-events.md  # Observer event-name contract for the new file flows
├── checklists/
│   └── requirements.md  # Created by /speckit-specify
└── tasks.md             # Phase 2 output (created by /speckit-tasks)
```

### Source Code (repository root)

```text
internal/plugins/gateway/traefik/
├── config_gen.go            # MODIFIED — drop HTTP block from static; add dashboard dynamic generator + legacy detector
├── config_gen_test.go       # MODIFIED — drop HTTP-block assertions; add new unit tests with stubbed lstat
├── spec.go                  # MODIFIED — remove HTTP field from staticConfig (httpConfig type itself stays; routing.go uses it)
├── plugin.go                # MODIFIED — call new dashboard generator from Deploy() after generateStaticConfig
├── routing.go               # UNCHANGED — already writes per-app dynamic files; new dashboard file is a sibling
└── routing_test.go          # UNCHANGED

tests/integration/
└── traefik_plugin_test.go   # MODIFIED — three new scenarios (clean deploy + dashboard, preserve dashboard.yml, warn on legacy http block)
```

**Structure Decision**: No new top-level packages or directories. The fix lives entirely inside the existing Traefik plugin package, mirroring the pattern already established for static-config preservation (spec 004) and per-app dynamic-file preservation (spec 009). Observer event names are extended rather than re-shaped, keeping the contract for downstream UI/log consumers stable.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations. This section is intentionally empty.
