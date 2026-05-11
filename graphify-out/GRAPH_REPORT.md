# Graph Report - internal  (2026-05-08)

## Corpus Check
- Corpus is ~38,923 words - fits in a single context window. You may not need a graph.

## Summary
- 591 nodes · 955 edges · 40 communities (30 shown, 10 thin omitted)
- Extraction: 81% EXTRACTED · 19% INFERRED · 0% AMBIGUOUS · INFERRED: 178 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- [[_COMMUNITY_Traefik Config Generation|Traefik Config Generation]]
- [[_COMMUNITY_Docker Container Management|Docker Container Management]]
- [[_COMMUNITY_Engine Core & Aliases|Engine Core & Aliases]]
- [[_COMMUNITY_Dry-Run Container Backend|Dry-Run Container Backend]]
- [[_COMMUNITY_Manifest Parsing|Manifest Parsing]]
- [[_COMMUNITY_Local Deployment Store|Local Deployment Store]]
- [[_COMMUNITY_Apply & Deploy Handlers|Apply & Deploy Handlers]]
- [[_COMMUNITY_Traefik Routing Tests|Traefik Routing Tests]]
- [[_COMMUNITY_Planner Loader|Planner Loader]]
- [[_COMMUNITY_Traefik Plugin & Fake Backend|Traefik Plugin & Fake Backend]]
- [[_COMMUNITY_Planner Resolve & Templates|Planner Resolve & Templates]]
- [[_COMMUNITY_Team Handlers|Team Handlers]]
- [[_COMMUNITY_Configuration|Configuration]]
- [[_COMMUNITY_Manifest Validation|Manifest Validation]]
- [[_COMMUNITY_Local Subnet Store|Local Subnet Store]]
- [[_COMMUNITY_Secret Resolver|Secret Resolver]]
- [[_COMMUNITY_Traefik Spec Types|Traefik Spec Types]]
- [[_COMMUNITY_Deployment Handlers|Deployment Handlers]]
- [[_COMMUNITY_Routing Collision Planner|Routing Collision Planner]]
- [[_COMMUNITY_Engine Interfaces & Types|Engine Interfaces & Types]]
- [[_COMMUNITY_Status Handlers|Status Handlers]]
- [[_COMMUNITY_Engine Tests|Engine Tests]]
- [[_COMMUNITY_Engine Events & Observers|Engine Events & Observers]]
- [[_COMMUNITY_Config Paths|Config Paths]]
- [[_COMMUNITY_Auto-Updater|Auto-Updater]]
- [[_COMMUNITY_Dry-Run DNS Backend|Dry-Run DNS Backend]]
- [[_COMMUNITY_Dry-Run Routing Backend|Dry-Run Routing Backend]]
- [[_COMMUNITY_Deployment State Types|Deployment State Types]]
- [[_COMMUNITY_Traefik Test Helpers|Traefik Test Helpers]]
- [[_COMMUNITY_Subnet State Types|Subnet State Types]]
- [[_COMMUNITY_Resource Handlers|Resource Handlers]]
- [[_COMMUNITY_App Handlers|App Handlers]]
- [[_COMMUNITY_Team State Types|Team State Types]]
- [[_COMMUNITY_State Store Interface|State Store Interface]]

## God Nodes (most connected - your core abstractions)
1. `DockerBackend` - 21 edges
2. `New()` - 20 edges
3. `Parse()` - 18 edges
4. `baseOp()` - 16 edges
5. `stubLstatNotExist()` - 15 edges
6. `stubReadFile()` - 14 edges
7. `captureWriteFileFn()` - 14 edges
8. `LoadDir()` - 12 edges
9. `Resolve()` - 12 edges
10. `testdataPath()` - 12 edges

## Surprising Connections (you probably didn't know these)
- `Teardown()` --calls--> `PlanTeardown()`  [INFERRED]
  handler/teardown.go → planner/plan.go
- `CreateTeam()` --calls--> `Parse()`  [INFERRED]
  handler/teams.go → manifest/parser.go
- `Order()` --calls--> `Sort()`  [INFERRED]
  planner/order.go → topo/topo.go
- `NewLocalEngineWithRouting()` --calls--> `NewLiveResolver()`  [INFERRED]
  engine/local/local_engine.go → resolver/resolver.go
- `renderTemplates()` --calls--> `New()`  [INFERRED]
  resolver/resolver.go → plugins/gateway/traefik/plugin.go

## Communities (40 total, 10 thin omitted)

### Community 0 - "Traefik Config Generation"
Cohesion: 0.09
Nodes (41): dashboardDynamicFileName(), emitLegacyHTTPBlockSignal(), emitTLSPortNoWebsecureSignal(), generateDashboardDynamicConfig(), generateStaticConfig(), hasLegacyDashboardHTTPBlock(), hasWebsecureEntrypoint(), htpasswdEntry() (+33 more)

### Community 1 - "Docker Container Management"
Cohesion: 0.08
Nodes (16): buildNetwork(), buildPortBindings(), buildRestartPolicy(), configHash(), containerName(), isContainerUpToDate(), resolvedProto(), networkName() (+8 more)

### Community 2 - "Engine Core & Aliases"
Cohesion: 0.08
Nodes (28): Engine, boolPtr(), TestFormatAliasesForLog(), TestFormatAliasesForLog_AppendsTLSMarker(), TestResolveAliasRoutes(), TestResolveAliasRoutes_CarriesTLS(), flattenEnv(), flattenOutputs() (+20 more)

### Community 3 - "Dry-Run Container Backend"
Cohesion: 0.07
Nodes (14): NewDryRunContainerBackend(), NewDryRunEngine(), DryRunContainerBackend, DryRun(), DeployOptions, NewDryRunResolver(), DryRunResolver, LiveResolver (+6 more)

### Community 4 - "Manifest Parsing"
Cohesion: 0.13
Nodes (23): Manifest, applicationName(), Parse(), parseManifest(), probeKind(), rejectTLSOutsideAliasEntries(), appYAMLWithAliases(), parseBytes() (+15 more)

### Community 5 - "Local Deployment Store"
Cohesion: 0.12
Nodes (13): NewDeploymentStore(), TestDeploymentStore_EmptyTeam(), TestDeploymentStore_Interface(), TestDeploymentStore_LoadTeam(), TestDeploymentStore_Persistence(), TestDeploymentStore_TeamIsolation(), DeploymentStore, NewLocalStore() (+5 more)

### Community 6 - "Apply & Deploy Handlers"
Cohesion: 0.11
Nodes (16): NewDockerBackend(), ApplySingle(), ApplySingleOptions, Deploy(), Teardown(), TeardownOptions, NewContainerBackend(), NewLocalEngine() (+8 more)

### Community 7 - "Traefik Routing Tests"
Cohesion: 0.23
Nodes (27): baseOp(), captureRemoveFileFn(), captureWriteFileFn(), newTestBackend(), stubLstatError(), stubLstatNotExist(), stubLstatPresent(), stubReadFileWebsecureMissing() (+19 more)

### Community 8 - "Planner Loader"
Cohesion: 0.09
Nodes (17): LoadDir(), TestLoadDir(), TestLoadDir_Duplicates(), TestLoadDir_ForeignOnly(), TestLoadDir_ValidPlusForeign(), TestLoadDir_ValidPlusMalformed(), TestPlanSingle_BadKind_WrapsFilePath(), ManifestSet (+9 more)

### Community 9 - "Traefik Plugin & Fake Backend"
Cohesion: 0.12
Nodes (7): fakeBackend, Plugin, New(), TestPlugin_Validate_AcceptsValidTLSPort(), TestPlugin_Validate_RejectsTLSPortCollidesWithDashboardPort(), TestPlugin_Validate_RejectsTLSPortCollidesWithPort(), TestPlugin_Validate_RejectsTLSPortOutOfRange()

### Community 10 - "Planner Resolve & Templates"
Cohesion: 0.12
Nodes (18): ExtractFieldRefs(), MockStore, allowedResourceTypes(), buildRegistryAliasMap(), checkImageAlias(), enforceQuota(), hasAccess(), Resolve() (+10 more)

### Community 11 - "Team Handlers"
Cohesion: 0.11
Nodes (14): ApplyTeams(), CreateTeam(), DescribeTeam(), printTeamDeploymentsSummary(), Class, Classify(), IsShrineAPIVersion(), TestClassify() (+6 more)

### Community 12 - "Configuration"
Cohesion: 0.12
Nodes (9): Config, GatewayPluginsConfig, PluginsConfig, RegistryConfig, TraefikDashboardConfig, TraefikPluginConfig, expandTilde(), resolvePath() (+1 more)

### Community 13 - "Manifest Validation"
Cohesion: 0.28
Nodes (15): countSet(), hasInvalidChars(), normalizePathPrefix(), Validate(), validateApplicationSpec(), validateExclusiveFields(), validateMetadata(), validateMetadataName() (+7 more)

### Community 14 - "Local Subnet Store"
Cohesion: 0.2
Nodes (7): NewSubnetStore(), TestSubnetStore_DefensiveCopy(), TestSubnetStore_Exhaustion(), TestSubnetStore_Interface(), TestSubnetStore_Load(), TestSubnetStore_Persistence(), SubnetStore

### Community 15 - "Secret Resolver"
Cohesion: 0.24
Nodes (11): fakeSecrets, NewLiveResolver(), newFakeSecrets(), TestResolveApplication_AppBuiltins(), TestResolveApplication_MissingResource(), TestResolveApplication_StaticAndValueFrom(), TestResolveApplication_Templates(), TestResolveResource_BareNonHostFails() (+3 more)

### Community 16 - "Traefik Spec Types"
Cohesion: 0.13
Nodes (14): apiConfig, basicAuth, entryPoint, fileProvider, httpConfig, loadBalancer, middleware, providersConfig (+6 more)

### Community 17 - "Deployment Handlers"
Cohesion: 0.28
Nodes (14): collectAllDeployments(), collectTeamDeployments(), deploymentsForTeamOrAll(), DescribeApplication(), describeDeployment(), DescribeResource(), filterByKind(), ListApplications() (+6 more)

### Community 18 - "Routing Collision Planner"
Cohesion: 0.37
Nodes (12): DetectRoutingCollisions(), normalizePrefix(), makeApp(), setWith(), TestDetectRoutingCollisions_AliasVsAlias(), TestDetectRoutingCollisions_Disjoint(), TestDetectRoutingCollisions_MultipleCollisions(), TestDetectRoutingCollisions_PrimaryVsAlias() (+4 more)

### Community 19 - "Engine Interfaces & Types"
Cohesion: 0.15
Nodes (12): AliasRoute, BindMount, ContainerBackend, ContainerInfo, CreateContainerOp, DNSBackend, PortBinding, RemoveContainerOp (+4 more)

### Community 20 - "Status Handlers"
Cohesion: 0.28
Nodes (10): containerStatusRow, mockBackend, inspectDeployments(), printStatusTable(), StatusApplication(), StatusResource(), statusSingleDeployment(), statusSingleDeploymentAutoTeam() (+2 more)

### Community 22 - "Engine Events & Observers"
Cohesion: 0.25
Nodes (5): Event, EventStatus, MultiObserver, NoopObserver, Observer

### Community 23 - "Config Paths"
Cohesion: 0.48
Nodes (6): Paths, defaultStateDir(), discoverConfigFile(), resolveConfigDir(), ResolvePaths(), resolveStateDir()

### Community 24 - "Auto-Updater"
Cohesion: 0.47
Nodes (4): Release, extractBinary(), LatestVersion(), Update()

## Knowledge Gaps
- **77 isolated node(s):** `ResolvedDependencies`, `Resolver`, `routeKey`, `PlannedStep`, `PlanResult` (+72 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **10 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `New()` connect `Traefik Plugin & Fake Backend` to `Traefik Config Generation`, `Dry-Run Container Backend`, `Local Deployment Store`, `Apply & Deploy Handlers`, `Traefik Routing Tests`, `Planner Resolve & Templates`, `Configuration`?**
  _High betweenness centrality (0.405) - this node is a cross-community bridge._
- **Why does `Deploy()` connect `Apply & Deploy Handlers` to `Planner Loader`, `Traefik Plugin & Fake Backend`, `Routing Collision Planner`, `Dry-Run Container Backend`?**
  _High betweenness centrality (0.184) - this node is a cross-community bridge._
- **Why does `Parse()` connect `Manifest Parsing` to `Planner Loader`, `Team Handlers`?**
  _High betweenness centrality (0.169) - this node is a cross-community bridge._
- **Are the 18 inferred relationships involving `New()` (e.g. with `renderTemplates()` and `.LoadTeam()`) actually correct?**
  _`New()` has 18 INFERRED edges - model-reasoned connections that need verification._
- **Are the 15 inferred relationships involving `Parse()` (e.g. with `LoadDir()` and `PlanSingle()`) actually correct?**
  _`Parse()` has 15 INFERRED edges - model-reasoned connections that need verification._
- **What connects `ResolvedDependencies`, `Resolver`, `routeKey` to the rest of the system?**
  _77 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Traefik Config Generation` be split into smaller, more focused modules?**
  _Cohesion score 0.09 - nodes in this community are weakly interconnected._