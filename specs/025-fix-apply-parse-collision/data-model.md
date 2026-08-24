# Data Model: Strict Apply Failures and Scoped Routing-Collision Detection

**Feature**: `025-fix-apply-parse-collision` | **Date**: 2026-08-24

No persisted schema changes. This document captures the in-memory concepts the feature manipulates and the rules that govern them.

## Entities

### ManifestSet (existing — `internal/planner`)

| Field | Type | Notes |
|-------|------|-------|
| `Applications` | `map[name]*manifest.ApplicationManifest` | Keyed by `metadata.name`; each carries `Metadata.Owner`, `Spec.Routing.Domain`, `Spec.Routing.PathPrefix`, `Spec.Routing.Aliases[]{Host, PathPrefix, TLS}` |
| `Resources` | `map[name]*manifest.ResourceManifest` | Never routing participants |

Loaded by `LoadDir` (full directory) or assembled by `NewManifestSet` + `MergeManifest` (single-file apply merges into the loaded directory; duplicates keep the directory copy).

### Route key (existing — `internal/planner/collisions.go`)

`routeKey{host, pathPrefix}` where `pathPrefix` is normalised by trimming trailing `/`. An application claims one key per primary domain (when non-empty) and one per alias. The `tls` flag is not part of the key (spec 012 FR-006).

### Routing footprint

The set of every route key claimed by every application in the ManifestSet, each mapped to its **first claimant** in sorted `owner/name` order. This is the reference every scoped check compares against — always the full loaded set, regardless of filter (spec FR-011).

### Filter (existing — `internal/planner/filter.go`) and its scope predicate (new)

| `Filter.Kind` | Produced by | `isAppInScope(app)` |
|---------------|-------------|---------------------|
| `FilterNone` | bare `shrine deploy` / `--dry-run` | `true` for every application |
| `FilterTeam{Name}` | `shrine deploy team <name>` / `--dry-run` | `app.Metadata.Owner == Name` (exact match, same rule as `filterStepsByOwner`) |
| `FilterApp{Name}` | `shrine apply -f <Application>` | `app.Metadata.Name == Name` |
| `FilterRes{Name}` | `shrine apply -f <Resource>` | `false` — resources have no routes, so no application is in scope |

`isAppInScope` is the **only** definition of "in scope" for collision purposes; step emission continues to use the existing filter logic, and the two must agree by construction (both derive from the same `Filter` value).

### Collision pair

| Attribute | Value |
|-----------|-------|
| `key` | the shared `routeKey` |
| `first` | `owner/name` of the first claimant (from the footprint) |
| `later` | `owner/name` of the later claimant encountered in the sorted walk |
| **Reported iff** | `isAppInScope(first) || isAppInScope(later)` |
| Diagnostic line | `routing collision: host=%q pathPrefix=%q declared by %q and %q` with the two refs in lexical order |

All reported lines are sorted and joined under `routing validation failed:\n- …` — unchanged from spec 016.

**Outcome matrix** (which collisions fail which command):

| Participants | bare `deploy` | `deploy team T` | `apply -f X` | `apply -f <Resource>` |
|--------------|---------------|-----------------|--------------|-----------------------|
| both owned by T (or X is one of them) | fail | fail | fail (if X involved) | pass |
| one owned by T / X, other elsewhere | fail | fail | fail (if X involved) | pass |
| neither owned by T / X not involved | fail | **pass** | **pass** | pass |
| no shared route keys | pass | pass | pass | pass |

Bold cells are the behaviour changes; every other cell is today's behaviour (the `apply -f` column was previously "pass" for all rows — the closed gap).

## `apply teams` run — state machine

```
                ┌──────────┐
                │  ScanDir │── malformed YAML / walk error ──▶ FAIL (exit 1, stderr)  [unchanged]
                └────┬─────┘
                     │ candidates (shrine-classified) + foreign paths
                     ▼
        ┌────────────────────────────┐
        │ collectTeamManifests       │  per candidate, in scan order:
        │  Parse ──✗──▶ errs += "parsing manifest %q: %v"
        │  Kind ≠ Team ──▶ stdout: "Skipping %s: not a Team manifest"
        │  Validate ──✗──▶ errs += "validating manifest %q: %v"
        │  ok ──▶ teams += m.Team
        └────────────┬───────────────┘
                     │
        len(errs) > 0 ? ──yes──▶ FAIL: "apply teams failed:\n- …"   (no SaveTeam calls; state untouched)
                     │ no
                     ▼
        ┌────────────────────────────┐
        │ saveTeams                  │  per team:
        │  SaveTeam ──✗──▶ FAIL: "saving team %q to state: %w"  (earlier teams in this run remain saved)
        │  ok ──▶ stdout: "Synced team: %s"
        └────────────┬───────────────┘
                     ▼
        stdout: "Successfully synced %d teams to state."
        stdout: foreign-file notice (if any)          ──▶ SUCCESS (exit 0)
```

Zero candidates short-circuits to today's `No team manifests found in %q directory.` + exit 0 before `collectTeamManifests`.

### Aggregated failure record

| Field | Content |
|-------|---------|
| header | `apply teams failed:` |
| bullets | one per failing candidate, scan order: `parsing manifest "<path>": <cause>` or `validating manifest "<path>": <cause>` |
| channel | returned `error` → Cobra → stderr; process exit 1 |

`<cause>` for a bad kind is `unknown manifest kind: "Aplication"` (from `manifest.Parse`), so the bullet carries both the file path and the offending kind value as spec 002 FR-003 requires.

## Invariants

1. **No partial team sync**: `SaveTeam` is never called if any candidate failed to parse/validate in the same run.
2. **Footprint is always the full set**: scoping narrows which pairs are *reported*, never which routes are *considered*.
3. **Diagnostic stability**: for `FilterNone`, the error text of `DetectRoutingCollisions(set, NoFilter())` is byte-identical to today's `DetectRoutingCollisions(set)`.
4. **Resources never collide**: `FilterRes` is a guaranteed no-op for routing collisions.
5. **Scope and emission agree**: any application for which a deploy step would be emitted is in scope for collision detection, and vice versa.
