# Tasks: A Configuration Default for the Image Pull Policy, and Manifests Generated to Match

**Input**: Design documents from `/specs/034-pull-policy-config-default/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: INCLUDED. The constitution mandates TDD (Principle V: integration test files are created before the implementation code) and the ticket's definition of done requires the integration scenarios to be written before the implementation, compiled under the integration tag, and never run locally. Unit tests touch no filesystem: config validation is tested on struct literals, the planner on hand-built sets, and the generate skeletons through pure render functions.

**Organization**: Phase 1 creates the three fixture directories. Phase 2 holds the integration scenarios (written first) and the one seam every story needs: the configuration key with its validation. Phases 3 to 7 follow the spec's five user stories in priority order. The ticket is delivered as one pull request, so stories land as commits on the same branch.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label (US1 to US5); Setup, Foundational, and Polish tasks carry none

## Phase 1: Setup

**Purpose**: Confirm a green baseline and create the on-disk fixtures the scenarios reference

- [x] T001 Verify green baseline on branch `034-pull-policy-config-default`: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/...` pass with no file changes; confirm `specs/epics/pinned-image-versions/` is present and `main` holds #62 (`git log --oneline -1 origin/main`)
- [x] T002 [P] Create fixture `tests/testdata/pull-policy-default/versioned/` for owner `shrine-deploy-test`, port 80 on Applications, no `imagePullPolicy` unless stated: `app-latest.yml` (Application `app-latest`, image `traefik/whoami:latest`); `app-fixed.yml` (Application `app-fixed`, image `traefik/whoami:v1.10.2`); `res-fixed.yml` (Resource `res-fixed`, type `cache`, image `traefik/whoami`, version `"16"`); `app-own-ifnotpresent.yml` (Application `app-own-ifnotpresent`, image `traefik/whoami:v1.10.1`, `imagePullPolicy: IfNotPresent`)
- [x] T003 [P] Create fixture `tests/testdata/pull-policy-default/pinned-shape/` for owner `shrine-deploy-test`: `app-untagged.yml` (Application `app-untagged`, image `traefik/whoami`, port 80, no policy); `res-noversion.yml` (Resource `res-noversion`, type `cache`, image `traefik/whoami`, no version, no policy); `res-own-pinned.yml` (Resource `res-own-pinned`, type `cache`, image `traefik/whoami`, no version, `imagePullPolicy: Pinned`)
- [x] T004 [P] Create fixture `tests/testdata/pull-policy-default/own-pinned-fixed/` for owner `shrine-deploy-test`: `app-own-pinned-fixed.yml` (Application `app-own-pinned-fixed`, image `traefik/whoami:v1.10.2`, port 80, `imagePullPolicy: Pinned`)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The integration scenarios, written first; then the configuration key with its validation, which every story reads

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

### Integration scenarios (write FIRST; compile only, never run locally)

- [x] T005 Create `tests/integration/pull_policy_default_test.go` (`//go:build integration`, package `integration_test`) with `TestPullPolicyDefaultPrecedence` on `NewSuite(t)` per research R8: a helper `policyFixturesPath(parts ...string)` modelled on `pinnedFixturesPath` pointing at `tests/testdata/pull-policy-default`; a helper `configDirWithPolicy(t, tc, value string) string` that writes `imagePullPolicy: <value>\n` (or an empty file when `value == ""`) into `tc.Path("config")` with the existing `writeConfig` helper from `traefik_plugin_test.go`; BeforeEach sets `tc.StateDir = tc.Path("state")` and applies `fixturesPath("team")` with `apply teams --path … --state-dir …`; a helper `dryRunWithDefault(tc, cfgDir, fixtureDir string) *TestCase` running `deploy --dry-run --config-dir <cfgDir> --path <fixtureDir> --state-dir <state>`. Cases, asserting on the `[DOCKER] ImageResolve: name=shrine-deploy-test.<name> image=<image> policy=<policy> -> <decision>` lines and stderr: (1) no key, `versioned/`: success, `policy=Always -> manifest-owned` for `app-latest`, `policy=IfNotPresent -> manifest-owned` for `app-fixed`, `res-fixed`, and `app-own-ifnotpresent`; (2) `Pinned`, `pinned-shape/`: success, `policy=Pinned -> would resolve newest and pin` for `app-untagged`, `res-noversion`, `res-own-pinned`; `tc.AssertFileNotExists(filepath.Join(tc.StateDir, testTeam, "pins.txt"))`; (3) `Pinned`, `versioned/`: failure, stderr contains both configuration-sourced lines of contracts/operator-output.md for `app-fixed` (`spec.image "traefik/whoami:v1.10.2"`) and `res-fixed` (`spec.version "16"`) ending `(from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default`, and does not contain `"app-latest"` or `"app-own-ifnotpresent"`; (4) `Pinned`, `own-pinned-fixed/`: failure, stderr contains `application "app-own-pinned-fixed": spec.image "traefik/whoami:v1.10.2" names a fixed version but the image pull policy is Pinned; use "traefik/whoami" or "traefik/whoami:latest"` and does not contain `from config.yml`; (5) `IfNotPresent`, `versioned/`: success, `policy=IfNotPresent -> manifest-owned` for `app-latest`; (6) `Always`, `versioned/`: success, `policy=Always -> manifest-owned` for `app-fixed`; (7) `IfNotPresent`, `pinned-shape/`: failure, stderr contains `resource "res-noversion": spec.version is required` and does not contain `"res-own-pinned"`; (8) value `pinned`, run `get deployed --config-dir <cfgDir> --state-dir <state>`: failure, stderr contains `loading config: imagePullPolicy: must be one of Always, IfNotPresent, Pinned` and does not contain `Validation errors`
- [x] T006 In the same file add `TestPullPolicyDefaultGenerateThenDeploy` on `NewDockerSuite(t, testTeam)` per research R8: BeforeEach as `newPinnedSuite` (state dir, `SeedSubnetState`, team fixture, `StartLocalRegistry`, `PushAs(tc, "traefik/whoami:v1.10.1", "shrine/whoami:latest")`), plus a config dir with `imagePullPolicy: Pinned` and an empty specs dir. Case A, "generate then deploy under Pinned": run `generate application whoami-gen --config-dir <cfg> --path <specs> --team shrine-deploy-test --image <registry.Host>/shrine/whoami --port 80` and `generate resource cache-gen --config-dir <cfg> --path <specs> --team shrine-deploy-test --type <registry.Host>/shrine/whoami`, both `AssertSuccess`; read both files and assert `whoami-gen.yml` contains `image: <host>/shrine/whoami\n` and neither file contains `imagePullPolicy` or `version:`; run `deploy --config-dir <cfg> --path <specs> --state-dir <state>` and assert success, `📌 Pinned shrine-deploy-test.whoami-gen at latest@`, `📌 Pinned shrine-deploy-test.cache-gen at latest@`, and two non-empty lines in `<state>/shrine-deploy-test/pins.txt` (reuse `nonEmptyLines` and `readFileOrEmpty` from the pinned suite); then rewrite the config to `imagePullPolicy: IfNotPresent`, remove `cache-gen.yml`, deploy again, and assert success, `AssertOutputNotContains("📌 Using pinned shrine-deploy-test.whoami-gen")`, and that `pins.txt` has no line containing `whoami-gen` (FR-007). Case B, "generate with no image or version flag writes the Pinned defaults": run both generate commands without `--image`, `--type`, or `--version` for names `web` and `db`, assert `web.yml` contains `image: web\n` and `db.yml` contains `type: postgres\n` directly followed by `networking:` on the next non-blank line and no `version:`; no deploy. AfterEach: `teardown` the team as `pinnedTeardown` does, remove the registry container as the pinned suite's AfterEach does
- [x] T007 Compile the suite: `go vet -tags integration ./tests/integration/...` is green (unused imports and helper signatures fixed here, never by running the suite). Confirm `git diff --stat main -- tests/integration/` shows only the new file

### The configuration key (test first, then code)

- [x] T008 Unit test in `internal/config/config_test.go`: `TestValidateImagePullPolicy` as a table over `Config{ImagePullPolicy: v}.validateImagePullPolicy()`: `""`, `Always`, `IfNotPresent`, `Pinned` return nil; `pinned`, `Never`, `Pinned ` (trailing space) return an error whose text is exactly `imagePullPolicy: must be one of Always, IfNotPresent, Pinned`. No file is read
- [x] T009 In `internal/config/config.go` add `ImagePullPolicy string \`yaml:"imagePullPolicy,omitempty"\`` to `Config` between `TeamsDir` and `Plugins`; add `func (c *Config) validateImagePullPolicy() error` (empty → nil; `manifest.IsKnownPullPolicy(c.ImagePullPolicy)` → nil; else `fmt.Errorf("imagePullPolicy: must be one of Always, IfNotPresent, Pinned")`), importing `github.com/CarlosHPlata/shrine/internal/manifest`; call it in `Load` right after `validateSecretsPlugins` (T008 passes). Confirm no import cycle: `go build ./...`
- [x] T010 Green checkpoint: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/... && gofmt -l .` all clean; commit

**Checkpoint**: the key is read and validated on every command; nothing reads it yet

---

## Phase 3: User Story 1 - Make pinning the house rule with one line of configuration (Priority: P1) 🎯 MVP

**Goal**: the three planning handlers pass the configuration default into `planner.Plan`, so a manifest that names no policy is pinned under `imagePullPolicy: Pinned` and a manifest with its own field is untouched

**Independent Test**: scenario (2) of T005 and case A of T006 (CI); locally, the planner unit test with a `Pinned` default and quickstart step 3

### Tests for User Story 1 (write FIRST; must fail before T012)

- [ ] T011 [P] [US1] Unit tests in `internal/planner/policy_test.go`: extend `TestPlan_NormalisesThePolicyIntoTheReturnedSet` (or add `TestPlan_AppliesTheConfigurationDefault`) so that `Plan(set, …, "Pinned")` over `policySet("web", "", "cache", "", "traefik/whoami", "")` returns a set whose Application and Resource both carry `Pinned`, and over `policySet("web:1.2", "IfNotPresent", …)` leaves the Application's `IfNotPresent` alone; `Plan(set, …, "")` over an untagged image still yields `Always` (T3 behaviour unchanged)
- [ ] T012 [US1] Thread the default in the three handlers: `internal/handler/deploy.go` passes `cfg.ImagePullPolicy` in `DryRun` and `b.Cfg.ImagePullPolicy` in `Deploy`; `internal/handler/apply.go` passes `b.Cfg.ImagePullPolicy` in `ApplySingle`; no other call site exists (`grep -rn 'planner.Plan(' internal cmd` shows three). T011 passes because `Plan` already applies the default
- [ ] T013 [US1] Build the binary into the scratchpad (`go build -o /tmp/claude-0/…/shrine .`), run quickstart step 3 against `tests/testdata/pull-policy-default/pinned-shape` with `imagePullPolicy: Pinned` and confirm the three `policy=Pinned -> would resolve newest and pin` lines and no `pins.txt`; run quickstart step 1 with no config file and confirm the derived-rule lines are unchanged from `main`
- [ ] T014 [US1] `go test ./...` green; `gofmt -l .` clean; commit

**Checkpoint**: one line of configuration pins every manifest that names no policy; the manifest field still wins

---

## Phase 4: User Story 2 - Existing manifests that contradict the house rule fail loudly and say what to do (Priority: P2)

**Goal**: the planner remembers where each artifact's policy came from and ends the fixed-version error with the configuration-sourced remedy when it came from the default

**Independent Test**: scenarios (3) and (4) of T005 (CI); locally, the planner unit tests and quickstart step 4

### Tests for User Story 2 (write FIRST; must fail before T016)

- [ ] T015 [P] [US2] Unit tests in `internal/planner/policy_test.go`: (a) source recording: after `applyEffectivePullPolicy(set, "Pinned")` over a set with one declared `IfNotPresent` application, one undeclared application, and one undeclared resource, `set.isPolicyFromDefault("application", <declared>)` is false and is true for the two undeclared; after `applyEffectivePullPolicy(set, "")` it is false for all; on a freshly built set it is false (nil map); (b) extend `TestValidateImagePolicies` to build sets through `applyEffectivePullPolicy(set, dflt)` before validating and assert byte-exact per contracts/operator-output.md: default `Pinned`, undeclared application `repo:1.2` → `application "x": spec.image "repo:1.2" names a fixed version but the image pull policy is Pinned (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default`; default `Pinned`, undeclared resource version `16` → `resource "db": spec.version "16" names a fixed version but the image pull policy is Pinned (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default`; default `Pinned`, undeclared resource with override `postgres:16` and no version → the resource `spec.image` message with the same ending, exactly one error; default `Pinned`, application declaring `Pinned` with `repo:1.2` → the T3 message ending `; use "repo" or "repo:latest"` (manifest-sourced, no `from config.yml`); default `Pinned`, resource declaring `Pinned` with version `16` → `; omit it or use "latest"`; (c) the existing T3 cases still pass with `validateImagePolicies(set)` (one argument)
- [ ] T016 [US2] Implement per data-model sections 2.1, 2.3, 2.4: in `internal/planner/loader.go` add `pullPolicySources map[string]pullPolicySource` to `ManifestSet` (unexported; `NewManifestSet` leaves it nil); in `internal/planner/policy.go` add `type pullPolicySource int` with `policyFromManifest`, `policyFromDefault`, `policyFromDerivedRule`, `(s *ManifestSet) recordPullPolicySource(kind, name string, src pullPolicySource)` (allocates the map lazily, key `kind + "/" + name`), `(s *ManifestSet) isPolicyFromDefault(kind, name string) bool` (false on a nil map), a private `pullPolicySourceOf(declared, dflt string) pullPolicySource`; make `applyEffectivePullPolicy` record the source for every application (`"application"`) and resource (`"resource"`) in its existing loops; change `validateImagePolicies(set *ManifestSet) []error` to drop the parameter and pass `set.isPolicyFromDefault(kind, name)` into `fixedVersionImageError(kind, name, image string, fromDefault bool)` and `fixedVersionResourceVersionError(name, version string, fromDefault bool)` (threading through `validateResourceVersion(set, res)`); add `fixedVersionRemedy(fromDefault bool, manifestHint string) string` returning ` (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default` or `; ` + manifestHint; rewrite the two error constructors as stem + remedy with the stems unchanged; delete the T3 comment saying the parameter is unused until T4 (T015 passes)
- [ ] T017 [US2] Remove the dead parameter: `Resolve(set, store, registries)` in `internal/planner/resolve.go` calls `validateImagePolicies(set)`; `Plan` in `internal/planner/plan.go` calls `Resolve(set, store, registries)`; drop the trailing `""` from every `Resolve(` call in `internal/planner/*_test.go` (`grep -rn 'Resolve(' internal/planner --include='*_test.go'`); `go build ./... && go vet ./...` clean
- [ ] T018 [US2] Run quickstart step 4 with the scratchpad binary: `versioned/` under `Pinned` prints exactly the two configuration-sourced lines for `app-fixed` and `res-fixed` and names neither `app-latest` nor `app-own-ifnotpresent`; `own-pinned-fixed/` prints the manifest-sourced line without `from config.yml`
- [ ] T019 [US2] `go test ./...` green; `gofmt -l .` clean; commit

**Checkpoint**: a house rule of `Pinned` names every offending manifest, the field, the setting, and the two ways out, in one run

---

## Phase 5: User Story 3 - The other two values work as a default too, and no setting changes nothing (Priority: P3)

**Goal**: `IfNotPresent` and `Always` flow through the middle layer with today's semantics, the version-required rule keeps applying under them, and an invalid value is refused by every command

**Independent Test**: scenarios (1), (5), (6), (7), (8) of T005 (CI); locally, the planner unit tests and quickstart steps 1, 2, and 5

### Tests for User Story 3 (write FIRST; T020 should pass against the Phase 2 to 4 code if the precedence is right, which is the point of writing it)

- [ ] T020 [P] [US3] Unit tests in `internal/planner/policy_test.go`: `applyEffectivePullPolicy(set, "IfNotPresent")` gives an undeclared `web:latest` application `IfNotPresent` (where `""` gives `Always`) and records `policyFromDefault`; `applyEffectivePullPolicy(set, "Always")` gives an undeclared `web:1.2` application `Always`; a declared `Pinned` resource under default `IfNotPresent` stays `Pinned` and, with no version, produces no error; an undeclared resource with no version under default `IfNotPresent` and under default `Always` produces exactly `resource "db": spec.version is required`
- [ ] T021 [US3] Run quickstart steps 1, 2, and 5 with the scratchpad binary: no config file → derived-rule lines; `imagePullPolicy: pinned` → `get deployed` fails with `Error: loading config: imagePullPolicy: must be one of Always, IfNotPresent, Pinned` and nothing else; `IfNotPresent` → `policy=IfNotPresent` for `app-latest`, and `pinned-shape/` fails only on `res-noversion`. Fix any divergence in `internal/planner/policy.go` or `internal/config/config.go`; T020 and the Phase 2 to 4 tests stay green
- [ ] T022 [US3] `go test ./...` green; `gofmt -l .` clean; commit

**Checkpoint**: all three values work as a default; an absent key reproduces T3 exactly; a bad key stops every command

---

## Phase 6: User Story 4 - Generated manifests follow the house rule (Priority: P4)

**Goal**: `generate application` and `generate resource` take their image and version defaults from the configuration default in the handler, write no policy line, and honour explicit flags verbatim; the CLI pages are regenerated

**Independent Test**: case A and case B of T006 (CI); locally, the renderer unit tests and quickstart step 6

### Tests for User Story 4 (write FIRST; must fail before T025 and T026)

- [ ] T023 [P] [US4] Create `internal/handler/apps_test.go` (package `handler`, no filesystem): `TestRenderAppSkeleton` table over `renderAppSkeleton(AppOptions{Name: "web", Team: "t", Port: 8080, Replicas: 1, Domain: "web.shrine.lab", PathPrefix: "/web", Image: img, PullPolicy: pol})`: `Image ""` with `PullPolicy` `""`, `Always`, `IfNotPresent` → output contains `  image: web:latest\n`; `Image ""` with `Pinned` → `  image: web\n`; `Image "ghcr.io/me/web:1.2"` with `Pinned` → `  image: ghcr.io/me/web:1.2\n`; every output lacks `imagePullPolicy`; the `""` policy output equals `fmt.Sprintf(appSkeleton, "web", "t", "web:latest", 8080, 1, "web.shrine.lab", "/web", false)` byte for byte. `TestDefaultAppImage`: `("web", "Pinned")` → `web`, `("web", "")` → `web:latest`, `("web", "IfNotPresent")` → `web:latest`
- [ ] T024 [P] [US4] Create `internal/handler/resources_test.go` (package `handler`, no filesystem): `TestRenderResourceSkeleton` table over `renderResourceSkeleton(ResourceOptions{Name: "db", Team: "t", Type: "postgres", Version: v, PullPolicy: pol})`: `Version ""` with `""`, `Always`, `IfNotPresent` → contains `  type: postgres\n  version: "16"\n  networking:\n`; `Version ""` with `Pinned` → contains `  type: postgres\n  networking:\n` and not `version:`; `Version "16"` with `Pinned` → contains `  version: "16"\n`; `Version "latest"` with `Pinned` → `  version: "latest"\n`; every output lacks `imagePullPolicy` and keeps the commented `env`/`outputs` guidance; the `""` policy output equals the pre-change skeleton text byte for byte (copy today's `resourceSkeleton` literal into the test as `legacyResourceSkeleton` and compare `fmt.Sprintf(legacy, "db", "t", "postgres", "16", false)`). `TestDefaultResourceVersion`: `Pinned` → `""`, `""` → `16`, `Always` → `16`. `TestVersionLine`: `""` → `""`, `"16"` → `  version: "16"\n`
- [ ] T025 [US4] Implement in `internal/handler/apps.go`: `AppOptions.PullPolicy string`; `defaultAppImage(name, pullPolicy string) string` (`name` when `pullPolicy == manifest.ImagePullPolicyPinned`, else `name + ":latest"`); `renderAppSkeleton(opts AppOptions) string` that applies `defaultAppImage` when `opts.Image == ""` and returns the `fmt.Sprintf(appSkeleton, …)` text; `GenerateApp` keeps `MkdirAll`, the existence check, and `WriteFile`, writing `renderAppSkeleton(opts)`. In `internal/handler/resources.go`: `ResourceOptions.PullPolicy string`; `defaultResourceVersion(pullPolicy string) string` (`""` under `Pinned`, else `"16"`); `versionLine(version string) string` (`""` when empty, else `fmt.Sprintf("  version: %q\n", version)`); change the `resourceSkeleton` literal so the fixed `  version: "%s"\n` line becomes a `%s` placeholder fed by `versionLine`; `renderResourceSkeleton(opts ResourceOptions) string` applies `defaultResourceVersion` when `opts.Version == ""`; `GenerateResource` writes `renderResourceSkeleton(opts)` (T023, T024 pass)
- [ ] T026 [US4] In `cmd/generate.go`: delete the `if image == "" { image = name + ":latest" }` default and pass `Image: appImage` as typed; pass `PullPolicy: cfg.ImagePullPolicy` on both `AppOptions` and `ResourceOptions`; change the `--image` help to `Docker image to run (defaults to [name]:latest, or [name] when imagePullPolicy in config.yml is Pinned)`; change the `--version` flag to `StringVar(&resVersion, "version", "", "Version of the resource (defaults to 16; omitted when imagePullPolicy in config.yml is Pinned)")`; `go build ./...`
- [ ] T027 [US4] Regenerate the CLI pages: `make docs-gen-cli`; confirm `git status` shows only `docs/content/cli/generate_application.md` and `docs/content/cli/generate_resource.md` changed under `docs/content/cli/`, with the two new help lines and no `(default "16")`; run `make docs-check` if Hugo is installed, otherwise `cd docs/tools/docsgen && go test ./...`
- [ ] T028 [US4] Run quickstart step 6 with the scratchpad binary: under `Pinned`, `web.yml` has `image: web` and `db.yml` has no `version:` line and neither has `imagePullPolicy`; with the config removed, `image: web:latest` and `version: "16"`; `generate resource --help` shows the new `--version` line
- [ ] T029 [US4] `go test ./...` green; `gofmt -l .` clean; commit

**Checkpoint**: the first manifest a house-rule user generates deploys without edits; today's skeletons are byte-identical when the key is absent

---

## Phase 7: User Story 5 - The documentation explains the setting and the precedence (Priority: P5)

**Goal**: the README configuration section, the manifest reference, and the contributor reference describe the key, its values, the absent case, the three-layer precedence, and the consequences of a `Pinned` default

**Independent Test**: read the three documents against spec SC-007

- [ ] T030 [P] [US5] Update `README.md` section Configuration per contracts/docs-and-deviations.md: add `imagePullPolicy: Pinned   # optional default for manifests that name no policy` after `teamsDir` in the example; add the `imagePullPolicy` row to the field table after `teamsDir`; add the bold **Making pinning the house rule** paragraph after the table covering what the default does, the loud failure with the two ways out, generate following the default, and the effect of changing the default on pins and on Resources without a version, linking to `docs/content/reference/manifest-schema.md#image-pull-policy` (use the relative link form the README already uses for docs pages)
- [ ] T031 [P] [US5] Update `docs/content/reference/manifest-schema.md` per contracts/docs-and-deviations.md: both `spec.imagePullPolicy` rows' Default cells become ``the `imagePullPolicy` default in `config.yml` when set; else `Always` for `:latest` or no tag, `IfNotPresent` otherwise``; the Image pull policy subsection's first paragraph states the order (manifest field, else the `imagePullPolicy` default in `config.yml`, else the derived rule) and that a `Pinned` default holds every manifest naming no policy to the rules below; the error block gains the configuration-sourced `resource "db"` line of contracts/operator-output.md after the two existing lines, introduced by one sentence saying the ending names the setting when the policy came from it
- [ ] T032 [P] [US5] Update `AGENTS.md` Config Directory Layout: the tree comment on `config.yml` gains `, image pull policy default`; the example `config.yml` gains `imagePullPolicy: Pinned                 # optional default pull policy for manifests that name none: Always | IfNotPresent | Pinned; absent = derived rule` after `specsDir`
- [ ] T033 [US5] Run `bash scripts/lint-docs-frontmatter.sh docs/content` and `bash scripts/check-md-shape.sh docs/public` if `docs/public` exists after `make docs-build`, else skip the shape check; read the three documents and confirm a reader can answer the five questions of spec SC-007 from the text alone; commit

**Checkpoint**: the key is documented where the configuration file is documented, and the precedence is stated once in the manifest reference

---

## Phase 8: Polish & Cross-Cutting Concerns

- [ ] T034 Record the three refinements of contracts/docs-and-deviations.md in `specs/epics/pinned-image-versions/design.md` as "*Amended by T4 (spec 034)*" notes: section 4.5 (source record on the set; parameter removed from `validateImagePolicies` and `Resolve`; the configuration-sourced Application ending), section 4.10 and the section 1 Generate templates row (defaults move into the handler; `--version` default `""`); do not reopen TD-1 to TD-13
- [ ] T035 Add the entry for 034 to `specs/progress.md` in the form of the 031 to 033 entries: title, spec link, issue #55, epic and ticket T4, what changed (key and validation in `Load`, the three handlers thread it, the source record and configuration-sourced message, generate defaults and the flag change, docs), acceptance SC-001 to SC-007 mapping, gates `TestPullPolicyDefaultPrecedence` and `TestPullPolicyDefaultGenerateThenDeploy` (CI executes)
- [ ] T036 Run `graphify update .` and commit the refreshed `graphify-out/`
- [ ] T037 Final verification: `go build ./... && go test ./... && go vet -tags integration ./tests/integration/... && gofmt -l .` clean; `make docs-gen-cli` produces no further diff; `git diff --stat main -- tests/integration/` shows only `pull_policy_default_test.go` added and no existing suite edited; `grep -rn '"github.com/CarlosHPlata/shrine/internal' tests/integration/` still finds nothing (integration tests stay isolated from internal packages); every task above checked; `git status` clean after commit
- [ ] T038 Rebase onto `origin/main` if it moved (expected conflicts only in `specs/progress.md`, `.specify/feature.json`, `CLAUDE.md`, and `graphify-out/`; keep both entries), push `034-pull-policy-config-default`, open the pull request with `/speckit-git-pr` following `.github/pull_request_template.md`: `Closes #55` in Why, the definition-of-done list of the issue, the `--version` flag default change and the removed `Resolve` parameter named explicitly, the three refinements, and the hand-off notes of research R10
- [ ] T039 Run `/shrine-pr-review` on the pull request, fix every real finding, push again, confirm CI is green (the integration job runs both new suites; the docs job runs the CLI drift check)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies; T002 to T004 in parallel
- **Foundational (Phase 2)**: depends on Phase 1 (the scenarios reference the fixtures); T005 before T006 (same file); T006 before T007; T008 before T009; T007 and T009 before T010
- **US1 (Phase 3)**: depends on Phase 2; T011 before T012; T013 after T012
- **US2 (Phase 4)**: depends on Phase 3 (the handlers must pass the default for the quickstart check to show configuration-sourced messages); T015 before T016; T016 before T017; T018 after T017
- **US3 (Phase 5)**: depends on Phase 4 (T020 asserts source recording); T020 before T021
- **US4 (Phase 6)**: depends on Phase 2 only (reads `cfg.ImagePullPolicy`); T023 and T024 before T025; T025 before T026; T026 before T027; T028 after T027
- **US5 (Phase 7)**: depends on Phase 6 for the flag help the README may quote; otherwise independent; T030 to T032 in parallel; T033 last
- **Polish (Phase 8)**: depends on everything above; T034 to T036 before T037; T037 before T038; T038 before T039

### Parallel Opportunities

- T002, T003, T004
- T008 can be written while T005 and T006 are being written (different files)
- T011 and T015 and T020 are all in `policy_test.go` and must be sequential; T023 and T024 are in different files and can run in parallel with each other and with Phase 3 to 5 work
- US4 can proceed in parallel with US1 to US3 once Phase 2 is done; US5's three documentation tasks can run in parallel with each other

---

## Implementation Strategy

### MVP First (User Story 1)

1. Phase 1, then Phase 2: fixtures, the integration suite first, then the configuration key with its validation.
2. Phase 3: pass the default from the three handlers. At this point one line of configuration pins every manifest that names no policy, which is journey J7's core; scenario (2) and case A of the suite are satisfiable.

### Incremental Delivery

- US2 makes the failure under the house rule name the setting and the ways out; US3 proves the other two values and the compatibility promise; US4 makes generate follow the rule; US5 documents. Each lands as its own commit on the same branch; the pull request carries all of them, per the ticket.

## Notes

- Integration scenarios are compiled (`go vet -tags integration`) and never run locally; another agent shares the daemon. The precedence suite needs no daemon at all and still runs only in CI.
- Unit tests touch no filesystem: config validation on struct literals, planner rules on hand-built sets, generate skeletons through `renderAppSkeleton` and `renderResourceSkeleton`; the file writes stay covered by the integration suite.
- Decisions TD-1 to TD-13 are settled; a disagreement goes to issue #55, not into the code.
- Every commit message ends with `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`.
- Never commit to or push `main`.
