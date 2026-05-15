# CLI Contract: `shrine deploy --dry-run` (Routing Collision Detection)

**Phase**: 1 (Design)
**Feature**: 016-fix-dryrun-routing-collisions

This is the only externally observable contract changed by this feature. No new flags, no new commands, no new manifest fields — only the exit-code and diagnostic behavior of an existing CLI invocation under specific manifest inputs.

## Command

```text
shrine deploy --dry-run [--path <manifest-dir>] [--state-dir <dir>]
```

## Inputs

A manifest directory containing two or more `Application` manifests whose `spec.routing.domain` values (or any `spec.routing.aliases[].host` plus `pathPrefix` pair, post-trailing-slash-normalization) collide.

## Behavior — Before This Change (Bug)

| Aspect           | Behavior                                                                          |
|------------------|-----------------------------------------------------------------------------------|
| Exit code        | `0` (clean exit, deceptive)                                                       |
| stdout           | Standard dry-run plan, including the colliding applications as if they were valid |
| stderr           | Empty                                                                             |
| Side effects     | None (as expected for `--dry-run`)                                                |

A subsequent `shrine deploy` (no `--dry-run`) against the same directory exits non-zero with a routing collision diagnostic — the inconsistency between the two paths is the bug.

## Behavior — After This Change (Fixed)

| Aspect           | Behavior                                                                          |
|------------------|-----------------------------------------------------------------------------------|
| Exit code        | Non-zero                                                                          |
| stdout           | Standard pre-error log lines (e.g. `[shrine] Planning deployment from: …`)        |
| stderr           | Cobra-rendered error: `Error: routing validation failed:\n- routing collision: …` |
| Side effects     | None                                                                              |

The diagnostic body has the exact shape `DetectRoutingCollisions` already produces:

```text
Error: routing validation failed:
- routing collision: host="example.local" pathPrefix="" declared by "team-one/app-a" and "team-one/app-b"
```

When N independent collisions exist, all N `routing collision: …` lines appear under the single `routing validation failed:` header, sorted alphabetically (per `DetectRoutingCollisions`'s existing `sort.Strings(errs)` step). A single invocation reports all collisions; users do not need to re-run. The diagnostic goes to **stderr** (per Constitution Principle II: "errors → stderr"), matching the pre-fix behavior of `shrine deploy` so existing tooling and tests asserting against stderr continue to work.

## Behavior — Unchanged Paths

| Input                                                  | Exit code | Output            |
|--------------------------------------------------------|-----------|-------------------|
| Manifest set with no duplicate `routing.domain` values | `0`       | Standard dry-run plan |
| Manifest set with malformed YAML                       | non-zero  | Existing parser error |
| Manifest set with other `Resolve` failures             | non-zero  | Existing `Validation errors:` block |

The real (non-dry-run) `shrine deploy` is unchanged in stderr shape — the collision diagnostic is still returned as a plain `error`, just from `planner.Plan` instead of from the handler's direct `DetectRoutingCollisions` call. Both `Deploy` and `DryRun` now flow through identical code, satisfying SC-001 (parity).

## Backwards Compatibility

- **Manifest schema**: unchanged.
- **CLI flags / commands**: unchanged.
- **Output on collision-free manifests**: unchanged.
- **Exit code on real deploy with collisions**: was non-zero, remains non-zero (the wrapper text framing changes but the failure direction does not). Any tooling parsing exit codes is unaffected; tooling parsing the exact text of the collision message — if any exists outside this repo — would need to know about the new `Validation errors:` header line, but this is an internal CLI not intended for unattended parsing.
