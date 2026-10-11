# Feature Specification: Machine-Readable Output for `shrine get`

**Feature Branch**: `039-get-json-output`  
**Created**: 2026-10-11  
**Status**: Draft  
**Input**: User description: "Add a new `--json` flag to the get command that returns the information in JSON instead of the presentation table. `shrine get deployed --json` should print `{ data: [ { team, name, kind, version, containerId } ] }` — the same data the table shows. `--json` only works with `get deployed`, `get resources`, `get applications` and `get teams`. For teams the fields are name, display name, max apps, max resources."

## Clarifications

### Session 2026-10-11

- User directive: each application/resource entry carries the information `describe` shows for that artifact — name, team, kind, image, pull policy, pin, running image, container ID and config hash — not only the table's five columns.
- Q: Should JSON include the live running image, which requires the container runtime? → A: Yes; read it live per workload, and when it cannot be read (no runtime, missing container) that entry's `runningImage` is `null` and the command still succeeds.
- Q: How is the pin represented? → A: A nested `pin` object `{reference, version, pinnedAt}` (exact image reference, readable version such as `v1.11.0@4f90b33ddca9`, full UTC timestamp), `null` when the artifact is not Pinned or its pin is missing; plus a top-level `version` carrying the table's VERSION value.
- Q: Do team entries also gain `describe team` detail (contact, registry user, allowed resource types, deployments summary)? → A: No; team entries keep exactly `name`, `displayName`, `maxApps`, `maxResources`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Script against deployed workloads (Priority: P1)

An operator writing a shell script, CI step or monitoring hook needs everything Shrine knows about each deployed workload — team, name, kind, image, pull policy, pin, running image, container and config hash — without scraping a column-aligned table or running `describe` once per artifact. They run `shrine get deployed --json` and pipe the result into a JSON tool, picking out exactly the fields they need.

**Why this priority**: `get deployed` is the superset listing (applications and resources together) and the most common thing an automation asks of Shrine. Delivering JSON for it alone already removes the need to parse the table or loop over `describe`.

**Independent Test**: With at least one application and one resource deployed, run `shrine get deployed --json`, parse stdout as JSON, and confirm one entry per row the table shows, each carrying the same values `describe` shows for that artifact.

**Acceptance Scenarios**:

1. **Given** two teams with deployed applications and resources, **When** the operator runs `shrine get deployed --json`, **Then** stdout is a single JSON document of the form `{"data": [...]}` holding one object per deployed workload, in the same order the table lists them, each with the workload fields defined in FR-003.
2. **Given** the same state, **When** the operator runs `shrine get deployed --team alpha --json`, **Then** `data` holds only team `alpha`'s workloads, exactly the rows the table would show with `--team alpha`.
3. **Given** a Pinned application `whoami` in team `sandbox`, **When** the operator runs `shrine get deployed --json`, **Then** its entry carries the same image, pull policy, pin, running image, full container ID and config hash that `shrine describe application whoami --team sandbox` shows — with the config hash in full rather than shortened.
4. **Given** the container runtime is stopped, **When** the operator runs `shrine get deployed --json`, **Then** the command succeeds, every entry's `runningImage` is `null`, and every state-derived field is still filled.
5. **Given** a workload whose container no longer exists while the runtime is up, **When** listed with `--json`, **Then** only that entry's `runningImage` is `null`; the others carry their live image.
6. **Given** nothing is deployed, **When** the operator runs `shrine get deployed --json`, **Then** stdout is `{"data": []}` and the command succeeds, with no "No deployments found." sentence mixed into the output.
7. **Given** any state, **When** the operator runs `shrine get deployed` without `--json`, **Then** the output is the existing table, unchanged, and still needs no container runtime.

---

### User Story 2 - JSON for applications and resources separately (Priority: P2)

An operator who only cares about one kind of workload — say, every database resource, or every application across teams — runs `shrine get applications --json` or `shrine get resources --json` and receives the same JSON shape as `get deployed`, filtered to that kind.

**Why this priority**: These are narrower views of the same data and share the same shape, so they follow naturally once `get deployed` exists; they save the script a filtering step.

**Independent Test**: With mixed applications and resources deployed, run each command with `--json` and confirm every entry's `kind` matches the command and the set equals the corresponding table's rows.

**Acceptance Scenarios**:

1. **Given** deployed applications and resources, **When** the operator runs `shrine get applications --json` (or its alias `get apps --json`), **Then** `data` holds only the applications, each with the workload fields defined in FR-003.
2. **Given** deployed applications and resources, **When** the operator runs `shrine get resources --json` (or its alias `get res --json`), **Then** `data` holds only the resources, in the same shape.
3. **Given** no applications are deployed, **When** the operator runs `shrine get applications --json`, **Then** stdout is `{"data": []}` and the command succeeds.
4. **Given** the `--team` filter, **When** combined with `--json` on either command, **Then** the filter applies exactly as it does to the table.

---

### User Story 3 - JSON for teams (Priority: P3)

An operator or provisioning script needs the registered teams and their quotas — for example, to check how much headroom a team has before adding a manifest. They run `shrine get teams --json` and read each team's name, display name and quota limits as data.

**Why this priority**: Teams change rarely and are fewer, so the need is less frequent, but it completes the set of listings scripts commonly read.

**Independent Test**: With two teams registered, run `shrine get teams --json` and confirm one entry per team with `name`, `displayName`, `maxApps` and `maxResources`, matching the table's values, where the two quota fields are numbers.

**Acceptance Scenarios**:

1. **Given** registered teams, **When** the operator runs `shrine get teams --json` (or its alias `get team --json`), **Then** stdout is `{"data": [...]}` with one object per team holding `name`, `displayName`, `maxApps` and `maxResources`, in the order the table lists them.
2. **Given** a team whose quotas are 5 apps and 3 resources, **When** listed with `--json`, **Then** `maxApps` is the number `5` and `maxResources` is the number `3`, not the strings `"5"` and `"3"`.
3. **Given** no teams are registered, **When** the operator runs `shrine get teams --json`, **Then** stdout is `{"data": []}` and the command succeeds, with no "Run 'shrine apply teams'" hint mixed into the output.

---

### Edge Cases

- **Unsupported subcommands**: `shrine get config-dir --json` and `shrine get state-dir --json` are rejected as an unknown flag, with a non-zero exit, rather than silently ignoring it. The flag appears in the help of exactly the four supported subcommands.
- **Missing values**: Where the table or `describe` prints a `-` placeholder (for example an artifact whose image is unknown) or a blank (a team with no display name, a workload with no container yet), the JSON carries an empty string for that field, so a script never mistakes the presentation placeholder `-` for a real value. Every field is always present in every entry.
- **Running image unavailable**: When the running image cannot be read — no container runtime, the runtime refuses the connection, or the container is gone — that entry's `runningImage` is `null` (never the human-readable "unavailable (...)" text `describe` prints), and the command still succeeds. Runtime trouble never turns a listing into an error.
- **Special characters**: Names, display names and image references containing quotes, backslashes or non-ASCII characters are escaped so the document always parses.
- **Failures**: If Shrine cannot read its state (for example a corrupt or unreadable state file), the command exits non-zero, writes its error to stderr only, and writes nothing to stdout — never a partial or half-written JSON document.
- **Unknown team filter**: `--team` naming a team with no deployments yields `{"data": []}`, matching the table's "nothing found" behavior rather than becoming an error.
- **Pin no longer matches**: A Pinned record whose pin is missing or stale has `pin: null` (where `describe` prints `Pinned: -`), and its top-level `version` shows the same fallback the table shows today.
- **Not Pinned**: An artifact with any other pull policy has `pin: null`; its `version` is the image reference its manifest named, as in the table.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `shrine get deployed`, `get applications`, `get resources` and `get teams` (and their existing aliases) MUST accept a `--json` flag.
- **FR-002**: With `--json`, the command MUST write to stdout exactly one JSON document whose top level is an object with a single `data` key holding an array of entries, and nothing else on stdout.
- **FR-003**: For `get deployed`, `get applications` and `get resources`, each entry MUST carry the information `describe` shows for that artifact plus the table's version: `team`, `name`, `kind`, `image` (the reference the manifest named), `pullPolicy`, `version` (the table's VERSION value), `pin`, `runningImage`, `containerId` (the full identifier) and `configHash` (the full hash, not the shortened preview `describe` prints). Values MUST equal what `describe` (and, for `version`, the table) shows for the same artifact, apart from the placeholder rule in FR-007 and the full-length container ID and config hash.
- **FR-003b**: `pin` MUST be an object with `reference` (the exact pinned image reference), `version` (the readable version, e.g. `v1.11.0@4f90b33ddca9`) and `pinnedAt` (the full UTC timestamp the pin was taken, in ISO-8601), or `null` when the artifact is not Pinned or has no matching pin.
- **FR-003a**: `runningImage` MUST be read live from the container runtime for each listed workload; when it cannot be read for an entry, that entry's `runningImage` MUST be `null` and the command MUST still succeed. Only `--json` output contacts the runtime; the table MUST stay state-only.
- **FR-004**: For `get teams`, each entry MUST contain exactly the fields `name`, `displayName`, `maxApps` and `maxResources`, where `name` and `displayName` are strings and `maxApps` and `maxResources` are numbers, carrying the same values the table's NAME, DISPLAY NAME, MAX APPS and MAX RESOURCES columns show.
- **FR-005**: Entries MUST appear in the same order as the corresponding table rows, and the set of entries MUST equal the set of table rows for the same command and filters.
- **FR-006**: The `--team` filter MUST apply to `--json` output exactly as it applies to the table.
- **FR-007**: A value the table renders as the `-` placeholder MUST be an empty string in JSON; every field MUST be present in every entry.
- **FR-008**: When there is nothing to list, `--json` output MUST be `{"data": []}` with a successful exit; the human-readable "nothing found" sentences MUST NOT appear on stdout.
- **FR-009**: On any error, the command MUST exit non-zero, report the error on stderr, and emit no JSON on stdout.
- **FR-010**: Passing `--json` to any `get` subcommand other than the four listed in FR-001 MUST fail as an unknown flag.
- **FR-011**: Without `--json`, the output of all four commands MUST be byte-for-byte unchanged from today.
- **FR-012**: The `--json` flag MUST be documented in each supported command's help text and in the generated CLI reference pages.

### Key Entities

- **Workload entry**: One deployed application or resource with the detail `describe` shows — the team that owns it, its name, its kind (`Application` or `Resource`), the image reference its manifest named, its pull policy, the version the table shows, its pin (reference, readable version, time pinned) when Pinned, the image its container is actually running (live, or `null` when unreadable), its full container identifier, and its full config hash.
- **Team entry**: One registered team as a listing reports it — its name, its display name, and its two quota limits (maximum applications, maximum resources).
- **Listing document**: The envelope wrapping either kind of entry: an object with a `data` array, leaving room to add sibling keys later without breaking scripts that read `data`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For every one of the four commands, with and without `--team`, 100% of `--json` outputs parse as valid JSON with a standard JSON parser, including when the listing is empty.
- **SC-002**: For any given state, the entries in `--json` output match the table's rows one-for-one in count and order, and each entry's values match what `describe` shows for that artifact (apart from the `-` → empty-string rule and full-length container ID and config hash).
- **SC-002a**: With the container runtime stopped, all four `--json` commands still succeed 100% of the time, with `runningImage` `null` on every workload entry.
- **SC-003**: An operator can extract a single field for every workload (for example every `containerId` for team `alpha`) with one command pipeline and no text slicing, cutting, or column counting.
- **SC-004**: Existing users who never pass `--json` see zero change: every existing table-output test passes unchanged.
- **SC-005**: Passing `--json` to any unsupported `get` subcommand fails 100% of the time with a non-zero exit.

## Assumptions

- **Field names** follow the user's example in camelCase (`containerId`, and by the same convention `displayName`, `maxApps`, `maxResources` for teams). The user's example used unquoted keys for readability; real output is standard JSON with quoted keys.
- **Same data as `describe`**: workload entries carry `describe`'s fields, not only the table's columns. `containerId` and `configHash` are given in full, because their shortened forms are presentation. `kind` uses the values printed today (`Application`, `Resource`). Routing, ports, health and other fields `describe` does not show are out of scope.
- **Container runtime optional**: `--json` reads everything from Shrine's state except `runningImage`, which it asks the container runtime for and leaves `null` when it can't; so the command works on a host without a running container engine.
- **Formatting**: The document may be indented for readability or compact; scripts must not depend on whitespace. Indented, human-readable formatting is the default choice.
- **Scope**: `describe`, `status`, and the `config-dir` / `state-dir` subcommands are out of scope. Team entries do not carry `describe team`'s extra detail (contact, registry user, allowed resource types, deployments summary); a team's workloads are available via `get deployed --team <name> --json`. A general `--output`/`-o` format selector (yaml, wide, etc.) is out of scope; `--json` is a standalone boolean flag.
- **Docs**: The CLI reference pages are regenerated from the command help, so documenting the flag in help text (FR-012) also updates the docs site.
