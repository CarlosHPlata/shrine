# Phase 1 Data Model: Per-Alias Opt-Out of Path Prefix Stripping

**Feature**: 008-alias-strip-prefix
**Date**: 2026-05-01
**Status**: No new data model. This document records the existing types this feature reads (introduced under spec 006) and the resolution rule the audit confirmed, so future readers do not relitigate the contract.

---

## Existing types (no change under this feature)

### `manifest.RoutingAlias` — manifest-side

Defined in `internal/manifest/types.go:39-43`:

```go
type RoutingAlias struct {
    Host        string `yaml:"host"`
    PathPrefix  string `yaml:"pathPrefix,omitempty"`
    StripPrefix *bool  `yaml:"stripPrefix,omitempty"`
}
```

The `StripPrefix` field is a `*bool` (not `bool`) so the parser can distinguish three operator-meaningful states:

| Operator wrote | Parser state | Meaning |
|---|---|---|
| nothing | `StripPrefix == nil` | "default — Shrine, you choose" |
| `stripPrefix: true` | `StripPrefix == &true` | "strip the prefix even though I'm being explicit about it" |
| `stripPrefix: false` | `StripPrefix == &false` | "do not strip the prefix" |

Validation accepts all three states without error (FR-006, evidence: `internal/manifest/validate_test.go:306-313`).

### `engine.AliasRoute` — engine-side projection

Defined in `internal/engine/backends.go:54-58`:

```go
type AliasRoute struct {
    Host        string
    PathPrefix  string
    StripPrefix bool
}
```

Note the engine-side type uses a plain `bool`, not `*bool`. The default-resolution rule below converts the manifest-side `*bool` to a concrete decision before the engine acts on it.

---

## Default-resolution rule (existing, audited under this feature)

Defined in `internal/engine/engine.go:305-322` (`resolveAliasRoutes`):

```text
prefix := strings.TrimRight(alias.PathPrefix, "/")
if alias.StripPrefix != nil:
    strip = *alias.StripPrefix
else:
    strip = (prefix != "")
```

Rendered as a truth table:

| `alias.PathPrefix` after trim | `alias.StripPrefix` | Engine `AliasRoute.StripPrefix` | Renderer behavior |
|---|---|---|---|
| `""` (empty / absent / bare `/`) | nil | `false` | no strip middleware |
| `""` | `&true` | `true` | no strip middleware (`PathPrefix=="" `gate in routing.go:61) |
| `""` | `&false` | `false` | no strip middleware |
| `"/finances"` (or any non-empty) | nil | `true` | **strip middleware emitted** |
| `"/finances"` | `&true` | `true` | **strip middleware emitted** |
| `"/finances"` | `&false` | `false` | no strip middleware |

The renderer gate at `internal/plugins/gateway/traefik/routing.go:61` (`if ar.StripPrefix && ar.PathPrefix != ""`) means the `*bool` value is irrelevant whenever `PathPrefix` is empty — `stripPrefix: true` on a host-only alias is a no-op, matching the spec's edge case (SC-004 byte-stability for unaffected manifests).

---

## State transitions

Stateless. Alias routes are reconciled by Traefik's file watcher whenever Shrine writes the dynamic-config file. There is no in-memory cache of alias state, no DB row, no `DeploymentStore` entry for routing config. Per Constitution Principle VI, Docker (and here, the Traefik file provider) is the source of truth — Shrine writes the YAML and is done.

This means FR-007 ("changing `stripPrefix` and re-deploying must remove orphaned middleware") is structurally satisfied by `WriteRoute` (`internal/plugins/gateway/traefik/routing.go:37-95`) regenerating the entire YAML from `op.AdditionalRoutes` on every call. There is no "patch" code path; the file is wholly rewritten each deploy.

---

## Refactor decision (carried from research.md Decision 3)

The FR-010 implementation extends `formatAliasesForLog` (`internal/engine/engine.go:324-335`). The function stays as a single named function — we do NOT extract `formatOneAliasEntry` or `joinAliasEntries` helpers — because the function has one call site. If a future change needs a third or higher caller, extract then; not now (Constitution IV).

The post-modification function shape:

```text
function formatAliasesForLog(routes):
    entries = []
    for r in routes:
        entry = r.Host
        if r.PathPrefix != "":
            entry += "+" + r.PathPrefix
        if r.PathPrefix != "" and !r.StripPrefix:
            entry += " (no strip)"
        entries.append(entry)
    sort(entries)
    return join(entries, ",")
```

Two readability invariants the implementation MUST preserve:

1. The existing comma-joined shape for default-strip aliases is byte-identical to the prior release (no leading space, no extra fields). This is what makes FR-009 / SC-004 hold.
2. The marker is appended to the entry *before* sorting, which means an entry like `gateway.x+/finances (no strip)` sorts after `gateway.x+/finances` — this is fine and intuitive (the unmarked entries collate first when both shapes coexist), but tests should pin a representative ordering so accidental sort changes are caught.

---

## Marker contract (referenced by `contracts/log-format.md`)

| Alias state | Per-entry log shape |
|---|---|
| `pathPrefix=""` (any `stripPrefix`) | `<host>` |
| `pathPrefix="/X"`, `stripPrefix=true` (default or explicit) | `<host>+/X` |
| `pathPrefix="/X"`, `stripPrefix=false` | `<host>+/X (no strip)` |

The aggregate (event field `aliases`) is always the comma-joined sorted-ascending list of entries. See `contracts/log-format.md` for the full contract and worked examples.
