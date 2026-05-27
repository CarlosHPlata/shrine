---
description: "Task list for Split Resource env and output (SRP)"
---

# Tasks: Split Resource `env` and `output` (SRP)

**Input**: Design documents from `/specs/021-resource-env-output-split/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/resource-env-output.md, quickstart.md

**Tests**: Included. The constitution (Principle V) enforces TDD — integration
test files are created before implementation, and unit tests are written to fail
first. Per project memory: unit tests MUST NOT touch the filesystem (use
`t.Setenv`, in-memory fakes); integration tests are slow (~10 min) — run the
full suite sparingly, iterate with `go test ./...`.

**Organization**: Tasks grouped by user story. This feature is a schema +
resolver refactor, so Phase 2 (Foundational) is heavy: it lands the new
`spec.env` field and the `ResolvedResource{Env, Exports}` resolver contract that
every story depends on. FR-014 (resources as first-class consumers, confirmed in
clarification) is a non-user-story requirement carried in its own phase.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no incomplete dependencies)
- **[Story]**: US1 / US2 / US3 (user-story phases only)
- Paths are repository-relative.

---

## Phase 1: Setup

**Purpose**: Track the phase per the constitution's development workflow.

- [x] T001 Add the `021-resource-env-output-split` phase and its acceptance criteria (SC-001…SC-005) to `specs/progress.md`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Land the new manifest field and the new resolver contract so the
build is green and a resource's `env` reaches its container and its `Exports`
flow to consumers. **No user story can begin until this phase is complete.**

**⚠️ CRITICAL**: This phase changes the `resolver.Resolver` interface; the tree
will not compile until T003–T008 are done together.

- [x] T002 [P] Add `Env []EnvVar` to `ResourceSpec`, add `Generated bool` to `EnvVar`, and update the `Output` doc comment (retain `Value`/`Generated`/`ValueFrom` solely for rejection detection) in `internal/manifest/types.go`
- [x] T003 Add `ResolvedResource{Env, Exports map[string]string}` and change the `Resolver` interface to `ResolveResource(res *manifest.ResourceManifest, deps ResolvedDependencies) (ResolvedResource, error)` in `internal/resolver/resolver.go`
- [x] T004 [P] Write failing unit tests for `LiveResolver.ResolveResource`: env resolution (`value`, `generated` reusing the `<resource>.<name>` secret key, `valueFrom: vault:`, cross-manifest `valueFrom` via `deps`, `template`) and export computation (non-template re-export, `host`/`port`, template over a private env var) in `internal/resolver/resolver_test.go`
- [x] T005 Implement `LiveResolver.ResolveResource` — build `Env` from `spec.env`, then `computeExports` from `spec.output` (templates rendered against resolved env + `host`/`port`/`team`/`name`; non-template entries copy the env var or built-in) — in `internal/resolver/resolver.go` (depends on T003, T002; makes T004 pass)
- [x] T006 [P] Implement `DryRunResolver.ResolveResource` with placeholder `Env` (`[GENERATED]`, `[VAULT:…]`, `[PORT]`) and computed `Exports`, updating `internal/resolver/dry_run_resolver.go` and `internal/resolver/dry_run_resolver_test.go` (depends on T003)
- [x] T007 Update `Engine.ExecuteDeploy`/`deployResource` to set `deps.Resources[name] = resolved.Exports` and build the container environment from `resolved.Env` in `internal/engine/engine.go` (depends on T003, T005)
- [x] T008 Update engine unit tests for the new `ResolvedResource` return (container env from `Env`, consumer map from `Exports`) in `internal/engine/engine_test.go` (depends on T007)

**Checkpoint**: `go build ./...` and `go test ./...` green; a resource's `env`
reaches the container and its `Exports` populate `deps.Resources`.

---

## Phase 3: User Story 1 - Configure a resource and curate what it exposes (Priority: P1) 🎯 MVP

**Goal**: An operator declares runtime config in `env` (incl. `generated` and
`valueFrom: vault:`) and a curated interface in `output` (re-exports, `host`, a
derived `template`), and gets correct, validated behavior.

**Independent Test**: Deploy a resource with mixed `env` and an `output` listing
a subset + `host` + a `DB_URL` template; assert the container holds all resolved
env and the published interface is exactly the listed keys.

### Tests for User Story 1 (write first, must fail)

- [x] T009 [US1] Write failing unit tests for resource `env` validation (name required; reserved built-in names `host`/`port`/`team`/`name` rejected; exactly one of `value`/`valueFrom`/`template`/`generated`; unique names) AND `output` valid-surface (name + optional template; non-template `name` must match a declared env var or `host`/`port`; unique names) in `internal/manifest/validate_test.go`
- [x] T011 [US1] Write failing unit tests for output-template referenceable names (declared env names + `host`/`port`/`team`/`name`; reject unknown names; reject output→output references) in `internal/planner/templates_test.go`
- [x] T013 [US1] Create failing integration test `tests/integration/deploy_resource_env_output_test.go` (build tag `integration`, `NewDockerSuite`): deploy a resource with `env` (`value` + `generated` + `template`) and `output` (`POSTGRES_DB`, `host`, `DB_URL` template referencing the private password); a consumer app reads `resource.<name>.DB_URL` and `resource.<name>.POSTGRES_DB`; assert container env and resolved consumer values

### Implementation for User Story 1

- [x] T010 [US1] Implement resource `env` validation and `output` valid-surface/non-template-match validation (multi-error) in `validateResourceSpec` in `internal/manifest/validate.go` (makes T009 pass)
- [x] T012 [US1] Implement output-template validation (referenceable = declared env names + built-ins; no output→output) and resource env-template validation in `internal/planner/templates.go` (makes T011 pass)
- [x] T014 [US1] Resolve any gaps so the US1 integration scenario passes end-to-end (`port` export requires `spec.port`; built-in `host` formatting) across `internal/resolver/` and `internal/manifest/` (makes T013 pass)

**Checkpoint**: US1 fully functional and independently testable — authoring,
validation, container env, and curated exports all correct.

---

## Phase 4: User Story 2 - Keep configuration private unless explicitly exported (Priority: P2)

**Goal**: A consumer can read only keys listed in a resource's `output`;
un-exported env vars (including secrets) are unreadable.

**Independent Test**: An app referencing an exported key resolves; an app
referencing a declared-but-unexported env var is rejected with a clear error.

### Tests for User Story 2 (write first, must fail)

- [x] T015 [US2] Write failing unit tests for the strict allowlist in `validateValueFrom`: `resource.X.<exported>` OK; `resource.X.<unexported-env-var>` → error; `application.X.<key>` valid only for `host`/`port` in `internal/planner/resolve_test.go`

### Implementation for User Story 2

- [x] T016 [US2] Enforce the allowlist in `validateValueFrom` — a `resource.<name>.<key>` reference is valid only if `<key>` is present in the target resource's `output` list — in `internal/planner/resolve.go` (makes T015 pass)
- [x] T017 [US2] Add the US2 scenario to `tests/integration/deploy_resource_env_output_test.go`: an app referencing an un-exported resource env var fails `shrine deploy` with the allowlist error; the exported key still resolves (depends on T013)

**Checkpoint**: US1 and US2 both work; the export contract is enforced.

---

## Phase 5: User Story 3 - Migrate an old manifest with a clear error (Priority: P3)

**Goal**: A pre-split manifest that declares `value`/`valueFrom`/`generated` on
an `output` is rejected with an actionable migration error.

**Independent Test**: Validate a manifest with `generated: true` on an output;
the error names the resource + output and directs the operator to `env`.

### Tests for User Story 3 (write first, must fail)

- [x] T018 [US3] Write failing unit tests for old-style output rejection (`value`, `valueFrom`, or `generated` on any output → error naming the resource and output, pointing to `spec.env`) in `internal/manifest/validate_test.go`

### Implementation for User Story 3

- [x] T019 [US3] Implement the old-style output rejection in `validateResourceSpec` in `internal/manifest/validate.go` (makes T018 pass)
- [x] T020 [US3] Add the US3 scenario to `tests/integration/deploy_resource_env_output_test.go`: deploying an old-shape manifest exits non-zero with the migration error text; a migrated manifest deploys cleanly (depends on T013)

**Checkpoint**: All three user stories independently functional.

---

## Phase 6: FR-014 - Resources as first-class consumers

**Purpose**: Implement the confirmed clarification (Q1 = full symmetry): a
resource's `env` `valueFrom` may reference other manifests' exported outputs,
subject to the same access/reachability/ordering/inference rules as apps. Builds
on Foundational; independent of US1–US3.

- [x] T021 Write failing unit tests for `Order` with resource dependencies (resource→resource edge ordered after its target; resource↔resource cycle reported) in `internal/planner/order_test.go`
- [x] T022 Update `Order` to contribute outgoing edges from `res.Spec.Dependencies` (resources are no longer hardcoded leaves) in `internal/planner/order.go` (makes T021 pass)
- [x] T023 Write failing unit tests for enrichment scanning resource consumers (same-team inferred edge; cross-team/absent fail-fast; dedup vs explicit `spec.dependencies`) in `internal/planner/enrich_valuefrom_test.go`
- [x] T024 Generalize `applyEnrichmentRule` to iterate Resource consumers (sorted-by-name) and add `cloneResourceWithDeps` mirroring `cloneApplicationWithDeps` in `internal/planner/enrich_valuefrom.go` and `internal/planner/enrich.go` (makes T023 pass)
- [x] T025 Replace `validateResourceVaultOutputs` with resource-`env` `valueFrom` validation (vault refs + cross-manifest refs with allowlist) and apply access/reachability checks to resource consumers in `internal/planner/resolve.go`, with tests in `internal/planner/resolve_test.go`
- [x] T026 Update `Engine.ExecuteDeploy` to resolve resources in the topological `steps` order, passing `deps` so a resource reads an earlier resource's `Exports`, in `internal/engine/engine.go`
- [x] T027 Add the FR-014 scenario to `tests/integration/deploy_resource_env_output_test.go`: a resource consuming another same-team resource's exported `host` deploys in the correct order with the resolved value (depends on T013)

**Checkpoint**: Resources can consume other manifests' exports; ordering and
inference hold uniformly.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [x] T028 [P] Update the generate-resource skeleton to scaffold `env` and `output` blocks in `internal/handler/resources.go`
- [x] T029 [P] Update the dry-run rendering to distinguish the resource's container environment (`Env`) from its published interface (`Exports`) in `internal/handler/dryrun.go` (and `internal/handler/deploy_plan_format.go` if it lists resource values)
- [x] T030 [P] Update the resource manifest reference (env/output split + migration guide) under `docs/` and sync `AGENTS.md`
- [x] T031 Mark the `021` acceptance criteria complete in `specs/progress.md`
- [x] T032 Run `graphify update .` to refresh the knowledge graph after code changes
- [x] T033 Run the quickstart validation gate: `go build -o shrine .`, `go test ./...`, then `go test -tags integration ./tests/integration/...` (Principle V)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (P1)**: no dependencies.
- **Foundational (P2)**: depends on Setup; **blocks all stories and FR-014**. Within P2: T003 before T004/T005/T006; T005 before T007; T007 before T008.
- **US1 (P3)**: depends on Foundational. MVP.
- **US2 (P4)**: depends on Foundational. Independent of US1 (shares only the integration test file via T013).
- **US3 (P5)**: depends on Foundational. Independent of US1/US2.
- **FR-014 (P6)**: depends on Foundational. Independent of US1–US3.
- **Polish (P7)**: depends on all desired phases.

### Shared-file ordering (NOT parallel)

- `internal/manifest/validate_test.go`: T009 → T018 (sequential).
- `internal/manifest/validate.go`: T010 → T019 (sequential).
- `tests/integration/deploy_resource_env_output_test.go`: T013 → T017 → T020 → T027 (sequential; T013 must exist first).
- `internal/engine/engine.go`: T007 → T026 (sequential).
- `internal/planner/resolve.go`: T016 → T025 (sequential).

### Parallel Opportunities

- Foundational: T002 ∥ (T004 after T003) ∥ T006.
- After Foundational, US1 / US2 / US3 / FR-014 can be staffed in parallel by different developers, coordinating only on the shared integration test file (T013 first).
- Polish: T028 ∥ T029 ∥ T030.

---

## Parallel Example: Foundational

```bash
# T002 (types) is independent; after T003 lands the interface,
# T004 (resolver tests) and T006 (dry-run resolver) run in parallel:
Task: "T002 Add Env/Generated/Output doc in internal/manifest/types.go"
Task: "T004 Failing LiveResolver tests in internal/resolver/resolver_test.go"
Task: "T006 DryRunResolver new signature in internal/resolver/dry_run_resolver.go"
```

## Parallel Example: post-Foundational stories

```bash
# Different developers, after Phase 2 checkpoint:
Dev A: US1 (T009→T010, T011→T012, T013→T014)
Dev B: US2 (T015→T016, then T017 on the shared test file)
Dev C: US3 (T018→T019, then T020 on the shared test file)
Dev D: FR-014 (T021→T022, T023→T024, T025, T026, then T027)
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1 Setup → Phase 2 Foundational (build green).
2. Phase 3 US1.
3. **STOP and VALIDATE**: deploy a resource with `env` + curated `output`; confirm container env and exported interface.

### Incremental Delivery

Foundational → US1 (MVP) → US2 (enforce privacy) → US3 (migration safety) →
FR-014 (resource consumers) → Polish. Each phase is an independently testable
increment.

---

## Notes

- **Unit tests**: no filesystem access — use `t.Setenv` and in-memory fakes
  (project rule). Keep secret-store/vault interactions behind the existing fakes.
- **Integration tests**: slow (~10 min, real Docker). Iterate with
  `go test ./...`; run the integration gate only at phase checkpoints and as the
  final gate (T033).
- **Migration invariant**: keeping the same variable name when moving a
  `generated` output to `env` preserves the `<resource>.<name>` secret key — no
  rotation (research.md Decision 1; SC-005).
- Commit after each task or logical group. Constitution Check applies to any PR
  touching `internal/engine/`, `internal/manifest/`, or `cmd/`.
