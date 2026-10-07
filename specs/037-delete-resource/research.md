# Research: `shrine delete resource` and pin release on every delete

**Feature**: 037-delete-resource | **Date**: 2026-10-07

No `NEEDS CLARIFICATION` markers existed in the Technical Context. Design section 4.8 settles the shape ("`DeleteResource` in T7 is `DeleteApplication` generalised over the kind: no host port, same refusal while the container exists, record and pin released, `--team` and `--dry-run`"). The entries below record how that lands on `main` after T6 (#65), the two places where the code as it is needed a refinement, and the alternatives rejected. Decisions TD-1 to TD-13 are not reopened.

## R1. One kind-parameterised delete behind two exported handlers

**Decision**: `handler.DeleteApplication` becomes a one-line wrapper over an unexported `deleteArtifact(store, container, kind, opts)`, and `handler.DeleteResource` is the second wrapper. `DeleteApplicationOptions` is renamed `DeleteOptions` (`Name`, `Team`, `DryRun`) and both wrappers take it. Inside `deleteArtifact`, the host-port step (read, dry-run line, release, success line) runs only when `kind == manifest.ApplicationKind`; the container check, the pin step, the record step, and the nothing-held line are identical, with the lower-cased kind word (`application`, `resource`) in every message where the word `application` appears today. The three helpers it calls gain a kind: `resolveDeleteTeam(store, kind, name, team)`, `findImagePin(store, team, kind, name)`, `findDeploymentRecord(store, team, kind, name)` (renamed from `findApplicationRecord`).

**Rationale**: design 4.8 and T7-01 name the generalisation. Two exported functions that differ in one word and one step are what constitution VII calls out for extraction; a single private function with a kind parameter keeps the refusal, the search, the dry run, and the output lines identical in form between the verbs (spec FR-007) with no second copy to drift. The exported wrappers keep `cmd/` thin and keep the handler's public surface readable as verbs.

**Alternatives considered**: a single exported `DeleteArtifact(kind, …)` called by both commands was rejected because every other handler is named by the verb and kind it performs (`Bump` takes a kind in its options, but it is one verb over one code path; delete already has an exported `DeleteApplication` with four unit tests calling it by name). Keeping `DeleteApplication` untouched and writing `DeleteResource` beside it was rejected as a forty-line duplicate. Keeping the options type name `DeleteApplicationOptions` and reusing it for resources was rejected as misleading; the rename touches one command file and the handler tests, all mechanical.

## R2. Finding the team for a resource

**Decision**: `resolveDeleteTeam` collects candidate teams from three sources today: host-port allocations keyed `team/name`, deployment records whose `Kind == ApplicationKind`, and pins whose `Kind == ApplicationKind`. With a kind parameter, the host-port source is consulted only for `ApplicationKind` (a resource never holds a port), and the record and pin sources compare `Kind` to the requested kind. The ambiguity error reads `ambiguous: resource "x" found in teams [a, b], use --team to disambiguate`, the application form with the kind word swapped.

**Rationale**: spec FR-002 and the assumption that a resource whose record was lost but whose pin remains can still be retired by name, which is exactly why T3 added the pin source for applications. Filtering by kind is what makes "a name held as an application in team A and as a resource in team B" unambiguous for `delete resource` (spec US4 scenario 5).

**Alternatives considered**: searching the manifest directory as `bump` does was rejected: delete works on state, not manifests, and must find an artifact whose manifest has been removed (the retire journey J8 deletes a manifest and then its state).

## R3. The pin read is made kind-aware, and why the record removal need not be

**Decision**: `findImagePin` today returns whatever `ImagePins.Get(team, name)` holds; it gains the guard `pin.Kind == kind`, the same guard `resolveDeleteTeam` already applies when listing pins and `isPinFor` applies in the queries. `Deployments.Remove(team, name)` stays by name: the local store keeps one record per name per team (a map keyed by name), the pin store keeps one pin per `team/name`, and the container is named `<team>.<name>` for both kinds, so an application and a resource of the same name in one team cannot coexist in state. The guard on the pin read is therefore a consistency measure, not a reachable branch: `delete resource` can never release an application's pin, and a request for a resource that only exists as an application is reported as nothing to delete (spec FR-006, edge cases).

**Rationale**: spec FR-006 asks that the two delete verbs never release each other's pin. The cheapest way to guarantee it is to make every pin read in the delete path agree with every pin listing in the delete path. Adding a kind to `Remove` would change a `DeploymentStore` method and a local store for a case the naming convention already excludes; constitution IV says no.

**Alternatives considered**: leaving `findImagePin` kind-blind was rejected: the candidate search and the release would disagree, and a unit test could show `delete resource` releasing an `Application` pin from a hand-built store. Extending `ImagePinKey` with the kind was rejected as a T3 format change outside this ticket's scope.

## R4. The command file and its flags

**Decision**: `cmd/delete.go` gains `deleteResourceCmd` (`Use: "resource [name]"`, `cobra.ExactArgs(1)`, no alias, because `delete application` has none) with its own `--team`/`-t` and `--dry-run` flags, and a shared `runDelete(kind string, team *string, dryRun *bool)` that builds the query container backend with `app.NewQueryContainerBackend` and calls `handler.DeleteResource` or `handler.DeleteApplication` by kind. `Short` and `Long` are the application texts with the kind word swapped and the host-port clause removed. `delete team` is untouched.

**Rationale**: constitution II: thin dispatcher, resource type before the name, `--team` optional, `--dry-run` present. The flags stay per subcommand rather than persistent on `delete` because `delete team` takes neither, and making them persistent would advertise flags `delete team` ignores.

**Alternatives considered**: adding `app`/`res` aliases to both delete subcommands was rejected for this ticket: the spec's assumption says mirroring `delete application` means no alias, and adding aliases to an existing verb is a separate decision for the owner. A persistent flag set on `delete` was rejected as above.

## R5. Where the integration scenarios live and which level covers ambiguity

**Decision**: `tests/integration/delete_test.go` (the delete suite the ticket names) gains `TestDeleteResource` on `newPinnedSuite` (the loopback registry world of T3, which deploys one pinned application and one pinned resource, `shrine-deploy-test.cache-pinned`), with scenarios for: retire a torn-down resource and pin afresh on the next deploy with the registry's newest moved; refusal while the container exists, state untouched; dry run prints the pin and the record and writes nothing; `--team` given and omitted both find the resource; a name nothing is held for is a soft success; and one scenario that runs all three delete verbs in one world and asserts each releases its pins (`delete application` → application pin gone and resource pin kept; `delete resource` → resource pin gone; redeploy, teardown, `delete team` → `Released 2 image pin(s)`). The ambiguity-across-teams case is covered at unit level (`TestDeleteResource_AmbiguousAcrossTeams`, in the shape of the application test), not in the integration suite.

**Rationale**: ticket T7 asks to "extend the `delete` suite with resource cases and a pin-release assertion for each delete verb"; `TestPinnedImagePolicy` already asserts the application and team releases, but the ticket's value is the assertion in one place for all three, so the delete suite gets the three-verb scenario. The pinned world has one team and one fixture writer; building a second team holding a resource of the same name would mean hand-writing `deployments.txt` lines in the test, which is brittle and tests the file format rather than the command. The application verb's ambiguity is likewise unit-only today, so the two verbs stay symmetrical. This refines spec FR-009's "team resolution and ambiguity" clause: team resolution is integration-tested, ambiguity is unit-tested; recorded as a deviation in the plan.

**Alternatives considered**: a new `tests/integration/delete_resource_test.go` was rejected because the ticket names the `delete` suite. Seeding state files for the ambiguity case was rejected as above.

## R6. Documentation touch points

**Decision**: `make docs-gen-cli` regenerates `docs/content/cli/delete.md` (new SEE ALSO line) and creates `docs/content/cli/delete_resource.md`. `AGENTS.md`: the CLI reference section becomes `### shrine delete application/resource <name>`, saying `delete resource` mirrors `delete application` without the host-port step; the `cmd/` tree gains a `delete.go` line (it has none today); the `pins.txt` state line already says "released by delete". `docs/content/reference/manifest-schema.md` line 261 ("Only three things release a pin: `shrine delete application <name>`, `shrine delete team <name>`, and a deploy …") becomes four things with `shrine delete resource <name>` added. No configuration key changes. `specs/progress.md` gains the 037 entry above the 036 entry. The two refinements (R3's kind guard, R5's test level) are recorded in `design.md` section 4.8 and the T7 list with the pull request.

**Rationale**: R-30 and the ticket's definition of done. The manifest reference sentence is the one place in the docs that enumerates the release paths and would be wrong once a third verb exists.

**Alternatives considered**: none of substance.
