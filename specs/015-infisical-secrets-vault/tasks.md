# Tasks: Secrets Vault Plugin (Infisical)

**Input**: Design documents from `specs/015-infisical-secrets-vault/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/secrets-config.md, quickstart.md

**Integration tests**: Written as TDD-first skeleton tasks (they will fail until implemented — this is expected). Do NOT run integration tests locally; push to CI via PR.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (US1–US4)

---

## Phase 1: Setup

**Purpose**: Add the Infisical Go SDK dependency before any code is written.

- [x] T001 Add `github.com/infisical/go-sdk` dependency by running `go get github.com/infisical/go-sdk` and committing the updated `go.mod` and `go.sum`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core types and interface that ALL user stories depend on. No story work can begin until this phase is complete.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T002 [P] Create `SecretsPlugin` interface in `internal/plugins/secrets/plugin.go` with exactly two methods: `IsActive() bool` and `GetSecret(path string) (string, error)`
- [x] T003 [P] Create `InfisicalPluginConfig` struct in `internal/config/plugin_infisical.go` with `URL`, `ClientID`, and `ClientSecret` string fields and their `yaml` tags (`url`, `client-id`, `client-secret`)
- [x] T004 Add `SecretsPluginsConfig` struct and `Secrets SecretsPluginsConfig` field to `PluginsConfig` in `internal/config/config.go`; add `validateSecretsPlugins()` that returns an error when more than one `plugins.secrets.*` block is non-nil; call it from `Load()` after YAML unmarshalling
- [x] T005 [P] Write integration test fixtures in `tests/testdata/deploy/vault-secrets/`: a `shrine.yml` with `plugins.secrets.infisical` pointing to a local test instance, a Resource manifest with one `valueFrom: vault:` output, and an Application manifest that consumes that Resource output plus one direct `valueFrom: vault:` env var; include `docker-compose.yml` for the Infisical side-stack (infisical + postgres + redis) in the same directory
- [x] T006 Write integration test scenario in `tests/integration/deploy_test.go` covering vault secret resolution — start Infisical via the compose file in `tests/testdata/deploy/vault-secrets/`, provision project and secrets via API, run `shrine apply` as subprocess, assert the Application container has correct env vars for both the direct vault ref and the vault-backed Resource output; mark with `//go:build integration` tag and leave failing (CI-only)

**Checkpoint**: Interface defined, config types ready, integration test skeleton written. User story implementation can now begin.

---

## Phase 3: User Story 2 — Configure Vault Plugin in shrine.yml (Priority: P1)

**Goal**: Shrine accepts `plugins.secrets.infisical` config in shrine.yml, activates the plugin in live-deploy handlers, and rejects dual-plugin declarations at config load.

**Independent Test**: Add `plugins.secrets.infisical` block to shrine.yml, run `shrine dry-run` — no error. Remove the block, add a `valueFrom: vault:` ref, run `shrine apply` — clear error: "no secrets plugin configured".

- [x] T007 [US2] Wire `InfisicalPlugin` construction in `internal/handler/deploy.go:Deploy()` — construct the plugin from `cfg.Plugins.Secrets.Infisical` (returns nil-safe no-op when config is nil), pass it into `NewLocalEngineWithRouting`; `DryRun()` requires no changes (config validation is automatic via `config.Load()`; `DryRunResolver` handles vault refs as placeholders without the plugin)
- [x] T008 [US2] Wire `InfisicalPlugin` construction in `internal/handler/apply.go:ApplySingle()` — same pattern as `deploy.go:Deploy()`; construct and pass the plugin into the engine

**Checkpoint**: Config is accepted, validated (dual-plugin error), and handlers construct the plugin. Dry-run is unaffected.

---

## Phase 4: User Story 1 — Reference Vault Secrets in Application Manifests and Resource Outputs (Priority: P1)

**Goal**: `valueFrom: vault:project/env/key` resolves to the actual secret value at deploy time — in Application `spec.env[]` directly, and in Resource `spec.outputs[]` (making the resolved value available to downstream Applications via `valueFrom: resource.<name>.<output>`).

**Independent Test**: Configure shrine.yml with a live Infisical connection; deploy a Resource manifest with a `valueFrom: vault:` output and an Application that consumes it plus one direct `valueFrom: vault:` env var; assert both container env vars equal the values stored in Infisical.

- [x] T009 [P] [US1] Implement `InfisicalPlugin` struct in `internal/plugins/secrets/infisical/plugin.go` — `New(cfg *config.InfisicalPluginConfig) (*InfisicalPlugin, error)` initialises and authenticates the Infisical SDK client (returns nil, nil when cfg is nil); `IsActive()` returns false when cfg is nil; `GetSecret(path string)` splits path on `/` into `[project, environment, secretKey]` and calls `client.Secrets().Retrieve(...)`, returning only the value or an error that includes the path but never the value
- [x] T010 [P] [US1] Write unit tests for `InfisicalPlugin` in `internal/plugins/secrets/infisical/plugin_test.go` — use a mock or stub Infisical SDK client; cover: `IsActive()` false when nil config, `GetSecret()` success, `GetSecret()` error propagates path (not value), `New()` returns nil when cfg is nil
- [x] T011 [US1] Add `ValueFrom string \`yaml:"valueFrom,omitempty"\`` to the Resource `Output` struct in `internal/manifest/types.go`; extend the mutual-exclusion check in `internal/manifest/validate.go` to include `valueFrom` as a fourth exclusive option for Resource outputs (alongside `value`, `generated`, `template`)
- [x] T012 [US1] Extend `validateValueFrom` in `internal/planner/resolve.go` to accept the `vault:` prefix for both Application env vars and Resource outputs; validate the path splits into exactly 3 non-empty `/`-separated components and return a descriptive plan-time error if not
- [x] T013 [US1] Extend `LiveResolver` in `internal/resolver/resolver.go`: add `Vault secrets.SecretsPlugin` field; add private helpers `isVaultRef(s string) bool` and `parseVaultPath(s string) string`; add `vault:` case to `lookupValueFrom` used by `ResolveApplication`; extend `ResolveResource` to resolve output `ValueFrom` vault refs using the same helpers; update `NewLiveResolver` signature to `NewLiveResolver(store state.SecretStore, vault secrets.SecretsPlugin) Resolver`
- [x] T014 [US1] Update `internal/engine/local/local_engine.go` — pass the vault plugin argument through `NewLocalEngineWithRouting` (or equivalent constructor) down to `NewLiveResolver` so the updated signature is satisfied
- [x] T015 [US1] Extend unit tests in `internal/resolver/resolver_test.go`: vault ref in Application env resolved via active plugin; vault ref in Resource output resolved via active plugin; nil/inactive plugin returns error for both paths; non-vault `valueFrom` values unaffected; missing project/environment/secret surfaces full path in error message; unexpected vault SDK error surfaces path + SDK message
- [x] T016 [US1] Run `go test ./...` and confirm all unit tests pass

**Checkpoint**: `shrine apply` resolves `valueFrom: vault:` env vars from a live Infisical instance.

---

## Phase 5: User Story 3 — Dry-Run Shows Vault Secret Placeholders (Priority: P2)

**Goal**: `shrine dry-run` renders `[VAULT:<path>]` for vault refs without contacting the vault backend.

**Independent Test**: Run `shrine dry-run` on a manifest with `valueFrom: vault:project/env/key` with no live Infisical instance — output contains `[VAULT:project/env/key]` and command exits 0.

- [x] T017 [US3] Extend `DryRunResolver` in `internal/resolver/dry_run_resolver.go` — add a `vault:` branch in both Application env and Resource output resolution paths using the shared `isVaultRef` and `parseVaultPath` helpers; return `fmt.Sprintf("[VAULT:%s]", parseVaultPath(valueFrom))` with no network call
- [x] T018 [US3] Extend unit tests in `internal/resolver/resolver_test.go` for the dry-run placeholder: `vault:` ref in Application env produces `[VAULT:<path>]`; `vault:` ref in Resource output produces `[VAULT:<path>]`; no vault plugin is called; non-vault refs unaffected
- [x] T019 [US3] Run `go test ./...` and confirm all unit tests pass including the new dry-run placeholder tests

**Checkpoint**: `shrine dry-run` works correctly with vault refs and needs no vault connectivity.

---

## Phase 6: User Story 4 — Plugin Interface Extensibility (Priority: P3)

**Goal**: Confirm the `SecretsPlugin` interface and config loading are provider-agnostic, so a future alternative vault backend requires only: a new implementation of `SecretsPlugin` and a new field in `SecretsPluginsConfig`.

**Independent Test**: Inspect `internal/plugins/secrets/plugin.go` — zero Infisical imports. Inspect `internal/config/config.go` — `validateSecretsPlugins()` counts non-nil fields generically, no Infisical-specific logic.

- [x] T020 [US4] Audit `internal/plugins/secrets/plugin.go` — confirm the `SecretsPlugin` interface has no Infisical-specific method signatures, imports, or comments; if any exist, remove them
- [x] T021 [US4] Audit `internal/config/config.go:validateSecretsPlugins()` — confirm the check counts non-nil `SecretsPluginsConfig` fields without referencing Infisical by name; if any provider-specific logic exists, refactor to be generic

**Checkpoint**: Adding a second vault provider requires only a new `SecretsPlugin` impl and a new field in `SecretsPluginsConfig` — zero changes to the resolver, planner, or manifest format.

---

## Phase 7: Polish & Documentation (FR-012)

**Purpose**: Operator-facing documentation shipped alongside the implementation.

- [x] T022 [P] Create `docs/content/guides/secrets-vault.md` — cover: concept, activation, Application env syntax, Resource output syntax (with downstream Application consumption example), dry-run placeholder output, common pitfalls table (missing config, malformed path, auth failure, project/environment not found, vault SDK error), see-also links
- [x] T023 [P] Update `docs/content/guides/_index.md` — add `- [Secrets vault](secrets-vault/) — Store secrets in an external vault and reference them from manifests.` to the Contents list
- [x] T024 [P] Update `docs/content/reference/manifest-schema.md` — extend Application `spec.env[].valueFrom` row to document `vault:<path>`; add `valueFrom` as a fourth option in the Resource `spec.outputs[]` table (alongside `value`, `generated`, `template`); update Templating prose
- [x] T025 Run `go test ./...` as a final pass to confirm all unit tests still pass after documentation changes

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Foundational)**: Requires Phase 1 — **blocks all user story phases**
- **Phase 3 (US2)**: Requires Phase 2 — config types and interface must exist before handler wiring
- **Phase 4 (US1)**: Requires Phase 3 — handlers must pass the plugin before the resolver can use it
- **Phase 5 (US3)**: Requires Phase 4 — dry-run resolver uses shared helpers defined in resolver.go (T013)
- **Phase 6 (US4)**: Requires Phase 4 + Phase 5 — audit after implementation is complete
- **Phase 7 (Docs)**: Requires Phase 4 + Phase 5 — document the behaviour that was implemented

### User Story Dependencies

- **US2 (P1)**: Depends on Foundational (Phase 2)
- **US1 (P1)**: Depends on US2 (Phase 3) — handlers must wire the plugin before resolver extension is useful
- **US3 (P2)**: Depends on Phase 4 (helpers defined in resolver.go) — extend dry-run after live path is in place
- **US4 (P3)**: Depends on US1 + US3 — architectural audit after full implementation

### Within Each Phase

- Tasks marked [P] within a phase can run in parallel
- T009 and T010 (InfisicalPlugin impl + unit tests) are parallel
- T011, T012, T013, T014 must be sequential within Phase 4 (manifest types → planner → resolver → engine wiring)
- T022, T023, T024 (all doc files) are parallel

---

## Parallel Opportunities

### Phase 2 (Foundational)

```
T002 SecretsPlugin interface        ← in parallel →  T003 InfisicalPluginConfig
T004 Config wiring (after T002+T003)
T005 Integration test fixtures      ← in parallel with T004
T006 Integration test scenario      (after T005)
```

### Phase 4 (US1)

```
T009 InfisicalPlugin impl     ← in parallel →  T010 InfisicalPlugin unit tests
T011 Manifest types change    (after T002)
T012 Planner validation       (after T011)
T013 LiveResolver extension   (after T012)
T014 local_engine.go update   (after T013)
T015 Resolver unit tests      (after T013)
```

### Phase 7 (Docs)

```
T022 secrets-vault.md guide
T023 guides/_index.md update    ← all three in parallel
T024 manifest-schema.md update
```

---

## Implementation Strategy

### MVP (User Stories 1 + 2 only)

1. Complete Phase 1: Add SDK dependency
2. Complete Phase 2: Interface + config types + integration test skeleton
3. Complete Phase 3: Handler wiring (US2)
4. Complete Phase 4: Resolver + Infisical plugin (US1)
5. **STOP and VALIDATE**: Open PR → CI runs integration tests → confirm vault secret resolution end-to-end
6. Merge if green

### Incremental Delivery

1. Setup + Foundational → foundation ready
2. US2 (config wiring) + US1 (resolver) → `shrine apply` resolves vault secrets (core value)
3. US3 (dry-run) → `shrine dry-run` shows placeholders without vault connectivity
4. US4 (audit) → confirm extensibility contract holds
5. Docs → operator-facing guide published

---

## Notes

- Integration tests (`tests/integration/deploy_test.go`) are written in Phase 2 as TDD-first stubs. They will fail locally and on CI until Phase 4 is complete — this is expected and correct.
- Do NOT run `make test-integration` or `go test -tags integration ./...` locally. Push to CI via PR.
- `go test ./...` (unit tests only) can and should be run locally after each phase.
- Secret values MUST NOT appear in any error message or log output — enforce during code review of T009.
- Commit after each completed task or logical group.
