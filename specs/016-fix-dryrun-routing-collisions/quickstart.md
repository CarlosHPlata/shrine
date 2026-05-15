# Quickstart: Verifying the `--dry-run` Routing Collision Fix

**Phase**: 1 (Design)
**Feature**: 016-fix-dryrun-routing-collisions

This guide shows reviewers how to reproduce the bug on `main` and verify the fix on this branch. It is intentionally minimal — the bug is observable in seconds.

## Reproduce on `main`

1. Check out `main`, build the binary:

   ```bash
   git checkout main
   go build -o bin/shrine ./cmd/shrine
   ```

2. Author a two-application manifest set that shares a `routing.domain`. The simplest fixture is the issue's own example:

   ```yaml
   # /tmp/collision/manifests.yaml
   apiVersion: shrine/v1
   kind: Application
   metadata:
     name: app-a
     team: team-one
   spec:
     image: nginx
     port: 80
     routing:
       domain: example.local
   ---
   apiVersion: shrine/v1
   kind: Application
   metadata:
     name: app-b
     team: team-one
   spec:
     image: nginx
     port: 80
     routing:
       domain: example.local
   ```

   You will also need a minimal `Team` manifest declaring `team-one`. Reuse the existing one under `tests/testdata/deploy/team/` or author a one-liner.

3. Run dry-run:

   ```bash
   ./bin/shrine deploy --dry-run --path /tmp/collision --state-dir /tmp/collision-state
   echo "exit=$?"
   ```

   **Observed (bug)**: `exit=0`, no mention of the duplicated `example.local` domain.

## Verify on this branch

1. Check out `016-fix-dryrun-routing-collisions`, rebuild:

   ```bash
   git checkout 016-fix-dryrun-routing-collisions
   go build -o bin/shrine ./cmd/shrine
   ```

2. Run the same command against the same `/tmp/collision` directory:

   ```bash
   ./bin/shrine deploy --dry-run --path /tmp/collision --state-dir /tmp/collision-state
   echo "exit=$?"
   ```

   **Expected**:
   - `exit` is non-zero.
   - Output contains the line `routing validation failed:`.
   - Output names both applications by their `<team>/<name>` identifier — `team-one/app-a` and `team-one/app-b`.
   - Output names the duplicated host (`example.local`).

3. Confirm parity with real deploy. Without changing any manifests, drop the `--dry-run` flag:

   ```bash
   ./bin/shrine deploy --path /tmp/collision --state-dir /tmp/collision-state
   ```

   **Expected**: the same collision diagnostic appears, and the command exits non-zero before touching Docker.

4. Confirm no regression on clean manifests. Edit `app-b.yaml` to declare a different `routing.domain` (e.g. `example2.local`) and re-run dry-run:

   ```bash
   ./bin/shrine deploy --dry-run --path /tmp/collision --state-dir /tmp/collision-state
   echo "exit=$?"
   ```

   **Expected**: `exit=0`, normal plan output, no collision diagnostic.

## Automated verification

The work merges in two test layers:

- **Unit** (`go test ./internal/planner/...`) — a planner-level test exercises `planner.Plan` against an in-memory duplicate-domain set and asserts the collision lands in `PlanResult.ValidationErr`. Fast feedback loop.
- **Integration** (`go test -tags integration ./tests/integration/...` or `make test-integration`) — a new scenario in `TestDeploy` runs the real binary against the new `tests/testdata/deploy/routing-collision/` fixture and asserts non-zero exit plus the diagnostic substring. This is the Principle V gate.

Run order during implementation (TDD per Principle V): write the integration scenario first, observe it fail on the unmodified code, then make the planner edit, then watch both layers go green.
