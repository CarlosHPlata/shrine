# Contract: operator-visible output of `shrine delete resource`

**Feature**: 037-delete-resource

Every string below is exact unless marked as a shape. `<team>/<name>` is the slashed form the delete handlers print; `<pinned>` is the pin's pullable exact version (`127.0.0.1:5000/shrine/whoami@sha256:…`). Every line of `delete application` keeps its current text; the resource lines are the same text with `resource` for `application` and no host-port line.

## Command surface

```text
shrine delete resource <name> [-t <team>] [--dry-run]
```

No alias (none on `delete application`). Exactly one positional argument; Cobra's `accepts 1 arg(s)` error otherwise. Flags: `-t, --team string` ("Team owning the resource (searched automatically when omitted)"), `--dry-run` ("Print what would be released without changing state").

`Short`: `Delete a resource from state and release its image pin`.

`Long`:

```text
Forget a resource: release its image pin and drop its stale deployment
record. The resource's container must already be torn down — Docker state is
authoritative and a live container blocks the delete.
```

The `delete` parent's help lists `application`, `resource`, `team`.

## Success, pin and record held (stdout, exit 0)

```text
Released image pin for shrine-deploy-test/cache-pinned.
Removed deployment record for shrine-deploy-test/cache-pinned.
```

Record only: the second line alone. Pin only: the first line alone.

## Nothing held (stdout, exit 0)

Team resolved (with `--team`, or found by search):

```text
Nothing to delete for resource "cache-pinned" in team "shrine-deploy-test".
```

No team holds anything for the name (no `--team`):

```text
Nothing to delete for resource "cache-pinned".
```

## Dry run (stdout, exit 0, nothing written)

```text
[dry-run] would release image pin <pinned> for shrine-deploy-test/cache-pinned
[dry-run] would remove deployment record for shrine-deploy-test/cache-pinned
```

Nothing held: `[dry-run] nothing to delete for resource "cache-pinned" in team "shrine-deploy-test"`. The no-team case prints the non-dry-run `Nothing to delete for resource "<name>".` line, as `delete application` does today (the team resolution happens before the dry-run branch).

## Refusal, container exists (stderr, exit 1, nothing written)

```text
Error: resource "shrine-deploy-test/cache-pinned" still has a container; run "shrine teardown shrine-deploy-test" first
```

Applies under `--dry-run` too.

## Refusal, ambiguous name (stderr, exit 1, nothing written)

```text
Error: ambiguous: resource "cache" found in teams [demo, media], use --team to disambiguate
```

Teams sorted. Applies under `--dry-run` too.

## Failures (stderr, exit 1)

Shape, wrapped as today: `listing image pins: …`, `listing host port allocations: …` (applications only), `releasing image pin for <team>/<name>: …`, `removing deployment record for <team>/<name>: …`. A container runtime that cannot be reached fails inside `app.NewQueryContainerBackend` before the handler runs, with that error.

## Unchanged

`delete application`: every line as today. `delete team`: every line as today, including `Released N image pin(s) for team "<team>".` from T3. No other command's output changes.
