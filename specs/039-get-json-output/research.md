# Research: Machine-Readable Output for `shrine get`

**Feature**: 039-get-json-output | **Date**: 2026-10-11

No `NEEDS CLARIFICATION` markers remained in the Technical Context. The spec's
clarification session settled the data shape (describe-level workload entries, a
nested `pin`, a nullable `runningImage`, teams unchanged). The decisions below
bind those answers to the code on `main` at `91ac0ba` and record the
alternatives set aside.

## R1. Where the flag lives and how unsupported subcommands reject it

**Decision**: `--json` is a local boolean flag registered on each of
`getTeamsCmd`, `getApplicationsCmd`, `getResourcesCmd` and `getDeployedCmd`, all
four bound to one package variable `getJSONFlag`. It is not a persistent flag on
`getCmd`.

**Rationale**: FR-010 requires `get config-dir --json` and `get state-dir --json`
to fail. Cobra already rejects a flag a command does not declare
(`unknown flag: --json`, non-zero exit), so declaring it only where it applies
gives the rejection with no code. Cobra resolves one invocation to one
subcommand, so sharing a variable across the four is safe, the same way
`getTeamFlag` is shared today.

**Alternatives considered**:
- A persistent flag on `getCmd` with a guard in `config-dir`/`state-dir`
  returning an error. Rejected: two guards for a behaviour Cobra gives for free,
  and the flag would show in those commands' help.
- `-o/--output json` (kubectl style). Rejected by the spec's scope (a standalone
  boolean flag; no other formats are wanted, Principle IV).

## R2. Handler surface: separate JSON entry points, shared listing helpers

**Decision**: `internal/handler` gains `ListDeployedJSON`, `ListApplicationsJSON`,
`ListResourcesJSON` (each `(team string, store *state.Store, backend
engine.ContainerBackend) error`) and `ListTeamsJSON(store state.TeamStore)
error`. The fetch-and-filter step the table and JSON variants share is extracted
into `listDeployments(team, kind string, store *state.Store)` (an empty kind
means all), so each public function is the fetch plus one print call. Each
`cmd` `RunE` picks the table or JSON function on `getJSONFlag`.

**Rationale**: Principle II keeps `cmd/` a thin dispatcher; the branch on one
flag is dispatch. The JSON variants take a container backend the table variants
must not take (FR-003a keeps the table state-only), so a shared function with a
format parameter would force a backend argument on the table path. The shared
helper keeps the fetch and kind filter in one place (Principle VII).

**Alternatives considered**:
- One `ListDeployed(team, store, opts ListOptions{JSON bool, Backend ...})`.
  Rejected: an options struct for one boolean, and the table path would carry
  an unused backend.
- Doing the branch inside the handler on a `format string`. Rejected for the
  same reason, plus stringly-typed dispatch.

## R3. Reaching the container runtime without ever failing the listing

**Decision**: The JSON `RunE` for the three workload commands builds the
backend with `app.NewQueryContainerBackend(cfg, store)`, as `describe` does. If
construction fails, it passes a nil backend instead of returning the error.
A new `inspectRunningImage(backend, containerID) (string, error)` returns
`errNoContainerRuntime` for a nil backend, the inspect error, or the runtime's
image (possibly empty). The existing `runningImage` (describe's presentation
form) is rewritten on top of it, printing `unavailable (<err>)` exactly as
today, so both paths share one inspect call site. The JSON path's
`runningImageOrNull` maps any error to `null`, and skips the call for an empty
container ID (also `null`); `describe` keeps inspecting as it does today.

**Rationale**: The clarification (Q1 → A) and FR-003a say runtime trouble never
turns a listing into an error. Construction only fails on a malformed
`DOCKER_HOST`, which is runtime trouble like any other. With the daemon down,
each inspect fails fast on the refused connection, so no timeout is needed at
homelab scale. An empty container ID is skipped because inspecting `""` is a
guaranteed error and a wasted round trip.

**Alternatives considered**:
- Return the construction error, as `describe` does. Rejected: contradicts
  FR-003a and SC-002a.
- Abort the remaining inspects after the first connection error. Rejected: the
  listing is tens of records; the saving is milliseconds and the rule adds a
  branch (Principle IV).

## R4. Inspecting sequentially, not in parallel

**Decision**: Inspect each workload in listing order, one call at a time.

**Rationale**: The spec's scale is tens of records per host; one local socket
inspect is a few milliseconds, so a full listing stays well under a second.
Sequential calls keep the output order trivially identical to the table
(FR-005). This is the item deferred to planning in the clarify session's
coverage summary.

**Alternatives considered**:
- A bounded worker pool. Rejected until a host with hundreds of workloads shows
  a need (Principle IV).
- `ContainerList` with a name filter, one call for all. Rejected: it reports
  the image differently from `ContainerInspect` (which `describe` uses), so the
  two commands could disagree, and it would add a new backend method.

## R5. Field values: raw values, not display cells

**Decision**: JSON values come from the record and the pin, never from the
table's formatting helpers. `versionCell` is split into `versionOf(team, d,
pins) string` (raw, empty when unknown) and the table's
`valueOrUnknown(versionOf(...))`. A new `pinOf(team, d, pins) (state.ImagePin,
bool)` returns the pin only when the record's policy is `Pinned` and the pin
matches the record, so `versionOf` and the JSON `pin` agree by construction.
`image`, `pullPolicy`, `containerId`, `configHash` are the record's fields
unchanged (full container ID and full hash). Pins are read once per listing with
the existing `loadImagePins`.

**Rationale**: FR-007 maps the `-` placeholder to `""`. Taking raw values
avoids reverse-mapping `-` (which would be wrong for a real value of `-`, if one
ever existed). `pinOf` names the one rule both the VERSION column and the JSON
pin need, and `loadImagePins` keeps one pin-store read per listing rather than
one per artifact (as `describe`'s `lookupPin` does).

**Alternatives considered**:
- Building entries through `gatherDeploymentDetail` (describe's path). Rejected:
  one pin-store read per artifact, and its running image is already formatted
  for display.

## R6. Encoding: one document, built before anything is written

**Decision**: A generic envelope `jsonListing[T any] struct { Data []T
\`json:"data"\` }` is filled with a non-nil slice (`make([]T, 0, n)`) and
encoded with `json.NewEncoder` into a `bytes.Buffer`, two-space indent,
`SetEscapeHTML(false)`. The buffer is written to stdout only after encoding
succeeds. The `formatXJSON` functions return `([]byte, error)` so unit tests
read bytes, never stdout.

**Rationale**: A non-nil slice gives `{"data": []}` for an empty listing
(FR-008); a nil slice would encode as `null`. Encoding fully before writing
guarantees FR-009 (no partial document). `SetEscapeHTML(false)` keeps `&`, `<`
and `>` in display names readable; all other escaping (quotes, backslashes,
control characters) stays, so the document always parses. Indentation follows
the spec's formatting assumption. The generic envelope is one type serving two
entry types, so no per-type wrapper is written. String-returning formatters
follow `formatDeploymentsTable` and `formatDeployPlan`.

**Alternatives considered**:
- `json.MarshalIndent`. Rejected: it always escapes HTML characters.
- `map[string]any{"data": ...}`. Rejected: loses field order and types.

## R7. Field names, `null`s and the timestamp

**Decision**: Workload entry keys, in this order: `team`, `name`, `kind`,
`image`, `pullPolicy`, `version`, `pin`, `runningImage`, `containerId`,
`configHash`. `pin` is `*pinEntry` (`reference`, `version`, `pinnedAt`);
`runningImage` is `*string`; neither uses `omitempty`, so both are present as
`null` when absent. `pinnedAt` is `PinnedAt.UTC().Format(time.RFC3339)` (second
precision, `Z` suffix). Team entry keys: `name`, `displayName`, `maxApps`,
`maxResources`, the quotas as JSON numbers.

**Rationale**: FR-007 requires every field present in every entry. The key
order follows `describe`'s line order with `version` placed after the pull
policy that decides it. Second-precision RFC 3339 matches the precision the pin
store keeps and is what `jq`'s `fromdateiso8601` parses; Go's default
`time.Time` encoding (RFC 3339 with nanoseconds) is not.

**Alternatives considered**:
- `omitempty` on `pin`/`runningImage`. Rejected: FR-007.
- A `YYYY-MM-DD` date, as `describe` prints. Rejected: presentation; the
  clarification asked for the full UTC timestamp.

## R8. Tests: unit for shape and values, integration for the binary

**Decision**:
- Unit (`internal/handler/deployments_json_test.go`, `teams_json_test.go`): the
  formatters over in-memory records and pins with the existing `imageBackend`
  fake, decoded back with `encoding/json`: field set and order, `-` → `""`,
  `pin` present/`null` for Pinned / stale / manifest-owned / other-kind pins,
  `runningImage` `null` for nil backend, inspect error and empty container ID,
  the RFC 3339 timestamp, empty listing → `{"data":[]}`, special characters
  round-trip, quotas as numbers. No filesystem, no stdout capture.
- Integration (written first, Principle V): `TestGetNoDocker` gains teams JSON,
  empty `get deployed --json`, a seeded record with `DOCKER_HOST` at a closed
  port (`runningImage` `null`, success), and `get config-dir --json` failure.
  `TestGetDocker` gains `--json` for all three workload commands with and
  without `--team` (entries match the table rows, live `runningImage`, full
  container ID). `TestPinnedVersionQueries` gains one scenario asserting the
  `pin` object equals the `describe` values and the table's readable version.
  Each test decodes stdout with `encoding/json` in the test file, so no new
  `testutils` helper is needed.

**Rationale**: Matches the project's split (feedback: unit tests touch no
filesystem; integration runs in CI only, compile-checked locally with
`go vet -tags integration`). The pinned suite already owns the loopback
registry and pinned fixtures, so the pin assertions belong there.
