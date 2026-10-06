# Delivery plan: Pinned Image Versions

**Status**: Draft for review
**Sources**: [prd.md](prd.md), [design.md](design.md), [tickets.md](tickets.md)
**Created**: 2026-10-06

This plan says when each ticket runs, what must be true before it starts and before it is called done, how parallel tickets avoid stepping on each other, and what one agent does from issue to merge.

## 1. Shape

```text
Step 0   epic docs on main, GitHub issues created, spec numbers assigned
Wave 1   T1 ║ T2                      two agents in parallel
Wave 2   T3                           one agent, the critical path
Wave 3   T4 ║ T5 ║ T6 ║ T7            up to four agents; merge order T4, T7, T5, T6
Wave 4   T8                           one agent; drafting may start during wave 3
```

Critical path: T2, then T3, then the longest of wave 3, then T8. With sizes S=1, M=2, L=4 as relative effort, the waves take about 2, 4, 2, and 2 units, roughly ten in total against fifteen if run one after another.

Every merge leaves main releasable. Two milestones are worth a tagged release: after wave 2, when pinning works end to end, and after wave 4, when the feature is complete and documented.

## 2. Step 0: before any ticket starts

1. Merge the epic documents. The branch `docs/pinned-image-versions-prd` carries `specs/epics/pinned-image-versions/`; it goes to main through a docs pull request so every agent reads the same PRD, design, tickets, and plan from main.
2. Fold the approved PRD amendments in before that merge: M2 wording, R-15 in two halves, R-11 and R-12 release on a manifest-owned deploy, and the policy field in T1's record.
3. Create the eight GitHub issues, one per ticket, from the ticket sections. Each issue carries the ticket id, the wave, the pre-assigned spec number and branch name below, the Dependencies section verbatim, the links to the three documents, and the definition of done from section 4.
4. Label them `epic:pinned-image-versions` and `wave:1` to `wave:4`; put them in one milestone.

Spec numbers are assigned here, not at run time, because two agents starting in the same minute would otherwise both claim the next free number. An agent passes `--number` to the feature script, which the git extension's `create-new-feature.sh` supports.

| Ticket | Spec | Branch |
|---|---|---|
| T1 | 031 | `031-deployed-version-columns` |
| T2 | 032 | `032-preflight-image-resolve` |
| T3 | 033 | `033-pinned-image-policy` |
| T4 | 034 | `034-pull-policy-config-default` |
| T5 | 035 | `035-pinned-version-queries` |
| T6 | 036 | `036-bump-command` |
| T7 | 037 | `037-delete-resource` |
| T8 | 038 | `038-image-versions-guide` |

## 3. Schedule

| Wave | Tickets | Agents | Entry gate | Exit gate |
|---|---|---|---|---|
| 1 | T1, T2 | 2 | Step 0 complete; main green | Both merged; main green |
| 2 | T3 | 1 | Wave 1 merged, in particular T2 | T3 merged; main green; the local registry fixture runs in CI |
| 3 | T4, T5, T6, T7 | up to 4 | T3 merged; main green | All four merged in the order of section 5; main green |
| 4 | T8 | 1 | Wave 3 merged; T8 may draft from the moment T3 merges | Docs build and link check green; merged |

"Main green" means the unit suite and the integration suite pass on main, and `graphify-out/` is current, which the project's rule makes part of every pull request.

A wave does not wait for a straggler to start the next wave's independent work only when the dependency table in [tickets.md](tickets.md) allows it. Concretely: T1 can be late without delaying T3, because T3 depends only on T2; T5 then waits for T1. Nothing in wave 3 starts before T3 merges.

## 4. Definition of done for every ticket

A ticket is done when all of the following hold, and the pull request description lists them.

- The numbered spec, plan, tasks, and research exist under `specs/NNN-<name>/` and every task is checked.
- The integration scenarios the ticket names in [tickets.md](tickets.md) exist, were written before the implementation, compile under the integration build tag, and pass in CI. They are not run locally.
- Unit tests touch no filesystem; file-backed stores are tested through their injectable file operations.
- The documentation the ticket owns is updated in the same pull request: manifest reference, configuration docs, regenerated CLI pages, and the `AGENTS.md` lines the design names for it.
- `specs/progress.md` has an entry for the spec in the project's usual form, and `graphify update .` was run.
- A pull request review with `/shrine-pr-review` returned no open finding, and CI is green.
- Existing behaviour is unchanged for manifests and installations that do not opt in; the existing integration suites pass without edits to their assertions, apart from added output lines where the ticket says so.

## 5. Wave 3 merge order and the rebase rule

The four tickets share a few files. The order below merges the small tickets that touch shared seams first, so the larger ones rebase once onto a settled main.

1. **T4** first: it threads one parameter through the three plan call sites and touches the configuration layout of `AGENTS.md`.
2. **T7** second: small; its edits to `internal/handler/deployments.go` are a generalisation T5 then builds on.
3. **T5** third: the largest share of `internal/handler/deployments.go`, plus status and describe.
4. **T6** last: mostly new files; its only shared edits are the `Repin` field on the op, the fourth branch in `docker_image.go`, and the CLI reference lines of `AGENTS.md`.

Rebase rule: an agent rebases onto main before opening the pull request and again before merge when main moved. A conflict in `AGENTS.md` or `internal/handler/deployments.go` is expected and resolved by the later ticket; the earlier ticket never waits.

Hotspots, for the record:

| File | Touched by |
|---|---|
| `internal/handler/deployments.go` | T5, T7 |
| `internal/engine/backends.go` | T5 adds to `ContainerInfo`; T6 adds to `ResolveImageOp` |
| `internal/engine/local/dockercontainer/docker_image.go` | T6 only, in wave 3 |
| `internal/handler/deploy.go`, `internal/handler/apply.go` | T4 only, in wave 3 |
| `AGENTS.md` | T4 (config layout), T6 and T7 (CLI reference) |

## 6. The agent protocol: one agent, one spec, one ticket

1. **Start from main.** Fetch, check out main, confirm it is green and that the epic documents are present under `specs/epics/pinned-image-versions/`.
2. **Read in this order**: the GitHub issue; the ticket's section in [tickets.md](tickets.md); the ticket's requirement list and the sections it links in [design.md](design.md); the PRD journeys and requirements the ticket cites. Decisions TD-1 to TD-13 are settled; a disagreement is raised on the issue, not resolved in the spec.
3. **Specify** with `/speckit-specify`, passing the ticket's scope as the description and the pre-assigned number to the branch script. The spec's user stories are the ticket's acceptance items; its functional requirements start from the `T<n>-<nn>` list.
4. **Clarify** with `/speckit-clarify`; answer from the design before asking the owner. The open points in design section 7 are the only expected questions for T3.
5. **Plan** with `/speckit-plan`; the constitution check passes by construction for the seams the design names, and the plan says so with the design section as the reference.
6. **Tasks** with `/speckit-tasks`, integration scenarios first.
7. **Implement** with `/speckit-implement`. Run `go test ./...` after each logical change; compile the integration suite with `go vet -tags integration ./tests/integration/...`; do not run it locally.
8. **Finish**: documentation, `specs/progress.md`, `graphify update .`, the pull request through `/speckit-git-pr`, a `/shrine-pr-review` pass, CI, merge.
9. **Hand off**: the pull request description names anything the next tickets should know, in particular any deviation from the design, which is also recorded in design.md by the same pull request.

## 7. Risks to delivery

- **T3 is large and alone on the critical path.** Mitigation: it starts the moment T2 merges, its spec is the first one written in wave 2 before T1 is even done if needed, and its fixture helper is written first so the suite shape is known early. Splitting it stays the fallback described in [tickets.md](tickets.md).
- **The local registry in CI.** The integration job already needs Docker; a `registry:2` container on the loopback interface needs no daemon configuration. Risk: the runner's Docker cannot push to it, or the port is taken. Mitigation: bind to port zero and read the assigned port; prove it in T3's first CI run before the pinning scenarios are written.
- **Spec number collisions in parallel waves.** Mitigated by step 0's pre-assignment.
- **Merge conflicts in wave 3.** Mitigated by the order and the rebase rule in section 5.
- **Docs CI only checks the built site.** T8 and every ticket that edits a guide run the local docs build and link check before the pull request, as the reconcile feature did.
- **Design drift.** An agent that finds the design wrong fixes the design in its pull request and says so in the description, so the next agent reads the corrected version.

## 8. Tracking

- The GitHub milestone shows wave progress; each issue closes with its pull request.
- `specs/progress.md` gains one entry per spec as it lands, as for every feature.
- This plan is updated only when the order or the gates change; the per-ticket detail stays in [tickets.md](tickets.md).
