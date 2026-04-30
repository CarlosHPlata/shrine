# Feature Specification: Preserve Operator-Edited traefik.yml

**Feature Branch**: `004-preserve-traefik-yml`  
**Created**: 2026-04-30  
**Status**: Draft  
**Input**: User description: "Bug: shrine deploy overwrites traefik.yml when it already exists. On every deploy, Shrine regenerates traefik.yml unconditionally, discarding any manual edits. Shrine should treat traefik.yml as an operator-owned file once it exists — same policy already applied to files in dynamic/. Only generate it on first deploy when the file is absent."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator-Edited traefik.yml Survives Re-Deploys (Priority: P1)

An operator deploys Shrine for the first time on a new host. Shrine bootstraps the gateway by writing a default `traefik.yml` static configuration file. The operator then hand-edits `traefik.yml` to tune gateway behavior for their environment (for example, adjusting timeouts, adding entry points, enabling TLS, or wiring in a custom provider). On every subsequent `shrine deploy`, the operator's edits remain intact — Shrine does not regenerate or overwrite `traefik.yml` when it already exists on disk.

**Why this priority**: This is the core defect. Today, every deploy silently destroys the operator's gateway configuration, which can break production routing, downgrade security posture (e.g., revert TLS or auth tweaks), and erode trust in the tool. Without this fix, operators cannot safely customize the gateway at all.

**Independent Test**: Deploy Shrine on a fresh host so `traefik.yml` is created. Modify any field in the file (e.g., change a port or add a comment line). Run `shrine deploy` again. Verify the file's content is byte-for-byte identical to the operator's edited version, and that the deploy still completes successfully.

**Acceptance Scenarios**:

1. **Given** an existing `traefik.yml` with operator edits, **When** the operator runs `shrine deploy`, **Then** the file's content is unchanged on disk after the deploy completes.
2. **Given** an existing `traefik.yml`, **When** the operator runs `shrine deploy`, **Then** the deploy succeeds (the gateway plugin does not error out because the file is already present).
3. **Given** an existing `traefik.yml`, **When** the operator runs `shrine deploy` with changes to the Shrine plugin configuration that would alter the generated default (e.g., a different gateway port), **Then** the on-disk `traefik.yml` is still left unchanged, and the operator is responsible for reconciling the file by hand.

---

### User Story 2 - First Deploy Still Bootstraps a Working Gateway (Priority: P1)

An operator deploys Shrine on a host where `traefik.yml` does not yet exist. Shrine generates the default static configuration so the gateway starts cleanly without manual setup, exactly as it does today.

**Why this priority**: First-deploy bootstrapping is the existing onboarding path. The fix must not regress it — operators must still be able to install Shrine on a new host and have a working gateway without writing the static config by hand.

**Independent Test**: On a host with no `traefik.yml` present, run `shrine deploy`. Verify the file is created with the same default content the current implementation produces, and that the gateway container starts and serves traffic.

**Acceptance Scenarios**:

1. **Given** no `traefik.yml` exists in the routing directory, **When** the operator runs `shrine deploy` for the first time, **Then** Shrine writes a default `traefik.yml` and the gateway starts successfully.
2. **Given** the routing directory itself does not yet exist, **When** the operator runs `shrine deploy`, **Then** Shrine creates the routing directory and writes a default `traefik.yml` (current bootstrap behavior is preserved).

---

### User Story 3 - Operator Can Force Regeneration by Removing the File (Priority: P3)

An operator wants to discard their local edits to `traefik.yml` and return to the Shrine-managed default — for example, after an upgrade that changes the recommended template, or when troubleshooting a misconfiguration. The operator deletes `traefik.yml` and re-runs `shrine deploy`; Shrine treats the file as missing and writes a fresh default.

**Why this priority**: This is the natural, discoverable way to opt back into Shrine-managed defaults once the "preserve if present" rule is in place. It does not require new commands or flags — it falls out of the same logic as User Story 2 — but it is worth calling out so it is explicitly tested and documented.

**Independent Test**: With an existing `traefik.yml` in place, delete the file, then run `shrine deploy`. Verify a fresh default is written and matches the current generator output.

**Acceptance Scenarios**:

1. **Given** an existing `traefik.yml`, **When** the operator deletes the file and runs `shrine deploy`, **Then** Shrine regenerates a default `traefik.yml`.

---

### Edge Cases

- **`traefik.yml` exists but is empty or invalid YAML**: Shrine still treats the file as operator-owned and does not overwrite it. The gateway container may then fail to start; that failure surfaces from the gateway itself, not from Shrine silently fixing the file. Rationale: an empty file may be a deliberate operator choice (e.g., a placeholder before a config-management tool fills it in), and silent regeneration would re-introduce exactly the bug being fixed.
- **`traefik.yml` exists but is not a regular file** (e.g., a directory, device node, socket, or named pipe): Shrine still treats the path as "present" and does not touch it. Whatever lives at that path — first-deploy default, operator edit, or operator-created non-file — is the operator's responsibility. The gateway container will surface any resulting failure on its own.
- **`traefik.yml` exists as a symlink** (e.g., to a file managed by Ansible/Chef/etc.): Shrine treats the symlink's existence as "file is present" and does not touch it. This applies regardless of whether the symlink target exists — a broken symlink is still operator territory and Shrine MUST NOT write through it.
- **The routing directory does not exist on first deploy**: Shrine creates the directory and writes the default `traefik.yml`, matching existing behavior.
- **Concurrent edits during deploy**: out of scope — operators are expected not to hand-edit `traefik.yml` while a deploy is in progress.
- **Stat error on `traefik.yml`** (e.g., permission denied reading the path): the deploy fails with a clear error identifying the file and the underlying cause; Shrine does not silently fall back to overwriting.

## Clarifications

### Session 2026-04-30

- Q: When `traefik.yml` exists as a symlink whose target does not exist (a broken symlink), how should Shrine treat it on deploy? → A: Treat as "present"; Shrine does not care what the operator does with the file and must not write through the symlink.
- Q: When `traefik.yml` exists at the path but is not a regular file or symlink (e.g., directory, device, socket, named pipe), how should Shrine treat it? → A: Treat as "present" — anything at the `traefik.yml` path is operator-owned regardless of type or content; the gateway container surfaces any resulting failure.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: On deploy, Shrine MUST check whether `traefik.yml` exists in the gateway routing directory before generating it.
- **FR-002**: When `traefik.yml` is absent, Shrine MUST generate it using the current default-config logic so first-deploy bootstrapping continues to work without operator intervention.
- **FR-003**: When `traefik.yml` is present, Shrine MUST leave the file untouched — its content, mode, owner, and modification time MUST NOT change as a result of the deploy.
- **FR-004**: Shrine MUST continue to create the routing directory (and the `dynamic/` subdirectory) on deploy if they are absent, so first-deploy bootstrapping is unaffected.
- **FR-005**: Shrine MUST NOT require any new flag, env var, or config field for the preserve-on-exists behavior; it is the single, default policy for `traefik.yml`.
- **FR-006**: When Shrine skips regeneration because `traefik.yml` already exists, the deploy MUST log an observable signal (at info level or equivalent) indicating the file was preserved, so operators can confirm the new behavior in deploy output.
- **FR-007**: If a stat on `traefik.yml` fails for a reason other than "file does not exist", the deploy MUST fail with an error that names the file and the underlying cause; Shrine MUST NOT fall back to overwriting.
- **FR-008**: The preserve policy applies only to the `traefik.yml` static config file; Shrine-managed per-route files in `dynamic/` (one file per Shrine-managed route, with deterministic names) continue to be written and removed by Shrine as today.
- **FR-009**: The "exists" check MUST detect any entry at the `traefik.yml` path — regular file, symlink (regardless of target), directory, or any other file type — and treat it as operator-owned; Shrine MUST NOT follow symlinks when deciding whether to regenerate, and MUST NOT inspect or validate the file's type or content before deciding.

### Key Entities

- **traefik.yml (gateway static configuration)**: The Traefik static-configuration file that lives at the root of the gateway routing directory. Owned by Shrine on first deploy; owned by the operator from the moment it exists on disk. Contains entry points, providers, and (optionally) dashboard wiring.
- **Gateway routing directory**: The host directory mounted into the gateway container. Contains `traefik.yml` and a `dynamic/` subdirectory of per-route files. This spec changes the ownership policy of `traefik.yml` only; the directory and its `dynamic/` subtree are unchanged.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After this change ships, 100% of repeat deploys against a host that already has `traefik.yml` leave the file's content byte-for-byte unchanged.
- **SC-002**: First-deploy bootstrap on a clean host continues to produce a working gateway with no manual file authoring required (zero regression vs. current behavior).
- **SC-003**: Zero new configuration knobs are introduced — operators do not need to set any flag or env var to get the new behavior.
- **SC-004**: The deploy log clearly indicates, on every repeat deploy, whether `traefik.yml` was generated or preserved, so operators can verify the policy is in effect without inspecting file timestamps.
- **SC-005**: An operator who edits `traefik.yml` and runs ten consecutive deploys can describe the file's final state in one word: "unchanged."

## Assumptions

- The "operator" is whoever runs `shrine deploy` on the gateway host and has write access to the routing directory; no separate role/permission model is introduced for this feature.
- The semantics of "exists" mirror what the current Shrine codebase uses for the analogous decision in `dynamic/`: any entry at the file path counts as exists, regardless of file size, mode, type, or content validity. Symlinks are detected without following them, so a broken symlink still counts as present.
- "Preserve" means Shrine performs no write to the file — not "Shrine writes the same content back." This guarantees mode/owner/mtime are also untouched, and avoids any chance of partial writes.
- Operators who want to reset to the Shrine default do so by deleting `traefik.yml` and re-running deploy. No new "regenerate" subcommand is in scope.
- Plugin-config changes (e.g., changing the gateway port in Shrine's config) that would have produced a different generated `traefik.yml` are intentionally not reconciled into the existing file. The operator owns the file once it exists; reconciliation is their responsibility. This trade-off is accepted because (a) it matches the parallel policy for operator-added files in `dynamic/`, (b) the alternative — silently editing operator-owned config — is exactly the bug being fixed, and (c) any "merge" strategy would require defining what counts as an operator edit vs. a stale Shrine field, which is out of scope.
- Backwards compatibility: the change is transparent for first-time deploys (no `traefik.yml` yet) and for deploys against hosts where the operator has not edited the file. There is no migration step.
