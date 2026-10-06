# Data Model: A Configuration Default for the Image Pull Policy, and Manifests Generated to Match

**Feature**: 034-pull-policy-config-default | **Date**: 2026-10-06

One new configuration field, one new in-memory record on the planner's manifest set, two extended option structs, and no change to any recorded state.

## 1. Configuration (`internal/config`)

### 1.1 `Config` — MODIFIED

```go
type Config struct {
	Registries      []RegistryConfig `yaml:"registries,omitempty"`
	SpecsDir        string           `yaml:"specsDir,omitempty"`
	TeamsDir        string           `yaml:"teamsDir,omitempty"`
	ImagePullPolicy string           `yaml:"imagePullPolicy,omitempty"` // NEW: "", Always, IfNotPresent, or Pinned
	Plugins         PluginsConfig    `yaml:"plugins,omitempty"`
}

func (c *Config) validateImagePullPolicy() error // NEW: called from Load after validateSecretsPlugins
```

| Value | Meaning |
|---|---|
| absent or `""` | no default; the derived rule is the bottom layer for manifests that name no policy |
| `Always`, `IfNotPresent`, `Pinned` | the default for every manifest that names no policy |
| anything else | `Load` fails: `imagePullPolicy: must be one of Always, IfNotPresent, Pinned`; the command stops in `PersistentPreRunE` with `loading config: ` prefixed |

Validation uses `manifest.IsKnownPullPolicy`; matching is exact.

## 2. Planner (`internal/planner`)

### 2.1 Policy source — NEW

```go
type pullPolicySource int

const (
	policyFromManifest pullPolicySource = iota // the manifest's own spec.imagePullPolicy
	policyFromDefault                          // the configuration's imagePullPolicy
	policyFromDerivedRule                      // Always for latest or no tag, IfNotPresent otherwise
)

type ManifestSet struct {
	Applications      map[string]*manifest.ApplicationManifest
	Resources         map[string]*manifest.ResourceManifest
	pullPolicySources map[string]pullPolicySource // NEW, unexported; nil until applyEffectivePullPolicy runs
}

func (s *ManifestSet) recordPullPolicySource(kind, name string, src pullPolicySource) // NEW: allocates lazily
func (s *ManifestSet) isPolicyFromDefault(kind, name string) bool                      // NEW: false on a nil map
```

Key: `<kind>/<name>` with `kind` as `application` or `resource`, the same words the error messages use.

### 2.2 Effective policy — precedence (unchanged rule, now with a live middle layer)

| Manifest field | Configuration default | Effective policy | Source recorded |
|---|---|---|---|
| set | any | the field | `policyFromManifest` |
| empty | set | the default | `policyFromDefault` |
| empty | empty | `Always` for `latest` or no tag, `IfNotPresent` for any other tag or a digest | `policyFromDerivedRule` |

`applyEffectivePullPolicy(set, defaultPullPolicy)` writes the effective value into `Spec.ImagePullPolicy` and records the source in the same loop. `Plan` is the only caller and the only place the default enters.

### 2.3 Fixed-version rule — MODIFIED endings

Validated by `validateImagePolicies(set)` after normalisation, as in T3. The stem of each message is unchanged; the ending depends on the recorded source.

| Kind | Field | Invalid when (effective policy `Pinned`) | Source | Message |
|---|---|---|---|---|
| Application | `spec.image` | tag neither `""` nor `latest`, or a digest | manifest | `application "x": spec.image "<image>" names a fixed version but the image pull policy is Pinned; use "<repo>" or "<repo>:latest"` |
| Application | `spec.image` | same | default | `application "x": spec.image "<image>" names a fixed version but the image pull policy is Pinned (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default` |
| Resource | `spec.version` | not `""` and not `latest` | manifest | `resource "db": spec.version "<v>" names a fixed version but the image pull policy is Pinned; omit it or use "latest"` |
| Resource | `spec.version` | same | default | `resource "db": spec.version "<v>" names a fixed version but the image pull policy is Pinned (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default` |
| Resource | `spec.image` (override) | as Application | manifest / default | the Application-shaped message with the matching ending |
| Resource | `spec.version` | effective policy manifest-owned and version `""` | any | `resource "db": spec.version is required` (unchanged) |

`fixedVersionRemedy(fromDefault bool, manifestHint string) string` returns the ending; `fixedVersionImageError` and `fixedVersionResourceVersionError` take `fromDefault`.

### 2.4 Signatures — MODIFIED

```go
func Plan(set *ManifestSet, store state.TeamStore, registries []config.RegistryConfig, ports PortContext, filter Filter, defaultPullPolicy string) PlanResult // unchanged
func Resolve(set *ManifestSet, store state.TeamStore, registries []config.RegistryConfig) []error // defaultPullPolicy removed
func validateImagePolicies(set *ManifestSet) []error                                              // defaultPullPolicy removed
```

## 3. Handlers (`internal/handler`)

### 3.1 Planning call sites — MODIFIED

| Handler | Call | Default passed |
|---|---|---|
| `DryRun(out, errOut, dir, store, cfg, filter)` | `planner.Plan(set, store.Teams, cfg.Registries, ports, filter, cfg.ImagePullPolicy)` | configuration |
| `Deploy(b, dir, filter)` | `planner.Plan(set, b.Store.Teams, b.Cfg.Registries, ports, filter, b.Cfg.ImagePullPolicy)` | configuration |
| `ApplySingle(b, file, dir)` | `planner.Plan(set, b.Store.Teams, b.Cfg.Registries, ports, filter, b.Cfg.ImagePullPolicy)` | configuration |

### 3.2 Generate options — MODIFIED

```go
type AppOptions struct {
	Name, Team, OutputDir string
	Port, Replicas        int
	Domain, PathPrefix    string
	ExposeToPlatform      bool
	Image                 string // as typed; "" means default by policy
	PullPolicy            string // NEW: cfg.ImagePullPolicy
}

type ResourceOptions struct {
	Name, Team, OutputDir string
	Type                  string
	Version               string // as typed; "" means default by policy
	ExposeToPlatform      bool
	PullPolicy            string // NEW: cfg.ImagePullPolicy
}

func defaultAppImage(name, pullPolicy string) string      // NEW: name under Pinned, name+":latest" otherwise
func defaultResourceVersion(pullPolicy string) string     // NEW: "" under Pinned, "16" otherwise
func versionLine(version string) string                   // NEW: `  version: "<v>"` + newline, or "" when version is ""
func renderAppSkeleton(opts AppOptions) string            // NEW: pure; applies defaultAppImage when Image is ""
func renderResourceSkeleton(opts ResourceOptions) string  // NEW: pure; applies defaultResourceVersion when Version is ""
```

| Default | `generate application <name>` image | `generate resource <name>` version line |
|---|---|---|
| absent, `Always`, `IfNotPresent` | `<name>:latest` | `  version: "16"` |
| `Pinned` | `<name>` | omitted |
| any, flag given | the flag's value verbatim | the flag's value verbatim |

Neither skeleton contains an `imagePullPolicy:` line under any default.

### 3.3 Command — MODIFIED (`cmd/generate.go`)

| Flag | Before | After |
|---|---|---|
| `--image` | default `""`; help `Docker image to run (defaults to [name]:latest)`; command fills `name:latest` | default `""`; help `Docker image to run (defaults to [name]:latest, or [name] when imagePullPolicy in config.yml is Pinned)`; command passes as typed |
| `--version` | default `"16"`; help `Version of the resource` | default `""`; help `Version of the resource (defaults to 16; omitted when imagePullPolicy in config.yml is Pinned)` |

Both commands set `PullPolicy: cfg.ImagePullPolicy`.

## 4. Recorded state — UNCHANGED

`pins.txt` and `deployments.txt` keep their T3 shapes. The deployment record's policy field already stores the effective value, so a record written under a configuration default of `Pinned` says `Pinned`; nothing records the source.

## 5. Test fixtures (`tests/testdata/pull-policy-default/`) — NEW

| Directory | Manifests | Valid under |
|---|---|---|
| `versioned/` | `app-latest.yml` (`traefik/whoami:latest`, no policy); `app-fixed.yml` (`traefik/whoami:v1.10.2`, no policy); `res-fixed.yml` (`type: cache`, `image: traefik/whoami`, `version: "16"`, no policy); `app-own-ifnotpresent.yml` (`traefik/whoami:v1.10.1`, `imagePullPolicy: IfNotPresent`) | absent, `IfNotPresent`, `Always`; under `Pinned` fails on `app-fixed` and `res-fixed` with the configuration-sourced ending |
| `pinned-shape/` | `app-untagged.yml` (`traefik/whoami`, no policy); `res-noversion.yml` (`type: cache`, `image: traefik/whoami`, no version, no policy); `res-own-pinned.yml` (`type: cache`, `image: traefik/whoami`, no version, `imagePullPolicy: Pinned`) | `Pinned`; under absent, `IfNotPresent`, `Always` fails on `res-noversion` with `spec.version is required` |
| `own-pinned-fixed/` | `app-own-pinned-fixed.yml` (`traefik/whoami:v1.10.2`, `imagePullPolicy: Pinned`) | none; the message is manifest-sourced under every default |

All fixtures use `owner: shrine-deploy-test` and `port: 80` where a port is required, matching the pinned suite's team fixture.
