# Contract: documentation, progress entry, and design refinements

**Feature**: 037-delete-resource

## `cmd/delete.go` help text, then `make docs-gen-cli`

The `Short` and `Long` of [operator-output.md](operator-output.md). Generated: `docs/content/cli/delete_resource.md` (new) and `docs/content/cli/delete.md` (SEE ALSO gains `shrine delete resource`). Both committed; the docs workflow's drift check requires it. `delete_application.md` and `delete_team.md` regenerate unchanged.

## `docs/content/reference/manifest-schema.md`, subsection Image pull policy

The sentence at the pin lifecycle paragraph, "Only three things release a pin: `shrine delete application <name>`, `shrine delete team <name>`, and a deploy of the artifact under `Always` or `IfNotPresent`.", becomes "Only four things release a pin: `shrine delete application <name>`, `shrine delete resource <name>`, `shrine delete team <name>`, and a deploy of the artifact under `Always` or `IfNotPresent`." Nothing else in the file changes.

## `docs/content/guides/publish-localhost.md`

Unchanged: it documents host ports, which resources do not hold.

## `AGENTS.md`

- CLI Reference: the heading `### shrine delete application <name>` becomes `### shrine delete application/resource <name>`. The paragraph keeps its application sentences and gains: "`shrine delete resource <name>` is the same verb for a resource: it releases the image pin and drops the deployment record (a resource holds no host port), refuses while the container exists, and takes the same `--team` and `--dry-run`. Both verbs release only a pin of their own kind." The closing sentence about `delete team` stays.
- Project Structure: add `│   ├── delete.go               # shrine delete team|application|resource <name> [--team] [--dry-run]` under `cmd/`, after `teardown.go`.
- State layout line for `pins.txt`: unchanged ("released by delete" already covers all three verbs).

## Configuration docs

No configuration key is added or changed; the ticket's "configuration docs" item is satisfied by this statement.

## `specs/progress.md`

One `- [x]` entry in the form of the 031 to 036 entries, inserted above the 036 entry, naming `specs/037-delete-resource/`, issue #58, the epic and ticket T7; the behaviour (`delete resource` with `--team` and `--dry-run`; `deleteArtifact` as the kind-generalised body behind `DeleteApplication` and `DeleteResource`; `DeleteOptions`; the kind-aware candidate search and pin read; no host-port step for resources; the three-verb pin-release scenario); the acceptance (SC-001 to SC-007 restated); the gates (`TestDeleteResource` in `tests/integration/delete_test.go` on the loopback registry; CI executes). It also records the two refinements below.

## `graphify update .`

Run after the code changes; the regenerated `graphify-out/` is committed with the pull request as the previous tickets did.

## `design.md` refinements to record with the pull request

1. Section 4.8 and T7-01: `findImagePin` reads a pin as the artifact's only when `pin.Kind` equals the requested kind, the guard the candidate search and the queries already apply; `Deployments.Remove` stays by name because one record, one pin, and one container exist per `team/name` (research R3).
2. T7-02 and the T7 integration scenarios: the ambiguity-across-teams case for `delete resource` is a unit test, as it is for `delete application`; the integration suite covers `--team` given and omitted, and the three-verb pin-release assertion lives in `TestDeleteResource` in the delete suite (research R5).

## Pull request

`/speckit-git-pr` with `Closes #58` in the Why section, the definition-of-done items listed, and the two refinements named; a `/shrine-pr-review` pass with no open finding; green CI.
