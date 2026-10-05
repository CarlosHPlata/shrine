# Contract: Spec Amendments

What lands in the existing specs, and in what form. Section 1 is verbatim: it is the text every other document refers to. Sections 2–3 fix the note formats. Sections 4–6 state, for each amended statement, what it must say afterwards; the implementer writes the final sentence, and the meaning given here is binding.

Evidence for every "must say" is in [research.md](../research.md) (F-numbers in brackets).

## 1. The canonical statement (verbatim, into spec 009)

Inserted in `specs/009-preserve-app-configs/spec.md` after the Amendments section (§2) and before `## Clarifications`.

Anchor: `#generated-gateway-file-lifecycle-canonical`

```markdown
## Generated Gateway File Lifecycle (Canonical)

> Added 2026-10-05 by [spec 030](../030-reconcile-spec-docs/spec.md). This is the single statement of how Shrine treats the files it generates for the gateway. Other specs and the docs refer here instead of restating it. Where an older statement in any spec disagrees with this section, this section is correct.

**Rule.** Shrine writes a covered file only when nothing exists at its path. Once something exists there, no deploy and no teardown modifies or deletes it. Presence is the only test (FR-008): Shrine keeps no record of who wrote a file and does not distinguish a file it generated on an earlier deploy from one an operator created or edited. "Operator-owned" and "preserved", wherever a spec or a log line uses them, mean "already exists".

**Covered files.**

| File | Written | Afterwards | Established by |
|---|---|---|---|
| Per-application routing file, `dynamic/<team>-<name>.yml` | When a routed application is deployed and the file is absent | Never modified, never deleted. Tearing down the team leaves the file and warns (FR-009) | This spec |
| Gateway static configuration, `traefik.yml` | When the gateway is deployed and the file is absent | Never modified, never deleted | Spec 004 |

**Not covered.**

- The dashboard route file, `dynamic/__shrine-dashboard.yml`, is also written only when absent and left untouched while the dashboard stays configured, but Shrine deletes it on the first deploy after the dashboard is removed from the configuration (spec 024).
- Files an operator adds to the routing directory are never read, modified, or deleted.

**What does not propagate.** Changing any input of a file that already exists has no effect on that file:

| File | Inputs |
|---|---|
| Per-application routing file | `routing.domain`, `routing.pathPrefix`, the application's `port`; adding or removing an alias; an alias's `host`, `pathPrefix`, `stripPrefix`, or `tls` |
| Gateway static configuration | the gateway `port`; adding, changing, or removing `tlsPort`; enabling the dashboard or changing its port |
| Dashboard route file | the dashboard credentials |

The gateway container is not a file and does follow the configuration: its host port bindings are recreated on the next deploy. After a port change the container and a preserved static configuration can therefore disagree. Shrine warns about that for `tlsPort` (spec 011 FR-008) and not for the other ports.

**Applying a change.** Delete the file and redeploy — Shrine regenerates it from the current manifest and configuration — or edit the file by hand. There is no flag or command that forces regeneration (FR-005). A regenerated static configuration takes effect when the gateway container next starts; a deploy restarts it only if the container's own configuration changed.

**Signals.** Every deploy reports each covered file as generated or preserved (FR-006; spec 004 FR-006). Tearing down a team reports each per-application file it leaves behind (FR-009).
```

**How others refer to it.** From another spec: `[generated gateway file lifecycle](../009-preserve-app-configs/spec.md#generated-gateway-file-lifecycle-canonical)`. From `AGENTS.md`: the same target with the path `specs/009-preserve-app-configs/spec.md#…`. The docs site does not link into `specs/`; it carries the operator-facing version ([docs-pages.md](docs-pages.md) §1), which must agree with this text on every fact.

## 2. Note formats

**Amendments section** — one per amended spec, directly after the header block (before `## Clarifications` or `## User Scenarios`, whichever comes first):

```markdown
## Amendments

- **2026-10-05 — [spec 030](../030-reconcile-spec-docs/spec.md)**: <one sentence on why>. Amended: <identifiers and locations>. <Terminology note, where §4 calls for one.>
```

**Amendment note** — the statement is rewritten to be true, and the note sits directly beneath it, indented to the list item:

```markdown
- **FR-009**: <corrected requirement, linking the canonical statement>
  > *Amended 2026-10-05 by spec 030. Originally:* "<original text, verbatim>"
```

**Descope note** — the original text stays; the identifier is marked; the note says what shipped and where the rest is tracked:

```markdown
- **FR-013** *(descoped 2026-10-05)*: <original text, unchanged>
  > *Descoped by spec 030.* **Shipped:** <…>. **Not shipped:** <…>. Tracked in [`specs/progress.md`](../progress.md) under Known Gaps.
```

Use *(partly descoped 2026-10-05)* when part of the requirement is delivered.

**Descoped task** — follows the `[~]` convention already used in spec 018's task list:

```markdown
- [~] T029 [P] DESCOPED (spec 030, 2026-10-05) — <original text, unchanged>. <One sentence: what is true instead.>
```

Rules for all formats: the identifier never changes; the original wording is always recoverable from the file; clarification Q&A records are never edited.

## 3. Known-gaps entries (verbatim, into `specs/progress.md`)

Appended to the existing `## Known Gaps` list:

```markdown
- **The preview does not print resolved environment or exports.** `shrine deploy --dry-run` resolves every value to a placeholder (`[GENERATED]`, `[VAULT:<path>]`, `[PORT]`) without generating secrets or reading the vault, but prints only the deploy order and the container, network, and route operations — never a container's env or a resource's exported outputs. Descoped from `specs/021-resource-env-output-split/` FR-013 and `specs/015-infisical-secrets-vault/` US3 scenario 1 by `specs/030-reconcile-spec-docs/`. To deliver: print both from the dry-run container backend, keeping secret values as placeholders.
- **A non-boolean `tls` on an alias is rejected with a decoder error, not a named one.** The message gives the file and line (`cannot unmarshal !!str … into bool`) but not the application or alias index that `specs/012-tls-alias-routers/` FR-004 asked for. The decoder also accepts the YAML 1.1 boolean words (`yes`, `no`, `on`, `off`, `y`, `n`) as booleans even when quoted, so `tls: "yes"` is not rejected. Descoped by `specs/030-reconcile-spec-docs/`. To deliver: validate the field's YAML node type before decoding and name `spec.routing.aliases[N].tls`.
```

## 4. Preserve-policy amendments (User Story 1)

"Canonical" below means a link to the §1 anchor. Every row uses the amendment-note format unless it says otherwise.

| Spec | Statement | Must say afterwards |
|---|---|---|
| 004 | FR-008 | The policy in this spec covers `traefik.yml`. Per-route files in `dynamic/` were made write-once by spec 009 and are no longer rewritten or removed by Shrine — canonical. |
| 006 | Edge case "Operator removes or changes an alias and re-deploys" | Nothing changes while the per-application file exists. The alias's router is removed or updated when the file is regenerated (delete and redeploy) or edited — canonical. |
| 006 | FR-009 | A routing file generated after an alias is removed from the manifest MUST NOT contain that alias's router. An existing file is not rewritten — canonical. |
| 006 | SC-004 | Removing the alias, deleting the per-application file, and redeploying removes the route within that deploy. Without deleting or editing the file, the alias keeps resolving. |
| 006 | SC-001 *(added during implementation)* | A second hostname on an existing application needs the `aliases` entry, deletion of the application's routing file, and a normal deploy. An existing file is not rewritten — canonical. |
| 008 | US1 Independent Test, second half | The step that flips `stripPrefix` includes deleting the per-application routing file before redeploying. |
| 008 | Edge case "Operator changes `stripPrefix` … and re-deploys" | The router and middleware match the new value in a file generated afterwards; an existing file is unchanged — canonical. |
| 008 | FR-007 | A routing file generated after `stripPrefix` changes MUST reflect the new value and MUST NOT contain a strip middleware the new value does not call for. An existing file is not rewritten — canonical. |
| 008 | SC-001 | Resolved by adding `stripPrefix: false`, deleting the application's routing file, and redeploying; no code or container change is required. |
| 011 | Amendments section | Terminology note: in this spec "Shrine-generated" means "generated in this deploy because the file was absent", and "operator-preserved" / "operator-edited" mean "already exists". Shrine cannot tell the two apart — canonical. Covers the clarification record, the entry-points entity, and the spec-004 assumption without editing them. |
| 011 | Edge case "`tlsPort` removed between deploys" | The container is recreated without the `443/tcp` mapping. An existing `traefik.yml` is left untouched and keeps its `websecure` entrypoint until the operator deletes or edits the file — canonical. [F2] |
| 011 | FR-003 | When `tlsPort` is set AND `traefik.yml` does not yet exist, the file Shrine generates MUST declare `websecure` at `:443`. |
| 011 | SC-003 | On a host with no `traefik.yml`, one config line and one deploy suffice. On an existing deployment the operator must also delete `traefik.yml` (Shrine regenerates it in that deploy) or add the entrypoint by hand. |
| 012 | Amendments section | Terminology note: "operator-preserved" and "operator-owned" mean "already exists" — canonical. Covers the edge cases, FR-007, FR-008, and the spec-009 assumption without editing them. |
| 012 | US2 acceptance scenario 2 | The operator removes `tls: true`, deletes the application's routing file, and redeploys; the regenerated router is plain. (Matches spec 029's revert scenario.) |
| 012 | SC-005 | Removing `tls: true`, deleting the per-application file, and redeploying yields a plain router within that deploy. No hand-editing is needed; without deleting the file it is preserved — canonical. |
| 012 | SC-001 *(added during implementation)* | HTTPS on an existing alias needs the `tls: true` line, deletion of the application's routing file, and a normal deploy. An existing file is not rewritten — canonical. |

Spec 009 itself gains the §1 section and an Amendments entry. Its existing requirements, edge cases, and assumptions about the *write* path already agree with the canonical statement and are not touched.

## 5. Orphan-warning amendments in spec 009 (User Story 3) [F5]

| Statement | Must say afterwards |
|---|---|
| Edge case "App removed from manifest" | When a team is torn down, Shrine does not delete the per-application routing files of the applications it removes; it warns, naming each file. Removing an application from the manifest and deploying does not produce the warning — deploy does not look for routing files whose application is gone. |
| FR-009 | On `shrine teardown <team>`, Shrine MUST NOT delete an application's routing file; for each such file that exists it MUST emit a warning naming the file and telling the operator to delete it by hand. No file, no warning. A deploy emits no orphan warning. |
| FR-011 | Deploy success is determined by container outcomes; preserve-skips and stat-error warnings surface in deploy output without changing the exit code. Orphan warnings surface in teardown output and likewise do not fail the teardown. |
| Key entity "Gateway dynamic routing directory" | Orphan files are flagged by a teardown warning. |
| SC-004, second sentence | When a teardown leaves a per-application file behind, the teardown output names the file path. |
| SC-007 | The parenthetical about orphan warnings is removed; the criterion is about stat errors on deploy. |
| Assumption on the remove path | The mitigation is a warning at teardown that names the file. It is emitted once, at that moment, and is not repeated on later deploys. |
| Assumption on deploy responsibility | Preserve-skips and stat errors are deploy signals; the orphan warning is a teardown signal. |

## 6. Shipped-behaviour amendments in specs 015, 021, 012 (User Story 3)

**Spec 015** [F6, F7] — Amendments entry: spec 021 moved vault references off Resource outputs; outputs are a name-only allowlist with an optional template.

| Statement | Form | Must say afterwards |
|---|---|---|
| US1 acceptance scenario 4 | amendment | Given a Resource whose `env` entry is set by `valueFrom: vault:<path>` and whose name is listed under `outputs`, a downstream Application reading that output receives the value. |
| Edge case on `value:` plus `valueFrom: vault:` | amendment | The conflict is on an env key, Application or Resource. |
| Edge case "valid in both Application env vars and Resource outputs" | amendment | Valid in Application `env` and Resource `env`. Not valid on a Resource output; a vault-sourced value is exported by listing its env name under `outputs`. |
| FR-002 | amendment | For Resource manifests, `valueFrom: vault:` is a resolution type on `spec.env`, alongside `value`, `generated`, `template`. It is rejected on `spec.outputs`. |
| FR-009 | amendment | Mutual exclusion applies to `spec.env[]` of both kinds. Outputs have no resolution types to conflict. |
| Key entity `VaultSecretRef` | amendment | Valid in Application `spec.env[]` and Resource `spec.env[]`. |
| Assumption "supported in both …" | amendment | Supported in `spec.env[]` of both kinds; a Resource exports the value by name. |
| US3 acceptance scenario 1 | descope | **Shipped:** the preview resolves the reference to `[VAULT:<path>]` internally and never contacts the vault (scenario 2). **Not shipped:** the preview does not print env values, so the placeholder is not shown. |

**Spec 021** [F7]

| Statement | Form | Must say afterwards |
|---|---|---|
| FR-013 | descope | **Shipped:** the preview resolves env and exports to placeholders without generating secrets or reading the vault. **Not shipped:** it prints neither the container environment nor the published interface. |
| US1 Independent Test | amendment | Confirm the container's env and the exported keys on a real deploy; the preview validates the manifests but does not display values (FR-013). |
| `tasks.md` T029 | descoped task | Original text kept; adds that `internal/handler/dryrun.go` does not exist and the rendering was never implemented. |

**Spec 012** [F8]

| Statement | Form | Must say afterwards |
|---|---|---|
| FR-004 | partly descoped | **Shipped:** numbers, lists, objects, and most strings (including quoted `"true"` / `"false"`) are rejected at parse time. **Not shipped:** the error names the file and line, not the application or alias index; and the YAML 1.1 boolean words (`yes`, `no`, `on`, `off`, `y`, `n`) are accepted even when quoted *(second clause found during implementation, research F8)*. |
| SC-004 | amendment | Met for `tls` outside an alias entry (the error names the field). For a non-boolean value the error names the file and line only, and a few string values are not rejected — see FR-004. |
| `tasks.md` T006 | amendment (task stays `[X]`) | The test feeds quoted `"true"` and integer `1` and asserts the decoder's type tag (`!!str`, `!!int`) in the error, not the alias path. |
