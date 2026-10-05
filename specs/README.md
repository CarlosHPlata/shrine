# Shrine — Specs

This directory is the single source of truth for feature specifications, architecture decisions, and project progress.

## Design Principle

Specs are **provider-agnostic**. They describe what to build, not which AI to use or how it should behave. Any AI assistant (Claude, GPT, Gemini, etc.) can read these files and pick up the work.

## Starting a Session

Regardless of which AI tool you're using:

1. Read `../AGENTS.md` — complete project reference: manifest schemas, architecture, CLI commands, networking model
2. Read `progress.md` — phase checklist, current state, design decisions, known gaps
3. Read the spec for the feature you're working on: `NNN-short-name/spec.md`, with `plan.md` and `tasks.md` beside it. The feature in progress is named in `../.specify/feature.json` and in the pointer at the top of `../CLAUDE.md`. For the four features specified before numbered directories existed, read `features/<name>.md` instead.

That's all. No AI-specific config required.

## Directory Layout

```
specs/
├── README.md               ← this file
├── progress.md             ← phase checklist, current state, decisions, known gaps
├── NNN-short-name/         ← one directory per feature, numbered in order
│   ├── spec.md             ← what and why: user stories, requirements, success criteria
│   ├── plan.md             ← how: approach, constitution check, files touched
│   ├── research.md         ← findings and decisions behind the plan
│   ├── data-model.md       ← entities, or for docs features the edit inventory
│   ├── quickstart.md       ← how to verify the feature
│   ├── contracts/          ← interfaces and exact outputs the feature commits to
│   ├── checklists/         ← spec quality checklist
│   └── tasks.md            ← ordered task list, checked off as work lands
└── features/               ← legacy: one file per feature, from before numbered specs
```

## Numbered Specs

Every feature since the Traefik gateway plugin has its own numbered directory, produced by the Spec Kit workflow under `../.specify/`. Listing this directory is the index; there is no separate table to keep in step.

- **Numbers are sequential, and 005 and 007 are intentionally unused.** Both belonged to features that were withdrawn before they were specified.
- **Shipped status lives in `progress.md`.** The `Status: Draft` line in a numbered spec's header is the template default and is not maintained.
- **A spec that a later feature contradicts is amended in place, not silently rewritten.** It gains an Amendments section at the top, the statement is corrected, and the original wording is kept in a dated note beneath it. Requirement identifiers never change. `009-preserve-app-configs/spec.md` shows the pattern.
- **Generated gateway files have one canonical rule.** It is stated in `009-preserve-app-configs/spec.md` under "Generated Gateway File Lifecycle (Canonical)"; other specs link to it rather than restating it.

## Legacy Feature Files

Four features were specified as single files under `features/` before numbered directories were introduced. They are kept as design records.

| File | Status | Notes |
|---|---|---|
| `features/routing.md` | superseded | Shipped as a local gateway plugin with a different design. See `001-traefik-gateway-plugin/` and specs 002, 004, 006, 008–012, 016, 018, 024. |
| `features/logging-observer.md` | done | Later touched by `027-app-ui-unit-coverage/` and `028-fix-teardown-event-names/`. |
| `features/integration-tests.md` | in-progress | Phases 1–6, 8, and 10–12 complete; phases 7 and 9 are each missing one listed case. Extended by specs 026–029. |
| `features/daemon.md` | planned | Not started. |

## What a Good Spec Contains

Numbered specs follow `../.specify/templates/spec-template.md`. Legacy feature files use the shape below.

| Section | Purpose |
|---|---|
| **Status** | `pending`, `planned`, `in-progress`, `done`, or `superseded` |
| **Goal** | One sentence: what problem this solves |
| **Context** | What already exists, what's missing, relevant prior decisions |
| **Acceptance Criteria** | Testable list of what "done" looks like |
| **Implementation Notes** | Constraints, design hints, open questions to settle first |

## Provider-Specific Adapters

`../agents/` holds thin adapter files — one per AI consumer. Each adapter contains only the provider-specific persona or session-start instructions, and points back here for the actual specs. The directory is git-ignored, so adapters are kept locally rather than in the repository.

To onboard a new AI tool, add `../agents/<provider>.md` that references `specs/`.
