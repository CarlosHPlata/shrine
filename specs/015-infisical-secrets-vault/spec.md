# Feature Specification: Secrets Vault Plugin (Infisical)

**Feature Branch**: `015-infisical-secrets-vault`
**Created**: 2026-05-11
**Status**: Draft

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Reference Vault Secrets in Application Manifests (Priority: P1)

As a Shrine operator, I want to reference secrets stored in an external vault from my application manifests using a provider-agnostic syntax, so that sensitive values (API keys, database passwords, tokens) never appear in plaintext and manifests remain portable regardless of which vault backend is configured.

**Why this priority**: Core value delivery — without this, no other part of the integration matters.

**Independent Test**: Configure shrine.yml with a valid Infisical connection under `plugins.secrets.infisical`, write a manifest with `valueFrom: vault:myproject/production/DB_PASSWORD`, run `shrine apply`, and verify the container starts with the correct env var injected.

**Acceptance Scenarios**:

1. **Given** an application manifest with `env.DATABASE_PASSWORD.valueFrom: vault:myproject/production/DB_PASSWORD`, **When** `shrine apply` runs, **Then** the container is started with `DATABASE_PASSWORD` set to the value fetched from the configured vault backend.
2. **Given** the referenced secret does not exist in the vault, **When** `shrine apply` runs, **Then** the deploy fails with a clear error identifying the missing secret path.
3. **Given** a manifest where different env vars each use a distinct resolution type (`value:`, `generated:`, and `valueFrom: vault:` on separate keys), **When** `shrine apply` runs, **Then** all three resolution types succeed together.
4. **Given** a shrine.yml that switches the secrets plugin from one provider to another, **When** `shrine apply` runs, **Then** manifests using `vault:` refs work without modification.

---

### User Story 2 - Configure Vault Plugin in shrine.yml (Priority: P1)

As a Shrine operator, I want to declare which secrets vault plugin to use (Infisical is the only available now) and its connection parameters in shrine.yml, so that Shrine knows how to reach the vault without any changes to application manifests.

**Why this priority**: Required for the integration to be discoverable and operable; no other story works without it.

**Independent Test**: Add a `plugins.secrets.infisical` block with url and token to shrine.yml, then run `shrine dry-run` — it should not error on the secrets plugin config block.
   
**Acceptance Scenarios**:

1. **Given** shrine.yml contains a valid `plugins.secrets.infisical` block with `url`, `client-id`, and `client-secret`, **When** Shrine loads config, **Then** the Infisical connection parameters are accepted and the vault plugin is activated.
2. **Given** no secrets plugin is configured in shrine.yml, **When** any manifest uses `valueFrom: vault:...`, **Then** the deploy fails with a clear error indicating no vault backend is configured.
3. **Given** shrine.yml contains a secrets plugin block with an invalid or unreachable URL, **When** `shrine apply` runs, **Then** Shrine reports a connectivity error before any container changes are made.

---

### User Story 3 - Dry-Run Shows Vault Secret Placeholders (Priority: P2)

As a Shrine operator, I want `shrine dry-run` to show placeholder values for vault-sourced secrets instead of attempting to connect to the vault, so that I can validate my manifest structure without needing a live vault instance.

**Why this priority**: Consistent with Shrine's existing dry-run contract; enables offline/CI validation of manifests.

**Independent Test**: With no live vault server, run `shrine dry-run` on a manifest with `valueFrom: vault:project/env/key` — the output should show a placeholder like `[VAULT:project/env/key]` and exit without error.

**Acceptance Scenarios**:

1. **Given** a manifest with `valueFrom: vault:project/env/key`, **When** `shrine dry-run` runs, **Then** the env var is shown with a recognizable placeholder (e.g., `[VAULT:project/env/key]`) in the plan output.
2. **Given** the vault backend is unreachable, **When** `shrine dry-run` runs, **Then** no network connection to the vault is attempted and the command succeeds.

---

### User Story 4 - Swap Vault Backend Without Changing Manifests (Priority: P3)

As a Shrine operator, I want to be able to change the vault backend (e.g., from Infisical to a future alternative) by only editing shrine.yml, so that I am not locked in to any one provider.

**Why this priority**: Validates the plugin interface design; ensures the architecture is extensible even though only one provider ships initially.

**Independent Test**: Write manifests using `vault:` refs, deploy successfully against Infisical, then reconfigure shrine.yml to use a different secrets plugin — the manifests require zero changes.

**Acceptance Scenarios**:

1. **Given** manifests using `valueFrom: vault:...` deployed against Infisical, **When** the secrets plugin in shrine.yml is changed to another compliant provider, **Then** `shrine apply` resolves secrets from the new provider with no manifest edits.

---

### Edge Cases

- What happens when the vault token has expired or been revoked mid-deploy?
- What happens when the secret path uses a project or environment that does not exist in the vault?
- What happens when the same env key is set both with `value:` and `valueFrom: vault:`?
- What happens when `valueFrom: vault:` is used in a Resource manifest (not just Application)?
- What happens when the vault server is reachable but returns an unexpected error response?
- What happens when two secrets plugins are declared in shrine.yml simultaneously?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Operators MUST be able to declare a secrets vault plugin and its connection parameters in shrine.yml under `plugins.secrets.<provider>` (e.g., `plugins.secrets.infisical`).
- **FR-002**: Application manifests MUST reference vault secrets via the provider-agnostic syntax `valueFrom: vault:<path>`, where `<path>` is an opaque string interpreted by the active vault plugin.
- **FR-003**: The secrets plugin system MUST follow an interface-based design (mirroring the Traefik gateway plugin pattern) so that alternative vault providers can be added in the future by implementing the same interface, without changes to the resolver or manifest format.
- **FR-004**: Shrine MUST resolve all vault-referenced secrets before any container is started or modified, consistent with how other secret types are resolved.
- **FR-005**: If any vault-referenced secret cannot be fetched (missing, permission denied, vault unreachable), Shrine MUST abort the deploy and report which secret path failed.
- **FR-006**: Dry-run mode MUST NOT contact the vault backend; it MUST substitute a human-readable placeholder (e.g., `[VAULT:<path>]`) for each vault-sourced value.
- **FR-007**: The `vault:` prefix and path format MUST be validated at plan time so malformed references are rejected before execution begins.
- **FR-008**: If no secrets plugin is configured in shrine.yml and no manifest uses `valueFrom: vault:`, Shrine behavior MUST be identical to the current baseline (zero regression).
- **FR-009**: Vault-sourced env vars MUST be composable with existing `value:`, `generated:`, and `template:` env types within the same manifest.
- **FR-010**: Only one secrets plugin may be active at a time per shrine.yml; multi-vault federation is out of scope.

### Key Entities

- **SecretsPlugin** (interface): The provider-agnostic contract that any vault backend must implement — at minimum, fetching a secret by path and reporting whether the plugin is active.
- **SecretsPluginConfig**: The shrine.yml block that selects and configures the active secrets plugin. For Infisical this includes `url` (self-hosted instance URL), `client-id`, and `client-secret` (Machine Identity Universal Auth credentials — service tokens are deprecated upstream).
- **VaultSecretRef**: A parsed reference of the form `<path>` extracted from a `valueFrom: vault:<path>` value, passed opaquely to the active plugin. For Infisical the path convention is `<project>/<environment>/<secret-name>`.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can add a new application manifest using `valueFrom: vault:` refs and successfully deploy it with a single `shrine apply` invocation, with no manual steps beyond editing shrine.yml and the manifest.
- **SC-002**: Secret resolution adds no perceptible delay to deploys when fetching ≤20 secrets from a locally-hosted vault instance.
- **SC-003**: Malformed `valueFrom: vault:` paths are caught and reported at plan time, never reaching container execution.
- **SC-004**: Running `shrine dry-run` on a manifest with vault refs produces output without requiring a live vault server.
- **SC-005**: All existing deployments that do not use `valueFrom: vault:` continue to work exactly as before, with no configuration changes required.
- **SC-006**: A second vault provider can be added to the codebase by implementing the SecretsPlugin interface and registering it in shrine.yml, with zero changes to manifest syntax or the resolver.

## Assumptions

- The vault backend (Infisical for v1) is already running and accessible from the Shrine host before `shrine apply` is invoked; Shrine does not manage its lifecycle.
- Authentication to Infisical uses **Machine Identity Universal Auth** (`client-id` + `client-secret`). Service tokens are not used — they are deprecated upstream in favor of Machine Identities.
- The path structure within `vault:<path>` is treated as an opaque string by Shrine's core; interpretation is delegated entirely to the active plugin. For Infisical, the convention is `<project>/<environment>/<secret-name>`.
- Resource manifests are out of scope for `valueFrom: vault:` in v1 — only Application env vars are supported initially.
- The `client-id` and `client-secret` in shrine.yml are stored in plaintext on the operator's machine; secret encryption at rest for shrine.yml is out of scope.
- Infisical is the only secrets plugin shipped in v1; the interface is designed for extensibility but no second implementation is required to ship.

## Testing Setup (Infisical Docker Compose)

For local development and integration testing, Infisical requires three services:

- **infisical** — the main backend, exposed on port 80 (or a chosen host port)
- **postgres** — secrets database (internal only)
- **redis** — caching and job queues (internal only)

Minimum environment variables for the Infisical container:
- `ENCRYPTION_KEY` — generated once via `openssl rand -hex 32`; **must be backed up** — losing it makes all stored secrets unrecoverable
- `DATABASE_URL` — Postgres connection string
- `REDIS_URL` — Redis connection string
- `SITE_URL` — the base URL Infisical is reachable at (e.g., `http://localhost:8080`)

The official Docker Compose file and `.env.example` are maintained in the Infisical GitHub repository and should be used as the starting point for the test environment.
