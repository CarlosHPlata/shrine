# Research: Integration Coverage for TLS Alias Routers and the Routing Finalize Phase

**Date**: 2026-10-04 | **Spec**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md)

Planning input: *"check current implementations and ensure we will not break anything else"*. Part 1 records what the shipped code does today (read on `main` @ 64613a9). Part 2 records the decisions that follow. Part 3 is the regression-safety analysis.

## Part 1 — Current-state findings

### F1. TLS alias routing (spec 012) is implemented as specified

- `internal/plugins/gateway/traefik/routing.go:144-160` — every alias router starts as `entryPoints: [web]`; when `ar.TLS` is true it becomes `[web, websecure]` and gets `TLS: &tlsBlock{}`. The primary router is built separately (`routing.go:133-139`) and never receives TLS.
- `internal/plugins/gateway/traefik/spec.go:45-53` — `TLS *tlsBlock` has `yaml:"tls,omitempty"` and `tlsBlock` is an empty struct, so the file contains `tls: {}` only for opted-in aliases and no `tls` key otherwise.
- Router keys are `<team>-<app>` (primary) and `<team>-<app>-alias-<i>` (alias, manifest order). All routers share `service: <team>-<app>`.
- `internal/engine/engine.go:402-419` — `formatAliasesForLog` appends ` (tls)` per TLS alias; `internal/ui/terminal_logger.go:58-62` prints it on stdout as `    ↳ Aliases: <entries>`.
- `routing.go:62-96` — the `gateway.alias.tls_no_websecure` warning fires only when at least one alias has TLS **and** the static config lacks `websecure`; rendered at `terminal_logger.go:76-77` as `alias tls: true but websecure entrypoint missing`.

**Consequence**: US1–US3 are expected to pass against shipped code with no product change.

### F2. The spec 009 preserve policy governs every redeploy

`routing.go:103-130` — `WriteRoute` returns early with `gateway.route.preserved` if the per-app file already exists. Two consequences:

- **Revert (US2)**: removing `tls: true` and redeploying does nothing until the per-app file is removed. Spec 012 T024 as originally written ("rewrite the manifest; re-deploy; assert reverted") would fail; issue #38 item 3 already corrects this. The existing scenario `should drop alias routers when alias is removed and re-deployed` (`traefik_plugin_test.go:719`) is the pattern to follow: twin fixture with the same app name + explicit `os.Remove`.
- **Byte-stability (US3)**: an unchanged redeploy is byte-identical *because the file is not rewritten*. That is the real operator-visible behaviour and is asserted, but on its own it would not catch non-deterministic generation. See D5.

### F3. Routing finalize (spec 018) — where it runs and what the operator sees

- `internal/engine/engine.go:81-83` — `ExecuteDeploy` calls `finalizeRouting()` after the step loop; a step error returns before it.
- `engine.go:111-122` — emits `routing.finalize` `started`, then on error `emitErr("routing.finalize", …, "routing finalize: %w")`, else `routing.finalize` `info`.
- **Terminal**: `terminal_logger.go:22-23` prints every error event as `  ❌ Error [<name>]: <error>`. There is **no** terminal rendering for a successful `routing.finalize` — success is silent on stdout.
- **Log file**: `internal/ui/file_logger.go:46-52` writes every event to `<state-dir>/logs/shrine.log` as `<ts> [<status>] <name> k="v"…`. So success is observable as `[started] routing.finalize` and `[info] routing.finalize`; failure as `[error] routing.finalize error="routing finalize: …"`.
- **Exit code**: `main.go` exits 1 on any error returned from `cmd.Execute()`.
- `routing.go:203-251` — the Traefik `Finalize` creates the routing and dynamic dirs, generates `traefik.yml` (if absent), handles the dashboard file, then calls `containerBackend.CreateContainer` with `Image: r.resolvedImage()` (config key `plugins.gateway.traefik.image`, `internal/config/plugin_traefik.go:4`). A container error is wrapped as `traefik routing: starting traefik container: …`.

**Consequence for the spec**: spec US5 says the finalize phase is visible "in the output". On success that is true only of the log file, not stdout. The plan asserts success through `shrine.log` and failure through both stdout and `shrine.log`. This is an observation surface choice, not a product divergence — the log file is the operator-facing record spec 018 T012 itself names ("operator-facing log contains a `routing.finalize` event with `status=error`").

### F4. Dry-run

- `internal/handler/deploy.go:77` builds `dryrun.NewDryRunEngine`, which always wires `DryRunRoutingBackend`; the same `ExecuteDeploy` runs, so `Finalize` is invoked after all steps.
- `internal/engine/dryrun/dry_run_routing.go:16,26` — prints `[ROUTE]  WriteRoute: domain=…` per routed app and `[ROUTE]  Finalize` (two spaces) once.
- The existing scenario `should produce no side effects on dry-run while still printing route operations` (`traefik_plugin_test.go:238`) asserts only that stdout contains `[ROUTE]`.

### F5. Integration harness

- `tests/integration/main_test.go` builds the real binary once; `testutils.Execute` runs it as a subprocess and captures exit code, stdout, stderr. Black-box; no `internal/` imports.
- `TestTraefikPlugin` (`traefik_plugin_test.go:86-102`): `NewDockerSuite(t, traefikTestTeam)`; `BeforeEach` force-removes `platform.traefik`, cleans `aliasTestTeam`, creates a fresh `tc.StateDir`, applies the `traefik` fixture team; `AfterEach` repeats the cleanup. Alias scenarios additionally `apply teams` from their own fixture.
- Fixtures: `tests/testdata/deploy/traefik-alias-<variant>/{app.yaml,team.yaml}` via `aliasFixturePath(variant)`; image `traefik/whoami` (untagged), team `shrine-alias-test`.
- Available assertions used here: `AssertSuccess`, `AssertFailure`, `AssertOutputContains`, `AssertOutputNotContains`, `AssertFileExists`, `AssertFileNotExists`, `AssertFileContains`, `AssertContainerRunning`, `AssertContainerNotExists`, `RunResult()` (raw stdout).
- No test in the package calls `t.Parallel()`; scenarios run sequentially.
- Host ports in use across the file: `8081–8087, 8090–8092, 8094–8103, 8106–8108, 8110, 8112–8118`, TLS `8443–8446, 9443`. Nothing above `8118` / `8446` (other than `9443`) is used.
- CI: `.github/workflows/ci.yml` runs `make test-integration` = `go test -tags integration -v -timeout 5m ./tests/integration/...`. The five most recent CI runs complete end-to-end (checkout + build + unit + integration) in 2m48s–4m20s. Per-step timings could not be retrieved, so the integration step's exact share is unknown; it is bounded above by those totals.

### F6. An existing skipped scenario documents a dead end

`traefik_plugin_test.go:463-466` is skipped with the note that `chmod` on the routing dir fails the routing backend **before** `Finalize`. Filesystem-permission tricks are therefore not a usable way to provoke a finalize-only failure.

## Part 2 — Decisions

### D1. Provoke the finalize failure with an unpullable gateway image

- **Decision**: Configure `plugins.gateway.traefik.image: localhost:1/shrine-test/unpullable-gateway:0` for the failure scenario and deploy the standard `traefik` fixture.
- **Rationale**: Per-app steps use their own images and succeed; `WriteRoute` touches only the dynamic dir; the gateway image is resolved only inside `Finalize` → `CreateContainer` → `resolveImage`. Nothing listens on port 1, so the daemon's registry request is refused immediately — deterministic, no external network, no timing. It is a condition an operator really hits (typo in `image:`, private registry down). It needs no product change (FR-011). The scenario asserts only the `routing.finalize` attribution, not the inner `image.pull` wording, so it does not depend on Docker's error text.
- **Alternatives considered**:
  - *Occupy the gateway host port from the test process* — depends on daemon configuration (userland proxy) and fails at container start, leaving a created container; less deterministic.
  - *Filesystem permissions on the routing dir* — fails before Finalize (F6).
  - *Inject a failing backend* (018 T012 as written) — impossible black-box; would need a test-only hook in the shipped binary, forbidden by FR-011 and Constitution V.
  - *A non-existent Docker Hub repository* — works, but needs the network and depends on Hub's response.

### D2. Observe finalize through `shrine.log` on success, stdout + `shrine.log` on failure

- **Decision**: Success scenario asserts `[started] routing.finalize` and `[info] routing.finalize` in `<state-dir>/logs/shrine.log`. Failure scenario asserts stdout contains `Error [routing.finalize]`, and the log contains `[error] routing.finalize`.
- **Rationale**: F3 — stdout is silent on success. The log file is the only black-box evidence that the engine's finalize seam ran (the observable form of 018 T019).
- **Alternatives considered**: adding a terminal line for finalize success — a product change, out of scope (FR-016).

### D3. New scenarios only; no existing scenario is edited

- **Decision**: All seven scenarios are appended to `TestTraefikPlugin`. The existing dry-run and first-deploy scenarios are left byte-for-byte unchanged even though 018 T014/T018 said "extend".
- **Rationale**: Directly serves the planning instruction and SC-004. A new dry-run scenario costs milliseconds (no Docker work); one extra real deploy for the post-deploy scenario is the price of zero risk to existing assertions.

### D4. Revert uses a twin fixture, not in-test manifest rewriting

- **Decision**: Add `traefik-alias-tls-removed` — same app name as `traefik-alias-tls`, same alias, no `tls`. The scenario deploys `tls`, removes the per-app file, deploys `tls-removed`.
- **Rationale**: Mirrors `traefik-alias-prefix` / `traefik-alias-removed` (F2). No temp-dir manifest copying or string surgery in the test.
- **Alternatives considered**: `copyDir` + rewrite `app.yaml` in the test (012 T024 wording) — more code, and hides the manifest under test from a reader browsing fixtures.

### D5. Byte-stability asserts both preservation and regeneration

- **Decision**: Three deploys of `traefik-alias-prefix` with no `tlsPort`: (1) capture bytes; (2) redeploy unchanged → identical; (3) remove the per-app file, redeploy → identical again.
- **Rationale**: Step 2 is what issue #38 item 4 and 012 T026 ask for, but by F2 it passes trivially. Step 3 makes the scenario fail if generation becomes non-deterministic or starts emitting TLS fields for non-TLS aliases after a regenerate.

### D6. Parse the dynamic file; share two small helpers in the test file

- **Decision**: Add to `traefik_plugin_test.go`: `readDynamicRouters` (YAML → `map[string]map[string]any` under `http.routers`), `assertPlainRouter` (entryPoints == `[web]`, no `tls` key) and `assertTLSRouter` (entryPoints == `[web, websecure]`, `tls` present and empty).
- **Rationale**: Substring checks cannot say *which* router has the TLS block — the core of US2. The shape assertions are used ≥6 times across four scenarios (Constitution IV threshold met; VII requires extraction). They stay in the test file, not `testutils`, because they are Traefik-specific and `testutils` is generic.

### D7. Ports and names

- **Decision**: HTTP ports `8119–8125`, TLS ports `8447–8449`, one pair per scenario, none reused. App names `whoami-tls` and `whoami-tls-mixed` in team `shrine-alias-test`; image `traefik/whoami` (untagged, as existing fixtures).
- **Rationale**: F5 — outside every range already used; the existing `BeforeEach`/`AfterEach` already clean `shrine-alias-test.*` containers and `platform.traefik`, so no cleanup code changes. Using the untagged image avoids a second image pull in CI (012 T016 named `v1.10.1`; not adopted).

### D8. Suite timeout is watched, not pre-emptively changed

- **Decision**: Leave `-timeout 5m` alone. After the first CI run on the PR, read the integration step duration; raise the Makefile timeout only if the step exceeds 4 minutes.
- **Rationale**: The feature adds nine real deploys and one dry-run. Whole CI runs currently take 2m48s–4m20s (F5), so the integration step is under that. Changing shared build config without evidence contradicts "don't break anything else"; a timeout breach fails loudly and unambiguously if it happens.

## Part 3 — Regression-safety analysis

| Risk | Why it does not materialise / mitigation |
|------|------------------------------------------|
| Product behaviour changes | Zero files under `cmd/`, `internal/`, or `main.go` are touched. Gate: `git diff --stat main` shows only `tests/` and `specs/`. |
| Existing scenarios altered | D3 — append-only edit to `traefik_plugin_test.go`; existing helper signatures unchanged; three new helpers have new names. |
| Fixture discovery picks up new dirs | Every consumer addresses a fixture by explicit sub-path via `fixturesPath(...)`; nothing loads `tests/testdata/deploy` as a whole, and no non-integration Go code references it. |
| Host-port collisions | D7 — unused ranges; scenarios are sequential; `BeforeEach` removes the gateway container. |
| Leftovers after the failing-finalize scenario | The gateway container is never created (image resolution fails first); app containers belong to `shrine-traefik-test` and are removed by `NewDockerSuite`'s `AfterEach`; files live in `t.TempDir()`. |
| Team/app name collisions | New app names are unique; `shrine-alias-test` is already cleaned before and after every scenario. |
| Routing-collision detection across fixtures | Collision detection is scoped to the apps in the deployed path; each scenario deploys one fixture directory. `alias.shrine.lab` reuse across fixtures is therefore safe, as it already is for `host-only` and `prefix`. |
| Unit suite | No unit test or non-tagged file is added or changed; `go test ./...` does not compile `integration`-tagged files. |
| CI wall-clock | D8. |
| Docs drift | No Cobra strings change, so generated CLI docs are unaffected. |
| A scenario fails against shipped code | Per FR-016, stop and report; do not weaken the assertion or patch the product inside this feature. |
