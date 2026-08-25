# Quickstart: Verifying Field-Naming Config Path Errors

**Feature**: `026-fix-config-path-errors` | **Date**: 2026-08-24

Manual walkthrough of the two user stories against a built binary, plus the automated gates. All paths are relative to the repository root; `$STATE` and `$CONF` are throwaway directories. `env -u HOME` reproduces the cron/CI/container environment the issue describes.

```bash
go build -o shrine .
export STATE=$(mktemp -d) CONF=$(mktemp -d)
printf 'specsDir: ~/manifests\nteamsDir: ~/teams\n' > "$CONF/config.yml"
```

## Story 1 — the operator is told which field failed

```bash
env -u HOME ./shrine deploy --config-dir "$CONF" --state-dir "$STATE"; echo "exit=$?"
```

Expected:
- `exit=1`
- stderr: `Error: resolving specsDir: expanding ~: $HOME is not defined` — once
- `ls "$STATE"` shows **no** `logs/` directory (nothing was composed)

```bash
env -u HOME ./shrine apply teams --config-dir "$CONF" --state-dir "$STATE"; echo "exit=$?"
```

Expected: `exit=1`, stderr names `teamsDir`; `ls "$STATE/teams" 2>/dev/null` is empty.

```bash
# Fallback names the field that supplied the value
printf 'specsDir: ~/manifests\n' > "$CONF/config.yml"
env -u HOME ./shrine apply teams --config-dir "$CONF" --state-dir "$STATE" 2>&1 | grep -o 'resolving [a-zA-Z-]*'
```

Expected: `resolving specsDir` (not `teamsDir`).

```bash
# The flag is named when the flag supplied the value (quote the tilde so the shell does not expand it)
env -u HOME ./shrine deploy --path '~/manifests' --config-dir "$CONF" --state-dir "$STATE" 2>&1 | grep -o 'resolving [a-zA-Z-]*'
```

Expected: `resolving --path`.

```bash
# generate and apply -f fail the same way
env -u HOME ./shrine generate team demo --config-dir "$CONF" --state-dir "$STATE"; echo "exit=$?"
env -u HOME ./shrine apply -f tests/testdata/app.yml --config-dir "$CONF" --state-dir "$STATE"; echo "exit=$?"
```

Expected: both `exit=1` with `resolving specsDir: expanding ~: $HOME is not defined`.

```bash
# An absolute --path is never affected by the unresolvable config value
env -u HOME ./shrine apply teams --path "$PWD/tests/testdata/deploy/team" --config-dir "$CONF" --state-dir "$STATE"; echo "exit=$?"
env -u HOME ./shrine deploy --dry-run --path "$PWD/tests/testdata/deploy/basic" --config-dir "$CONF" --state-dir "$STATE"; echo "exit=$?"
```

Expected: both `exit=0`; the dry-run prints its plan.

## Story 2 — a resolution failure stops the command at every stage

`teardown` is the command with no command-layer check; before this fix it silently proceeded with an empty specs directory.

```bash
env -u HOME ./shrine teardown demo-team --config-dir "$CONF" --state-dir "$STATE"; echo "exit=$?"
ls "$STATE"
```

Expected:
- `exit=1`, stderr `Error: resolving specsDir: expanding ~: $HOME is not defined` — once
- no `logs/` directory under `$STATE` (the observer was never opened, so no engine, plugin, or Docker call happened)

```bash
# Absent specsDir is still fine for teardown (reads no manifests)
: > "$CONF/config.yml"
./shrine teardown demo-team --config-dir "$CONF" --state-dir "$STATE"; echo "exit=$?"
```

Expected: `exit=0` (team not in state → nothing to do) — unchanged behaviour; `logs/` now exists because the bundle was composed.

## Automated gates

```bash
go test ./internal/config/ ./internal/app/            # resolver tables (noHome cases) + bundle/helper tests; no filesystem
go test ./...                                          # full unit suite
go vet -tags integration ./tests/integration/...       # compile-check the new integration file (CI executes it)
graphify update .                                      # refresh the knowledge graph after the code change
```

CI runs `tests/integration/config_paths_test.go` (`NewSuite`, no Docker) and the existing `TestTeardown` (teardown without `specsDir`, Docker).
