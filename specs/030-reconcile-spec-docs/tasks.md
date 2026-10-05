# Tasks: Reconcile Specs, Tracking Metadata, and Docs with Shipped Behaviour

**Input**: Design documents from `/specs/030-reconcile-spec-docs/`
**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/spec-amendments.md](contracts/spec-amendments.md), [contracts/docs-pages.md](contracts/docs-pages.md), [contracts/wiring-guide.md](contracts/wiring-guide.md), [quickstart.md](quickstart.md)

**Tests**: None requested and none apply — the feature changes no product code. Each story ends with a verification task that runs the relevant gates from [quickstart.md](quickstart.md).

**Organization**: One phase per user story. Most tasks edit one file each and are parallel within their story; the three files shared between stories are called out under Dependencies.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: User story from spec.md (US1–US5)

## Ground rules for every task

- **Do not modify** anything under `cmd/`, `internal/`, `tests/`, `docs/content/cli/`, or `main.go`, `Makefile`, `.github/`, `.goreleaser.yml`.
- **Never change a requirement or criterion identifier.** `**FR-009**` stays `**FR-009**`; a descope marker goes after the bold identifier, not inside it.
- **Never edit** a clarification Q&A record, or any earlier feature's `plan.md`, `research.md`, `data-model.md`, or `contracts/`. The only `tasks.md` entries touched are 021 T029 and 012 T006.
- **Note formats** are fixed in [contracts/spec-amendments.md](contracts/spec-amendments.md) §2. Amendments are truth-first: rewrite the statement so it is correct, then put the original wording, verbatim, in the blockquote beneath it. Descopes keep the original text and add the marker and note. Date in every note: `2026-10-05`.
- **"Canonical link"** in a spec means `[generated gateway file lifecycle](../009-preserve-app-configs/spec.md#generated-gateway-file-lifecycle-canonical)`. **"Limitation link"** in a docs page means `[Generated gateway files are written once](/guides/traefik/#generated-gateway-files-are-written-once)`.
- **Amendments section** goes directly after a spec's header block (the `**Input**` / `**Status**` lines), before its first `##` heading.
- **Verbatim blocks** in the contracts are copied exactly. Where a contract gives a required meaning instead, write one or two plain sentences that say exactly that and no more.
- **Output lines** quoted in docs must come from [research.md](research.md) F3 or [contracts/wiring-guide.md](contracts/wiring-guide.md). Do not paraphrase or invent sample output.
- **Docs conventions**: root-relative Markdown links (`/guides/tls/`), no shortcodes, YAML front matter with `title`, `description`, `weight`.
- **Do not run** the integration suite. No Go test is needed for this feature except where a task says so.
- **If a statement being written does not match the product**, stop, follow the product, and record the divergence (FR-031; [quickstart.md](quickstart.md), last section).

---

## Phase 1: Setup

**Purpose**: Confirm the starting point matches what research assumed, so a later failure is attributable to this feature.

- [X] T001 On branch `030-reconcile-spec-docs`, run `git rev-parse --short main` and confirm it prints `64c93bc`. If it does not, run `git diff --stat 64c93bc main -- internal/plugins/gateway/traefik internal/engine internal/ui internal/resolver internal/manifest docs/content specs AGENTS.md` and re-verify every finding in `specs/030-reconcile-spec-docs/research.md` whose evidence lies in a changed file before continuing
- [X] T002 Run `bash scripts/lint-docs-frontmatter.sh docs/content` and `make docs-build` from the repo root and confirm both pass before any edit (run `make docs-tools` first if Hugo is missing)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The canonical statement. Every preserve-policy amendment, the project reference, and the legacy routing file link to its anchor, and the operator-facing section must agree with it.

**⚠️ CRITICAL**: No user story work begins until this phase is complete.

- [X] T003 In `specs/009-preserve-app-configs/spec.md`, insert between the header block and `## Clarifications`: (a) an `## Amendments` section with one entry — `- **2026-10-05 — [spec 030](../030-reconcile-spec-docs/spec.md)**: Added the canonical statement of the generated gateway file lifecycle (next section), which other specs and the docs now reference.` — then (b) the section `## Generated Gateway File Lifecycle (Canonical)` copied verbatim from `specs/030-reconcile-spec-docs/contracts/spec-amendments.md` §1 (everything inside the fenced block, without the fence). Change nothing else in the file
- [X] T004 Verify T003: `grep -rc '^## Generated Gateway File Lifecycle (Canonical)' specs/*/spec.md | grep -v ':0'` prints exactly one line, for spec 009; and the identifier-stability check from `specs/030-reconcile-spec-docs/quickstart.md` Gate 4 prints `ok` for `009-preserve-app-configs`

**Checkpoint**: The anchor `#generated-gateway-file-lifecycle-canonical` exists. Stories can proceed in any order.

---

## Phase 3: User Story 1 — One answer about a generated gateway file after its first deploy (Priority: P1) 🎯 MVP

**Goal**: Specs 004, 006, 008, 011, and 012 no longer promise that a manifest or config change alone alters an existing generated file; each affected statement links the canonical section.

**Independent Test**: Read specs 006, 008, 009, 011, 012 and answer "the operator changes this routing field and redeploys onto a host that already has the file — what changes on disk?" All five say "nothing, until the file is deleted or edited", four of them by linking spec 009.

Required meanings for every statement below are in [contracts/spec-amendments.md](contracts/spec-amendments.md) §4.

- [X] T005 [P] [US1] In `specs/004-preserve-traefik-yml/spec.md`: add an `## Amendments` section (entry: spec 009 extended this spec's write-once rule to per-route files, superseding FR-008; amended: FR-008). Amend **FR-008** (begins "The preserve policy applies only to the `traefik.yml` static config file") to say: the policy in this spec covers `traefik.yml`; per-route files in `dynamic/` were made write-once by spec 009 and are no longer rewritten or removed by Shrine — canonical link. Add the amendment note with the original text
- [X] T006 [P] [US1] In `specs/006-routing-aliases/spec.md`: add an `## Amendments` section (entry: spec 009 made per-application routing files write-once; amended: one edge case, FR-009, SC-004). Amend three statements, each with its note: (a) the edge case beginning "**Operator removes or changes an alias and re-deploys**" → nothing changes while the per-application file exists; the alias's router is removed or updated when the file is regenerated (delete and redeploy) or edited — canonical link; (b) **FR-009** → a routing file generated after an alias is removed from the manifest MUST NOT contain that alias's router; an existing file is not rewritten — canonical link; (c) **SC-004** → removing the alias, deleting the per-application file, and redeploying removes the route within that deploy; without deleting or editing the file the alias keeps resolving
- [X] T007 [P] [US1] In `specs/008-alias-strip-prefix/spec.md`: add an `## Amendments` section (entry as T006; amended: US1 Independent Test, one edge case, FR-007, SC-001). Amend four statements, each with its note: (a) in User Story 1's **Independent Test**, the step "Then change the alias to `stripPrefix: true` (or remove the field) and re-deploy" → also deletes the application's routing file before redeploying; (b) the edge case beginning "**Operator changes `stripPrefix` from `true` to `false` (or vice versa) and re-deploys**" → the router and middleware match the new value in a file generated afterwards; an existing file is unchanged — canonical link; (c) **FR-007** → a routing file generated after `stripPrefix` changes MUST reflect the new value and MUST NOT contain a strip middleware the new value does not call for; an existing file is not rewritten — canonical link; (d) **SC-001** → resolved by adding `stripPrefix: false`, deleting the application's routing file, and redeploying; no code or container change is required
- [X] T008 [P] [US1] In `specs/011-traefik-tlsport-config/spec.md`: add an `## Amendments` section whose entry includes the terminology note — in this spec "Shrine-generated" means "generated in this deploy because the file was absent", and "operator-preserved" / "operator-edited" mean "already exists"; Shrine cannot tell the two apart — canonical link (amended: one edge case, FR-003, SC-003). Amend three statements, each with its note: (a) the edge case beginning "**`tlsPort` removed between deploys**" → the container is recreated without the `443/tcp` mapping; an existing `traefik.yml` is left untouched and keeps its `websecure` entrypoint until the operator deletes or edits the file — canonical link; (b) **FR-003** → when `tlsPort` is set AND `traefik.yml` does not yet exist, the file Shrine generates MUST declare `websecure` at `:443`; (c) **SC-003** → on a host with no `traefik.yml`, one config line and one deploy suffice; on an existing deployment the operator must also delete `traefik.yml` (Shrine regenerates it in that deploy) or add the entrypoint by hand. Do not edit the clarification record, the entry-points Key Entity, or the spec-004 Assumption — the terminology note covers them
- [X] T009 [P] [US1] In `specs/012-tls-alias-routers/spec.md`: add an `## Amendments` section whose entry includes the terminology note — "operator-preserved" and "operator-owned" mean "already exists" — canonical link (amended: US2 acceptance scenario 2, SC-005). Amend two statements, each with its note: (a) User Story 2 acceptance scenario 2 (begins "**Given** the same manifest, **When** the operator removes `tls: true` from the second alias and re-deploys") → the operator removes `tls: true`, deletes the application's routing file, and redeploys; the regenerated router is plain; (b) **SC-005** → removing `tls: true`, deleting the per-application file, and redeploying yields a plain router within that deploy; no hand-editing is needed; without deleting the file it is preserved — canonical link
- [X] T010 [US1] Verify US1: (a) `grep -c 'generated-gateway-file-lifecycle-canonical' specs/004-*/spec.md specs/006-*/spec.md specs/008-*/spec.md specs/011-*/spec.md specs/012-*/spec.md` shows every count ≥ 1; (b) the Gate 4 identifier-stability loop in `specs/030-reconcile-spec-docs/quickstart.md` prints `ok` for 004, 006, 008, 011, 012; (c) run `grep -n -E -i 're-deploy|redeploy|rewritten|regenerat|within one deploy|Shrine-generated' specs/004-*/spec.md specs/006-*/spec.md specs/008-*/spec.md specs/011-*/spec.md specs/012-*/spec.md` and read every hit — each must be true as written, sit inside an "Originally:" note, or be covered by that spec's terminology note (FR-006). Fix any that is not

**Checkpoint**: US1 complete — SC-001 holds. Suggested commit boundary.

---

## Phase 4: User Story 2 — An operator learns why a routing change did not take effect (Priority: P1)

**Goal**: The docs site states the limitation, links to it where it bites, and no guide promises propagation that does not happen.

**Independent Test**: From the routing guide, the TLS guide, or the troubleshooting page with the symptom "I changed an alias and redeployed but the route did not change", reach the explanation and the fix in one link.

- [X] T011 [US2] In `docs/content/guides/traefik.md`, insert the `## Known limitations` section copied verbatim from `specs/030-reconcile-spec-docs/contracts/docs-pages.md` §1 (everything inside the outer four-backtick fence) between the "Dashboard access" section and `## Common pitfalls`
- [X] T012 [US2] In `docs/content/guides/traefik.md` (after T011), make the four edits in `contracts/docs-pages.md` §2, using the in-page anchor `#generated-gateway-files-are-written-once`: (a) "Configure entrypoints": rewrite the paragraph beginning "When `tlsPort` is set" → Shrine publishes `<tlsPort>:443/tcp` on the container and, when it generates `traefik.yml`, includes a `websecure` entrypoint at `:443`; an existing `traefik.yml` is not modified — link; keep the sentence about certificates; (b) "Per-app routing": after the sentence "Shrine writes a dynamic config file at …", add one sentence — the file is written once; later changes to the `routing` block do not rewrite it — link; (c) "Dashboard access": add one sentence — changing the credentials later does not rewrite the file — link; removing the `dashboard` block deletes it on the next deploy; (d) "Common pitfalls": add a bullet "**A manifest or config change has no effect on routing**" — link
- [X] T013 [P] [US2] In `docs/content/guides/routing-and-aliases.md`, add a section `## Changing routing after the first deploy` immediately before `## Logging`: Shrine writes an application's route file on its first deploy and never rewrites it; adding, removing, or editing an alias — or changing `routing.domain`, `pathPrefix`, `stripPrefix`, or `tls` — takes effect only after the file is deleted and the app redeployed, or the file is edited by hand; show a `bash` block with `rm {routing-dir}/dynamic/<team>-<app>.yml` then `shrine deploy`, and a `text` block with the line `  📄 Preserving operator-owned route file: <path>` as the sign that the file was left alone; end with the limitation link
- [X] T014 [P] [US2] In `docs/content/guides/tls.md`, make the three edits in `contracts/docs-pages.md` §2–§3: (a) "Configure the gateway": replace the sentence beginning "Run `shrine deploy` once." → Run `shrine deploy`. Shrine recreates the Traefik container with the new port binding. If `traefik.yml` does not exist yet, Shrine generates it with the `websecure` entrypoint (keep the YAML sample and the sentence after it); (b) same section: replace the paragraph beginning "If your `traefik.yml` was preserved from a prior operator edit" → on a host where the gateway has been deployed before, `traefik.yml` already exists and Shrine does not modify it; the deploy prints a warning beginning `tlsPort set but traefik.yml is missing websecure entrypoint`; either delete `traefik.yml` before deploying — Shrine regenerates it with `websecure` in the same deploy — or add the entrypoint to the file by hand and restart the gateway — limitation link; (c) "Mark an alias as TLS": after the paragraph beginning "Shrine generates that alias router", add — if the application was already deployed, its route file exists and is not rewritten; delete it and redeploy for `tls: true` to take effect — limitation link
- [X] T015 [P] [US2] In `docs/content/troubleshooting/_index.md`, add a section `## A routing change in the manifest did not take effect` immediately before `## See also`: symptom — a changed domain, alias, `stripPrefix`, `tls`, or gateway port is not reflected after a redeploy; evidence — the deploy output shows `Preserving operator-owned route file` or `Preserving operator-owned traefik.yml`; fix — delete the file named in that line and redeploy, or edit it by hand — limitation link. Match the length and voice of the neighbouring entries
- [X] T016 [US2] Verify US2: run `bash scripts/lint-docs-frontmatter.sh docs/content` and `make docs-build`; then `grep -c 'id=generated-gateway-files-are-written-once' docs/public/guides/traefik/index.html` prints `1`; `grep -rl 'generated-gateway-files-are-written-once' docs/content | sort` lists exactly `docs/content/guides/routing-and-aliases.md`, `docs/content/guides/tls.md`, `docs/content/guides/traefik.md`, `docs/content/troubleshooting/_index.md`; the changed-pages link check from `quickstart.md` Gate 3 prints nothing. Then read the canonical section in `specs/009-preserve-app-configs/spec.md` beside the new section in `docs/content/guides/traefik.md` and confirm they list the same files, the same non-propagating inputs, and the same way to apply a change ([data-model.md](data-model.md) §3 rule 2)

**Checkpoint**: US2 complete — SC-002 and SC-003 hold. Suggested commit boundary.

---

## Phase 5: User Story 3 — Specs 015, 009, 021, and 012 say what shipped (Priority: P2)

**Goal**: Each spec matches the product, and requirements that did not ship are visibly descoped and tracked.

**Independent Test**: For each of the four specs, take the statement the issue names, compare it with the product (evidence in [research.md](research.md) F5–F8), and confirm they agree or that the statement is marked descoped with a pointer to the known-gaps list.

Required meanings are in [contracts/spec-amendments.md](contracts/spec-amendments.md) §5 (spec 009) and §6 (specs 015, 021, 012).

- [X] T017 [US3] In `specs/009-preserve-app-configs/spec.md` (after T003), amend eight statements, each with its note, per `contracts/spec-amendments.md` §5: the edge case beginning "**App removed from manifest**"; **FR-009**; **FR-011**; the Key Entity "**Gateway dynamic routing directory**"; the second sentence of **SC-004**; **SC-007** (remove the parenthetical about orphan warnings); the Assumption beginning "The remove path on app deletion (FR-009)" (the warning is emitted once, at teardown — not "on every deploy until the file is removed"); the Assumption beginning "Shrine's deploy responsibility is Docker/container management". Every one must place the orphan warning on `shrine teardown <team>` and none may place it on deploy. Extend the Amendments entry from T003 with "Amended: …" listing these eight and the reason (the warning fires at team teardown; this spec's own `plan.md` Decision 3 resolved it and the spec text was never updated). Do not edit the clarification record
- [X] T018 [P] [US3] In `specs/015-infisical-secrets-vault/spec.md`: add an `## Amendments` section (entry: spec 021 moved vault references off Resource outputs — outputs are a name-only export allowlist with an optional template; amended: the seven statements below; descoped: US3 acceptance scenario 1). Amend seven statements, each with its note, per `contracts/spec-amendments.md` §6: User Story 1 acceptance scenario 4 (begins "**Given** a Resource manifest with a `valueFrom: vault:<path>` output"); the edge case beginning "If the same env key (Application) or output name (Resource)"; the edge case beginning "`valueFrom: vault:` is valid in both Application env vars and Resource outputs"; **FR-002**; **FR-009**; the Key Entity **VaultSecretRef**; the Assumption beginning "`valueFrom: vault:` is supported in both". Then mark User Story 3 acceptance scenario 1 (begins "**Given** a manifest with `valueFrom: vault:project/env/key`, **When** `shrine dry-run` runs") descoped with a descope note — Shipped: the preview resolves the reference to `[VAULT:<path>]` internally and never contacts the vault (scenario 2); Not shipped: the preview does not print env values, so the placeholder is not shown; tracked in `specs/progress.md` Known Gaps
- [X] T019 [P] [US3] In `specs/021-resource-env-output-split/spec.md`: add an `## Amendments` section (entry: descoped FR-013; amended US1 Independent Test). Mark **FR-013** `*(descoped 2026-10-05)*` with its original text unchanged and a descope note — Shipped: the preview resolves env and exports to placeholders without generating secrets or reading the vault; Not shipped: it prints neither the container environment nor the published interface; tracked in `specs/progress.md` Known Gaps. In User Story 1's **Independent Test**, amend "Deploy (real and `--dry-run`) and confirm …" → confirm the container's env and the exported keys on a real deploy; the preview validates the manifests but does not display values (FR-013) — with its note
- [X] T020 [P] [US3] In `specs/021-resource-env-output-split/tasks.md`, change task **T029** from `- [x] T029 [P] …` to the descoped-task format in `contracts/spec-amendments.md` §2: `- [~] T029 [P] DESCOPED (spec 030, 2026-10-05) — <original text unchanged>.` followed by one sentence: `internal/handler/dryrun.go` does not exist and the rendering was never implemented; the preview prints neither `Env` nor `Exports`. Change no other line
- [X] T021 [US3] In `specs/012-tls-alias-routers/spec.md` (after T009; if US1 was not done, create the `## Amendments` section here): mark **FR-004** `*(partly descoped 2026-10-05)*` with its original text unchanged and a descope note — Shipped: a non-boolean `tls` is rejected at parse time; Not shipped: the error names the file and line (`cannot unmarshal !!str … into bool`), not the application or alias index; tracked in `specs/progress.md` Known Gaps. Amend **SC-004** with its note → met for `tls` outside an alias entry (the error names the field); for a non-boolean value the error names the file and line only — see FR-004. Extend the Amendments entry with these two
- [X] T022 [P] [US3] In `specs/012-tls-alias-routers/tasks.md`, correct the description of task **T006** (it stays `[X]`): replace the clause saying the test asserts the error message "contains the alias path (e.g., `routing.aliases[1].tls`) and indicates a boolean type was expected" with what `TestParseApplication_RoutingAlias_TLS_RejectsNonBoolean` in `internal/manifest/parser_test.go` asserts — the YAML decoder's type tag (`!!str`, `!!int`) in the error — and append "(corrected 2026-10-05 by spec 030; the alias-path error shape is descoped, see FR-004)". Change no other line
- [X] T023 [US3] In `specs/progress.md`, append to the `## Known Gaps` list the two entries copied verbatim from `specs/030-reconcile-spec-docs/contracts/spec-amendments.md` §3. Change nothing else in the file in this task
- [X] T024 [P] [US3] In `docs/content/guides/secrets-vault.md`, rewrite the body of `## Dry-run behaviour` per `contracts/docs-pages.md` §4: `shrine deploy --dry-run` does not contact the vault — every `vault:` reference is resolved to a placeholder internally, so the preview succeeds without network access or credentials; the preview validates manifest structure and dependency wiring and prints the deploy order and the container operations; it does not print environment values, so the placeholders are not displayed. Remove the "Example dry-run output" block (`env DB_PASSWORD=[VAULT:…]`). Keep the closing sentence about CI lint jobs. Then run `grep -n 'VAULT:\|placeholder' docs/content/guides/secrets-vault.md` and correct any other sentence saying the placeholder is shown or rendered in the output
- [X] T025 [US3] Verify US3 with `quickstart.md` Gate 4: `grep -n -i 'resource output\|spec\.outputs' specs/015-infisical-secrets-vault/spec.md` — every hit is inside an "Originally:" note or states the form is not valid; `grep -n -i 'orphan' specs/009-preserve-app-configs/spec.md | grep -i 'deploy'` — every hit is inside an "Originally:" note or says deploy does not emit the warning; `grep -n -i 'descoped' specs/021-*/spec.md specs/012-*/spec.md specs/015-*/spec.md specs/021-*/tasks.md` hits each file; `grep -n 'does not print resolved environment\|rejected with a decoder error' specs/progress.md` prints two lines; the identifier-stability loop prints `ok` for 009, 012, 015, 021. Then run `make docs-build` and confirm it passes

**Checkpoint**: US3 complete — SC-004 and SC-005 hold. Suggested commit boundary.

---

## Phase 6: User Story 4 — An operator wires a database to an application by following a guide (Priority: P2)

**Goal**: A walkthrough on the docs site that builds a database and its consumers and covers all seven wiring patterns with manifests that work as written.

**Independent Test**: Someone who has never used Resource outputs follows the guide from an empty directory and gets a preview that exits 0.

- [X] T026 [US4] Create `docs/content/guides/wiring-env-and-outputs.md` with the front matter from `specs/030-reconcile-spec-docs/contracts/docs-pages.md` §5 and the thirteen sections of the outline in `contracts/wiring-guide.md` §1, in that order. Use the manifests in §2 and §5 exactly as given; quote preview output only from §4 and §5 and error messages only from §5 and §6; show the commands in §3. Sections 4–9 each show the manifest fragment that introduces the pattern and one sentence on when to use it; the five complete same-team manifests appear in full exactly once each. Say once that `traefik/whoami` stands in for the reader's own images. Obey §7 ("What the guide must not say") — in particular, section 5 must state that a consumer of `url` receives a string containing the password, and section 11 must state that the preview does not print environment values. Link to `/reference/manifest-schema/` for field detail, `/guides/secrets-vault/` for vault references, and `/guides/team-scoped-deploy/` in "See also"; do not reproduce the reference's field tables. Follow the section pattern and voice of `docs/content/guides/secrets-vault.md`
- [X] T027 [P] [US4] In `docs/content/guides/_index.md`, add to the Contents list, after the "TLS / HTTPS" entry: `- [Wiring env and outputs](wiring-env-and-outputs/) — Connect resources and applications: private config, exported outputs, and deploy order.`
- [X] T028 [P] [US4] In `docs/content/reference/manifest-schema.md`, in the Resource section directly after the paragraph beginning "**Strict allowlist.**", add: `See the [Wiring env and outputs guide](/guides/wiring-env-and-outputs/) for a walkthrough.`
- [X] T029 [US4] Verify the guide's manifests per `quickstart.md` Gate 5: build `go build -o "$W/shrine" .` into an empty scratch directory `$W` outside the repo; copy the five complete same-team manifests out of `docs/content/guides/wiring-env-and-outputs.md` exactly as the page shows them into `$W/guide/`; run `apply teams` then `deploy --dry-run` with `--config-dir "$W/cfg" --state-dir "$W/state" --path "$W/guide"`; confirm exit 0 and that the output from `Deploy order:` onward equals both `contracts/wiring-guide.md` §4 and the block printed in the guide. Then add the `ops` team and `reporter` and repeat at each cross-team stage; confirm the three messages of §5 in order, then exit 0. If anything differs, the guide is wrong — fix the guide
- [X] T030 [US4] Verify US4: run `bash scripts/lint-docs-frontmatter.sh docs/content`, `make docs-build`, `bash scripts/check-md-companions.sh docs/public`, `bash scripts/check-md-shape.sh docs/public`; `test -f docs/public/guides/wiring-env-and-outputs/index.html`; the changed-pages link check from `quickstart.md` Gate 3 prints nothing. Confirm each of the seven patterns FR-026 (a)–(g) has a manifest fragment in the page

**Checkpoint**: US4 complete — SC-006 holds; SC-007 needs a human reader. Suggested commit boundary.

---

## Phase 7: User Story 5 — The session-start ritual lands on accurate status (Priority: P3)

**Goal**: The progress file, the specs README, the legacy feature statuses, and the project reference match the product.

**Independent Test**: Follow the three session-start steps in `specs/README.md` exactly, list what the materials say has shipped and what is next, and compare with the product ([research.md](research.md) F9).

- [X] T031 [US5] In `specs/progress.md` (after T023), per research D12: (a) change the Phase 9 line to `- [x] **Phase 9: Routing**` and replace its description — shipped as a local Traefik gateway plugin, not the SSH push `specs/features/routing.md` describes; delivered by `specs/001-traefik-gateway-plugin/` and specs 002, 004, 006, 008–012, 016, 018, 024; generated files are written once (`specs/009-preserve-app-configs/spec.md`, canonical section); (b) change the Phase 11 line to `[x]` — `shrine teardown <team>` plans from recorded deployments, removes applications then resources then the network, and leaves route files with a warning (specs 009, 028); (c) leave Phase 10 unchanged; (d) leave Phase 12 `[ ]` and append — blocked on Phase 10; no `--verbose` flag exists; the preview covers Docker and routing operations today; (e) leave Phase 13 `[ ]` and add two sub-items — `[x]` GoReleaser config, GitHub releases for linux/darwin on amd64/arm64, `install.sh`, `shrine update`; `[ ]` `.deb` packaging with post-install scripts; (f) rewrite the three bullets under `## Current State` — phases complete: 1–9, 11, 14, 15, with 13 partly shipped; next phase: Phase 10 — DNS; Go version: 1.25 (`go.mod`: `go 1.25.0`) — keeping the module-path bullet; (g) add, directly above the spec 029 entry and in its style, a `[x]` entry for this feature naming `specs/030-reconcile-spec-docs/` and issue #39 and summarising in two or three sentences what was reconciled
- [X] T032 [P] [US5] Rewrite `specs/README.md` per research D13, keeping the "Design Principle" and "Provider-Specific Adapters" sections: (a) "Starting a Session" — read `../AGENTS.md`; read `progress.md`; read the spec for the feature: `NNN-short-name/spec.md` (with `plan.md` and `tasks.md` beside it), where the feature in progress is named by `../.specify/feature.json` and the pointer at the top of `../CLAUDE.md`; for the four features specified before numbered directories existed, read `features/<name>.md`; (b) "Directory Layout" — a tree showing `README.md`, `progress.md`, `NNN-short-name/` with `spec.md`, `plan.md`, `research.md`, `data-model.md`, `quickstart.md`, `contracts/`, `checklists/`, `tasks.md`, and `features/` marked legacy; (c) a short "Numbered specs" section — numbers are sequential, `ls` of this directory is the index, numbers **005 and 007 are intentionally unused** (features withdrawn before they were specified), and a spec that a later feature contradicts is amended in place with a dated note rather than silently rewritten; (d) a "Legacy feature files" table — `routing.md` superseded, `logging-observer.md` done, `integration-tests.md` with the status T035 sets, `daemon.md` planned — each with the numbered specs that superseded or extended it where any; (e) in "What a Good Spec Contains", add `superseded` and `planned` to the Status vocabulary and note that numbered specs follow `../.specify/templates/spec-template.md`. The file must not describe routing as upcoming or mention SSH push as a plan
- [X] T033 [P] [US5] In `specs/features/routing.md`, change the Status from `Pending` to `Superseded` and add directly beneath it: routing shipped as a local gateway plugin specified in `specs/001-traefik-gateway-plugin/` and extended by specs 002, 004, 006, 008–012, 016, 018, 024; this file is kept as the original design record; the shipped design differs in three ways — (1) Shrine writes files to a local routing directory mounted into a Traefik container it manages; there is no SSH push; (2) per-application files are named `<team>-<name>.yml` under `dynamic/`; (3) files are written once and never removed by Shrine — teardown warns instead (link `../009-preserve-app-configs/spec.md#generated-gateway-file-lifecycle-canonical`). Above the acceptance-criteria list add one line: the criteria below describe the original design and were not implemented as written. Leave the checkboxes and the rest of the body unchanged
- [X] T034 [P] [US5] In `specs/features/logging-observer.md`, verify each of the five acceptance criteria and check only those that hold: (1) `grep -rn 'fmt\.Print' internal/engine --include='*.go' | grep -v '_test\|/dryrun/'` prints nothing; (2) `internal/ui/terminal_logger.go` exists and renders events; (3) `grep -rln -i 'spinner' internal/` lists files under `internal/ui/` only; (4) `Observer` is an interface in `internal/engine/events.go` and the engine takes it by injection; (5) `go test ./...` passes (unit tests only; do not add the `integration` tag). If all five hold, change the Status from `Pending` to `Done`; otherwise set `In progress` and list what is missing beneath it
- [X] T035 [P] [US5] In `specs/features/integration-tests.md`, for each of Phases 6–12 (headings ending "(pending)"), compare the phase's listed test cases with the sub-test names in the file the phase names under `tests/integration/` (`apply_test.go`, `delete_test.go`, `describe_test.go`, `get_test.go`, `status_test.go`, `teardown_test.go`). Where every listed case has a matching sub-test, replace "(pending)" with "✅" in the heading; where some are missing, replace it with "(partial)" and add one line beneath the heading naming the missing cases. Then set the `## Status` line to match — "Done (Phases 1–12 complete)" only if all are ✅, otherwise "In progress" with the accurate phase list. Do not edit the phase bodies or any file under `tests/`
- [X] T036 [P] [US5] In `AGENTS.md`, per research D15: (a) in the project tree, change the `plan.go` line (currently `# Plan(), PlanSingle() entry points: load → resolve → enrich → order/single-step`) to name `Plan()` and `PlanTeardown()` and drop `PlanSingle()` and "single-step"; (b) in "Gateway: Traefik", replace the bullet "preserves operator-added files in routing-dir (only files matching `{team}-{name}.yml` produced by shrine are managed)" with: writes each generated file once and never rewrites or removes it afterwards — `traefik.yml` and `dynamic/{team}-{name}.yml` are left untouched once they exist, and teardown warns about leftover route files; operator-added files in routing-dir are never touched; see `specs/009-preserve-app-configs/spec.md#generated-gateway-file-lifecycle-canonical`
- [X] T037 [US5] Remove the two empty, untracked directories from the working tree: `rmdir specs/005-traefik-entrypoints/checklists specs/005-traefik-entrypoints specs/007-fix-traefik-dynamic-dashboard/checklists specs/007-fix-traefik-dynamic-dashboard`. Use `rmdir` so the command fails rather than deletes if either turns out not to be empty; this produces no diff
- [X] T038 [US5] Verify US5 with `quickstart.md` Gate 4: `grep -n 'PlanSingle' AGENTS.md` prints nothing; `grep -n 'Phase 9\|Phase 11\|Next phase\|Go version' specs/progress.md` shows Phases 9 and 11 checked, next phase 10, Go 1.25; `ls -d specs/005-traefik-entrypoints specs/007-fix-traefik-dynamic-dashboard 2>/dev/null` prints nothing; `grep -n -i 'ssh push\|Phase 9' specs/README.md` prints nothing that describes routing as upcoming; `grep -n -A1 '^## Status' specs/features/routing.md specs/features/logging-observer.md specs/features/integration-tests.md` shows the three new statuses

**Checkpoint**: US5 complete — SC-008 holds. Suggested commit boundary.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Whole-change verification and hand-off.

- [X] T039 Run `quickstart.md` Gate 1 on the whole change: `{ git diff --name-only main; git ls-files --others --exclude-standard; } | sort -u | grep -Ev '^(specs/|docs/content/|AGENTS\.md$|CLAUDE\.md$|\.specify/feature\.json$)'` prints nothing, and `git diff --name-only main -- docs/content/cli` prints nothing (FR-029, SC-009)
- [X] T040 Run `quickstart.md` Gates 2, 3, and 4 once more in full on the finished change and confirm every expected result, including the identifier-stability loop printing eight `ok` lines (SC-010)
- [X] T041 Do the `quickstart.md` Gate 6 reading checks: read `specs/006-routing-aliases/spec.md` FR-009, the canonical section in `specs/009-preserve-app-configs/spec.md`, the new section in `docs/content/guides/routing-and-aliases.md`, and the known-limitations section in `docs/content/guides/traefik.md`, and confirm all four give the same answer to "I removed an alias and redeployed — is the route gone?" (SC-002); confirm every descope note in specs 012, 015, 021 points to a Known Gaps entry that exists in `specs/progress.md` ([data-model.md](data-model.md) §3 rule 3)
- [X] T042 Run `graphify update .` from the repo root to refresh the knowledge graph (`graphify-out/` is git-ignored; no diff results)
- [X] T043 When the pull request is opened, include in its description: the seven scope adjustments E1–E7 from `specs/030-reconcile-spec-docs/research.md` Part 3; the out-of-scope observations from research Part 4, for the maintainer to decide on follow-ups; that SC-007 (a first-time reader succeeds with the guide alone) is not machine-checkable and awaits a human read; and any divergence recorded under FR-031. Do not push or open the pull request as part of this task

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: none.
- **Foundational (Phase 2)**: after Setup. Blocks every story — it creates the anchor US1, US3, and US5 link to and the text US2 must agree with.
- **User Stories (Phases 3–7)**: each depends only on Phase 2. They can be done in any order or in parallel, subject to the three shared files below.
- **Polish (Phase 8)**: after every story that is going into the change.

### User Story Dependencies

- **US1 (P1)**: independent after Phase 2.
- **US2 (P1)**: independent after Phase 2. Its verification (T016) reads the canonical section from T003.
- **US3 (P2)**: independent after Phase 2. T021 extends the Amendments section T009 creates in spec 012, and creates it if US1 has not run.
- **US4 (P2)**: independent after Phase 2; touches no file another story touches.
- **US5 (P3)**: independent after Phase 2. T032's legacy table uses the status T035 sets, so run T035 before T032 or revisit that one row.

### Files shared between stories (sequence these)

| File | Order |
|---|---|
| `specs/009-preserve-app-configs/spec.md` | T003 → T017 |
| `specs/012-tls-alias-routers/spec.md` | T009 → T021 |
| `specs/progress.md` | T023 → T031 |

Within US2, `docs/content/guides/traefik.md` is edited by T011 then T012.

### Parallel Opportunities

- **US1**: T005, T006, T007, T008, T009 — five different spec files.
- **US2**: T013, T014, T015 alongside T011→T012.
- **US3**: T018, T019, T020, T022, T024 — five different files; T017, T021, T023 are single-file edits that only wait on the earlier task for the same file.
- **US4**: T027 and T028 alongside T026.
- **US5**: T033, T034, T035, T036 together; then T032.
- Across stories: US2 and US4 share no file with any other story and can run alongside anything.

---

## Parallel Example: User Story 1

```bash
# Five independent spec files, one task each:
Task: "T005 Amend FR-008 in specs/004-preserve-traefik-yml/spec.md"
Task: "T006 Amend edge case, FR-009, SC-004 in specs/006-routing-aliases/spec.md"
Task: "T007 Amend US1 test, edge case, FR-007, SC-001 in specs/008-alias-strip-prefix/spec.md"
Task: "T008 Amend edge case, FR-003, SC-003 in specs/011-traefik-tlsport-config/spec.md"
Task: "T009 Amend US2-AS2, SC-005 in specs/012-tls-alias-routers/spec.md"
# Then, alone:
Task: "T010 Verify US1"
```

---

## Implementation Strategy

### MVP First (User Story 1)

1. Phase 1 (T001–T002), Phase 2 (T003–T004).
2. Phase 3 (T005–T010).
3. **Stop and validate**: the Independent Test for US1. At this point the contradiction the issue leads with is gone from the spec set, and the change is mergeable on its own.

### Incremental Delivery

Each story after the MVP is a self-contained commit: US2 (operators learn the limitation), US3 (specs match the product), US4 (the guide), US5 (tracking metadata). The plan is one pull request with five commits in priority order; if review asks for a split, cut along the story checkpoints. Phase 8 runs once on whatever set is being merged.

### Suggested order for a single implementer

Phases 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8. This respects every shared-file sequence without having to think about it.

---

## Notes

- 43 tasks: Setup 2, Foundational 2, US1 6, US2 6, US3 9, US4 5, US5 8, Polish 5.
- Every verification command comes from [quickstart.md](quickstart.md); if a command and a task disagree, quickstart is the source and the task should be corrected.
- Line positions in the older specs shift as Amendments sections are inserted, so tasks identify statements by identifier or opening words, never by line number.
- A grep hit in a verification task is a prompt to read, not an automatic failure — each task says which hits are acceptable.
