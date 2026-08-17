# Research: Publish Application Ports on Localhost

**Feature**: 023-publish-host-ports | **Date**: 2026-08-17

No `NEEDS CLARIFICATION` markers existed in the Technical Context; the research
below records the design decisions that resolve every open choice, each grounded
in an existing codebase precedent.

## R1. Manifest field shape

**Decision**: `spec.networking.publish`, accepting two YAML forms via a custom
`yaml.v3` unmarshaler on a `*Publish` type:

```yaml
networking:
  publish: true              # automatic allocation
```
```yaml
networking:
  publish:
    hostPort: 8080           # explicit host port
```

`publish: false` and omission are equivalent (nil → not published). Internally
`Publish{HostPort int}` with `0` meaning automatic.

**Rationale**: One option, two modes (FR-001, SC-004). Placing it inside
`networking` groups it with `exposeToPlatform` — the option whose interplay the
documentation table explains — so the reference reads as one cohesive block. The
`true | {hostPort: N}` polymorphism matches the design note already recorded at
`specs/progress.md:101`.

**Alternatives considered**:
- Top-level `spec.publish` (the literal progress.md sketch) — rejected: splits
  networking semantics across two places in the spec block.
- Two flat fields (`publish: true` + `publishPort: 8080`) — rejected: one concept
  across two keys; reintroduces "do I need both?" ambiguity the feature exists to
  avoid.
- `publish: {hostPort: 0}` / `publish: {}` for automatic (no custom unmarshaler) —
  rejected: the common case reads worst; empty-map YAML is unidiomatic for operators.
- Port `0` sentinel on a plain int field (`publishPort: 0`) — rejected: cannot
  distinguish "automatic" from "not set" without a pointer plus doc folklore.

## R2. Bind address

**Decision**: Fixed `127.0.0.1`, not configurable. New `HostIP` field on
`engine.PortBinding`, passed to `nat.PortBinding.HostIP`.

**Rationale**: The stated goal is `localhost:<port>`; loopback-only is the safe
default and satisfies FR-003 (unreachable from other machines) without firewall
assumptions. Traefik's own bindings keep `HostIP` empty (unchanged 0.0.0.0
behavior).

**Alternatives considered**: `0.0.0.0` (reachable LAN-wide) — rejected as default
for security; a future `publish: {bind: ...}` extension can widen deliberately.

## R3. Automatic allocation range

**Decision**: `30000–32767` (2768 ports), hardcoded constants in the store,
published in the manifest reference documentation.

**Rationale**: Kubernetes NodePort convention — operators recognize it; far from
common service ports and the IANA ephemeral range (49152–65535) that the kernel
hands to outbound connections, reducing accidental collisions with running
processes. Hardcoding matches the constitution's acceptance of single-instance
constants (like `10.200.0.0/24`).

**Alternatives considered**: 49152+ ephemeral range — rejected: the kernel
allocates outbound source ports there, guaranteeing eventual collisions; a
config-file range — rejected (YAGNI, no current need).

## R4. Explicit-port validity range

**Decision**: Explicit `hostPort` must be an integer in `1024–65535` **excluding
30000–32767** (the automatic range). Enforced in `validateApplicationSpec`
(multi-error, parse/validate time).

**Rationale**: Below 1024 collides with system services and conventional gateway
ports. Excluding the automatic block eliminates by construction the only
allocation race in the design: without it, a dynamic allocation made for app A
mid-deploy could take the port app B claims explicitly later in the same deploy —
a conflict the planner cannot see before allocation happens. Disjoint ranges make
planner-time detection complete.

**Alternatives considered**: Allowing explicit ports inside the automatic range
with a set-aware allocator (engine passes the set's explicit ports into the
backend) — rejected: threads manifest-set knowledge into the container backend
and still leaves a cross-deploy ordering hazard; complexity for a range operators
have no reason to want.

## R5. Allocation state — `HostPortStore` mirroring `SubnetStore`

**Decision**: New `state.HostPortStore` interface + `state/local` implementation
persisting to a single global `<stateDir>/hostports.txt` with `team/app=port`
lines (sorted, `#` comments tolerated, atomic temp+rename, `sync.Mutex`).
Explicit ports are recorded too (claimed), not just automatic ones. Reserved
gateway ports (Traefik HTTP, TLS, dashboard from config) seed the taken set at
construction.

**Rationale**: Exact mirror of `internal/state/local/subnets.go` — the proven
allocation-persistence pattern in this codebase (idempotent lookup-or-allocate,
rollback of in-memory maps on failed save, sentinel errors). A single global file
matches the allocation domain: host ports are unique host-wide, not per-team.
Recording explicit claims makes cross-deploy conflict detection work when teams
deploy at different times, and gives `delete`/status one authoritative registry.

**Alternatives considered**:
- Extending `deployments.txt` with a port column — rejected: its parser
  (`SplitN(line, " ", 4)`) would silently fold a fifth field into `ConfigHash`
  for old readers, and the record's lifecycle (deleted on container removal) is
  exactly wrong for a value that must survive container recreation.
- No persistence, re-derive from `docker inspect` — rejected: violates stability
  across teardown (container gone → port forgotten) and makes dry-run preview of
  held ports impossible.

## R6. Where allocation happens

**Decision**: Inside the Docker `ContainerBackend`, in a `resolvePublishBinding`
helper called by `createFreshContainer`/`isContainerUpToDate`: explicit →
`ClaimHostPort` (idempotent upsert; error if held by a different app), automatic
→ `AllocateHostPort` (idempotent lookup-or-allocate). The resolved binding is
appended to the op's port bindings with `HostIP: "127.0.0.1"` before
`buildPortBindings` and the config hash.

**Rationale**: Exact mirror of subnet allocation, which lives in the Docker
backend's `CreateNetwork` (`docker_network.go:21`), not in the engine. Keeps
Principle III intact (no backend-specific logic in `engine.go`) and — critically —
keeps dry-run side-effect free for free: the dry-run container backend is a
printer and never touches the store.

**Alternatives considered**: Allocation in the engine before the backend call —
rejected: the dry-run engine *is* the real engine with print backends, so any
engine-core allocation would execute during dry-run, violating FR-011/SC-005;
allocation in the planner — rejected for the same reason plus planner's
no-side-effect contract.

## R7. Conflict detection

**Decision**: New `planner.DetectHostPortCollisions(set, ports PortContext)
error` in `internal/planner/hostports.go`, where `PortContext{Reserved []int,
Persisted map[string]int}` is assembled by the deploy/dry-run/apply handlers from
config + `HostPortStore.ListHostPorts()`. Detects: (a) duplicate explicit claims
in the set, (b) explicit claim = reserved gateway port, (c) explicit claim =
persisted port of a *different* application (same application adopting its own
persisted port is allowed). All conflicts reported in one deterministic, sorted,
joined error. Called from `planner.Plan` for **all** filters (unlike routing
collisions, which skip single-manifest filters — a port conflict is meaningful
even for a one-app apply because reserved and persisted ports exist outside the
set).

**Rationale**: Direct sibling of `DetectRoutingCollisions`
(`internal/planner/collisions.go`) — same accumulation, sorting, and message
discipline (016 precedent: all collisions in one invocation, deterministic
order). `planner.Plan` is the proven convergence point where dry-run and deploy
share behavior (FR-005).

**Alternatives considered**: Checking in `manifest.Validate` — rejected:
validation is per-manifest and has no view of the set, config, or state; checking
only at backend time — rejected: fails one app mid-deploy instead of failing the
whole plan before any change.

## R8. Exposure semantics (publish × exposeToPlatform)

**Decision**: Derive the op-level platform attachment once, in the engine
projection: `op.ExposeToPlatform = spec.Networking.shouldAttachToPlatform()`
(`ExposeToPlatform || Publish != nil`). The planner's cross-team dependency gates
(`resolve.go:164,194`) and the routing gate (`engine.go:187`) keep reading the
raw `ExposeToPlatform` field.

**Rationale**: Satisfies FR-012/FR-013 exactly: publish alone suffices (implied
attachment, visible in dry-run because the derived value flows through the shared
engine), platform exposure alone publishes nothing, and publishing never widens
cross-team dependency rights or accidentally enables gateway routing. Both-set is
valid and redundant.

**Alternatives considered**: Validation error requiring both options — rejected
(the "silly dependency" the spec forbids); publish implying the full manifest
field (including cross-team access) — rejected: localhost access and cross-team
consumption are different trust decisions.

## R9. Config-hash compatibility

**Decision**: Include the resolved publish binding in the existing `portSpecs`
hash input, and include `HostIP` in a binding's spec string **only when
non-empty** (`127.0.0.1:8080:3000/tcp` vs today's `8080:3000/tcp`).

**Rationale**: Port changes must force container recreation (FR-015) — the hash
already provides that. Making `HostIP` conditional keeps every existing hash
(Traefik's bindings have no HostIP) byte-identical, so upgrading shrine does not
force-recreate running containers (SC-006).

**Alternatives considered**: Unconditionally prefixing HostIP — rejected: rewrites
the Traefik container's hash on upgrade, causing a surprise gateway restart.

## R10. Allocation lifecycle & the release surface

**Decision**: Allocations are released only by: (a) `shrine delete team <name>`
(releases all the team's ports, next to its existing subnet release), and (b) a
new `shrine delete application <name>` subcommand (verb-first, `--team` optional
for disambiguation, `--dry-run` supported) that queries Docker by container name,
**refuses while the container exists** ("tear it down first"), then releases the
port allocation and drops the stale deployment record. Redeploy, container
recreation, teardown, and removing the `publish` option never release.

**Rationale**: FR-008/FR-009 require survival across every container-lifecycle
event; the subnet precedent (release at team deletion, never in
`RemoveContainer`) is the codebase's existing answer. No per-application deletion
command exists today, and US3/AS-4 requires one to be testable — the `delete`
command tree already exists (`delete team`), so the subcommand is a thin,
pattern-following addition. Refusing while the container runs keeps Docker
authoritative (Principle VI).

**Alternatives considered**: Team-deletion-only release — rejected: leaves US3/AS-4
untestable and gives operators no way to free one port; releasing in
`RemoveContainer`/teardown — rejected: breaks stability (the redeploy down/up path
runs exactly that code).

## R11. Constitution & stale-note reconciliation

**Decision**: Amend the constitution's Technical Stack line from "External
access: Traefik only; host-port publishing is unsupported by design" to "External
access: Traefik by default; per-application loopback-only host publishing via
`networking.publish`" — MINOR bump 1.1.1 → 1.2.0 with rationale, propagated per
governance. Update the superseded note at `specs/progress.md:101` in the same
change.

**Rationale**: Governance requires the amendment before behavior and constitution
diverge; the progress.md note itself anticipated this exact feature and names the
mechanism this plan uses.

**Alternatives considered**: None viable — shipping against an explicit
constitutional constraint without amendment contradicts the governance section.

## R12. Dry-run preview of held/pending ports

**Decision**: `handler.DryRun` loads a read-only `HostPortMap` snapshot via
`ListHostPorts()` and passes it to `dryrun.NewDryRunEngine`; the dry-run
container backend prints `publish=127.0.0.1:<port>-><containerPort>/tcp` for
explicit or already-held ports and `publish=127.0.0.1:(auto)-><containerPort>/tcp`
for a first-time automatic request. It also prints the platform-attachment line
whenever the *derived* attachment is true, making the implication visible (US4/AS-5).

**Rationale**: FR-011 requires showing the held allocation without side effects;
a snapshot keeps the print backend storeless and pure. The store is already
available in the dry-run handler (it receives `*state.Store` today).

**Alternatives considered**: Giving the dry-run backend the live store — rejected:
a printer with a persistence handle invites accidental writes and complicates the
SC-005 guarantee; printing "(auto)" always — rejected: FR-011 explicitly requires
showing the already-held port on subsequent dry-runs.

## R13. Test strategy

**Decision**:
- **Integration (TDD gate, written first)**: `tests/integration/publish_test.go`
  with `NewDockerSuite`: explicit publish reachable via HTTP on
  `localhost:<port>`; loopback binding asserted via `docker inspect`
  (`HostConfig.PortBindings` HostIP = `127.0.0.1`, the 011-tlsPort precedent);
  duplicate/reserved/persisted conflicts fail dry-run and deploy with named
  ports; automatic port stable across redeploy, forced recreation, and
  teardown+redeploy; `delete team` / `delete application` release;
  dry-run leaves `hostports.txt` byte-identical.
- **Unit**: manifest unmarshal (bool/map/false/invalid) and validation ranges;
  `DetectHostPortCollisions` cases; `HostPortStore` with injected file-op
  functions (no real filesystem, per project test policy); engine projection
  (derived attachment, op.Publish); Docker backend via the existing
  `fakeDockerAPI` pattern; dry-run print lines.

**Rationale**: Constitution V (integration gate, real binary, real Docker, TDD)
plus the project's standing rules: unit tests never touch the filesystem, and the
integration suite (~10 min) runs as the final gate, not during iteration.

**Alternatives considered**: Asserting loopback-only by connecting from a second
interface — rejected as environment-dependent/flaky; `docker inspect` is
deterministic and proven in the 011 suite.
