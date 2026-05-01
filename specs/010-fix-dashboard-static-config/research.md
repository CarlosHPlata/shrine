# Phase 0: Research & Decisions

**Feature**: Fix Traefik Dashboard Generated in Static Config
**Spec**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md)

This phase resolves the open design questions raised by the Technical Context in plan.md and consolidates them into concrete decisions before any code is written. There were no `NEEDS CLARIFICATION` markers in plan.md — the open spec clarification (FR-010 / Question 1) was already resolved during `/speckit-specify` (option C). The decisions below are the design choices that follow from that resolution and from a read of the existing plugin code in `internal/plugins/gateway/traefik/`.

## Decision 1 — Filename and location of the dashboard dynamic file

**Decision**: Write the dashboard dynamic file at `<routing-dir>/dynamic/__shrine-dashboard.yml`. The double-leading-underscore prefix is the namespacing token; the rest of the name is human-readable.

**Rationale**:
- FR-009 requires the name to be deterministic and namespaced so it cannot collide with per-application routing files. Per-app files are named `<team>-<service>.yml` by `routeFileName` in `config_gen.go:114`. Team names in Shrine manifests follow Kubernetes-style identifiers (lowercase alphanumerics and hyphens), and a leading `_` is not a legal first character for a team name in any existing fixture or validator. A leading `__` is therefore both visually distinctive and structurally impossible to collide with.
- Putting the file inside `<routing-dir>/dynamic/` (the same directory as per-app routing files) means Traefik picks it up automatically: the static config's `providers.file.directory: /etc/traefik/dynamic` already points there, so no change to the static file beyond removing the `http:` block is needed.
- Keeping `dashboard` in the visible portion of the name makes the file's purpose obvious to an operator browsing the directory.

**Alternatives considered**:
- *`<routing-dir>/dashboard.yml` (sibling of `traefik.yml`, outside `dynamic/`)*: rejected — would require either adding a second `providers.file` entry or a `providers.file.filename` mode, both of which complicate the static config and break the "the dynamic dir is the dynamic dir" mental model.
- *`<routing-dir>/dynamic/dashboard.yml` (no namespacing prefix)*: rejected — a Shrine team called `dashboard` with a service called something whose `routeFileName` happens to collapse to `dashboard.yml` (extremely unlikely but not impossible) would collide. FR-009 is explicit about deterministic non-collision.
- *`<routing-dir>/dynamic/_shrine.dashboard.yml`*: rejected — the dot in the basename would visually compete with the `.yml` extension and is harder to grep for.

## Decision 2 — Detecting the legacy `http:` block in a pre-existing `traefik.yml`

**Decision**: After the existing preservation early-return in `generateStaticConfig`, perform a targeted detection pass: read the file, unmarshal into a struct that has only one field (`HTTP *yaml.Node \`yaml:"http"\``), and emit `gateway.config.legacy_http_block` (status `Warning`) if `HTTP != nil`. The file is never modified.

**Rationale**:
- FR-010 requires that the file is left untouched, that the warning is emitted on every deploy where the block is still present, and that detection is limited to the specific dynamic-only sections the buggy version was known to generate (the `http:` top-level section and its descendants — FR-011).
- Using a single-field shadow struct rather than re-using `staticConfig` keeps the detector's blast radius minimal: it does not validate, re-shape, or warn on any other content in the file.
- Using `*yaml.Node` instead of `*httpConfig` means we don't depend on the field set of `httpConfig` matching the buggy file's exact shape — if a slightly different historical Shrine version emitted, say, a `tls:` sibling under `http:`, the detector still fires correctly because we only check whether the `http:` key is present at all.
- Emitting the warning every deploy (as opposed to once) matches FR-010 and keeps the code stateless — no "have I warned the operator yet?" tracking required.

**Alternatives considered**:
- *Regex-scan the file for `^http:`*: rejected — fragile to comments, indentation, and quoting; YAML parse is more robust and the file is small.
- *Parse the full `staticConfig` struct and check `cfg.HTTP != nil`*: works today but couples the detector to the static struct's evolution. Decoupled detection is cleaner.
- *One-shot warn-then-stamp-a-marker-file-to-suppress*: rejected — adds state, and the operator can simply edit the file once and the warning stops naturally.

## Decision 3 — Preservation regime for the dashboard dynamic file (resolves FR-006/FR-007 tension)

**Decision**: Mirror the existing convention in `RoutingBackend.WriteRoute` (`routing.go:37`) and `generateStaticConfig` (`config_gen.go:46`) exactly: if `<routing-dir>/dynamic/__shrine-dashboard.yml` already exists, preserve it unconditionally and emit a `gateway.dashboard.preserved` event. No diff. No partial credential rewrite. **Recommend amending FR-007 in the spec to match this behaviour** — i.e., when the dashboard password changes between deploys, the operator must edit (or delete) the dashboard dynamic file themselves; Shrine emits the preserved-event so the divergence is visible in deploy output.

**Rationale**:
- The spec's FR-006 ("preserve operator edits") and FR-007 ("auto-update credential portion on password rotation") are technically in tension. FR-006 is explicit about the operator-edit-preservation regime that the existing code already implements (specs 004 and 009). FR-007 was an informed default added during specification but it requires read-modify-write logic that distinguishes "credential portion" from "operator edit" — the kind of partial-rewrite logic that the user explicitly rejected for the static `traefik.yml` (Question 1, option C).
- Preserving unconditionally is the strict generalization of "spirit of plugins" (Question 1's chosen rationale): the plugin generates artefacts, never re-edits them.
- The escape hatch for an operator who *wants* Shrine to re-generate the file (e.g., because they rotated the password and didn't want to hand-edit) is already present and consistent: delete the file, run `shrine deploy`, get a freshly-generated file. The edge case "operator manually deletes the dashboard dynamic file" in spec.md already prescribes regeneration in that scenario.

**Spec amendment proposed** (apply during `/speckit-tasks` or on operator approval):
- Reword FR-007: "When the dashboard-related Shrine configuration changes between deploys, the existing dashboard dynamic file is preserved with no modification, and Shrine emits a `gateway.dashboard.preserved` event so the operator can see the file was kept. Operators rotate the dashboard password by deleting the file (and optionally re-running deploy) or by editing the file's credentials directly."

**Alternatives considered**:
- *Implement partial credential rewrite for FR-007*: rejected — adds parsing/diff complexity and contradicts the option-C "spirit of plugins" rationale chosen for the same problem class on the static file.
- *Add a `--force` flag to overwrite operator-edited files*: rejected on YAGNI grounds and on Constitution Principle II (no new flags for existing capabilities).

## Decision 4 — Event names for the new flows

**Decision**: Three new Observer event names, all under the `gateway.` namespace already used by the plugin:
- `gateway.dashboard.generated` (StatusInfo) — emitted on first-time write of the dashboard dynamic file.
- `gateway.dashboard.preserved` (StatusInfo) — emitted when the file already exists and is left untouched.
- `gateway.config.legacy_http_block` (StatusWarning) — emitted when an `http:` block is detected in the pre-existing static file.

Each carries a `path` field; the legacy-block event additionally carries a `hint` field with the human-readable cleanup instruction.

**Rationale**:
- The naming mirrors `gateway.config.generated` / `gateway.config.preserved` and `gateway.route.generated` / `gateway.route.preserved` already in the codebase. Operators and downstream UI consumers will recognize the shape.
- Three events (not one) lets the terminal-logger UI render the right visual treatment: info for the happy paths, warning for the legacy-block path.
- A single-line `hint` field avoids a free-form message in the event and keeps the event payload structured.

**Alternatives considered**:
- *One event with a `kind` field*: rejected — splits a well-formed enum across two fields and is harder to filter on.
- *`gateway.dashboard.warning` instead of `gateway.config.legacy_http_block`*: rejected — the warning is about the static config file, not the dashboard surface; placing it under `gateway.config.` keeps the event hierarchy faithful.

## Decision 5 — Test split: unit vs. integration

**Decision**:
- **Unit tests** (`config_gen_test.go`): cover the new helpers' branch logic by stubbing `lstatFn` (existing pattern). No filesystem writes — explicitly note in a comment that the write paths are exercised by integration. Three new tests: `TestGenerateDashboardDynamicConfig_Skip_WhenPresent`, `TestGenerateDashboardDynamicConfig_StatError`, `TestHasLegacyDashboardHTTPBlock_*` (parse-driven, in-memory bytes).
- **Integration tests** (`tests/integration/traefik_plugin_test.go`): three new scenarios:
  1. `should expose a working dashboard on a clean deploy` — deploys, asserts `<routing-dir>/dynamic/__shrine-dashboard.yml` exists, asserts the static `traefik.yml` has no `http:` block, and (if feasible within the existing harness) issues an HTTP request against the dashboard port and asserts a 401 challenge.
  2. `should preserve operator edits to the dashboard dynamic file` — deploy, hand-edit the generated file, redeploy, assert content unchanged.
  3. `should warn but not modify a pre-existing traefik.yml containing an http block` — pre-stage a static file with a buggy `http:` block, deploy, assert file unchanged, assert the dashboard dynamic file was generated alongside, and (where possible) assert the warning event surfaced via the deploy output.

**Rationale**:
- Aligns with Constitution Principle V (real binary, real Docker) and the project's strict no-filesystem-in-unit-tests memory rule.
- The HTTP probe in scenario 1 is the most direct verification of the headline bug being fixed; if the existing harness does not yet expose an HTTP-probe helper, the assertion can be downgraded to "the dashboard router file is present and well-formed" without weakening the test's ability to catch a regression of this specific bug.

**Alternatives considered**:
- *Add a `httpexpect`-style helper for the dashboard probe*: deferred — out of scope for this fix; if the existing harness has no probe primitive, scenario 1 can land as a file-shape assertion and a follow-up spec can introduce the probe.

## Open follow-ups

None. All design questions implied by the spec are resolved by the five decisions above. The single spec amendment (FR-007 reword) is recommended but not blocking — it can be applied during `/speckit-tasks` review.
