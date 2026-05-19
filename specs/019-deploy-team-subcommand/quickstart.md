# Quickstart: `shrine deploy team` + Unified Planner Filter

Two audiences here:

- **Operators** (sections 1–3): how to use the new `shrine deploy team` verb.
- **Contributors** (sections 4–6): how to verify the planner-consolidation
  refactor locally and what to grep for when extending it.

---

## 1. Operator setup

You already have a Shrine specs directory with manifests owned by multiple
teams. Confirm with:

```bash
grep -hE '^\s*owner:' ~/shrine/specs/**/*.yml | sort -u
# expected output, e.g.:
#   owner: marketing
#   owner: ops
#   owner: platform
```

If only one team owns manifests, `shrine deploy team <only-team>` is
functionally identical to `shrine deploy`. The interesting cases are
multi-team directories.

## 2. Deploy a single team

Plan and apply just `team-a`'s stack:

```bash
shrine deploy team team-a
# [shrine] Planning deployment for team "team-a" from: /home/operator/shrine/specs
# ... (steps for team-a-owned apps and resources only)
```

Other teams' containers are untouched. Verify with `shrine status`:

```bash
shrine status
# team-a   app   alpha    RUNNING   (just reconciled)
# team-b   app   beta     RUNNING   (untouched — same container id as before)
```

## 3. Preview with `--dry-run`

Same safety guarantee as today's `shrine deploy --dry-run` — no side
effects on Docker, routing files, or state:

```bash
shrine deploy team team-a --dry-run
# [shrine] Planning deployment for team "team-a" from: /home/operator/shrine/specs
# [PLAN]   resource db (team-a) ...
# [PLAN]   application alpha (team-a) ...
# (no [DOCKER] lines, no [ROUTE] writes)
```

Mistype the team name to see the error UX (FR-008, SC-005):

```bash
shrine deploy team markting
# Error: team "markting" not found in specs directory: known teams = [marketing, ops, platform]
# exit code 1
```

The error includes the typo *and* the list of teams that exist, so you can
correct it without grep'ing manifests.

---

## 4. Contributor: the planner contract after this feature

Single entry point in `internal/planner`:

```go
result := planner.Plan(set, store, registries, filter)
```

where `filter` is one of:

```go
planner.NoFilter()                  // bare `shrine deploy`
planner.ByTeam("team-a")            // `shrine deploy team team-a`
planner.ByApp("alpha")              // `shrine apply -f alpha.yaml` (Application kind)
planner.ByResource("db")            // `shrine apply -f db.yaml` (Resource kind)
```

`planner.PlanSingle` no longer exists. A grep across the repo:

```bash
grep -r "PlanSingle" internal/ cmd/ tests/
# expected: zero matches (SC-007)
```

## 5. Verify locally

After pulling the branch:

```bash
# Unit tests for the planner — must pass.
go test ./internal/planner/... ./internal/handler/...

# Build the binary.
go build -o /tmp/shrine ./

# Smoke: bare deploy (regression gate — SC-003).
/tmp/shrine deploy --dry-run --path ./testdata/multi-team-specs

# Smoke: team-scoped deploy (new behavior).
/tmp/shrine deploy team team-a --dry-run --path ./testdata/multi-team-specs

# Smoke: apply -f (regression gate — SC-004).
/tmp/shrine apply -f ./testdata/multi-team-specs/team-a/alpha.yaml --dry-run

# Final gate: integration suite.
make test-integration

# Docs regen — confirms cobra-tree → CLI reference is in sync.
make docs-gen-cli
git diff --exit-code docs/content/cli/
# expected: empty diff. A non-empty diff means committed CLI docs are stale.

# Hugo build — confirms hand-written content compiles.
make docs
```

Expected: all smokes exit 0; integration suite passes including the new
`tests/integration/deploy_team_test.go`; both docs commands exit clean.

## 6. Extending the filter

If a future feature needs a fifth filter case (e.g., `ByLabel`), the diff
surface is small but the abstraction's "three concrete usages" rule
(Principle IV) must be re-evaluated:

1. Add a new `FilterKind` constant in `internal/planner/filter.go`.
2. Add a constructor (`ByLabel(key, val string) Filter`).
3. Extend `Filter.Validate` with the new case.
4. Extend the `switch filter.Kind` in `Plan` with the new step-emission rule.
5. Wire the new constructor to a CLI surface OR a handler call site that has
   a concrete operator need. **Don't add the filter case ahead of a real
   call site** — that's exactly the YAGNI failure the constitution warns
   against.

Today's filter set (`None`, `Team`, `App`, `Res`) is justified because each
has at least one concrete caller at landing time. Adding a fifth without a
caller would be premature.

---

## 7. Where to look when something breaks

| Symptom | First place to check |
|---|---|
| `shrine deploy team X` succeeds but no steps run | `internal/planner/filter.go` — `Filter.Validate(set)` is letting an empty match through |
| `shrine deploy` regresses (steps missing) | `internal/planner/plan.go` — `FilterNone` branch must call `DetectRoutingCollisions` + `Order` + emit every step |
| `shrine apply -f X.yaml` regresses | `internal/handler/apply.go` — `loadSetForSingle` or filter construction is wrong |
| Cross-team `valueFrom` fails under `deploy team` | The `ManifestSet` was filtered before `Resolve`; the contract says only steps are filtered (post-Order). Bug location: `internal/planner/plan.go` |
| Unknown-team error message lacks the "known teams" list | `internal/planner/filter.go` — `discoveredOwners(set)` helper |
| Integration test `deploy_team_test.go` fails on a clean Docker daemon | The test fixtures under `tests/integration/testdata/` must include manifests owned by ≥2 distinct teams |
