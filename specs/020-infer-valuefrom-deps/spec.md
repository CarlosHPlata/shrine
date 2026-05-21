# Feature Specification: Infer Implicit Deploy-Order Dependencies from Same-Owner valueFrom References

**Feature Branch**: `020-infer-valuefrom-deps`
**Created**: 2026-05-20
**Status**: Draft
**Input**: User description: "Infer implicit deploy-order dependencies from same-owner valueFrom references. When an Application's env declares `valueFrom: resource.<name>.<output>` or `valueFrom: application.<name>.<output>` and the referenced manifest belongs to the same owner, the planner should treat the reference as a deploy-order dependency. Today only `spec.dependencies` feeds the topological sort, so an app reading from a same-team resource via `valueFrom` can be ordered before that resource, causing deploy failures."

## Clarifications

### Session 2026-05-20

- Q: When a cross-team `valueFrom` reference on a consumer Application has no matching explicit `spec.dependencies` entry, should the planner emit an informational nudge (original US3 behavior) or fail the deploy? → A: Fail. Cross-team `valueFrom` references require an explicit `spec.dependencies` entry; only same-team references are enriched implicitly. When the explicit entry is missing, the planner must fail with a clear error and not proceed to deploy.
- Q: What should the enrichment layer do when a `valueFrom: resource.<X>.<output>` or `valueFrom: application.<X>.<...>` reference's target manifest is not present in the current `ManifestSet` at all (e.g., absent because it lives in another team or because of a typo)? → A: Fail under the same FR-010 rule. Because `shrine deploy team <T>` loads all manifests owned by team `<T>`, a missing target implies the reference is not same-team; it is treated identically to a cross-team reference and requires an explicit `spec.dependencies` entry to proceed.
- Q: When multiple `valueFrom` references would each trigger an FR-010 failure in a single planner run, should the planner collect them all into one error or fail fast on the first one? → A: Fail fast. The enrichment layer raises an FR-010 error on the first offending reference encountered and stops; subsequent references are not enumerated in that run. The operator fixes one issue at a time across successive planner invocations.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Application is sequenced after its same-team resource (Priority: P1)

An operator on team `ops_bot` writes an Application manifest that declares an env var pulling its value from a Resource manifest owned by the same team — for example, `DB_CONNECTION_URL` sourced via `valueFrom: resource.ops-bot-db.DB_CONNECTION_URL`. The operator does not add an explicit `spec.dependencies` block on the Application; they expect the platform to understand that the Application reads from the Resource and to deploy the Resource first.

**Why this priority**: This is the original bug. Without this story, `shrine deploy team ops_bot` can deploy the Application before the Resource and the Application starts against a missing or unready data source. P1 because it is the failure mode the user reproduced and the reason the feature exists.

**Independent Test**: Place a same-owner Application + Resource pair in a fresh specs directory where the Application's env uses `valueFrom: resource.<resource-name>.<output>` and the Application has no `spec.dependencies` block. Run `shrine deploy team <owner>`. The plan emits the Resource step before the Application step. With only this story shipped, the original bug is resolved.

**Acceptance Scenarios**:

1. **Given** a same-owner Application and Resource where the Application's env contains `valueFrom: resource.<X>.<output>` and the Application has no `spec.dependencies`, **When** the operator runs `shrine deploy team <owner>`, **Then** the planned order lists `Resource:<X>` strictly before `Application:<app-name>`.
2. **Given** the same setup but invoked with `shrine deploy --dry-run`, **When** the operator inspects the dry-run output, **Then** the Application step shows its dependency on `Resource:<X>` tagged as inferred from the env var (for example `Resource:<X> (inferred from env DB_CONNECTION_URL)`).
3. **Given** the Application also has an explicit `spec.dependencies` entry naming the same Resource, **When** the planner runs, **Then** no duplicate edge is created, the explicit entry is preserved as written (including its `owner` field), and ordering is identical to the inference-only case.

---

### User Story 2 - Application-to-application inference within a team (Priority: P2)

An operator on team `ops_bot` deploys two Applications where one reads a built-in value from the other through `valueFrom: application.<other-app>.<built-in>` (for example, the other application's hostname). Both Applications are owned by the same team. The operator wants the consumer to be deployed after the producer without having to list it explicitly.

**Why this priority**: Same-team coupling between two Applications is less common than App-to-Resource, but it is part of the supported `valueFrom` grammar and the inference rule generalizes cleanly. Shipping this together with US1 closes the loop on "any same-team `valueFrom` implies ordering." P2 because the bug report focused on the Resource case; this is the symmetry guarantee.

**Independent Test**: Set up two same-owner Applications A and B where A's env contains `valueFrom: application.B.<built-in>` and A has no `spec.dependencies`. Run `shrine deploy team <owner>`. The plan lists `Application:B` strictly before `Application:A`.

**Acceptance Scenarios**:

1. **Given** two same-owner Applications A and B where A's env contains `valueFrom: application.B.<built-in>` and A has no `spec.dependencies`, **When** the operator runs `shrine deploy team <owner>`, **Then** the planned order lists `Application:B` strictly before `Application:A`.
2. **Given** the dry-run output for the same setup, **When** the operator inspects A's dependencies, **Then** `Application:B` appears tagged as inferred from the relevant env var.

---

### User Story 3 - Cross-team valueFrom without explicit dependency fails the deploy (Priority: P3)

An operator on team `team-a` writes an Application that reads from a Resource owned by `team-b` (with access granted via the Resource's `metadata.access` list). The operator expects the platform to refuse to plan the deploy unless they have declared an explicit `spec.dependencies` entry for the cross-team target, so that cross-team coupling is never silently inferred and is always made visible in code review.

**Why this priority**: Cross-team ordering is intentionally an operator decision and must remain visible in code review. Silently inferring a cross-team edge would hide coupling that the other team owns; merely warning about it lets the deploy proceed in an under-specified state. Failing the planner forces the operator to acknowledge the cross-team coupling explicitly. P3 because cross-team `valueFrom` is rare and the primary inference path (US1, US2) handles the common case.

**Independent Test**: Set up a Resource owned by `team-b` with `metadata.access` granting `team-a`, and an Application owned by `team-a` whose env contains `valueFrom: resource.<team-b-resource>.<output>` and no matching explicit `spec.dependencies` entry. Run `shrine deploy team team-a` (or any plan that includes the Application). The planner must fail with an error that names the cross-team reference and recommends declaring an explicit `spec.dependencies` entry. Adding the explicit entry makes the same input plan successfully.

**Acceptance Scenarios**:

1. **Given** a cross-team `valueFrom` reference with access granted and no explicit `spec.dependencies` entry naming the same target, **When** the operator runs the planner, **Then** the planner fails with an error naming the reference (kind, name, output) and recommending an explicit `spec.dependencies` entry. No inferred edge is added and no deploy steps are executed.
2. **Given** the operator adds an explicit `spec.dependencies` entry for the cross-team target, **When** the planner runs, **Then** the explicit edge is honoured exactly as it is today, no error is raised for that reference, and no duplicate edge is created.

---

### Edge Cases

- An env var uses `valueFrom: vault:<project>/<env>/<key>` — the inference layer ignores it because it does not name a manifest in the set.
- An env var uses `value:` only (literal) — no `valueFrom` to interpret, so the inference layer ignores it.
- An env var uses `valueFrom: resource.<X>.<output>` where `<X>` is not present in the current `ManifestSet` (typo, target owned by another team that did not provide the manifest, or otherwise absent). Because `shrine deploy team <T>` loads all manifests owned by team `<T>`, a missing target implies the reference is not same-team. The enrichment layer treats this case identically to a cross-team reference under FR-010: the planner fails unless the consumer's explicit `spec.dependencies` already covers the reference under the `(Kind, Name)` dedup key.
- An operator declares an explicit dependency with a different `owner` value than the referenced manifest's actual owner (typo or stale value). The explicit entry is preserved verbatim; the inferred entry is dropped because the `(Kind, Name)` dedup key matches. The operator's text is never overwritten.
- The same Resource is referenced by multiple env vars on the same Application. Only one inferred edge is added (dedup is per `(Kind, Name)`). The dry-run source tag names one of the originating env vars; multiple sources are coalesced.
- An Application references a Resource the operator did declare explicitly but with extra fields (e.g., a hypothetical future field). Dedup still treats them as the same edge by `(Kind, Name)`. The explicit entry wins; inferred metadata is discarded.
- Two same-owner Applications form a cycle via `valueFrom: application.…` references (A depends on B and B depends on A). The existing topological sort detects the cycle and emits its current cycle error. The inference layer does not need to detect or special-case cycles.
- The `ManifestSet` passed into the inference layer is reused by callers after enrichment runs (e.g., for diagnostics or display of the raw manifests as authored). Those callers must observe the original `Spec.Dependencies` lengths and contents, unchanged by enrichment.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST derive implicit deploy-order dependencies for an Application from its env vars whose `valueFrom` references a manifest in the current `ManifestSet`.
- **FR-002**: The system MUST apply implicit inference only when the referenced manifest's `metadata.owner` equals the consumer Application's `metadata.owner` (the same-owner gate).
- **FR-003**: The system MUST support inference from both `valueFrom: resource.<name>.<output>` (producing a `Resource` dependency edge) and `valueFrom: application.<name>.<output>` (producing an `Application` dependency edge).
- **FR-004**: The system MUST NOT modify any YAML file on disk during inference. Enrichment is performed entirely in memory against the already-parsed manifest objects.
- **FR-005**: The system MUST return a new `ManifestSet` value from enrichment rather than mutating the input. Manifests that gained no inferred edges MAY be shared by reference with the input; manifests that gained inferred edges MUST be returned as new manifest values whose `spec.dependencies` was constructed by copying the original slice and appending the inferred entries.
- **FR-006**: The system MUST dedupe inferred entries against existing explicit `spec.dependencies` entries using the `(Kind, Name)` pair as the key. When an explicit entry already names a target, the inferred entry MUST be dropped and the explicit entry preserved verbatim, including its `owner` field.
- **FR-007**: The system MUST organize inference rules as a chain of independently composable units (one per inference rule), so that a future rule can be added without modifying existing rules and without changes to the topological-sort step.
- **FR-008**: The system MUST place the inference step strictly between manifest validation and the existing ordering step (`Validate → Enrich → Order`). The inference step MUST NOT alter the existing manifest validation rules and MUST NOT change the topological-sort algorithm. Enrichment MAY introduce additional same-team validation checks as defined in FR-010, raised as planner errors that prevent the Order step from running.
- **FR-009**: The dry-run/plan output MUST distinguish inferred dependencies from explicit ones. Each inferred edge MUST be shown with a source tag identifying the originating env var name on the consumer manifest (for example, `Resource:ops-bot-db (inferred from env DB_CONNECTION_URL)`).
- **FR-010**: When a consumer Application's env contains a `valueFrom: resource.<name>.<output>` or `valueFrom: application.<name>.<output>` reference and no manifest with the matching kind and name owned by the consumer's `metadata.owner` is present in the current `ManifestSet`, the enrichment layer MUST fail the planner with an error that names the reference (kind, name, output, and the env var on the consumer) and recommends declaring an explicit `spec.dependencies` entry. This rule MUST be skipped when the consumer's explicit `spec.dependencies` already contains an entry matching the reference under the `(Kind, Name)` dedup key — in that case the explicit entry satisfies the requirement and the planner proceeds. The same failure MUST apply whether the target manifest is absent from the loaded set or is present but owned by a different team. The failure MUST occur before any deploy step executes. The enrichment layer MUST fail fast: on the first offending reference, it stops and returns the error without enumerating subsequent offending references in the same run.
- **FR-011**: The system MUST silently ignore `valueFrom` strings that are not manifest references in the `resource.<name>.<output>` / `application.<name>.<built-in>` grammar — specifically vault references (`valueFrom: vault:…`) and literal `value:` env vars. These categories MUST NOT trigger the FR-010 failure and MUST NOT emit warnings.
- **FR-012**: The system MUST NOT introduce changes to the manifest YAML schema. Authors MAY continue to declare explicit `spec.dependencies` entries; inference is purely additive and optional.
- **FR-013**: Inference MUST be deterministic: the resulting set of dependency edges (post-dedup) MUST be a function of the input `ManifestSet` alone and MUST not depend on map iteration order or processing order across runs. When enrichment fails under FR-010, the offending reference reported by the fail-fast error MUST also be deterministic for a given input — re-running the planner on the same `ManifestSet` MUST report the same first-offending reference.

### Key Entities *(include if feature involves data)*

- **ManifestSet**: The collection of Application, Resource, and Team manifests currently in scope for a single planner invocation. Used as both the input and the output type of the enrichment layer.
- **Application Manifest**: Declares an application's image, runtime env vars (some of which may use `valueFrom`), and an optional explicit `spec.dependencies` list.
- **Resource Manifest**: Declares a backing service (database, queue, etc.) and its named outputs. May be the target of an `valueFrom: resource.<name>.<output>` reference.
- **Dependency Edge**: An ordered pair `(consumer, target)` where the consumer must be deployed after the target. Each edge has a provenance: explicit (operator-declared) or inferred (with a source env var name).
- **Enrichment Rule**: A single inference rule that transforms a `ManifestSet` into a new `ManifestSet` by adding edges according to its specific rule (e.g., `valueFrom: resource.…` or `valueFrom: application.…`). Composes via a chain so additional rules can be added later without touching existing rules.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For any same-owner Application/Resource pair where the Application's env uses `valueFrom: resource.<X>.<output>` and the Application has no explicit `spec.dependencies`, the planned deploy order places the Resource step strictly before the Application step in 100% of runs.
- **SC-002**: For any same-owner Application-to-Application reference via `valueFrom: application.<X>.<built-in>` with no explicit `spec.dependencies`, the planned deploy order places the producer Application strictly before the consumer Application in 100% of runs.
- **SC-003**: For every cross-team `valueFrom` reference encountered during planning without a matching explicit `spec.dependencies` entry, the planner fails with an error that names the reference and recommends declaring an explicit `spec.dependencies` entry; no deploy step executes for that plan. When a matching explicit entry exists, the planner does not fail on that reference and no duplicate edge is added.
- **SC-004**: For any plan run with `--dry-run`, every inferred dependency edge is annotated with the consumer env var that originated it, and explicit edges carry no such annotation. An operator reading the dry-run output can distinguish the two without consulting code.
- **SC-005**: Re-running enrichment on the same `ManifestSet` zero times, one time, or N times produces the same outcome: either the same final dependency edge set (post-dedup) when enrichment succeeds, or the same FR-010 error citing the same first-offending reference when enrichment fails. Enrichment is idempotent and order-independent across rules in both the success and failure paths.
- **SC-006**: After enrichment, no file under the operator's specs directory has been written, modified, renamed, or deleted by the planner; the on-disk state is identical before and after the planning step.
- **SC-007**: A reader of the pre-enrichment `ManifestSet` after the enrichment step has run observes the same `spec.dependencies` length and entries on every manifest as before enrichment (immutability of inputs).
- **SC-008**: The original reproduction case in the bug report — a same-team Application `ops-bot` with `valueFrom: resource.ops-bot-db.DB_CONNECTION_URL` and a Resource `ops-bot-db`, no explicit deps — produces a plan that orders `Resource:ops-bot-db` before `Application:ops-bot` under `shrine deploy team ops_bot`.

## Assumptions

- The current `valueFrom` grammar (`resource.<name>.<output>`, `application.<name>.<built-in>`, `vault:<project>/<env>/<key>`) is stable for the scope of this feature. The inference layer parses only these prefixes; new prefixes would warrant their own decorators in a follow-up.
- Manifest validation runs before enrichment and rejects malformed `valueFrom` strings and references to manifests outside the operator's access scope. The enrichment layer is allowed to treat any reference that passes validation but is not present in the in-memory `ManifestSet` as out-of-scope (silently skipped, except for the same-owner cross-team nudge).
- The `metadata.owner` field is authoritative for the same-owner gate. Operators do not impersonate other owners in their own manifests; that is enforced elsewhere.
- The current planner pipeline (`LoadDir → Validate → Order → topo.Sort`) is the right place to insert enrichment between Validate and Order. Existing callers of the planner do not bypass Validate, so they will all observe the enrichment effect uniformly. Enrichment may now return errors (FR-010); callers must propagate them as planner failures.
- When the planner is invoked via `shrine deploy team <T>`, the loaded `ManifestSet` contains all manifests owned by team `<T>`. The enrichment layer relies on this completeness assumption to treat absence-from-set as equivalent to not-same-team for the FR-010 failure rule.
- Cross-team coupling that today depends on luck or hand-ordering is rare and acceptable to surface via the informational nudge; no migration of existing manifests is required by this feature.
- Inferred dependencies do not need to be persisted to disk or surfaced anywhere outside the planner's in-memory data structures and the dry-run/plan output. There is no need to write them back into the YAML.
- The dry-run output already enumerates per-step dependencies in some form; this feature extends that surface to include provenance tagging. If the current output is silent about dependencies, FR-009 implies adding the minimal dependency listing needed to make provenance visible.
