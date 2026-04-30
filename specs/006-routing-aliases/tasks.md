---
description: "Task list for feature 006-routing-aliases"
---

# Tasks: Routing Aliases for Application Manifests

**Input**: Design documents from `/specs/006-routing-aliases/`
**Prerequisites**: plan.md (✓), spec.md (✓), research.md (✓), data-model.md (✓), contracts/manifest-schema.md (✓)

**Tests**: Constitution V mandates an integration-test gate via `tests/integration/` and TDD ordering ("integration test files are created before the implementation code"). Test tasks are therefore included and ordered before the implementation they cover.

**Organization**: Tasks are grouped by user story so US1 (Traefik-active aliases — the MVP) can ship independently of US2 (plugin-inactive portability).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no incomplete dependencies)
- **[Story]**: User story label (US1, US2). Setup, Foundational, and Polish tasks have no story label.
- File paths in every implementation task are absolute-relative to repo root.

## Path Conventions

Single Go project, repo root = `/root/projects/shrine/`. Module path is `github.com/CarlosHPlata/shrine`. Source under `internal/`, tests under `tests/integration/` (real-binary, build-tagged) and adjacent `_test.go` files (unit). Test fixtures under `tests/testdata/`.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Add the YAML test fixtures that integration scenarios in both US1 and US2 will consume. Fixtures are pure data; they do not depend on any code change.

- [X] T001 [P] Create fixture `tests/testdata/hello-api-aliased.yml` — clone of `hello-api.yml` with `spec.routing.aliases` containing one entry (`host: gateway.tail9a6ddb.ts.net`, `pathPrefix: /finances`, `stripPrefix` omitted so default `true` applies).
- [X] T002 [P] Create fixture `tests/testdata/hello-api-noprefix-alias.yml` — clone of `hello-api.yml` with one alias having only `host: lan.home.lab` (no `pathPrefix`, no `stripPrefix`) for the host-only acceptance scenario.
- [X] T003 [P] Create fixture `tests/testdata/hello-api-multi-alias.yml` — clone of `hello-api.yml` with three aliases: one host-only, one host+pathPrefix (default strip), one host+pathPrefix+stripPrefix:false. Used for the multi-alias and no-strip scenarios.
- [X] T004 [P] Create fixture `tests/testdata/hello-api-collision.yml` — second Application manifest under a different `metadata.name` and `metadata.owner` whose `spec.routing.domain` is exactly the primary domain of `hello-api.yml` (collision target for FR-008a).

**Checkpoint**: Fixtures exist on disk. No Go code touched yet.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Manifest schema + validation that *both* user stories rely on. US1 needs aliases parsed and routed; US2 needs aliases parsed and silently inert. The schema, the YAML round-trip, and same-app validation rules (V1–V7) are therefore foundational.

**⚠️ CRITICAL**: No US1 or US2 task can begin until this phase is complete. Until `RoutingAlias` is a real Go type, every downstream package fails to compile.

- [X] T005 Add `RoutingAlias` struct (fields: `Host string`, `PathPrefix string`, `StripPrefix *bool`) and `Aliases []RoutingAlias` field on the `Routing` struct in `internal/manifest/types.go`. Match the YAML tags spelled out in `data-model.md` §1.
- [X] T006 Add helper `validateRoutingAliases(routing Routing) []string` in `internal/manifest/validate.go` implementing rules V1–V7 from `data-model.md` §2 (cross-reference table for error message shapes); call it from `validateApplicationSpec` so its messages join the existing multi-error report.
- [X] T007 [P] Extend `internal/manifest/parser_test.go` with a sub-test that loads `tests/testdata/hello-api-aliased.yml` and asserts `app.Spec.Routing.Aliases` contains exactly one entry with the expected `Host`, `PathPrefix`, and a non-nil `StripPrefix` pointing at `true` only when the YAML set it explicitly (here it should be nil — default-applied later by the engine).
- [X] T008 [P] Add a new `internal/manifest/validate_test.go` table-driven test (or extend the existing one) covering the negative cases for each of V1–V7, asserting the exact error substrings from `contracts/manifest-schema.md` ("Validation errors" table).

**Checkpoint**: `go test ./internal/manifest/...` passes. Manifest with `aliases` round-trips through parse + validate; bad alias manifests produce the documented multi-error reports. The codebase compiles. US1 and US2 can now branch in parallel.

---

## Phase 3: User Story 1 — Expose an Application Under Multiple Hostnames (Priority: P1) 🎯 MVP

**Goal**: With the Traefik gateway plugin active, a manifest with `routing.aliases` produces additional routers in the app's dynamic config file, all forwarding to the same backend service. Cross-app host+path collisions fail the deploy before any gateway file is written.

**Independent Test**: Deploy `tests/testdata/hello-api-aliased.yml` against a host with the Traefik plugin enabled. `curl http://hello-api.home.lab/health` and `curl http://gateway.tail9a6ddb.ts.net/finances/health` both reach the same backend container. Re-deploying after adding `tests/testdata/hello-api-collision.yml` to the manifest set fails with a clear error and leaves the gateway dynamic config unchanged.

### Tests for User Story 1 (TDD — write FIRST, ensure they FAIL before T015–T024)

> Per Constitution V, integration tests run the real `shrine` binary as a subprocess against a live Docker daemon via `NewDockerSuite`. New scenarios are added to the existing `tests/integration/traefik_plugin_test.go` (same file as the Traefik plugin's other end-to-end coverage) so they share the harness setup.

- [X] T009 [US1] `should publish alias router for a host-only alias` in `tests/integration/traefik_plugin_test.go`.
- [X] T010 [US1] `should publish alias router with default-strip middleware for path-prefixed alias` in `tests/integration/traefik_plugin_test.go`.
- [X] T011 [US1] `should publish multiple alias routers with sparse strip indexing` in `tests/integration/traefik_plugin_test.go`.
- [X] T012 [US1] `should fail deploy when two applications collide on host+pathPrefix` in `tests/integration/traefik_plugin_test.go`.
- [X] T013 [US1] `should reach backend through both primary and alias addresses` (config-level shared-service assertion; live `curl` skipped — no docker-exec primitive in harness) in `tests/integration/traefik_plugin_test.go`.
- [X] T014 [US1] `should include aliases field in routing.configure log signal` in `tests/integration/traefik_plugin_test.go`.
- [X] T014a [US1] `should drop alias routers when alias is removed and re-deployed` in `tests/integration/traefik_plugin_test.go`; uses `traefik-alias-removed/` fixture for the second deploy.

### Implementation for User Story 1

- [X] T015 [P] [US1] Add `stripPrefix` (struct: `Prefixes []string`) and a pointer field `StripPrefix *stripPrefix` on `middleware` in `internal/plugins/gateway/traefik/spec.go`. Match the YAML shape in `research.md` R1.
- [X] T016 [P] [US1] Add `AliasRoute` struct (fields: `Host string`, `PathPrefix string`, `StripPrefix bool`) and `AdditionalRoutes []AliasRoute` field on `WriteRouteOp` in `internal/engine/backends.go`.
- [X] T017 [US1] Add `resolveAliasRoutes(aliases []manifest.RoutingAlias) []engine.AliasRoute` in `internal/engine/engine.go` — applies the default-strip rule and trims the trailing `/` from `PathPrefix` for byte-stable rule output.
- [X] T018 [US1] Extend the `routing.configure` observer event in `internal/engine/engine.go` to include an `aliases` field — sorted, comma-joined `host[+pathPrefix]` strings — gated on both alias-list non-empty and `engine.Routing != nil`.
- [X] T019 [US1] Add helper `buildRouterRule(host, pathPrefix string) string` in `internal/plugins/gateway/traefik/routing.go`.
- [X] T020 [US1] Add helper `stripMiddlewareName(team, service string, aliasIndex int) string` in `internal/plugins/gateway/traefik/routing.go`.
- [X] T021 [US1] Rewrite `WriteRoute` in `internal/plugins/gateway/traefik/routing.go` to render N+1 routers, sparse strip middlewares, and one shared service per app.
- [X] T022 [P] [US1] Add new file `internal/planner/collisions.go` with `DetectRoutingCollisions(set *ManifestSet) error`.
- [X] T023 [US1] Wire `DetectRoutingCollisions` into the deploy pipeline in `internal/handler/deploy.go`, gated on `engine.Routing != nil`.
- [X] T024 [P] [US1] Add unit test file `internal/plugins/gateway/traefik/routing_test.go` (filesystem-free via `writeFileFn`/`mkdirAllFn` indirection).
- [X] T025 [P] [US1] Add unit tests for `resolveAliasRoutes` in `internal/engine/engine_aliases_test.go`.
- [X] T026 [P] [US1] Add unit tests for `DetectRoutingCollisions` in `internal/planner/collisions_test.go`.

**Checkpoint**: All US1 integration tests pass against a live Docker daemon. The MVP is shippable.

---

## Phase 4: User Story 2 — Aliases Are Inert When the Traefik Plugin Is Not Active (Priority: P2)

**Goal**: A manifest containing `routing.aliases` deploys cleanly on a host where no Traefik gateway plugin is enabled, with no errors, no warnings, and no alias-related side effects.

**Independent Test**: Deploy `tests/testdata/hello-api-aliased.yml` against a Shrine config that omits the `traefik:` plugin block. The deploy succeeds, no `<routingDir>/dynamic/` is created, and stdout/stderr contain no occurrence of the substrings "alias", "warning", or "ignored" — the alias is parsed and forgotten without comment.

> Most of US2 falls out for free from US1's design: `engine.Routing` is `nil` on a no-plugin host, the existing nil-check in `deployApplication` already skips routing-backend calls, and the collision check in T023 is gated on the same condition. US2's value is *evidence* of inertness via dedicated integration coverage — the no-op behavior must be tested, not assumed.

### Tests for User Story 2 (TDD)

- [X] T027 [US2] `should accept alias manifest silently when traefik plugin disabled` in `tests/integration/deploy_test.go`.
- [X] T028 [US2] `should run alias-bearing manifest on traefik-enabled and traefik-disabled hosts` in `tests/integration/traefik_plugin_test.go`.

### Implementation for User Story 2

- [X] T029 [US2] Verified: `engine.go:165` gates the entire routing block on `engine.Routing != nil`; `engine.go:172` adds the `aliases` field only inside that gate. Dual gate from F4 remediation is in place; no code change required.

**Checkpoint**: T027 and T028 pass. The same manifest deploys cleanly on both Traefik-active and Traefik-inactive hosts. SC-003 is verifiable.

---

## Phase 5: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, regression coverage, and final acceptance.

- [X] T030 [P] Updated `AGENTS.md` Application example to include the `aliases:` field with a per-line annotation pointing operators at the new behavior.
- [~] T031 Unit-test gate green (`go build`, `go vet`, `go vet -tags integration`, `go test ./...` all pass). Integration suite (`make test-integration`) intentionally NOT run — per user guidance, run once as the final acceptance gate to avoid the ~10-minute cost.
- [ ] T032 Manual walk-through of `quickstart.md` — DEFERRED to the operator with a real Traefik-enabled host. Cannot be performed in this implementation session.
- [~] T033 SKIPPED. `specs/progress.md` tracks the legacy build-phase model; the last four spec-kit features (002–005) did not update it either. Adding an aliases-specific entry would be inconsistent with established practice. No-op.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 Setup** (T001–T004): No code dependencies; can run immediately.
- **Phase 2 Foundational** (T005–T008): Depends only on Setup; **blocks** US1 and US2.
- **Phase 3 US1** (T009–T026): Depends on Foundational; independently testable end-to-end.
- **Phase 4 US2** (T027–T029): Depends on Foundational. Can run in parallel with US1 once Phase 2 is done; integration assertions assume the engine nil-routing gate already exists (it does, in current `engine.go`).
- **Phase 5 Polish** (T030–T033): Depends on US1 (and US2 if it ships in the same release).

### User Story Dependencies

- **US1 (P1)**: Depends on Phase 2. Owns the bulk of new code (engine widening, Traefik rendering, planner collision check).
- **US2 (P2)**: Depends on Phase 2. Largely a verification phase — the engine's existing `nil`-routing-backend short-circuit is what makes aliases inert; T029 confirms it covers the new alias-log field too.

### Within US1

Strict order:
1. Tests (T009–T014) — written first, must fail.
2. Spec + struct widenings (T015, T016) — make the code compile.
3. Engine resolver and event extension (T017, T018) — make the data flow.
4. Traefik renderer rewrite (T019–T021) — make the tests for routing pass.
5. Planner collision check + wiring (T022, T023) — make the collision test pass.
6. Unit tests (T024–T026) — pin down the helpers.

### Parallel Opportunities

- T001–T004 (fixtures): all four [P], different files.
- T007, T008 (manifest unit tests): both [P], different files.
- T009–T014 (integration test stubs): all [US1], same file `tests/integration/traefik_plugin_test.go` — write sequentially within the file but can be drafted in parallel as separate functions before any of them is run.
- T015 (Traefik spec.go) and T016 (engine backends.go): [P], different files.
- T022 (planner/collisions.go): [P] with T015/T016 — different package.
- T024 (Traefik routing_test.go), T025 (engine_aliases_test.go), T026 (planner/collisions_test.go): all [P] once their respective implementations are in.
- T030 and T031: [P], independent.

### Story-Boundary Parallelism

Once Phase 2 is done, US1 and US2 can be staffed independently. US2 has very little code (T029 may even be a no-op) — a single contributor can take both stories without losing throughput.

---

## Parallel Example: Phase 1 Setup

```bash
# All four fixtures are independent files — write in parallel:
Task: "Create tests/testdata/hello-api-aliased.yml"
Task: "Create tests/testdata/hello-api-noprefix-alias.yml"
Task: "Create tests/testdata/hello-api-multi-alias.yml"
Task: "Create tests/testdata/hello-api-collision.yml"
```

## Parallel Example: US1 Implementation Kickoff

```bash
# After T009–T014 are written and failing, three independent files can move in parallel:
Task: "Add stripPrefix middleware type in internal/plugins/gateway/traefik/spec.go"   # T015
Task: "Add AliasRoute and AdditionalRoutes in internal/engine/backends.go"           # T016
Task: "Add detectRoutingCollisions in internal/planner/collisions.go"                # T022
```

---

## Implementation Strategy

### MVP First (US1 only)

1. Phase 1 Setup — fixtures.
2. Phase 2 Foundational — manifest schema and validation.
3. Phase 3 US1 — engine widening, Traefik rendering, planner collision check.
4. **STOP and VALIDATE**: run the US1 integration tests; manually walk `quickstart.md` against a real Traefik-enabled host.
5. Ship.

### Incremental Delivery

- Setup + Foundational → manifest accepts `aliases` and validates them. (Not directly user-visible but unblocks both stories.)
- + US1 → operators can publish their apps at additional addresses via Traefik. **MVP shipped.**
- + US2 → the same manifest is portable to no-Traefik hosts. (Mostly a verification milestone; little new code.)
- + Polish → docs in sync, regression suite green, quickstart matches reality.

### Single-Contributor Strategy

Given US2 is a thin verification layer, a solo contributor can interleave T027/T028 alongside US1 work without context-switching cost. Recommended order: T001–T008 → T009–T014 → T015–T026 → T027–T029 → T030–T033.

---

## Format validation

All tasks above:
- Start with `- [ ]` checkbox.
- Have a sequential ID (T001–T033).
- Include `[P]` only when truly parallelizable (different files, no incomplete deps).
- Carry `[US1]` / `[US2]` for user-story phases; Setup/Foundational/Polish carry no story label.
- Name an exact file path in the description.
