# Research: Pinned Versions in `get`, `describe`, and `status`

**Feature**: 035-pinned-version-queries | **Date**: 2026-10-07

No `NEEDS CLARIFICATION` markers existed in the Technical Context. The epic design settles every choice of substance; the entries below record how each one lands in the codebase as it is on `main` after T4 (#63), the places where the design's wording needed refinement against the code, and the alternatives rejected. Design decisions TD-1 to TD-13 are not reopened.

## R1. One readable form, shared by the terminal and the tables

**Decision**: the helpers that render a pin for a person move from `internal/ui/terminal_logger.go`, where T3 wrote them unexported, into `internal/manifest` as `ReadableVersion(requested, digest string) string` and `ShortDigest(digest string) string`, beside `TagOf` and `IsDigestReference`, which they already call. `manifest` gains `DigestOf(ref string) string`, the part after `@` of a digest reference, so a pin's `Pinned` field (`repo@sha256:…`) can feed `ReadableVersion` the way the event's `digest` field does. `ui` calls the exported helpers; `exactVersion` stays in `ui`, its only reader.

**Rationale**: design section 3.5 defines one readable form for "wherever a pin is shown in a table", and T5 adds two more places that show it (the VERSION column and the `Pinned:` line). Constitution VII forbids a second copy; `manifest` is the package both `ui` and `handler` already import for the reference helpers, so no new dependency edge appears and no cycle is possible (`manifest` imports nothing under `internal/`).

**Alternatives considered**: a new `internal/imageref` package was rejected because `manifest` already owns `TagOf` and `IsDigestReference`, and splitting the reference helpers across two packages would be the harder thing to find. Having `handler` import `ui` was rejected: `ui` renders events and would then be imported for a string function.

## R2. Reading the pins for the listing once, not per row

**Decision**: `ListDeployed`, `ListApplications`, and `ListResources` load every pin once through `loadImagePins(store)`, which returns an empty map when `store.ImagePins` is nil (hand-built stores in tests) and the error of `ImagePins.ListAll()` otherwise. The pure `formatDeploymentsTable(deployments, pins map[string]state.ImagePin)` fills VERSION through `versionCell(d state.Deployment, pins)`: when `d.Policy` is `Pinned` and `pins[state.ImagePinKey(team, name)]` exists with a matching `Kind`, the readable form; otherwise `valueOrUnknown(d.Image)` as T1 left it. No row is read from Docker.

**Rationale**: design section 4.7 says "read the pin"; `ListAll` already exists for the dry-run snapshot and reads each team's `pins.txt` once, which is what a table over all teams needs, whereas `Get` per row would reopen the file per artifact. A nil `ImagePins` is tolerated the way `DeleteApplication` tolerates it, so the handler unit tests keep their in-memory stores. The kind check costs one comparison and guards against the one ambiguity T3's key (`team/name`) left open.

**Alternatives considered**: `Get(team, name)` per pinned row was rejected as a file read per row for no benefit. Failing the command when `ListAll` errors was kept: a broken `pins.txt` is a real fault and the listing commands already fail on a broken `deployments.txt`; the spec's "nothing fails" (FR-003) is about a missing pin, not a broken file.

## R3. `describe` gains a read-only container backend and tolerates its absence

**Decision**: `handler.DescribeApplication` and `handler.DescribeResource` gain a fourth parameter, `backend engine.ContainerBackend`. `cmd/describe.go` builds it with `app.NewQueryContainerBackend(cfg, store)`, exactly as `cmd/delete.go` does, and returns the constructor's error as every other command does. The handler never fails because of the backend: `runningImage(backend, containerID)` returns `unavailable (no container runtime)` when the backend is nil, `unavailable (<error>)` when `InspectContainer` fails, `-` when the inspection returns an empty image, and the reference otherwise. The team search and the not-found and ambiguity errors are untouched.

**Rationale**: design section 4.7 and T5-02: "`describe` gains a container backend through `NewQueryContainerBackend`; when Docker is unreachable the running image prints as unavailable and the command still succeeds". The Docker client constructor does not connect; it fails only on a malformed environment (`DOCKER_HOST` unparsable), which `status` and `delete` also refuse to run under, so treating it the same keeps one rule for all three. Unreachability shows up at `ContainerInspect`, which is the call the handler degrades. `InspectContainer` emits its error through the backend's observer, and the query backend carries `NoopObserver`, so nothing extra is printed.

**Alternatives considered**: swallowing the constructor error in `cmd/` and passing nil was rejected because it would hide a host misconfiguration that every other Docker-backed command reports. Adding a `DescribeOptions` struct was rejected: one new parameter, three call sites, no option to vary (constitution IV).

## R4. The running image comes from the container's configuration

**Decision**: `engine.ContainerInfo` gains `Image string`, filled in `DockerBackend.InspectContainer` from `resp.Config.Image` with a nil guard on `Config`. For a container T3 created from a digest reference this is `repo@sha256:…`; for a manifest-owned container it is the expanded tag reference T2's `ResolvedRef` carried. The dry-run backend's `InspectContainer` keeps returning the zero value, so its `Image` is empty.

**Rationale**: design section 4.1, last sentence, names `ContainerInspect(...).Config.Image` as the source. It is the reference the container was created from, which is what makes a recorded-but-undeployed pin show as a difference: the pin changes, the container's creation reference does not.

**Alternatives considered**: deriving the running image from the local image id (`resp.Image`) and the pin store was rejected because it answers "which bytes" rather than "which reference", and a bump to an image with the same id would then show no difference.

## R5. The `Pinned:` line carries the full reference, the readable form, and the date

**Decision**: `describe` prints, after `Pull policy:` and only when the record's policy is `Pinned`, `Pinned:       <pin.Pinned> (<readable>, <YYYY-MM-DD>)`, where `<readable>` is `manifest.ReadableVersion(pin.Requested, manifest.DigestOf(pin.Pinned))`, the same string the VERSION column shows, and the date is `pin.PinnedAt.UTC().Format(time.DateOnly)`, the same format T3's `pinned_at` event field uses. When the record is `Pinned` and no pin exists the line reads `Pinned:       -`. Then `Running image:` for every record. The twelve-character label column is kept.

**Rationale**: PRD R-18 names three facts, readable version, exact version, date; design section 4.7 says "`Pinned:` with the full digest reference and the date". Showing the readable form exactly as the table prints it lets an operator connect the row they saw in `get` to the full reference in `describe` without translating. One line keeps the detail block scannable; the spec records the two-line alternative for clarification and nothing in the design asks for it.

**Alternatives considered**: separate `Pinned at:` and `Pinned from:` lines were rejected as three lines for one fact. Printing `Pinned:` for manifest-owned records with `-` was rejected because the line would then suggest those artifacts can hold a pin.

## R6. The status IMAGE column and how a digest reference is shortened in it

**Decision**: `containerStatusRow` gains `Image`; the table prints `NAME KIND RUNNING STATUS IMAGE IMAGE ID`, IMAGE formatted `%-40s` between STATUS and IMAGE ID, the separator computed from the header length as the deployments table does. `inspectDeployments` fills it through `shortImageReference(info.Image)`: a digest reference becomes `<repository>@<twelve hex>` via `manifest.ShortDigest`; a tag reference is printed as is; an empty value prints `-`. Printing moves into a pure `formatStatusTable(rows) string` so it can be unit-tested; `printStatusTable` prints it.

**Rationale**: design section 4.7 ("the table gains an IMAGE column; IMAGE ID stays") and PRD OD-3; the shortening follows OD-4 and TD-11, which make twelve hex characters the table form of an exact version, and sits beside an IMAGE ID column that is itself truncated. The spec names this as the one presentation choice left open; this plan takes the short form and records it as a design refinement.

**Alternatives considered**: the full digest reference in the table was rejected because a loopback-registry reference with a 64-character digest is wider than the rest of the row combined. Dropping IMAGE ID in favour of IMAGE was rejected: the design keeps it, and the id is what tells two containers created from the same tag apart.

## R7. Making a pin differ from the running image before `bump` exists

**Decision**: the integration scenario for the difference pushes the newer `whoami` to the loopback registry (which returns its digest), rewrites the application's line in `<state>/<team>/pins.txt` with the new digest in place of the old through a test helper, and then describes the artifact. `Pinned:` must carry the new digest and `Running image:` the old one. The scenario reads the same once T6's `bump` lands; T6 may replace the helper with the command.

**Rationale**: T6 is in the same wave and may merge after T5; the spec's Assumptions already settle this. The pin file is a documented, line-oriented state format (design section 3.4), so editing it in a test is editing state the way `SeedLegacyDeploymentRecords` does.

**Alternatives considered**: waiting for T6 was rejected as a cross-ticket dependency the delivery plan does not have.

## R8. Where the integration scenarios go

**Decision**: one new file, `tests/integration/pinned_version_queries_test.go`, with `TestPinnedVersionQueries` on `newPinnedSuite` (the loopback registry world from `pinned_image_policy_test.go`, same package), covering the pinned fixture across `get`, `describe`, and `status`, the difference, and teardown. Two small scenarios are appended to the existing suites for the manifest-owned rows: `TestDescribeDocker` asserts `Running image:` with the expanded `traefik/whoami` reference, and `TestStatusDocker` asserts the IMAGE column; `TestDescribeNoDocker` gains a scenario on seeded records asserting `Running image:` reads `unavailable` and the command succeeds. No existing assertion changes.

**Rationale**: the ticket says "extend `get`, `describe`, and `status` suites with a pinned fixture"; the pinned fixture needs the registry world, which is built once in `newPinnedSuite`, so the pinned scenarios live beside it and the existing suites gain only what their fixtures can show. Per the project's rules the scenarios are written first, compile-checked with `go vet -tags integration ./tests/integration/...`, and run only in CI.

## R9. Documentation

**Decision**: per [contracts/docs-and-deviations.md](contracts/docs-and-deviations.md): the manifest reference's Image pull policy subsection gains a paragraph on reading a pinned version in `get`, `describe`, and `status`; the `Long` help of `describe app`, `describe resource`, `status`, `status application`, and `status resource` names the new lines and column and `make docs-gen-cli` regenerates the five pages; `AGENTS.md`'s CLI reference lines for `status` and `describe` name them. The operator guide is T8's.
