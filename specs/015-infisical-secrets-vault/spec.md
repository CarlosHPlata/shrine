# Feature Specification: Infisical Secrets Vault Integration

**Feature Branch**: `015-infisical-secrets-vault`
**Created**: 2026-05-11
**Status**: Draft

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Reference Vault Secrets in Application Manifests (Priority: P1)

As a Shrine operator, I want to reference secrets stored in an external Infisical vault from my application manifests, so that sensitive values (API keys, database passwords, tokens) never need to be written in plaintext in my repo or shrine.yml.

**Why this priority**: Core value delivery — without this, no other part of the integration matters.

**Independent Test**: Configure shrine.yml with a valid Infisical connection, write a manifest with `valueFrom: infisical:myproject/production/DB_PASSWORD`, run `shrine apply`, and verify the container starts with the correct env var injected.

**Acceptance Scenarios**:

1. **Given** an application manifest with `env.DATABASE_PASSWORD.valueFrom: infisical:myproject/production/DB_PASSWORD`, **When** `shrine apply` runs, **Then** the container is started with `DATABASE_PASSWORD` set to the value fetched from Infisical.
2. **Given** the referenced secret does not exist in Infisical, **When** `shrine apply` runs, **Then** the deploy fails with a clear error identifying the missing secret path.
3. **Given** a mix of `value:`, `generated:`, and `valueFrom: infisical:` env entries in the same manifest, **When** `shrine apply` runs, **Then** all three resolution types succeed together.

---

### User Story 2 - Configure Infisical Connection in shrine.yml (Priority: P1)

As a Shrine operator, I want to declare my Infisical server URL and access token in shrine.yml, so that Shrine knows how to reach the vault without any additional setup steps.

**Why this priority**: Required for the integration to be discoverable and operable; no other story works without it.

**Independent Test**: Add a `plugins.secrets.infisical` block with url and token to shrine.yml, then run `shrine dry-run` — it should not error on the Infisical config block.

**Acceptance Scenarios**:

1. **Given** shrine.yml contains a valid `plugins.secrets.infisical` block with `url` and `token`, **When** Shrine loads config, **Then** the Infisical connection parameters are accepted without error.
2. **Given** the `plugins.secrets.infisical` block is absent from shrine.yml, **When** any manifest uses `valueFrom: infisical:...`, **Then** the deploy fails with a clear error indicating the vault is not configured.
3. **Given** shrine.yml contains an `infisical` block with an invalid or unreachable URL, **When** `shrine apply` runs, **Then** Shrine reports a connectivity error before any container changes are made.

---

### User Story 3 - Dry-Run Shows Infisical Secret Placeholders (Priority: P2)

As a Shrine operator, I want `shrine dry-run` to show placeholder values for Infisical-sourced secrets instead of attempting to connect to the vault, so that I can validate my manifest structure without needing a live Infisical instance.

**Why this priority**: Consistent with Shrine's existing dry-run contract; enables offline/CI validation of manifests.

**Independent Test**: With no live Infisical server, run `shrine dry-run` on a manifest with `valueFrom: infisical:project/env/key` — the output should show a placeholder like `[INFISICAL:project/env/key]` and exit without error.

**Acceptance Scenarios**:

1. **Given** a manifest with `valueFrom: infisical:project/env/key`, **When** `shrine dry-run` runs, **Then** the env var is shown with a recognizable placeholder (e.g., `[INFISICAL:project/env/key]`) in the plan output.
2. **Given** Infisical is unreachable, **When** `shrine dry-run` runs, **Then** no network connection to Infisical is attempted and the command succeeds.

---

### Edge Cases

- What happens when the Infisical token has expired or been revoked mid-deploy?
- What happens when the secret path uses a project or environment that does not exist in Infisical?
- What happens when the same env key is set both with `value:` and `valueFrom: infisical:`?
- What happens when `valueFrom: infisical:` is used in a Resource manifest (not just Application)?
- What happens when the Infisical server is reachable but returns a non-200 response?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Operators MUST be able to declare an Infisical vault connection (URL and access token) in shrine.yml under a dedicated secrets plugin config block.
- **FR-002**: Application manifests MUST support referencing Infisical secrets via `valueFrom: infisical:<project>/<environment>/<secret-name>` syntax.
- **FR-003**: Shrine MUST resolve all Infisical-referenced secrets before any container is started or modified, consistent with how other secret types are resolved.
- **FR-004**: If any referenced Infisical secret cannot be fetched (missing, permission denied, vault unreachable), Shrine MUST abort the deploy and report which secret path failed.
- **FR-005**: Dry-run mode MUST NOT contact Infisical; it MUST substitute a human-readable placeholder for each Infisical-sourced value.
- **FR-006**: The `infisical:` prefix MUST be validated at plan time so malformed paths are rejected before execution begins.
- **FR-007**: If no Infisical config is present in shrine.yml and no manifest uses `valueFrom: infisical:`, Shrine behavior MUST be identical to the current baseline (zero regression).
- **FR-008**: Infisical-sourced env vars MUST be composable with existing `value:`, `generated:`, and `template:` env types within the same manifest.

### Key Entities

- **InfisicalConfig**: Vault connection parameters — server URL, access token. Lives in shrine.yml under `plugins.secrets.infisical`.
- **InfisicalSecretRef**: A parsed reference of the form `project/environment/secret-name` extracted from a `valueFrom: infisical:...` value.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can add a new application manifest using Infisical-sourced secrets and successfully deploy it with a single `shrine apply` invocation, with no manual steps beyond editing shrine.yml and the manifest.
- **SC-002**: Secret resolution adds no perceptible delay to deploys when fetching ≤20 secrets from a locally-hosted Infisical instance.
- **SC-003**: Malformed `valueFrom: infisical:` paths are caught and reported at plan time, never reaching container execution.
- **SC-004**: Running `shrine dry-run` on a manifest with Infisical refs produces output without requiring a live Infisical server.
- **SC-005**: All existing deployments that do not use `valueFrom: infisical:` continue to work exactly as before, with no configuration changes required.

## Assumptions

- Infisical is already running and accessible from the Shrine host before `shrine apply` is invoked; Shrine does not manage the Infisical lifecycle.
- The access token in shrine.yml is a long-lived service token or machine identity token with read access to the referenced projects and environments.
- The Infisical project/environment/secret-name path components are treated as opaque strings; Shrine does not validate project or environment existence at config-load time.
- Resource manifests (not just Application manifests) are out of scope for `valueFrom: infisical:` in v1 — only Application env vars are supported initially.
- Only one Infisical instance is configured per shrine.yml (single vault, not multi-vault federation).
- The shrine.yml token is stored in plaintext on the operator's machine; secret encryption at rest for shrine.yml is out of scope.
