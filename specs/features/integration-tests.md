# Spec: Integration Test Suite

## Status
In progress (Phases 1–5 complete)

## Goal

An integration test suite that runs shrine commands as a real binary against a live Docker daemon and asserts results at the filesystem, Docker, and YAML level.

## Design Decisions

### Binary harness (not in-process)
Tests build the real shrine binary once in `TestMain` and run it as a subprocess. This is a true black-box integration test — it tests exactly the artifact a user would run, not the internal call graph. The existing `cmd/cmd_test.go` uses in-process testing (`cmd.Execute()`); the integration suite is a separate, higher-level layer.

### Build tags
All files under `tests/integration/` carry `//go:build integration`. This keeps `go test ./...` fast (unit tests only) and requires an explicit opt-in: `go test -tags integration ./tests/integration/...`.

### Jest/JUnit-style fluent API
Tests read like a spec, not like Go boilerplate. The `testutils` package exposes:
- `Test(t, "description", func(tc))` — named subtest wrapper (mirrors Jest's `test("name", () => {...})`)
- `tc.Run("shrine", "args"...)` — executes the binary, stores Result
- `tc.AssertSuccess()` / `tc.AssertFailure()` — exit code assertions
- `tc.AssertOutputContains(s)` — stdout substring
- `tc.AssertStderrContains(s)` — stderr substring
- `tc.AssertFileExists(path)` — filesystem assertion
- `tc.AssertSpecHas(path, "dot.notation.key", "value")` — parses YAML and asserts a field value

All assertion methods return `*TestCase` for chaining.

### Capture both stdout and stderr
Success messages go to stdout (`fmt.Printf`). Cobra prints errors to stderr. Both streams are captured in `Result.Stdout` / `Result.Stderr`.

### Assert YAML content, not just file existence
Flag tests parse the generated YAML and assert field values via dot-notation keys. A flag silently ignored by the handler would be caught.

---

## Directory Structure

```
tests/
    integration/
        testdata/               # fixture files for future phases
        testutils/
            harness.go          # Setup(binaryPath), Execute(), Result struct
            testcase.go         # Test(), TestCase struct, all assertion methods
            dockersuite.go      # NewDockerSuite — Docker client + lifecycle hooks
            assert_docker.go    # Docker-specific assertions
            assert_state.go     # State dir assertions
            assert_spec.go      # YAML spec assertions
            assert_general.go   # General assertions
        main_test.go            # TestMain: builds binary once, calls testutils.Setup
        generate_test.go        # generate team / application / resource tests
        apply_test.go           # apply teams + apply -f tests
        deploy_test.go          # deploy tests (requires Docker)
```

---

## Run Commands

```bash
# Unit tests only
make test

# Integration tests
make test-integration

# Both
make test-all
```

---

## Phase Checklist

### Phase 1 — Generate command tests ✅
Command under test: `cmd/generate.go`

- `generate team <name>`
  - generates a new team manifest; assert file created and YAML fields match
  - fails when a manifest with the same name already exists
  - writes to the directory specified by `--path`
- `generate application <name>`
  - generates a new app manifest with defaults
  - populates manifest fields from flags: `--team`, `--port`, `--replicas`, `--domain`, `--pathprefix`, `--expose`, `--image`
  - fails when a manifest with the same name already exists
  - writes to the directory specified by `--path`
- `generate resource <name>`
  - generates a new resource manifest with defaults
  - populates manifest fields from flags: `--team`, `--type`, `--version`, `--expose`
  - fails when a manifest with the same name already exists
  - writes to the directory specified by `--path`

### Phase 2 — Suite hooks (beforeEach / afterEach) ✅

`Suite` struct alongside the existing standalone `Test()` function:

```go
s := testutils.NewSuite(t)
s.BeforeEach(func(tc *TestCase) { ... })
s.AfterEach(func(tc *TestCase) { ... })
s.Test("should generate a new team manifest", func(tc *TestCase) {
    tc.Run("generate", "team", "my-team", "--path", tc.TempDir()).
        AssertSuccess().
        AssertFileExists(tc.Path("my-team.yml"))
})
```

Also added `tc.Path(name string) string` → `filepath.Join(tc.TempDir(), name)`.

### Phase 3 — Apply teams command tests ✅

No Docker needed. `apply teams` reads YAML and writes JSON to the state directory — pure filesystem.

**State assertions** (`testutils/assert_state.go`):
- `tc.AssertTeamInState(name string) *TestCase` — asserts `<StateDir>/teams/<name>.json` exists
- `tc.AssertTeamCount(n int) *TestCase` — asserts number of `.json` files in `<StateDir>/teams/`

Test cases (`tests/integration/apply_test.go`):
- `apply teams --path <dir>` — generate a team manifest, apply it, assert it appears in state
- `apply teams --path <dir>` with team in a subdirectory — assert recursive walk picks it up
- `apply teams --path <dir>` with multiple teams — assert all saved, assert count

### Phase 4 — Docker-backed deploy tests ✅

Requires a live Docker daemon. Ubuntu GH Actions runners have Docker out of the box — no DinD needed.

**Image choice:** `traefik/whoami` (~3MB). No config, starts immediately, stays running, serves HTTP.

**Test isolation strategy:** Fixture manifests use a fixed team name `shrine-test`. `BeforeEach` force-removes any leftover containers/network. `AfterEach` via `t.Cleanup` removes all containers and network created by the test.

**New files:**
- `testutils/dockersuite.go` — `NewDockerSuite(t) *DockerSuite`, wraps `Suite`, holds a Docker client, wires BeforeEach/AfterEach automatically
- `testutils/assert_docker.go` — `AssertContainerRunning`, `AssertNetworkExists`, `AssertContainerEnvVar`
- `tests/testdata/deploy/` — fixture manifests (checked in, not generated at test time)

**Test cases** (`tests/integration/deploy_test.go`):
- basic: team + app → assert container running, name is `<team>.<app>`, network `<team>` exists
- static env vars: app with hardcoded env vars → assert container has them
- resource env vars: team + resource + app with dependency → assert injected connection env vars
- exposeToPlatform: app with `exposeToPlatform: true` → assert container attached to `shrine.platform` network

### Phase 5 — Generated & Template Secret Tests ✅

**New assertion helpers:**
- `tc.AssertSecretInState(team, resource, output)` — reads `<StateDir>/<team>/secrets.env`, asserts key `<resource>.<output>` exists and is non-empty
- `tc.AssertSecretValueInState(team, resource, output, expected)` — asserts exact value
- `tc.SecretFromState(team, resource, output) string` — returns value, `t.Fatal` if missing
- `tc.AssertContainerEnvVarNotEmpty(container, key)` — asserts env var set and non-empty

**Fixture:** `tests/testdata/deploy/secrets/`
- `resource.yml`: `secret-store` resource with `password` (`generated: true`) and `connection` (`template: "redis://{{.host}}:6379"`)
- `app.yml`: `whoami-secrets` app using both outputs via `valueFrom`

**Test cases:**
- generated secret persisted: deploy → `AssertSecretInState(team, "secret-store", "password")`
- template output injected: deploy → `AssertContainerEnvVar(..., "SECRET_CONNECTION", "redis://shrine-deploy-test.secret-store:6379")`
- generated value injected: deploy → `AssertContainerEnvVarNotEmpty(..., "SECRET_PASSWORD")`
- idempotent re-deploy: deploy twice → `SecretFromState` after each → values match

---

### Phase 6 — `apply -f` error cases (pending)

No Docker. All errors are returned before the deploy engine is reached — pure parsing and validation.

Suite: `NewSuite` in `tests/integration/apply_test.go`

Test cases:
- show error when the target file does not exist
- show error when the file contains malformed YAML (fails YAML parse)
- show error when the manifest kind is unknown (e.g. `kind: Unknown`)
- show error when applying a Team manifest via `-f` ("use 'shrine apply teams' instead")
- show error when the file extension is not `.yml` or `.yaml` (e.g. `.txt`)

Note: `manifest.Parse` does not currently validate the file extension. This test defines expected behavior; if it fails, shrine needs an extension guard in the apply handler or cmd layer.

---

### Phase 7 — `apply -f` deploy cases (pending)

Requires a live Docker daemon.

Suite: `NewDockerSuite(t, testTeam)` in `tests/integration/apply_test.go`

Fixtures: `tests/testdata/apply/`
- `team.yml` — Team manifest for `shrine-deploy-test`
- `app.yml` — minimal Application (`traefik/whoami`, name `whoami-apply`)
- `app.yaml` — identical content, `.yaml` extension
- `resource.yml` — minimal Resource (`traefik/whoami`, name `apply-cache`)
- `app-with-dep.yml` — Application with `valueFrom: resource.apply-cache.host`

Test cases:
- deploy an application spec via `apply -f app.yml` → `AssertContainerRunning(testTeam+".whoami-apply")`
- deploy a resource spec via `apply -f resource.yml` → `AssertContainerRunning(testTeam+".apply-cache")`
- accept a `.yaml` extension → `AssertContainerRunning(...)`
- after resource deployed, `apply -f app-with-dep.yml` resolves `valueFrom` env from state → `AssertContainerEnvVar(..., "CACHE_HOST", testTeam+".apply-cache")`

Note: `PlanSingle` uses `specsDir` from config for dependency resolution. In tests with no config, a minimal ManifestSet containing only the target file is used. The last test may require passing `--config-dir` pointing to a config with `specsDir` set to the `apply/` fixture dir. Investigate before writing the fixture.

---

### Phase 8 — `delete team` tests (pending)

Split across two suites because the "has deployments" check reads from `deployments.txt`, only written after a real Docker deploy.

**No-Docker suite** (`tests/integration/delete_test.go`):
- delete a team from state → `apply teams` to register, `delete team <name>`, assert exit 0, assert team JSON file gone
- error when deleting a team that does not exist → `AssertFailure()`, `AssertStderrContains("not found")`

**Docker suite** (`tests/integration/delete_test.go`):
- error when the team has active deployments → deploy a basic app, `delete team <name>`, `AssertFailure()`, `AssertStderrContains("active deployments")`

New assertion helper: `tc.AssertTeamNotInState(name string) *TestCase` (add to `assert_state.go`)

---

### Phase 9 — `describe` tests (pending)

**No-Docker suite** (`tests/integration/describe_test.go`):
- can describe a team → `AssertSuccess()`, `AssertOutputContains(teamName)`
- error if team does not exist → `AssertFailure()`
- error if app does not exist → `AssertStderrContains("not found")`
- error if resource does not exist → `AssertStderrContains("not found")`

**Docker suite** (`tests/integration/describe_test.go`):
- can describe an app → `describe app whoami --team shrine-deploy-test`, `AssertOutputContains("whoami")`
- can describe a resource → `describe resource <name> --team shrine-deploy-test`, `AssertOutputContains("<name>")`

---

### Phase 10 — `get` tests (pending)

**No-Docker suite** (`tests/integration/get_test.go`):
- can get teams → `shrine get teams`, `AssertOutputContains(teamName)`

**Docker suite** (`tests/integration/get_test.go`):
- can get deployed → `AssertOutputContains("whoami")`
- can get apps → `AssertOutputContains("whoami")`
- can get resources → `AssertOutputContains("<resource-name>")`

---

### Phase 11 — `status` tests (pending)

**No-Docker suite** (`tests/integration/status_test.go`):
- error when getting status of non-existing app → `AssertStderrContains("not found")`
- error when getting status of non-existing resource → same

**Docker suite** (`tests/integration/status_test.go`):
- status of a team → `AssertOutputContains("running")`
- status of an app → `AssertOutputContains("running")`
- status of a resource → `AssertOutputContains("running")`

---

### Phase 12 — `teardown` tests (pending)

All require Docker.

**Analysis:** `PlanTeardown(team)` reads only that team's `deployments.txt`. No cross-team dependency check in the teardown path. Team-b containers continue running after team-a teardown — env vars were baked in at deploy time.

New assertion helpers (add to `assert_docker.go`):
- `tc.AssertContainerNotRunning(name string) *TestCase` — inspect; assert absent or stopped
- `tc.AssertNetworkNotExists(name string) *TestCase` — network inspect; assert not found

Fixtures for cross-team test (`tests/testdata/teardown/`):
- `team-a.yml` + `team-b.yml`
- `team-a/resource.yml` — resource owned by team-a with `exposeToPlatform: true`
- `team-b/app.yml` — app owned by team-b with `valueFrom` on team-a's resource

Test cases:
- can teardown a deployed team → deploy basic, teardown, assert containers gone, assert network gone
- deploy two teams, teardown one → assert team-a gone, team-b still running
- cross-team dep teardown → teardown team-a with team-b depending on it → team-a gone, team-b still running, exit 0
