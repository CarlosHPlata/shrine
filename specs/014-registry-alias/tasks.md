# Tasks: Registry Aliases

**Input**: Design documents from `specs/014-registry-alias/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no shared state)
- **[Story]**: User story label — US1, US2, US3
- Exact file paths are listed in every task description

---

## Phase 2: Foundational — Config Struct Extension

**Purpose**: Add the `Alias` field to `RegistryConfig`. This is a pure structural
change (no behavior) that unblocks all three user stories. Must complete before any
other phase.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T0\1 Add `Alias string \`yaml:"alias,omitempty"\`` field to `RegistryConfig` struct in `internal/config/config.go`

**Checkpoint**: `RegistryConfig` has the new field; existing config files load without change.

---

## Phase 3: User Story 1 — Config Alias Validation (Priority: P1)

**Goal**: Operators can add an `alias` to a registry entry in `config.yml`; duplicate
or malformed aliases are caught at startup before any manifest is processed.

**Independent Test**: Run `shrine deploy --dry-run --config-dir <dir>` where `<dir>/config.yml`
contains a duplicate alias → the command must exit non-zero with a message naming the duplicate.

### TDD: Integration Test Fixtures & Tests for User Story 1

> **Write these BEFORE T006–T009 and verify they FAIL before implementation.**

- [x] T0\1 [P] [US1] Create `tests/testdata/deploy/registry-alias/config.yml` containing a registry entry with `alias: myregistry` (valid case)
- [x] T0\1 [P] [US1] Create `tests/testdata/deploy/registry-alias-dup/config.yml` containing two registry entries sharing the same alias (duplicate case)
- [x] T0\1 [P] [US1] Create `tests/testdata/deploy/registry-alias-badformat/config.yml` containing a registry alias with a dot in the name (invalid format case)
- [x] T0\1 [US1] Write integration tests for US1 scenarios in `tests/integration/registry_alias_test.go`:
  - valid alias loads without error (dry-run succeeds)
  - duplicate alias produces error naming the alias
  - alias with invalid characters produces a format error

### Implementation for User Story 1

- [x] T0\1 [P] [US1] Add `ValidateRegistries() error` to `*Config` in `internal/config/config.go` — validates alias uniqueness (case-sensitive) and format (`^[a-zA-Z0-9_-]+$`)
- [x] T0\1 [P] [US1] Add unit tests for `ValidateRegistries` covering: valid aliases, duplicate alias, invalid characters, missing alias (no-op) in `internal/config/config_test.go`
- [x] T0\1 [US1] Call `cfg.ValidateRegistries()` in `DryRun` after `config.Load` in `internal/handler/deploy.go`; return error on failure
- [x] T0\1 [US1] Call `cfg.ValidateRegistries()` in `Deploy` (live path) after config load in `internal/handler/deploy.go`; return error on failure
- [x] T0\1 [US1] Call `cfg.ValidateRegistries()` in `ApplySingle` after config load in `internal/handler/apply.go`; return error on failure

**Checkpoint**: `shrine deploy --dry-run --config-dir <dup-dir>` exits non-zero with a
"duplicate alias" message. US1 integration tests pass.

---

## Phase 4: User Story 2 — App Image `reg:` Prefix (Priority: P1)

**Goal**: Application manifests use `image: reg:<alias>/image:tag`; the planner validates
the alias exists at plan/dry-run time; the docker backend expands the alias at live
execution time; dry-run output preserves the `reg:` form.

**Independent Test**: Create an app manifest with `image: reg:myregistry/whoami:latest`, a
config with `alias: myregistry`, run `shrine deploy --dry-run --config-dir <dir>` and
verify the output shows `reg:myregistry/whoami:latest` (alias preserved). Then verify that
using an unknown alias produces an error naming the alias.

### TDD: Integration Test Fixture & Tests for User Story 2

> **Write these BEFORE T016–T023 and verify they FAIL before implementation.**

- [x] T0\1 [P] [US2] Create `tests/testdata/deploy/registry-alias/app.yml` — application manifest with `image: reg:myregistry/traefik/whoami:latest`
- [x] T0\1 [P] [US2] Create `tests/testdata/deploy/registry-alias-unknown/` fixture — app manifest referencing `reg:unknown/whoami:latest` with a config that has no matching alias
- [x] T0\1 [US2] Write integration tests for US2 in `tests/integration/registry_alias_test.go`:
  - dry-run output shows `reg:myregistry/traefik/whoami:latest` (alias preserved, not expanded)
  - unknown alias produces error: `alias "unknown" is not defined in config registries`
  - plain image (`image: traefik/whoami`) is unaffected (regression guard)

### Implementation for User Story 2

- [x] T0\1 [P] [US2] Add `validateRegistryImages(set *ManifestSet, registries []config.RegistryConfig) []error` to `internal/planner/resolve.go`:
  - iterate `set.Applications`; for each `app.Spec.Image` starting with `reg:`, extract alias and verify it exists in the alias map
  - return descriptive error: `app "<name>": image "<ref>": alias "<a>" is not defined in config registries`
  - empty alias after `reg:` (`reg:/image`) is also an error
- [x] T0\1 [US2] Extend `Resolve(set, store)` signature to `Resolve(set, store, registries)` in `internal/planner/resolve.go`; call `validateRegistryImages` as step 5 in the existing validation sequence
- [x] T0\1 [US2] Update `Plan` in `internal/planner/plan.go` to accept `registries []config.RegistryConfig` and pass them to `Resolve`
- [x] T0\1 [US2] Update `PlanSingle` in `internal/planner/plan.go` to accept `registries []config.RegistryConfig` and pass them to `Resolve`
- [x] T0\1 [US2] Update `DryRun` call in `internal/handler/deploy.go` to pass `cfg.Registries` to `planner.Plan`
- [x] T0\1 [US2] Update `Deploy` call in `internal/handler/deploy.go` to pass `cfg.Registries` to `planner.Plan`
- [x] T0\1 [US2] Update `ApplySingle` call in `internal/handler/apply.go` to pass `cfg.Registries` to `planner.PlanSingle`
- [x] T0\1 [P] [US2] Add `hasRegistryAliasPrefix(ref string) bool`, `buildAliasMap(registries []config.RegistryConfig) map[string]string`, and `expandRegistryAlias(ref string, registries []config.RegistryConfig) (string, error)` to `internal/engine/local/dockercontainer/registry_auth.go`
- [x] T0\1 [US2] Call `expandRegistryAlias` at the top of `ensureImage` in `internal/engine/local/dockercontainer/docker_image.go`; propagate error
- [x] T0\1 [US2] Call `expandRegistryAlias` at the top of `resolveImage` in `internal/engine/local/dockercontainer/docker_image.go`; propagate error
- [x] T0\1 [P] [US2] Add unit tests for `hasRegistryAliasPrefix`, `buildAliasMap`, and `expandRegistryAlias` in `internal/engine/local/dockercontainer/registry_auth_test.go` (create file)
- [x] T0\1 [P] [US2] Add unit tests for `validateRegistryImages` (valid alias, missing alias, empty alias, plain image passthrough) in `internal/planner/resolve_test.go`

**Checkpoint**: US2 integration tests pass. `shrine deploy --dry-run` shows alias as-is;
unknown alias fails at dry-run time with a clear error.

---

## Phase 5: User Story 3 — Resource Image `reg:` Prefix (Priority: P2)

**Goal**: Resource manifests support the same `reg:<alias>` syntax as applications.

**Independent Test**: Create a resource manifest with `image: reg:myregistry/postgres:15`,
run dry-run, verify alias is shown as-is with no error. Verify unknown alias fails.

### TDD: Integration Test Fixture & Tests for User Story 3

> **Write these BEFORE T027 and verify they FAIL (if resources not yet covered).**

- [x] T0\1 [P] [US3] Create `tests/testdata/deploy/registry-alias-resource/` fixture — resource manifest with `image: reg:myregistry/postgres:15` and matching config
- [x] T0\1 [US3] Write integration tests for US3 in `tests/integration/registry_alias_test.go`:
  - dry-run shows `reg:myregistry/postgres:15` for a resource (alias preserved)
  - unknown alias on a resource produces error naming the resource and alias

### Implementation for User Story 3

- [x] T0\1 [US3] Extend `validateRegistryImages` in `internal/planner/resolve.go` to also iterate `set.Resources`; for each `res.Spec.Image` starting with `reg:`, apply the same alias check with error: `resource "<name>": image "<ref>": alias "<a>" is not defined in config registries`

**Checkpoint**: US3 integration tests pass. Resources behave identically to applications
for `reg:` images.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [x] T0\1 [P] Verify error messages in `internal/config/config.go`, `internal/planner/resolve.go`, and `internal/engine/local/dockercontainer/registry_auth.go` match the contracts defined in `specs/014-registry-alias/contracts/registry-config.md`
- [x] T0\1 Run `go test ./...` to confirm zero regressions across all unit tests
- [x] T0\1 Run `make test-integration` (or `go test -tags integration ./tests/integration/...`) as the final gate; all integration tests must pass

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 2 (Foundational)**: No dependencies — start immediately
- **Phase 3 (US1)**: Depends on Phase 2 (T001)
- **Phase 4 (US2)**: Depends on Phase 3 completion (T001–T010)
- **Phase 5 (US3)**: Depends on Phase 4 completion — specifically T014/T015 which implement `validateRegistryImages`
- **Phase 6 (Polish)**: Depends on all story phases complete

### User Story Dependencies

- **US1 (P1)**: Can start after T001 — no dependency on US2/US3
- **US2 (P1)**: Depends on US1 completion (ValidateRegistries wired, Alias field available)
- **US3 (P2)**: Depends on US2 completion (validateRegistryImages already exists, just extend it)

### Within Each Phase

- TDD fixtures [P] (T002–T004, T011–T012, T026) can be created in parallel
- Integration test file tasks (T005, T013, T027) must follow their respective fixtures
- Implementation tasks must follow their integration test tasks (tests fail first)
- `expandRegistryAlias` (T021) can be written in parallel with planner changes (T014–T017)
- Unit test tasks marked [P] can be written in parallel with their implementation counterparts

---

## Parallel Example: Phase 4 (User Story 2)

```text
# Fixtures can be created in parallel:
Task T011: Create registry-alias/app.yml
Task T012: Create registry-alias-unknown/ fixture

# After fixtures: write integration tests (T013) [FAIL first]

# After tests: implementation can parallelize across layers:
Task T014: validateRegistryImages in internal/planner/resolve.go
Task T021: expandRegistryAlias in internal/engine/local/dockercontainer/registry_auth.go

# After T014: planner signature + handler wiring (sequential per dependency):
Task T015 → T016 → T017 → T018 → T019 → T020

# After T021: engine call sites (T022, T023)

# Unit tests can be written in parallel with implementation:
Task T024: registry_auth_test.go
Task T025: resolve_test.go
```

---

## Implementation Strategy

### MVP First (User Story 1 + 2)

1. Complete Phase 2: Foundational (T001) — config field
2. Complete Phase 3: US1 (T002–T010) — config validation
3. Complete Phase 4: US2 (T011–T025) — app image expansion
4. **STOP and VALIDATE**: `shrine deploy --dry-run` works with `reg:` aliases
5. Deliver MVP

### Incremental Delivery

1. T001 → US1 (T002–T010) → Foundation: aliases defined in config ✓
2. T011–T025 (US2) → App `reg:` images work end-to-end ✓ (MVP)
3. T026–T028 (US3) → Resource `reg:` images work ✓
4. T029–T031 (Polish) → Verified clean ✓

---

## Notes

- [P] = different files with no overlapping state — safe to run in parallel
- TDD order is mandatory per Constitution Principle V: integration tests must exist and FAIL before implementation
- `ValidateRegistries()` is called in handlers (not in `config.Load`) to preserve `Load`'s pure unmarshal contract
- Dry-run preserves the `reg:` alias form because `DryRunContainerBackend` never calls `expandRegistryAlias` — expansion lives only in `DockerBackend`
- The `Resolve` signature change (adding `registries`) will require updating all existing call sites in `plan.go` — unit tests in `resolve_test.go` and callers in `handler/` must be updated together
