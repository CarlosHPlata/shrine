# Feature Specification: Official Shrine CLI Documentation Site

**Feature Branch**: `013-docs-site`
**Created**: 2026-05-02
**Status**: Draft
**Input**: User description: "I want to start the official documentation for the Shrine CLI, this requires to first check which options we have, whathever choice we choose, it should be deployed in github as page or wiki, be easy to modify, and have a feature that allow copy content into MD for agents use."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - New operator finds install and first-run docs (Priority: P1)

A developer who has just heard about Shrine CLI lands on the documentation site, follows clearly signposted "Getting Started" content, and has the CLI installed and serving its first app within a few minutes — without needing to read source code or open issues.

**Why this priority**: This is the entry path for every new adopter. Without a credible install + quick-start flow, the project cannot grow, and every other docs feature depends on the site being publicly reachable and navigable. This is the MVP slice.

**Independent Test**: Can be fully validated by giving a fresh user only the docs URL and a bare machine, and observing whether they reach a successful first deploy without external help.

**Acceptance Scenarios**:

1. **Given** a public visitor lands on the documentation home page, **When** they look for "how do I install Shrine CLI?", **Then** they reach an installation page within two clicks and see a copy-pasteable install command.
2. **Given** a visitor has just finished the installation page, **When** they continue to the "Quick Start" page, **Then** they can run a complete end-to-end example (install → configure → first request) using only commands shown on the docs.
3. **Given** a visitor uses the site on a phone-sized viewport (≥360px wide), **When** they open any page, **Then** the navigation and main content remain readable and usable.

---

### User Story 2 - AI agent ingests a doc page as clean Markdown (Priority: P2)

A developer working alongside an AI coding assistant (or an agent fetching context on its own) needs to feed Shrine documentation into the assistant's prompt. From any documentation page, they trigger a one-click "copy as Markdown" action and paste a self-contained, well-formed Markdown blob into their agent — preserving headings, prose, code samples, command examples, and inline tables, with no site chrome.

**Why this priority**: This is the differentiating feature the user explicitly asked for and aligns docs with how Shrine's audience increasingly works (AI-assisted operations and code). It builds on top of the basic docs site (P1) and is independently testable once any single page exists.

**Independent Test**: Open any documentation page in a browser, trigger the copy-Markdown action, paste into a plain-text editor, and verify the output is valid Markdown that round-trips through a Markdown renderer to a near-identical visual result.

**Acceptance Scenarios**:

1. **Given** a visitor is on a documentation page that contains headings, paragraphs, fenced code blocks, and a table, **When** they trigger the "copy as Markdown" action, **Then** their clipboard receives a single Markdown document containing all of that content with structure preserved and without HTML tags or site navigation.
2. **Given** the copied Markdown is pasted into an AI assistant, **When** the assistant renders the message, **Then** code blocks remain code blocks (with language hints), tables render as tables, and headings remain in the same hierarchy as the source page.
3. **Given** a visitor uses the action a second time on a different page, **When** they paste both copies together, **Then** the two Markdown documents are independently valid and clearly delimited (e.g., each begins with its own H1 page title).

---

### User Story 3 - Maintainer fixes or extends docs in minutes (Priority: P3)

A maintainer notices a typo, an outdated CLI flag, or a missing section. They edit the source content using the workflow the project uses for everything else (a Pull Request, or a wiki edit if that path is chosen during planning). After the change is approved/merged, the site updates automatically, and the new content appears live for visitors and AI agents.

**Why this priority**: Docs that are painful to update become wrong docs. Low edit friction protects the value created by P1 and P2 over time. This story stands alone: maintainers can validate it the moment a single page exists, independently of any new content.

**Independent Test**: Make a one-character edit to a documentation source file (or wiki page) on a feature branch, follow the documented contribution path, and verify the change reaches the live site without manual deployment steps.

**Acceptance Scenarios**:

1. **Given** a maintainer edits a Markdown documentation source and merges the change, **When** they refresh the public docs URL, **Then** the change is visible within a few minutes with no further action.
2. **Given** a visitor reading any documentation page, **When** they want to suggest an edit, **Then** the page links to the underlying source so they can propose a change directly.
3. **Given** a contributor opens a documentation Pull Request, **When** the change is reviewed, **Then** the diff is a clean Markdown diff (no generated HTML or build artifacts) so review is straightforward.

---

### Edge Cases

- **Page with no body content** (only a title): the copy-Markdown action MUST still produce a valid Markdown document containing at least the page title as an H1.
- **Very long pages**: the copy-Markdown action MUST copy the entire page content; any size limit imposed by the browser clipboard MUST be reported to the user (no silent truncation).
- **Mismatch between docs and shipped CLI**: any page that documents a CLI command MUST display the version of Shrine the docs were built from, so a visitor on an older or newer CLI can spot a mismatch.
- **Broken internal link**: a build that references a missing page MUST fail visibly (or at minimum surface a warning) rather than silently shipping a 404.
- **Search returns no result**: the search experience MUST clearly say "no results" and offer the navigation as a fallback.
- **Visitor with JavaScript disabled**: core reading and navigation MUST still work; only the copy-Markdown action and search may degrade.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST publish the Shrine CLI documentation as a public website hosted on GitHub (either GitHub Pages or GitHub Wiki — the specific platform is selected during planning).
- **FR-002**: System MUST present a Home page that introduces Shrine, states what it does, and surfaces direct entry points to "Install", "Quick Start", "CLI Reference", and "Troubleshooting".
- **FR-003**: System MUST provide a persistent navigation that lets a visitor reach any documentation page within at most three clicks from the Home page.
- **FR-004**: System MUST cover the following content areas at launch: introduction/overview, installation, quick-start, CLI command reference, plugin/integration guides (Traefik gateway), routing and aliases configuration, TLS / HTTPS configuration, and troubleshooting.
- **FR-005**: Every documentation page MUST expose a one-click "copy as Markdown" action that places the page's complete content on the visitor's clipboard as a self-contained Markdown document.
- **FR-006**: The Markdown produced by the copy action MUST preserve page title (as H1), heading hierarchy, paragraph text, ordered/unordered lists, fenced code blocks with their language hints, inline code, and tables — and MUST exclude site chrome (header navigation, sidebar, footer, breadcrumbs, edit-this-page links).
- **FR-007**: Every documentation page MUST display a link to the underlying source location (file in the repository, or wiki page) so contributors can edit it directly.
- **FR-008**: System MUST provide a site-wide search that indexes the body and headings of every documentation page and is reachable from any page.
- **FR-009**: System MUST display the Shrine CLI version that the documentation was built against on the Home page (and ideally in the page footer of every page) so visitors can confirm relevance.
- **FR-010**: System MUST be readable and navigable on viewports from 360px wide (mobile) up to standard desktop widths without horizontal scroll on body content.
- **FR-011**: System MUST publish documentation source as Markdown files versioned in this repository (or as wiki pages if that path is chosen), so changes flow through the same review process the project already uses.
- **FR-012**: System MUST publish documentation updates automatically once they are merged/accepted on the canonical branch — no manual deploy step.
- **FR-013**: System MUST surface a way for visitors to report a documentation issue (e.g., a link to the project's issue tracker pre-filled with the page path).
- **FR-014**: System MUST handle missing pages with a clear 404 page that links back to the Home page and the search.

### Key Entities *(include if feature involves data)*

- **Documentation Page**: A single readable unit on the docs site. Attributes: title, body content (Markdown), category/section, slug/URL, source location pointer (file path or wiki page name), build-time version stamp.
- **Navigation Tree**: An ordered, grouped list of Documentation Pages organized by category (Getting Started, CLI Reference, Guides, Reference, Troubleshooting, etc.).
- **Copy-as-Markdown Output**: The Markdown document produced by the per-page copy action. Attributes: page title (as H1), structured body, no site chrome, no embedded HTML beyond what valid Markdown allows.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A first-time visitor with no prior context can locate the install command and run it successfully within 5 minutes of arriving at the docs Home page.
- **SC-002**: 100% of currently-shipped Shrine CLI subcommands are covered by at least one documentation page at launch.
- **SC-003**: The Markdown produced by the copy-as-Markdown action, when re-rendered by a standards-compliant Markdown renderer, is visually equivalent to the source page's body for at least 95% of pages (no missing sections, no broken structure) — verified by spot-checking every page at launch.
- **SC-004**: A documentation change goes from PR merge (or accepted wiki edit) to live on the public site within 10 minutes, with no manual steps from a maintainer.
- **SC-005**: The Home page first-meaningful-paint completes in under 2 seconds on a typical broadband connection.
- **SC-006**: A maintainer can add a new documentation page (file or wiki page), see it picked up by navigation and search, and link to it from existing pages, in under 15 minutes of effort end-to-end.
- **SC-007**: Every documentation page links back to its source location, verified at build time (no missing source links at launch).

## Assumptions

- **Audience**: Primary readers are technical operators of the Shrine CLI and AI coding assistants/agents that consume context on their behalf. No authenticated/private docs are needed at launch.
- **Hosting platform decision deferred to planning**: Both GitHub Pages and GitHub Wiki satisfy "deployed on GitHub" and "easy to modify". Choosing between them — including how each impacts the copy-as-Markdown action, search, theming, and build pipeline — is an explicit task for `/speckit-plan` (research phase).
- **Source format**: Documentation content is authored in Markdown so it travels with code review and AI agents can consume the source directly when needed.
- **Versioning**: A single "latest" line of documentation that mirrors the canonical branch is sufficient at launch. Per-release archived versions are out of scope for v1.
- **Localization**: English only at launch.
- **Comments / discussion**: Out of scope for v1; visitors who want to engage are directed to the project's existing issue tracker.
- **Analytics**: Not required at launch; can be added later without affecting this spec.
- **Automatic doc-from-code generation** (e.g., generating CLI reference from `--help` output) is desirable but treated as an implementation choice for `/speckit-plan`, not a spec-level requirement — what matters is that the resulting pages exist and stay current.
- **Contribution access**: Anyone with write access to this repository (or the project's wiki, depending on platform choice) can update docs through the project's existing review workflow; no separate docs-only access control is introduced.
- **NO tests**: Since this feature touches only documentation, the rule about TDD and integration tests is not applicable here.