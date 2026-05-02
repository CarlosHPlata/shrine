---

description: "Task list for feature 013-docs-site"
---

# Tasks: Official Shrine CLI Documentation Site

**Input**: Design documents from `/specs/013-docs-site/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md
**Tests**: Per Constitution Principle V, integration tests are written **before** implementation for any new Cobra command. Site-level verification uses CI smoke checks; full UI fidelity is spot-checked at launch.

**Organization**: Tasks are grouped by user story so each can be implemented, tested, and shipped independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Maps the task to the user story it serves (US1 / US2 / US3)
- Setup, Foundational, and Polish tasks have no story label
- File paths are relative to repo root (`/root/projects/shrine/`)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Stand up the empty Hugo site skeleton and pin the Hextra theme. No content yet.

- [X] T001 Create the Hugo site directory tree at `docs/` with subdirectories `docs/content/`, `docs/content/getting-started/`, `docs/content/cli/`, `docs/content/guides/`, `docs/content/reference/`, `docs/content/troubleshooting/`, `docs/layouts/_default/`, `docs/layouts/partials/`, `docs/static/`
- [X] T002 [P] Initialize Hugo module file at `docs/go.mod` with `module github.com/CarlosHPlata/shrine/docs`
- [X] T003 [P] Create `docs/hugo.yaml` with: `baseURL`, `languageCode: en`, `title: "Shrine CLI"`, `theme: github.com/imfing/hextra`, empty `outputs` block (filled by US2), `params` for footer + repo URL
- [X] T004 Add Hextra theme as a Hugo module (run `cd docs && hugo mod init github.com/CarlosHPlata/shrine/docs && hugo mod get github.com/imfing/hextra`); commit the resulting `docs/go.sum`
- [X] T005 [P] Add `docs/.gitignore` ignoring `resources/`, `public/`, `.hugo_build.lock`
- [X] T006 [P] Add placeholder static assets at `docs/static/favicon.ico` and `docs/static/logo.svg` (use existing `assets/` artwork if available)
- [X] T007 [P] Add Makefile targets `docs-serve` (runs `cd docs && hugo serve --buildDrafts`) and `docs-build` (runs `cd docs && hugo --gc --minify --strict`) at repo-root `Makefile`

**Checkpoint**: `cd docs && hugo --buildDrafts` builds an empty-but-valid site against the Hextra theme.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build the auxiliary `docsgen` tool that all CLI-reference content (US1) and the drift check (US3) depend on. The tool lives in a **separate Go module** at `docs/tools/docsgen/` so the main `shrine` binary stays clean of docs-gen dependencies (per the project memory note "Auxiliary tooling lives in a separate Go module"). Unit tests are in-memory only (no filesystem).

**⚠️ CRITICAL**: User-story phases cannot start until Phase 2 completes (US1 needs `make docs-gen-cli`; US3's drift check invokes the same tool).

- [X] T008 Expose the Cobra command tree to external tooling: add a `RootCmd() *cobra.Command` getter to `cmd/root.go`. The main binary itself does not use this getter; it exists solely so the auxiliary docsgen module (T010) can introspect the command tree without anything else in the main module changing.
- [X] T009 [P] Write the unit test at `docs/tools/docsgen/docsgen_test.go` against a synthetic in-memory Cobra tree using `bytes.Buffer` writers — NO filesystem writes. Cases: front-matter + banner produced; title/description derived from `Use`/`Short`; `description` escapes embedded quotes; hidden commands skipped; `-include-hidden` reverses the skip; `help` always skipped.
- [X] T010 Implement the docsgen module at `docs/tools/docsgen/`. Files: `go.mod` (own module, `replace github.com/CarlosHPlata/shrine => ../../..`), `docsgen.go` (package `docsgen` exposing `Generate(rootCmd, dir, Options)` and `WriteCommand(cmd, w)` for in-memory testing). Use `github.com/spf13/cobra/doc.GenMarkdownCustom` per command (not `GenMarkdownTreeCustom`, so hidden-skip happens naturally during traversal). Set `cmd.DisableAutoGenTag = true` around each call for byte-stability. Helper names per Constitution VII: `generateCommandTree`, `writeMarkdownForCommand`, `ShouldSkipCommand` (exported for the black-box test), `frontMatterFor`, `linkHandler`, `cleanGeneratedFiles`. `cobra/doc` and its transitive deps (`md2man`, `blackfriday`, `yaml.in/yaml/v3`) live ONLY in this module's `go.mod` — never in the main module's.
- [X] T011 Add the binary entry point at `docs/tools/docsgen/cmd/docsgen/main.go` (package `main`). Uses Go's stdlib `flag` package (not Cobra) for `-out`, `-clean`, `-include-hidden`. Calls `cmd.RootCmd()` to get the tree, then `docsgen.Generate(...)`. Update `Makefile` `docs-gen-cli` target to `cd docs/tools/docsgen && go run ./cmd/docsgen -out ../../content/cli -clean`.
- [X] T012 Verify: `cd docs/tools/docsgen && go test ./...` is GREEN; `make docs-gen-cli` produces one Markdown file per visible Cobra subcommand under `docs/content/cli/`; main module stays clean: `grep -E "cobra/doc|md2man|blackfriday|yaml.in" /root/projects/shrine/go.mod` returns nothing.

**Checkpoint**: `make docs-gen-cli` produces a populated `docs/content/cli/` and the main `shrine` binary's `go.mod` has zero docs-related dependencies. Foundational complete; user-story work can begin.

---

## Phase 3: User Story 1 - New operator finds install and first-run docs (Priority: P1) 🎯 MVP

**Goal**: A new visitor lands on the public docs site, follows clearly signposted Getting Started content, and reaches a successful first deploy without external help.

**Independent Test**: Hand a fresh user the docs URL only; observe whether they install and run their first Shrine command using only what the site shows them.

### Implementation for User Story 1

- [X] T013 [P] [US1] Author `docs/content/_index.md` — home page: project name, one-paragraph intro, three primary CTAs (Install / Quick Start / CLI Reference), version stamp from `git describe` (via Hugo `params` set in CI build)
- [X] T014 [P] [US1] Author `docs/content/getting-started/_index.md` — section landing with `cascade.type: docs` and a curated TOC of the two pages below
- [X] T015 [P] [US1] Author `docs/content/getting-started/install.md` — installation methods: `go install`, `install.sh`, release-binary download; verify-install command; troubleshooting links
- [X] T016 [P] [US1] Author `docs/content/getting-started/quick-start.md` — end-to-end first-run example: install → write a minimal `Application` manifest → `shrine apply` → verify; matches what `shrine --help` flow produces
- [X] T017 [P] [US1] Author `docs/content/cli/_index.md` — curated intro to the CLI reference: explanation of the verb-resource convention, `--dry-run` semantics, link to per-command pages (this file is hand-edited; subcommand pages are auto-generated)
- [X] T018 [US1] Run `make docs-gen-cli` to populate per-command Markdown pages under `docs/content/cli/` (depends on Phase 2). Commit the generated files.
- [X] T019 [P] [US1] Author `docs/content/guides/_index.md` — section landing for guides
- [X] T020 [P] [US1] Author `docs/content/guides/traefik.md` — deploying the Traefik gateway plugin (lift content from `specs/001-traefik-gateway-plugin/` and the recent Traefik specs 011/012)
- [X] T021 [P] [US1] Author `docs/content/guides/routing-and-aliases.md` — `routing.aliases` configuration (lift from `specs/006-routing-aliases/` and `specs/008-alias-strip-prefix/`)
- [X] T022 [P] [US1] Author `docs/content/guides/tls.md` — TLS / HTTPS configuration end-to-end (lift from `specs/011-traefik-tlsport-config/` and `specs/012-tls-alias-routers/`)
- [X] T023 [P] [US1] Author `docs/content/reference/_index.md` — section landing for reference
- [X] T024 [P] [US1] Author `docs/content/reference/manifest-schema.md` — `apiVersion`, the three manifest kinds (Team / Resource / Application), required vs optional fields per kind
- [X] T025 [P] [US1] Author `docs/content/troubleshooting/_index.md` — list of known issues at launch with diagnostic commands
- [X] T026 [US1] Configure navigation: in `docs/hugo.yaml` set Hextra `menu.main` entries for the five top-level sections in the right order; verify ≤ 3 clicks from home to any leaf page (FR-003)
- [X] T027 [US1] Configure version stamp surfacing: in `docs/hugo.yaml` `params.version` placeholder; in `.github/workflows/docs.yml` (T028) inject `git describe --tags --always` via `--config` override; render in home page and footer partial (FR-009)
- [X] T028 [US1] Add `.github/workflows/docs.yml` that on push to `main`: sets up Go, runs `make docs-gen-cli` (which `go run`s the auxiliary docsgen tool from its own module), installs Hugo extended via `go install`, runs `hugo --gc --minify` (NOT `--strict` yet — that arrives in US3), uploads `docs/public/` artifact, deploys via `actions/deploy-pages@v4`. Add a `workflow_dispatch` trigger for manual runs.
- [X] T029 [US1] Local verification: run `make docs-serve` and confirm home page loads in < 2s (SC-005), every top-level menu entry resolves, every CLI subcommand has a page (SC-002), mobile viewport (360px) renders without horizontal scroll (FR-010).

**Checkpoint**: Public docs site is live at the GitHub Pages URL with all launch content. **MVP shipped.** Stop here to validate before continuing to US2.

---

## Phase 4: User Story 2 - AI agent ingests a doc page as clean Markdown (Priority: P2)

**Goal**: Every page exposes a one-click "copy as Markdown" button and a stable `/<path>/index.md` URL that returns the verbatim source `.md`.

**Independent Test**: Open any page in a browser, click the button, paste into a Markdown viewer; verify structural fidelity. Equivalently: `curl -s <page>/index.md` returns clean Markdown beginning with `# <title>`.

### Implementation for User Story 2

- [X] T030 [US2] Add the custom output format to `docs/hugo.yaml`: under `outputFormats.markdown` set `mediaType: text/markdown`, `baseName: index`, `suffix: md`, `isPlainText: true`, `notAlternative: true`. Under `outputs.home`, `outputs.section`, `outputs.page` add `markdown` alongside `html`.
- [X] T031 [US2] Implement `docs/layouts/_default/single.md`: render `# {{ .Title }}` (only if `.RawContent` does not already start with `# `), then `{{ .RawContent }}`. Strip front-matter (Hugo's `RawContent` already excludes it).
- [X] T032 [US2] Implement `docs/layouts/_default/list.md` for section index pages: same pattern as `single.md`.
- [X] T033 [US2] Implement the button at `docs/layouts/partials/copy-as-markdown.html`: a small `<button>` that on click does `fetch('./index.md').then(r => r.text()).then(t => navigator.clipboard.writeText(t))`, with success / failure toast and progressive enhancement (button hidden if `navigator.clipboard` is unavailable, satisfying the JS-disabled edge case).
- [X] T034 [US2] Wire the partial into the page: override Hextra's content top partial at `docs/layouts/partials/content/header.html` (or whichever Hextra partial is closest) to include `{{ partial "copy-as-markdown.html" . }}`. Verify the button appears at the top of every page kind (home, section, single).
- [X] T035 [US2] Add CI smoke step in `.github/workflows/docs.yml`: after `hugo` build, run `scripts/check-md-companions.sh` which walks `docs/public/` and asserts every `index.html` has a sibling `index.md`. Fail the job otherwise. Commit the script under `scripts/check-md-companions.sh`.
- [X] T036 [US2] Add CI shape check `scripts/check-md-shape.sh`: for every `index.md` in `docs/public/`, assert non-empty, first non-blank line begins with `# `, no `<html>` / `<body>` / `{{` markers leaked through. Wire into the docs workflow.
- [X] T037 [US2] Manual launch verification (record results in `specs/013-docs-site/checklists/requirements.md` notes): pick one page per top-level section + one CLI reference page; click the copy button; paste into a Markdown renderer; confirm structural fidelity ≥ 95% (SC-003). For any page with structural drift, add the source path and observed issue to the checklist.

**Checkpoint**: Every page has a working "copy as Markdown" button and a fetchable `/<path>/index.md`. AI-agent ingestion path delivered.

---

## Phase 5: User Story 3 - Maintainer fixes or extends docs in minutes (Priority: P3)

**Goal**: Edits flow from PR merge to live site in under 10 minutes; broken refs and CLI drift are caught at PR time.

**Independent Test**: Open a PR with a one-character edit to any `.md`; verify CI passes, merge, verify the change is live within 10 minutes (SC-004).

### Implementation for User Story 3

- [X] T038 [US3] Configure the "Edit this page" link in `docs/hugo.yaml` Hextra params (`editURL: https://github.com/CarlosHPlata/shrine/edit/main/docs/content/...`) so every page links to its source file (FR-007).
- [X] T039 [US3] Add `scripts/lint-docs-frontmatter.sh`: walk `docs/content/`, for each `*.md` parse YAML front-matter (use `yq` or a small Go helper) and assert `title` non-empty, `description` ≤ 160 chars when present, no disallowed fields (`date`, `lastmod`, `version`).
- [X] T040 [US3] Add a CI job in `.github/workflows/docs.yml` (or a new `pr-checks.yml`) that runs `scripts/lint-docs-frontmatter.sh` on every PR touching `docs/content/`.
- [X] T041 [US3] Add CI drift-check job: `mkdir -p /tmp/expected-cli && cd docs/tools/docsgen && go run ./cmd/docsgen -out /tmp/expected-cli`, then `diff -r /tmp/expected-cli docs/content/cli --exclude=_index.md` and fail if non-empty. This implements the contract in `specs/013-docs-site/contracts/cli-docs-gen.md`.
- [X] T042 [US3] Switch the production docs build to `hugo --gc --minify --strict` so broken internal links fail the build (FR-014 edge case "broken internal link"). Update `Makefile docs-build` target accordingly.
- [X] T043 [US3] Add 404 page at `docs/layouts/404.html` with link back to home and the search box (FR-014).
- [X] T044 [US3] Add the report-an-issue link in the Hextra footer partial: a per-page link to `https://github.com/CarlosHPlata/shrine/issues/new?title=docs:%20<page-path>&labels=docs` (FR-013). Override Hextra's footer partial at `docs/layouts/partials/footer.html` to inject it.
- [X] T045 [US3] Author `docs/content/getting-started/contributing-to-docs.md` from `specs/013-docs-site/quickstart.md` content; ensure it covers local serve, adding a page, regenerating CLI ref, the copy-as-MD verification path, and the PR workflow.

**Checkpoint**: Edit-to-live loop is fast, drift-safe, and link-safe. All three user stories functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T046 [P] Update `AGENTS.md` to point to the docs site URL and document the `<page>/index.md` raw-Markdown pattern as the canonical agent-ingest path
- [X] T047 [P] Update repository `README.md` to link to the docs site under a "Documentation" section
- [X] T048 [P] Add a `docs-check` target to the `Makefile` that runs lint + drift + companions scripts locally (the `docs-gen-cli` target was already added in T011/T012).
- [X] T049 Run the full integration suite as the final gate: `make test-integration` (Constitution Principle V). Per the integration-tests-are-slow memory: only run this once at the end, not during iteration.
- [X] T050 Walk through `specs/013-docs-site/quickstart.md` step-by-step on a fresh checkout; fix any inaccuracies found
- [X] T051 Update `specs/progress.md` to mark feature 013-docs-site as `[x]` complete and link to the PR

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately. T002, T003, T005, T006, T007 are [P]; T001 must precede T004 (theme module needs the directory).
- **Foundational (Phase 2)**: The auxiliary `docsgen` module is purely Go and lives entirely under `docs/tools/docsgen/`; can technically run in parallel with Phase 1. Phase 1 must complete before US1's content tasks can use the generated output.
- **User Story 1 (Phase 3)**: Depends on **both** Phase 1 (Hugo site exists) and Phase 2 (T018 needs `make docs-gen-cli`).
- **User Story 2 (Phase 4)**: Depends on Phase 1 and at least one US1 page existing for testing. Not strictly dependent on US1 fully shipping.
- **User Story 3 (Phase 5)**: Depends on Phase 2 (drift check uses the docsgen tool) and Phase 4 (extends the same `docs.yml` workflow with stricter checks).
- **Polish (Phase 6)**: Depends on the user stories you intend to ship; T049 is the final gate.

### User Story Dependencies (independence)

- **US1 (P1 / MVP)**: Independent — ships a complete launchable site. Stops here for MVP.
- **US2 (P2)**: Independent — can be developed and validated locally without auto-deploy. Adds the copy-as-MD button and raw-MD URL.
- **US3 (P3)**: Independent in value (faster edit loop, drift safety) but technically extends the workflow file added in US1. Safe to develop after US1 in any sprint.

### Within Each User Story

- The auxiliary `docsgen` tool is exercised by in-memory unit tests (T009) and the US3 drift-check job (T041); no `NewDockerSuite` integration test applies because the main `shrine` binary's behavior is unchanged.
- Within US1: content authoring tasks (T013–T025) are heavily [P]. T018 depends on Phase 2. T026/T027/T028/T029 are sequential and run after content exists.
- Within US2: T030 (output format) → T031/T032 (templates) → T033 (button) → T034 (wire-in). T035/T036 are CI-only and [P]. T037 is launch validation.
- Within US3: T038, T039, T040 are [P] with each other; T041 depends on T028 (workflow exists); T042 depends on T028; T043, T044, T045 are [P].

### Parallel Opportunities

- All Phase 1 tasks marked [P] can run together (T002, T003, T005, T006, T007).
- T009 (unit test) can run in parallel with T008 (integration test) — different files. But T010 cannot start until T008 has been observed to fail.
- All US1 content authoring tasks (T013–T017, T019–T025) are [P] — different files, no cross-dependencies. A team can split them.
- US1 (Phase 3) and US2 (Phase 4) can be developed in parallel by different developers once Phase 2 is complete, then merged.
- All Polish tasks marked [P] (T046, T047, T048) can run together.

---

## Parallel Example: User Story 1 content authoring

```bash
# After Phases 1 + 2 complete, fan out content tasks:
Task: "Author docs/content/_index.md"                              # T013
Task: "Author docs/content/getting-started/_index.md"              # T014
Task: "Author docs/content/getting-started/install.md"             # T015
Task: "Author docs/content/getting-started/quick-start.md"         # T016
Task: "Author docs/content/cli/_index.md"                          # T017
Task: "Author docs/content/guides/traefik.md"                      # T020
Task: "Author docs/content/guides/routing-and-aliases.md"          # T021
Task: "Author docs/content/guides/tls.md"                          # T022
Task: "Author docs/content/reference/manifest-schema.md"           # T024
Task: "Author docs/content/troubleshooting/_index.md"              # T025

# Then sequential: T018 (run docs gen), T026 (nav), T027 (version stamp), T028 (workflow), T029 (verify)
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Complete Phase 1 (Setup) — empty Hugo site builds.
2. Complete Phase 2 (Foundational) — auxiliary `docsgen` tool works under `docs/tools/docsgen/`; unit tests green; main `go.mod` clean.
3. Complete Phase 3 (User Story 1) — public docs site live with all launch content.
4. **STOP and VALIDATE**: hand the URL to a fresh user; confirm SC-001, SC-002, SC-005, SC-007.
5. Tag and announce the docs site.

### Incremental Delivery

1. Setup + Foundational → infrastructure ready.
2. Add US1 → site is live (MVP).
3. Add US2 → AI agents can ingest pages cleanly.
4. Add US3 → maintainer loop is tight, drift is caught.
5. Polish → repository + AGENTS.md cross-link the new docs.

### Parallel Team Strategy

- Developer A: Phase 2 (Cobra command + integration test).
- Developer B: Phase 1 + start of US1 content authoring.
- Once Phase 2 lands: Developer A picks up US3 plumbing; Developer B continues US1; a third developer can take US2 in parallel.

---

## Notes

- [P] tasks touch different files and have no incomplete-task dependencies.
- [Story] labels exist only in user-story phases — Setup, Foundational, and Polish phases never carry [Story].
- TDD applies to the new Cobra command per Constitution Principle V — write the integration test first and confirm it fails before implementing.
- Integration tests are slow (~10 min full run); use `go test ./internal/handler/docsgen/...` for iteration, run `make test-integration` only as the final gate (T049). This is recorded as a feedback memory.
- Unit tests must not write to the filesystem (use `bytes.Buffer` writers); filesystem use is acceptable in integration tests via `t.TempDir()`. This is recorded as a feedback memory.
- Commit after each task or logical group; the optional `after_*` hooks in `.specify/extensions.yml` will offer to do this for you.
- Do not edit files under `docs/content/cli/` by hand — they are owned by the auxiliary `docsgen` tool (`make docs-gen-cli`).
