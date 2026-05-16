---
description: "Tasks for feature 017: Separate Composition Root from internal/handler/"
---

# Tasks: Separate Composition Root from `internal/handler/`

**Input**: Design documents from `/specs/017-refactor-composition-root/`
**Prerequisites**: plan.md, spec.md (required); research.md, data-model.md, contracts/app-package.md, quickstart.md (loaded)

**Tests**: The spec does not request new test files. The Constitution Principle V gate is the existing `tests/integration/` suite — it must continue to pass (SC-005). Per the user's no-filesystem rule for unit tests and the "smoke tests would have to construct real plugins which touch disk" finding from research, `internal/app/` ships **without** new unit tests; integration coverage is the gate. US3 verification is documentation-based (T019) for the same reason.

**Organization**: Tasks are grouped by user story. US1 (apply) is the MVP slice that proves the bundle pattern; US2 (deploy + teardown) completes the single-location-edit property; US3 verifies testability via documentation.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Different files, no dependency on incomplete tasks
- **[Story]**: US1, US2, US3 — maps to spec.md user stories
- File paths are absolute relative to repo root

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the new package directory and empty files so foundational work can begin.

- [X] T001 Create the `internal/app/` package skeleton: add `internal/app/app.go` and `internal/app/components.go` containing only `package app` declarations and the imports that will be filled in T002 / T003.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Implement the private helpers shared across every bundle constructor. No user story can begin until these exist because every `BuildXBundle` will compose them.

**⚠️ CRITICAL**: No user-story phase may begin until T002 is complete.

- [X] T002 Implement the six private helpers in `internal/app/components.go` per `specs/017-refactor-composition-root/data-model.md` (Private helpers table):
  - `newObserverPair(out io.Writer, paths *config.Paths) (engine.Observer, func() error, error)` — composes `ui.NewTerminalObserver` + `ui.NewFileLogger` into an `engine.MultiObserver`; returns the file logger's `Close` as the cleanup func.
  - `newVault(cfg *config.Config) (<vaultPluginType>, error)` — wraps `infisicalplugin.New(cfg.Plugins.Secrets.Infisical)`.
  - `newContainerBackend(store *state.Store, cfg *config.Config, observer engine.Observer) (engine.ContainerBackend, error)` — wraps `local.NewContainerBackend`.
  - `newTraefikPlugin(cfg *config.Config, container engine.ContainerBackend, specsDir string, observer engine.Observer) (*traefik.Plugin, error)` — wraps `traefik.New`.
  - `newLocalEngine(opts local.EngineOptions) (engine.DeployEngine, error)` — wraps `local.NewLocalEngine`.
  - `routingFromPlugin(plugin *traefik.Plugin) (engine.RoutingBackend, error)` — returns nil when `!plugin.IsActive()`, otherwise `plugin.RoutingBackend()`.
  Each helper mirrors today's call site verbatim (do not change argument order or behavior). Helpers themselves do NOT add error wrapping; that is the bundle constructor's job (per `contracts/app-package.md`).

**Checkpoint**: Foundation ready — user story implementation can now begin.

---

## Phase 3: User Story 1 - Establish bundle pattern via `apply` (Priority: P1) 🎯 MVP

**Goal**: Prove the new-command path. After this story, a maintainer can add a new command needing the same dependency shape as `apply` without writing any plugin/observer/engine construction code in `internal/handler/`.

**Independent Test**: `shrine apply --file <manifest>` produces byte-identical stdout/stderr and exit code to the pre-refactor binary against an `tests/integration/` fixture; `grep -E 'infisicalplugin\.New|local\.NewLocalEngine|ui\.NewTerminalObserver|ui\.NewFileLogger' internal/handler/apply.go` returns no matches.

### Implementation for User Story 1

- [X] T003 [US1] Define `ApplyBundle` struct in `internal/app/app.go` with the fields listed in `specs/017-refactor-composition-root/data-model.md` (`ApplyBundle` table): `Out io.Writer`, `Cfg *config.Config`, `Store *state.Store`, `Paths *config.Paths`, `Observer engine.Observer`, `Vault <vaultPluginType>`, `Engine engine.DeployEngine`.

- [X] T004 [US1] Implement `BuildApplyBundle(cfg *config.Config, store *state.Store, paths *config.Paths, out io.Writer) (*ApplyBundle, func() error, error)` in `internal/app/app.go`. Sequence: `cfg.ValidateRegistries()` → `newObserverPair` → `newVault` → `newLocalEngine`. Wrap each error with the documented slot prefixes from `contracts/app-package.md` (`"validating registries: "`, `"observer: "`, `"vault: "`, `"engine: "`) using `fmt.Errorf("...: %w", err)`. On any failure, invoke partial cleanup before returning `(nil, nil, err)`. (Depends on T002, T003.)

- [X] T005 [US1] Refactor `handler.ApplySingle` in `internal/handler/apply.go` to signature `ApplySingle(b *app.ApplyBundle, file, manifestDir string) error`. Delete the inline `ValidateRegistries`, observer pair construction, `infisicalplugin.New`, and `local.NewLocalEngine` calls — read the equivalents from `b.*`. Remove the now-unused imports `github.com/CarlosHPlata/shrine/internal/engine/local`, `github.com/CarlosHPlata/shrine/internal/plugins/secrets/infisical`, `github.com/CarlosHPlata/shrine/internal/ui`. Keep `internal/engine` only if a type like `engine.MultiObserver` is still referenced (it should not be after this task).

- [X] T006 [US1] Update `cmd/apply.go` `RunE` for the `--file` branch: replace the current `handler.ApplySingle(handler.ApplySingleOptions{...})` call with: resolve `dir`, call `bundle, cleanup, err := app.BuildApplyBundle(cfg, store, paths, cmd.OutOrStdout())`, return on error, `defer cleanup()`, then `return handler.ApplySingle(bundle, applyFile, dir)`. Add `github.com/CarlosHPlata/shrine/internal/app` to the imports. Delete the now-unused `handler.ApplySingleOptions` type from `internal/handler/apply.go` if no other caller remains.

- [X] T007 [US1] Run `go build ./...` and `go test ./...` from the repo root. Both must pass with zero changes outside the files touched by T003–T006. Then run `grep -E 'infisicalplugin\.New|local\.NewLocalEngine|ui\.NewTerminalObserver|ui\.NewFileLogger' internal/handler/apply.go` and confirm no matches.

**Checkpoint**: US1 complete — `apply` consumes the bundle; `deploy` and `teardown` still use today's inline wiring (and still work). Single-handler version of the pattern is shipped and verifiable.

---

## Phase 4: User Story 2 - Migrate `deploy` and `teardown` (Priority: P2)

**Goal**: After this story, every signature change to a leaf constructor (`infisicalplugin.New`, `traefik.New`, `local.NewLocalEngine`, `ui.New*`) is a one-file edit because every consumer goes through `internal/app/`.

**Independent Test**: `grep -rE 'infisicalplugin\.New|traefik\.New|local\.NewLocalEngine|ui\.NewTerminalObserver|ui\.NewFileLogger' internal/handler/` returns zero matches (SC-001). For each of those constructor names, `grep -rn '<name>' internal/` returns hits only inside `internal/app/components.go` (SC-002). All `shrine` CLI commands continue to behave identically to pre-refactor (SC-004).

### Implementation for User Story 2

- [X] T008 [P] [US2] Define `DeployBundle` struct in `internal/app/app.go` with the fields listed in `specs/017-refactor-composition-root/data-model.md` (`DeployBundle` table): `Out`, `Cfg`, `Store`, `Paths`, `SpecsDir`, `Observer`, `Vault`, `ContainerBackend`, `TraefikPlugin`, `Routing`, `Engine`.

- [X] T009 [P] [US2] Define `TeardownBundle` struct in `internal/app/app.go`: `Out`, `Cfg`, `Store`, `Paths`, `SpecsDir`, `Observer`, `TraefikPlugin`, `Routing`, `Engine`. (T008 and T009 touch the same file; sequence them — they are marked [P] only because they are logically independent definitions, not to encourage parallel editing of one file.)

- [X] T010 [US2] Implement `BuildDeployBundle(cfg *config.Config, store *state.Store, paths *config.Paths, manifestDir string, out io.Writer) (*DeployBundle, func() error, error)` in `internal/app/app.go`. Sequence (mirroring `internal/handler/deploy.go:67-130` exactly): `cfg.ValidateRegistries()` → resolve `specsDir` via `cfg.ResolveSpecsDir(manifestDir)` → `newObserverPair` → `newContainerBackend` → `newTraefikPlugin(cfg, containerBackend, specsDir, observer)` → `newVault` → `routingFromPlugin(plugin)` → `newLocalEngine(local.EngineOptions{Store: store, Registries: cfg.Registries, Observer: observer, Routing: routing, Vault: vault})`. Slot-wrap errors as in `contracts/app-package.md`. (Depends on T002, T008.)

- [X] T011 [US2] Implement `BuildTeardownBundle(cfg *config.Config, store *state.Store, paths *config.Paths, out io.Writer) (*TeardownBundle, func() error, error)` in `internal/app/app.go`. Sequence (mirroring `internal/handler/teardown.go:24-63`): resolve `specsDir` via `cfg.ResolveSpecsDir("")` → `newObserverPair` → `newTraefikPlugin(cfg, nil, specsDir, observer)` (nil container backend, intentional — teardown does not push images) → `routingFromPlugin(plugin)` → `newLocalEngine(local.EngineOptions{Store: store, Registries: cfg.Registries, Observer: observer, Routing: routing})` (no `Vault`). Slot-wrap errors. (Depends on T002, T009.)

- [X] T012 [US2] Add `ValidateTraefikConfig(cfg *config.Config) error` to `internal/app/app.go`. Body: `_, err := traefik.New(cfg.Plugins.Gateway.Traefik, nil, "", nil); return err`. This is the validation-only entry point used exclusively by `handler.DryRun` and replaces its current direct call to `traefik.New`.

- [X] T013 [US2] Refactor `handler.Deploy` in `internal/handler/deploy.go` to signature `Deploy(b *app.DeployBundle, manifestDir string) error`. Replace inline construction (lines 67-130) with reads from `b.*`. Keep the planner call (`planner.Plan(manifestDir, b.Store.Teams, b.Cfg.Registries)`), the validation-error printing block, the empty-steps branch, the `b.Engine.ExecuteDeploy(result.Steps, result.ManifestSet)` call, and the post-execute `if b.TraefikPlugin.IsActive() { b.TraefikPlugin.Deploy() }` block — these are request-shaped logic, they stay. Remove the now-unused imports: `internal/engine`, `internal/engine/local`, `internal/plugins/gateway/traefik`, `internal/plugins/secrets/infisical`, `internal/ui`. Delete the `DeployOptions` struct after confirming no external caller references it.

- [X] T014 [US2] Refactor `handler.DryRun` in `internal/handler/deploy.go` to call `app.ValidateTraefikConfig(cfg)` instead of `traefik.New(...)`. Keep `cfg.ValidateRegistries()` (not on the SC-001 list, may stay in the handler). Keep `planner.Plan`, the validation-error printing, the empty-steps branch, and the `dryrun.NewDryRunEngine(out).ExecuteDeploy(...)` call. Remove the `internal/plugins/gateway/traefik` import. Add the `internal/app` import.

- [X] T015 [US2] Refactor `handler.Teardown` in `internal/handler/teardown.go` to signature `Teardown(b *app.TeardownBundle, team string) error`. Replace inline construction (lines 24-63) with reads from `b.*`. Keep the `planner.PlanTeardown(team, b.Store.Deployments)` call and the `b.Engine.ExecuteTeardown(team, result.Steps)` call. Preserve the existing comment at lines 38-41 explaining why the routing plugin is wired even when inactive — this is a non-obvious WHY (Constitution VII). Remove unused imports: `internal/engine`, `internal/engine/local`, `internal/plugins/gateway/traefik`, `internal/ui`. Delete the `TeardownOptions` struct after confirming no external caller references it.

- [X] T016 [US2] Update `cmd/deploy.go` `RunE`: when `dryRun` is true, keep `return handler.DryRun(cmd.OutOrStdout(), dir, store, cfg)` (signature unchanged); otherwise replace the current `handler.Deploy(handler.DeployOptions{...})` call with: `bundle, cleanup, err := app.BuildDeployBundle(cfg, store, paths, dir, cmd.OutOrStdout())`, return on error, `defer cleanup()`, `return handler.Deploy(bundle, dir)`. Add the `internal/app` import.

- [X] T017 [US2] Update `cmd/teardown.go` `RunE`: replace the current `handler.Teardown(handler.TeardownOptions{...})` call with: `bundle, cleanup, err := app.BuildTeardownBundle(cfg, store, paths, cmd.OutOrStdout())`, return on error, `defer cleanup()`, `return handler.Teardown(bundle, team)`. Add the `internal/app` import.

- [X] T018 [US2] Run `go build ./...` and `go test ./...`. Then run `grep -rE 'infisicalplugin\.New|traefik\.New|local\.NewLocalEngine|ui\.NewTerminalObserver|ui\.NewFileLogger' internal/handler/` and confirm zero matches (SC-001). Run `grep -rn 'infisicalplugin\.New' internal/`, `grep -rn 'traefik\.New' internal/`, `grep -rn 'local\.NewLocalEngine' internal/`, `grep -rn 'ui\.NewTerminalObserver' internal/`, `grep -rn 'ui\.NewFileLogger' internal/` and confirm each returns hits only inside `internal/app/components.go` (and inside `internal/plugins/...` / `internal/ui/...` definitions themselves) — SC-002.

**Checkpoint**: US2 complete — all three in-scope handlers consume composed bundles. SC-001 and SC-002 verified.

---

## Phase 5: User Story 3 - Document handler unit-testability (Priority: P3)

**Goal**: Verify the design satisfies the testability property — a handler can in principle be exercised in tests with a stand-in bundle. Per research R5 and the user's no-filesystem rule for unit tests, no new test file is added; the verification is documentary (the design itself preserves the property by giving handlers interface-typed dependencies).

**Independent Test**: A documented pattern exists in `quickstart.md` showing how to construct a stand-in bundle for unit testing. Reading the bundle struct definitions confirms that `Engine engine.DeployEngine` and `Observer engine.Observer` are interface fields suitable for fake substitution.

### Implementation for User Story 3

- [X] T019 [US3] Append a "Scenario 4 — Unit-testing a migrated handler with a stand-in bundle" section to `specs/017-refactor-composition-root/quickstart.md`. Show: (a) constructing an `*app.ApplyBundle` literal with `Engine` set to a fake `engine.DeployEngine` and `Observer` set to a fake `engine.Observer`, (b) calling `handler.ApplySingle(bundle, ...)` and asserting on its returned error / the fake engine's recorded calls, (c) the constraint that the test must not write to the filesystem (per `[[feedback_unit_tests_no_filesystem]]`). If `Vault` or `TraefikPlugin` block injection in any bundle today (concrete plugin types), list them in a "Known limitations" subsection — they are follow-up candidates, not blockers for this refactor.

**Checkpoint**: US3 verified — the design's testability property is documented and discoverable.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final verification, graph refresh, progress tracking.

- [ ] T020 [P] Verify SC-004 (CLI parity): from a clean git worktree at `HEAD~` (pre-refactor), build `/tmp/shrine-pre`. From the post-refactor branch, build `/tmp/shrine-post`. For a representative manifest fixture under `tests/integration/`, capture stdout+stderr+exit code of `shrine deploy --dry-run --path <fixture>`, `shrine apply --file <one-app.yaml> --path <fixture>`, and `shrine teardown <team>` from each binary. Diff the captures and confirm byte-identical output and identical exit codes. (Use `git worktree add` so the comparison is non-destructive.)

- [ ] T021 [P] Update `specs/progress.md` to add the entry for phase 017 (composition root) and mark it `[x]` once T022 passes.

- [ ] T022 Run `make test-integration` (or `go test -tags integration ./tests/integration/...`) as the Constitution Principle V gate. This is the slow (~10 min, per `[[feedback_integration_tests_slow]]`) final gate — run once after T018 and T020 are clean.

- [X] T023 [P] Run `graphify update .` from the repo root to refresh `graphify-out/` after the package layout change. (No API cost — AST-only update.)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: T001 has no dependencies — start immediately.
- **Foundational (Phase 2)**: T002 depends on T001 — BLOCKS all user stories.
- **US1 (Phase 3)**: depends on T002. Internally sequential (T003 → T004 → T005 → T006 → T007).
- **US2 (Phase 4)**: depends on T002. Independent of US1 *in principle* (different bundles, different handlers, different cmd files), but in practice **US1 should complete first** so the bundle pattern is validated end-to-end on the smaller `apply` surface before applying it to the more complex `deploy` flow.
- **US3 (Phase 5)**: T019 depends on US1 + US2 being complete (the quickstart example references the migrated `ApplyBundle`).
- **Polish (Phase 6)**: T020, T021, T022, T023 depend on all user stories being complete. T022 should be the *last* task touched because it is slow.

### Within-story dependencies

- US1: T003 → T004 → T005 → T006 → T007 (each consumes the previous).
- US2: (T008, T009 may be co-edited in app.go) → T010, T011, T012 (constructors) → T013, T014, T015 (handler refactors) → T016, T017 (cmd refactors) → T018 (verification).

### Parallel Opportunities

Within US2: handler refactors T013, T014, T015 touch `deploy.go` (T013, T014) and `teardown.go` (T015). T015 is file-disjoint from T013/T014 and can run in parallel; T013 and T014 must serialize on `deploy.go`.

Within US2: cmd refactors T016 (cmd/deploy.go) and T017 (cmd/teardown.go) are file-disjoint and parallelizable.

Polish phase: T020, T021, T023 are file-disjoint and parallelizable; T022 should run last (or alongside, but its result blocks completion).

---

## Parallel Example: US2 handler refactors

```bash
# After T010-T012 complete, the three handler refactors can split:
# Terminal A:
edit internal/handler/deploy.go      # T013 (Deploy) then T014 (DryRun)
# Terminal B:
edit internal/handler/teardown.go    # T015 (file-disjoint from A)

# Then cmd refactors split file-disjoint:
# Terminal A:
edit cmd/deploy.go                   # T016
# Terminal B:
edit cmd/teardown.go                 # T017
```

---

## Implementation Strategy

### MVP (US1 only)

1. T001 → T002 → T003 → T004 → T005 → T006 → T007.
2. **Validate independently**: run `go test ./...`, then run `shrine apply --file ...` against a fixture; output must match pre-refactor.
3. Optional ship-point — `apply` is now using the bundle pattern; the codebase still works. If you stop here, you have not yet satisfied SC-001 for `deploy.go` / `teardown.go`, but the pattern is proven and reusable.

### Incremental delivery

1. Setup + Foundational + US1 → ship (`apply` migrated; pattern proven).
2. Add US2 → ship (all handlers migrated; SC-001 + SC-002 satisfied).
3. Add US3 → ship (testability property documented).
4. Polish phase → final integration gate + progress mark + graph refresh.

### Single-developer strategy (recommended for this refactor)

This is a small refactor with tight ordering between bundle definition, handler change, and cmd change. A single developer can complete US1 in one sitting, then US2, then US3 + polish in another. Branching this work across multiple developers is unlikely to be faster than the integration test cycle time (~10 min).

---

## Notes

- `[P]` = different files, no dependency on incomplete tasks.
- `[Story]` label maps task to spec.md user story for traceability.
- The Constitution's TDD-for-integration rule (Principle V) does not apply here because the refactor is behaviour-preserving — no new integration test files are needed; existing scenarios are the regression detector (SC-005).
- Per `[[feedback_integration_tests_slow]]`, `make test-integration` runs once at T022, not during iteration. Use `go test ./...` for the inner loop.
- Per `[[feedback_unit_tests_no_filesystem]]`, no unit tests are added for `internal/app/`; the integration suite is the gate.
- Commit after each task or logical group. Suggested commit boundaries: after T002, after T007 (US1 done), after each of T013/T014/T015 if split across PRs, after T018 (US2 done), after T019 (US3 done), after T022 (gate clean).
- Stop at any checkpoint to validate the increment.
- Avoid: editing `cmd/` files before the corresponding handler refactor lands (would temporarily break the build); leaving stale `*Options` structs in `internal/handler/` after their callers are gone.
