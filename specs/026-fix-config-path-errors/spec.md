# Feature Specification: Field-Naming Config Path Errors That Always Stop the Command

**Feature Branch**: `026-fix-config-path-errors`
**Created**: 2026-08-24
**Status**: Draft
**Input**: GitHub issue [#37](https://github.com/CarlosHPlata/shrine/issues/37) — "fix: swallowed specsDir resolve error; tilde-expansion errors do not name the failing field"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The operator is told which path field failed (Priority: P1)

An operator keeps `specsDir: ~/manifests` and `teamsDir: ~/teams` in `config.yml` and runs shrine from an environment where the home directory cannot be determined — a cron job, a CI runner, a container, or a service unit with a stripped environment. Today the command fails with `expanding ~: $HOME is not set` and nothing else: the operator cannot tell whether `specsDir`, `teamsDir`, or the `--path` value is the one that needs fixing, and has to open the config and guess. After this change every subcommand that resolves a path-typed field fails with a single error that names the field whose value could not be resolved and the underlying cause — for example `resolving specsDir: expanding ~: $HOME is not set` — so the operator knows exactly which line to change.

**Why this priority**: This is the visible defect. Spec 003 (FR-005, SC-001) already promised an error that "identifies the failing field and the cause"; only the Traefik `routing-dir` field honours that promise today, while `specsDir` and `teamsDir` — the two fields every operator sets — do not. A diagnostic that omits the field it is about is the difference between a ten-second fix and a debugging session, especially for operators hitting this from a non-interactive environment where they cannot experiment.

**Independent Test**: With `specsDir: ~/manifests` in `config.yml` and the home directory unavailable, run `shrine deploy` (no `--path`) and assert the command exits non-zero with a single error containing both `specsDir` and the cause. Repeat with `teamsDir: ~/teams` and `shrine apply teams`, asserting the error contains `teamsDir`.

**Acceptance Scenarios**:

1. **Given** `config.yml` sets `specsDir: ~/manifests` and the home directory cannot be determined, **When** the operator runs `shrine deploy` without `--path`, **Then** the command exits non-zero with exactly one error that names `specsDir` and states the cause, and no deployment side effect occurs.
2. **Given** `config.yml` sets `teamsDir: ~/teams` and the home directory cannot be determined, **When** the operator runs `shrine apply teams` without `--path`, **Then** the error names `teamsDir` and the cause, and no team is written to state.
3. **Given** `teamsDir` is not set, `specsDir: ~/manifests` is set, and the home directory cannot be determined, **When** the operator runs `shrine apply teams` (which falls back to `specsDir`), **Then** the error names `specsDir` — the field that actually supplied the value the operator must fix — not `teamsDir`.
4. **Given** the operator passes `--path ~/manifests` on the command line and the home directory cannot be determined, **When** any subcommand that accepts `--path` runs, **Then** the error identifies the `--path` flag as the source of the value rather than blaming a config field the command never consulted.
5. **Given** `specsDir: ~/manifests` and no home directory, **When** the operator runs `shrine apply -f app.yml`, `shrine deploy team <name>`, `shrine deploy --dry-run`, or any `shrine generate` subcommand without `--path`, **Then** each fails with an error naming `specsDir` and the cause, and nothing is deployed, written to state, or generated on disk.
6. **Given** `specsDir: ~/manifests` and no home directory, **When** the operator runs `shrine teardown <team>`, **Then** the command fails with an error naming `specsDir` and the cause, and no container, network, route, or DNS entry is removed.
7. **Given** `specsDir` is an absolute path and the home directory cannot be determined, **When** any subcommand runs, **Then** no resolution error occurs and behaviour is unchanged from today — a value that needs no expansion never fails.

---

### User Story 2 - A resolution failure always stops the command, whichever stage hits it first (Priority: P2)

Shrine resolves `specsDir` in more than one place during a single command: the command layer resolves it to pick the manifest directory, and a later stage resolves it again while composing the deployment or teardown machinery. Today that later stage silently discards any resolution error and carries on with an empty directory value. The only reason this is harmless is that the command layer happens to fail first. If the order of those stages ever changes — a refactor, a new subcommand that skips the command-layer check, or `teardown`, which has no command-layer check at all — the command would proceed with a partially-resolved configuration, exactly what spec 003 FR-005 forbids. After this change, every stage that resolves a path surfaces the failure with the same field-naming error and creates nothing; no stage relies on an earlier one having already caught it.

**Why this priority**: There is no user-visible symptom today, so it ranks below Story 1 — but it is the guarantee that keeps Story 1 true. Without it, the "fails before side effects" promise is an accident of call order rather than a property of the system, and a routine refactor could quietly turn a clean failure into a deployment against the wrong directory.

**Independent Test**: Invoke the deployment-composition stage directly with a configuration whose `specsDir` cannot be resolved (bypassing the command-layer check) and assert it returns the field-naming error and constructs nothing — no observers, no gateway, no backends. Repeat for the teardown-composition stage. Then invoke the teardown-composition stage with `specsDir` absent altogether and assert it still succeeds.

**Acceptance Scenarios**:

1. **Given** a configuration whose `specsDir` is set but cannot be resolved, **When** the deployment-composition stage is invoked directly, **Then** it fails with the same field-naming error Story 1 describes and does not create any observer, gateway, container backend, routing backend, or engine.
2. **Given** a configuration whose `specsDir` is set but cannot be resolved, **When** the teardown-composition stage is invoked directly, **Then** it fails with the same field-naming error and creates nothing.
3. **Given** a configuration with no `specsDir` at all, **When** `shrine teardown <team>` runs, **Then** it proceeds exactly as today — teardown reads no manifests, so an absent `specsDir` is not a resolution failure for it.
4. **Given** a configuration whose `specsDir` is set and resolvable, **When** any subcommand runs, **Then** the resolved directory is identical to the one produced today (no regression).

---

### Edge Cases

- `--path` is given as an absolute path while `config.yml` holds `specsDir: ~/manifests` and the home directory is unavailable: the flag takes priority and needs no expansion, so the command proceeds; the unresolvable config value is never consulted and never reported.
- `teamsDir` is absolute and `specsDir` is `~`-prefixed with no home directory: `shrine apply teams` uses `teamsDir` and succeeds; `shrine deploy` uses `specsDir` and fails naming `specsDir`.
- Neither `--path` nor `specsDir` is set for a command that requires a directory (`deploy`, `apply`, `generate`): the existing "no specs directory: set --path/-p flag or specsDir in config.yml" guidance is unchanged — it already names the fields to set. "Not configured" and "configured but unresolvable" remain distinct messages.
- `teardown` with no `specsDir` configured: unchanged, proceeds (see Story 2, scenario 3). `teardown` with an unresolvable `specsDir`: fails, naming `specsDir`.
- The error must appear exactly once in the command output — the command layer and any later stage must not each report it.
- Dry-run deploys fail at the same point with the same error as real deploys.
- The Traefik `routing-dir` field already names itself in its error; it keeps doing so through the same field-naming rule as `specsDir` and `teamsDir` (the field is named once, by its exact config key), and its behaviour is otherwise unchanged.
- Values in the `~user` form (for example `~alice/specs`) are not expanded today and remain unchanged; they cannot trigger this error.
- Any future cause of resolution failure — not only an unavailable home directory — must produce the same field-naming shape of error.

## Requirements *(mandatory)*

### Functional Requirements

**Field-naming resolution errors**

- **FR-001**: When resolution of `specsDir` or `teamsDir` fails, every subcommand that resolves that field — `deploy`, `deploy team`, `deploy --dry-run`, `apply -f`, `apply teams`, `generate team`, `generate application`, `generate resource`, and `teardown` — MUST exit non-zero with an error that contains the name of the field whose value could not be resolved and the underlying cause (for example `resolving specsDir: expanding ~: $HOME is not set`).
- **FR-002**: The named field MUST be the source that actually supplied the value being resolved: when `teamsDir` falls back to `specsDir`, the error names `specsDir`; when the value came from the `--path` flag, the error identifies `--path`.
- **FR-003**: A resolution failure MUST be reported exactly once per command run, even though more than one stage of the command resolves the same field.

**Fail before side effects, at every stage**

- **FR-004**: A subcommand whose path resolution fails MUST NOT perform any side effect: no state is written, no manifest file is generated, no container, network, route, DNS entry, or gateway is created or removed, and no gateway configuration is written.
- **FR-005**: Every stage of a command that resolves a path-typed field MUST surface a resolution failure to its caller; no stage may discard the failure and continue with an empty or partially-resolved value. This MUST hold independently of whether an earlier stage of the same command has already checked the field.
- **FR-006**: For `teardown`, an absent `specsDir` (no value configured) MUST NOT be treated as a resolution failure; the command MUST continue to work without `specsDir` as it does today. Only a configured value that cannot be resolved fails the command.

**No regression**

- **FR-007**: For every configuration that resolves successfully today — absolute values, `~`-prefixed values with a home directory available, and relative values — the resolved directories MUST be identical after this change, and the existing "no specs directory" guidance for commands that require a directory MUST be unchanged.

**Regression coverage**

- **FR-008**: Automated unit coverage MUST assert, for each of `specsDir`, `teamsDir`, the `teamsDir`→`specsDir` fallback, and the `--path` source, that a resolution failure produces an error naming that source and the cause; that the deployment-composition and teardown-composition stages refuse to proceed (return the field-naming error, construct nothing) when `specsDir` is configured but unresolvable; and that the teardown-composition stage still succeeds when `specsDir` is absent. This closes the coverage gap for spec 003 FR-005 / SC-001.

### Key Entities

- **Path-typed configuration field**: A `config.yml` value that names a directory — `specsDir`, `teamsDir`, and the Traefik `routing-dir` — which may be absolute, `~`-prefixed, or relative and must be turned into an absolute directory before use (spec 003).
- **Value source**: Where the value being resolved came from: the `--path` flag, the field itself, or a fallback field (`teamsDir` falling back to `specsDir`). The error names the source, because that is what the operator must edit.
- **Resolution failure**: A configured value that cannot be turned into an absolute directory — today, because the home directory cannot be determined. Distinct from "not configured", which has its own existing message and, for `teardown`, is not an error at all.
- **Side effect**: Any change a subcommand makes outside its own output: state writes, generated manifests, gateway configuration files, containers, networks, routes, DNS entries.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of subcommand runs in which a configured `specsDir` or `teamsDir` cannot be resolved exit non-zero with a single error naming that field (or `--path`) and the cause, before any side effect — across `deploy`, `deploy team`, dry-run, `apply -f`, `apply teams`, all `generate` subcommands, and `teardown`.
- **SC-002**: An operator can identify which configuration line to fix from the error message alone, without opening `config.yml` or re-running the command with different inputs — the field name is present in every resolution error.
- **SC-003**: Zero regressions: every configuration that resolves today resolves to the same directories, `teardown` without a configured `specsDir` continues to succeed, and the existing integration suite passes unchanged.
- **SC-004**: The "fails before side effects" guarantee no longer depends on call order: the composition stages for deployment and teardown each refuse to proceed on an unresolvable `specsDir` when exercised in isolation, and that behaviour is pinned by automated tests so spec 003 FR-005 / SC-001 have coverage.

## Assumptions

- "The failing field" means the source that supplied the value being resolved. Naming `teamsDir` when the value actually came from the `specsDir` fallback would send the operator to the wrong line, so the fallback names `specsDir`; likewise a value from `--path` names the flag. The issue text lists only `specsDir` and `teamsDir`; extending to the flag source is the minimal step that keeps the message truthful.
- `teardown` is not required to have a `specsDir`. It has no `--path` flag, reads no manifests, and works today without one (the existing teardown integration tests run it that way); it consults `specsDir` only to derive the default gateway routing directory. Making `specsDir` mandatory for `teardown` would be a breaking change outside a bug fix, so "absent" stays tolerated and only "configured but unresolvable" fails.
- The `generate` subcommands are included even though the issue names only `deploy` and `apply`: they resolve the same field via the same path, and spec 003 FR-004 requires the rules to apply uniformly to every path-typed field and every subcommand that uses it.
- The exact wording of the message is not mandated beyond containing the field (or flag) name and the underlying cause; the example `resolving specsDir: expanding ~: $HOME is not set` is illustrative. The Traefik `routing-dir` message already names its field; it is brought under the same field-naming rule so all three fields share one message shape, with the field named exactly once by its config key.
- An unavailable home directory is the only realistic resolution failure today, but the requirements apply to any cause of resolution failure so they remain true if resolution grows new failure modes.
- When `specsDir` is absent, `teardown` derives a gateway routing directory relative to the working directory unless `routing-dir` is set. That is pre-existing behaviour and out of scope here.
- This feature changes error propagation and error wording only; it does not change the resolution rules (priority order, `~` expansion, relative-path handling, idempotence) defined in spec 003.
