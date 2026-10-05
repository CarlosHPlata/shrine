# Research: Reconcile Specs, Tracking Metadata, and Docs with Shipped Behaviour

**Date**: 2026-10-05 | **Checked against**: `main` @ `64c93bc` | **Spec**: [spec.md](spec.md)

There were no `NEEDS CLARIFICATION` markers to resolve. Research for a documentation feature means pinning two things before any text is written: what the product actually does, and exactly which sentences say otherwise. Part 1 records the first, Part 2 the decisions, Part 3 where research widened the scope, and Part 4 what was seen and deliberately left alone.

Every behavioural claim below was verified by reading the code or by running a freshly built binary in `--dry-run` with isolated `--config-dir` / `--state-dir`. Nothing was verified against a live Docker daemon.

## Part 1 — Current-state findings

### F1. Three generated gateway files, two lifecycles

Source: `internal/plugins/gateway/traefik/config_gen.go`, `routing.go`.

| File | Written | When present | Deleted by Shrine |
|---|---|---|---|
| `{routing-dir}/traefik.yml` (static config) | On deploy, in the finalize step, when absent | Left untouched; `gateway.config.preserved` | Never |
| `{routing-dir}/dynamic/<team>-<name>.yml` (per-app route) | On deploy of a routed app, when absent | Left untouched; `gateway.route.preserved` | Never. `shrine teardown <team>` emits `gateway.route.orphan` and leaves it |
| `{routing-dir}/dynamic/__shrine-dashboard.yml` (dashboard route) | On deploy with a dashboard configured, when absent | Left untouched; `gateway.dashboard.preserved` | **Yes** — on the first deploy after the dashboard block is removed (spec 024); `gateway.dashboard.removed` |

Presence is the only test (`isPathPresent`). Nothing records who wrote a file, so Shrine cannot tell a file it generated on an earlier deploy from one an operator edited. The phrases "Shrine-generated (not operator-preserved)" and "preserved from a prior operator edit", used by spec 011 and the TLS guide, describe a distinction the product does not make.

### F2. What feeds each file, and therefore what does not propagate

| File | Inputs | Source |
|---|---|---|
| Static config | gateway `port` (the `web` entrypoint address), `tlsPort` (presence of `websecure`), `dashboard.port` (the `traefik` entrypoint and `api.dashboard`) | `generateStaticConfig` |
| Per-app route | `routing.domain`, `routing.pathPrefix`, `spec.port`, each alias's `host` / `pathPrefix` / `stripPrefix` / `tls`, team and app name | `WriteRoute` |
| Dashboard route | `dashboard.username`, `dashboard.password` | `generateDashboardDynamicConfig` |

Two consequences the docs must state:

- **Container settings do propagate.** Host port bindings are part of the container's config hash; changing `port`, `tlsPort`, or `dashboard.port` recreates the gateway container on the next deploy (`docker_container.go`: hash mismatch → remove and create). The preserved static config is then out of step with the new bindings. For `tlsPort` Shrine warns (`gateway.config.tls_port_no_websecure`); for `port` and `dashboard.port` it does not.
- **Regenerating the static config does not restart the gateway.** If `traefik.yml` is deleted and regenerated but nothing in the container config changed, the container is left running (`isContainerUpToDate` → `ensureRunning`). Traefik reads the static config at startup, so the operator must restart the container. Dynamic files are watched (`providers.file.watch: true`) and need no restart.

### F3. Operator-visible text

Source: `internal/ui/terminal_logger.go`. These are the strings the docs may quote.

| Event | Output line |
|---|---|
| `gateway.route.generated` | `  📝 Generated route file: <path>` |
| `gateway.route.preserved` | `  📄 Preserving operator-owned route file: <path>` |
| `gateway.route.orphan` | `  ⚠️  Orphan route file left on disk; remove with: rm <path>` |
| `gateway.config.generated` | `  📝 Generated default traefik.yml: <path>` |
| `gateway.config.preserved` | `  📄 Preserving operator-owned traefik.yml: <path>` |
| `gateway.config.tls_port_no_websecure` | `  ⚠️  tlsPort set but traefik.yml is missing websecure entrypoint at <path> — <hint>` |
| `gateway.dashboard.preserved` | `  📄 Preserving operator-owned dashboard dynamic file: <path>` |
| `gateway.dashboard.removed` | `  🗑️  Removed stale dashboard dynamic file: <path>` |

"Operator-owned" in these lines means "already existed", not "was edited by an operator".

### F4. Statements that contradict F1–F2

Full sweep of specs 004, 006, 008, 009, 011, 012. "Named" means issue #39 cites it.

| Spec | Location | Says | Named |
|---|---|---|---|
| 004 | FR-008 | Per-route files in `dynamic/` "continue to be written and removed by Shrine as today" | no |
| 006 | Edge case "Operator removes or changes an alias and re-deploys" | Routers for the removed alias are cleaned up | no |
| 006 | FR-009 | Alias routers MUST be removed on redeploy | yes |
| 006 | SC-004 | Route removed "within one deploy cycle … without operator intervention beyond `shrine deploy`" | yes |
| 006 | SC-001 | A second hostname on an *existing* application with one `aliases` entry and "a normal `shrine deploy`" (found during implementation) | no |
| 008 | US1 Independent Test (second half) | Change `stripPrefix`, redeploy, backend now sees the stripped path | no |
| 008 | Edge case "Operator changes `stripPrefix` … and re-deploys" | Router and middleware are regenerated | no |
| 008 | FR-007 | Config MUST be rewritten to reflect the new value | yes |
| 008 | SC-001 | Fixed by adding `stripPrefix: false` and redeploying, "no other … change is required" | no |
| 011 | Edge case "`tlsPort` removed between deploys" | Static config regenerated without `websecure` "when Shrine-generated (not operator-preserved)" | yes |
| 011 | FR-003 | Entry point declared when the file "is being generated by Shrine (i.e., not preserved from a prior operator edit)" | no |
| 011 | SC-003 | HTTPS added to an *existing* deployment with one config line and one deploy | no |
| 011 | Clarification Q&A, Key Entity, Assumption on spec 004 | Use "Shrine-generated" / "operator-edited" as if distinguishable | no |
| 012 | US2 acceptance scenario 2 | Remove `tls: true`, redeploy, router regenerated without TLS | no |
| 012 | SC-005 | Router loses TLS "within one deploy cycle (subject to spec 009 preservation …)" | yes |
| 012 | SC-001 | HTTPS on an *existing* alias with one `tls: true` line and "a normal `shrine deploy`" (found during implementation) | no |
| 012 | Edge case, FR-008 | "operator-preserved" used as a conditional state | no |

Spec 011's first user story and SC-001 are conditioned on a clean host and are correct as written. The first half of 011's edge case (the container is recreated without the `443/tcp` mapping) is also correct (F2).

### F5. Where the orphan warning fires

`RemoveRoute` is called from exactly one place: `engine.teardownKind`, reached only through `shrine teardown <team>` (`internal/handler/teardown.go`). Nothing on the deploy path detects a routing file whose application has left the manifest, and `shrine delete application` only edits state. The warning therefore fires once, at team teardown, not "on every deploy until the file is removed".

Spec 009 statements that say otherwise: edge case "App removed from manifest", FR-009, FR-011, the "Gateway dynamic routing directory" entity, SC-004, SC-007, and two assumptions (the remove-path assumption, including its "observable on every deploy" rationale, and the deploy-responsibility assumption). The clarification Q&A records the question as asked and is left as history. Spec 009's own `plan.md` and `research.md` already say teardown.

### F6. Spec 015 against the shape spec 021 shipped

Vault references are valid on Application `env` and Resource `env`. A Resource output accepts only `name` and an optional `template`; `value`, `valueFrom`, or `generated` on an output is rejected at validation:

```text
- spec.outputs[0] "legacy" must not set value/valueFrom/generated — those fields are deprecated on outputs; declare it under spec.env and list its name under spec.outputs to export it
```

Spec 015 statements describing the old shape: US1 acceptance scenario 4, two edge cases, FR-002, FR-009, the `VaultSecretRef` entity, and one assumption. The published secrets vault guide was already rewritten to the new shape. Spec 015's `plan.md`, `data-model.md`, `tasks.md`, and `contracts/` also describe the old shape; they are records of how the feature was built at the time (see D9).

### F7. What `deploy --dry-run` prints

Observed output for a database, two applications, and a second resource:

```text
[shrine] Planning deployment from: <path>
Deploy order:
  1. Resource:shop-db
  2. Application:api
       depends on:
         - Resource:shop-db (inferred from env DATABASE_URL)
  …
[DOCKER] CreatePlatformNetwork name=shrine.platform
[DOCKER] NetworkCreate: name=shop
[DOCKER] ContainerCreate: name=shop.shop-db image=postgres:16
[DOCKER] ContainerCreate: name=shop.api image=ghcr.io/example/shop-api:1.0.0
…
[ROUTE]  Finalize
```

The dry-run container backend prints name, image, volumes, platform attachment, and published port. It has never printed environment variables or exports (`git log -S'Env'` on the file finds nothing). The dry-run resolver does compute placeholders (`[GENERATED]`, `[VAULT:<path>]`, `[PORT]`) and a dry-run writes no secrets — the state directory held only the team record afterwards — but nothing displays the result.

So three places claim the same unshipped behaviour:

- spec 021 FR-013 (named in the issue) and its task T029, marked complete, which names `internal/handler/dryrun.go` — a file that does not exist;
- spec 015 US3 acceptance scenario 1 ("the env var is shown with a recognizable placeholder … in the plan output");
- the secrets vault guide's "Dry-run behaviour" section, which shows `env DB_PASSWORD=[VAULT:…]` as example output.

Spec 015 US3 scenario 2 (no vault connection attempted, command succeeds) did ship.

### F8. Spec 012 FR-004's error shape

A non-boolean `tls` is rejected, by the YAML decoder:

```text
Error: parsing manifest "tls/app.yml": parsing Application manifest: yaml: unmarshal errors:
  line 14: cannot unmarshal !!str `true` into bool
```

It names the file and line, not the application or alias index. Task T006 is marked complete and says the test asserts the message contains the alias path; the test asserts `!!str` / `!!int`.

*Corrected during implementation (FR-031):* "a non-boolean `tls` is rejected" overstates it. Numbers, lists, objects, and most strings — including quoted `"true"` and `"false"` — are rejected, but the decoder accepts the YAML 1.1 boolean words (`yes`, `no`, `on`, `off`, `y`, `n`, in any case) as booleans even when quoted: `tls: "yes"` previews with exit 0. The test's own comment records this, which is why it feeds `"true"` rather than the `"yes"` its task entry names. The descope note, SC-004's amendment, the T006 correction, and the known-gaps entry all say so. SC-004 ("a single clear error naming the offending field") is met for `tls` outside an alias (FR-005 names the field) and unmet for the non-boolean case.

### F9. Tracking metadata

| Item | Says | Actual |
|---|---|---|
| `progress.md` Phase 9 Routing | pending | Shipped by specs 001, 002, 004, 006, 008–012, 016, 018, 024 as a local gateway plugin, not the SSH push the legacy spec describes |
| `progress.md` Phase 10 DNS | pending | Pending (only the dry-run backend exists) |
| `progress.md` Phase 11 Teardown | pending | Shipped: `shrine teardown <team>` plans from state (`PlanTeardown`); later touched by specs 009 and 028 |
| `progress.md` Phase 12 End-to-end dry run | pending | Pending as written: no `--verbose` flag exists and DNS is not real |
| `progress.md` Phase 13 Packaging | pending | Partly shipped: GoReleaser, GitHub releases, `install.sh`, `shrine update`. No `.deb` |
| `progress.md` Current State | "Last completed phase: 8", "Next phase: Phase 9 — Routing", "Go version: 1.24.4" | Next unshipped phase is 10; `go.mod` says 1.25.0 |
| `progress.md` Known Gaps | no entry for either descoped requirement | — |
| `specs/README.md` | Layout shows only `features/` with three files; ritual step 3 is "read `features/<spec>.md`"; routing described as "Phase 9 … SSH push" | Numbered spec directories 001–030 (005 and 007 unused); four legacy files |
| `features/routing.md` | Pending, all criteria unchecked | Superseded; shipped design differs (F1) |
| `features/logging-observer.md` | Pending | Done: `internal/engine/events.go`, `internal/ui/terminal_logger.go`; zero `fmt.Print` calls left in `internal/engine` outside the dry-run backends |
| `features/integration-tests.md` | "In progress (Phases 1–5 complete)", phases 6–12 "(pending)" | Test files exist for every pending phase (`apply`, `delete`, `describe`, `get`, `status`, `teardown`) |
| `features/daemon.md` | planned | Accurate |
| `AGENTS.md:185` | `plan.go # Plan(), PlanSingle() entry points` | `plan.go` exports `Plan` and `PlanTeardown`; `PlanSingle` is gone |
| `AGENTS.md:304` | "only files matching `{team}-{name}.yml` produced by shrine are managed" | Contradicts F1: those files are written once and then left alone |
| `specs/005-…`, `specs/007-…` | exist | Untracked, each holding one empty `checklists/` directory. Git has never tracked them |

Every `.go` filename mentioned in `AGENTS.md` exists; `PlanSingle` is the only dead symbol found.

### F10. Docs site mechanics

- Hugo + Hextra, content under `docs/content/`; guides use YAML front matter (`title`, `description`, `weight`) and the section pattern "What this guide covers / Concept / … / Common pitfalls / See also".
- Internal links are plain root-relative Markdown links (`/guides/tls/`), not `ref` shortcodes. No callout shortcodes are in use.
- CI (`.github/workflows/docs.yml`): front-matter lint, CLI drift check, Hugo build, Markdown-companion checks. **Nothing checks plain links or anchors.**
- Hugo is installed locally; `make docs-build` completes in about a second. Heading IDs are generated automatically and appear unquoted in the minified HTML (`id=common-pitfalls`).
- Existing guide weights: traefik 10, routing 20, secrets-vault 20, custom-registries 25, tls 30, team-scoped-deploy 35, publish-localhost 40.

### F11. Guide passages that contradict or omit the policy

| Page | Passage | Problem |
|---|---|---|
| `guides/tls.md` "Configure the gateway" | "Run `shrine deploy` once … when the static config is Shrine-generated (not operator-preserved), adds the `websecure` entrypoint" | False on any host that has already deployed the gateway (F1) |
| `guides/tls.md` same section | "If your `traefik.yml` was preserved from a prior operator edit …" | Same distinction |
| `guides/tls.md` "Mark an alias as TLS" | No mention that adding `tls: true` to an already-deployed app changes nothing | Omission |
| `guides/traefik.md` "Configure entrypoints" | "When `tlsPort` is set, Shrine adds a `websecure` entrypoint … to the generated static config" | True only when the file is absent |
| `guides/traefik.md` "Per-app routing", "Dashboard access" | Describe the files as written, with no lifecycle | Omission |
| `guides/routing-and-aliases.md` | Documents aliases, `stripPrefix`, `tls`; silent on changing them later | Omission |
| `troubleshooting/_index.md` | No entry for the symptom | Omission |

### F12. The wiring example works as drafted

Five manifests for a team `shop` were written and previewed: a `shop-db` Postgres resource, an `api` application, a `web` application, a `shop-db-metrics` resource that consumes `shop-db`, and — for the cross-team case — an `ops` team's `reporter` application. All validate and preview with exit code 0. The manifests and every observed message are recorded in [contracts/wiring-guide.md](contracts/wiring-guide.md). Observed along the way:

- Same-team references order themselves; the preview tags each inferred edge with the env var that caused it.
- A cross-team reference with no explicit dependency fails planning with a message that names the fix.
- With the dependency declared, the producer still needs `metadata.access` listing the consumer's team and `networking.exposeToPlatform: true`; each omission has its own message.
- Reading a key the resource does not export fails validation and names the key.

## Part 2 — Decisions

**D1. The canonical statement is its own section at the top of spec 009.**
A second-level heading, "Generated Gateway File Lifecycle (Canonical)", placed directly after the header block and before Clarifications, with the stable anchor `#generated-gateway-file-lifecycle-canonical`.
*Rationale*: FR-001 requires it to be linkable and readable without the rest of the spec. *Rejected*: a new FR in 009's requirements list (not standalone, and would be an eleventh requirement written five months after the feature shipped); a separate file (one more place to look, and the issue asks for it in 009).

**D2. Amendments are truth-first, with the original quoted in a note.**
The requirement's text is replaced by a correct statement that links to the canonical section; directly beneath it, an indented blockquote records the date, this feature, and the original wording verbatim.
*Rationale*: a reader scanning FR-009 must get the truth from the first sentence, and the history must survive for the tasks and tests that cite the ID. *Rejected*: leaving the original and appending a correction (the false sentence still reads first); strikethrough (unreadable on long sentences).

**D3. Each amended spec gets an "Amendments" section listing what changed.**
Placed directly after the header block: one dated line per amendment round naming this feature and the affected identifiers.
*Rationale*: makes the changes discoverable without reading the whole spec, and gives the terminology note (D4) a home.

**D4. Terminology is corrected once per spec, not at every occurrence.**
Specs 011 and 012 use "Shrine-generated", "operator-preserved", and "operator-owned" throughout. Their Amendments section states what those words mean in the shipped product (an existing file, whoever wrote it). Only statements that are false even with that reading get an inline amendment. Clarification Q&A records are not edited.
*Rationale*: rewriting a dozen occurrences would bury the substantive amendments and falsify the Q&A record. *Rejected*: global search-and-replace.

**D5. Descopes keep the original text and add a status marker and a note.**
The identifier gains a "(descoped 2026-10-05)" or "(partly descoped …)" marker; the original text stays; a blockquote states what shipped, what did not, and the known-gaps entry that tracks the rest.

**D6. Two known-gaps entries carry the descoped work.**
One for "the preview does not print resolved env or exports" (covers 021 FR-013 and 015 US3-AS1), one for "a non-boolean `tls` error does not name the application or alias index" (012 FR-004). Both go in the existing Known Gaps list in `specs/progress.md`.
*Rationale*: the project has no backlog document, and that list already holds deferred work with pointers. *Rejected*: opening tracker issues from this change (an outward-facing action the maintainer has not asked for).

**D7. The known-limitations section lives in the Traefik gateway guide.**
A new "Known limitations" section in `docs/content/guides/traefik.md`, before "Common pitfalls", with one subsection: "Generated gateway files are written once". Link target: `/guides/traefik/#generated-gateway-files-are-written-once`.
*Rationale*: the limitation belongs to the gateway plugin, the guide already introduces all three files, and one limitation does not justify a page (Principle IV). *Rejected*: a new `reference/known-limitations.md` (a page with one entry); the troubleshooting page (symptom-indexed, a poor home for a design statement — it gets an entry that links instead).

**D8. The section is reached by links at the point of use.**
The routing guide gains a short section, "Changing routing after the first deploy"; the TLS guide's gateway and alias sections each gain a sentence; the Traefik guide's entrypoints, per-app routing, and dashboard sections each gain a sentence; the troubleshooting page gains "A routing change in the manifest did not take effect". Each links to the D7 anchor.

**D9. Only `spec.md` is amended in each feature directory.**
Plans, research, data models, contracts, and tasks are records of how a feature was built. Exception: the two task entries FR-017 names, and nothing else.
*Rationale*: the spec is what readers and later specs treat as the statement of behaviour. Rewriting implementation records would be rewriting history, at many times the size of this change.

**D10. The wiring guide is one page built around one cast.**
`docs/content/guides/wiring-env-and-outputs.md`, weight 15, following the existing guide section pattern, listed in the guides index, linked from the manifest reference's Resource section. Cast: team `shop` with `shop-db`, `api`, `web`, `shop-db-metrics`; team `ops` with `reporter` for the cross-team step.
*Rationale*: one continuous example lets each pattern build on the last. Weight 15 places it after the gateway overview and before the pages that assume the env/outputs model. *Rejected*: seven disconnected snippets (the reference already does that).

**D11. The guide's preview section shows only real output.**
It shows the deploy order with inferred-dependency tags and the container-create lines (F7), says in one sentence that the preview does not print environment values, and shows `docker exec <container> env` as the way to confirm values after a real deploy.

**D12. The progress file is corrected in place, minimally.**
Phase 9 and Phase 11 are checked with pointers to the delivering specs. Phase 13 is split into its shipped and unshipped halves. Phases 10 and 12 stay pending; Phase 12 gains a note on what blocks it. Current State is rewritten (next phase 10, Go 1.25). One checklist entry is added for this feature, in the style of entries 024–029.
*Rejected*: back-filling entries for specs 001–022 (not asked for; the README now tells readers that the numbered directories are the record).

**D13. The specs README describes the convention rather than indexing every spec.**
It explains numbered directories and what each contains, where the current feature pointer lives, the legacy `features/` files and their status, and that 005 and 007 are intentionally unused. The session-start ritual becomes: project reference, progress file, then the numbered spec for the feature — or the legacy file if no numbered spec exists.
*Rationale*: a hand-maintained table of 29 rows is exactly the kind of metadata this feature exists to clean up. `ls specs/` is the index.

**D14. Legacy feature files gain a status and a pointer; their bodies are not rewritten.**
`routing.md` → "Superseded", naming the delivering specs and the three ways the shipped design differs (local plugin instead of SSH push; `<team>-<name>.yml` naming; files are preserved, not removed, on teardown). `logging-observer.md` → "Done", criteria checked. `integration-tests.md` → status and per-phase markers updated, each phase marked complete only after its listed scenarios are found in the test files. The README's status vocabulary gains `superseded`.

**D15. Two lines change in the project reference.**
Line 185 drops `PlanSingle()` and names `PlanTeardown()`. Line 304 is rewritten to state the write-once rule and link the canonical section.

**D16. Verification is a set of local gates plus the existing docs CI.**
Scope gate (no product files in the diff), grep gates for each success criterion that can be checked mechanically, a docs build with a link-and-anchor check over the built site (CI has none), and a re-run of the guide's manifests through the preview. Details in [quickstart.md](quickstart.md).

**D17. One change set, committed per user story.**
Five commits in priority order, so the change can be split along story lines if review asks for it.

**D18. The spec was adjusted where research contradicted it.**
See Part 3. Existing identifiers were kept; one requirement (FR-032) was added.

## Part 3 — Where research widened the scope

Each item is the same defect the issue describes, found in a place it did not look. All are included in the plan; the spec was updated to match.

| # | Finding | Why included | Spec change |
|---|---|---|---|
| E1 | Spec 004 FR-008 promises per-route files are "written and removed by Shrine" | It is the direct opposite of the canonical statement, in the spec the statement cites as its origin | FR-006 now lists 004 |
| E2 | Spec 011 SC-003 promises HTTPS on an existing deployment with one line and one deploy | Same contradiction as the edge case the issue names, same spec | Covered by FR-006 |
| E3 | Further statements in 006, 008, 012 (F4 rows marked "no") | Fixing FR-009 and leaving the edge case beside it wrong helps nobody | Covered by FR-006 |
| E4 | The non-propagating inputs are more than the spec listed: gateway `port`, `dashboard.port`, `spec.port`, primary `pathPrefix`, alias `host` | The published list must be complete to be useful | FR-004 broadened |
| E5 | Spec 015 US3-AS1 and the secrets vault guide claim the preview prints env placeholders (F7) | It is the behaviour FR-013 descopes; descoping it in 021 while 015 and a published guide still claim it would be incoherent | FR-013 sharpened, FR-032 added |
| E6 | `AGENTS.md:304` describes per-app files as "managed" | The project reference must agree with the canonical statement; the constitution requires it be kept consistent | FR-021 broadened |
| E7 | `progress.md` reports Go 1.24.4 | Part of the current-state summary FR-018 already covers | none |

## Part 4 — Seen and left alone

Reported here so they are not lost; none is part of this plan.

- **The constitution's Development Workflow section is stale in the same way as the specs README.** It says new features need a spec in `specs/features/<name>.md` and puts integration tests in `test/integration/`; practice since spec 001 is numbered directories and `tests/integration/`. Amending the constitution needs a version bump through `/speckit-constitution`.
- **`guides/traefik.md` opens with "By design, Shrine never publishes host ports on application containers."** Superseded by spec 023 (`networking.publish`).
- **Two docs pages show sample output the product does not print**: the quick start's `[dry-run] create container …` lines, and the routing guide's "Logging" section (`[shrine] alias … -> …`, which also carries a leftover TODO comment).
- **`guides/_index.md` does not list the publish-on-localhost guide.**
- **Every numbered spec's header says `Status: Draft`.** Already recorded as out of scope in the spec.
- **Specs 015 and 021 refer to `shrine apply` and `shrine dry-run`** where the commands are `shrine deploy` and `shrine deploy --dry-run`.
