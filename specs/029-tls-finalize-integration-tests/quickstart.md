# Quickstart: Verifying Feature 029

## Local gates (fast, no Docker)

Run from the repository root. All must pass before pushing.

```bash
go build ./...
```

```bash
go test ./...
```

```bash
go vet -tags integration ./tests/integration/...
```

The last command compile-checks the new scenarios without running them.

Confirm nothing outside tests and specs changed:

```bash
git diff --stat main -- . ':!tests' ':!specs' ':!graphify-out' ':!CLAUDE.md' ':!.specify'
```

Expected: empty output.

## Integration gate (CI)

The integration suite is slow and needs a Docker daemon; by project practice it is not run locally. Push the branch and let CI run `make test-integration`.

In the CI log, confirm:

1. The seven new sub-tests of `TestTraefikPlugin` report `--- PASS` (names in [data-model.md](data-model.md)).
2. Every pre-existing sub-test still reports `--- PASS` (or the one pre-existing `--- SKIP`).
3. The `Integration Test` step finished in under 4 minutes. If not, raise `-timeout` in the `test-integration` Makefile target (research D8).

Re-run the workflow twice more to satisfy SC-005 (three consecutive green runs).

## Optional: run one scenario locally

Only if a Docker daemon is available and a CI failure needs reproducing:

```bash
go test -tags integration -v -timeout 5m ./tests/integration/... -run 'TestTraefikPlugin/should_publish_alias_router_with_tls_block'
```

## Proving the scenarios bite (SC-003)

Optional spot-check on a throwaway branch — never commit these edits:

| Temporary break | Scenario expected to fail |
|-----------------|---------------------------|
| In `internal/plugins/gateway/traefik/routing.go`, drop `r.TLS = &tlsBlock{}` | S1, S2 |
| Apply the TLS branch unconditionally | S2, S3, S4 |
| In `engine.finalizeRouting`, return `nil` on error | S5 |
| Rename the `routing.finalize` event | S5, S7 |
| Remove the `Finalize` print from `DryRunRoutingBackend` | S6 |

## If a new scenario fails against unchanged product code

Stop. Per FR-016 this is a divergence between shipped behaviour and spec 012/018: report it with the failing assertion and the CI output, and decide separately whether the product or the expectation is wrong. Do not weaken the assertion and do not patch `internal/` on this branch.
