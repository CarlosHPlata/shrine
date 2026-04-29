# Shrine — Specs

This directory is the single source of truth for feature specifications, architecture decisions, and project progress.

## Design Principle

Specs are **provider-agnostic**. They describe what to build, not which AI to use or how it should behave. Any AI assistant (Claude, GPT, Gemini, etc.) can read these files and pick up the work.

## Starting a Session

Regardless of which AI tool you're using:

1. Read `../AGENTS.md` — complete project reference: manifest schemas, architecture, CLI commands, networking model
2. Read `progress.md` — phase checklist, current state, design decisions, known gaps
3. Read `features/<spec>.md` — the spec for the feature you're working on next

That's all. No AI-specific config required.

## Directory Layout

```
specs/
├── README.md               ← this file
├── progress.md             ← phase checklist, current state, decisions, known gaps
└── features/               ← one file per feature
    ├── routing.md          ← Phase 9: Traefik route generation + SSH push
    ├── logging-observer.md ← Decoupled event stream for CLI output
    └── integration-tests.md← Integration test suite: architecture, phases, API
```

## What a Good Spec Contains

| Section | Purpose |
|---|---|
| **Status** | `pending`, `in-progress`, or `done` |
| **Goal** | One sentence: what problem this solves |
| **Context** | What already exists, what's missing, relevant prior decisions |
| **Acceptance Criteria** | Testable list of what "done" looks like |
| **Implementation Notes** | Constraints, design hints, open questions to settle first |

## Provider-Specific Adapters

`../agents/` holds thin adapter files — one per AI consumer. Each adapter contains only the provider-specific persona or session-start instructions, and points back here for the actual specs.

To onboard a new AI tool, add `../agents/<provider>.md` that references `specs/`.
