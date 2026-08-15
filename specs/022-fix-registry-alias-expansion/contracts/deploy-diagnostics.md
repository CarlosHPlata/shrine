# Contract: Deploy Diagnostics for Container Creation

**Feature**: `022-fix-registry-alias-expansion` | **Date**: 2026-08-14

The operator-facing contract for `shrine deploy` output around container
creation. Integration tests assert against these exact shapes; changing them is
a breaking change to this contract.

## 1. Container-spec contract (the fix itself)

For every container Shrine creates:

- The image reference handed to the container runtime MUST be fully qualified.
  It MUST NOT begin with `reg:`.
- If the manifest used `reg:<alias>/<tail>`, the reference MUST be
  `<configured-host>/<tail>` for the alias's configured host (bare
  `reg:<alias>` expands to the bare host).
- Observable post-hoc via the runtime: inspecting the created container
  reports the expanded reference as its configured image.

## 2. Error contract (`container.create` failure)

**Event** (structured observers):

```
Name:   "container.create"
Status: StatusError
Fields: {
  "name":  "<team>.<resource>",        // container name (existing)
  "image": "<expanded image reference>", // NEW — the reference Docker was given
  "error": "<wrapped error text>",       // existing
}
```

**Wrapped error text** (surfaces in the `❌` line and the process exit error):

```
creating container "<team>.<resource>" (image "<expanded ref>"): <upstream cause>
```

Guarantees:

- The image reference in both the field and the message is the *expanded* form
  — the exact string the runtime rejected, not the manifest's alias form.
- The container name keeps its existing `<team>.<resource>` shape.

## 3. Progress-line contract (terminal output)

For one container-creation attempt, the terminal renderer prints:

| Outcome | Lines |
|---|---|
| Success | `  🏗️  Creating container: <team>.<name>` — exactly once |
| Failure | `  🏗️  Creating container: <team>.<name>` — exactly once, followed by `❌ Error [container.create]: …` line(s) |

Guarantees:

- The `🏗️` progress line renders only for informational status, never as a
  side effect of rendering an error event.
- Every rendered container name contains both segments; a name with a leading
  separator or an empty team segment MUST never appear.
- Successful-deploy output is byte-identical to today's.

## 4. Dry-run contract (unchanged, re-affirmed)

`shrine deploy --dry-run` output presents image references exactly as authored
in the manifest — the `reg:<alias>/…` form is preserved, never expanded. The
existing assertions in `tests/integration/registry_alias_test.go` pin this and
MUST continue to pass unmodified.
