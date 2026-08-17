# Feature Specification: Publish Application Ports on Localhost

**Feature Branch**: `023-publish-host-ports`
**Created**: 2026-08-17
**Status**: Draft
**Input**: User description: "Expose application ports to the localhost network via a manifest option. The port is either allocated dynamically or set by the user; user-set ports must fail on conflicts with other user-set ports at dry run and deploy. A dynamically allocated port must stay the same across redeploys of the application, including when its container is taken down and brought back up. Publishing must not require also enabling platform exposure, platform exposure alone must not publish a port, and a published application must always be attached to the platform network. The goal is being able to hit localhost:port to access an application."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Reach an application on a chosen localhost port (Priority: P1)

An operator declares in an application's manifest that its service port should be published on the host at a specific port number. After deploying, the operator opens `localhost:<port>` (browser, curl, local tooling) and reaches the application directly, without going through the platform gateway or DNS.

**Why this priority**: This is the core value of the feature — direct host access to a deployed application. An explicit port is the simplest complete slice: no allocation machinery is needed, and it is immediately useful for local development and debugging.

**Independent Test**: Deploy a single application whose manifest requests publishing on an explicit port; verify the application answers on `localhost:<port>` from the host, and that a second machine on the network cannot reach that port.

**Acceptance Scenarios**:

1. **Given** an application manifest that requests publishing on explicit port 8080, **When** the operator deploys, **Then** the application is reachable at `localhost:8080` from the host.
2. **Given** a deployed application published on port 8080, **When** the operator changes the manifest to port 9090 and redeploys, **Then** the application stops answering on 8080 and answers on 9090.
3. **Given** an application manifest with no publish option, **When** the operator deploys, **Then** no host port is exposed for that application (behavior identical to today).
4. **Given** an application manifest that requests publishing, **When** the operator runs a dry run, **Then** the plan output states the host port mapping that would be created, and no changes are made.
5. **Given** a published application, **When** a client on another machine attempts to connect to the host's IP on that port, **Then** the connection is refused — the port answers only on the host's own loopback interface.

---

### User Story 2 - Conflicting user-set ports fail fast at dry run and deploy (Priority: P2)

An operator (or two teams sharing a host) declares explicit host ports in more than one application manifest. When two declarations claim the same port — or a declaration claims a port the platform itself reserves, or a port already held by another application's automatic allocation — both the dry run and the real deploy stop before making any changes, naming every conflict found.

**Why this priority**: Without conflict detection, a duplicated port either fails halfway through a deploy with a low-level error or silently steals a port another application relied on. Fail-fast with a complete report is what makes explicit ports safe to use; it completes the P1 story.

**Independent Test**: Author two application manifests claiming the same explicit port; verify dry run and deploy both abort before any change, with an error naming the port and both applications.

**Acceptance Scenarios**:

1. **Given** two application manifests that both claim explicit port 8080, **When** the operator runs a dry run, **Then** the run fails with an error naming port 8080 and both applications, and nothing is deployed.
2. **Given** the same two manifests, **When** the operator deploys, **Then** the deploy aborts with the same error before any application is created, updated, or removed.
3. **Given** a manifest set containing several distinct port conflicts, **When** the operator runs a dry run once, **Then** every conflict is reported in that single invocation, in a stable, deterministic order.
4. **Given** an application manifest claiming an explicit port that the platform gateway already uses for its own endpoints, **When** the operator runs a dry run or deploys, **Then** the run fails naming the port as platform-reserved.
5. **Given** application A holds an automatically allocated port from an earlier deploy, **When** a different application B claims that same port explicitly, **Then** dry run and deploy fail naming the port and both applications.
6. **Given** application A holds an automatically allocated port, **When** application A itself is changed to claim that same port explicitly, **Then** this is not a conflict — the deploy succeeds and A keeps the port.

---

### User Story 3 - Automatic port allocation that stays stable across redeploys (Priority: P3)

An operator who does not care which port an application gets requests publishing without naming a port. The system picks a free port from a documented range, reports it in the deploy output, and — critically — the application keeps that same port on every subsequent redeploy, including redeploys that take the container down and recreate it. The port is only given up when the application is explicitly deleted (or its team is deleted).

**Why this priority**: Automatic allocation removes the coordination burden for the common case, but it is only useful if the port is stable — bookmarks, local configs, and scripts pointing at `localhost:<port>` must keep working across redeploys.

**Independent Test**: Deploy an application with automatic publishing, note the assigned port, then redeploy with a change that forces the container to be recreated; verify the port is unchanged. Tear the application down, redeploy, and verify the port is still unchanged.

**Acceptance Scenarios**:

1. **Given** an application manifest requesting automatic publishing, **When** the operator deploys, **Then** a free port from the documented range is assigned, reported in the deploy output, and the application answers at `localhost:<assigned port>`.
2. **Given** an application with an automatically assigned port, **When** the operator redeploys it — including with changes that force its container to be removed and recreated — **Then** the application keeps the exact same host port.
3. **Given** an application with an automatically assigned port, **When** the operator tears the application down and later redeploys it, **Then** it comes back on the same port.
4. **Given** an application with an automatically assigned port, **When** the operator explicitly deletes the application (or deletes its team), **Then** the port is released and may be assigned to other applications in the future.
5. **Given** an application manifest requesting automatic publishing, **When** the operator runs a dry run before the first deploy, **Then** the output indicates a port will be assigned automatically, and no port is consumed or recorded by the dry run.
6. **Given** an application that already holds an automatically assigned port, **When** the operator runs a dry run, **Then** the output shows the already-held port.
7. **Given** every port in the automatic range is already taken, **When** an operator deploys a new application requesting automatic publishing, **Then** the deploy fails with a clear message that the range is exhausted, and no partial changes are left behind.
8. **Given** an application with an automatically assigned port, **When** the operator switches its manifest to an explicit port and redeploys, **Then** the old automatic allocation is released and the application answers on the explicit port.

---

### User Story 4 - Publishing works without a second switch, and platform exposure stays independent (Priority: P4)

An operator publishes an application's port without having to also enable the existing platform-exposure option. Publishing by itself is a complete declaration: the system attaches the application to the shared platform network automatically, because a published application must always be a platform-visible workload. Conversely, an application that only enables platform exposure gets no host port, and publishing a port never grants other teams the right to declare dependencies on the application.

**Why this priority**: This is a semantic guarantee rather than a new capability, but it prevents two real failure modes: a frustrating "why do I need two flags?" experience, and an accidental widening of cross-team access just because someone wanted localhost access.

**Independent Test**: Deploy one application with only the publish option, one with only platform exposure, and one with both; verify network attachment, port exposure, and cross-team dependency behavior independently for each.

**Acceptance Scenarios**:

1. **Given** an application manifest with the publish option and without the platform-exposure option, **When** the operator deploys, **Then** the deploy succeeds, the application is attached to the platform network, and its port is published — no validation error demands a second option.
2. **Given** an application manifest with only the platform-exposure option, **When** the operator deploys, **Then** the application is attached to the platform network and no host port is published.
3. **Given** an application published via the publish option only, **When** another team declares a dependency on it, **Then** the dependency is rejected exactly as it would be for any non-platform-exposed application — publishing grants no cross-team consumption rights.
4. **Given** an application manifest with both options set, **When** the operator deploys, **Then** the deploy succeeds with no warning — the combination is valid and redundant, not an error.
5. **Given** an application with the publish option, **When** the operator runs a dry run, **Then** the plan output shows the platform-network attachment alongside the port publication, so the implied attachment is visible.

---

### User Story 5 - Operators can read how publishing works in the reference documentation (Priority: P5)

An operator who has never used the feature opens the manifest reference documentation and finds the publish option fully described: both modes (automatic and explicit), the valid explicit range, the automatic range, the conflict rules, the lifecycle of an automatic allocation (stable across redeploys, released only on deletion), and — most importantly — a single table showing how the publish option and the existing platform-exposure option combine, so nobody has to guess whether one implies the other.

**Why this priority**: The interaction semantics (US4) are deliberate but not guessable from option names alone. Without reference documentation, every operator rediscovers them by trial and error; with it, the behavior contract in this spec is public. It is last in priority only because it documents the other stories and therefore lands after them.

**Independent Test**: Open the published manifest reference page and verify an operator can answer, from the page alone: "how do I publish on a fixed port?", "what happens to a dynamic port when I redeploy?", "do I also need platform exposure?", and "what conflicts will stop my deploy?".

**Acceptance Scenarios**:

1. **Given** the published manifest reference documentation, **When** an operator looks up the publish option, **Then** both modes are described with a manifest example each, including the valid explicit range and the automatic range.
2. **Given** the same reference page, **When** an operator wants to know how publishing relates to platform exposure, **Then** a single table presents all four combinations of the two options with their resulting behavior — platform-network attachment, host port exposure, and cross-team dependency access:

   | Platform exposure | Publish | On platform network | Host port published | Other teams may depend on it |
   |---|---|---|---|---|
   | off | off | no | no | no |
   | on | off | yes | no | yes |
   | off | on | yes (implied) | yes | no |
   | on | on | yes | yes | yes |

3. **Given** the same reference page, **When** an operator reads about automatic allocation, **Then** the lifecycle is stated explicitly: the port stays identical across redeploys, container recreation, and teardown, and is released only on application or team deletion.
4. **Given** the same reference page, **When** an operator reads about explicit ports, **Then** the conflict rules of this feature (duplicate explicit ports, platform-reserved ports, ports held by other applications) and the fail-fast behavior at dry run and deploy are described.

---

### Edge Cases

- An explicit port equal to the application's own previously assigned automatic port is adopted, not flagged as a conflict (US2, scenario 6).
- Switching between automatic and explicit publishing in either direction must not leak allocations: explicit → automatic assigns a port normally; automatic → explicit releases the old allocation (US3, scenario 8).
- A port occupied by an unrelated process on the host (outside this system's knowledge) cannot be detected at planning time; the deploy fails at container start with the underlying error naming the application and port.
- Removing the publish option from a deployed application and redeploying stops exposing the host port; an automatic allocation is retained (not released) so re-enabling publishing later returns the same port — only explicit deletion releases it.
- A dry run must never allocate, persist, or release ports, so repeated dry runs are free of side effects.
- Conflict detection must behave the same whether manifests are deployed as a full set, as a single team, or as a single application file.
- Explicit ports outside the valid range (or in the privileged range below 1024) are rejected by manifest validation with the other per-manifest errors, before any conflict analysis.
- Deleting a team releases every port held by that team's applications.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Application manifests MUST support a publish option with two forms: automatic (no port named) and explicit (a specific host port named).
- **FR-002**: A deployed application with the publish option MUST be reachable from the host at `localhost:<port>`, where `<port>` is the explicit port or the automatically assigned one.
- **FR-003**: Published ports MUST answer only on the host's loopback interface; they MUST NOT be reachable from other machines.
- **FR-004**: Explicit host ports MUST be validated per manifest: an integer in the allowed range (1024–65535), excluding the range reserved for automatic allocation. Violations are reported together with other manifest validation errors.
- **FR-005**: At both dry run and deploy, before any changes are made, the system MUST detect and reject: (a) two or more manifests claiming the same explicit port, (b) an explicit port that matches a platform-reserved port, and (c) an explicit port already held by a *different* application's persisted allocation.
- **FR-006**: All port conflicts present in one invocation MUST be reported in that single invocation, in deterministic order, each naming the port and the applications (or platform reservation) involved.
- **FR-007**: An application requesting automatic publishing MUST be assigned a free port from a documented, dedicated range, skipping platform-reserved ports, persisted allocations, and explicit ports declared in the same manifest set.
- **FR-008**: An automatically assigned port MUST remain identical across redeploys of the same application, including redeploys that remove and recreate its container, and across teardown followed by redeploy.
- **FR-009**: Automatic allocations MUST be released only on explicit application deletion or team deletion — never as a side effect of redeploy, container recreation, or teardown.
- **FR-010**: An application declaring an explicit port equal to its own persisted automatic allocation MUST adopt that port without a conflict; an application switching from automatic to a different explicit port MUST release its old allocation.
- **FR-011**: Dry run MUST preview publishing without side effects: it shows the explicit port, the already-held allocation, or an "assigned automatically" placeholder, and never allocates, persists, or releases anything.
- **FR-012**: The publish option alone MUST be sufficient: it implies attachment to the platform network, with no requirement to also set the platform-exposure option, and no error or warning when both are set.
- **FR-013**: The platform-exposure option alone MUST NOT publish any host port, and the publish option MUST NOT grant other teams the right to depend on the application.
- **FR-014**: Deploy output MUST state the resulting host port mapping for each published application, so the operator can discover an automatically assigned port without consulting anything else.
- **FR-015**: Adding, removing, or changing the publish option MUST take full effect on the next deploy of the application.
- **FR-016**: When the automatic range is exhausted, deploy MUST fail with a clear message and leave no partial changes.
- **FR-017**: Applications without the publish option MUST behave exactly as today: no host port exposure of any kind.
- **FR-018**: The user-facing manifest reference documentation MUST describe the publish option: both modes with an example each, the valid explicit range, the automatic range, the conflict rules and their fail-fast behavior, the automatic-allocation lifecycle, and a table covering all four combinations of the publish and platform-exposure options with their resulting network attachment, port exposure, and cross-team access.

### Key Entities

- **Publish declaration**: The per-application manifest option expressing intent to publish, in one of two modes — automatic or explicit — with the host port number in the explicit case. The published port always maps to the application's already-declared service port.
- **Port allocation record**: The persisted association between an application (identified by team and name) and its assigned host port. Outlives containers and teardown; removed only on explicit application or team deletion.
- **Platform-reserved ports**: The host ports the platform gateway itself occupies (its entry points and dashboard). These are never assignable and always conflict with explicit claims.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can go from "manifest edited to add publishing" to "application answering on `localhost:<port>`" in a single deploy invocation, on the first attempt.
- **SC-002**: 100% of explicit-port conflicts present in a manifest set are reported by one dry-run invocation, and zero deploy actions occur when any conflict exists.
- **SC-003**: Across 10 consecutive redeploys of an automatically published application — at least one forcing container recreation and at least one preceded by teardown — the assigned port is identical 10 out of 10 times.
- **SC-004**: Publishing an application requires exactly one manifest option; zero additional options, flags, or commands are needed to make the port reachable.
- **SC-005**: Running dry run any number of times leaves persisted state byte-for-byte unchanged.
- **SC-006**: Applications that do not opt in show zero change in exposed ports, network attachment, and cross-team access compared to the previous release.
- **SC-007**: An operator with no prior knowledge of the feature can answer, from the reference documentation alone, how to publish on a fixed port, what happens to an automatic port on redeploy, whether platform exposure is also required, and which conflicts stop a deploy — without reading this specification or the source code.

## Assumptions

- **Applications only**: Resources (databases, caches) are out of scope for this feature; the option can be extended to them later. The user's stated goal is accessing applications.
- **One published port per application**: mapping to the application's single declared service port. Multi-port publishing is out of scope.
- **TCP only**: matching how applications are routed today; UDP publishing is out of scope.
- **Loopback-only binding is not configurable** in this feature: the stated goal is `localhost:<port>` access, and loopback-only is the safe default. A future option could widen it deliberately.
- **Explicit ports are restricted to 1024–65535, excluding the automatic range**: the privileged range is rejected to avoid colliding with system services and the platform gateway's conventional ports, and the automatic-allocation range is reserved exclusively for automatic assignment so an explicit claim can never race an automatic allocation happening in the same deployment.
- **The automatic range is a dedicated high range** (for example 30000–32767), chosen during planning and published in the manifest reference documentation; its exact bounds are not a user-facing commitment beyond being documented there.
- **Conflicts with processes outside the platform's knowledge are not pre-detected**; they surface as a deploy-time failure naming the application and port.
- **Single-host model**: the platform deploys to one host, so "localhost" is unambiguous; multi-host concerns are out of scope.
- **Status/reporting surfaces beyond deploy and dry-run output** (for example a status command listing published ports) are a welcome follow-up but not required by this feature.
