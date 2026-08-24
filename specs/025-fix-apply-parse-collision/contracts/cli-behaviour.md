# Contract: CLI Behaviour

**Feature**: `025-fix-apply-parse-collision` | **Date**: 2026-08-24
**Scope**: Operator-visible contract (exit code, stdout, stderr, side effects) for the three affected commands. Every row maps to an integration scenario in research Decision 9.

Conventions: informational lines → stdout; failures → stderr as `Error: <message>` (Cobra); exit 1 on any returned error. No flags, subcommands, or output formats are added or renamed.

## `shrine apply teams [--path <dir>]`

| # | Directory contents | Exit | stderr contains | stdout | State |
|---|-------------------|------|-----------------|--------|-------|
| T1 | Only valid Team manifests (+ optional foreign YAML, + optional parseable Application/Resource files) | 0 | — | `Synced team: <name>` per team, `Successfully synced N teams to state.`, `Skipping <file>: not a Team manifest` per non-Team shrine file, foreign-file notice if any | all teams written **(unchanged)** |
| T2 | One shrine-classified file with `kind: Aplication` beside a valid team (`tests/testdata/apply/bad-kind/`) | 1 | `apply teams failed:`, `typo.yaml`, `Aplication` | — | **no** team written (`AssertTeamNotInState`) |
| T3 | Two broken shrine files + one valid team (`apply/multi-broken/`) | 1 | both file paths, one bullet each | — | `AssertTeamCount(0)` |
| T4 | Team manifest that parses but lacks `metadata.name` (`apply/invalid-team/`) | 1 | `validating manifest`, the file path, `metadata.name is required` | — | no team written |
| T5 | Malformed YAML with `.yml` extension anywhere in the tree | 1 | file path + parse error (from `ScanDir`) | — | untouched **(unchanged)** |
| T6 | No shrine-classified files at all | 0 | — | `No team manifests found in "<dir>" directory.` | untouched **(unchanged)** |
| T7 | Valid teams but the state store rejects a write | 1 | `saving team "<name>" to state:` + cause | `Synced team:` lines for teams saved before the failure | partial (teams saved before the failure remain) |

Diagnostic shape for T2–T4:

```
Error: apply teams failed:
- parsing manifest "/…/bad-kind/typo.yaml": unknown manifest kind: "Aplication"
```

```
Error: apply teams failed:
- parsing manifest "/…/multi-broken/typo.yaml": unknown manifest kind: "Aplication"
- validating manifest "/…/multi-broken/noname.yaml": validation failed:
- metadata.name is required
```

## `shrine apply -f <file> [--path <dir>]`

Let **X** be the application in `<file>`; the footprint is every application loaded from `<dir>` (plus X).

| # | Situation | Exit | stderr contains | Side effects |
|---|-----------|------|-----------------|--------------|
| F1 | X's primary domain+path matches another app's primary or alias (`apply/routing-collision/app-b.yml` vs `app-a.yml`) | 1 | `routing validation failed:`, `routing collision: host="collision.apply.local"`, `shrine-apply-test/app-a`, `shrine-apply-test/app-b` | none — `AssertContainerNotExists("shrine-apply-test.app-b")`, no routing file written |
| F2 | X's alias matches another app's primary or alias | 1 | same shape as F1 | none |
| F3 | X collides with nothing; other apps in `<dir>` collide with each other (`app-c.yml`) | 0 | — | X deployed **(collision not X's problem)** |
| F4 | X collides with nothing; directory is collision-free | 0 | — | X deployed **(unchanged)** |
| F5 | X is already present in `<dir>` (re-apply) | 0 | — | no self-collision **(unchanged)** |
| F6 | `<file>` is a Resource | 0 | — | resource deployed; routing check is a no-op **(unchanged)** |
| F7 | No `--path` and no configured specs dir | 0 | — | one-app set, nothing to collide with **(unchanged)** |

The F1/F2 diagnostic is produced by the same code path as `shrine deploy` for the same directory, so the application refs and host+path are identical between the two commands (spec FR-009, SC-002).

## `shrine deploy team <team> [--dry-run] [--path <dir>]`

Let **T** be the requested team.

| # | Collision participants in `<dir>` | Exit | stderr contains | Notes |
|---|-----------------------------------|------|-----------------|-------|
| D1 | Two apps both owned by T (`deploy_team/collision/`) | 1 | `collide.test.local`, both `shrine-team-a/…` refs | **unchanged** (existing Scenario E) |
| D2 | One app owned by T, one by another team (`deploy_team/collision-cross/`) | 1 | `shrine-team-a/alpha`, `shrine-team-b/beta`, the host | in-scope vs out-of-scope still fails |
| D3 | Both apps owned by a team other than T (`deploy_team/collision-other/`, T = team-a) | 0 | — | **new**: out-of-scope collision does not block; stdout carries the normal plan / dry-run output |
| D4 | Same directory as D3, T = team-b | 1 | both `shrine-team-b/…` refs | the owning team still sees its own collision |
| D5 | Same directory as D3, bare `shrine deploy --dry-run` | 1 | both refs | bare deploy is whole-directory **(unchanged)** |
| D6 | Several pairs each touching a T app | 1 | one `routing collision:` line per pair | all reported in one run |

`--dry-run` and the real deploy share the planning path, so every row holds for both forms (spec FR-014).

## Unchanged surfaces (regression guards)

- `shrine deploy` / `shrine deploy --dry-run` on `tests/testdata/deploy/routing-collision/`: still exit 1 naming `shrine-deploy-test/app-a` and `app-b`.
- `shrine deploy` on collision-free fixtures: exit 0, identical output.
- `shrine apply teams` on `apply/success/`, `apply/foreign-yaml/`, nested and multi-team temp dirs: exit 0, identical output.
- `shrine apply -f` error cases (`errors/malformed.yml`, `errors/unknown-kind.yml`, `errors/team.yml`, `errors/file.txt`, `bad-kind/typo.yaml`): unchanged failures.
