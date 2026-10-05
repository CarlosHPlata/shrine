# Data Model: Reconcile Specs, Tracking Metadata, and Docs with Shipped Behaviour

This feature has no runtime data. Its "model" is the set of documents it changes and the relationships that keep them consistent. Section 1 defines the entities; section 2 is the complete edit inventory, which `/speckit-tasks` should turn into tasks one row or one file at a time.

## 1. Entities

### Canonical preserve-policy statement

- **Lives in**: `specs/009-preserve-app-configs/spec.md`, anchor `#generated-gateway-file-lifecycle-canonical`
- **Content**: verbatim in [contracts/spec-amendments.md](contracts/spec-amendments.md) §1 — rule, covered files, exceptions, non-propagating inputs, how to apply a change, signals
- **Referenced by**: every preserve-policy amendment (specs 004, 006, 008, 011, 012), `AGENTS.md`
- **Mirrored by**: the known-limitations section on the docs site. The two must agree on every fact; they differ only in audience and voice.
- **Invariant**: there is exactly one. No other document restates the rule; they link to it or to its mirror.

### Amendment note

- **Fields**: date (2026-10-05), source feature (spec 030), original text (verbatim)
- **Attached to**: one requirement, success criterion, acceptance scenario, edge case, entity, or assumption
- **Invariants**: the identifier of the amended statement is unchanged; the statement reads correctly without the note; the original wording is recoverable from the note

### Descope note

- **Fields**: date, source feature, what shipped, what did not, pointer to the known-gaps entry
- **Attached to**: a requirement or acceptance scenario, whose original text is left in place and whose identifier gains a "(descoped …)" or "(partly descoped …)" marker
- **Relationship**: each descope note points to exactly one known-gaps entry; one entry may be pointed to by several notes

### Known-gaps entry

- **Lives in**: `specs/progress.md`, `## Known Gaps`
- **Fields**: what is missing, what does exist, which requirements it was descoped from, what delivering it would involve
- **Instances**: two (verbatim in [contracts/spec-amendments.md](contracts/spec-amendments.md) §3)

| Entry | Descoped from |
|---|---|
| Preview does not print resolved env or exports | 021 FR-013; 015 US3 scenario 1 |
| Non-boolean `tls` error is not named | 012 FR-004 |

### Amendments section

- **Lives in**: each amended spec, directly after its header block
- **Fields**: date, source feature, one-sentence reason, list of amended identifiers, optional terminology note
- **Instances**: specs 004, 006, 008, 009, 011, 012, 015, 021

### Known-limitations section

- **Lives in**: `docs/content/guides/traefik.md`, anchor `#generated-gateway-files-are-written-once`
- **Content**: verbatim in [contracts/docs-pages.md](contracts/docs-pages.md) §1
- **Linked from**: three places in the same guide, the routing guide, the TLS guide (twice), the troubleshooting page

### Wiring guide

- **Lives in**: `docs/content/guides/wiring-env-and-outputs.md`
- **Content**: outline, manifests, and quotable output in [contracts/wiring-guide.md](contracts/wiring-guide.md)
- **Cast**: team `shop` (`shop-db`, `api`, `web`, `shop-db-metrics`); team `ops` (`reporter`)
- **Linked from**: the guides index, the manifest reference
- **Invariant**: every manifest in it passes `shrine deploy --dry-run` as written; every output block in it was printed by the product

### Tracking metadata

| Document | Status vocabulary | Role in the session-start ritual |
|---|---|---|
| `specs/progress.md` phase list | `[x]` / `[ ]`, with sub-items where a phase is partly shipped | Step 2 |
| `specs/README.md` | — | Defines the ritual and the layout |
| `specs/features/*.md` Status line | `pending`, `in-progress`, `done`, `superseded` (new), `planned` (existing, used by `daemon.md`) | Step 3 for features with no numbered spec |
| `AGENTS.md` | — | Step 1 |

## 2. Edit inventory

One row per file. "Contract" says where the required content or meaning is fixed.

### User Story 1 — canonical preserve policy

| File | Change | Contract | FR |
|---|---|---|---|
| `specs/009-preserve-app-configs/spec.md` | Add Amendments section; add canonical section | spec-amendments §1, §2 | 001–004 |
| `specs/004-preserve-traefik-yml/spec.md` | Amendments section; FR-008 | spec-amendments §4 | 006, 007 |
| `specs/006-routing-aliases/spec.md` | Amendments section; edge case, FR-009, SC-004 | spec-amendments §4 | 005–007 |
| `specs/008-alias-strip-prefix/spec.md` | Amendments section; US1 Independent Test, edge case, FR-007, SC-001 | spec-amendments §4 | 005–007 |
| `specs/011-traefik-tlsport-config/spec.md` | Amendments section with terminology note; edge case, FR-003, SC-003 | spec-amendments §4 | 005–007 |
| `specs/012-tls-alias-routers/spec.md` | Amendments section with terminology note; US2-AS2, SC-005 | spec-amendments §4 | 005–007 |

### User Story 2 — operator-facing limitation

| File | Change | Contract | FR |
|---|---|---|---|
| `docs/content/guides/traefik.md` | New "Known limitations" section; four link edits | docs-pages §1, §2 | 008, 009 |
| `docs/content/guides/routing-and-aliases.md` | New section "Changing routing after the first deploy" | docs-pages §2 | 009 |
| `docs/content/guides/tls.md` | Two corrections; one link sentence | docs-pages §2, §3 | 009, 010 |
| `docs/content/troubleshooting/_index.md` | New symptom entry | docs-pages §2 | 009 |

### User Story 3 — specs describe what shipped

| File | Change | Contract | FR |
|---|---|---|---|
| `specs/009-preserve-app-configs/spec.md` | Eight orphan-warning statements (same Amendments entry as US1) | spec-amendments §5 | 012 |
| `specs/015-infisical-secrets-vault/spec.md` | Amendments section; seven old-shape statements; US3-AS1 descope | spec-amendments §6 | 011, 032 |
| `specs/021-resource-env-output-split/spec.md` | Amendments section; FR-013 descope; US1 Independent Test | spec-amendments §6 | 013, 015 |
| `specs/021-resource-env-output-split/tasks.md` | T029 → descoped task | spec-amendments §2, §6 | 017 |
| `specs/012-tls-alias-routers/spec.md` | FR-004 partly descoped; SC-004 (same Amendments entry as US1) | spec-amendments §6 | 014, 015 |
| `specs/012-tls-alias-routers/tasks.md` | T006 description corrected | spec-amendments §6 | 017 |
| `specs/progress.md` | Two Known Gaps entries | spec-amendments §3 | 016 |
| `docs/content/guides/secrets-vault.md` | "Dry-run behaviour" section corrected | docs-pages §4 | 032 |

### User Story 4 — wiring guide

| File | Change | Contract | FR |
|---|---|---|---|
| `docs/content/guides/wiring-env-and-outputs.md` | New page | wiring-guide (all); docs-pages §5 | 023–027 |
| `docs/content/guides/_index.md` | One list entry | docs-pages §5 | 023 |
| `docs/content/reference/manifest-schema.md` | One link sentence in the Resource section | docs-pages §5 | 028 |

### User Story 5 — tracking metadata

| File | Change | Decision | FR |
|---|---|---|---|
| `specs/progress.md` | Phases 9, 11 checked with pointers; Phase 13 split; Phase 12 note; Current State rewritten; entry for this feature | research D12 | 018 |
| `specs/README.md` | Layout, ritual, legacy-file table, unused numbers, status vocabulary | research D13 | 019, 022 |
| `specs/features/routing.md` | Status → superseded; pointer and three differences | research D14 | 020 |
| `specs/features/logging-observer.md` | Status → done; criteria checked | research D14 | 020 |
| `specs/features/integration-tests.md` | Status and per-phase markers, each verified against the test files first | research D14 | 020 |
| `AGENTS.md` | Line 185 (planner entry points); line 304 (generated files) | research D15 | 021 |
| `specs/005-traefik-entrypoints/`, `specs/007-fix-traefik-dynamic-dashboard/` | Remove (untracked; local only, no diff) | research F9 | 022 |

### Files touched by more than one story

`specs/009-…/spec.md` (US1, US3), `specs/012-…/spec.md` (US1, US3), `specs/progress.md` (US3, US5), `docs/content/guides/traefik.md` (US2 only, but five edits). Tasks for these must be sequenced, not parallel.

### Not edited

- Clarification Q&A records in any spec.
- `plan.md`, `research.md`, `data-model.md`, `contracts/`, and — apart from the two tasks above — `tasks.md` of any earlier feature (research D9).
- Any file under `cmd/`, `internal/`, `tests/`, `main.go`, `Makefile`, `.github/`, `.goreleaser.yml`.
- `docs/content/cli/` (generated).
- `specs/features/daemon.md` (accurate).
- `.specify/memory/constitution.md` (research Part 4).

## 3. Consistency rules

These are what a reviewer checks; [quickstart.md](quickstart.md) turns the mechanical ones into commands.

1. Every preserve-policy amendment links the canonical anchor; none restates the rule at length.
2. The canonical statement and the known-limitations section list the same files, the same non-propagating inputs, and the same way to apply a change.
3. Every descope note points to a known-gaps entry that exists; every known-gaps entry names the requirements it came from.
4. No amended identifier differs from its original.
5. Every output block in the docs changed by this feature appears in research F3 or in the wiring-guide contract.
6. No file outside the inventory above appears in the diff.
