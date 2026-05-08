# Feature Specification: Registry Aliases

**Feature Branch**: `014-registry-alias`
**Created**: 2026-05-08
**Status**: Draft

## Clarifications

### Session 2026-05-08

- Q: When dry-run plan output contains a `reg:<alias>` image, what is shown — the alias or the resolved host? → A: The alias is preserved (`reg:myregistry/image:tag`). Dry-run uses the same planner path; actual host expansion happens only at live execution time (container engine layer).
- Q: Should one registry entry support multiple aliases, or exactly one? → A: Exactly one alias per registry entry (single string field). Operators who need multiple names for the same host duplicate the entry.
- Q: Should dots be permitted in alias names? → A: No. Aliases are restricted to alphanumeric characters, hyphens, and underscores only. Dots are excluded to avoid confusion with DNS hostnames and image path segments.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Define a Registry Alias in Config (Priority: P1)

As a platform operator, I want to assign a short alias to a private registry host in the
global config so that application authors never have to embed raw registry URLs in their
manifests.

**Why this priority**: This is the foundational capability — without aliases in config,
the `reg:` prefix syntax has nothing to resolve against. All other stories depend on it.

**Independent Test**: Configure a `shrine.yml` with one registry entry that includes an
`alias` field and verify that the config loads without error and the alias is accessible
to the planning pipeline.

**Acceptance Scenarios**:

1. **Given** a `shrine.yml` with a registry entry `host: 192.168.1.1:3000, alias: myregistry`, **When** the config is loaded, **Then** the alias `myregistry` is associated with host `192.168.1.1:3000` and available throughout the system.
2. **Given** two registry entries with the same alias value, **When** the config is loaded, **Then** the system reports a validation error indicating the duplicate alias.
3. **Given** a registry entry with no `alias` field, **When** the config is loaded, **Then** the entry behaves exactly as before (host-only, no alias resolution).

---

### User Story 2 — Use `reg:<alias>` Prefix in Application Image (Priority: P1)

As an application author, I want to write `image: reg:myregistry/myimage:latest` in my
application manifest so that I can reference a private registry by a memorable alias
without knowing its IP or port.

**Why this priority**: This is the primary user-facing change; it directly addresses the
privacy/leakage concern and is the core value of the feature.

**Independent Test**: Write an application manifest with `image: reg:myregistry/myimage:latest`,
run a dry-run plan with a config that defines `alias: myregistry`, and verify that no
alias-not-found error is reported and the image appears as `reg:myregistry/myimage:latest`
in the plan output. Then run a live deployment and verify the container engine pulls from
the actual resolved host.

**Acceptance Scenarios**:

1. **Given** an app manifest with `image: reg:myregistry/myimage:latest` and a config alias `myregistry → 192.168.1.1:3000`, **When** a dry-run plan is executed, **Then** the plan output shows `reg:myregistry/myimage:latest` (alias preserved) and no error is reported. **When** live deployment is executed, **Then** the container engine receives `192.168.1.1:3000/myimage:latest`.
2. **Given** an app manifest with `image: reg:unknown/myimage:latest` where `unknown` is not a defined alias, **When** planning is executed, **Then** the system reports an error naming the unresolvable alias.
3. **Given** an app manifest with `image: myimage:latest` (no `reg:` prefix), **When** planning is executed, **Then** the image string is left unchanged — no alias expansion is attempted.

---

### User Story 3 — Use `reg:<alias>` Prefix in Resource Image (Priority: P2)

As an application author, I want to use the same `reg:<alias>` syntax in resource
manifests (e.g., a self-hosted database container) for the same privacy benefit.

**Why this priority**: Resources share the same `image` field; consistency across both
kinds is expected, but it is a lower priority than the app path since resources are less
frequently authored than applications.

**Independent Test**: Write a resource manifest with `image: reg:myregistry/postgres:15`,
run a dry-run plan, and verify that the alias is shown as-is with no error. Then run a
live deployment and verify the container engine receives the actual host.

**Acceptance Scenarios**:

1. **Given** a resource manifest with `image: reg:myregistry/postgres:15` and a defined alias, **When** a dry-run plan is executed, **Then** the plan output shows `reg:myregistry/postgres:15` with no error. **When** live deployment is executed, **Then** the container engine receives `192.168.1.1:3000/postgres:15`.
2. **Given** a resource manifest with `image: reg:missing/postgres:15` where `missing` is not defined, **When** planning is executed, **Then** the system reports a clear error for the resource.

---

### Edge Cases

- What happens when the `reg:` prefix is present but the alias portion is empty (e.g., `image: reg:/myimage:latest`)?  → System reports a validation error: alias name is required.
- What happens when an alias contains characters that are invalid in a DNS label or registry path segment? → System reports a validation error on config load, before any manifest is processed.
- What happens when two registry entries have the same host but different aliases? → Both aliases are valid and independently resolvable; this is a supported multi-alias scenario.
- What happens when a config file defines no registries at all and a manifest uses `reg:`? → System reports an error: no registries configured, alias cannot be resolved.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The `RegistryConfig` type MUST support an optional `alias` field containing a short, human-readable name for the registry host.
- **FR-002**: On config load, the system MUST validate that all alias values across all registry entries are unique (case-sensitive comparison). Duplicate aliases MUST produce a load-time error.
- **FR-003**: An alias MUST consist only of alphanumeric characters, hyphens, and underscores, and MUST NOT be empty when provided. Violations MUST produce a load-time error.
- **FR-004**: During planning and dry-run validation, the system MUST detect any `image` field value starting with the prefix `reg:` and verify that the named alias exists in the loaded config. A missing alias MUST produce an error at this stage, before any execution begins.
- **FR-005**: During live execution (container engine path), the system MUST replace a `reg:<alias>` prefix with the corresponding registry host from the config, producing a fully-qualified image reference passed to the container engine.
- **FR-006**: When a `reg:<alias>` prefix references an alias that does not exist in the config, the system MUST return an error at plan/validation time that clearly identifies the manifest, the image field value, and the unresolvable alias name.
- **FR-007**: Image strings that do not start with `reg:` MUST be passed through unmodified at all stages; alias resolution MUST NOT affect plain image references.
- **FR-008**: The alias expansion MUST occur before the container engine receives the image value. Dry-run output intentionally preserves the `reg:<alias>` form, as it operates through the same planner path without invoking the container engine.

### Key Entities

- **RegistryConfig**: Extended with an optional `alias` string field alongside the existing `host`, `username`, and `password` fields.
- **Config**: Gains a validation method that enforces alias uniqueness and format rules across its `Registries` slice.
- **ImageRef**: The fully-qualified image string passed to the container engine after alias expansion at live execution time. In dry-run and plan output the `reg:<alias>` form is preserved.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can add an `alias` to any registry entry and immediately use `reg:<alias>` in manifests without any other configuration change.
- **SC-002**: A manifest referencing a non-existent alias produces an error message that names the alias, allowing the author to correct it without ambiguity.
- **SC-003**: All existing manifests that use plain image references (no `reg:` prefix) continue to plan and deploy without change — zero regressions.
- **SC-004**: Duplicate or malformed aliases in config are caught at load time, before any manifest is processed, so errors surface immediately on startup.
- **SC-005**: The raw registry host URL is never visible in application or resource manifest files when aliases are used — operators' internal network topology remains private to the config layer.

## Assumptions

- Alias matching is case-sensitive; `myregistry` and `MyRegistry` are treated as distinct aliases.
- An alias is purely a config-layer indirection; it does not affect authentication — existing `username`/`password` fields on the same `RegistryConfig` entry continue to apply to the resolved host.
- The `reg:` prefix is a reserved prefix within Shrine image strings; no existing manifest uses it today, so there are no backwards-compatibility concerns.
- Alias expansion is applied only to the `image` field of Application and Resource manifests; other string fields are not scanned for `reg:` prefixes.
- Out of scope: runtime image-pull-secret injection, alias namespacing per team, or wildcard aliases.
