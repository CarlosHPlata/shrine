# Quickstart: Verifying Strict Apply Failures and Scoped Collision Detection

**Feature**: `025-fix-apply-parse-collision` | **Date**: 2026-08-24

Manual walkthrough of the three user stories against a built binary, plus the automated gates. All paths are relative to the repository root; `$STATE` is a throwaway state directory.

```bash
go build -o shrine .
export STATE=$(mktemp -d)
```

## Story 1 — `apply teams` fails loudly

```bash
# bad-kind/ holds a valid team.yaml and typo.yaml (kind: Aplication)
./shrine apply teams --path tests/testdata/apply/bad-kind --state-dir "$STATE"; echo "exit=$?"
```

Expected:
- `exit=1`
- stderr: `Error: apply teams failed:` followed by `- parsing manifest ".../typo.yaml": unknown manifest kind: "Aplication"`
- `ls "$STATE"` shows **no** team record for `shrine-apply-test` (nothing written)

```bash
# A clean directory still syncs and exits 0
./shrine apply teams --path tests/testdata/apply/success --state-dir "$STATE"; echo "exit=$?"
```

Expected: `exit=0`, stdout `Synced team: shrine-apply-test` and `Successfully synced 1 teams to state.`; the Application/Resource files in that directory are reported as `Skipping ...: not a Team manifest`.

## Story 2 — `apply -f` rejects a colliding manifest

```bash
FIX=tests/testdata/apply/routing-collision   # team.yml, app-a.yml + app-b.yml share collision.apply.local, app-c.yml has no routing
./shrine apply teams --path "$FIX" --state-dir "$STATE"

./shrine apply -f "$FIX/app-b.yml" --path "$FIX" --state-dir "$STATE"; echo "exit=$?"
```

Expected:
- `exit=1`
- stderr: `routing validation failed:` and `routing collision: host="collision.apply.local" pathPrefix="" declared by "shrine-apply-test/app-a" and "shrine-apply-test/app-b"`
- `docker ps -a --filter name=shrine-apply-test.app-b` shows nothing

```bash
# Parity with deploy: same directory, same diagnostic
./shrine deploy --dry-run --path "$FIX" --state-dir "$STATE" 2>&1 | grep 'routing collision'

# An unrelated collision does not block a clean app
./shrine apply -f "$FIX/app-c.yml" --path "$FIX" --state-dir "$STATE"; echo "exit=$?"
docker ps --filter name=shrine-apply-test.app-c
./shrine teardown shrine-apply-test --state-dir "$STATE"
```

Expected: the `grep` prints the identical collision line; `app-c` applies with `exit=0` and its container is running.

## Story 3 — team-scoped deploy only fails on its own collisions

```bash
TFIX=tests/testdata/deploy_team
./shrine apply teams --path "$TFIX/teams" --state-dir "$STATE"

# collision-other/: team-a's alpha is clean; team-b's beta1/beta2 collide
./shrine deploy team shrine-team-a --dry-run --path "$TFIX/collision-other" --state-dir "$STATE"; echo "exit=$?"   # 0
./shrine deploy team shrine-team-b --dry-run --path "$TFIX/collision-other" --state-dir "$STATE"; echo "exit=$?"   # 1, names beta1 and beta2
./shrine deploy --dry-run --path "$TFIX/collision-other" --state-dir "$STATE"; echo "exit=$?"                       # 1 (bare deploy unchanged)

# collision-cross/: team-a's alpha collides with team-b's beta
./shrine deploy team shrine-team-a --dry-run --path "$TFIX/collision-cross" --state-dir "$STATE"; echo "exit=$?"   # 1, names shrine-team-a/alpha and shrine-team-b/beta

# collision/: both colliding apps are team-a (existing Scenario E)
./shrine deploy team shrine-team-a --dry-run --path "$TFIX/collision" --state-dir "$STATE"; echo "exit=$?"         # 1 (unchanged)
```

## Automated gates

```bash
go test ./internal/planner/ ./internal/handler/ ./internal/manifest/   # unit — in-memory only, no filesystem
go test ./...                                                          # full unit suite
go vet -tags integration ./tests/integration/...                       # compile-check the integration scenarios
```

The integration suite (`make test-integration`) is executed by CI; it is intentionally not run locally. The scenarios that gate this feature:

| Suite | Scenario | Story |
|-------|----------|-------|
| `TestApplyTeams` | bad-kind exits non-zero, names file + kind, writes no team | 1 |
| `TestApplyTeams` | multi-broken reports both files | 1 |
| `TestApplyTeams` | invalid-team (no `metadata.name`) rejected | 1 |
| `TestApplyFile` | `apply -f app-b.yml` rejected with the deploy diagnostic, no container | 2 |
| `TestApplyFile` | `apply -f app-c.yml` succeeds despite an unrelated collision | 2 |
| `TestDeployTeam` | out-of-scope collision passes for team-a, fails for team-b and bare deploy | 3 |
| `TestDeployTeam` | cross-team in-scope collision fails naming both apps | 3 |
| `TestDeployTeam` | Scenario E (both team-a) still fails | 3 |

## Cleanup

```bash
rm -rf "$STATE"; unset STATE
```
