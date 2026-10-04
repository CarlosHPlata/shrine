# Feature Specification: Teardown Headers That Print and Teardown Event Names That Match the Rest of the Log

**Feature Branch**: `028-fix-teardown-event-names`
**Created**: 2026-10-02
**Status**: Draft
**Input**: GitHub issue [#46](https://github.com/CarlosHPlata/shrine/issues/46) — "bug: teardown events are emitted as Application.teardown, so the terminal never prints the "Tearing down" headers"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The operator sees which deployment is being torn down (Priority: P1)

An operator runs `shrine teardown <team>` on a team with several applications and resources. Deploy already announces each deployment before working on it (`🚀 Deploying Application: whoami (owner: …)`), and teardown was designed to do the same with `🗑️  Tearing down Application: whoami (team: …)` and `🗑️  Tearing down Resource: db (team: …)`. Today those headers never appear: the output goes straight to the indented container-removal lines, so the operator sees a flat run of "Removing container" steps with nothing saying which deployment each one belongs to or that a new deployment has started. After this change, every deployment removed by a teardown is introduced by its own header line, printed before that deployment's removal steps.

**Why this priority**: This is the visible defect and the reason the issue was filed. Teardown is a destructive command; the header is the operator's confirmation of *what* is being destroyed, in what order, and for which team. The header text has existed in the product since teardown was introduced — it is simply never reached in a real run.

**Independent Test**: Deploy a team with at least one application, run `shrine teardown <team>`, and assert the terminal output contains `Tearing down Application: <name> (team: <team>)` before that application's container-removal lines. Repeat with a team that also has a resource and assert the `Tearing down Resource: …` header.

**Acceptance Scenarios**:

1. **Given** team `shrine-deploy-test` has the application `whoami` deployed, **When** the operator runs `shrine teardown shrine-deploy-test`, **Then** the terminal prints `🗑️  Tearing down Application: whoami (team: shrine-deploy-test)` exactly once, before the lines reporting removal of that application's container.
2. **Given** a team has one application and one resource deployed, **When** the operator runs `shrine teardown <team>`, **Then** the terminal prints one `Tearing down Application: …` header and one `Tearing down Resource: …` header, each immediately preceding its own deployment's removal steps, with applications torn down before resources as today.
3. **Given** a team has several applications deployed, **When** the operator runs `shrine teardown <team>`, **Then** exactly one header is printed per deployment — the number of headers equals the number of deployments removed.
4. **Given** a team with no recorded deployments, **When** the operator runs `shrine teardown <team>`, **Then** no "Tearing down" header is printed and the rest of the output is unchanged from today.
5. **Given** any team, **When** the operator runs `shrine deploy`, **Then** the deploy output is byte-for-byte unchanged — deploy is not affected by this fix.

---

### User Story 2 - Teardown entries in the log and in error lines use the same naming as everything else (Priority: P2)

An operator investigating a past teardown searches `shrine.log` for teardown activity, or has a script that filters the log by event name. Every other event in the log is lowercase and dot-separated — `application.deploy`, `container.remove`, `network.remove` — so the natural search is `application.teardown`. Today that search finds nothing, because teardown alone writes `Application.teardown` / `Resource.teardown`. The same inconsistency reaches the terminal when a removal fails: the operator sees `❌ Error [Application.remove]: …` while every deploy-side failure reads `❌ Error [application.…]`. After this change, teardown events follow the same lowercase convention as the rest of the product, in the log and in error lines alike.

**Why this priority**: The defect here is inconsistency rather than missing information — the entries exist, they are just named differently — so it ranks below Story 1. It still matters: a log whose names follow one rule can be searched and filtered with one rule, and an operator who greps for `application.teardown` after a failed teardown should not come up empty.

**Independent Test**: Deploy a team, tear it down, and assert the log contains `[started] application.teardown name="<app>" team="<team>"` and contains no entry whose name starts with a capital letter. Separately, force a container-removal failure during teardown and assert the terminal error line and the log entry are named `application.remove`.

**Acceptance Scenarios**:

1. **Given** team `shrine-deploy-test` has the application `whoami` deployed, **When** the operator runs `shrine teardown shrine-deploy-test`, **Then** the log gains the entry `[started] application.teardown name="whoami" team="shrine-deploy-test"` and no entry named `Application.teardown`.
2. **Given** a team with a resource deployed, **When** the team is torn down, **Then** the log records `resource.teardown` for that resource, not `Resource.teardown`.
3. **Given** removal of an application's container fails during teardown, **When** the failure is reported, **Then** the terminal shows `❌ Error [application.remove]: …`, the log records the failure under `application.remove`, and the command exits non-zero as it does today.
4. **Given** removal of a resource's container fails during teardown, **When** the failure is reported, **Then** the failure is named `resource.remove` on the terminal and in the log.
5. **Given** an application's container is removed but removal of its route fails, **When** the failure is reported, **Then** the failure is named `application.routing_remove` on the terminal and in the log.
6. **Given** a full teardown of a team with applications and resources, **When** the run completes, **Then** every event name written to the log during that run is lowercase.

---

### Edge Cases

- **Existing state**: deployments recorded before this fix were recorded with the capitalised kind (`Application` / `Resource`). They must tear down correctly, with headers, without any migration or re-deploy — the fix concerns how the kind is *named in events*, not how it is recorded.
- **Kind-dependent behaviour is untouched**: only applications have routes removed on teardown, and applications are torn down before resources. Changing how events are named must not change which deployments are treated as applications.
- **Removal failure after the header**: when a removal fails, the header for that deployment has already been printed; the error line follows it and the run stops there, as today. Deployments later in the order get no header because they are never attempted.
- **Container already gone**: when a recorded deployment's container no longer exists, the header is still printed, followed by the existing "not found, skipping removal" line.
- **Past log entries**: `shrine.log` is append-only. Entries written by earlier versions under `Application.teardown` / `Resource.teardown` stay as they are; the log will contain both spellings across the upgrade boundary.
- **No double reporting**: a header is printed once per deployment, and a failure produces one error line under the lowercase name — never one under each spelling.
- **Human-readable error text**: the message that follows the error name (for example `Application "whoami": …`) is prose, not an event name, and is out of scope; only the name inside `[...]` and in the log changes.
- **Other commands**: `deploy`, `apply`, `status`, and dry-run output are unchanged. The team network removal step at the end of teardown (`network.remove`) is already lowercase and unchanged.

## Requirements *(mandatory)*

### Functional Requirements

**Teardown headers**

- **FR-001**: For every deployment removed by `shrine teardown <team>`, the terminal MUST print exactly one header line before that deployment's removal steps: `🗑️  Tearing down Application: <name> (team: <team>)` for an application and `🗑️  Tearing down Resource: <name> (team: <team>)` for a resource.
- **FR-002**: Headers MUST appear in the order deployments are torn down (applications, then resources, as today), and a teardown of a team with no recorded deployments MUST print no header.

**Consistent event naming**

- **FR-003**: The event announcing the start of a deployment's teardown MUST be named `application.teardown` or `resource.teardown` — lowercase — everywhere it is observable, including `shrine.log`.
- **FR-004**: Teardown failure events MUST be named in lowercase everywhere they are observable (terminal error line and log): `application.remove` and `resource.remove` for a failed container removal, and `application.routing_remove` for a failed route removal.
- **FR-005**: Every event produced during a teardown run MUST have a name free of uppercase letters; teardown event names MUST follow the same lowercase, dot-separated convention as every other event in the product.
- **FR-006**: Each teardown event MUST be emitted under exactly one name; the capitalised names (`Application.teardown`, `Resource.teardown`, `Application.remove`, `Resource.remove`, `Application.routing_remove`) MUST no longer be produced, and MUST NOT be emitted alongside the lowercase ones.

**No regression**

- **FR-007**: Teardown behaviour other than event naming and the newly visible headers MUST be unchanged: the same containers, routes, and network are removed in the same order; route removal still applies only to applications; failures still stop the run and exit non-zero with the same underlying error.
- **FR-008**: Deployments recorded by earlier versions MUST tear down correctly, with headers and lowercase event names, without any migration, re-deploy, or manual edit of state. How a deployment's kind is recorded in state MUST NOT change.
- **FR-009**: Output and log entries of `deploy`, `apply`, `status`, and dry-run MUST be unchanged.

**Regression coverage**

- **FR-010**: Automated unit coverage MUST assert the names of the events a teardown produces — the started event for an application and for a resource, and the failure events for container removal and route removal — so that a return to capitalised names turns a test red. The existing unit test that deliberately pins the capitalised names as "observed behaviour" (added under spec 027) MUST be updated to assert the lowercase names.
- **FR-011**: The end-to-end teardown scenario MUST assert that the "Tearing down Application: <name>" header appears in the real command's output and that the log records `[started] application.teardown`, so the fix is verified through the real binary and not only at unit level.
- **FR-012**: Project documentation that describes observed teardown event names — the spec 027 notes recording the capitalised names as a known defect, and the feature progress record — MUST be updated to reflect that the defect is fixed.

### Key Entities

- **Teardown event**: A named notification produced as a team is torn down — one announcing the start of each deployment's removal, and one for each failure. Every observer (terminal, log file) receives the same name; the name is what the terminal matches to decide whether to print a header and what the log writes at the start of each entry.
- **Deployment kind**: Whether a recorded deployment is an Application or a Resource. Recorded in state in its capitalised manifest form; it decides teardown order and whether a route is removed. This feature changes only how the kind appears *inside event names*, never how it is recorded or compared.
- **Teardown header**: The single terminal line that introduces a deployment's removal, naming its kind, name, and team.
- **Log entry**: One line in `shrine.log` of the form `[status] event.name key="value" …`; append-only, never rewritten.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In 100% of teardown runs, the number of "Tearing down …" headers printed equals the number of deployments removed, and each header appears before its deployment's removal lines.
- **SC-002**: An operator reading teardown output can tell, for every removal step, which deployment and team it belongs to without consulting state or the log.
- **SC-003**: A single lowercase search of the log (`application.teardown`, `resource.teardown`) finds every teardown started after this fix; zero log entries written after the fix have a name containing an uppercase letter.
- **SC-004**: 100% of teardown failures are reported under a lowercase name (`application.remove`, `resource.remove`, `application.routing_remove`), matching the naming of deploy-side failures.
- **SC-005**: Zero regressions: the same containers, routes, and networks are removed as before; teams deployed before the fix tear down without any manual step; `deploy`, `apply`, `status`, and dry-run output are unchanged; the existing integration suite passes.
- **SC-006**: Reintroducing a capitalised teardown event name causes at least one automated unit test and the end-to-end teardown scenario to fail.

## Assumptions

- The intended behaviour is the lowercase naming. The terminal already knows how to render `application.teardown` / `resource.teardown`, every other event name is lowercase, and earlier project documents (spec 009's event contract) describe `application.teardown` — so the capitalised names are the defect, not the terminal's expectations. Teaching the terminal to accept the capitalised names instead would leave the log inconsistent and is rejected.
- The fix is confined to how teardown events are *named*. The kind stored in state and used to decide ordering and route removal keeps its capitalised manifest form; no state migration is needed or wanted.
- The human-readable error text that accompanies a failure (for example `Application "whoami": …`) is left as is. The issue asks only for the event names to change, and rewording error prose is a separate, purely cosmetic decision.
- Renamed log entries are an accepted user-visible change, as the issue states. No operator tooling is known to depend on the capitalised names — they have never matched the documented convention — so no compatibility alias or transition period is provided, and historical log entries are not rewritten.
- The issue lists the end-to-end assertion as optional. It is treated as required here because the project constitution makes the integration suite the acceptance gate for user-visible behaviour, and this defect survived precisely because nothing asserted on teardown's terminal output. Per project practice the scenario is authored and compile-checked locally and executed by CI.
- The defect was identified from the code path and confirmed by a unit test; it has not been observed on a live run. The end-to-end assertion in FR-011 doubles as that live confirmation.
- Only teardown is affected. Deploy names its events with fixed lowercase names and needs no change; no other command derives event names from the recorded kind.
- The existing file-logger integration scenario asserts on the team network-removal entry and is unaffected by the rename.
