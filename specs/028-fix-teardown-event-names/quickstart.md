# Quickstart: Verifying Teardown Headers and Lowercase Teardown Event Names

**Feature**: `028-fix-teardown-event-names` | **Date**: 2026-10-03

Manual walkthrough of the two user stories against a built binary, plus the automated gates. All paths are relative to the repository root. The manual part needs a running Docker daemon and mirrors the setup of `TestTeardown`; `$STATE` is a throwaway state directory.

```bash
go build -o shrine .
```

```bash
export STATE=$(mktemp -d)
```

```bash
./shrine apply teams --path tests/testdata/deploy/team --state-dir "$STATE"
```

```bash
./shrine deploy --path tests/testdata/deploy/basic --state-dir "$STATE"
```

## Story 1 — the operator sees which deployment is being torn down

```bash
./shrine teardown shrine-deploy-test --state-dir "$STATE"
```

Expected, in this order on stdout:

- `[shrine] Planning teardown for team: shrine-deploy-test`
- `🗑️  Tearing down Application: whoami (team: shrine-deploy-test)` — **new**; exactly once
- the container-removal lines for `shrine-deploy-test.whoami`
- the network-removal line for `shrine.shrine-deploy-test.private`

Before the fix the second line is missing and the output goes straight from "Planning teardown" to the removal lines.

For the resource header, repeat with the two-team fixture (`apply teams --path tests/testdata/teardown`, `deploy --path tests/testdata/teardown`, `teardown shrine-teardown-a`) and expect `🗑️  Tearing down Resource: shared-cache (team: shrine-teardown-a)`. Tear down `shrine-teardown-b` afterwards to clean up.

## Story 2 — teardown entries are named like everything else

```bash
grep -E 'teardown' "$STATE/logs/shrine.log"
```

Expected: one line ending in `[started] application.teardown name="whoami" team="shrine-deploy-test"`.

```bash
grep -cE '\] [A-Z]' "$STATE/logs/shrine.log"
```

Expected: `0` — no entry in a log produced entirely by the fixed binary has a name starting with a capital letter. (A log that already held entries from an older binary keeps them; the log is append-only.)

The failure names (`application.remove`, `resource.remove`, `application.routing_remove`) cannot be provoked reliably against a healthy Docker daemon; they are verified by the engine unit tests below.

## Automated gates

Unit tests — run locally, no Docker, no filesystem:

```bash
go test ./internal/engine/ ./internal/handler/ ./internal/ui/
```

```bash
go test ./...
```

Integration tests — compile-check locally, executed by CI (`make test-integration`):

```bash
go vet -tags integration ./tests/integration/...
```

## Red-first check

With the test changes applied and `internal/engine/engine.go` still unfixed:

```bash
go test ./internal/engine/ -run 'TestEngine_ExecuteTeardown_(AnnouncesEachDeploymentBeforeRemovingIt|NamesFailureEventsInLowercase)'
```

```bash
go test ./internal/handler/ -run 'TestTeardown_'
```

Expected: both fail, reporting `Application.teardown` / `Application.remove` where the lowercase names are wanted. After the one-function fix in `teardownKind`, both pass and no other test changes status.

## Regression probe (SC-006)

Temporarily revert `eventPrefix` to `kind` in `internal/engine/engine.go` and re-run `go test ./internal/engine/ ./internal/handler/` — the two engine tests and two handler tests above must go red. Restore the fix afterwards.
