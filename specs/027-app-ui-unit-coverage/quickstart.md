# Quickstart: Verifying the Composition-Root and Renderer Coverage

**Feature**: `027-app-ui-unit-coverage` | **Date**: 2026-08-28

This feature delivers tests, so verification is running them — hermetically, under the race detector, and with a few deliberate mutations to prove they bite. All paths are relative to the repository root.

## 1. The three unit packages, hermetically

Stop or point away from any Docker daemon, strip the environment the tests pin themselves, and run the packages this feature touches:

```bash
env -u HOME -u DOCKER_HOST -u DOCKER_CERT_PATH -u DOCKER_TLS_VERIFY \
  go test -count=1 ./internal/app/ ./internal/ui/ ./internal/handler/
```

Expected: all pass; no `logs/` directory or `shrine.log` appears anywhere (`git status` is clean; nothing under `/tmp` from these packages); wall-clock for the three packages combined stays under 5 s (the spinner tests account for ≈ 1–2 s).

## 2. Data-race check (spec FR-001)

```bash
go test -race -count=3 ./internal/app/ ./internal/ui/ ./internal/handler/
```

Expected: pass. The spinner tests are the ones that would flag a race if the terminal tests wrote into a bare `bytes.Buffer` — they use the mutex-guarded `safeBuffer` instead (research D8).

## 3. Story-by-story spot checks

### Story 1 — composition root

```bash
go test -run 'TestBuild|TestBundleCleanup|TestJoinCleanup|TestRoutingFromPlugin' -v ./internal/app/
```

Expected: happy-path tests for apply/deploy/teardown, the two subset-shape tests, the slot-failure table (every prefix: `validating registries:`, `observer:`, `container backend:`, `traefik:`, `vault:`, `routing:`, `engine:`), and the cleanup tests all pass.

### Story 2 — terminal rendering

```bash
go test -run 'TestTerminalObserver' -v ./internal/ui/
```

Expected: one sub-test per rendered kind (33 kinds), the generic error line, the indicator lifecycle per step kind, `volume.created`, `container.remove` not-found, and the silence cases.

### Story 3 — handler isolation seam

```bash
go test -run 'TestTeardown' -v ./internal/handler/
```

Expected: three passing tests; `grep -n "app.Build\|local\.\|traefik\.\|infisical" internal/handler/teardown_test.go` prints nothing but the `app.TeardownBundle{` literal.

### Story 4 — file logger

```bash
go test -run 'TestFileLogger' -v ./internal/ui/
```

Expected: format, field-less, every-status, concurrency (1000 whole lines), and Close-forwarding tests pass.

## 4. Mutation spot-checks (spec SC-007)

Each edit below must turn at least one unit test red; revert after each.

```bash
# (i) drop a slot prefix
sed -i 's/fmt.Errorf("vault: %w", err)/err/' internal/app/app.go && go test ./internal/app/ ; git checkout internal/app/app.go
# (ii) forget to close the log writer on a late failure
sed -i '0,/_ = closeObserver()/{/_ = closeObserver()/d}' internal/app/app.go && go test ./internal/app/ ; git checkout internal/app/app.go
# (iii) render the wrong field
sed -i 's/Creating fresh container: %s\\n", e.Fields\["name"\]/Creating fresh container: %s\\n", e.Fields["ref"]/' internal/ui/terminal_logger.go && go test ./internal/ui/ ; git checkout internal/ui/terminal_logger.go
# (iv) unsort log fields
sed -i '/sort.Strings(keys)/d' internal/ui/file_logger.go && go test -count=3 ./internal/ui/ ; git checkout internal/ui/file_logger.go   # multi-field events make unsorted output detectable
```

Expected: `FAIL` for each, `ok` after each revert.

## 5. Integration scenario (authored locally, executed by CI)

```bash
go vet -tags integration ./tests/integration/...      # compile-check the new file_logger_test.go
```

CI runs `make test-integration`, which executes `TestFileLogger` on `NewDockerSuite`: after `apply teams` + `deploy` the log exists and holds `[started] application.deploy`; after `teardown` that entry is still there and `[started] application.teardown` follows it.

To watch it by hand against a Docker host (optional; slow):

```bash
go build -o shrine . && export STATE=$(mktemp -d)
./shrine apply teams --path "$PWD/tests/testdata/deploy/team" --state-dir "$STATE"
./shrine deploy --path "$PWD/tests/testdata/deploy/basic" --state-dir "$STATE"
./shrine teardown shrine-deploy-test --state-dir "$STATE"
grep -c 'application.deploy\|application.teardown' "$STATE/logs/shrine.log"   # ≥ 2, deploy lines before teardown lines
```

## 6. Whole-repo gates

```bash
go build ./...
go test ./...
graphify update .        # refresh the knowledge graph after the code change
```
