# Research: Deployed Version in get and describe

**Feature**: 031-deployed-version-columns | **Date**: 2026-10-06

No `NEEDS CLARIFICATION` markers existed in the Technical Context. The epic's
design settles the shape of the change (TD-4, section 3.3, section 4.7, T1-01 to
T1-05); the decisions below record how each binds to the code on `main` at
`d90b8c6` and which alternatives were set aside.

## R1. Where the unexpanded reference is captured

**Decision**: `CreateContainer` builds the `state.Deployment` prototype as its first
step, from `op.Kind`, `op.Name`, `op.Image`, and `op.ImagePullPolicy`, before the
line `op.Image = expanded`. The prototype is threaded through `ensureRunning` and
`createFreshContainer` in place of the bare `wantHash string`, carrying the config
hash in its `ConfigHash` field; `recordDeployment(team, record, containerID)` sets
the container id and writes it.

**Rationale**: T1-01 says the record stores the reference as written, so it has to
be taken before expansion. Threading one value instead of a hash and a second
string keeps the two helper signatures the same width they have today. The `#33`
invariant that everything after expansion (pull, credential lookup, hash,
container spec) sees the same fully-qualified reference is untouched.

**Alternatives considered**:
- Stop mutating `op.Image` and pass `expanded` to each consumer. Rejected: it
  reopens the invariant `#33` fixed and touches every consumer for no gain.
- Add a `manifestImage string` parameter beside `wantHash`. Rejected: two loose
  strings through two functions where one record value does the job.

## R2. Line format and the reader rule

**Decision**: `saveTeam` writes `Kind Name ContainerID ConfigHash Image Policy`,
six fields joined by single spaces, always, even when some are empty. `loadTeam`
splits each line with `strings.Fields` and reads fields four, five, and six as
optional: present when the index exists, empty otherwise.

**Rationale**: Design section 3.3 fixes the order and the optional-trailing rule.
`strings.Fields` also parses the aligned example in the design and tolerates the
trailing spaces an empty optional field leaves. Image references never contain
spaces, so the split is unambiguous. The three-field tolerance the loader has
today (records from before config hashes) is kept by the same rule.

**Alternatives considered**:
- `strings.SplitN(line, " ", 6)`, the current reader's style. Rejected: a
  hand-aligned or doubly spaced line would produce empty fields in the middle.
- Writing a `-` placeholder for an empty optional field. Rejected: the design's
  reader rule is "absent reads as empty", and a placeholder would need to be
  translated back on every read and would collide with the display rule.

## R3. Keeping the store's unit tests off the filesystem

**Decision**: `DeploymentStore` gains `readFile readFileFn` and `writeFile
writeFileFn`, the types `hostports.go` already declares, plus the unexported
constructor `newDeploymentStoreWithFileOps(baseDir, read, write)`.
`NewDeploymentStore` keeps its signature and wires `os.ReadFile` and a writer that
creates the team directory then calls `atomicWriteFile`. The temp-file prefix in
`atomicWriteFile` is derived from the target's base name so the helper serves both
stores. The existing tests are rewritten on an in-memory file map keyed by path;
the cases they pinned (comment and blank-line tolerance, three- and four-field
lines, persistence across instances, update, remove, team isolation) are kept and
the six-field read and write cases are added.

**Rationale**: the project's test policy (also the ticket's definition of done):
unit tests touch no filesystem; file-backed stores are tested through their
injectable file operations. The pattern exists in the same package.

**Alternatives considered**:
- Leave the existing `t.TempDir()` tests and add new ones beside them. Rejected:
  the reader and writer are what this feature changes; the tests covering them
  would be the ones violating the policy.
- A `bufio.Scanner` over an injected `io.Reader`. Rejected: the write side would
  still need injection; one byte-slice pair matches `hostports.go`.

## R4. Testing the table and the describe output

**Decision**: `formatDeploymentsTable(deployments) string` and
`formatDeploymentDetail(team, d) string` build the output; the existing
`printDeploymentsTable` and `printDeploymentDetail` become one-line wrappers that
print the string, so their callers do not change. A helper `valueOrUnknown(value)
string` returns `-` for an empty value and serves the table's VERSION cell and the
describe `Image:` and `Pull policy:` lines.

**Rationale**: `formatDeployPlan` in the same package is the precedent: a
string-returning formatter unit-tested directly. The unit tests assert the
header order, the row content, and the `-` fallback without capturing stdout.

**Alternatives considered**:
- Redirect `os.Stdout` through a pipe in the tests. Rejected: process-global
  state, unsafe with `t.Parallel`, and the package has a better precedent.
- Pass an `io.Writer` into every `List*`/`Describe*` handler. Rejected: changes
  the handler signatures `cmd/` calls for a testability concern the formatter
  split solves locally.

## R5. Column layout

**Decision**: row format `%-20s %-30s %-15s %-40s %-15s` for TEAM, NAME, KIND,
VERSION, CONTAINER ID. The separator is a run of dashes as long as the formatted
header, computed, not hard-coded.

**Rationale**: VERSION sits after KIND (design 4.7, T1-03); the other four
columns keep header, order, and value. Forty characters fit the references the
fixtures and the design examples use (`192.168.1.206:8080/hello-api:latest` is
35); a longer reference overflows its cell without truncation, as the other
columns do today. The spec records that widths and the separator are not a
contract.

**Alternatives considered**: truncating long references. Rejected: the column
exists so the operator can read the reference; truncation defeats it.

## R6. Describe line placement and wording

**Decision**: `Image:` then `Pull policy:` directly after `Kind:`, before
`Container ID:`, with the same fourteen-character label column the existing lines
use. Both print `-` when the record has no value.

**Rationale**: identity lines first; T5 appends `Pinned:` and `Running image:`
after them without reordering (design 4.7). The spec's Clarifications record the
placement and the placeholder.

## R7. Which policy value is recorded

**Decision**: `op.ImagePullPolicy` verbatim. The engine already fills it with
`manifest.EffectivePullPolicy(image, declared)`, so the recorded value is the
effective policy: the declared one when present, else `Always` for `latest` or an
untagged image, else `IfNotPresent`.

**Rationale**: T1-01 asks for the effective policy; the backend is the writer
(TD-8) and the op already carries the effective value. When T3 lands `Pinned` and
TD-7 moves normalisation to plan time, the op still carries the effective value
and this code does not change.

## R8. Integration scenarios and the two test helpers

**Decision**: extend `TestGetDocker` with: the VERSION header and, on the row of
each fixture artifact, the manifest's reference `traefik/whoami`; `--team` on
`get deployed`; legacy records (rewritten to four fields) listing with `-`; a
redeploy with unchanged manifests, which takes the up-to-date path, after which
the listing shows the reference again. Extend `TestDescribeDocker` with `Image:`
and `Pull policy: Always` for the app, the same for a resource after deploying the
`resources` fixture, and `-` on both lines for a legacy record. Two helpers go into
`testutils`: `AssertOutputLineContains(anchor, want)` asserts on the stdout line
that contains `anchor`, which is what a table needs; `SeedLegacyDeploymentRecords
(tc, team)` rewrites `<state>/<team>/deployments.txt` keeping the first four
fields of every line, which is exactly what the previous release wrote.

**Rationale**: the ticket names the two suites and the two scenarios (version
column present after deploy; legacy record tolerated). `traefik/whoami` carries no
tag, so the effective policy is `Always`, a value the describe scenario can assert.
Integration tests are isolated from internal packages, so the helpers live in
`testutils` and T5 reuses the line assertion for the pinned column.

**Alternatives considered**: hand-writing a legacy `deployments.txt` with a fake
container id. Rejected: the redeploy-heals scenario needs the real container id
and hash so the up-to-date path is the one exercised.

## R9. Documentation

**Decision**: `AGENTS.md` only, the `deployments.txt` line in the State Directory
Layout block (design 4.11). No docs-site change and no CLI regeneration.

**Rationale**: no page under `docs/content/` documents the listing columns, the
describe lines, or the record format; the CLI pages are generated from Cobra
definitions that do not change. The progress entry and the spec-kit pointers
(`.specify/feature.json`, `CLAUDE.md`) follow the project's usual form.
