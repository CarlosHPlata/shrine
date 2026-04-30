# Specification Quality Checklist: Expand `~` Consistently Across Path-Typed Config Fields

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-04-30
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

- The user explicitly directed that relative config paths resolve against the operator's home directory (not CWD, not config-file directory). This is captured in FR-003 and called out in Assumptions so reviewers can confirm this deliberate departure from typical Unix conventions.
- `certsDir` is named in the user prompt but does not exist in code today. Scope is limited to existing path-typed fields (`specsDir`, `teamsDir`, `routing-dir`); the rule is defined generically so a future `certsDir` automatically inherits it (FR-005).
- Items marked incomplete require spec updates before `/speckit-clarify` or `/speckit-plan`.
