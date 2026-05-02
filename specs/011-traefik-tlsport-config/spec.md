# Feature Specification: Traefik Plugin `tlsPort` Config Option

**Feature Branch**: `011-traefik-tlsport-config`
**Created**: 2026-05-01
**Status**: Draft
**Input**: GitHub issue [#12](https://github.com/CarlosHPlata/shrine/issues/12) — "Traefik plugin should support a tlsPort config option to expose port 443"

## Clarifications

### Session 2026-05-01

- Q: Is TLS certificate provisioning (creation, storage, distribution, file-path validation, ACME/Let's Encrypt setup, default-cert behavior) part of Shrine's responsibility for this feature? → A: No. Shrine's responsibility is exactly two things — (1) declare a `websecure` entrypoint at `:443` in the generated Traefik static configuration when that file is Shrine-generated, and (2) publish the host→container port mapping `<tlsPort>:443/tcp` on the Traefik container. Everything else about TLS (certificate sources, trust roots, default-cert behavior, ACME providers, HTTPS-redirect, mTLS, router-level `tls: {}` blocks) is operator-owned via standard Traefik mechanisms (preserved `traefik.yml`, per-app routing files, external cert management). Encoded as FR-010; the User Story 1 independent test and the TLS-cert Assumption have been tightened to reflect this scope.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator publishes HTTPS through the Traefik plugin (Priority: P1)

An operator wants Shrine-routed applications to be reachable over HTTPS. They add a single `tlsPort` field to the Traefik plugin section of `~/.config/shrine/config.yml` (e.g., `tlsPort: 443`). After `shrine deploy`, the Traefik container has a host-to-container mapping for the chosen TLS port to container port 443, and the generated Traefik static configuration declares a `websecure` entrypoint listening on `:443`. Inbound HTTPS traffic to the host on the configured port reaches Traefik and is routed to applications as expected.

**Why this priority**: This is the headline capability requested by the issue. Without it, there is no way to tell Shrine to publish 443 on the Traefik container — operators who manually add a `websecure` entrypoint to `traefik.yml` find that Traefik starts but the host port is never bound, so HTTPS is unreachable end-to-end. P1 because no useful HTTPS publishing path exists today.

**Independent Test**: On a clean host, configure the Traefik plugin with `port: 80` and `tlsPort: 443`, then `shrine deploy`. Inspect the running Traefik container's published ports and confirm `443/tcp` is mapped from the host. Inspect the generated Traefik static configuration file and confirm a `websecure` entrypoint listening on `:443` is present alongside the existing `web` entrypoint on `:80`, and that the entrypoint declaration contains only the address (no Shrine-injected `tls`, `http.tls`, or `certResolver` keys). Open a TCP connection to the host on the configured TLS port; the connection reaches Traefik. (How Traefik then behaves at the TLS layer is determined entirely by the operator's separately-managed Traefik TLS configuration and is out of scope for this feature — see FR-010.)

**Acceptance Scenarios**:

1. **Given** a Shrine configuration with the Traefik plugin and `tlsPort: 443`, **When** `shrine deploy` completes successfully on a clean host, **Then** the Traefik container is created with host port `443` published to container port `443/tcp`.
2. **Given** the same successful deploy, **When** the generated Traefik static configuration file is inspected, **Then** it declares a `websecure` entrypoint listening on `:443` in addition to the existing `web` entrypoint on the configured HTTP port.
3. **Given** a Shrine configuration with `port: 80`, `tlsPort: 8443` (a non-standard host port), **When** `shrine deploy` completes, **Then** the Traefik container publishes host port `8443` to container port `443/tcp` (host-side flexibility) and the static configuration's `websecure` entrypoint still listens on container port `:443`.

---

### User Story 2 - Existing deploys without `tlsPort` keep working unchanged (Priority: P1)

An operator who has not opted into HTTPS leaves `tlsPort` unset (omits the field entirely from `~/.config/shrine/config.yml`). After `shrine deploy`, the Traefik container is created with exactly the same published ports it had before this feature shipped (HTTP entrypoint and dashboard entrypoint only), and the generated Traefik static configuration contains no `websecure` entrypoint. No new errors, warnings, or behavior changes are introduced for the no-`tlsPort` configuration.

**Why this priority**: Backward compatibility with the existing fleet of Shrine deployments is non-negotiable. Adding a new config field MUST NOT change behavior for operators who do not adopt it. P1 because a regression here would break every existing deployment on upgrade.

**Independent Test**: On a clean host, configure the Traefik plugin without a `tlsPort` field, then `shrine deploy`. Inspect the generated static configuration: it MUST be byte-identical (modulo unrelated, already-shipped changes) to the file generated by the previous Shrine release for the same input. Inspect the container's published ports: only the previously-published ports are present; no `443/tcp` mapping exists.

**Acceptance Scenarios**:

1. **Given** a Shrine configuration with the Traefik plugin and no `tlsPort` field, **When** `shrine deploy` completes, **Then** the Traefik container is created with no host-to-container mapping for container port `443/tcp`.
2. **Given** the same configuration, **When** the generated Traefik static configuration is inspected, **Then** it contains no `websecure` entrypoint and no other 443-related artefact.
3. **Given** an operator upgrades Shrine on a host that previously deployed Traefik successfully without `tlsPort`, **When** they re-run `shrine deploy` with their existing (unmodified) configuration, **Then** the deploy succeeds with no new errors and the Traefik container's published port set is unchanged from the pre-upgrade state. *(Note: this release extends Shrine's container-config hash to include port bindings, so every existing container — Traefik included — is recreated exactly once on the first deploy after upgrade. The recreate is transparent: same image, same volumes, same network, same published ports. Subsequent deploys with unchanged config are no-ops.)*

---

### User Story 3 - Operator-edited `traefik.yml` benefits from `tlsPort` for the host binding (Priority: P2)

An operator has manually edited their preserved `traefik.yml` to add a `websecure` entrypoint (per the operator-edit preservation regime established in spec 004). Today, that edit is dead — the Traefik container starts but nothing bound on the host means HTTPS traffic never reaches the entrypoint. With this feature, the operator adds `tlsPort: 443` to the Shrine config and re-runs `shrine deploy`: the Traefik container is recreated with the host port mapping, and the operator's manually-added `websecure` entrypoint now actually receives traffic. Shrine does NOT modify the preserved `traefik.yml`.

**Why this priority**: This is the exact scenario named in the issue's "When an operator manually adds a websecure entrypoint to traefik.yml…" paragraph. P2 because P1 already delivers HTTPS publishing for the clean-deploy path; this story closes the gap for operators who pre-emptively edited their static config.

**Independent Test**: Start from a host where Shrine has previously deployed Traefik (so `traefik.yml` is preserved on subsequent deploys per spec 004). Manually edit `traefik.yml` to add a `websecure` entrypoint at `:443`. Add `tlsPort: 443` to the Shrine config. Run `shrine deploy`. Confirm: (a) the host now publishes `443` to container `443/tcp`; (b) the operator's edits to `traefik.yml` are intact (Shrine did not overwrite the file); (c) HTTPS traffic to the host on `443` reaches the operator-defined entrypoint.

**Acceptance Scenarios**:

1. **Given** a previously-preserved `traefik.yml` containing operator-added `websecure` entrypoint configuration AND a Shrine config newly setting `tlsPort: 443`, **When** `shrine deploy` runs, **Then** the host-to-container port mapping is established for `443/tcp` and the operator-edited `traefik.yml` is left unmodified.
2. **Given** the same scenario, **When** the deploy output is inspected, **Then** Shrine reports the file was preserved (using the same `gateway.config.preserved` signal already used for static-config preservation), and Shrine does not emit a warning about a missing `websecure` entrypoint in the generated config (because Shrine did not generate it).

---

### Edge Cases

- **`tlsPort` collides with `port` or `dashboard.port`**: validation at deploy time MUST fail loudly with a clear message naming the offending fields. Two entrypoints cannot share a host port on the same container.
- **`tlsPort` outside the valid TCP port range** (e.g., `0`, negative, `>65535`): validation at deploy time MUST fail loudly with a clear message; deploy is blocked.
- **`tlsPort` already in use on the host by another process**: the failure surfaces from the container runtime when port binding fails. Shrine MUST surface that error verbatim from the runtime — it is not Shrine's job to pre-flight every host port — but the error message MUST clearly tie the failure to the Traefik container creation step, not bury it.
- **`tlsPort` changed between deploys** (e.g., from `443` to `8443`): `shrine deploy` MUST recreate the Traefik container so the new host port mapping takes effect. This follows from Shrine's existing reconciliation model: container configuration drift triggers recreation.
- **`tlsPort` removed between deploys** (operator deletes the field): `shrine deploy` MUST recreate the Traefik container without the `443/tcp` host mapping AND, when `traefik.yml` is Shrine-generated (not operator-preserved), regenerate it without the `websecure` entrypoint. Operator-preserved `traefik.yml` is left untouched per the existing preservation regime.
- **`tlsPort` set but `traefik.yml` is operator-preserved and missing a `websecure` entrypoint**: the host port mapping is still published (the container publishes the port regardless of static-config content), but inbound traffic on that port has no entrypoint to land on inside Traefik. Shrine MUST emit a deploy-time warning that names this exact mismatch and instructs the operator to add a `websecure` entrypoint to their preserved `traefik.yml` (or delete the file to regenerate). Without the warning, the operator sees a confusing partial-success: port bound on host, but HTTPS requests rejected by Traefik.
- **`port` (HTTP) and `tlsPort` (HTTPS) both set, dashboard port set**: the three host ports are independent and MUST all be published; the generated static config MUST declare three entrypoints (`web`, `websecure`, `traefik`).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The Traefik plugin configuration schema in `~/.config/shrine/config.yml` MUST accept an optional `tlsPort` integer field alongside the existing `port` and `dashboard.port` fields.
- **FR-002**: When `tlsPort` is set, Shrine MUST configure the Traefik container so that host port `<tlsPort>` is published to container port `443/tcp`.
- **FR-003**: When `tlsPort` is set AND the Traefik static configuration file is being generated by Shrine (i.e., not preserved from a prior operator edit), the generated file MUST declare a `websecure` entrypoint listening on `:443` in addition to the existing entrypoints.
- **FR-004**: When `tlsPort` is omitted, Shrine MUST NOT publish a `443/tcp` host mapping on the Traefik container, MUST NOT add a `websecure` entrypoint to the generated static configuration, and MUST exhibit behavior identical to the pre-feature release for the same input.
- **FR-005**: Configuration validation MUST reject a `tlsPort` value that is not a valid TCP port number (outside 1–65535), and MUST reject a `tlsPort` that equals the configured `port` or `dashboard.port`. Validation errors MUST identify the offending field by name.
- **FR-006**: When the operator changes `tlsPort` between deploys (set, change value, or remove), `shrine deploy` MUST detect the drift in container port configuration and recreate the Traefik container so the new host-port mapping takes effect.
- **FR-007**: When the operator-edit preservation regime applies to `traefik.yml` (per spec 004), Shrine MUST NOT modify the preserved file even when `tlsPort` is set or changed. The host port mapping (FR-002) is independent of static-config generation and MUST still be applied.
- **FR-008**: When `tlsPort` is set but the preserved `traefik.yml` does not declare a `websecure` entrypoint at `:443`, Shrine MUST emit a deploy-time warning that names the file path, identifies the missing entrypoint, and instructs the operator to either add the entrypoint to the preserved file or delete the file so Shrine regenerates it. The warning MUST be emitted on every deploy where the mismatch is still present.
- **FR-009**: The new `tlsPort` field MUST be documented in the same configuration reference / quick-start surface that already documents the existing Traefik plugin fields (`port`, `dashboard.port`, `dashboard.username`, `dashboard.password`).
- **FR-010**: Shrine MUST NOT generate, validate, reference, distribute, or otherwise interact with TLS certificates, certificate file paths, certificate resolvers, ACME providers, default-cert configuration, HTTP→HTTPS redirects, or any other TLS-termination concern as part of this feature. The Shrine-generated `websecure` entrypoint MUST contain only its `address` (`:443`); Shrine MUST NOT inject `tls`, `http.tls`, `certResolver`, or any sibling TLS-configuration key into the entrypoint declaration. Operators configure TLS termination on the `websecure` entrypoint exclusively through standard Traefik mechanisms outside this feature's surface (e.g., a preserved `traefik.yml` augmenting the entrypoint, per-app routing files setting `tls: {}` on routers, externally-managed ACME providers).

### Key Entities *(include if feature involves data)*

- **Traefik plugin configuration**: The `plugins.gateway.traefik` block of `~/.config/shrine/config.yml`. Already contains `image`, `port`, and `dashboard.{port,username,password}`. Gains a new optional integer field `tlsPort`.
- **Traefik container port mapping**: The set of host-to-container port publishings on the Shrine-managed Traefik container. Today: `<port>:80`, `<dashboard.port>:<dashboard.port>` (or equivalent). Gains an additional `<tlsPort>:443` mapping when `tlsPort` is set.
- **Traefik static configuration `entryPoints`**: The `entryPoints` map in the generated static configuration file. Today: `web` (port 80) and `traefik` (dashboard port). Gains an additional `websecure` entrypoint at `:443` when `tlsPort` is set AND the file is Shrine-generated.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of clean Shrine deploys configured with the Traefik plugin and `tlsPort` produce a Traefik container that publishes the configured host port to container port `443/tcp` and a generated static configuration that declares a `websecure` entrypoint at `:443`, with no manual operator edits required.
- **SC-002**: 100% of existing Shrine deployments that do NOT set `tlsPort` continue to deploy successfully after upgrading to the release containing this feature, with no change in published host ports and no change in generated static configuration content (modulo unrelated, already-shipped changes).
- **SC-003**: An operator can add HTTPS publishing to an existing Shrine deployment by adding a single `tlsPort` line to their config and running `shrine deploy` once — no other configuration edits, container surgery, or manual Docker commands are required.
- **SC-004**: Configuration with an invalid `tlsPort` (out-of-range, or colliding with `port` / `dashboard.port`) is rejected at validation time with a single clear error naming the offending field; no Traefik container is created or modified in that scenario.
- **SC-005**: Zero new GitHub issues filed against Shrine in the 30 days following the release reporting "configured tlsPort but port 443 not bound on the host" or "manually added websecure entrypoint never receives traffic".

## Assumptions

- The fix targets the Traefik plugin's deploy-time configuration generator and the Traefik container creation step. Runtime Traefik behavior, certificate-resolver configuration, and the broader Shrine plugin contract are out of scope for this spec — operators retain full control of TLS certificate provisioning via Traefik's own configuration (preserved `traefik.yml` or future Shrine-level surface).
- Container port `443/tcp` is the canonical Traefik HTTPS entrypoint port and is hardcoded on the container side; only the host side is operator-configurable via `tlsPort`. This mirrors the existing pattern for the HTTP entrypoint, where container port `80` is fixed and only the host side is operator-configurable via `port`.
- The operator-edit preservation regime established by spec 004 (preserving operator-edited `traefik.yml` across redeploys) is the reference behavior; this feature inherits that regime unchanged.
- Shrine's existing reconciliation model (recreate container when its desired configuration drifts from the running container) covers the "operator changed `tlsPort` between deploys" scenario without new infrastructure.
- TLS certificate management (default-cert, ACME / Let's Encrypt, mTLS), TLS-redirect routing, and any other TLS-termination concern are out of scope per FR-010 — this feature only opens the network path and declares the bare `websecure` entrypoint. Certificates and TLS-termination configuration are entirely operator-owned via standard Traefik mechanisms (preserved `traefik.yml`, per-app routing files, external cert management), exactly as they would be on any non-Shrine Traefik deployment.
- "Dashboard port" and `tlsPort` are independent configuration surfaces: a deploy may set both, either, or neither.
- Honoring FR-006 (drift detection on `tlsPort` change) requires extending Shrine's container-config hash to include port bindings. The extension is engine-wide rather than Traefik-specific, so on the first `shrine deploy` after upgrading to this release every existing container — Traefik plus any application containers — recreates exactly once. The recreate is transparent (same image, same volumes, same network, same published ports) and idempotent across bind mounts and named volumes; subsequent deploys with unchanged config are no-ops. This trade-off is documented in the implementation plan's Complexity Tracking section.
