# Feature Specification: Preserve Operator-Edited Per-App Routing Files

**Feature Branch**: `009-preserve-app-configs`
**Created**: 2026-05-01
**Status**: Draft
**Input**: User description: "As a user operator using shrine CLI I found annoying that the traefik plugin files generated for app override my personal configs for the apps, I want the plugin just to create the files once, when the file does not exists, if the file already exists the CLI should not re-write it, so my personal changes are not overrided."

## Clarifications

### Session 2026-05-01

- Q: When an app is removed from the manifest, what should Shrine do with its per-app routing file? → A: Preserve the file on app removal but emit a loud warning naming the file the operator must clean up by hand.
- Q: When a stat on a per-app routing file fails (e.g., permission denied, I/O error) mid-deploy, how should Shrine treat that app and the rest of the deploy run? → A: Treat the file as present (operator-owned), log a warning that names the file and the underlying cause, and continue the deploy. The per-app file is not written and Shrine does not abort or exit non-zero solely on this; Shrine's deploy success is governed by Docker/container outcomes, not by gateway-plugin file outcomes once the first template has shipped.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator-Edited Per-App Routing File Survives Re-Deploys (Priority: P1)

An operator deploys an application through Shrine. The traefik plugin writes a per-app routing file (one file per Shrine-managed application) into the gateway's dynamic configuration directory. The operator then hand-edits that file to tune routing behavior for the app — for example, adding a custom middleware, attaching headers, tightening a router rule, wiring a TLS option, or hand-crafting a sticky-session policy that Shrine does not yet expose at the manifest level. On every subsequent `shrine deploy`, the operator's edits remain intact — Shrine does not regenerate or overwrite the per-app file when it already exists on disk.

**Why this priority**: This is the core defect. Today, every deploy silently destroys any operator customization to per-app routing files, which can break carefully-tuned routing, downgrade security posture (e.g., revert a header-stripping middleware), and erode trust in the tool. Without this fix, operators cannot safely customize per-app routing at all and are forced to either fork the tool or accept that only manifest-derivable behavior is reachable.

**Independent Test**: Deploy an app through Shrine so its per-app routing file is created. Modify any field in that file (e.g., add a comment, add a middleware, change a header). Run `shrine deploy` again. Verify the file's content is byte-for-byte identical to the operator's edited version, and that the deploy still completes successfully.

**Acceptance Scenarios**:

1. **Given** an existing per-app routing file with operator edits, **When** the operator runs `shrine deploy`, **Then** the file's content is unchanged on disk after the deploy completes.
2. **Given** an existing per-app routing file, **When** the operator runs `shrine deploy`, **Then** the deploy succeeds (the gateway plugin does not error out because the file is already present).
3. **Given** an existing per-app routing file, **When** the operator runs `shrine deploy` with manifest changes that would alter the generated default (e.g., a different domain, a new alias, a different path prefix, a different service port), **Then** the on-disk per-app file is still left unchanged, and the operator is responsible for reconciling the file by hand or by deleting it and redeploying.

---

### User Story 2 - First Deploy of an App Still Bootstraps a Working Route (Priority: P1)

An operator deploys an app for the first time. Shrine generates the per-app routing file from the manifest so the gateway picks up the route without manual setup, exactly as it does today.

**Why this priority**: First-deploy bootstrapping is the existing onboarding path for every Shrine-managed app. The fix must not regress it — operators must still be able to declare an app in a manifest and have a working route appear without writing the routing file by hand.

**Independent Test**: On a host where no per-app file exists for a given app, run `shrine deploy` with that app declared in a manifest. Verify the file is created with the same default content the current implementation produces, and that traffic to the configured host is routed to the app.

**Acceptance Scenarios**:

1. **Given** no per-app routing file exists for an app, **When** the operator runs `shrine deploy`, **Then** Shrine writes the per-app file from the current manifest and the route becomes active.
2. **Given** the dynamic routing directory itself does not yet exist, **When** the operator runs `shrine deploy`, **Then** Shrine creates the directory and writes the per-app file (current bootstrap behavior is preserved).

---

### User Story 3 - Operator Can Force Regeneration by Removing the File (Priority: P2)

An operator wants to discard their local edits to a per-app routing file and return to the Shrine-managed default — for example, after editing the manifest (changing an alias, port, or path prefix) and wanting that change to take effect, or when troubleshooting a misconfiguration. The operator deletes the per-app file and re-runs `shrine deploy`; Shrine treats the file as missing and writes a fresh default from the current manifest.

**Why this priority**: This is the natural, discoverable way to opt back into Shrine-managed defaults once the "preserve if present" rule is in place. It does not require new commands or flags — it falls out of the same logic as User Story 2 — but it is worth calling out so it is explicitly tested and documented. It is the operator's single-step recipe for "apply my latest manifest changes."

**Independent Test**: With an existing per-app routing file in place, delete it, then run `shrine deploy`. Verify a fresh default is written from the current manifest and matches the current generator output.

**Acceptance Scenarios**:

1. **Given** an existing per-app routing file, **When** the operator deletes the file and runs `shrine deploy`, **Then** Shrine regenerates a default per-app file from the current manifest.
2. **Given** an existing per-app routing file with stale content (e.g., the manifest now declares a new alias), **When** the operator deletes the file and runs `shrine deploy`, **Then** the freshly written file reflects the current manifest, including the new alias.

---

### Edge Cases

- **Per-app file exists but is empty or invalid YAML**: Shrine still treats the file as operator-owned and does not overwrite it. The gateway may then ignore the file or surface a parse error; that failure surfaces from the gateway itself, not from Shrine silently fixing the file. Rationale: an empty file may be a deliberate operator choice (e.g., a placeholder), and silent regeneration would re-introduce exactly the bug being fixed.
- **Per-app file exists but is not a regular file** (e.g., a directory, device node, socket, or named pipe): Shrine treats the path as "present" and does not touch it. Whatever lives at that path is the operator's responsibility.
- **Per-app file exists as a symlink** (e.g., to a file managed by Ansible/Chef/etc.): Shrine treats the symlink's existence as "file is present" and does not touch it. This applies regardless of whether the symlink target exists — a broken symlink is still operator territory and Shrine MUST NOT write through it.
- **Manifest changes that the operator expects to take effect but do not** (e.g., the operator changes the app's domain, adds an alias, or flips `stripPrefix`): the on-disk file is left untouched and the deploy log clearly signals that the file was preserved, so the operator can recognize that they need to delete the file (or hand-edit it) for the change to land. Silently re-applying manifest changes would re-introduce the bug being fixed.
- **App removed from manifest**: when an app is removed from the manifest, Shrine MUST NOT delete the corresponding per-app routing file. Instead, it MUST emit a loud warning that names the file the operator must clean up by hand. This applies symmetrically to the write policy: once the file exists on disk, it is operator-owned, regardless of whether the backing app is still declared. The trade-off — that the orphan file may keep routing traffic to a now-gone backend until the operator deletes it — is surfaced by the warning so operators are not silently exposed to it.
- **Stat error on the per-app file** (e.g., permission denied reading the path, I/O error): Shrine treats the file as present (operator-owned), does not write the file, and logs a warning that names the file path and the underlying error cause. The deploy continues with the rest of the apps and does NOT abort or exit non-zero solely because of a per-app stat error. Rationale: the traefik plugin is one of several plugins; Shrine's primary deploy responsibility is Docker/container status, not per-app gateway-file outcomes once the first template has shipped. Forcing a full-deploy failure on a file that is, by policy, operator-owned would punish operators for state Shrine does not control. The warning surfaces the issue so the operator can investigate.
- **Two apps that resolve to the same per-app file path**: out of scope — Shrine's per-app file naming is already deterministic and unique per `(team, service)` tuple. This spec does not change that.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: On deploy, before writing a per-app routing file, Shrine MUST check whether the file exists at its target path in the gateway dynamic routing directory.
- **FR-002**: When the per-app routing file is absent, Shrine MUST generate it from the current manifest using the existing per-app generation logic so first-deploy bootstrapping continues to work without operator intervention.
- **FR-003**: When the per-app routing file is present, Shrine MUST leave the file untouched — its content, mode, owner, and modification time MUST NOT change as a result of the deploy.
- **FR-004**: Shrine MUST continue to create the dynamic routing directory on deploy if it is absent, so first-deploy bootstrapping is unaffected.
- **FR-005**: Shrine MUST NOT require any new flag, env var, or manifest field to enable the preserve-on-exists behavior; it is the single, default policy for per-app routing files.
- **FR-006**: When Shrine skips regeneration because a per-app routing file already exists, the deploy MUST log an observable signal (at info level or equivalent) that names the app and indicates the file was preserved, so operators can confirm the new behavior in deploy output and recognize when manifest changes did not propagate.
- **FR-007**: If a stat on a per-app routing file fails for a reason other than "file does not exist" (e.g., permission denied, I/O error), Shrine MUST treat the file as present (operator-owned) and MUST NOT write the per-app file. The failure MUST be surfaced as a warning-level log signal that names the file and the underlying cause. Shrine MUST NOT abort the deploy run, MUST NOT exit non-zero, and MUST NOT skip subsequent apps solely on the basis of this stat error; per-app gateway-file failures do not gate deploy success.
- **FR-008**: The "exists" check MUST detect any entry at the per-app file path — regular file, symlink (regardless of target), directory, or any other file type — and treat it as operator-owned; Shrine MUST NOT follow symlinks when deciding whether to regenerate, and MUST NOT inspect or validate the file's type or content before deciding.
- **FR-009**: When an app is removed from the manifest, Shrine MUST NOT delete the corresponding per-app routing file if it exists on disk; instead, the deploy MUST emit a warning-level log signal that names the file and instructs the operator to delete it by hand to fully tear down the route. If the per-app file does not exist (e.g., the app was never deployed, or the operator already removed it), the warning MUST NOT be emitted and the deploy MUST treat the removal as a no-op for that app.
- **FR-010**: The preserve policy MUST be evaluated independently per app: a deploy that involves multiple apps MUST preserve any per-app file that already exists and MUST write fresh files for any per-app file that is absent, in a single deploy run.
- **FR-011**: A `shrine deploy` run's overall success/failure status MUST be determined by Docker/container outcomes, not by per-app gateway-file outcomes. Per-app preserve-skips, orphan-file warnings (FR-009), and stat-error warnings (FR-007) MUST surface in the deploy log without changing the run's exit code or aborting subsequent apps.

### Key Entities

- **Per-app routing file**: A YAML file in the gateway's dynamic routing directory, one per Shrine-managed application, named deterministically from the application's `(team, service)` tuple. Today it contains the router rule, service backend, and any alias routers / strip middleware derived from the manifest. Owned by Shrine on first deploy; owned by the operator from the moment it exists on disk.
- **Gateway dynamic routing directory**: The directory inside the gateway routing directory where per-app routing files live. Shrine creates this directory on first deploy. This spec changes the lifecycle policy for the per-app files inside it: Shrine writes a per-app file only on first creation (preserve-on-exists) and never deletes one it finds on disk — orphan files left behind by app removal are flagged via a deploy-log warning and cleaned up by the operator. Directory creation is unchanged.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After this change ships, 100% of repeat deploys against a host that already has per-app routing files leave those files' content byte-for-byte unchanged.
- **SC-002**: First-deploy bootstrap of any app continues to produce a working route with no manual file authoring required (zero regression vs. current behavior).
- **SC-003**: Zero new configuration knobs are introduced — operators do not need to set any flag, env var, or manifest field to get the new behavior.
- **SC-004**: The deploy log clearly indicates, on every repeat deploy, whether each per-app routing file was generated or preserved, so operators can verify the policy is in effect and notice when their manifest changes did not propagate, without inspecting file timestamps. When an app removal leaves an orphan per-app file behind, the deploy log MUST surface a warning that names the file path so the operator can find and delete it without inspecting the directory by hand.
- **SC-005**: An operator who edits a per-app routing file and runs ten consecutive deploys can describe the file's final state in one word: "unchanged."
- **SC-006**: An operator who wants their latest manifest changes to take effect for a specific app can do so in a single, discoverable step (delete the per-app file, redeploy), with no documentation lookup beyond the preserve-signal hint in the deploy log.
- **SC-007**: A `shrine deploy` run that has a stat error on one or more per-app routing files (or an orphan-file warning from a removed app) still deploys all healthy apps end-to-end and exits zero so long as the underlying Docker/container operations succeed; the gateway-file issues are visible in the deploy log without being deploy blockers.

## Assumptions

- The "operator" is whoever runs `shrine deploy` on the gateway host and has write access to the dynamic routing directory; no separate role/permission model is introduced for this feature.
- The semantics of "exists" mirror the policy already adopted for the static `traefik.yml` config in spec 004: any entry at the file path counts as exists, regardless of file size, mode, type, or content validity. Symlinks are detected without following them, so a broken symlink still counts as present.
- "Preserve" means Shrine performs no write to the file — not "Shrine writes the same content back." This guarantees mode/owner/mtime are also untouched and avoids any chance of partial writes.
- Operators who want to reset to the Shrine default for a given app do so by deleting that app's per-app routing file and re-running deploy. No new "regenerate" subcommand or `--force` flag is in scope.
- Manifest changes (e.g., changing the domain, port, alias list, path prefix, or `stripPrefix` of an app) that would have produced a different generated per-app file are intentionally not reconciled into the existing file. The operator owns the file once it exists; reconciliation is their responsibility (via delete-and-redeploy or hand-edit). This trade-off is accepted because (a) it is the symmetric extension of the policy adopted for `traefik.yml` in spec 004, (b) the alternative — silently editing operator-owned config — is exactly the bug being fixed, (c) any "merge" strategy would require defining what counts as an operator edit vs. a stale Shrine field, which is out of scope, and (d) the per-deploy log signal (FR-006 / SC-004) makes the trade-off observable so operators are not surprised.
- The remove path on app deletion (FR-009) is symmetric with the write path: once a per-app routing file exists, it is operator-owned and Shrine does not delete it automatically. The orphan-route risk (the gateway may keep routing traffic to a now-gone backend) is mitigated by the warning-level deploy-log signal that names the file so the operator can delete it by hand. This trade-off was chosen over silent cleanup because (a) silent cleanup would destroy operator edits the operator may have invested significant effort in, (b) the warning makes the operator's required action observable on every deploy until the file is removed, and (c) the operator is the only party who can decide whether the orphan file's content (which may include hand-tuned middleware, headers, or other routing logic) should be preserved, archived, or discarded.
- Backwards compatibility: the change is transparent for first-time deploys (no per-app file yet) and for operators who have never hand-edited a per-app file and re-deploy with an unchanged manifest (the on-disk content already matches what would be generated). There is no migration step. Operators with existing per-app files who *have* edited their manifest after the deploy that wrote those files will, after this change ships, observe that the manifest change does not propagate until they delete the file and redeploy — this is the new expected behavior, surfaced via the deploy-log preserve signal.
- Spec 004's FR-008 ("Shrine-managed per-route files in `dynamic/` continue to be written and removed by Shrine as today") is intentionally superseded by this feature on **both** the *write* axis (FR-001/FR-003: preserve on exists) and the *remove* axis (FR-009: never auto-delete; warn and defer to operator). After this feature ships, Shrine never modifies an existing per-app routing file — write or delete — outside of the first-deploy bootstrap path.
- Shrine's deploy responsibility is Docker/container management; the traefik plugin is one of several plugins and its per-app file outcomes are secondary signals. Once the first per-app template ships, the file becomes operator-owned and any subsequent gateway-file anomaly (preserve-skip, orphan, stat error) is surfaced as a deploy-log signal without gating deploy success (FR-011). This trade-off is accepted because forcing a full-deploy failure on operator-owned state would conflate two different concerns and would degrade the operator UX in the common case where Docker is healthy and only the gateway file is in an unexpected state.
