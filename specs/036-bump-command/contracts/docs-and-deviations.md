# Contract: documentation, progress entry, and design refinements

**Feature**: 036-bump-command

## `cmd/bump.go` help text, then `make docs-gen-cli`

The `Short` and `Long` texts of [operator-output.md](operator-output.md). Generated pages: `docs/content/cli/bump.md`, `docs/content/cli/bump_application.md`, `docs/content/cli/bump_resource.md`. The docs workflow's drift check requires the generated pages to be committed; `bump.md` carries the `-v`, `-t`, `-p`, and `--dry-run` persistent flags in its Options block and the two subcommand pages inherit them.

## `docs/content/reference/manifest-schema.md`, subsection Image pull policy

1. After the paragraph that begins **The pin lifecycle.** and the "Only three things release a pin" paragraph, a new paragraph titled in bold **Moving a pin.** stating: `shrine bump application <name>` and `bump resource <name>` move a `Pinned` artifact to a chosen version (`-v 17`, `-v v1.4.0`, or an exact version `-v sha256:…`) or, without `-v`, to the newest; the repository always comes from the manifest; the bump verifies the version exists in the registry, records the new pin, prints the previous and the new version, and touches no container, so the next `shrine deploy` applies it; rolling back is a bump to the earlier version; a manifest-owned artifact (`Always`, `IfNotPresent`) is refused because its version is changed by editing the manifest; an artifact that was never deployed can be bumped so its first deploy runs the chosen version; `--dry-run` prints the reference that would be resolved and writes nothing. One short example block with the bump line and the `Bumped …` output.
2. The sentence "If the registry no longer serves a pinned exact version … names the artifact, the exact version, and the way out" ends instead with "and the `shrine bump` command to run".
3. In the **Reading what is pinned.** paragraph, the clause "a `Pinned:` that differs from `Running image:` is a pin that has been recorded but not yet deployed" gains "(the result of a `bump`)".

## `AGENTS.md`

- Quick Start block: add `shrine bump app my-api -v 1.4.0             # pin an app to a version; the next deploy applies it` after the status lines.
- CLI Reference: a new section `### shrine bump application/resource <name>` after `### shrine describe app/resource <name>`: aliases `app`/`res`; `-v` readable or exact version, newest without it; repository from the manifest; resolves and verifies in the registry immediately, records the pin, prints previous and new, touches no container, next deploy applies; refuses manifest-owned artifacts (names the policy) and unknown names (names the directory); works for undeployed artifacts; `--team/-t` verifies the owner (a name is unique per manifest directory); `--path/-p` as deploy; `--dry-run` prints and writes nothing.
- Project Structure: `│   ├── bump.go                 # shrine bump application|resource <name> [-v] [--dry-run]` under `cmd/`.
- The pipeline note that begins **Image resolution runs first and fails with zero changes.**: append "A `shrine bump` records a new pin through the same `ResolveImage` (`Repin` set, source `repinned`) without touching any container; a pinned exact version the registry no longer serves fails the pre-pass with a message naming the bump to run. See `specs/036-bump-command/`."
- State layout line for `pins.txt`: append "; replaced by `shrine bump`".

## Configuration docs

No configuration key is added or changed. The ticket's "configuration docs" item is satisfied by this statement; nothing under `docs/content` about `config.yml` changes.

## `specs/progress.md`

One `- [x]` entry in the form of the 031 to 035 entries, inserted above the 035 entry, naming `specs/036-bump-command/`, issue #57, the epic and ticket T6; the behaviour (the command and flags, the planning path through `Plan` with a by-name filter, `Repin` on `ResolveImageOp` and the `repinned` source with `pinReference` shared with the first deploy, the previous-and-new line, the dry-run path that builds nothing, the refusals, the reworded vanished-version message, `manifest.RepositoryOf`, `planManifestSet`); the one thing found while planning (a name is unique per manifest directory, so `--team` verifies and no ambiguity error exists); the acceptance criteria by SC id; and the gate `TestBump` (`tests/integration/bump_test.go`, loopback registry) plus the reworded assertion in `TestPinnedImagePolicy` (CI executes).

## `graphify update .`

Run after the code changes, per the repository rule; the regenerated `graphify-out/` is committed with the pull request as the previous tickets did.

## `design.md` refinements to record with the pull request

1. Section 4.6 step 1: the handler obtains the effective policy by running `planner.Plan` with `ByApp`/`ByResource` after `LoadDir`, through the helper `planManifestSet` shared with `Deploy` and `DryRun`, rather than calling `applyEffectivePullPolicy` directly (research R1).
2. Section 4.6 step 1 and R-21: a name is unique per manifest directory (`MergeManifest` rejects duplicates), so `--team` verifies `metadata.owner` and there is no ambiguity error for bump; a wrong team is reported as not found in that team (research R2).
3. Section 4.6 step 3: `manifest.RepositoryOf` is the one repository helper; the planner's and the backend's private copies are removed (research R3).
4. Section 4.2 point 4 and 4.6 step 4: `Repin` carries the unexpanded target; the backend expands it, emits the started event with the expanded target as `ref`, and records through `pinReference`, the renamed first-deploy branch; a `Repin` under a manifest-owned policy is refused by the backend (research R4).
5. Section 4.6: `--dry-run` runs through `handler.BumpDryRun` without a bundle, so no Docker client is built and no log opened, as `deploy --dry-run` does (research R5).
6. Section 4.2 point 3: the vanished-version message's second clause now reads `run "shrine bump <kind> <name>" to choose another version`, with the pull cause appended as T3 did (research R7).
7. Section 5 and the T5 refinement 6: the "pin differs from the running image" scenario may now use `bump`; the T5 suite is left as is because it passes and edits to passing suites are not owed.
