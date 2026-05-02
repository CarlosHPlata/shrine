# Shrine — Build Progress

## Phase Checklist

- [x] **Phase 1: Scaffold** — `go mod init`, Cobra setup, all CLI commands stubbed
  - `main.go` — thin entry point, delegates to `cmd.Execute()`
  - `cmd/root.go` — root Cobra command with `--config-dir` and `--state-dir` flags
  - `cmd/deploy.go` — `shrine deploy [path]` (stub)
  - `cmd/teardown.go` — `shrine teardown [team]` (stub)
  - `cmd/status.go` — `shrine status [team]` (stub)
- [x] **Phase 2: Manifest parsing** — Go structs for Application, Resource, Team; YAML parsing; field validation
  - `internal/manifest/types.go` — all manifest structs with `TypeMeta` embedding
  - `internal/manifest/parser.go` — two-pass parser (probe kind, then unmarshal into correct type)
  - `internal/manifest/validate.go` — multi-error validation for all kinds
  - `test/testdata/` — YAML fixtures for valid and invalid manifests
  - `apiVersion` uses `shrine/v1` (not `homelab/v1`)
- [x] **Phase 3: Team management** — verb-first CLI; `Store` interface; handler layer; state directory resolution
  - `cmd/generate.go` — `shrine generate team|app|resource <name>` (now supports detailed property flags like `--port`, `--type`, etc.)
  - `cmd/create.go` — `shrine create team -f <path>`
  - `cmd/apply.go` — `shrine apply teams`
  - `cmd/get.go` — `shrine get teams`
  - `cmd/describe.go` — `shrine describe team <name>`
  - `cmd/delete.go` — `shrine delete team <name>`
  - `internal/handler/teams.go`, `apps.go`, `resources.go` — resource-specific business logic
  - `internal/config/paths.go` — dynamic path resolution (Flag > Env > .env > XDG/Root)
  - `internal/state/store.go` — `Store` interface for pluggable persistence
  - `internal/state/file_store.go` — filesystem-based implementation with atomic writes
- [x] **Phase 4: Planner** — dependency graph, `valueFrom` resolution, access checks, quota enforcement
  - `internal/planner/loader.go` — `ManifestSet` struct, `LoadDir` with duplicate detection
  - `internal/planner/resolve.go` — dependency resolution, access checks, quota enforcement, `valueFrom` validation
  - `internal/planner/order.go` — `PlannedStep` struct, deterministic ordering (Resources first, then Applications, alphabetical within kind)
  - `internal/planner/resolve_test.go` — MockStore, 9 test cases (deps, access, quotas, valueFrom, env mutual exclusivity)
  - `internal/planner/order_test.go` — deterministic ordering verification
  - `internal/planner/plan.go` — `Plan()` entry point: load → resolve → order pipeline
- [x] **Phase 5: Executor interface** — pluggable backend architecture + dry-run implementation
  - `internal/engine/backends/backends.go` — `ContainerBackend`, `RoutingBackend`, `DNSBackend` interfaces with operation structs
  - `internal/engine/backends/dry_run_container.go` — prints Docker operations
  - `internal/engine/backends/dry_run_routing.go` — prints route operations
  - `internal/engine/backends/dry_run_dns.go` — prints DNS operations
  - `internal/engine/engine.go` — `Engine` struct with optional backends, `ExecuteDeploy` dispatches steps to backends
  - `internal/handler/dry_run.go` — orchestrates plan → engine for the deploy command
  - `cmd/deploy.go` — `--dry-run` flag wired, calls handler
  - End-to-end dry-run working: `shrine deploy ./path --dry-run`
- [x] **Phase 6: State** — subnet allocator, secret generator, deployment tracking
  - `internal/state/subnets.go` + `internal/state/local/subnets.go` — `SubnetStore` with third-octet allocator (10.100.5.0/24 through 10.100.255.0/24), idempotent `AllocateSubnet`
  - `internal/state/secrets.go` + `internal/state/local/secrets.go` — `SecretStore` with `GetOrGenerate` (crypto/rand + base64.RawURLEncoding); 0700 dir, 0600 file; no in-memory cache
  - `internal/state/deployments.go` + `internal/state/local/deployments.go` — `DeploymentStore` with upsert `Record`, idempotent `Remove`, `List`; space-separated `<kind> <name> <container-id>` format at `<baseDir>/<team>/deployments.txt`
  - All stores: atomic temp+rename writes, `sync.Mutex` for in-process serialization, sentinel errors (`errors.Is`-friendly)
  - Wired into `state.Store` aggregator and `local.NewLocalStore`
- [x] **Phase 7: Docker** — network creation, container lifecycle, env injection via Executor
  - [x] **Entry work (resolver + schema):**
    - `internal/manifest/types.go` — added `Template string` to `Output`; removed the now-redundant `StaticEnv()` helpers (resolver supersedes them)
    - `internal/manifest/validate.go` — enforces `value` XOR `generated` XOR `template` on outputs; `host` must be bare (CLI built-in); non-`host` bare outputs rejected. Env-level `value` XOR `valueFrom` also moved here from the planner.
    - `internal/manifest/template.go` — `ExtractFieldRefs` walks `text/template` parse trees; shared by planner validation and resolver
    - `internal/planner/templates.go` — `validateTemplates` parses every template output and confirms each `{{.X}}` is a sibling output or built-in (`team`, `name`)
    - `internal/planner/resolve.go` — delegates per-field structural checks to `manifest.Validate`; calls `validateTemplates` per resource
    - `internal/resolver/` — new package: `ResolveResource` (literals, generated secrets via `SecretStore`, topologically sorted templates with cycle detection via Kahn's algorithm) and `ResolveApplication` (walks `valueFrom: resource.<name>.<output>` references)
    - `internal/engine/engine.go` — pre-resolves every resource up front in `ExecuteDeploy` so apps can reference outputs regardless of step order; `flattenEnv`/`flattenOutputs` produce deterministic sorted KEY=VALUE slices; resource container env now includes generated secrets and rendered templates (not just literals)
    - Tests added for all of the above; full test suite green
  - [x] **DockerBackend implementation** — deploy path done, teardown path stubbed:
    - [x] `internal/engine/local/docker_backend.go` — `DockerBackend` struct injected with `*state.Store` and `[]config.RegistryConfig`; Docker client via `client.FromEnv` + API version negotiation
    - [x] `CreateNetwork(team)` — inspect-first, idempotent; bridge network `shrine.<team>.private` with `/24` from `SubnetStore`; errors on subnet mismatch
    - [x] `CreateContainer(op)` — reconciles by name (`<team>.<resource>`): creates fresh, restarts stopped, remove+recreate on image drift. Labels: `shrine.team`, `shrine.resource`, `shrine.kind`. Records in `DeploymentStore` after start.
    - [x] `ensureImage` + `internal/engine/local/registry_auth.go` — `ImageList` cache check before pull; per-registry base64url-encoded `X-Registry-Auth` from `config.yml`; anonymous fallback for Docker Hub; `ImagePull` stream drained to EOF so the pull actually completes
    - [x] `internal/config/config.go` — `config.yml` loader with `RegistryConfig`; missing-file tolerant
    - [x] `internal/engine/local/local_engine.go` — real-engine constructor; wired through `cmd/deploy.go` + `handler/deploy.go`. `--dry-run` path left intact.
    - [x] `CreateContainerOp` refactor — dropped the `Network`-as-team hack; now carries `Team`, `Name`, `Kind`, `Image`, `Env` explicitly. Engine + dryrun updated.
    - [x] End-to-end deploy verified with a real manifest: resource (redis) + app (finances from homelab registry `192.168.1.206:5000`) running on `shrine.test-team.private`, labels stamped, reconcile-by-name holds across redeploys and image-tag bumps.
    - [x] `RemoveContainer(name)` — stub; needs inspect-first, soft-success on not-found, force-remove, clean `DeploymentStore` entry
    - [x] `RemoveNetwork(team)` — stub; needs inspect-first, soft-success on not-found, remove (design: error vs. force-disconnect if containers still attached; design: free subnet in `SubnetStore` or keep it allocated)
    - [x] Teardown orchestration — `engine.Engine.ExecuteTeardown(team)` (reverse of deploy: apps before resources, then network), `handler/teardown.go`, wire `cmd/teardown.go`
  - **Scope note — `exposeToPlatform` is not implemented in Phase 7.** The field is parsed and scaffolded by `shrine generate`, but the engine ignores it: the second `NetworkingConfig` slot in `CreateContainer` is a literal `nil, //platform` at `internal/engine/local/docker_backend.go:235`, and no `shrine.platform` network is ever created. Cross-team reachability is deferred to Phase 8. Until then, apps can only reference resources in their own team.
- [x] **Phase 8: Application composition & platform networking** — cross-project wiring: env templates, app→app dependencies, platform network, Application built-in outputs. The gate for this phase is `test/smock/` round-tripping end-to-end.
  - [x] **Env templates on `EnvVar`** — add `Template string` to `EnvVar`; extend `validateApplicationSpec` to require exactly-one-of `{value, valueFrom, template}`; plan-time check that every `{{.X}}` references a sibling env name (built-ins: `team`, `name`); two-pass resolve in both `LiveResolver` and `DryRunResolver` (literals/`valueFrom` first, templates via topological order, Kahn's algorithm with cycle rejection). `topoSort` lives in `internal/topo/` (standalone package). Both `internal/resolver/` and `internal/planner/` import it. `renderTemplates` in resolver is now generic (scope string, `map[string]string`) — shared between resource output rendering and application env rendering
  - [x] **Platform network backend** — `shrine.platform` bridge; hardcoded `10.200.0.0/24` (YAGNI — single platform network, no allocator needed); inspect-first idempotent create; when `networking.exposeToPlatform: true`, attach a second `EndpointSettings` for the platform network in `CreateContainer`; teardown leaves the platform network up (platform-global, not team-owned)
  - [x] **`kind: Application` dependencies** — extended `resolveDependencies` with `switch dep.Kind`; `validateApplicationDep` looks up in `set.Applications`; enforces owner match and `access:` check via symmetric `hasAccess()`
  - [x] **Topo sort in planner ordering** — `planner/order.go` uses Kahn's algorithm over the combined Resource+Application graph with `"Kind:name"` composite keys; cycle detection rejects cycles with a descriptive error
  - [x] **Application built-in outputs (`host`, `port`)** — synthesized up-front in `engine.ExecuteDeploy`: `host = <owner>.<name>`, `port = strconv.Itoa(spec.port)`; `valueFrom` grammar extended to accept `application.*` prefix in `lookupValueFrom`
  - [x] **Cross-team reachability check** — planner errors if an app depends on a resource/app in another team without `exposeToPlatform: true` on the producer
  - [x] **Application access control** — `hasAccess()` symmetric against `app.Metadata.Access`, cross-team app consumption requires the producer to list the consuming team
  - [x] **Smock round-trip (phase gate)** — `test/smock/` deploys end-to-end (externaldeps + backendredis + aterrizar, across teams `external` and `backend`); `shrine teardown backend` then `shrine teardown external` cleans up; `shrine.platform` persists; `docker ps -a` empty
- [ ] **Phase 9: Routing** — see `specs/features/routing.md` for full spec
- [ ] **Phase 10: DNS** — AdGuard HTTP API client via Executor
- [ ] **Phase 11: Teardown** — reverse of deploy, reading from state
- [ ] **Phase 12: End-to-end dry run** — full `shrine deploy --dry-run --verbose` against real manifests (full stack: Docker + Traefik + DNS)
- [ ] **Phase 13: Packaging & Distribution** — GoReleaser config; .deb packaging with post-install scripts
- [x] **Phase 14: Documentation site** — see `specs/013-docs-site/`. Hugo + Hextra at `docs/`, deployed to GitHub Pages on push to `main`. Per-page "copy as Markdown" button (every page exposes `<page>/index.md` for AI-agent ingestion). CLI reference auto-generated from the Cobra tree by an isolated tool at `docs/tools/docsgen/` (separate Go module, main `go.mod` stays clean). PR-time gates: front-matter lint, CLI drift check, Markdown companion check, Markdown shape check.

## Current State

- **Last completed phase:** 8 (Application composition & platform networking). `test/smock/` round-trip passes end-to-end: three containers across two teams deploy, platform network wires cross-team app communication, teardown cleans up completely.
- **Next phase:** Phase 9 — Routing. See `specs/features/routing.md`.
- **Go version:** 1.24.4
- **Module path:** `github.com/CarlosHPlata/shrine`

## Known Gaps

- **DNS record `Value` is hardcoded as `[IP_ADDRESS]`** in `internal/engine/engine.go` (deployApplication, DNS step). The gateway IP lives in `<config-dir>/config.yml` and needs to be plumbed into the engine. Deferred to Phase 10 (DNS) — that's when the real AdGuard backend replaces the dry-run one and the actual IP matters.
- **Generated-secret length is hardcoded** (`generatedSecretLength = 32` in `internal/resolver/resolver.go`). If a resource ever needs a specific length, expose a `length:` field on `Output` and thread it through. Low priority.
- **`ExtractFieldRefs` returns root identifiers only** — `{{.foo.bar}}` yields `"foo"`. Fine while all outputs are flat strings. If dotted cross-resource template refs are ever needed, this walker needs to handle the full `Ident` slice.
- **Host-port publishing is intentionally unsupported.** External access is via Traefik only. If a protocol that can't go through Traefik is ever needed, add `spec.publish.hostPort` (explicit) or `spec.publish: true` (auto-allocate) with a `HostPortStore` allocator.
- **Cross-team direct network attachment deferred.** Phase 8 implements cross-team reachability via `exposeToPlatform: true` → producer joins the shared `shrine.platform` network. A finer-grained model (app declares `networking.joinNetworks: [team-b]`, consent by target team's `access:` list) was discussed and deliberately parked.

## Decisions Made

- CLI commands live in flat files under `cmd/` (not subdirectories) — simpler for now, can split later
- `team remove` command added beyond original spec
- Verb-first CLI style (kubectl convention): `shrine get teams` instead of `shrine team list`
- Resource-specific logic lives in `internal/handler/` — cmd files are thin dispatchers
- State directory resolution: Flag > Env > .env (godotenv) > XDG/FHS defaults
- Case-insensitive team lookups — state files always stored in lowercase
- **Pluggable backends:** Engine uses three optional interfaces (Container, Routing, DNS). Nil backends are skipped. Dry-run is just a print implementation — no special-casing in the engine.
- **Resource outputs (Option B):** Resources declare explicit `outputs` (name/value/generated/template). The planner validates `valueFrom` references against output names — no hardcoded type-specific knowledge. Scales to any resource type without code changes.
- **Output kinds are a four-way union:** `value` (literal), `generated` (random secret), `template` (Go `text/template`), bare (infrastructure-synthesized, currently only `host` → `<team>.<name>`).
- **Template resolution ordering:** Outputs within a resource form a DAG; the resolver topologically sorts within each resource. Literals and generated secrets resolve first, templates resolve in DAG order. Cycle detection required. Template variables are validated at plan time.
- **`env` vs `outputs` naming:** Applications use `spec.env` (what the container consumes); Resources use `spec.outputs` (what they publish). The asymmetry reflects the directional difference: outputs are produced, env is consumed.
- **No `url` built-in for Applications.** Applications expose `host` and `port` only. Scheme composition is the consumer's job via `template` env entries. Avoids baking HTTP assumptions into the engine.
- **Deployed state is a cache, not source of truth.** `deployed.txt` records beliefs; Docker is authoritative. Reconcile by querying container state by name on each operation. "Not found" during teardown is soft success.
- **Cross-team reachability via the shared platform network.** Single `shrine.platform` bridge with `/24` from `10.200.0.0/16`. Producer opts in with `exposeToPlatform: true`; planner enforces this as a reachability error (distinct from the authorization error raised by `access:`).
- **Env templates mirror Output templates.** Applications can compose env values with `text/template`, referencing sibling env names plus built-ins `team` and `name`. Resolve is a two-pass DAG identical to the Resource output pipeline.
