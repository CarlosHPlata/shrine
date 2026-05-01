# Contract: `routing.configure` event — `aliases` field shape

**Feature**: 008-alias-strip-prefix
**Audience**: operators reading deploy logs; integration test authors; downstream log scrapers (if any)

This contract defines the externally-visible shape of the `aliases` field on the `routing.configure` deploy event. It is the canonical reference for FR-010 in `spec.md` and the basis for the log-line assertion in `engine_aliases_test.go`.

---

## Event surface (existing, unchanged)

The `routing.configure` event is emitted once per application deploy that reaches the routing-write step. Existing fields:

| Field | Source | Example |
|---|---|---|
| `domain` | `application.Spec.Routing.Domain` | `finances.home.lab` |
| `port` | `application.Spec.Port` | `3000` |
| `aliases` | `formatAliasesForLog(aliasRoutes)` | see below |

The `aliases` field is present only when `len(application.Spec.Routing.Aliases) > 0` (`internal/engine/engine.go:172`). This contract describes only the value of that field.

---

## Per-entry shape

Each alias contributes one entry to the comma-joined `aliases` field value. The entry shape is:

```text
<host>[+<pathPrefix>][ (no strip)]
```

Read literally — the `+`, the parentheses, and the leading space before `(no strip)` are all literal characters in the output, not metasyntax.

### Cases

| Alias state | Entry |
|---|---|
| `pathPrefix == ""` (any `stripPrefix`) | `<host>` |
| `pathPrefix != ""`, `stripPrefix == true` (default or explicit) | `<host>+<pathPrefix>` |
| `pathPrefix != ""`, `stripPrefix == false` | `<host>+<pathPrefix> (no strip)` |

The `pathPrefix` value in the entry is the *normalized* form — trailing `/` already stripped by `resolveAliasRoutes` before `formatAliasesForLog` sees it.

---

## Aggregate shape

```text
aliases = sort_ascending(per_entry_strings) joined with ","
```

The sort is lexicographic ascending over the rendered per-entry string (so the `(no strip)` marker affects ordering — see worked examples below).

---

## Worked examples

### Example 1 — single host-only alias

Input alias list:

```yaml
aliases:
  - host: lan.home.lab
```

Log field value: `lan.home.lab`

### Example 2 — single stripping alias (current default)

```yaml
aliases:
  - host: gateway.tail9a6ddb.ts.net
    pathPrefix: /finances
    # stripPrefix omitted → defaults to true
```

Log field value: `gateway.tail9a6ddb.ts.net+/finances`

The shape is byte-identical to what spec 006 emitted; FR-009 / SC-004 require this remain unchanged.

### Example 3 — single non-stripping alias (the issue #9 fix)

```yaml
aliases:
  - host: gateway.tail9a6ddb.ts.net
    pathPrefix: /finances
    stripPrefix: false
```

Log field value: `gateway.tail9a6ddb.ts.net+/finances (no strip)`

### Example 4 — three aliases, mixed strip

```yaml
aliases:
  - host: lan.home.lab
  - host: gateway.tail9a6ddb.ts.net
    pathPrefix: /notes
  - host: gateway.tail9a6ddb.ts.net
    pathPrefix: /notes-raw
    stripPrefix: false
```

Per-entry strings before sort:

- `lan.home.lab`
- `gateway.tail9a6ddb.ts.net+/notes`
- `gateway.tail9a6ddb.ts.net+/notes-raw (no strip)`

After lexicographic sort:

- `gateway.tail9a6ddb.ts.net+/notes`
- `gateway.tail9a6ddb.ts.net+/notes-raw (no strip)`
- `lan.home.lab`

Log field value: `gateway.tail9a6ddb.ts.net+/notes,gateway.tail9a6ddb.ts.net+/notes-raw (no strip),lan.home.lab`

### Example 5 — `stripPrefix: false` on a host-only alias (no-op)

```yaml
aliases:
  - host: gateway.x.y
    stripPrefix: false   # no-op, no pathPrefix to strip
```

Log field value: `gateway.x.y`

The marker is suppressed because `pathPrefix == ""` — `stripPrefix` has no observable behavior, so the log line does not pretend it does. This matches the spec's edge case ("`stripPrefix: false` on an alias with no `pathPrefix`: ... no error, no marker change").

---

## Compatibility guarantees

- **Default-strip and host-only aliases produce byte-identical log lines compared to the prior release.** No leading space, no parenthetical, no extra characters. Existing `TestFormatAliasesForLog` cases (`internal/engine/engine_aliases_test.go:70-105`) MUST keep passing without modification — the new test case is *additive* only.
- **The marker is suffix-only on the offending entry.** It does not change the `,` separator, the `+` separator, or the sort order rule.
- **Sort is over the rendered string including the marker.** This means a `(no strip)` entry collates after the same-host stripped variant under lexicographic ordering (because `(` (0x28) sorts after `+` (0x2B) — actually `(` < `+`, see implementation note below).

### Implementation note on sort order

When two entries share a common prefix and one is strictly longer (because it carries the `(no strip)` suffix), Go's lexicographic string comparison places the shorter string first — the comparison is settled by length once one string runs out of characters, regardless of what the longer string holds beyond that point. Therefore: `gateway.x+/A` < `gateway.x+/A (no strip)`. The unit test for FR-010 MUST pin this ordering with an explicit case so accidental sort changes are caught.

---

## Test obligations

The FR-010 unit test additions to `internal/engine/engine_aliases_test.go` MUST cover:

1. **Single non-stripping alias** (Example 3) — the canonical issue-#9 case. Asserts the `(no strip)` marker appears on the only entry.
2. **Mixed strip across multiple aliases** (Example 4) — asserts marker placement is per-entry, not per-aggregate, and that sort ordering pins the documented shape.
3. **`stripPrefix: false` on a host-only alias** (Example 5) — asserts no marker is emitted when `pathPrefix == ""`. This protects against an over-eager implementation that gates on `!r.StripPrefix` alone without checking `r.PathPrefix != ""`.

The existing three test cases in `TestFormatAliasesForLog` (one alias no prefix, one alias with prefix, multiple sorted) MUST continue to pass character-for-character — they pin the SC-004 / FR-009 byte-stability guarantee for the default-strip path.
