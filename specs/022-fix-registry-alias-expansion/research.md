# Research: Expand `reg:` Registry Aliases Before the Container Is Created

**Feature**: `022-fix-registry-alias-expansion` | **Date**: 2026-08-14

No NEEDS CLARIFICATION markers existed in the Technical Context — the defect,
its mechanism, and its blast radius were established by a forensic pass over
`main` @ 94c3307 (recorded in issue #33 and verified in-repo). Research
therefore focused on the five design decisions the fix requires.

## D1 — Where the expansion happens

**Decision**: Expand the alias exactly once, at the top of
`DockerBackend.CreateContainer`, into the by-value `op` (`op.Image = expanded`)
before anything reads `op.Image`. Remove the now-redundant expansion inside
`resolveImage` and document on `resolveImage` that its caller owns expansion.

**Rationale**: `CreateContainerOp` is passed by value, so mutating `op.Image`
is local to the one creation and cannot leak. Every downstream reader — the
pull (`resolveImage`), the credential lookup (`registryAuthFor`), the config
hash, and the container spec (`createFreshContainer`) — then agrees on one
string, which is FR-003 verbatim. Leaving a second expansion inside
`resolveImage` would be harmless (expansion is a no-op on non-`reg:` refs) but
would re-create the split-brain the bug came from: two sites that each expand
"their" copy, with no single owner.

**Alternatives considered**:
- *Expand only in `createFreshContainer`*: fixes `ContainerCreate` but leaves
  `resolveImage` expanding separately — two owners again, FR-003 violated by
  construction.
- *Return the expanded ref from `resolveImage`*: changes a signature and makes
  the caller thread a second return value everywhere; more churn for the same
  result.
- *Resolve at plan time*: structurally superior (the engine would only ever see
  fully-qualified refs) but explicitly out of scope per the spec — it requires
  the plan to carry both forms to keep dry-run output faithful (FR-007).
  Tracked as follow-up work.

## D2 — The observation seam (`dockerAPI` interface)

**Decision**: Introduce an unexported `dockerAPI` interface inside package
`dockercontainer` and change `DockerBackend.client` from `*client.Client` to
it. The interface lists exactly the 12 methods the backend calls today:

`ContainerCreate`, `ContainerInspect`, `ContainerRemove`, `ContainerStart`,
`ImageInspect`, `ImageList`, `ImagePull`, `NetworkCreate`, `NetworkInspect`,
`NetworkRemove`, `VolumeCreate`, `VolumeInspect`.

`*client.Client` satisfies it structurally, so `NewDockerBackend` and every
call site are unchanged — the diff is one field type plus one new file.

**Rationale**: The spec's SC-003/VR-002 require asserting on the container
specification *at the point of creation*. `docker_container.go` currently has
zero unit tests because nothing behind `backend.client` is fakeable — the exact
gap that let the bug ship. A structural interface is the minimal seam: no
mocking framework, no constructor changes, no production behaviour change.

**Alternatives considered**:
- *No seam, integration-only coverage*: satisfies VR-001 but leaves the
  in-milliseconds assertion impossible; the container-spec check would live
  only in the ~10-minute suite. Rejected — and recorded in the plan's
  Complexity Tracking as the rejected simpler alternative.
- *Public interface in `internal/engine`*: wider surface than needed; the seam
  is a package-internal testing concern, not an engine contract (Principle III
  already defines the engine-level backends).

## D3 — Unit-test shape under the no-filesystem constraint

**Decision**: The regression unit test constructs `DockerBackend` directly
(same package) with a fake `dockerAPI`, a no-op observer, `nil` state store,
and the alias registries. The fake's `ContainerInspect` returns
`errdefs.ErrNotFound` (forcing the fresh-create path), and its
`ContainerCreate` **captures the `*container.Config` and returns an error**.
The test asserts the captured `Config.Image` equals the expanded reference and
has no `reg:` prefix.

**Rationale**: Unit tests in this repo must not touch the filesystem (project
rule). `CreateContainer` writes deployment state only after a successful
create+start; capture-then-error stops the flow before `recordDeployment`, so
`state` can be `nil` and no file is ever written. The assertion is still on the
outcome (what Docker would have received), per VR-002.

**Alternatives considered**:
- *Fake succeeds, then assert*: reaches `recordDeployment` → requires a real
  `state.Store` → filesystem. Rejected.
- *Assert via `resolveImage`'s behaviour*: mechanism assertion, exactly the
  kind VR-002 forbids.

## D4 — Integration-test shape (VR-001, VR-003)

**Decision**: Extend `tests/integration/registry_alias_test.go` with real
(non-dry-run) deploys using `NewDockerSuite`, reusing the existing fixtures —
`registry-alias/config.yml` already maps `myregistry → docker.io`, so
`reg:myregistry/traefik/whoami:latest` expands to a publicly pullable image.
Add one `AssertContainerImage(name, wantRef)` helper to
`tests/integration/testutils/assert_docker.go`, following the
`AssertContainerEnvVar` pattern (`ContainerInspect(...).Config.Image`).
Cover: alias app deploy (US1), alias resource deploy (US2), and the
form-equivalence no-recreate check (FR-010) via two fixture dirs identical
except for the image form, asserting the container ID is unchanged across the
two deploys.

**Rationale**: `Config.Image` as reported by the daemon is the string Docker
was handed at create time — the closest possible observation of "what the
container runtime received" from outside the process, satisfying VR-001 with
no test-only code path. Fail-first (VR-003) is trivially demonstrable: against
unfixed code the deploy itself fails with `invalid reference format`.

**Alternatives considered**:
- *Local registry container for fixtures*: heavier, slower, and unnecessary —
  the docker.io alias mapping already exercises expansion end-to-end.
- *Asserting only `AssertContainerRunning`*: would catch this bug (the deploy
  fails outright) but not a future partial-expansion bug; the image assertion
  is the durable regression guard.

## D5 — Error enrichment and renderer guard (FR-008, FR-009)

**Decision**:
1. In `createFreshContainer`, include the image in both the wrapped error
   (`creating container %q (image %q): %w`) and the event fields
   (`"image": op.Image`). By that point `op.Image` is the expanded form — the
   string Docker actually rejected.
2. In `internal/ui/terminal_logger.go`, guard the `container.create` case to
   print the `🏗️  Creating container` progress line only for
   `engine.StatusInfo` events.

**Rationale**: The terminal renderer prints `❌ Error [...]: <fields["error"]>`
for every error event, so putting the image into the wrapped error message is
what makes it operator-visible (FR-008); adding it to fields keeps structured
observers informed too. The three-line phantom output (FR-009) exists because
the `container.create` case currently renders on *every* status: once for the
engine's info event, once for the backend's error event (whose only field is
the full container name, hence the leading-dot `.team.name` artifact), and once
for the engine's error re-emit. A status guard fixes both phantom lines in one
place; the two `❌` lines remain, as they should.

**Alternatives considered**:
- *Add team/name fields to backend error events*: makes the phantom line
  well-formed instead of removing it — still three "creating" lines for one
  attempt, FR-009 unmet.
- *De-duplicate the engine's error re-emit*: changes engine/observer semantics
  for a presentation problem; the renderer is the layer that chose to render
  errors twice.

## D6 — FR-010 (form equivalence never triggers recreate) — verification only

**Decision**: No code change. `configHash` hashes the resolved image *digest*
(plus env/volumes/ports/expose), not the reference string, so the alias form
and its fully-qualified equivalent hash identically both before and after this
fix. The integration equivalence test (D4) pins this behaviour so a future
change to `configHash` cannot silently break it.

**Rationale**: The digest-based hash is currently what *masks* the bug for
existing containers, but after the fix it is exactly the property FR-010 wants.
It must graduate from accident to tested contract.
