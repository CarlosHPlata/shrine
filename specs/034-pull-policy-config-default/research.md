# Research: A Configuration Default for the Image Pull Policy, and Manifests Generated to Match

**Feature**: 034-pull-policy-config-default | **Date**: 2026-10-06

No `NEEDS CLARIFICATION` markers existed in the Technical Context. The epic design settles every choice of substance; the entries below record how each one lands in this codebase as it is on `main` after T3 (#62), the three places where the design's wording needed refinement against the code, and the alternatives rejected.

## R1. The configuration key and its validation

**Decision**: `config.Config` gains `ImagePullPolicy string \`yaml:"imagePullPolicy,omitempty"\`` as a top-level field beside `SpecsDir`, `TeamsDir`, `Registries`, and `Plugins`. `Load` calls a new `(*Config).validateImagePullPolicy()` right after `validateSecretsPlugins()`: an empty value passes; otherwise `manifest.IsKnownPullPolicy` must accept it, else `imagePullPolicy: must be one of Always, IfNotPresent, Pinned`. `cmd/root.go` already wraps `Load` errors as `loading config: …` in `PersistentPreRunE`, so every command, including `generate` and `get`, fails before acting. Matching is exact, as `IsKnownPullPolicy` is.

**Rationale**: design section 3.2 and TD-10 (PRD OD-2). Validating in `Load` rather than in a caller-invoked method like `ValidateRegistries` is what the spec's FR-002 asks for: no command may act on a bad setting. `config` importing `manifest` introduces no cycle: `manifest` imports nothing under `internal/`, and the three values are already named once there (constitution VII).

**Alternatives considered**: duplicating the three strings in `config` to avoid the import was rejected as a DRY violation with a real drift risk. A caller-invoked `ValidateImagePullPolicy` like `ValidateRegistries` was rejected because `generate` and `get` would then run on an invalid setting, contradicting FR-002. Case-insensitive matching was rejected because the manifest field is exact and the two must agree.

## R2. Threading the default into the planner

**Decision**: the three call sites that pass `""` to `planner.Plan` pass the configuration value: `handler.DryRun` and `handler.Deploy` pass `cfg.ImagePullPolicy`, `handler.ApplySingle` passes `b.Cfg.ImagePullPolicy`. `deploy team` and team-scoped deploys go through `Deploy` with a filter, so no fourth site exists (verified by grep: `planner.Plan(` appears in `deploy.go` twice and `apply.go` once). `applyEffectivePullPolicy` and `EffectivePullPolicyWithDefault` are unchanged: manifest field, else default, else derived rule.

**Rationale**: design TD-7 and T4-02; the precedence is already implemented and the ticket only supplies the middle layer. `DryRun` dereferences `cfg.Registries` unconditionally today, so `cfg` is never nil there and no nil guard is added (constitution IV).

**Alternatives considered**: a `Config.DefaultPullPolicy()` accessor was rejected as an abstraction with one reader per call site and nothing to compute.

## R3. Knowing where the policy came from

**Decision**: `applyEffectivePullPolicy` records, per artifact, which layer supplied the effective policy, in a new unexported field on `planner.ManifestSet`: `pullPolicySources map[string]pullPolicySource`, keyed `<kind>/<name>`, with values `policyFromManifest`, `policyFromDefault`, `policyFromDerivedRule`, allocated lazily by `recordPullPolicySource`. `validateImagePolicies` asks `set.isPolicyFromDefault(kind, name)` when it builds a fixed-version error and picks the configuration-sourced ending when it is true. A set that was never normalised (hand-built in tests, or a future caller of `Resolve` alone) has a nil map and reads as manifest-sourced, which is also what it is: its field was set by whoever built it. The now unread `defaultPullPolicy` parameter is removed from `validateImagePolicies` and from `Resolve`; `Plan` keeps it and remains the only place the default enters.

**Rationale**: design section 4.5 says "the validation pass learns whether a policy came from the default". After normalisation a `Pinned` written by the operator and a `Pinned` filled from the configuration are the same string, so the only way to know is to remember at the moment of filling. Recording on the set keeps the signature of `validateImagePolicies` to the one thing it validates and removes a parameter that would otherwise be dead, which a reviewer would flag under constitution VII. The derived-rule source is recorded although nothing reads it today, because recording two of three sources and leaving the third implicit would make the map harder to read than it needs to be; it costs one assignment.

**Alternatives considered**: keeping the `defaultPullPolicy` parameter and inferring "from default" as "the field equals the default" was rejected because an operator who writes `Pinned` on the manifest while the default is also `Pinned` would get the configuration message for a field they set. A `yaml:"-"` source field on the manifest spec types was rejected because it leaks a planner concern into the manifest package and would ride along in every copy of the spec. Returning the source map from `applyEffectivePullPolicy` and passing it to `Resolve` was rejected because it changes `Resolve`'s signature anyway and separates the record from the set it describes.

## R4. The configuration-sourced messages

**Decision**: the three fixed-version errors keep their stem and change their ending by source. Manifest-sourced (T3, unchanged): `…is Pinned; use "<repo>" or "<repo>:latest"` for `spec.image` on either kind, `…is Pinned; omit it or use "latest"` for Resource `spec.version`. Configuration-sourced (new, both fields, both kinds): `…is Pinned (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default`. `fixedVersionImageError` and `fixedVersionResourceVersionError` gain a `fromDefault bool` and share `fixedVersionRemedy(fromDefault, manifestHint string)`. Errors are still appended to the planner's list and printed under `Validation errors:` with the set's other errors.

**Rationale**: design section 4.5 gives the Resource wording verbatim and says the parenthetical appears only for a configuration-sourced policy; T3's amendment kept the manifest-sourced Resource message to `omit it or use "latest"` and left the configuration wording to T4. PRD R-07 names exactly two ways out, add a policy or change the default, and the design agrees, so the Application-shaped configuration message takes the same two rather than the repository hint. The file name `config.yml` is the only name the configuration file has in every search location (README Configuration section), so naming it is safe.

**Observation, not a deviation**: PRD journey J7's "done when" names the ways out as "add a policy to the manifest or drop the fixed version", while R-07 and the design say "add a policy or change the default". The plan follows R-07 and the design; dropping the fixed version remains possible and is what the manifest reference's `Pinned` rules already explain.

**Alternatives considered**: a three-way message naming all of add a policy, drop the version, and change the default was rejected as longer than R-07 asks and inconsistent with the design text the owner approved.

## R5. Generate follows the default

**Decision**: `handler.AppOptions` and `handler.ResourceOptions` gain `PullPolicy string`; `cmd/generate.go` fills both from `cfg.ImagePullPolicy`. The image default leaves the command: `cmd` passes `Image: appImage` as typed (empty when the flag is absent) and `GenerateApp` calls `defaultAppImage(name, policy)` when it is empty: `<name>` under `Pinned`, `<name>:latest` otherwise. `GenerateResource` calls `defaultResourceVersion(policy)` when `Version` is empty: `""` under `Pinned`, `16` otherwise, and `versionLine(version)` returns `  version: "16"\n` or nothing, spliced into the skeleton where the fixed `version: "%s"` line is today. Neither skeleton writes `imagePullPolicy:`. An explicit `--image` or `--version` is written verbatim under every default (spec FR-011). The rendering is extracted into pure `renderAppSkeleton(opts) string` and `renderResourceSkeleton(opts) string` so unit tests check the text without touching the filesystem; `GenerateApp` and `GenerateResource` keep the directory, existence check, and write.

**Rationale**: design section 4.10 and T4-04; constitution II puts the defaulting logic in the handler, and the project rule that unit tests touch no filesystem forces the render-versus-write split. Omitting the version line rather than writing `version: latest` matches the design and the manifest reference's hand-written `Pinned` example.

**Alternatives considered**: refusing a fixed `--version` or a tagged `--image` at generate time under a `Pinned` default was rejected as new behaviour the design does not ask for; the next deploy already reports the contradiction with the configuration-sourced message. Rewriting a tagged explicit image to its repository was rejected because it silently alters what the operator typed.

## R6. The `--version` flag default

**Decision**: the `--version` flag of `generate resource` changes its Cobra default from `"16"` to `""` with the help text `Version of the resource (defaults to 16; omitted when imagePullPolicy in config.yml is Pinned)`; the `--image` help becomes `Docker image to run (defaults to [name]:latest, or [name] when imagePullPolicy in config.yml is Pinned)`. `make docs-gen-cli` regenerates `docs/content/cli/generate_resource.md` and `generate_application.md`; the docs workflow's drift check requires the regenerated pages to be committed.

**Rationale**: with a Cobra default of `"16"` the handler cannot tell an explicit `--version 16` from an omitted flag, and `cmd.Flags().Changed` in the command would put defaulting logic back where the constitution does not want it. The visible cost is that `--help` no longer prints `(default "16")` and says it in words instead.

**Alternatives considered**: keeping the Cobra default and passing `Changed("version")` to the handler was rejected as a second source of truth for one default. Writing `version: "16"` under `Pinned` and letting deploy reject it was rejected because R-08 requires the generated manifest to be valid under the effective default.

## R7. Documentation

**Decision**: per [contracts/docs-and-deviations.md](contracts/docs-and-deviations.md): the README Configuration section gains `imagePullPolicy: Pinned` in the example, a table row, and a paragraph on what a `Pinned` default does to manifests that name a fixed version and no policy, the two ways out, and the effect of changing the default on pins and on Resources without a version; the manifest reference's two `spec.imagePullPolicy` rows change their default column to name the configuration default before the derived rule, and the Image pull policy subsection states the three-layer precedence and quotes the configuration-sourced error beside the two manifest-sourced ones; `AGENTS.md` Config Directory Layout gains the key with a one-line comment; the two generate CLI pages are regenerated.

**Rationale**: PRD R-29 and the precedence clause of R-28; the ticket's definition of done; the repository's doc-and-code drift rule. The README is where the configuration file is documented today (there is no configuration page under `docs/content/reference/`), so that is where FR-013 lands.

## R8. Integration scenarios

**Decision**: one new file, `tests/integration/pull_policy_default_test.go`, two suites, both written before the implementation.

`TestPullPolicyDefaultPrecedence` uses `NewSuite` (no daemon): each case writes a `config.yml` into a temp config directory with the existing `writeConfig` helper, applies the team fixture, and runs `deploy --dry-run --config-dir <dir> --path <fixtures> --state-dir <state>`. The dry-run backend prints `[DOCKER] ImageResolve: name=<team>.<name> image=<image> policy=<policy> -> <decision>`, which makes the effective policy per artifact assertable without a container. Cases: no key with `versioned/` (derived rule: `policy=Always` for the `:latest` app, `policy=IfNotPresent` for the fixed-tag app); `Pinned` with `pinned-shape/` (`policy=Pinned -> would resolve newest and pin` for the untagged app and the no-version resource; the own-`Pinned` resource identical); `Pinned` with `versioned/` (fails naming the fixed-tag app's `spec.image` and the resource's `spec.version` with the configuration-sourced ending, both in one run; the `:latest` app and the own-`IfNotPresent` app are not named); `Pinned` with `own-pinned-fixed/` (the manifest-sourced message, no `from config.yml`); `IfNotPresent` with `versioned/` (`policy=IfNotPresent` for the `:latest` app, where the derived rule would say `Always`); `Always` with `versioned/` (`policy=Always` for the fixed-tag app); `IfNotPresent` with `pinned-shape/` (`spec.version is required` for the no-version resource that names no policy; the own-`Pinned` resource passes); an invalid value (`pinned`) with any command (`loading config: imagePullPolicy: must be one of Always, IfNotPresent, Pinned`, nothing else on stderr about manifests).

`TestPullPolicyDefaultGenerateThenDeploy` uses `NewDockerSuite` with `StartLocalRegistry` and `PushAs`, writes `imagePullPolicy: Pinned`, then runs `generate application whoami-gen --image <host>/shrine/whoami --port 80 --team shrine-deploy-test` and `generate resource cache-gen --type <host>/shrine/whoami --team shrine-deploy-test` into a specs directory, asserts the files have no `imagePullPolicy:` line, the application has the untagged image, and the resource has no `version:` line, then deploys and asserts `📌 Pinned shrine-deploy-test.whoami-gen at latest@` and the same for `cache-gen`, and a `pins.txt` with two lines. A final step rewrites `config.yml` to `imagePullPolicy: IfNotPresent`, removes the generated resource manifest (a Resource without `version` is invalid under a manifest-owned default, and one failing manifest fails the set), deploys again, and asserts the application resolved as manifest-owned and its line is gone from `pins.txt` (FR-007). A second generate case with no `--image` and no `--version` asserts the pure defaults (`image: whoami-gen`, no `version:`) by file content only, since `whoami-gen` is not pullable.

**Rationale**: ticket T4 names "the three defaults against fixture manifests" and "generate then deploy under the third value". Dry run is the cheapest faithful probe of the effective policy and needs no daemon, so most of the matrix runs on `NewSuite`. `spec.type` has no constraint beyond non-empty and the parser derives the Resource image as `<type>` when no version is set, so a registry repository as the type makes a generated resource pullable and pinnable without an image flag the command does not have. Per the project's rules, these are authored and compile-checked locally and run only in CI.

**Alternatives considered**: reusing `tests/testdata/pinned/` fixtures was rejected because every manifest there names `Pinned` itself, which is the one case the configuration default must not change. Running the precedence cases through a real deploy was rejected because the policy decision is visible in dry run and a daemon adds nothing to the assertion.

## R9. Unit tests

**Decision**: `internal/config/config_test.go` tests `validateImagePullPolicy` on struct literals (empty, each of the three, `pinned`, `Never`), no file. `internal/planner/policy_test.go` extends `TestApplyEffectivePullPolicy` with a source-recording table, extends `TestValidateImagePolicies` with the configuration-sourced shapes for Application image, Resource version, and Resource override, a case where the field is set and the default is also `Pinned` (manifest-sourced), and `IfNotPresent` and `Always` defaults; `TestPlan_NormalisesThePolicyIntoTheReturnedSet` gains a default case. `internal/handler/apps_test.go` and `resources_test.go` test the renderers under the four settings and with explicit values, and assert that a non-`Pinned` render equals today's skeleton text exactly. Existing `Resolve` call sites in planner tests drop the `""` argument.

**Rationale**: the project's rule that unit tests touch no filesystem; the planner tests already own the message shapes.

## R10. Hand-off notes for later tickets

- T5 (`get`, `describe`, `status`): the effective policy in the deployment record already reflects the configuration default, because normalisation happens before the record is written; nothing in T5 needs to read the configuration.
- T6 (`bump`): the manifest-sourced "no longer served" clause still points at the manifest; the configuration-sourced case has no separate wording in this ticket because that failure is about a pin, not a policy source.
- T8 (operator guide): the README paragraph written here is the seed for the guide's "house rule" section.
