# Contract: documentation and design refinements

**Feature**: 034-pull-policy-config-default

## `README.md`, section Configuration

- Example block gains, after `teamsDir`, `imagePullPolicy: Pinned   # optional default for manifests that name no policy`.
- Field table gains a row: `` `imagePullPolicy` `` | `Default image pull policy for manifests that name no spec.imagePullPolicy: Always, IfNotPresent, or Pinned. When absent, Shrine derives one per image: Always for latest or no tag, IfNotPresent otherwise. The manifest's own field always wins.`
- A paragraph after the table, titled in bold **Making pinning the house rule**: setting `imagePullPolicy: Pinned` pins every manifest that names no policy; the next deploy rejects, before any change, every such manifest that still names a fixed version, with a message that names this setting and the two ways out (set `spec.imagePullPolicy` on that manifest, or change the default); generated manifests follow the default (`shrine generate application` writes the bare repository, `shrine generate resource` writes no `version`); changing the default later is as consequential as editing every manifest that names no policy: artifacts it pinned are released on their next deploy, and a Resource that relied on `Pinned` to omit its version must name one again. Links to the manifest reference's Image pull policy subsection.

## `docs/content/reference/manifest-schema.md`

- Resource and Application `spec.imagePullPolicy` rows: the Default column becomes `` the `imagePullPolicy` default in `config.yml` when set; else `Always` for `:latest` or no tag, `IfNotPresent` otherwise ``.
- Image pull policy subsection, first paragraph: replace "when absent, Shrine derives one from the image reference" with the three-layer order: the manifest's own field; else the `imagePullPolicy` default in `config.yml`; else the derived rule. One sentence that a `Pinned` default holds every manifest that names no policy to the rules below.
- The error block gains the configuration-sourced line after the two manifest-sourced ones: `resource "db": spec.version "16" names a fixed version but the image pull policy is Pinned (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default`, introduced by one sentence saying the ending names the setting when the policy came from it.

## `AGENTS.md`, Config Directory Layout

- The example `config.yml` gains, after `specsDir`, the line `imagePullPolicy: Pinned                 # optional default pull policy for manifests that name none: Always | IfNotPresent | Pinned; absent = derived rule`.
- The `config.yml` comment in the tree (`# registry credentials, specsDir, gateway IP`) gains `, image pull policy default`.

## `docs/content/cli/`

`make docs-gen-cli` regenerates `generate_application.md` and `generate_resource.md` from the changed flag help; the docs workflow's drift check fails otherwise. No other page changes.

## `specs/progress.md`

One `- [x]` entry in the form of the 031 to 033 entries, naming the spec directory, issue #55, the epic and ticket T4, the behaviour (key, precedence, configuration-sourced message, generate defaults, flag change), the acceptance criteria by SC id, and the gates `TestPullPolicyDefaultPrecedence` and `TestPullPolicyDefaultGenerateThenDeploy` (CI executes).

## `design.md` refinements to record with the pull request

1. Section 4.5: `validateImagePolicies` reads the policy's source from a record `applyEffectivePullPolicy` leaves on the `ManifestSet`; the `defaultPullPolicy` parameter is removed from `validateImagePolicies` and `Resolve` (research R3).
2. Section 4.5: the configuration-sourced Application-shaped message ends `set spec.imagePullPolicy on the manifest or change the default`, like the Resource message; the manifest-sourced messages keep T3's endings (research R4).
3. Section 4.10 and the section 1 Generate templates row: the image default moves from `cmd/generate.go` into `handler.GenerateApp`; the `--version` flag default becomes `""` and `handler.GenerateResource` fills `16` unless the default is `Pinned` (research R5, R6).
