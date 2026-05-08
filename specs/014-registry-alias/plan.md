# Implementation Plan: Registry Aliases

**Branch**: `014-registry-alias` | **Date**: 2026-05-08 | **Spec**: [spec.md](./spec.md)

## Summary

Extend `RegistryConfig` with an optional `alias` field so operators can define
short names for private registries in `shrine.yml`. Application and Resource manifests
then use `image: reg:<alias>/image:tag` to reference those registries without embedding
raw hostnames. Alias validation happens at plan time (catches errors in both dry-run and
live paths); alias expansion to the real hostname happens inside the Docker container
backend immediately before the image is pulled.

## Technical Context

**Language/Version**: Go 1.24+ (`github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: Cobra CLI, Docker SDK, `gopkg.in/yaml.v3`
**Storage**: N/A (no new state; config is read-only at plan time)
**Testing**: `go test ./...`; integration via `make test-integration` / `go test -tags integration ./tests/integration/...`
**Target Platform**: Linux server (homelab Docker daemon)
**Project Type**: CLI tool
**Performance Goals**: Alias lookup is O(n) over a small registry list — no measurable overhead
**Constraints**: Zero regressions for manifests that do not use `reg:` prefix
**Scale/Scope**: Typical config has ≤ 10 registry entries

## Constitution Check

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | ✅ Pass — new `alias` field in config YAML; `reg:` prefix in existing `image` manifest field |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | N/A — no new commands |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | ✅ Pass — alias expansion lives in `DockerBackend`; dry-run backend prints alias as-is with no code change |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | ✅ Pass — `expandRegistryAlias` is called from `ensureImage` and `resolveImage`; `validateRegistryImages` iterates both apps and resources |
| V. Integration-Test Gate | Does this phase map to an integration test phase using `NewDockerSuite` against a real binary? | ✅ Pass — integration test scenario added to `tests/integration/deploy_test.go` before implementation |
| VI. Docker-Authoritative State | Does state update happen after Docker operations complete? | N/A — feature does not change state management |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting? | ✅ Pass — `expandRegistryAlias`, `hasRegistryAliasPrefix`, `buildAliasMap`, `validateRegistryImages` |

## Project Structure

### Documentation (this feature)

```text
specs/014-registry-alias/
├── plan.md              ← this file
├── research.md          ← Phase 0 (generated below)
├── data-model.md        ← Phase 1 (generated below)
├── quickstart.md        ← Phase 1 (generated below)
├── contracts/
│   └── registry-config.md   ← Phase 1 (generated below)
└── tasks.md             ← Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
internal/config/
  config.go                        ← add Alias field to RegistryConfig; add ValidateRegistries()

internal/planner/
  resolve.go                       ← add validateRegistryImages(set, registries)
  plan.go                          ← extend Plan/PlanSingle to accept []config.RegistryConfig

internal/handler/
  deploy.go                        ← pass cfg.Registries to Plan and DryRun
  apply.go                         ← pass cfg.Registries to PlanSingle

internal/engine/local/dockercontainer/
  registry_auth.go                 ← add expandRegistryAlias, hasRegistryAliasPrefix, buildAliasMap

tests/integration/
  deploy_test.go                   ← new registry alias scenario (TDD-first)
```

**Structure Decision**: Single Go module; all changes are additive extensions to
existing files within the established package layout. No new packages required.
