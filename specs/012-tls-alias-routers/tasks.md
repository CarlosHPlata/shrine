---

description: "Task list for tls-alias-routers"
---

# Tasks: Per-Alias TLS Opt-In for Routing Aliases

**Input**: Design documents from `/specs/012-tls-alias-routers/`
**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/observer-events.md](./contracts/observer-events.md), [quickstart.md](./quickstart.md)

**Tests**: REQUIRED — Constitution Principle V mandates that integration tests are written before the implementation they cover. Unit tests follow the project's strict no-filesystem rule (use the existing `writeFileFn` / `mkdirAllFn` / `readFileFn` injection points and in-memory YAML bytes only).

**Organization**: Tasks are grouped by user story so each story can be implemented and verified independently. All three stories are P1 — together they form the feature, but each is independently testable. US1 delivers the headline capability (single alias over HTTPS); US2 verifies the realistic mixed-TLS deployment shape; US3 is the byte-identical regression guard for the existing fleet.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel — different files OR different test functions in the same file with no dependency on an incomplete task
- **[Story]**: Which user story this task belongs to (US1, US2, US3)
- File paths in every task are repo-root-relative

## Path Conventions

- Manifest schema: `internal/manifest/types.go` + `internal/manifest/parser_test.go` + `internal/manifest/validate_test.go`
- Engine alias projection: `internal/engine/backends.go` + `internal/engine/engine.go` + `internal/engine/engine_aliases_test.go`
- Plugin code: `internal/plugins/gateway/traefik/` (modify in place)
- Terminal logger: `internal/ui/terminal_logger.go`
- Unit tests: same package as the code under test; no filesystem touches; reuse the existing `writeFileFn` / `readFileFn` injection points and the `engine.NoopObserver` / recording-observer scaffolding already in `routing_test.go`
- Integration tests: `tests/integration/traefik_plugin_test.go` (build tag `integration`; uses `NewDockerSuite`)
- Integration fixtures: `tests/integration/fixtures/traefik-alias-tls*/` (mirror existing `traefik-alias-host-only/` layout)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Establish a known-green baseline before any code change.

- [X] T001 Confirm baseline `go build ./...` and `go test ./...` succeed on branch `012-tls-alias-routers` with no pre-existing failures, and confirm the latest `make test-integration` run on `main` passed (snapshot the SHA for comparison if a fresh local run is too slow per the user's auto-memory rule on integration-test cost). Record the snapshot SHA in the PR description. — Verified `go build ./...` clean before any edit; integration suite delegated to CI per user instruction.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Land the manifest field, the engine-side projection field, and the YAML-shape primitives. Every story's tests reference these; no story-level test compiles until these land.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [X] T002 [P] Add the `TLS bool \`yaml:"tls,omitempty"\`` field to `RoutingAlias` in `internal/manifest/types.go`, placed immediately after the existing `StripPrefix *bool` field. Plain `bool`, not `*bool` — the field has only two semantic states and `tls: false` is required to behave identically to omission per the spec's edge-case section ([data-model.md §1](./data-model.md), [research.md Decision 1](./research.md)). Do NOT add a field on `Routing`; FR-005's "tls only valid inside an alias entry" is enforced structurally by `yaml.Unmarshal` because the field exists only on `RoutingAlias`.
- [X] T003 Add the `TLS bool` field to `engine.AliasRoute` in `internal/engine/backends.go`, placed immediately after the existing `StripPrefix bool` field. Then extend `resolveAliasRoutes` in `internal/engine/engine.go` (around line 322) to populate the new field as a straight pass-through: `TLS: alias.TLS`. Do NOT change `WriteRouteOp` — its `AdditionalRoutes []AliasRoute` carries the new field for free. T003 reads the new manifest field, so it sequences after T002.
- [X] T004 [P] Add the new YAML-shape primitives to `internal/plugins/gateway/traefik/spec.go`: (a) define `type tlsBlock struct{}` immediately after the existing `loadBalancer` / `server` types; (b) add `TLS *tlsBlock \`yaml:"tls,omitempty"\`` as the last field of the existing `router` struct. Verify by `go build ./...` that the package compiles. The pointer + `omitempty` ensures FR-009's byte-identical regression behaviour for routers without TLS — `nil` produces no `tls:` key in the marshalled output.

**Checkpoint**: Foundation ready — every user story's tests can now compile against the new manifest field, the new engine field, and the new router shape.

---

## Phase 3: User Story 1 - Operator publishes a single alias over HTTPS via the manifest (Priority: P1) 🎯 MVP

**Goal**: Adding a single `tls: true` line to an existing `routing.aliases[]` entry results in (a) the generated alias router declaring `entryPoints: [web, websecure]` and carrying `tls: {}`, while (b) the primary-domain router stays HTTP-only, and (c) when the active static Traefik config has no `websecure` entrypoint, a `gateway.alias.tls_no_websecure` warning fires (deploy still succeeds — the alias router is still written so operators can land both edits in any order). Manifest validation rejects non-boolean values and rejects `tls` declared outside an alias entry.

**Independent Test**: Author a manifest with `routing.domain` plus one alias setting `tls: true`. With the Traefik gateway plugin active (and any working `websecure` wiring — via spec 011's `tlsPort` or a preserved `traefik.yml`), run `shrine deploy`. The generated `<routing-dir>/dynamic/<team>-<service>.yml` contains an alias router with `entryPoints: [web, websecure]` and `tls: {}`; the primary-domain router contains `entryPoints: [web]` and no `tls` field. The deploy log includes `↳ Aliases: <host>+<path> (tls)`. A non-boolean `tls` value or a top-level `routing.tls` key fails parse with a path-bearing error and no dynamic config is written.

### Tests for User Story 1 ⚠️ Write FIRST and confirm they FAIL before T017–T021

- [X] T005 [P] [US1] Manifest parser test `TestParseApplication_RoutingAlias_TLS_RoundTrip` in `internal/manifest/parser_test.go` — three sub-cases: (a) YAML with `tls: true` round-trips to `RoutingAlias.TLS == true`; (b) YAML with `tls: false` round-trips to `RoutingAlias.TLS == false`; (c) YAML with the `tls` field omitted entirely round-trips to `RoutingAlias.TLS == false`. All three must produce no error from `Parse(...)`.
- [X] T006 [P] [US1] Manifest parser test `TestParseApplication_RoutingAlias_TLS_RejectsNonBoolean` in `internal/manifest/parser_test.go` — feed YAML where one alias's `tls` field is the string `"yes"` (and a sibling sub-case for an integer `1`); assert `Parse(...)` returns a non-nil error whose message contains the alias path (e.g., `routing.aliases[1].tls`) and indicates a boolean type was expected. Covers FR-004.
- [X] T007 [P] [US1] Manifest parser test `TestParseApplication_Routing_TLS_RejectsTopLevelField` in `internal/manifest/parser_test.go` — feed YAML where `tls: true` is declared at `spec.routing.tls` (sibling to `domain`, not inside an `aliases[]` entry). Assert `Parse(...)` returns a non-nil error containing `field tls not found` and `manifest.Routing` (or substring `Routing`). Covers FR-005.
- [X] T008 [P] [US1] Manifest validate test `TestValidateRoutingAliases_TLS_DoesNotAffectCollisionKey` in `internal/manifest/validate_test.go` — two sub-cases: (a) two aliases with the same `host`+`pathPrefix` but one has `tls: true` and the other does not — must still be rejected as a duplicate (the `tls` flip is not a uniqueness key per FR-006); (b) two aliases with different `host`s but both with `tls: true` — must validate clean. Reuse the existing `validateRoutingAliases` test scaffolding in the file.
- [X] T009 [P] [US1] Engine test `TestResolveAliasRoutes_CarriesTLS` in `internal/engine/engine_aliases_test.go` — feed a `[]manifest.RoutingAlias` containing one entry with `TLS: true` and one with `TLS: false`; assert `resolveAliasRoutes(...)` returns a `[]AliasRoute` whose `TLS` fields match the inputs index-for-index. Covers the engine projection of FR-001.
- [X] T010 [P] [US1] Engine test `TestFormatAliasesForLog_AppendsTLSMarker` in `internal/engine/engine_aliases_test.go` — table-driven cases mirroring the existing `(no strip)` precedent: (a) `{Host:"a.example.com", TLS:true}` → `"a.example.com (tls)"`; (b) `{Host:"a.example.com", PathPrefix:"/x", TLS:true}` (default strip true) → `"a.example.com+/x (tls)"`; (c) `{Host:"a.example.com", PathPrefix:"/x", StripPrefix:false, TLS:true}` → `"a.example.com+/x (no strip) (tls)"`; (d) two aliases mixed — assert alphabetic sort puts the `(tls)`-marked entry in the right position. Covers FR-010.
- [X] T011 [P] [US1] Traefik routing test `TestWriteRoute_AliasWithTLS_AddsWebsecureAndTLSBlock` in `internal/plugins/gateway/traefik/routing_test.go` — extend the existing `baseOp()` helper (or write inline) with an `engine.AliasRoute{Host:"a.example.com", TLS:true}` in `AdditionalRoutes`. Stub `writeFileFn` to capture the bytes written. Stub `readFileFn` to return YAML containing a `websecure` entrypoint (so no warning fires). Assert the captured bytes contain (i) the alias router with `entryPoints:` block listing both `web` and `websecure` AND a `tls: {}` line; (ii) the primary-domain router still has `entryPoints:` listing only `web` and NO `tls` line. Covers FR-002, FR-003 (primary-domain invariant).
- [X] T012 [P] [US1] Traefik routing test `TestWriteRoute_AliasWithTLS_PreservesPrimaryRouterShape` in `internal/plugins/gateway/traefik/routing_test.go` — capture the bytes for an op with one `TLS: true` alias and one `TLS: false` alias; assert the primary-domain router YAML block (lookup by router name `<team>-<svc>`) is byte-identical to the byte block produced by `TestWriteRoute_OneAlias_NoStrip`'s primary-domain router for the same `Domain`/`Team`/`ServiceName` inputs. This is the surgical regression guard for "primary-domain router MUST remain unchanged regardless of any alias's `tls` value" (FR-003). Stub `readFileFn` to return YAML with `websecure` so warning emission is silent.
- [X] T013 [P] [US1] Traefik routing test `TestWriteRoute_AliasWithTLS_EmitsWarning_WhenStaticConfigLacksWebsecure` in `internal/plugins/gateway/traefik/routing_test.go` — construct a `RoutingBackend{routingDir:"/fake", staticConfigPath:"/fake/traefik.yml", observer: rec}` (recording observer); stub `readFileFn` to return YAML with only a `web` entrypoint (no `websecure`); call `WriteRoute` with `AdditionalRoutes` containing two TLS-enabled aliases. Assert exactly one event with `Name == "gateway.alias.tls_no_websecure"`, `Status == engine.StatusWarning`, `Fields["team"] == op.Team`, `Fields["name"] == op.ServiceName`, `Fields["path"] == "/fake/traefik.yml"`, `Fields["tls_aliases"]` is a comma-separated alphabetically-sorted list of the two TLS aliases formatted as `host[+pathPrefix]`, and `Fields["hint"]` contains the substring `websecure`. Covers FR-007.
- [X] T014 [P] [US1] Traefik routing test `TestWriteRoute_AliasWithTLS_NoWarning_WhenWebsecurePresent` in `internal/plugins/gateway/traefik/routing_test.go` — same fixture as T013 but stub `readFileFn` to return YAML containing `entryPoints.websecure`; assert the recorder captures zero `gateway.alias.tls_no_websecure` events. The router-shape side of the assertion is already covered by T011; this test is purely the warning-suppression branch.
- [X] T015 [P] [US1] Traefik routing test `TestWriteRoute_NoAliasHasTLS_NoWarning_RegardlessOfStaticConfig` in `internal/plugins/gateway/traefik/routing_test.go` — call `WriteRoute` with `AdditionalRoutes` containing only `TLS: false` (or omitted) aliases; stub `readFileFn` to return YAML lacking `websecure` (i.e., the warning's preconditions are otherwise met except no alias requested TLS). Assert zero `gateway.alias.tls_no_websecure` events. Guards against the warning firing spuriously when the operator has not opted in.
- [~] T016 [US1] (DEFERRED TO CI) Create integration fixture `tests/integration/fixtures/traefik-alias-tls/{application.yml, team.yml}` mirroring the existing `traefik-alias-prefix/` layout. The application manifest declares `metadata.name: whoami-tls`, `spec.image: traefik/whoami:v1.10.1`, `spec.port: 80`, `spec.routing.domain: primary.shrine.lab`, `spec.routing.aliases: [{host: alias.shrine.lab, pathPrefix: /tls, stripPrefix: false, tls: true}]`, and `spec.networking.exposeToPlatform: true`. The team manifest reuses the existing `aliasTestTeam` value (`shrine-alias-test`) so `BeforeEach`/`AfterEach` cleanup paths in the existing alias scenarios cover this fixture too.
- [~] T017 [US1] (DEFERRED TO CI) Integration test scenario `should publish alias router with tls block when alias sets tls: true` in `tests/integration/traefik_plugin_test.go` (alongside existing US-006 alias scenarios). Apply the team and application fixtures from T016 with the Traefik plugin active and `tlsPort: 8444` configured (so `websecure` is wired and no warning fires). Read `<routing-dir>/dynamic/shrine-alias-test-whoami-tls.yml`; parse to `map[string]any`; assert: (a) the alias router (`shrine-alias-test-whoami-tls-alias-0`) has `entryPoints` equal to the slice `[web, websecure]` AND has a key `tls` whose value is an empty map; (b) the primary-domain router (`shrine-alias-test-whoami-tls`) has `entryPoints` equal to `[web]` and has NO `tls` key. Assert the deploy stdout contains the substring `(tls)` (FR-010 log marker) by `tc.AssertOutputContains("(tls)")`.

### Implementation for User Story 1

- [X] T018 [US1] Extend `formatAliasesForLog` in `internal/engine/engine.go` (around line 331) to append `" (tls)"` to each alias's log string when `r.TLS == true`. Order is fixed: append `(no strip)` first (existing behaviour, preserves current diffs), then `(tls)` second. The function continues to alphabetically sort the joined entry list. One-line `if r.TLS { entry += " (tls)" }` immediately after the existing `if r.PathPrefix != "" && !r.StripPrefix` block. Covers FR-010.
- [X] T019 [US1] Add a `staticConfigPath string` field to `RoutingBackend` in `internal/plugins/gateway/traefik/routing.go` (place after `routingDir`, before `observer`). Update the constructor in `internal/plugins/gateway/traefik/plugin.go` (around line 184) — `Plugin.RoutingBackend()` — to pass `staticConfigPath: filepath.Join(routingDir, "traefik.yml")`. Update every call-site of `&RoutingBackend{...}` in `routing_test.go` to thread the new field (use `"/fake/traefik.yml"` for the existing tests where the value does not matter).
- [X] T020 [US1] Extend the alias-loop body inside `WriteRoute` in `internal/plugins/gateway/traefik/routing.go` (the loop currently spans lines 82–95). Inside the loop where the alias router `r` is constructed, add — after the existing strip-middleware branch — a TLS branch: `if ar.TLS { r.EntryPoints = []string{"web", "websecure"}; r.TLS = &tlsBlock{} }`. The order matters: keep the strip-middleware block first (it touches `r.Middlewares`), then the TLS block (it touches `r.EntryPoints` + `r.TLS`). Verify via `go test ./internal/plugins/gateway/traefik/...` that T011 and T012 pass.
- [X] T021 [US1] Add the `emitAliasTLSNoWebsecureSignal(op engine.WriteRouteOp, staticConfigPath string, observer engine.Observer)` helper in `internal/plugins/gateway/traefik/routing.go`, modelled on spec 011's `emitTLSPortNoWebsecureSignal`. Early-return when `op.AdditionalRoutes` contains zero entries with `TLS == true`. Otherwise call the existing `hasWebsecureEntrypoint(staticConfigPath)` (already defined in `config_gen.go`); on probe error reuse spec 011's `gateway.config.tls_port_probe_error` event verbatim ([contracts/observer-events.md "Non-events"](./contracts/observer-events.md)) and return; on `(true, nil)` return silently; on `(false, nil)` build the alphabetically-sorted `tls_aliases` field by collecting `formatAliasEntry(ar)` (extract a small helper if not already present — produces `host[+pathPrefix]`) for every TLS-enabled alias and emitting `engine.Event{Name:"gateway.alias.tls_no_websecure", Status:engine.StatusWarning, Fields: {"team":op.Team, "name":op.ServiceName, "path":staticConfigPath, "tls_aliases":joined, "hint":"Set plugins.gateway.traefik.tlsPort to publish a websecure entrypoint, or add the entrypoint to your preserved traefik.yml."}}`. Wire the call into `WriteRoute` at the very top of the function (immediately after the existing `mkdirAllFn` call and before the `path := filepath.Join(...)` line, OR after `path` is computed but BEFORE the `present` check returns) so the warning fires on EVERY `WriteRoute` invocation regardless of dynamic-file preservation state. The decision is purely a function of (a) manifest intent (`op.AdditionalRoutes` TLS flags) and (b) static config state (the websecure probe) — both observable independently of the per-app dynamic file. The warning fires on the preserved branch too, because operators who set `tls: true` in the manifest deserve the same nudge even if Shrine isn't currently rewriting their preserved per-app dynamic file (they may have copied the TLS shape into the preserved file or may not have — Shrine cannot tell, but the static-config mismatch is independently a problem worth flagging).
- [X] T022 [US1] Add a `case "gateway.alias.tls_no_websecure":` clause to `internal/ui/terminal_logger.go` immediately after the existing `gateway.config.tls_port_no_websecure` case. Render as `fmt.Fprintf(t.out, "  ⚠️  alias tls: true but websecure entrypoint missing in %s for %s.%s (%s) — %s\n", e.Fields["path"], e.Fields["team"], e.Fields["name"], e.Fields["tls_aliases"], e.Fields["hint"])`. Match the indentation and `⚠️` glyph already used by sibling warnings.

**Checkpoint**: User Story 1 is complete and shippable as MVP. T005–T017 must all pass. The headline issue from GitHub #13 is resolved end-to-end: `tls: true` on an alias produces the expected router shape and the manifest is the source of truth.

---

## Phase 4: User Story 2 - Mixed TLS-on / TLS-off aliases on the same application (Priority: P1)

**Goal**: An application can declare two or more aliases where some set `tls: true` and others do not. The generated dynamic config contains the right per-alias router shape for each entry, and the primary-domain router remains HTTP-only regardless of alias mix. Removing `tls: true` from an alias and re-deploying restores the alias router to HTTP-only.

**Independent Test**: Apply a manifest with `routing.domain` and two aliases — first omits `tls`, second sets `tls: true`. After `shrine deploy`, the generated dynamic config has exactly two alias routers: the first with `entryPoints: [web]` and no `tls` key, the second with `entryPoints: [web, websecure]` and `tls: {}`. Both alias routers `service:` field points to the same backend service as the primary-domain router. Edit the manifest to remove `tls: true` from the second alias; re-deploy; confirm both alias routers now declare `entryPoints: [web]` and contain no `tls` block.

### Tests for User Story 2 ⚠️ Write FIRST

- [~] T023 [US2] (DEFERRED TO CI) Create integration fixture `tests/integration/fixtures/traefik-alias-tls-mixed/{application.yml, team.yml}` mirroring the T016 layout. Application name `whoami-tls-mixed`; aliases list contains TWO entries: (i) `{host: lan.shrine.lab, pathPrefix: /lan}` (no `tls` field); (ii) `{host: ext.shrine.lab, pathPrefix: /ext, stripPrefix: false, tls: true}`. Reuse `aliasTestTeam`.
- [~] T024 [US2] (DEFERRED TO CI) Integration test scenario `should produce mixed TLS-on / TLS-off router shapes when only some aliases set tls` in `tests/integration/traefik_plugin_test.go`. Apply T023's fixtures with `tlsPort: 8445` configured. Read `<routing-dir>/dynamic/shrine-alias-test-whoami-tls-mixed.yml`; parse to `map[string]any`; assert: (a) router `shrine-alias-test-whoami-tls-mixed-alias-0` (the lan alias) has `entryPoints == [web]` and NO `tls` key; (b) router `shrine-alias-test-whoami-tls-mixed-alias-1` (the ext alias) has `entryPoints == [web, websecure]` and a `tls` key whose value is the empty map; (c) both aliases' `service` field equals the primary-domain router's `service` field, proving they all share the same backend. Then rewrite the application manifest with the second alias's `tls: true` removed; re-deploy; re-read the dynamic file; assert alias-1 now has `entryPoints == [web]` and NO `tls` key (the alias router reverts cleanly).

### Implementation for User Story 2

User Story 2 introduces no new production code. The conditional added in T020 (`if ar.TLS { r.EntryPoints = []string{"web", "websecure"}; r.TLS = &tlsBlock{} }`) is per-alias, so a mix of `TLS: true` and `TLS: false` aliases automatically produces the differentiated router shapes covered by T024. If T024 fails, the bug is in the US1 conditional branch, not in new US2 code.

**Checkpoint**: User Story 2 verified — operators can declare per-alias TLS independently and the manifest is the source of truth across re-deploys. T023–T024 must pass.

---

## Phase 5: User Story 3 - Existing manifests without `tls` keep working unchanged (Priority: P1)

**Goal**: Manifests that do NOT use `tls` on any alias produce dynamic config that is byte-identical (modulo unrelated, already-shipped changes) to the file generated by the previous Shrine release for the same input. No new entrypoints, no `tls` blocks, no log noise on the no-TLS path.

**Independent Test**: Apply a manifest with one or more aliases, none of which set `tls`. Capture the generated dynamic config bytes. Re-deploy the same manifest a second time. Assert the file's bytes are unchanged. Inspect the file: it contains no occurrence of the strings `"websecure"` or `"tls:"`. Repeat with a manifest that has no `routing.aliases` field at all — same byte-stable result, no `(tls)` marker in the deploy log, no `gateway.alias.tls_no_websecure` event.

### Tests for User Story 3 ⚠️ Write FIRST

- [X] T025 [US3] Extend the existing `TestWriteRoute_NoAliases`, `TestWriteRoute_OneAlias_Strip`, `TestWriteRoute_OneAlias_NoStrip`, `TestWriteRoute_HostOnlyAlias`, and `TestWriteRoute_ThreeAliases_SparseStrip` in `internal/plugins/gateway/traefik/routing_test.go` to assert that the captured `writeFileFn` bytes do NOT contain the substrings `"websecure"` or `"tls:"`. The assertion is one line per test added at the end of the existing assertion block. Stub `readFileFn` (where not already stubbed) to return YAML lacking `websecure` so the new warning would fire IF any alias had `TLS: true` — proving by absence that no warning is emitted when no alias opts in. Collectively this is the FR-009 byte-identical regression guard at the unit-test layer.
- [~] T026 [US3] (DEFERRED TO CI) Integration test scenario `should produce byte-stable dynamic config and emit no tls warnings when no alias sets tls` in `tests/integration/traefik_plugin_test.go`. Reuse the existing `traefik-alias-prefix` fixture (one alias with `pathPrefix`, no `tls`). Deploy with `tlsPort` UNSET (so the static config has no `websecure` either — the worst-case configuration for accidental warning emission). Read `<routing-dir>/dynamic/shrine-alias-test-whoami-prefix.yml`; capture the bytes. Re-deploy with the identical manifest; re-read the file; assert `bytes.Equal(first, second)`. Inspect the captured bytes: assert they do NOT contain `"websecure"` or `"tls:"`. Inspect the deploy stdout via `tc.AssertOutputContains` / `tc.AssertOutputDoesNotContain` (or equivalent): assert the substring `(tls)` is absent and the warning rendering `"alias tls: true but websecure entrypoint missing"` is absent. Covers FR-009 + FR-010 (no log noise) + the structural guarantee from FR-007 (warning is opt-in via at least one `tls: true`).

### Implementation for User Story 3

User Story 3 introduces no new production code. The byte-identical regression behaviour falls out of (a) the `omitempty` YAML tag on `RoutingAlias.TLS` (T002), `engine.AliasRoute.TLS` carrying false through (T003), the `*tlsBlock` pointer on `router.TLS` being `nil` for non-TLS aliases (T004), and the warning early-return when no alias has `TLS == true` (T021). T025 + T026 are the regression guards that lock these properties in. If either fails, the bug is in the foundational layer or the US1 conditional, not in new US3 code.

**Checkpoint**: All three user stories are independently verified end-to-end. T025–T026 must pass.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T027 [P] Update `AGENTS.md` to add the new `tls` field to the `routing.aliases` example in the Traefik plugin section. Place it as a sibling of `host`, `pathPrefix`, `stripPrefix`, with a one-line trailing comment matching the existing alignment style. Add one short prose sentence: "`tls: true` opens the routing/entrypoint side only; certificate provisioning, ACME, and HTTPS redirects remain operator-owned via standard Traefik mechanisms (mirrors spec 011's `tlsPort` contract)." Covers FR-011.
- [X] T028 [P] Cross-check that the existing alias integration scenarios in `tests/integration/traefik_plugin_test.go` (the `host-only`, `prefix`, `multi`, and any spec 008/009 scenarios) still pass byte-for-byte after the new `RoutingBackend.staticConfigPath` field lands. These scenarios do not declare `tls: true` on any alias and so MUST observe the FR-009 regression guard. If any pre-existing scenario is byte-equality-asserting on the dynamic config, verify the assertion still holds. Run `go test ./internal/plugins/gateway/traefik/...` to confirm the full unit suite stays green; flag any drift that requires a structural rewrite (likely none — the existing assertions use `AssertFileContains`, which is substring-based and unaffected).
- [~] T029 (DEFERRED INTEGRATION TO CI) Final regression gate: from a clean checkout of branch `012-tls-alias-routers`, run `go build ./...`, `go test ./...`, and `make test-integration` (or rely on the CI pipeline's integration run per the user's auto-memory rule on integration-test cost). All three must pass green; no flakes accepted on this branch. Manual quickstart sanity ([quickstart.md](./quickstart.md)) is OPTIONAL — the integration scenarios T017, T024, and T026 collectively cover every step of the quickstart against a real Docker daemon, so the manual run is operational sanity rather than a merge gate. Record any drift in a follow-up commit before merging.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories. T002 (manifest field) blocks T003 (engine pass-through reads it). T004 (traefik spec primitives) is independent of T002+T003 and may land in parallel.
- **User Story 1 (Phase 3, P1 MVP)**: Depends on Phase 2 completing T002 + T003 + T004. Internally: T005–T015 (unit tests) can be authored in parallel; T016 (fixture) blocks T017 (integration test); T018–T022 (implementation) make all these tests pass. The implementation order T018 → T019 → T020 → T021 → T022 is loosely sequential — T020 depends on T004 (router struct) plus T019 (RoutingBackend field), T021 depends on T020 (the loop it observes), T022 depends on the event name fixed by T021.
- **User Story 2 (Phase 4, P1)**: Depends on Phase 2 + US1 implementation (T020 + T021) since US2 has no new production code — its tests assert per-alias differentiated behaviour of the conditionals added in US1.
- **User Story 3 (Phase 5, P1)**: Depends on Phase 2 + US1 implementation (specifically T020's `if ar.TLS` branch, whose `else` path is what US3 verifies) since US3 has no new production code — its tests assert the no-`tls`-anywhere regression behaviour. T025 can be authored against the unit harness as soon as T011 lands; T026 needs the full US1 implementation chain green.
- **Polish (Phase 6)**: Depends on US1 + US2 + US3 complete. T027 and T028 are independent — parallel. T029 is the gating final check.

### Within Each User Story

- Tests come before implementation (TDD per Constitution Principle V; the user's auto-memory rule that integration tests are slow makes "write once, run sparingly" the default).
- Manifest tests (T005–T008) are independent of engine tests (T009–T010) and traefik routing tests (T011–T015); all eleven can be authored together.
- The integration fixture T016 must land before the integration test T017; same for T023 → T024.
- Implementation tasks T018–T022 are sequential within `internal/plugins/gateway/traefik/`: T019 (struct field) → T020 (loop body) → T021 (warning helper + wiring) → T022 (logger renderer). T018 (engine.go log marker) is independent and may land first or last.

### Parallel Opportunities

- T005 / T006 / T007 / T008 (manifest unit tests, two files): **safe to author together**.
- T009 / T010 (engine unit tests, single file but different test functions): **safe to author together**.
- T011 / T012 / T013 / T014 / T015 (traefik routing unit tests, single file but different test functions): **safe to author together**.
- T002 / T004 (foundational, different files, no dependency): **parallel**.
- T027 / T028 (polish): **parallel** — different surfaces.
- US1 and US2 fixture files (T016, T023): **parallel** — different directories.
- US3 unit-test extension T025 can be authored alongside the US1 unit tests since both touch `routing_test.go` but at different test-function scopes.

---

## Parallel Example: User Story 1 unit tests

```bash
# Author US1 unit tests together (different test functions, three files):
Task: "TestParseApplication_RoutingAlias_TLS_RoundTrip in parser_test.go"
Task: "TestParseApplication_RoutingAlias_TLS_RejectsNonBoolean in parser_test.go"
Task: "TestParseApplication_Routing_TLS_RejectsTopLevelField in parser_test.go"
Task: "TestValidateRoutingAliases_TLS_DoesNotAffectCollisionKey in validate_test.go"
Task: "TestResolveAliasRoutes_CarriesTLS in engine_aliases_test.go"
Task: "TestFormatAliasesForLog_AppendsTLSMarker in engine_aliases_test.go"
Task: "TestWriteRoute_AliasWithTLS_AddsWebsecureAndTLSBlock in routing_test.go"
Task: "TestWriteRoute_AliasWithTLS_PreservesPrimaryRouterShape in routing_test.go"
Task: "TestWriteRoute_AliasWithTLS_EmitsWarning_WhenStaticConfigLacksWebsecure in routing_test.go"
Task: "TestWriteRoute_AliasWithTLS_NoWarning_WhenWebsecurePresent in routing_test.go"
Task: "TestWriteRoute_NoAliasHasTLS_NoWarning_RegardlessOfStaticConfig in routing_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Complete Phase 1 (T001).
2. Complete Phase 2 (T002 + T004 in parallel → T003).
3. Complete Phase 3 (T005–T015 in parallel → T016 → T017 → T018–T022 sequentially).
4. **STOP and VALIDATE**: Run the US1 unit + integration tests; manually walk [quickstart.md](./quickstart.md) Steps 1–5 and confirm `tls: true` on a single alias produces a working alias router with `entryPoints: [web, websecure]` and `tls: {}`.
5. At this point, the headline ask from GitHub #13 is resolved and shippable on its own.

### Incremental Delivery

1. Foundation (T001 → T002 + T004 → T003).
2. US1 (MVP) → ship → close GitHub issue #13 with the per-alias HTTPS confirmation.
3. US2 → ship → mixed-deployment regression suite is in CI.
4. US3 → ship → byte-identical regression guard for the existing fleet is in CI.
5. Polish (Phase 6) → AGENTS.md docs, full regression sweep, final gate.

### Parallel Team Strategy

Single-developer feature; the constitution's "small, commit-able increments" rule applies. No parallel-team strategy needed. If two developers were available, the natural split is: Dev A owns Phases 2 + 3 (foundational + MVP); Dev B owns Phases 4 + 5 fixture/test authoring (T023, T026), picking up after Dev A merges T020+T021. Phase 6 polish is shared.

---

## Notes

- All tasks comply with the user's auto-memory rules: unit tests reuse the existing `writeFileFn` / `readFileFn` injection points and the `engine.NoopObserver` / recording-observer scaffolding already in `routing_test.go` (no filesystem touches); integration tests run sparingly via `make test-integration` against a real binary and a real Docker daemon (Constitution Principle V).
- The `tlsBlock struct{}` type is intentionally empty — its single purpose is to marshal to literal `tls: {}` in YAML 1.2 and to structurally guarantee that no `certResolver`, `domains`, or `options` keys can leak into the generated alias router. This satisfies FR-012 at the type-system level. Future extension (if Traefik ever requires per-route TLS configuration that Shrine should generate) would be an explicit addition to this struct, never an accidental one.
- The new event name `gateway.alias.tls_no_websecure` is the only new entry on the observer contract; reuse of spec 011's `gateway.config.tls_port_probe_error` for probe-failure I/O errors is intentional and documented in [contracts/observer-events.md "Non-events"](./contracts/observer-events.md).
- Manifest validation rejections at parse time mean callers in `cmd/` get an error before any container or filesystem mutation happens — covered structurally by `yaml.Unmarshal` rather than by new code in `validate.go`. T006 + T007 lock this in at the unit-test layer.
- Spec 009's per-app dynamic config preservation regime applies unchanged. When the file is operator-owned, `WriteRoute` early-returns at the `present` check (see `routing.go` lines 56–68) BEFORE building any router; the new TLS branch in T020 is in the build path, so preserved files remain untouched in response to manifest `tls` flips (FR-008). The new warning helper T021 is wired at the very top of `WriteRoute` (BEFORE the `present` check returns), so the warning still fires on preserved-file deploys when at least one alias requests TLS and the static config lacks `websecure` — operators get a useful nudge even when the file write itself is skipped.
