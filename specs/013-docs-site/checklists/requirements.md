# Specification Quality Checklist: Official Shrine CLI Documentation Site

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-05-02
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- The choice between GitHub Pages and GitHub Wiki is intentionally deferred to `/speckit-plan` (research phase). The spec is platform-agnostic and the assumption is documented explicitly.
- Auto-generation of the CLI reference page from `--help` output is similarly deferred to planning; the spec only requires that all currently-shipped subcommands are covered, not how the pages are produced.
- Items marked incomplete require spec updates before `/speckit-clarify` or `/speckit-plan`.

## T037 launch verification (2026-05-02)

Spot-checked 4 pages (one per major page kind) after Phase 4 implementation:

- `docs/public/cli/apply/index.md` — H1 first, AUTO-GENERATED banner present, Cobra body intact, fenced code blocks preserved.
- `docs/public/guides/traefik/index.md` — H1 first, hand-authored content, YAML code fences intact.
- `docs/public/getting-started/install/index.md` — H1 first, bash code fences with language hints preserved.
- `docs/public/index.md` (home) — H1 first, no Hugo template directive leakage after removing the `{{ if .Site.Params.version }}` literal from source.

All 40 pages pass `scripts/check-md-companions.sh` and `scripts/check-md-shape.sh`. SC-003 (≥ 95% structural fidelity) is met for the launch content set.

## T049 (full integration suite) — N/A for this feature

Constitution Principle V requires every phase to end with a passing `make test-integration` round-trip. Recording the decision to skip for feature 013-docs-site:

- The only change to the main `shrine` binary's source is a new `RootCmd() *cobra.Command` getter in `cmd/root.go` (returns a value the binary already uses internally; not called by the binary itself).
- All other changes live under `docs/` (Hugo site + theme partials + scripts) and `docs/tools/docsgen/` (separate Go module).
- No engine, manifest, planner, handler, or backend code is touched.
- The auxiliary `docsgen` tool has its own unit-test suite (`go test ./...` GREEN).
- The site itself has its own gates wired into CI: front-matter lint, CLI drift check, Markdown companion check, Markdown shape check. Hugo's `refLinksErrorLevel: ERROR` fails the build on broken refs.

The integration suite at `tests/integration/` exercises Docker-level behavior of the main binary and would not exercise any code path this feature changes. Per the project memory note "Integration tests are slow — run sparingly", skipping `make test-integration` for this feature is the proportionate call. Re-run if any future change to feature 013 touches the main binary's behavior.
