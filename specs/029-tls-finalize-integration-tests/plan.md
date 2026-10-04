# Implementation Plan: Integration Coverage for TLS Alias Routers and the Routing Finalize Phase

**Branch**: `029-tls-finalize-integration-tests` | **Date**: 2026-10-04 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/029-tls-finalize-integration-tests/spec.md` (GitHub issue #38). Planning instruction: *check current implementations and ensure we will not break anything else.*

## Summary

Specs 012 (TLS alias routers) and 018 (routing finalize phase) shipped with nine integration tasks deferred and never written. A read of the shipped code confirms both features behave as specified (research F1, F3, F4), so this feature is test-only: seven new scenarios appended to `TestTraefikPlugin` and three new fixtures, with **zero** changes under `cmd/`, `internal/`, or `main.go` and **zero** edits to existing scenarios. TLS scenarios parse the generated per-app routing file and assert each router's shape by name (single TLS alias, mixed aliases, revert after removing `tls: true`, byte-stability for non-TLS manifests). Finalize scenarios assert the dry-run prints the finalize operation after the per-app route operations, that a successful deploy leaves the static config and gateway container in place with `routing.finalize` recorded in `shrine.log`, and that a finalize failure — provoked black-box by pointing the gateway at an unpullable image on `localhost:1` — exits non-zero with `Error [routing.finalize]` on stdout. Scenarios are compile-checked locally and executed by CI.

## Technical Context

**Language/Version**: Go 1.25 (`go.mod`: `go 1.25.0`; module `github.com/CarlosHPlata/shrine`)
**Primary Dependencies**: None new. Test file already imports `gopkg.in/yaml.v3`, `bytes`, `os`, `path/filepath`, and the Docker client; `strings` is the only added stdlib import
**Storage**: N/A — scenarios read files the product already writes (`<routing-dir>/dynamic/*.yml`, `<routing-dir>/traefik.yml`, `<state-dir>/logs/shrine.log`) inside `t.TempDir()`
**Testing**: `NewDockerSuite` harness, real binary as subprocess, real Docker daemon, `-tags integration`. Local: `go build ./...`, `go test ./...`, `go vet -tags integration ./tests/integration/...`. CI: `make test-integration` is the pass/fail gate
**Target Platform**: Linux (GitHub Actions `ubuntu-latest` with Docker)
**Project Type**: Single Go CLI
**Performance Goals**: Integration step stays under the existing `-timeout 5m`; adds nine real deploys and one dry-run (research D8)
**Constraints**: No product change (FR-016, SC-006); no existing scenario edited (SC-004, research D3); black-box only (FR-014); no test-only hooks in the binary (FR-011); unique host ports `8119–8125` / `8447–8449` (research D7)
**Scale/Scope**: 1 test file appended (7 scenarios + 3 helpers), 3 new fixture directories (6 YAML files), 4 doc updates (012 tasks, 018 tasks, `specs/features/integration-tests.md`, `specs/progress.md`)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate Question | Status |
|-----------|---------------|--------|
| I. Declarative Manifest-First | Does this feature expose new capabilities via manifest fields (not CLI flags)? | Pass — no new capability; fixtures exercise the existing `routing.aliases[].tls` manifest field |
| II. Kubectl-Style CLI | Do new commands follow verb-first convention and include `--dry-run`? | N/A — no new command; S6 verifies the existing `--dry-run` path stays side-effect free and previews finalize |
| III. Pluggable Backend | Is new infrastructure logic behind a backend interface (not engine core)? | N/A — no production code; S5–S7 verify the `RoutingBackend.Finalize` seam from outside |
| IV. Simplicity & YAGNI | Is every abstraction justified by ≥3 concrete usages? | Pass — three test helpers, each used ≥3 times (research D6); no harness changes; twin fixture reuses an existing pattern (D4) |
| V. Integration-Test Gate | Does this phase map to an integration test phase in `specs/features/integration-tests.md` using `NewDockerSuite` against a real binary? | Pass — this feature *is* the gate that 012 and 018 deferred; scenarios run the real binary via `NewDockerSuite`, reuse the suite's `BeforeEach`/`AfterEach` cleanup, and `integration-tests.md` is updated |
| VI. Docker-Authoritative State | Does state update happen *after* Docker operations complete? | N/A — no state code touched |
| VII. Clean Code & Readability | Is repeated logic extracted into named helpers? Are names self-documenting (no WHAT comments)? | Pass — `readDynamicRouters`, `assertPlainRouter`, `assertTLSRouter` replace repeated map-walking; comments limited to WHY (preserve-policy file removal, unpullable-image choice) |

**Post-Phase-1 re-check**: all gates unchanged — Pass. No Complexity Tracking entries required.

## Regression Safety (planning instruction)

Full analysis in [research.md](research.md) Part 3. The plan's guard rails:

1. **No production code.** Shipped behaviour was read and matches specs 012/018; nothing needs fixing to make the scenarios pass. Gate: `git diff --stat main` lists only `tests/` and `specs/` (plus tooling metadata).
2. **Append-only test edit.** New scenarios go at the end of `TestTraefikPlugin`; new helpers have new names; existing helpers, scenarios, `BeforeEach`, and `AfterEach` are untouched. The existing dry-run and first-deploy scenarios are not extended.
3. **Isolated resources.** New fixtures are new directories addressed by explicit path; new app names are unique; host ports are outside every range in use; scenarios run sequentially and are cleaned by the existing hooks.
4. **Failure scenario leaves nothing behind.** The gateway container is never created; application containers and temp dirs are removed by existing cleanup.
5. **Unit suite unaffected.** Only `integration`-tagged and YAML files are added.
6. **CI budget watched.** Timeout unchanged unless the first CI run shows the integration step above 4 minutes.
7. **Divergence protocol.** If a scenario fails against unchanged product code, stop and report (FR-016).

Findings that adjust the spec's wording without changing its intent:

- A successful finalize prints nothing on stdout; it is observable only in `shrine.log`. S7 asserts there (research F3, D2).
- Unchanged redeploys are byte-identical because of the spec 009 preserve policy, so S4 additionally removes the file and compares the regenerated bytes (research F2, D5).

## Project Structure

### Documentation (this feature)

```text
specs/029-tls-finalize-integration-tests/
├── plan.md              # This file
├── research.md          # Phase 0 — current-state findings, 8 decisions, regression-safety analysis
├── data-model.md        # Phase 1 — fixtures, scenarios S1–S7, deferred-task traceability
├── quickstart.md        # Phase 1 — local gates, CI gate, spot-check table, divergence protocol
├── contracts/
│   └── operator-observables.md   # Phase 1 — every operator-visible value the scenarios pin
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 (/speckit-tasks — NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
tests/integration/
└── traefik_plugin_test.go     # MODIFIED (append-only): import "strings";
                               #   helpers readDynamicRouters, assertPlainRouter, assertTLSRouter;
                               #   scenarios S1–S7 at the end of TestTraefikPlugin

tests/testdata/deploy/
├── traefik-alias-tls/             # NEW: app.yaml (whoami-tls, one alias with tls: true), team.yaml
├── traefik-alias-tls-removed/     # NEW: app.yaml (whoami-tls, same alias without tls), team.yaml
└── traefik-alias-tls-mixed/       # NEW: app.yaml (whoami-tls-mixed, plain alias + tls alias), team.yaml

specs/012-tls-alias-routers/tasks.md          # MODIFIED: T016, T017, T023, T024, T026 → delivered by 029
specs/018-routing-backend-finalize/tasks.md   # MODIFIED: T012, T014, T018, T019 → delivered by 029
specs/features/integration-tests.md           # MODIFIED: list the seven scenarios and three fixtures
specs/progress.md                             # MODIFIED: entry for 029

cmd/, internal/, main.go, Makefile, .github/   # UNCHANGED
```

**Structure Decision**: Single Go CLI; the feature lives entirely in the existing integration test file and fixture tree, following the alias-scenario conventions already there (`aliasFixturePath`, `shrine-alias-test` team, inline `writeConfig`).

## Implementation Outline

Ordered so each step is independently compile-checkable. Detail per scenario is in [data-model.md](data-model.md); asserted values are in [contracts/operator-observables.md](contracts/operator-observables.md).

1. **Fixtures** — create the three fixture directories.
2. **Helpers** — add `readDynamicRouters`, `assertPlainRouter`, `assertTLSRouter` next to the existing file-level helpers.
3. **S1 (US1)** — deploy `tls` with `tlsPort`; primary router plain, `alias-0` TLS; stdout has `(tls)` and no missing-websecure warning.
4. **S2 (US2)** — deploy `tls-mixed`; `alias-0` plain, `alias-1` TLS, primary plain; all three share one `service`.
5. **S3 (US2)** — deploy `tls`, assert TLS; remove the per-app file; deploy `tls-removed`; `alias-0` plain; stdout has no `(tls)`.
6. **S4 (US3)** — deploy `prefix` without `tlsPort` three times (unchanged, then after file removal); bytes equal each time; no `websecure`/`tls:` in the file; no `(tls)` or warning in any stdout.
7. **S5 (US4)** — deploy `traefik` with the unpullable gateway image; `AssertFailure`; stdout has `Deploying Application: hello-eligible` and `Error [routing.finalize]`; log has `[error] routing.finalize`; the app's route file exists; `platform.traefik` does not.
8. **S6 (US5)** — dry-run `traefik`; `[ROUTE]  Finalize` occurs once and after the last `[ROUTE]  WriteRoute`; no container, no `traefik.yml`, no `dynamic/`.
9. **S7 (US5)** — deploy `traefik`; `traefik.yml` exists, `platform.traefik` running, log has `[started] routing.finalize` and `[info] routing.finalize`.
10. **Local gates** — `go build ./...`, `go test ./...`, `go vet -tags integration ./tests/integration/...`, diff-scope check ([quickstart.md](quickstart.md)).
11. **Docs** — update the 012/018 task lists, `integration-tests.md`, `progress.md`; run `graphify update .`.
12. **CI gate** — push; confirm the seven new sub-tests pass, all existing ones still pass, and the integration step duration is under 4 minutes.

## Complexity Tracking

No violations.
