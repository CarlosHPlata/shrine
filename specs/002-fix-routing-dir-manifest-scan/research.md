# Phase 0 Research: Fix Routing-Dir Manifest Scan Crash

All open items from the spec's clarifications and the Technical Context are resolved below. There are **no remaining `NEEDS CLARIFICATION`** entries.

---

## R-001: Where does the classification helper live?

**Decision**: Add a new file `internal/manifest/classify.go` exposing:

```go
type Class int

const (
    ClassShrine  Class = iota // apiVersion matches ^shrine/v\d+([a-z]+\d+)?$
    ClassForeign              // .yaml/.yml file but apiVersion absent or non-matching
)

// Classify reads `path`, returns Class plus the parsed top-level TypeMeta when
// ClassShrine. Malformed YAML returns an error (FR-004). Caller is responsible
// for the .yaml/.yml extension filter — Classify never inspects the filename.
func Classify(path string) (Class, *TypeMeta, error)

// IsShrineAPIVersion is the regex check exposed for callers that already hold
// a parsed TypeMeta and just need to ask "is this ours?".
func IsShrineAPIVersion(apiVersion string) bool
```

**Rationale**:

- `internal/manifest/` already owns parsing (`parser.go`), validation (`validate.go`), and the `TypeMeta` type. Classification is the natural sibling.
- Keeping it out of `internal/planner/` lets `internal/handler/teams.go` import the same helper without creating a planner→handler dependency cycle.
- Returning `(Class, *TypeMeta, error)` lets callers reuse the already-unmarshalled metadata for the subsequent `Parse` call, avoiding double `yaml.Unmarshal` on the bytes. (Minor optimisation but free.)

**Alternatives considered**:

- *Inline the regex in `LoadDir`*: would force `internal/handler/teams.go` to copy the same six lines — direct violation of Constitution Principle VII (DRY).
- *Put the helper in `internal/planner/`*: would force `internal/handler/` to import `internal/planner/` just to scan for team manifests, which is a layer inversion (planner depends on manifests, not the other way around).
- *Add a method on `*Manifest`*: `Manifest` does not exist before parsing succeeds — classification has to happen on raw bytes, so a free function is the right shape.

---

## R-002: What exact regex anchors `apiVersion`?

**Decision**: `^shrine/v\d+([a-z]+\d+)?$` — matches the spec's clarification verbatim. Compiled once at package init via `regexp.MustCompile`.

**Examples (must match)**: `shrine/v1`, `shrine/v2`, `shrine/v1beta1`, `shrine/v10alpha7`.
**Examples (must NOT match)**: `Shrine/v1` (capital), `shrines/v1` (typo), `shrine/dev` (no version), `shrine/v1 ` (trailing whitespace — YAML strips this for scalar values, so the runtime input is already trimmed), `apps/v1`, `traefik.containo.us/v1alpha1`, `""` (empty), missing field (Go zero value `""`).

**Rationale**: Strict regex is the spec's chosen trade-off (spec §Clarifications, Q1) — operators who typo `apiVersion` accept silent skip rather than ambiguous detection. The `([a-z]+\d+)?` group covers the Kubernetes-style stability suffix (`alpha`, `beta`) that shrine has already adopted in [.specify/memory/constitution.md](../../.specify/memory/constitution.md) §I ("`apiVersion: shrine/v1`").

**Alternatives considered**:

- *Loose prefix match (`strings.HasPrefix(apiVersion, "shrine/")`)*: would accept `shrine/dev`, `shrine/`, `shrine/v1-WIP`, defeating the "strict, unambiguous detection" requirement.
- *Multi-document YAML support*: explicitly out of scope per spec §Assumptions and §Edge Cases.
- *Case-insensitive match*: rejected by spec clarification — `Shrine/v1` is treated as a typo and skipped.

---

## R-003: How is the foreign-file notice surfaced (FR-006)?

**Decision**: After every `LoadDir` / `walkYAMLFiles` call that classified at least one file as Foreign, emit a single line to stdout via the existing `fmt.Printf` channel used elsewhere in the handler layer:

```
shrine: ignored 2 non-shrine YAML file(s) under specs/: traefik/traefik.yml, traefik/dynamic/team-x-app.yml
```

The notice is emitted by the **caller** (`LoadDir`, `ApplyTeams`), not by `Classify`, so unit tests of `Classify` stay deterministic and silent. The notice is suppressed when the foreign list is empty.

**Rationale**:

- Shrine has no structured logger today (see [internal/handler/teams.go:117](../../internal/handler/teams.go#L117) using `fmt.Printf` for status output). Introducing one is out of scope per Principle IV (YAGNI).
- Spec §FR-006 marks this as SHOULD, not MUST, and explicitly says "presence or absence MUST NOT change exit status" — a single Printf line satisfies both.
- Files filtered by extension (FR-001(a)) are deliberately excluded from the count to keep the notice signal-rich.

**Alternatives considered**:

- *Per-file lines*: noisy on projects with a populated `routing-dir/dynamic/` containing dozens of generated route files.
- *Stderr*: spec §II says user-visible non-error output goes to stdout; this is informational, not an error.
- *Log only when verbose flag is set*: shrine has no `--verbose` flag today; introducing one is YAGNI for this fix.

---

## R-004: How do we keep `LoadDir` and `walkYAMLFiles` in sync (Principle VII / DRY)?

**Decision**: Extract a single shared scanner into `internal/manifest/scan.go`:

```go
// ScanResult lists, for one directory walk, the .yaml/.yml files split into
// shrine-classified candidates (with their probed TypeMeta) and foreign paths.
type ScanResult struct {
    Shrine  []ShrineCandidate
    Foreign []string
}

type ShrineCandidate struct {
    Path     string
    TypeMeta TypeMeta
}

// ScanDir walks `dir`, applies the .yaml/.yml extension filter, then calls
// Classify on each admitted file. Foreign files are silently collected.
// Malformed YAML in an admitted file aborts the scan with a wrapped error
// (FR-004).
func ScanDir(dir string) (*ScanResult, error)
```

`LoadDir` then becomes "call `ScanDir`, parse + validate every `Shrine` candidate, route by `Kind`, emit foreign-files notice". `walkYAMLFiles` (renamed and inlined into `ApplyTeams` since it has only one caller) does the same and discards non-`Team` candidates.

**Rationale**:

- Today `getFiles` ([internal/planner/loader.go:51](../../internal/planner/loader.go#L51)) and `walkYAMLFiles` ([internal/handler/teams.go:67](../../internal/handler/teams.go#L67)) are line-for-line duplicates of each other. Constitution VII demands extraction "immediately" once a behaviour has two call sites.
- Putting `ScanDir` in `internal/manifest/` is the only place both callers can already import without inversion (planner→manifest, handler→manifest).
- FR-005 ("apply uniformly to every code path that scans a directory") makes this consolidation a correctness requirement, not a style preference: any future scan-using command must call `ScanDir` to inherit the fix.

**Alternatives considered**:

- *Two parallel implementations kept in sync by convention*: rejected — the bug we are fixing exists today partly because there are already two scanners.
- *Keep `getFiles` / `walkYAMLFiles` and just have each call `Classify` on every result*: works correctly but duplicates the walk logic forever. Constitution VII is unambiguous here.

---

## R-005: How is the integration scenario built (Principle V)?

**Decision**: Add a new fixture directory `tests/testdata/deploy/foreign-yaml/` containing:

- `team.yaml` — valid `Team` manifest (mirrors existing `tests/testdata/deploy/basic/`).
- `app.yaml` — valid `Application` manifest, e.g. a `whoami` deployment.
- `traefik/traefik.yml` — a static Traefik-shaped YAML file with **no** `apiVersion` field (mirrors what `internal/plugins/gateway/traefik/config_gen.go` writes).
- `traefik/dynamic/team-foo-app.yml` — a Traefik dynamic route file, also without `apiVersion`.

Add one new `s.Test` case to [tests/integration/deploy_test.go](../../tests/integration/deploy_test.go):

```go
s.Test("should deploy successfully when specsDir contains foreign YAML files", func(tc *TestCase) {
    tc.Run("deploy",
        "--path", fixturesPath("foreign-yaml"),
        "--state-dir", tc.StateDir,
    ).AssertSuccess()
    tc.AssertContainerRunning(testTeam + ".whoami")
    // Foreign files must not produce containers / state entries:
    tc.AssertContainerNotExists(testTeam + ".traefik")
})
```

A second case under [tests/integration/traefik_plugin_test.go](../../tests/integration/traefik_plugin_test.go) exercises the **canonical** failure: enable the Traefik plugin with the default `routing-dir = {specsDir}/traefik`, then re-run `shrine deploy` and assert it succeeds (this is the SC-001 acceptance path that crashes on `main` today).

**Rationale**: Principle V requires the real binary against a real Docker daemon; both new cases use the existing `NewDockerSuite` harness with `tc.Run("deploy", ...)`. Unit tests of `Classify` and `ScanDir` cover edge cases cheaply but do not satisfy the gate alone (Principle V: "Unit tests supplement but NEVER substitute").

**Alternatives considered**:

- *Only unit tests*: forbidden by Principle V.
- *Modify an existing fixture*: would couple unrelated test cases to this regression and mask future drift; a dedicated fixture is clearer and cheaper to maintain.

---

## R-006: Behaviour for `kind` that doesn't match a known kind but `apiVersion` is shrine (FR-003)?

**Decision**: After `Classify` returns `ClassShrine`, the existing pipeline (`manifest.Parse` → `manifest.Validate`) already handles this:

- [internal/manifest/parser.go:65](../../internal/manifest/parser.go#L65) returns `"unknown manifest kind: %q"` when `Kind` is not Application/Resource/Team.
- [internal/manifest/validate.go:23](../../internal/manifest/validate.go#L23) returns `"kind is required"` / `"kind must be one of: Team, Resource, Application"`.

Both errors are wrapped in `LoadDir` with the file path ([internal/planner/loader.go:35](../../internal/planner/loader.go#L35), `parsing manifest %q`). FR-003's "error that identifies the file path and the offending kind value" is therefore satisfied **once we route shrine-classified files through `Parse`** — exactly what the new `LoadDir` does. No new error path is needed; we only need to confirm with a unit test that a file with `apiVersion: shrine/v1` and `kind: Aplication` (typo) still fails loudly with the file path in the message.

**Rationale**: Adding a parallel error message would duplicate behaviour and risk drift. The spec's FR-007 ("existing behaviour for valid shrine manifests MUST be unchanged") is best honoured by leaving `Parse`/`Validate` exactly as-is and only changing what enters them.

**Alternatives considered**:

- *Custom `kind`-check before calling `Parse`*: rejected — duplicates `validateTypeMeta`.

---

## R-007: What about `cmd/shrine generate` and other not-yet-enumerated scan paths (FR-005)?

**Decision**: Audit pass during implementation: `grep -rn "filepath.WalkDir\|filepath.Walk" internal/ cmd/` will list every directory walker. Today only two manifest-scanning walkers exist (`internal/planner/loader.go`, `internal/handler/teams.go`); both are migrated. Any future addition of a third walker is gated by the new `ScanDir` API — there is no other primitive to call, so FR-005 holds by construction.

**Rationale**: FR-005 is forward-looking ("every code path… so that no command crashes on the same foreign file"). Centralising the walker is the cheapest mechanism to guarantee that.

**Alternatives considered**:

- *Lint rule banning ad-hoc `WalkDir` in `internal/`*: out of scope and YAGNI for a one-binary CLI with two call-sites.

---

## Summary of Decisions

| ID | Decision | Locked by |
|----|----------|-----------|
| R-001 | New file `internal/manifest/classify.go` exposing `Classify` + `IsShrineAPIVersion` | FR-001(b), Principle VII |
| R-002 | Regex `^shrine/v\d+([a-z]+\d+)?$`, compiled once | Spec Clarification Q1 |
| R-003 | Single stdout notice listing foreign paths, suppressed when none | FR-006, Principle II |
| R-004 | Extract shared `ScanDir` into `internal/manifest/scan.go`; collapse the two duplicated walkers | FR-005, Principle VII |
| R-005 | New fixture `tests/testdata/deploy/foreign-yaml/` + 2 integration scenarios | Principle V, SC-001..SC-004 |
| R-006 | Reuse existing `Parse`/`Validate` error paths for FR-003 | FR-007 |
| R-007 | `ScanDir` is the only primitive future scanners call | FR-005 |
