# Tasks: Field-Naming Config Path Errors That Always Stop the Command

**Input**: Design documents from `/specs/026-fix-config-path-errors/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/config-api.md, contracts/cli-behaviour.md, quickstart.md

**Tests**: Included — Constitution Principle V mandates TDD (tests authored red-first, before implementation), and spec FR-008 explicitly requires the coverage. Per team practice, unit tests run locally and never touch the filesystem (`t.Setenv("HOME", "")` simulates the missing home directory; app-level tests pass nil store/paths because the bundles must return before touching them); integration tests are authored and compile-checked locally (`go vet -tags integration ./tests/integration/...`) and executed by the CI pipeline as the gate — never run locally.

**Organization**: Tasks are grouped by user story. US1 (field-naming errors) is delivered entirely in `internal/config/` (labelled `pathSource` values in `resolvePath`) plus a one-line wrap change in the Traefik plugin — `cmd/` needs no change because the dispatchers already return the resolver's error. US2 (a resolution failure stops every stage) is delivered in `internal/app/` (propagate in `BuildDeployBundle`; `resolveOptionalSpecsDir` in `BuildTeardownBundle`). US2's tests assert the US1 message shape (`resolving specsDir`), so implement US1 first. There is no shared scaffolding, so the Foundational phase is empty.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1 (P1 the operator is told which path field failed), US2 (P2 a resolution failure always stops the command, whichever stage hits it first)

## Phase 1: Setup

**Purpose**: Confirm a green baseline — this is a bug fix on existing code, no scaffolding needed.

- [X] T001 Verify green baseline at repository root: `go build ./...`, `go test ./internal/config/ ./internal/app/ ./internal/plugins/gateway/traefik/ ./cmd/`, `go vet -tags integration ./tests/integration/...`, and `gofmt -l .` (prints nothing) all pass before any change

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Not applicable — no infrastructure precedes the stories. The labelled resolver *is* US1's implementation, and US2 builds on its message text; see the story order in Dependencies.

---

## Phase 3: User Story 1 - The operator is told which path field failed (Priority: P1) 🎯 MVP

**Goal**: Every subcommand that resolves `specsDir`/`teamsDir` (`deploy`, `deploy team`, `deploy --dry-run`, `apply -f`, `apply teams`, `generate *`) fails with `resolving <source>: expanding ~: $HOME is not defined`, where `<source>` is the flag or config key that actually supplied the value (`--path`, `specsDir`, `teamsDir`; `routing-dir` for Traefik), exactly once per run and before any side effect. Resolution rules and the "not configured" messages are unchanged.

**Independent Test**: `go test ./internal/config/` (new `noHome` cases naming each source) plus `TestConfigPathResolution` in `tests/integration/config_paths_test.go` (CI): with `HOME` empty and `specsDir: ~/manifests` / `teamsDir: ~/teams`, each command exits 1 naming the right source; an absolute `--path` still succeeds. `teardown` is covered in US2 (its failure needs the app-layer fix).

### Tests for User Story 1 (write FIRST — must FAIL before T007–T010 land) ⚠️

- [X] T002 [P] [US1] Extend `internal/config/config_test.go`: add a `noHome bool` field to the case structs of `TestResolveSpecsDir` and `TestResolveTeamsDir` and change both loops from `t.Setenv("HOME", fakeHome)` to `home := fakeHome; if tc.noHome { home = "" }; t.Setenv("HOME", home)`. Add to `TestResolveSpecsDir`: `{name: "tilde in config value with no home names specsDir", cfg: Config{SpecsDir: "~/specs"}, noHome: true, wantErr: "resolving specsDir: expanding ~"}`, `{name: "tilde in flag value with no home names --path", flagValue: "~/specs", noHome: true, wantErr: "resolving --path: expanding ~"}`, `{name: "absolute config value with no home succeeds", cfg: Config{SpecsDir: "/abs/specs"}, noHome: true, want: "/abs/specs"}`, `{name: "absolute flag with tilde config and no home returns the flag", flagValue: "/abs/from/flag", cfg: Config{SpecsDir: "~/specs"}, noHome: true, want: "/abs/from/flag"}`. Add to `TestResolveTeamsDir`: `{name: "tilde teamsDir with no home names teamsDir", cfg: Config{TeamsDir: "~/teams"}, noHome: true, wantErr: "resolving teamsDir: expanding ~"}`, `{name: "fallback to tilde specsDir with no home names specsDir", cfg: Config{SpecsDir: "~/specs"}, noHome: true, wantErr: "resolving specsDir: expanding ~"}`, `{name: "tilde flag with no home names --path", flagValue: "~/teams", noHome: true, wantErr: "resolving --path: expanding ~"}`, `{name: "absolute teamsDir with tilde specsDir and no home returns teamsDir", cfg: Config{TeamsDir: "/from/teams", SpecsDir: "~/specs"}, noHome: true, want: "/from/teams"}`. Add a dedicated `TestResolveTeamsDir_FallbackNamesSpecsDirNotTeamsDir`: `t.Setenv("HOME", "")`, `Config{SpecsDir: "~/specs"}.ResolveTeamsDir("")` → error non-nil, contains `resolving specsDir`, and `!strings.Contains(err.Error(), "teamsDir")` (FR-002; contracts/config-api.md)
- [X] T003 [P] [US1] Extend `internal/config/plugin_traefik_test.go` `TestResolveRoutingDir` the same way (`noHome bool` field, `home := fakeHome; if tc.noHome { home = "" }` in the loop) and add `{name: "tilde routingDir with no home names routing-dir", cfg: TraefikPluginConfig{RoutingDir: "~/traefik"}, noHome: true, wantErr: "resolving routing-dir: expanding ~"}` and `{name: "tilde specsDir fallback with no home names specsDir", specsDir: "~/specs/traefik", noHome: true, wantErr: "resolving specsDir: expanding ~"}`. Leave `internal/config/utils_test.go` untouched — `TestExpandTilde` must keep passing on `expanding ~`
- [X] T004 [P] [US1] Add `func (tc *TestCase) AssertStderrNotContains(s string) *TestCase` to `tests/integration/testutils/assert_general.go` directly after `AssertOutputNotContains`, mirroring it against `tc.result.Stderr` (`tc.t.Fatalf("expected stderr NOT to contain %q\nstderr: %s", s, tc.result.Stderr)`)
- [X] T005 [US1] Create `tests/integration/config_paths_test.go` (`//go:build integration`, `package integration_test`, imports `path/filepath`, `strings`, `testing`, dot-import `github.com/CarlosHPlata/shrine/tests/integration/testutils`). Declare `const tildeConfig = "specsDir: ~/manifests\nteamsDir: ~/teams\n"` and `const tildeSpecsOnlyConfig = "specsDir: ~/manifests\n"`; helper `func tildeConfigDir(t *testing.T, tc *TestCase, content string) string { dir := tc.Path("config"); writeConfig(t, dir, content); return dir }` (reuses `writeConfig` from `traefik_plugin_test.go` — same package); helper `func assertStderrOnce(tc *TestCase, s string) { if n := strings.Count(tc.RunResult().Stderr, s); n != 1 { tc.Fatalf("expected %q exactly once on stderr, got %d\nstderr: %s", s, n, tc.RunResult().Stderr) } }`. `func TestConfigPathResolution(t *testing.T)`: `s := NewSuite(t)`; `s.BeforeEach(func(tc *TestCase) { tc.Setenv("HOME", ""); tc.StateDir = tc.TempDir() })`. Scenarios (contracts/cli-behaviour.md), each starting with `cfgDir := tildeConfigDir(t, tc, tildeConfig)` unless noted: **C1** `"should deploy fail naming specsDir when HOME is unset"` — `tc.Run("deploy", "--config-dir", cfgDir, "--state-dir", tc.StateDir).AssertFailure().AssertStderrContains("resolving specsDir: expanding ~").AssertStderrContains("$HOME is not defined")`, then `assertStderrOnce(tc, "resolving specsDir")` and `tc.AssertFileNotExists(filepath.Join(tc.StateDir, "logs"))`; **C2** `"should deploy --dry-run fail the same way"` — `tc.Run("deploy", "--dry-run", …)` same assertions; **C3** `"should deploy team fail naming specsDir"` — `tc.Run("deploy", "team", "any-team", …)`; **C4** `"should apply teams fail naming teamsDir"` — `tc.Run("apply", "teams", …).AssertFailure().AssertStderrContains("resolving teamsDir: expanding ~")` then `tc.AssertTeamCount(0)`; **C5** `"should apply teams name specsDir when teamsDir falls back to it"` — `cfgDir := tildeConfigDir(t, tc, tildeSpecsOnlyConfig)`, `.AssertFailure().AssertStderrContains("resolving specsDir").AssertStderrNotContains("resolving teamsDir")`, `tc.AssertTeamCount(0)`; **C6** `"should apply -f fail naming specsDir"` — `tc.Run("apply", "-f", filepath.Join(fixturesPath(), "..", "app.yml"), "--config-dir", cfgDir, "--state-dir", tc.StateDir).AssertFailure().AssertStderrContains("resolving specsDir")`, `tc.AssertFileNotExists(filepath.Join(tc.StateDir, "logs"))`; **C7** `"should generate team fail naming specsDir"` — `tc.Run("generate", "team", "demo", "--config-dir", cfgDir, "--state-dir", tc.StateDir).AssertFailure().AssertStderrContains("resolving specsDir")`; **C9** `"should name --path when the flag supplied the tilde value"` — `tc.Run("deploy", "--path", "~/manifests", "--config-dir", cfgDir, "--state-dir", tc.StateDir).AssertFailure().AssertStderrContains("resolving --path: expanding ~").AssertStderrNotContains("resolving specsDir")`; **S1+S2** `"should ignore an unresolvable config value when --path is absolute"` — `tc.Run("apply", "teams", "--path", fixturesPath("team"), "--config-dir", cfgDir, "--state-dir", tc.StateDir).AssertSuccess()` then `tc.Run("deploy", "--dry-run", "--path", fixturesPath("basic"), "--config-dir", cfgDir, "--state-dir", tc.StateDir).AssertSuccess().AssertStderrNotContains("resolving")` (`fixturesPath` is defined in `deploy_test.go`, same package). Leave room for the US2 teardown scenario (T013)
- [X] T006 [US1] Verify red: `go test ./internal/config/` fails on the new cases (today's error is `expanding ~: $HOME is not defined` with no `resolving …` prefix); `go vet -tags integration ./tests/integration/...` compiles clean (the scenarios are red by construction and executed by CI)

### Implementation for User Story 1

- [X] T007 [US1] In `internal/config/utils.go` add `type pathSource struct { name, value string }` and change `resolvePath` to `func resolvePath(sources []pathSource, missingErr string) (string, error)`: iterate `sources`, `continue` on empty `value`, `resolved, err := expandTilde(s.value)`; on error `return "", fmt.Errorf("resolving %s: %w", s.name, err)`; otherwise `return resolved, nil`; after the loop `return "", errors.New(missingErr)`. `expandTilde` is unchanged; no comments needed (contracts/config-api.md; research D1–D3)
- [X] T008 [US1] In `internal/config/config.go` update the two callers: `ResolveSpecsDir` → `resolvePath([]pathSource{{"--path", flagValue}, {"specsDir", c.SpecsDir}}, "no specs directory: set --path/-p flag or specsDir in config.yml")`; `ResolveTeamsDir` → `resolvePath([]pathSource{{"--path", flagValue}, {"teamsDir", c.TeamsDir}, {"specsDir", c.SpecsDir}}, "no specs directory: set --path/-p flag, teamsDir or specsDir in config.yml")`. Doc comments and `missingErr` strings stay byte-identical (FR-007)
- [X] T009 [US1] In `internal/config/plugin_traefik.go` update `ResolveRoutingDir` → `resolvePath([]pathSource{{"routing-dir", p.RoutingDir}, {"specsDir", specsDir}}, "no routing directory: set --path/-p flag or routing-dir in config.yml")`
- [X] T010 [US1] In `internal/plugins/gateway/traefik/plugin.go` `resolvedRoutingDir` (line 122) change the wrap from `fmt.Errorf("traefik plugin: resolving routing directory: %w", err)` to `fmt.Errorf("traefik plugin: %w", err)` so the field is named exactly once (`traefik plugin: resolving routing-dir: expanding ~: …`; research D4)
- [X] T011 [US1] Verify green: `go build ./...`; `go test ./internal/config/ ./internal/plugins/gateway/traefik/ ./cmd/` passes (T002/T003 green, every pre-existing case unchanged, `TestExpandTilde` untouched); `go vet -tags integration ./tests/integration/...` clean; `gofmt -l internal/ tests/` prints nothing

**Checkpoint**: Every cmd-layer subcommand names the failing source — MVP complete for issue #37 defect 2 (CI executes T005 as the gate).

---

## Phase 4: User Story 2 - A resolution failure always stops the command, whichever stage hits it first (Priority: P2)

**Goal**: `BuildDeployBundle` and `BuildTeardownBundle` surface an unresolvable `specsDir` with the US1 message before constructing anything (no observer/file logger, plugin, backend, vault, or engine); `teardown` keeps working when `specsDir` is absent (FR-006).

**Independent Test**: `go test ./internal/app/` — both bundles return `(nil, nil, err)` with `resolving specsDir` on `cfg{SpecsDir: "~/manifests"}` + `HOME=""` (no filesystem); `resolveOptionalSpecsDir` distinguishes absent from unresolvable. Integration (CI): `teardown demo-team` with the tilde config exits 1 naming `specsDir` and creates no `<state>/logs/`; the existing `TestTeardown` (no `specsDir` configured) stays green.

### Tests for User Story 2 (write FIRST — must FAIL before T015–T016 land) ⚠️

- [X] T012 [P] [US2] Create `internal/app/app_test.go` (`package app`; imports `io`, `strings`, `testing`, `github.com/CarlosHPlata/shrine/internal/config`): `TestBuildDeployBundle_FailsBeforeConstructionWhenSpecsDirUnresolvable` — `t.Setenv("HOME", "")`; `cfg := &config.Config{SpecsDir: "~/manifests"}`; `bundle, cleanup, err := BuildDeployBundle(cfg, nil, nil, "", io.Discard, io.Discard)`; `err` non-nil containing `resolving specsDir` and `expanding ~`; `bundle == nil`; `cleanup == nil`. `TestBuildTeardownBundle_FailsBeforeConstructionWhenSpecsDirUnresolvable` — same with `BuildTeardownBundle(cfg, nil, nil, io.Discard)`. `TestResolveOptionalSpecsDir` table `{name, specsDir, home, want, wantErr}`: `{"absent specsDir is not an error", "", "/home/test-user", "", ""}`, `{"absolute value passes through", "/abs/specs", "/home/test-user", "/abs/specs", ""}`, `{"tilde expands when home is known", "~/specs", "/home/test-user", "/home/test-user/specs", ""}`, `{"tilde without home names specsDir", "~/specs", "", "", "resolving specsDir"}` — each `t.Setenv("HOME", tc.home)` then `resolveOptionalSpecsDir(&config.Config{SpecsDir: tc.specsDir})`. Nil store/paths are deliberate: today's code proceeds to `newObserverPair(out, paths)` and dereferences nil `paths`, so the tests are red by construction (research D8)
- [X] T013 [P] [US2] Add scenario **C8** to `TestConfigPathResolution` in `tests/integration/config_paths_test.go`: `"should teardown fail naming specsDir before composing anything"` — `cfgDir := tildeConfigDir(t, tc, tildeConfig)`; `tc.Run("teardown", "demo-team", "--config-dir", cfgDir, "--state-dir", tc.StateDir).AssertFailure().AssertStderrContains("resolving specsDir: expanding ~")`; `assertStderrOnce(tc, "resolving specsDir")`; `tc.AssertFileNotExists(filepath.Join(tc.StateDir, "logs"))` — the missing `logs/` directory proves the file logger (and therefore the bundle) was never composed (contracts/cli-behaviour.md C8; data-model side-effect boundary)
- [X] T014 [US2] Verify red: `go test ./internal/app/` fails (nil-pointer panic in the bundle tests, undefined `resolveOptionalSpecsDir`); `go vet -tags integration ./tests/integration/...` compiles clean

### Implementation for User Story 2

- [X] T015 [US2] In `internal/app/components.go` add `func resolveOptionalSpecsDir(cfg *config.Config) (string, error)` next to `newTraefikPlugin`: one WHY comment line (`// Teardown reads no manifests, so an unset specsDir is not an error.`), body `if cfg.SpecsDir == "" { return "", nil }` then `return cfg.ResolveSpecsDir("")` (research D6; contracts/config-api.md)
- [X] T016 [US2] In `internal/app/app.go` replace the two discards: `BuildDeployBundle` (line 113) → `specsDir, err := cfg.ResolveSpecsDir(manifestDir)` / `if err != nil { return nil, nil, err }` (unwrapped — the resolver text is self-describing; research D5), keeping it after `ValidateRegistries` and before `newObserverPair`; `BuildTeardownBundle` (line 177) → `specsDir, err := resolveOptionalSpecsDir(cfg)` / `if err != nil { return nil, nil, err }` as the first statement, before `newObserverPair`. Existing doc comments remain accurate — no edits
- [X] T017 [US2] Verify green: `go build ./...`; `go test ./internal/app/ ./internal/config/` passes; `go vet -tags integration ./tests/integration/...` clean; `gofmt -l internal/app/` prints nothing

**Checkpoint**: Both stories complete — no stage can proceed on a partially-resolved configuration, and `teardown` still tolerates an absent `specsDir` (CI executes T013 and the existing `TestTeardown` as the gate).

---

## Phase 5: Polish & Cross-Cutting Concerns

- [X] T018 [P] Add a troubleshooting section to `docs/content/troubleshooting/_index.md` immediately before `## See also` (line 29), matching the existing symptom → fix style: heading `` ## `resolving specsDir: expanding ~: $HOME is not defined` `` and one paragraph — a `~`-prefixed `specsDir`, `teamsDir`, `--path`, or `routing-dir` value cannot be expanded because the process has no home directory (typical under cron, CI runners, systemd units, or containers); the error names the field or flag to fix — use an absolute path there, export `HOME`, or pass an absolute `--path`. Front matter untouched (research D10)
- [X] T019 [P] Update `specs/features/integration-tests.md`: add `tc.AssertStderrNotContains(s)` to the fluent-API list (after `AssertStderrContains`, line ~23); insert a `### Config path resolution — spec 026` subsection after Phase 3 (before `### Phase 4`, line 128) stating it is a `NewSuite` file with no Docker, `HOME` empty via `tc.Setenv`, and listing the `tests/integration/config_paths_test.go` scenarios C1–C9 and S1+S2 by name with their assertions (named source on stderr, once-only, `AssertTeamCount(0)`, `<state>/logs` absent), plus a note that the existing `TestTeardown` remains the gate for "absent `specsDir` still tolerated"
- [X] T020 [P] Add the fix entry to `specs/progress.md` directly above the 025 entry (line 89), same style: `- [x] **Fix: field-naming config path errors that always stop the command** — see `specs/026-fix-config-path-errors/` (issue #37). …` summarising `pathSource`-labelled `resolvePath` (`resolving <--path|specsDir|teamsDir|routing-dir>: expanding ~: …`, fallback names the supplier), the simplified Traefik wrap, `BuildDeployBundle` propagation, `resolveOptionalSpecsDir` in `BuildTeardownBundle` (absent tolerated, unresolvable fails before the file logger), acceptance SC-001–SC-004, and the gate (`TestConfigPathResolution` C1–C9/S1–S2 + existing `TestTeardown`, CI executes)
- [X] T021 Run the full local gate at repository root: `gofmt -l .` prints nothing, `go vet ./...` clean, `go test ./...` zero failures, `go vet -tags integration ./tests/integration/...` clean (SC-003)
- [X] T022 [P] Run `graphify update .` to refresh the knowledge graph after the code changes (project rule, AST-only)
- [X] T023 Push the branch and confirm the CI integration pipeline is green — it executes `TestConfigPathResolution` (T005/T013) and `TestTeardown`, which together form the Constitution V gate; optionally walk `specs/026-fix-config-path-errors/quickstart.md` against a built binary with `env -u HOME`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: empty — nothing blocks the stories beyond T001
- **US1 (Phase 3)**: after T001 — touches `internal/config/`, `internal/plugins/gateway/traefik/plugin.go`, `tests/integration/testutils/assert_general.go`, `tests/integration/config_paths_test.go`
- **US2 (Phase 4)**: after US1 (T011) — its assertions rely on the `resolving specsDir` text T007 introduces; production changes are confined to `internal/app/`, and T013 appends to the file T005 created
- **Polish (Phase 5)**: after both story phases

### Story Completion Order

```text
T001 ─▶ US1 (T002…T011) ─▶ US2 (T012…T017) ─▶ Polish (T018…T023)
```

### Parallel Opportunities

- **T002, T003, T004 [P]** are three different files with no dependencies; **T005** needs T004 (`AssertStderrNotContains`) to compile
- **T007 → T008 → T009** are sequential only because T008/T009 do not compile until `pathSource` exists (T007); T009 and T010 touch different files and can land together after T008
- **T012 and T013 [P]** are different files (`internal/app/app_test.go` vs `tests/integration/config_paths_test.go`)
- **T018, T019, T020, T022 [P]** touch disjoint files
- Within US1, the author of T007–T010 can start while a second author writes T005 — the integration file compiles against the unchanged binary and is red by construction

## Parallel Example: User Story 1

```bash
# Right after T001, three authors, disjoint files:
Task: "T002: noHome cases in internal/config/config_test.go"
Task: "T003: noHome cases in internal/config/plugin_traefik_test.go"
Task: "T004: AssertStderrNotContains in tests/integration/testutils/assert_general.go"
# then
Task: "T005: TestConfigPathResolution in tests/integration/config_paths_test.go"
```

## Parallel Example: User Story 2

```bash
# After T011:
Task: "T012: bundle + resolveOptionalSpecsDir tests in internal/app/app_test.go"
Task: "T013: teardown scenario C8 in tests/integration/config_paths_test.go"
```

## Implementation Strategy

### MVP First (US1 only)

1. T001, then US1 red tests (T002–T006), then T007–T011.
2. **STOP and VALIDATE**: `go test ./internal/config/` green with every source named; integration file compiles; CI executes it.
3. This alone closes issue #37's visible defect — every operator-facing command now names the field to fix.

### Incremental Delivery

1. US1 → field-naming errors from every cmd-layer subcommand (deployable on its own).
2. US2 → the composition root can no longer swallow the failure; `teardown` gains the same error and keeps tolerating an absent `specsDir`.
3. Polish → troubleshooting entry, integration-tests spec, progress entry, graph refresh, CI gate; ship as one PR closing issue #37.

### Format validation

All 23 tasks use the checklist format (`- [ ] Txxx [P?] [Story?] description + exact file path`); story labels appear only in Phases 3–4.
