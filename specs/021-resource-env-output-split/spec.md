# Feature Specification: Split Resource `env` and `output` (SRP)

**Feature Branch**: `021-resource-env-output-split`
**Created**: 2026-05-26
**Status**: Draft
**Input**: User description: "Right now resources have an Output spec that is breaking the SRP principle, first it was planned to be used as to declare the output values so others can consume, but, it became nowadays the same place to declare env variables, we have to change it, so resources declare env part like application specs, and additionally the output. How it will work values on env are infered for the secrets, and output it's only the lists of env and host and port that can be exported, they should not be able to declare any value there, no value, valuefrom, the only valid one is the template"

## Overview

A Resource's `output` block currently serves two unrelated jobs: it declares the
values the resource's own container runs with (its environment), **and** it
declares the public interface other manifests consume. These two concerns have
collapsed into one block, so an operator cannot configure a resource without
also exposing that configuration, and cannot expose a curated interface without
mixing it into runtime config. This feature separates the two responsibilities:

- Resources gain an **`env`** block — modelled on the Application `env` block —
  for everything the resource container runs with (static values, secrets from
  the vault, auto-generated secrets, and templated values).
- **`output`** is reduced to a pure **export list**: the set of env var names,
  plus the built-ins `host` and `port`, that the resource publishes to consumers,
  with an optional `template` for derived/computed exports. An output entry may
  **not** declare a `value`, `valueFrom`, or `generated` — only a `name` and an
  optional `template`.

## Clarifications

### Session 2026-05-26

- Q: Is resource `env` `valueFrom` limited to vault refs only, or may it also reference other manifests' exported outputs (full symmetry with Application env)? → A: Full symmetry — resource `env` `valueFrom` may reference other manifests' exported outputs, making resources first-class consumers subject to the same access/reachability/ordering/inference rules as Application consumers.
- Q: May an exported `template` output reference a private (un-exported) env var? → A: Yes — an `output` template may reference any of the resource's own declared env var names (exported or not) plus the built-ins; values from other specs reach the template through the resource's `env` `valueFrom`. "Private" blocks only *direct* reads of a key; an operator may deliberately fold a private value into a template they author.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Configure a resource and curate what it exposes (Priority: P1)

A platform operator authoring a database resource wants to give the container
its runtime configuration (database name, an auto-generated password, a
vault-sourced API key) in one place, and separately decide exactly which of
those — plus the connection host and a derived connection string — other teams'
applications may read.

**Why this priority**: This is the core capability the feature exists to
deliver. Without it, configuration and the export contract remain conflated and
the SRP violation persists.

**Independent Test**: Author a resource with an `env` block (mix of `value`,
`generated`, `valueFrom: vault:...`, and `template`) and an `output` block that
lists a subset of those env names plus `host` and a `template` connection
string. Deploy (real and `--dry-run`) and confirm the container receives the
full resolved `env`, while only the listed keys are published as the resource's
exported interface.

**Acceptance Scenarios**:

1. **Given** a resource whose `env` declares `POSTGRES_DB` (value), `POSTGRES_PASSWORD` (generated), and `API_KEY` (`valueFrom: vault:...`), **When** the resource is deployed, **Then** the container's environment contains the resolved `POSTGRES_DB`, `POSTGRES_PASSWORD`, and `API_KEY`.
2. **Given** the same resource whose `output` lists `POSTGRES_DB`, `host`, and a `DB_URL` template referencing `host`, `port`, and `POSTGRES_DB`, **When** the plan is resolved, **Then** the resource's exported interface contains exactly `POSTGRES_DB`, `host`, and `DB_URL`.
3. **Given** an `output` entry that is just `name: host`, **When** the resource is resolved, **Then** `host` is published using its CLI built-in value; **And** when `host` is omitted from `output`, **Then** it is not published.

---

### User Story 2 - Keep configuration private unless explicitly exported (Priority: P2)

A consuming application references a resource value via
`valueFrom: resource.<name>.<key>`. The operator wants confidence that secrets
and internal config that were placed in `env` but deliberately left out of
`output` cannot be read by other manifests.

**Why this priority**: This encapsulation guarantee is the payoff of the split —
it is what makes `output` a meaningful contract rather than an incidental side
effect of configuration.

**Independent Test**: Reference an exported key and an un-exported env var from a
consumer application; confirm the exported reference resolves and the un-exported
reference is rejected with a clear error.

**Acceptance Scenarios**:

1. **Given** a resource whose `env` declares `POSTGRES_PASSWORD` (generated) that is **not** listed in `output`, **When** an application uses `valueFrom: resource.<name>.POSTGRES_PASSWORD`, **Then** validation fails with an error stating the key is not exported.
2. **Given** the same resource exports `POSTGRES_DB`, **When** an application uses `valueFrom: resource.<name>.POSTGRES_DB`, **Then** the reference resolves to the resolved value of that env var.
3. **Given** an auto-generated secret used internally by the resource and never exported, **When** any consumer attempts to read it, **Then** the value is never exposed through the resource's interface.

---

### User Story 3 - Migrate an old manifest with a clear error (Priority: P3)

An operator with a pre-existing resource manifest that declares
`value`/`valueFrom`/`generated` directly on `outputs` runs a deploy after
upgrading. They need an unambiguous, actionable error telling them what changed
and how to fix it, rather than silent or confusing behavior.

**Why this priority**: Safe rollout. The feature is a breaking schema change;
the migration experience determines whether operators can adopt it without
guesswork.

**Independent Test**: Run validation against a manifest that still declares
`generated: true` on an output and confirm the error names the offending output
and directs the operator to move it under `env`.

**Acceptance Scenarios**:

1. **Given** a resource whose `output` entry declares `generated: true`, **When** the manifest is validated, **Then** validation fails with an error that names the resource and output and instructs the operator to declare the value under `env` and list the name under `output` to export it.
2. **Given** a resource whose `output` entry declares `value:` or `valueFrom:`, **When** the manifest is validated, **Then** validation fails with the same class of actionable error.
3. **Given** a resource whose `output` entries declare only `name` and optional `template`, **When** the manifest is validated, **Then** validation passes.

---

### Edge Cases

- **Output name with no matching env var or built-in**: An `output` entry that is just `name: FOO` where `FOO` is neither a declared env var nor `host`/`port` → validation fails.
- **Output template referencing an undeclared name**: A `template` output referencing `{{.MISSING}}` where `MISSING` is not a declared env var or a built-in (`host`/`port`/`team`/`name`) → validation fails.
- **Duplicate output names** → validation fails.
- **Env entry declaring none of value/valueFrom/template/generated** → validation fails (an env entry must resolve to exactly one source).
- **Env entry declaring more than one of value/valueFrom/template/generated** → validation fails (mutually exclusive).
- **Exported `port` without `spec.port`**: Listing `port` in `output` while `spec.port` is unset → resolution fails, as it does today for the bare `port` built-in.
- **Re-exporting a generated secret**: An operator may list a `generated` env var's name in `output` to expose it deliberately — this is allowed; privacy is opt-out by omission, not enforced on secrets.
- **Consumer references an env var that exists but is not exported** → rejected (strict allowlist).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A Resource spec MUST support an `env` block whose entries have the same shape as Application `env` entries — a `name` plus exactly one of `value`, `valueFrom`, or `template`.
- **FR-002**: Resource `env` entries MUST additionally support `generated: true` to auto-mint a secret value, preserving today's generated-output capability. `generated` is mutually exclusive with `value`, `valueFrom`, and `template`.
- **FR-003**: Resource `env` values MUST be resolved through the same mechanisms available to Application `env` — static `value`, vault `valueFrom`, sibling-referencing `template`, and (additionally) generated secrets — including the inference of secret values from the active secrets vault.
- **FR-004**: The resource container's runtime environment MUST be populated from the resolved `env` block, NOT from `output`.
- **FR-005**: A Resource `output` MUST be a list whose entries declare only a `name` and an optional `template`. Declaring `value`, `valueFrom`, or `generated` on an output entry MUST fail validation.
- **FR-006**: An `output` entry without a `template` MUST export the value of the resource env var whose name equals `name`, or the CLI built-in `host`/`port` when `name` is `host`/`port`. If `name` matches neither a declared env var nor a built-in, validation MUST fail.
- **FR-007**: An `output` entry with a `template` MUST export the rendered template. The template MAY reference any of the resource's declared env var names — including env vars that are NOT themselves exported — and the built-ins `host`, `port`, `team`, and `name`; referencing any other name MUST fail validation. Cross-spec values are reachable only when first pulled into the resource's `env` via `valueFrom` (FR-014); there is no direct cross-manifest reference syntax inside a template.
- **FR-008**: The built-ins `host` and `port` MUST be exported only when explicitly listed in `output`.
- **FR-009**: A manifest MUST be able to consume, via `valueFrom: resource.<name>.<key>`, only keys present in the referenced resource's `output`. Referencing a resource env var that exists but is not listed in `output` MUST fail (strict allowlist). This rule applies uniformly to any consumer (Application or Resource).
- **FR-014**: A Resource MUST be a first-class consumer: its `env` `valueFrom` MAY reference other manifests' exported outputs (`resource.<name>.<key>` / `application.<name>.<built-in>`) in addition to vault refs, and such references MUST be subject to the same rules that already apply to Application consumers — access/reachability checks, deploy-order dependency resolution, and same-team `valueFrom` deploy-order inference.
- **FR-010**: Output names MUST be unique within a Resource.
- **FR-011**: A manifest that declares `value`, `valueFrom`, or `generated` on any `output` entry MUST be rejected with an error that names the offending resource and output and directs the operator to declare the value under `env` and list its name under `output` to export it.
- **FR-012**: The deploy-order dependency inference (derived from same-team `valueFrom` references) MUST continue to function, keyed off the referenced **exported** output keys.
- **FR-013**: Dry-run output MUST reflect the split — the resource's resolved `env` as the container environment and the resource's `output` as the published interface — without performing real secret generation or vault reads.

### Key Entities *(include if feature involves data)*

- **Resource env entry**: A single runtime configuration variable for the resource container. Attributes: `name`, and exactly one source among `value`, `valueFrom`, `template`, or `generated`.
- **Resource output entry**: A single published item in the resource's export contract. Attributes: `name`, and an optional `template`. No value-bearing fields.
- **Exported interface**: The set of keys (env names, built-ins `host`/`port`, and template-derived names) a resource publishes; the only keys consumers may reference.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A resource can declare its runtime configuration (`env`) and its export contract (`output`) as two separate blocks, with no requirement that an env var be exported or that an export correspond one-to-one to runtime config.
- **SC-002**: 100% of resource env vars not listed in `output` are unreadable *directly* by other manifests — a consumer reference to an un-exported key is rejected. (An operator may still deliberately surface a value by folding it into a `template` output they author.)
- **SC-003**: An auto-generated secret used by a resource's own container does not appear in its exported interface by default — it is published only if the operator explicitly lists its name or references it from a `template` output. Verified end-to-end.
- **SC-004**: Any manifest using the old output-with-value shape is rejected in a single validation pass with an actionable error that names the field and states the fix.
- **SC-005**: A resource that previously exported only `host`/`port`/templated values can be migrated by moving runtime config into `env`, with no change to the resolved container environment or to the values seen by existing consumers.

## Assumptions

- **Breaking change, no auto-migration**: Existing manifests that declare `value`/`valueFrom`/`generated` on outputs are rejected with a clear error; operators update them. This is consistent with the project's pre-1.0 schema evolution.
- **Env mirrors Application env semantics**: Resource `env` accepts the same `valueFrom` forms as Application `env` (vault references and references to other manifests' exported outputs), plus `generated`. Resources are first-class consumers — cross-manifest `valueFrom` from a resource is in scope and subject to the same access/reachability/ordering/inference rules as Application consumers (FR-014). Exported-output allowlist rules apply uniformly to any consumer.
- **Built-ins unchanged**: `host` and `port` remain CLI-derived built-ins; `port` still requires `spec.port` to be set when exported.
- **Template engine unchanged**: The templating syntax and rendering behavior are the same as today's output templates; only the set of referenceable names changes to "declared env vars + built-ins".
- **No new CLI surface**: No new commands or flags; behavior is layered behind existing `shrine deploy` / `shrine deploy --dry-run` and manifest validation.
- **Out of scope**: Changes to the vault plugin or secret store interfaces; changes to the Application spec (which already has `env`).
