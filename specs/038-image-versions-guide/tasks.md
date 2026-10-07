---
description: "Task list for the image-versions operator guide"
---

# Tasks: Operator guide, managing image versions

**Input**: Design documents from `specs/038-image-versions-guide/`
**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/](contracts/), [quickstart.md](quickstart.md)

**Tests**: The ticket names no integration scenario, and no executable behaviour changes. The gate is the documentation build and its checks (`make docs-check`), the grep link check, and `go test ./...` with the integration suite compile-checked (quickstart steps 0 to 3).

**Organization**: One phase per user story of spec.md. Stories 1 to 6 are sections of one page, `docs/content/guides/image-versions.md`, so their tasks run in order. The troubleshooting entry, the help texts, and the manifest reference are separate files and can run in parallel with them.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1 to US7)

---

## Phase 1: Setup

**Purpose**: A reproducible capture of every block the binary prints without a container runtime.

- [X] T001 Copy the session's capture script to `specs/038-image-versions-guide/capture.sh`. It must take the binary path as `$1` (default `./shrine`) and work in a temporary directory it creates and removes. It sets `DOCKER_HOST=tcp://127.0.0.1:1`, writes the `shop` manifests of research R4 and the seeded `pins.txt` and `deployments.txt` with the placeholders of research R5, rewrites the temporary manifest directory to `/home/me/shop/manifests` in its output, and strips trailing spaces. Also update quickstart.md step 4 to run it.
- [X] T002 Build the binary from this branch (`go build -o /tmp/<scratch>/shrine .`), run `bash specs/038-image-versions-guide/capture.sh <binary>`, and keep the output in the session scratchpad as the source for every captured block (never committed).

---

## Phase 2: Foundational

**Purpose**: The page exists, is listed, and states the concept and the example that every journey section builds on.

- [X] T003 Create `docs/content/guides/image-versions.md` with the front matter of contracts/guide-outline.md and sections 0 to 2: what the guide covers; the concept (the three-policy table with who owns the version; the precedence manifest field, then configuration default, then derived rule; what a pin records; the vocabulary of PRD section 5 in one short list; the note that exact versions, dates, ids, and paths are illustrative; the `$` prompt and `…` trim conventions); and the example (`config.yml` with `specsDir`, the team, and the `api`, `shop-db`, and `cache` manifests exactly as `capture.sh` writes them)
- [X] T004 Add `- [Managing image versions](image-versions/) — Pin an image at its newest version, see which version runs, and move it on purpose.` as the last list line of `docs/content/guides/_index.md`

**Checkpoint**: `make docs-build` succeeds and the page is listed.

---

## Phase 3: User Story 1 - Pin a service and keep its version (Priority: P1) 🎯 MVP

**Goal**: The reader learns how to pin, and what happens to a pinned version across redeploys, recreation, teardown, and a wiped image cache (J1, J2).

**Independent Test**: From sections 3 and 4 alone, the reader answers "how do I pin" and "what happens on prune", and can name where pins live.

- [X] T005 [US1] Write section 3, "Pin a service at its newest version" (J1), in `docs/content/guides/image-versions.md`. It contains: the captured validation error for `traefik/whoami:v1.10.1` and `version: "17"` under `Pinned` (trimmed after `Error: Spec validation errors`); the captured `deploy --dry-run` with the `would resolve newest and pin` lines; the assembled `bump resource shop-db -v 16` output (`🔎 Resolving image for shop.shop-db (postgres:16)`, `    ✅ Pulled image postgres:16`, `  📌 Bumped shop.shop-db to 16@2d6f0b8e4a1c`, `Pinned shop/shop-db at 16@2d6f0b8e4a1c; run "shrine deploy" to apply`), introduced as choosing a database's first version in advance; the captured dry run that now shows `-> pinned postgres@sha256:2d6f… (16, 2026-10-01)`; the assembled first deploy, trimmed after the resolution block, with `📌 Pinned shop.api at latest@3f2a9c1b4d7e`, `📌 Using pinned shop.shop-db 16@2d6f0b8e4a1c (since 2026-10-01)`, and `🔎 Resolved shop.cache redis:7.4@e1c7a93f5b20`; and the assembled redeploy a week later, after upstream `latest` moved, showing the same exact version with `(since 2026-10-01)`. Every assembled line must match its research R2 source.
- [X] T006 [US1] Write section 4, "Rebuild the host" (J2), in `docs/content/guides/image-versions.md`: `shrine teardown shop`, then `docker image prune -a -f`, then the assembled deploy whose resolution block shows `    ✅ Pulled image traefik/whoami@sha256:3f2a…` (written in full) before `📌 Using pinned`, plus the same for `shop-db`. Add prose on where pins live (`<state-dir>/<team>/pins.txt`, by default under `~/.local/share/shrine`, so keep or back up the state directory when rebuilding; research R6), that a pin is fetched by exact version and never by tag, and that a pin lasts only while the registry serves the exact version, linking forward to section 11.
- [X] T007 [US1] Update the dry-run expectations in `specs/038-image-versions-guide/capture.sh` so the `-v 16` bump and the matching seeded pin produce the section 3 blocks. Re-run T002 and confirm every captured block in sections 3 and 4 appears in the capture output.

**Checkpoint**: The MVP is in place: an operator can pin and knows what survives.

---

## Phase 4: User Story 2 - See which version is deployed (Priority: P2)

**Goal**: The reader reads `get`, `describe`, and `status` (J3).

**Independent Test**: Given the section 5 outputs, the reader says which rows are pinned, at which readable and exact version, since when, and whether a pin is waiting.

- [X] T008 [US2] Write section 5, "See which version runs" (J3), in `docs/content/guides/image-versions.md`, with:
  - the captured `get deployed`, with `latest@3f2a9c1b4d7e`, `redis:7.4`, and `16@2d6f0b8e4a1c`;
  - the captured `get resources --team shop`;
  - `describe resource shop-db`, captured except for its `Running image: postgres@sha256:2d6f…` line, which is assembled and written in full;
  - the assembled `status shop` table, built with `statusRowFormat`, its IMAGE cells in `shortImageReference` form;
  - the captured `describe` with the runtime unreachable.

  Explain the two halves of the readable form and that a manifest-owned row shows the reference the manifest named. Say that `get` needs no runtime. Say that a `Pinned:` line differing from `Running image:` is a pin waiting for the next deploy.

---

## Phase 5: User Story 3 - Upgrade, roll back, and take the newest on purpose (Priority: P3)

**Goal**: The reader moves a pinned artifact deliberately (J4, J5, J6).

**Independent Test**: From sections 6 to 8 alone, the reader upgrades in two commands, rolls back to the exact earlier version, and takes the newest, and says when the container changes.

- [X] T009 [US3] Write section 6, "Upgrade one artifact" (J4), in `docs/content/guides/image-versions.md`. Include:
  - the assembled `bump resource shop-db -v 17`, ending `Bumped shop/shop-db: 16@2d6f0b8e4a1c -> 17@9c1b4d7e3f2a; run "shrine deploy" to apply`;
  - `describe` showing `Pinned:` at 17 and `Running image:` still at 16 (captured, apart from `Running image:`);
  - the assembled deploy whose resolution block shows `Using pinned shop.shop-db 17@9c1b4d7e3f2a (since 2026-10-20)`, then `…`, then the `shop-db` recreate lines of research R2;
  - the assembled failure of `bump resource shop-db -v 71`, trimmed to its `Error: resource "shop-db": pulling image "postgres:71": …` line, with the statement that the pin is unchanged;
  - the captured invalid `-v postgres:18` error;
  - the captured `bump resource shop-db -v 17 --dry-run`;
  - the captured refusal of `bump resource cache -v 8`.

  Add one sentence that Shrine moves the image, not the data, so a major database upgrade still needs the database's own procedure.
- [X] T010 [US3] Write section 7, "Roll back" (J5), in `docs/content/guides/image-versions.md`. Rolling back is the same command. `-v 16` resolves whatever tag 16 points at now, which may be a newer 16.x. `-v sha256:<exact version>` returns to exactly the earlier version, and the reader finds that value in the previous version a bump printed, or in `describe` before the bump. Show the assembled `bump resource shop-db -v sha256:2d6f…` (written in full), with output `Bumped shop/shop-db: 17@9c1b4d7e3f2a -> 2d6f0b8e4a1c; run "shrine deploy" to apply`, then the assembled deploy resolution line `Using pinned shop.shop-db 2d6f0b8e4a1c (since 2026-10-21)`.
- [X] T011 [US3] Write section 8, "Take the newest again" (J6), in `docs/content/guides/image-versions.md`: the captured `bump app api --dry-run`, then the assembled `bump app api` with no `-v` ending `Bumped shop/api: latest@3f2a9c1b4d7e -> latest@5e8c2b7a1f04; run "shrine deploy" to apply`. State that newest is only ever taken by this explicit act, never by a redeploy.

---

## Phase 6: User Story 4 - Make pinning the house rule (Priority: P4)

**Goal**: The reader knows what the configuration default changes (J7).

**Independent Test**: From section 9 alone, the reader predicts which manifests pin, which are refused, and which keep their own policy, and fixes a refused one both ways.

- [X] T012 [US4] Write section 9, "Make pinning the house rule" (J7), in `docs/content/guides/image-versions.md`, with:
  - the `imagePullPolicy: Pinned` line in `config.yml`;
  - the captured refusal naming `cache`, trimmed after `Error: Spec validation errors`, with the statement that nothing was deployed;
  - the two ways out: add `imagePullPolicy: IfNotPresent` to `cache`, or drop its `version` line;
  - the captured dry run after the first way out;
  - the captured `generate application web --team shop` line and the manifest it writes, showing `image: web`;
  - the rule that a manifest's own field wins;
  - what removing or changing the default does: the pins it created are released on the next deploy, and a resource that relied on `Pinned` to omit its version must name one again.

  Link the README configuration section.

---

## Phase 7: User Story 5 - Retire an artifact (Priority: P5)

**Goal**: The reader knows what keeps a pin and what releases one (J8).

**Independent Test**: From section 10 alone, the reader lists every action that releases a pin and every one that does not, and retires a resource in the right order.

- [X] T013 [US5] Write section 10, "Retire an artifact" (J8), in `docs/content/guides/image-versions.md`. Include:
  - `shrine teardown shop`, then the assembled deploy resolution block reusing every pin with its original date;
  - the assembled refusal `Error: resource "shop/shop-db" still has a container; run "shrine teardown shop" first`;
  - after a teardown, the captured `delete resource shop-db --dry-run` and `delete resource shop-db` (`Released image pin for shop/shop-db.`);
  - the captured `delete team shop` (`Released 1 image pin(s) for team "shop".`, `Deleted team "shop" from state.`), noting that `delete team` has no `--dry-run`;
  - the list of the four things that release a pin (the three deletes, and a deploy under `Always` or `IfNotPresent`), and of what never does (redeploy, recreation, teardown).

---

## Phase 8: User Story 6 - Recover when a pin cannot be honoured (Priority: P6)

**Goal**: An operator who hits the "no longer served" error finds the way out (risk "registry retention", R-14).

**Independent Test**: Starting from the error message, the reader finds the entry, learns nothing changed, and knows the command.

- [X] T014 [P] [US6] Add the section `## A deploy stops because a pinned version is no longer served` before "See also" in `docs/content/troubleshooting/_index.md`, per contracts/docs-touch-points.md. It shows the assembled `Error: application "api": pinned exact version "traefik/whoami@sha256:3f2a…" for shop/api is no longer served by the registry; run "shrine bump application api" to choose another version: pulling image "traefik/whoami@sha256:3f2a…": Error response from daemon: manifest for traefik/whoami@sha256:3f2a… not found: manifest unknown: manifest unknown`, with the exact version written in full. It also covers what it means, that nothing changed, the bump fix, the access-error case, and a link to `/guides/image-versions/`.
- [X] T015 [US6] Write section 11, "When a pin cannot be honoured", in `docs/content/guides/image-versions.md`: two sentences and a link to the troubleshooting entry's anchor

---

## Phase 9: User Story 7 - One vocabulary across the documentation (Priority: P7)

**Goal**: Every audited page uses the PRD terms with their PRD meaning (T8-02).

**Independent Test**: For each of the eight terms, every use on the audited pages carries its PRD meaning, and `shrine <cmd> --help` matches the generated page.

- [X] T016 [P] [US7] Add the VERSION paragraph of contracts/docs-touch-points.md to the `Long` text of the `deployed`, `applications`, and `resources` subcommands in `cmd/get.go`
- [X] T017 [P] [US7] Replace the record paragraph of the `app` and `resource` subcommands' `Long` text in `cmd/describe.go` with the contract's wording
- [X] T018 [P] [US7] Replace the IMAGE sentence in the three `Long` texts of `cmd/status.go` with the contract's wording
- [X] T019 [US7] Run `make docs-gen-cli` and confirm with `git status --short docs/content/cli` that only the `get_*`, `describe_app`, `describe_resource`, and `status*` pages changed
- [X] T020 [P] [US7] Make the edits to the image pull policy section of `docs/content/reference/manifest-schema.md` that contracts/docs-touch-points.md lists:
  - link the guide;
  - switch the lifecycle sample lines to `shop.api`;
  - replace the bump example with `bump resource shop-db -v 17` and its output line;
  - replace the "Reading what is pinned" example with the captured `get resources` table and the `Pinned:` and `Running image:` lines for shop-db (pinned at 17, running 16), written in full.

  Change no rule or behaviour statement.
- [X] T021 [US7] Sweep the guide, the troubleshooting entry, `docs/content/reference/manifest-schema.md`, the README configuration section, and the regenerated pages for `bump`, `delete`, `describe`, `get`, `status`, and `generate` against the eight PRD terms. Each page's first use of "digest" must name it the exact version, and no page may use "version" alone for the exact version. Fix any remaining hit at its source: help text, then regenerate.

---

## Phase 10: Polish & cross-cutting

- [X] T022 Write sections 12, "What this does not do" (FR-010), and 13, "Where to read more" (FR-014), in `docs/content/guides/image-versions.md`
- [X] T023 [P] Append the *Amended by T8 (spec 038)* line of research R10 under "T8. Operator guide" in `specs/epics/pinned-image-versions/design.md`
- [X] T024 Read every assembled block in the guide, the troubleshooting entry, and the manifest reference against its source in research R2, line by line: indentation, emoji, spacing, quotes, and punctuation
- [X] T025 Run quickstart steps 0 to 3: `go test ./...`, `go vet -tags integration ./tests/integration/...`, `make docs-gen-cli` (no further drift), `make docs-check`, and the grep link check over `docs/public`. All must pass.
- [X] T026 Run the quickstart step 5 reader check. Each of the six M7 questions must point to a heading of the guide (SC-001).
- [X] T027 Add the 038 entry at the top of the completed list in `specs/progress.md`, in the form of the 031 to 037 entries: summary, deviations, acceptance SC-001 to SC-007, and the gate (`make docs-check`; no integration scenario)
- [X] T028 Run `graphify update .`
- [ ] T029 With the owner's go-ahead, open the pull request through `/speckit-git-pr` with `Closes #59` in its Why section, the definition-of-done list, and the capture deviation named. Then run `/shrine-pr-review`, address every finding, and confirm CI is green.

---

## Dependencies & Execution Order

- **Setup (T001 to T002)** comes first: the captured blocks come from it.
- **Foundational (T003 to T004)** blocks every story section, because they all live in the same page.
- **US1 to US5 (T005 to T013)** follow in order: one file, and each journey continues the example state of the one before.
- **US6**: T014 can run any time after Setup (a different file). T015 follows T013.
- **US7**: T016, T017, T018, and T020 can run any time after Setup and in parallel with each other. T019 follows T016 to T018. T021 follows everything that writes prose.
- **Polish**: T022 follows T015. T023 can run any time. T024 to T029 run last, in order.

## Parallel example

```text
After T004:  T005 (guide, US1)  ‖  T014 (troubleshooting)  ‖  T016 + T017 + T018 (help texts)  ‖  T020 (manifest reference)  ‖  T023 (design)
```

## Implementation strategy

1. **MVP**: Setup, Foundational, and US1. The guide then answers how to pin and what survives a prune.
2. **Increment**: US2 to US5 complete the eight journeys. US6 covers the failure path.
3. **Coherence**: US7 runs once all prose exists, so the sweep sees the final wording.
4. **Gate**: Polish T024 to T026 before the pull request.
