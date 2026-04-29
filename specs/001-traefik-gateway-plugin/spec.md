# Feature Specification: Traefik Gateway Plugin

**Feature Branch**: `001-traefik-gateway-plugin`  
**Created**: 2026-04-29  
**Status**: Draft  
**Input**: User description: "Add a pluggable Traefik gateway plugin to shrine that deploys Traefik as part of the platform network and generates its configuration only when the plugin is active."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Configure Traefik Gateway (Priority: P1)

A shrine operator adds a `plugins.gateway.traefik` section to their shrine config file to enable Traefik as a reverse proxy attached to the platform network. When the section is present and non-empty, Traefik is deployed automatically as part of the normal deploy flow.

**Why this priority**: Core enablement — without config parsing and deployment, nothing else works.

**Independent Test**: Operator adds a minimal `plugins.gateway.traefik` block to the config, runs deploy, and Traefik container is started on the platform network.

**Acceptance Scenarios**:

1. **Given** a shrine config with a populated `plugins.gateway.traefik` section, **When** the operator runs a deploy, **Then** Traefik is deployed as an app on the platform network using the specified (or default) image.
2. **Given** a shrine config with an empty or absent `plugins.gateway.traefik` section, **When** the operator runs a deploy, **Then** Traefik is not deployed and no errors are raised.
3. **Given** `plugins.gateway.traefik.image` is omitted, **When** the plugin is deployed, **Then** the default image `v3.7.0-rc.2` is used.

---

### User Story 2 - Custom Routing Directory (Priority: P2)

A shrine operator specifies a custom `routing-dir` path in the Traefik plugin config so that Traefik loads its static and dynamic configuration from a location other than the default routing directory.

**Why this priority**: Operators with non-standard project layouts need control over where Traefik config files live.

**Independent Test**: Operator sets `routing-dir` to a custom path, runs deploy, and Traefik is mounted with config files from that path.

**Acceptance Scenarios**:

1. **Given** `plugins.gateway.traefik.routing-dir` is set to a custom path, **When** the plugin deploys Traefik, **Then** Traefik uses configuration files from that path.
2. **Given** `plugins.gateway.traefik.routing-dir` is omitted, **When** the plugin deploys Traefik, **Then** Traefik uses configuration files from `{specsDir}/traefik/` (default).

---

### User Story 3 - Traefik Config Generation Gated by Plugin Active State (Priority: P3)

When the Traefik plugin block is absent or empty, no Traefik-specific configuration files are generated, keeping the project clean for teams that do not use the gateway plugin.

**Why this priority**: Avoids polluting projects that opt out of the gateway plugin with unused generated files.

**Independent Test**: Remove the `plugins.gateway.traefik` section, run config generation, and confirm no Traefik config files are produced.

**Acceptance Scenarios**:

1. **Given** the Traefik plugin is active (config block present and non-empty), **When** config generation runs, **Then** shrine derives Traefik routing rules exclusively from apps with both a non-empty `Routing.Domain` and `ExposeToPlatform: true`, and writes them as configuration files to `routing-dir`.
2. **Given** the Traefik plugin is absent or empty, **When** config generation runs, **Then** no Traefik configuration files are created or modified.
3. **Given** the plugin is active and an operator has added custom files to `routing-dir`, **When** config generation runs, **Then** shrine-generated files are written without removing operator-added files.

---

### Edge Cases

- If `routing-dir` does not exist, shrine creates it automatically before writing or mounting it.
- If `dashboard.port` is set without credentials, deploy fails with a validation error before any container starts.
- If `dashboard.port` is absent, the dashboard and API are not exposed regardless of other config.
- How does the system handle an invalid or unreachable Traefik image tag?
- What if the platform network does not exist at deploy time?
- What if `plugins.gateway.traefik` is present but all fields are empty strings?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The shrine config file MUST support a `plugins.gateway.traefik` section with the following fields:
  - `image` (optional, default `v3.7.0-rc.2`)
  - `routing-dir` (optional, default: `{specsDir}/traefik/`) — where Traefik routing config files are generated
  - `port` (optional, default: 80) — the Traefik HTTP routing entrypoint port
  - `dashboard.port` (optional) — when set, enables the Traefik dashboard on the given port
  - `dashboard` credentials (required when `dashboard.port` is set) — basic-auth username and password for dashboard access
- **FR-002**: When `plugins.gateway.traefik` is populated, the system MUST deploy Traefik as an application attached to the platform network during the deploy lifecycle.
- **FR-003**: When `plugins.gateway.traefik` is empty or absent, the system MUST skip all Traefik deployment and config-generation steps without error.
- **FR-004**: The `image` field MUST default to `v3.7.0-rc.2` when not specified.
- **FR-005**: The `routing-dir` field MUST default to `{specsDir}/traefik/` when not specified; shrine creates this subdirectory automatically — it does NOT write Traefik files into `specsDir` root.
- **FR-006**: When the plugin is active, shrine MUST derive Traefik routing rules only from apps that have both a non-empty `Routing.Domain` AND `Networking.ExposeToPlatform: true`, and write those rules as configuration files to `routing-dir`; apps without both conditions are excluded; when the plugin is absent or empty, no generation runs.
- **FR-007**: The plugin implementation MUST be self-contained in a dedicated module or package so it can be moved to an external repository without changes to the shrine core.
- **FR-008**: The `routing-dir` MUST be mounted as a volume into the Traefik container so that both shrine-generated and operator-added files are visible to Traefik at runtime.
- **FR-009**: Shrine-generated config writes MUST NOT delete or overwrite operator-added files in `routing-dir`.
- **FR-010**: If `routing-dir` does not exist when the plugin runs, shrine MUST create it automatically before writing generated files or mounting it.
- **FR-011**: The Traefik routing entrypoint MUST listen on the port specified by `port` (default 80).
- **FR-012**: When `dashboard.port` is set, shrine MUST enable the Traefik dashboard on that port secured with basic auth using the provided credentials.
- **FR-013**: When `dashboard.port` is set without basic-auth credentials, the deploy MUST fail with a clear validation error before any container is started.
- **FR-014**: When `dashboard.port` is absent, the Traefik dashboard and API MUST NOT be exposed.
- **FR-015**: The Traefik container MUST always restart automatically on failure.

### Key Entities

- **Plugin Config**: Represents the `plugins.gateway.traefik` block; attributes: `image` (string, optional), `routing-dir` (path, optional, default `{specsDir}/traefik/`), `port` (integer, optional, default 80), `dashboard.port` (integer, optional — enables dashboard when present), and dashboard credentials `dashboard.username` and `dashboard.password` (username + password, required when `dashboard.port` is set).
- **Gateway Plugin**: The deployable unit — a Traefik container connected to the platform network, driven by the plugin config. Shrine must decide a name that will never be taken by another application.
- **Platform Network**: The shared network that all shrine-managed apps, including the gateway, attach to. Shrine can use the reserved network ip from 0 to 5.
- **App Routing Definition**: The domain and path-prefix pair declared on a shrine application; shrine generates a Traefik routing rule only when the app also has `ExposeToPlatform: true`.
- **Generated Routing File**: A Traefik-format configuration file written by shrine into `routing-dir`, derived from one or more app routing definitions.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Operators can enable the Traefik gateway by adding fewer than 5 lines to the shrine config file.
- **SC-002**: A deploy with the plugin active completes without additional manual steps beyond what a standard shrine deploy requires.
- **SC-003**: A deploy with the plugin absent or empty produces zero Traefik-related artifacts or errors.
- **SC-004**: The plugin module can be extracted to a standalone repository with no changes to shrine core code.
- **SC-005**: Config generation with the plugin active produces valid Traefik configuration files on the first run.
- **SC-006**: If the Traefik container stops unexpectedly, it resumes serving traffic without operator intervention.
- **SC-007**: A deploy with `dashboard.port` set but no credentials provided fails before any container is started, with a message that identifies the missing fields.

## Clarifications

### Session 2026-04-29

- Q: Does shrine generate Traefik config from app routing definitions, or just mount an operator-managed dir, or both? → A: Both — shrine generates Traefik routing rules from each app's `Routing` definition (domain + path prefix), writes them to `routing-dir`, mounts that dir as a volume into Traefik, and the operator can also add or modify files in that dir.
- Q: Which apps get Traefik routing rules generated? → A: Only apps that have both a non-empty `Routing.Domain` AND `Networking.ExposeToPlatform: true`.
- Q: When `routing-dir` does not exist at deploy time, what should shrine do? → A: Create the directory automatically.
- Q: Should the Traefik dashboard/API be exposed, and how is it secured? → A: Two new config fields are added — `port` (routing entrypoint, defaults to 80) and `dashboard.port` (when set, enables the dashboard and makes basic-auth credentials mandatory; deploy fails if `dashboard.port` is set without credentials).
- Q: What restart behavior should the Traefik container have? → A: Always restart automatically on failure.

## Assumptions

- The platform network already exists or is created as part of the standard shrine deploy flow; the plugin attaches to it but does not own it.
- Traefik is deployed as a container using the same deployment mechanism shrine uses for other apps (Docker Compose or equivalent).
- "Plugin active" means the `plugins.gateway.traefik` YAML block is present **and** contains at least one non-empty field.
- Mobile/browser UI for managing the plugin is out of scope; configuration is file-based only.
- The shrine config file is already parsed by existing infrastructure; the plugin section is an additive extension to that schema.
