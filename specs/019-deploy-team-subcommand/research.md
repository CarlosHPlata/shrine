# Phase 0 Research: Unified Planner Filter

This phase resolves the API-shape and migration questions that the spec
intentionally deferred to planning. There were no `NEEDS CLARIFICATION` markers
in the spec; the open questions came from the plan's Technical Context.

---

## R-001: Filter representation — tagged struct vs. interface sum-type vs. functional option

**Decision**: **Tagged struct with constructor functions.**

```go
// internal/planner/filter.go
package planner

type FilterKind int

const (
    FilterNone FilterKind = iota
    FilterTeam
    FilterApp
    FilterRes
)

type Filter struct {
    Kind FilterKind
    Name string  // empty when Kind == FilterNone; required otherwise
}

func NoFilter() Filter                { return Filter{Kind: FilterNone} }
func ByTeam(name string) Filter       { return Filter{Kind: FilterTeam, Name: name} }
func ByApp(name string) Filter        { return Filter{Kind: FilterApp, Name: name} }
func ByResource(name string) Filter   { return Filter{Kind: FilterRes, Name: name} }
```

**Rationale**:

1. **Consistency with existing code.** `internal/planner.PlannedStep` is already
   a tagged struct: `{Kind: string, Name: string}`. Using the same shape for
   `Filter` makes the package's mental model uniform.
2. **Switch readability.** Filter handling lives in one `switch f.Kind` in
   `Plan()` — easy to grep, easy to extend, no interface plumbing.
3. **Zero value is safe.** `Filter{}` is `FilterNone` with empty name, which is
   the "all manifests" mode. Useful for default values and tests.
4. **Constructor validation point.** `NoFilter()` / `ByTeam(name)` /
   `ByApp(name)` / `ByResource(name)` are the only public way to build a Filter
   in idiomatic call sites. They give a single place to add empty-string checks
   if needed (we delegate runtime validation to `Filter.Validate(set)` to keep
   constructors trivial).

**Alternatives considered**:

- **Interface sum-type** (`type Filter interface{ isPlanFilter() }` + four
  concrete types). Rejected: ceremony without benefit. Type switches replace
  `switch f.Kind` one-for-one; no compile-time exhaustiveness in Go anyway.
- **Functional options** (`Plan(set, store, registries, WithTeam("x"))`).
  Rejected: filter is a single-valued choice, not a composition of orthogonal
  knobs. Functional options shine when you're combining N independent settings;
  here it would obscure that exactly one of four modes is selected.
- **Separate functions per mode** (`PlanAll`, `PlanForTeam`, `PlanSingleApp`,
  `PlanSingleResource`). Rejected: that's where we started; the spec exists to
  collapse this duplication (FR-001..003).

---

## R-002: Where does the "load directory and merge an extra file" logic live after `PlanSingle` is deleted?

**Decision**: **Expose `ManifestSet.MergeManifest(m, path) error` as a public method**
(today's private `mapKind`), and let `handler.ApplySingle` compose `LoadDir +
MergeManifest` itself when it needs the file's manifest in the set.

**Rationale**:

1. `mapKind` already does the right thing — it's named badly (it does more than
   "map a kind") and it's private only because nothing outside the package
   needed it before. Promoting and renaming it is a one-line public API
   addition.
2. The composition belongs in the handler, not the planner. The planner's
   contract becomes "you give me a set + filter, I give you a plan." The
   handler knows whether it has a directory + file (today's `apply -f --path
   somedir`) or only a file (today's `apply -f file.yaml`), and assembles the
   set accordingly.
3. This keeps `Plan()` purely a function of `(set, store, registries, filter)`
   with no I/O. That makes it trivial to unit-test (FR-014).

**Concrete handler pseudocode** (replaces today's `planner.PlanSingle`):

```go
// internal/handler/apply.go
func ApplySingle(b *app.ApplyBundle, file, manifestDir string) error {
    m, err := manifest.Parse(file)
    if err != nil { return fmt.Errorf("parsing manifest %q: %w", file, err) }
    if err := manifest.Validate(m); err != nil {
        return fmt.Errorf("validating manifest %q: %w", file, err)
    }

    set, err := loadSetForSingle(file, manifestDir, m)
    if err != nil { return err }

    var filter planner.Filter
    switch m.Kind {
    case manifest.ApplicationKind:
        filter = planner.ByApp(m.Application.Metadata.Name)
    case manifest.ResourceKind:
        filter = planner.ByResource(m.Resource.Metadata.Name)
    case manifest.TeamKind:
        return fmt.Errorf("team manifests cannot be applied with --file; use 'shrine apply teams' instead")
    default:
        return fmt.Errorf("unsupported manifest kind %q for single-file apply", m.Kind)
    }

    result := planner.Plan(set, b.Store.Teams, b.Cfg.Registries, filter)
    // ...rest identical to today's ApplySingle tail
}

func loadSetForSingle(file, manifestDir string, m *manifest.Manifest) (*planner.ManifestSet, error) {
    if manifestDir == "" {
        set := planner.NewManifestSet()
        if err := set.MergeManifest(m, file); err != nil { return nil, err }
        return set, nil
    }
    set, err := planner.LoadDir(manifestDir)
    if err != nil { return nil, err }
    if err := set.MergeManifest(m, file); err != nil && !errors.Is(err, planner.ErrDuplicateManifest) {
        return nil, err
    }
    return set, nil
}
```

`NewManifestSet()` is a thin constructor for the empty case; today's tests
already build `ManifestSet{Applications: ..., Resources: ...}` inline, so
exposing a constructor is a small ergonomic win that keeps the struct literal
out of consumer code.

**Alternatives considered**:

- **Keep loading inside `Plan(dir, ...)`**. Rejected: this re-introduces the
  fork between "Plan reads a directory" and "PlanSingle parses one file" inside
  `Plan` itself — exactly what the refactor is collapsing.
- **Add a separate helper `planner.LoadFileWithContext(file, dir)`**. Rejected:
  the composition is two lines (LoadDir + MergeManifest); wrapping it doesn't
  earn its keep. Three concrete callers might justify it later — YAGNI now.

---

## R-003: Filter validation — where and when

**Decision**: **`Filter.Validate(set *ManifestSet) error` runs as the first
step inside `Plan()`**, before `Resolve`. On failure, `Plan` returns
`PlanResult{Error: err}` with the spec's unknown-team error format (FR-008).

```go
// internal/planner/filter.go
func (f Filter) Validate(set *ManifestSet) error {
    switch f.Kind {
    case FilterNone:
        return nil
    case FilterTeam:
        if f.Name == "" { return errors.New("team filter requires a non-empty name") }
        if hasOwner(set, f.Name) { return nil }
        owners := discoveredOwners(set) // sorted slice
        if len(owners) == 0 {
            return fmt.Errorf("team %q not found: specs directory contains no Application or Resource manifests", f.Name)
        }
        return fmt.Errorf("team %q not found in specs directory: known teams = %v", f.Name, owners)
    case FilterApp:
        if _, ok := set.Applications[f.Name]; !ok {
            return fmt.Errorf("application %q not found in manifest set", f.Name)
        }
        return nil
    case FilterRes:
        if _, ok := set.Resources[f.Name]; !ok {
            return fmt.Errorf("resource %q not found in manifest set", f.Name)
        }
        return nil
    default:
        return fmt.Errorf("unknown filter kind: %v", f.Kind)
    }
}
```

**Rationale**:

1. **One validation point.** Every `Plan(set, ..., filter)` call goes through
   the same gate; no per-handler duplication.
2. **Same error surface as `PlanResult.Error`.** Today's `Plan` and `PlanSingle`
   both put validation failures in `PlanResult.Error` (vs. `ValidationErr`
   which is for per-manifest errors from `Resolve`). Keeping that distinction
   means the error-rendering code in handlers and tests doesn't need to change.
3. **Fast failure.** The unknown-team check is O(K) on the manifest set, much
   cheaper than `Resolve`. Running it first means `shrine deploy team typo`
   exits before doing any quota or template work — matches the operator's
   mental model.
4. **Discoverability.** Putting "known teams" in the error message (FR-008,
   SC-005) lives in one place: `discoveredOwners(set)`. Tests target this one
   helper.

**Alternatives considered**:

- **Validate in the constructors** (`ByTeam("") → panic`). Rejected: the
  constructors don't have a `ManifestSet` available; emptiness vs. existence
  are two different checks. Constructors are trivial; validation needs context.
- **Validate inside the Cobra layer** (`cmd/deploy.go` checks the team exists
  before calling the handler). Rejected: duplicates `LoadDir` work and pulls
  planner concerns into `cmd/`. Principle II: "Commands MUST be thin
  dispatchers."

---

## R-004: Cross-team dependency handling under team-scoped filter

**Decision**: **No new code path.** The team filter only affects which steps
are *emitted*; `Resolve()` is called over the full `ManifestSet` exactly as
today. All cross-team checks (`resolveDependencies`, `validateValueFrom`,
`hasAccess`, `networking.exposeToPlatform`) run unchanged.

**Rationale**:

- The spec's Session-2026-05-18 Clarification #1 locked this in: cross-team
  resolution uses the existing rules in `resolve.go`, no new plan-time
  deployment-state lookup. The plan implements that decision by literally
  changing nothing about `Resolve()`.
- The only new behavior is post-`Order` step filtering, which is one helper:

  ```go
  func filterStepsByOwner(steps []PlannedStep, set *ManifestSet, owner string) []PlannedStep {
      out := make([]PlannedStep, 0, len(steps))
      for _, s := range steps {
          if stepOwner(set, s) == owner {
              out = append(out, s)
          }
      }
      return out
  }
  ```

**Alternatives considered**:

- **Pre-filter the `ManifestSet` before Resolve** (drop other teams' manifests
  from the set entirely). Rejected: would break cross-team `valueFrom`
  references because `validateValueFrom` walks `set.Resources` looking up the
  referenced name. The team-scoped flow needs the *full* set for resolution
  and only filters at the end. This is exactly the trade-off captured in the
  spec's Key Entities entry.

---

## R-005: Routing collision detection scope under team-scoped filter

**Decision**: **Collision detection runs over the full `ManifestSet`**, not
the team-scoped subset. Matches today's `DetectRoutingCollisions(set)` call
verbatim.

**Rationale**:

- FR-010 requires that a team-scoped deploy cannot introduce a route that
  collides with another team's route. The only way to detect that is to walk
  the full routing footprint.
- The cost is O(K) — same as bare `deploy`. No new work; we just don't
  short-circuit when the filter narrows the step set.

**Alternatives considered**:

- **Skip collision detection for App/Res filters** (today's `PlanSingle`
  skips it). Kept that behavior — apply -f is incremental and never enabled
  collision checks. The branch is `if filter.Kind == FilterNone ||
  filter.Kind == FilterTeam { detectAndOrder }` else single-step emission.

---

## R-006: `PlanResult` shape

**Decision**: **Unchanged.** Today's struct already carries `Steps`,
`ManifestSet`, `Error`, `ValidationErr` — those are exactly the four pieces
every caller needs. No new fields.

```go
// internal/planner/plan.go (existing — UNCHANGED)
type PlanResult struct {
    Steps         []PlannedStep
    ManifestSet   *ManifestSet
    Error         error
    ValidationErr []error
}
```

**Rationale**: Both `handler.Deploy` and `handler.ApplySingle` read these
exact fields today. Keeping the result shape stable means the handler
diffs are minimal (one line: the planner call).

---

## R-007: Output header phrasing for `shrine deploy team`

**Decision**: **`[shrine] Planning deployment for team %q from: %s`** (FR-013).

**Rationale**: Mirrors today's `[shrine] Planning deployment from: %s` shape;
quoting `%q` on the team name makes typos and whitespace visible. Lives in
`cmd/deploy.go` (Cobra layer) so the handler's signature doesn't bloat to
carry a label.

---

## R-008: Test scope split — unit vs. integration

**Decision**:

| Layer | Test file | What it gates |
|---|---|---|
| Unit | `internal/planner/filter_test.go` | Filter constructors; `Validate(set)` happy + sad paths for each Kind; unknown-team error includes sorted "known teams" list. |
| Unit | `internal/planner/plan_test.go` | `Plan(set, ..., NoFilter())` ≡ today's `Plan` (full set, ordered, collision-checked). `Plan(set, ..., ByApp/ByResource)` ≡ today's `PlanSingle` (one step, no collision). `Plan(set, ..., ByTeam)` emits only owner-matching steps; cross-team resolution still succeeds. |
| Unit | `internal/handler/apply_test.go` | `ApplySingle` constructs the right Filter from each manifest kind; Team-kind file rejected with today's exact error message. |
| Integration | `tests/integration/deploy_team_test.go` | `shrine deploy team team-a` in a two-team specs dir: team-b containers untouched. Unknown-team produces non-zero exit + "known teams" error. Dry-run produces zero side effects. |
| Integration | `tests/integration/deploy_test.go` | UNCHANGED. Passing this unmodified is SC-003. |
| Integration | `tests/integration/apply_*_test.go` | UNCHANGED. Passing these unmodified is SC-004. |

**Rationale**: The unit tier proves the abstraction (the filter switch
behaves correctly for each mode). The integration tier proves the end-to-end
operator experience and gates regressions on the two pre-existing call
sites. No mocks; the integration tier uses the existing `NewDockerSuite`
harness against the real binary, per Principle V.

---

## R-009: Documentation scope — what's auto-generated vs. hand-written

**Decision**: **Split docs work along the existing auto-gen boundary.**

- Anything under `docs/content/cli/` is regenerated by `make docs-gen-cli`,
  which walks the live Cobra tree. Authors NEVER edit those files by hand —
  the header `<!-- AUTO-GENERATED ... DO NOT EDIT BY HAND -->` is present in
  every file. After adding `deployTeamCmd` in `cmd/deploy.go`, one
  `make docs-gen-cli` run produces the new `deploy_team.md` and refreshes
  `deploy.md`'s SEE ALSO list.
- `docs/public/` is the Hugo-built output, served by GitHub Pages. It's
  committed because the project uses a `gh-pages`-from-`main` flow (no
  separate publish branch). `make docs` rebuilds it after content changes.
- Hand-written content lives under `docs/content/{getting-started,guides}/`.
  This is where the *workflow* and *decision rationale* are explained — what
  the auto-gen pages cannot say.

**Hand-written deliverables, justified per file**:

| File | Why it's necessary |
|---|---|
| `docs/content/getting-started/quick-start.md` | The quick-start is the first surface a new operator reads. Omitting the verb here means new users discover it only via `--help` or by grep'ing the changelog. A 3–5-line callout is enough; no full rewrite. |
| `docs/content/guides/team-scoped-deploy.md` (NEW) | The cross-team-dependency rule (Clarification 1) and the typo-error UX (SC-005) are non-obvious. The auto-gen page just lists flags; the guide explains the *why*. Mirrors the granularity of existing guides like `docs/content/guides/custom-registries.md` (~100 lines). |
| `docs/content/guides/_index.md` | Hugo sidebar/index needs the new entry so the guide is discoverable. One link addition. |
| `AGENTS.md` | The constitution's Governance section mandates `AGENTS.md` stays consistent with CLI changes. The repo's existing quick-reference table already lists `shrine deploy`; we add a row for `shrine deploy team <name>`. |

**Hand-written deliverables explicitly NOT touched** (kept short because the
question "should I update this too?" will come up at review time):

| File | Reason for skipping |
|---|---|
| `docs/content/guides/traefik.md`, `tls.md`, `custom-registries.md`, `secrets-vault.md` | They reference `shrine deploy` as a generic verb. The bare verb is unchanged (FR-005); the team subcommand is a strict superset workflow. Updating these pages would be churn without information gain. |
| `README.md` | Today it links out to the docs site rather than enumerating CLI verbs. No update needed. |
| `docs/content/troubleshooting/` | The typo-error UX is already self-explanatory (FR-008 error names known teams). No new troubleshooting entry warranted. |

**Verification step** (added to the contributor smoke list in `quickstart.md`):

```bash
make docs-gen-cli
git diff --exit-code docs/content/cli/
```

A non-empty diff means someone hand-edited a file under `docs/content/cli/`
or the Cobra wiring drifted from the committed docs. Either is a regression.

**Alternatives considered**:

- **Skip the hand-written guide; rely on auto-gen alone**. Rejected: the
  cross-team-dependency rule, the error-UX promise, and the dry-run parity
  guarantee don't fit into the Cobra `Short`/`Long` strings without ruining
  `--help` output. They belong in a dedicated guide page.
- **Add team-scope content to every existing guide that mentions `deploy`**.
  Rejected: those guides are scoped to their respective topics (Traefik,
  TLS, registries, secrets). Sprinkling team-scope context into all of them
  would be noise; one well-placed guide page is the right shape.

---

## Open questions remaining

None. All Technical Context items are resolved; the plan can proceed to
Phase 1 design artifacts.
