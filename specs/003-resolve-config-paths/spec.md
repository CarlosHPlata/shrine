# Feature Specification: Expand `~` Consistently Across Path-Typed Config Fields

**Feature Branch**: `003-resolve-config-paths`  
**Created**: 2026-04-30  
**Status**: Draft  
**Input**: User description: "Bug: shrine deploy crashes when routing-dir is a relative path. If routing-dir is set as a relative path in the config, Shrine crashes at deploy time. All path fields in the config (specsDir, routing-dir, certsDir) should be resolved to absolute paths at config load time, expanding ~ and relative references against the user's home directory."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Tilde-Prefixed Paths Work in Every Config Field (Priority: P2)

A shrine operator writes their config with `~`-prefixed paths (e.g., `specsDir: ~/shrine/specs`, `routing-dir: ~/shrine/routing`) so the same config file is portable between machines with different home directories. When any shrine command reads the config, every path field with a `~` prefix is expanded to the operator's home directory and the command behaves identically regardless of which path field carried the prefix.

**Why this priority**: Tilde expansion already works for `specsDir` and `teamsDir`, but not for the Traefik plugin's `routing-dir`. The inconsistency surprises operators who reasonably expect every path-typed field in the same file to follow the same rules. Without this, copy-pasting `~/...` values between fields produces silent failures or crashes that are hard to diagnose.

**Independent Test**: Write a config in which every path field uses a `~/...` value, run any shrine command that touches each of those fields (e.g., `shrine deploy`), and confirm that every path is expanded to the home directory and the command succeeds.

**Acceptance Scenarios**:

1. **Given** `routing-dir: ~/traefik` in the config, **When** a shrine subcommand reads that field, **Then** the field is treated as an absolute path under the operator's home directory.
2. **Given** a path field set to exactly `~` (no slash, no remainder), **When** a shrine subcommand reads that field, **Then** the field resolves to the operator's home directory itself.
3. **Given** a path field whose value is already absolute (e.g., `/etc/shrine/specs`), **When** a shrine subcommand reads that field, **Then** the value is left unchanged — the rule never rewrites absolute paths.


### Edge Cases

- A path field is empty (the operator omitted it). → Field stays empty; the existing "field not set" behavior (e.g., the existing "no specs directory" error from `--path`/`specsDir` resolution) is unchanged.
- A path is exactly `~`. → Resolves to the home directory.
- A path is already absolute (`/etc/shrine/...`). → Left unchanged.
- The same config file is loaded twice in the same process. → Resolution is idempotent: re-resolving an already-absolute path does not move it.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: All path-typed fields in the configuration MUST follow the same path-resolution rules (defined in FR-002 and FR-003). No field may be treated differently based on which field name carries the value.
- **FR-002**: For each path-typed field in the config, the system MUST expand a leading `~` (alone) or `~/` to the operator's home directory.
- **FR-003**: For each path-typed field in the config, the system MUST leave absolute values unchanged.
- **FR-004**: The rules defined in FR-001 through FR-003 MUST apply uniformly to every path-typed field currently in the configuration — `specsDir`, `teamsDir`, and the Traefik plugin's `routing-dir` — and to any new path-typed field added in the future (for example, a `certsDir` field if introduced later).
- **FR-005**: If path resolution fails for any field (for example, because the home directory cannot be determined), the failing subcommand MUST surface a single, clear error that identifies the failing field and the cause, and MUST NOT proceed with side effects using a partially-resolved configuration.
- **FR-006**: Resolution MUST be idempotent: applying the rules to an already-absolute, already-resolved path MUST return the same path unchanged.

### Key Entities

- **Configuration file**: The shrine `config.yml` (or equivalent) read at startup. Contains zero or more path-typed fields whose values can be absolute, `~`-prefixed, or relative.
- **Operator home directory**: The directory returned by the operating system as the current user's home directory. Used as the anchor for `~` expansion and for resolving relative config paths.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: When the home directory cannot be resolved, the affected subcommand fails before performing any side effects, with an error that names the offending field; no subcommand crashes mid-flight from this cause.
- **SC-002**: No regression for absolute or `~`-prefixed paths: every config that worked before this change continues to produce the same absolute paths after the change.

## Assumptions

- "Relative" is interpreted by the operating system's standard rules (a path is relative if it is not absolute and does not start with `~` after expansion).
