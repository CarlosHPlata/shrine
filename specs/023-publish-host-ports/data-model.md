# Data Model: Publish Application Ports on Localhost

**Feature**: 023-publish-host-ports | **Date**: 2026-08-17

## 1. Manifest types (`internal/manifest/types.go`)

### 1.1 `Publish` — NEW

```go
// Publish declares host publishing for the application's service port.
// YAML forms: `publish: true` (automatic) or `publish: {hostPort: N}` (explicit).
type Publish struct {
	HostPort int // 0 = automatic allocation
}

func (p *Publish) UnmarshalYAML(node *yaml.Node) error
```

Unmarshaling rules:
- scalar `true` → `Publish{HostPort: 0}` (automatic)
- scalar `false` → field left nil (equivalent to omission)
- mapping `{hostPort: N}` → `Publish{HostPort: N}`
- any other node kind/keys → parse error naming the field

### 1.2 `Networking` — MODIFIED

```go
type Networking struct {
	ExposeToPlatform bool     `yaml:"exposeToPlatform,omitempty"`
	Publish          *Publish `yaml:"publish,omitempty"`   // NEW
}

func (n Networking) shouldAttachToPlatform() bool // ExposeToPlatform || Publish != nil
```

`shouldAttachToPlatform` is the single derivation point for the implied platform
attachment (exported or package-internal per final placement of the projection;
naming follows the constitution's boolean-method rule).

### 1.3 Validation (`internal/manifest/validate.go`, `validateApplicationSpec`)

New checks, appended to the existing multi-error list:
- `publish.hostPort` must be `0` or within `1024–65535`
- `publish.hostPort` must not fall within `30000–32767` (reserved for automatic allocation)
- publishing requires `spec.port` to be set (already mandatory today — no new rule, but the error path must stay coherent if that ever changes)

Resources: `ResourceSpec` is **not** extended (applications only, per spec assumption).

## 2. Allocation state (`internal/state`)

### 2.1 `HostPortStore` interface — NEW (`internal/state/hostports.go`)

```go
var (
	ErrHostPortNotFound     = errors.New("host port allocation not found")
	ErrNoAvailableHostPorts = errors.New("no available host ports in the automatic range")
	ErrHostPortTaken        = errors.New("host port already allocated to another application")
)

type HostPortMap map[string]int // "team/app" → host port

type HostPortStore interface {
	AllocateHostPort(team, app string) (int, error) // idempotent lookup-or-allocate (30000–32767)
	ClaimHostPort(team, app string, port int) error // explicit claim: idempotent upsert; ErrHostPortTaken if another app holds it
	GetHostPort(team, app string) (int, error)      // ErrHostPortNotFound when absent
	ReleaseHostPort(team, app string) error         // idempotent no-op when absent
	ReleaseTeamHostPorts(team string) error         // releases every "team/*" entry
	ListHostPorts() (HostPortMap, error)
}
```

Semantics:
- `AllocateHostPort` returns the existing entry unchanged if present (any mode) —
  this is the redeploy-stability guarantee. New allocations scan 30000→32767 for
  the first free slot, skipping reserved ports and every persisted entry.
- `ClaimHostPort` records/overwrites the caller's own entry; switching an app from
  automatic to explicit implicitly releases the old value by overwrite.
- All mutations: in-memory update → atomic save (temp+rename) → rollback the
  in-memory maps if the save fails (the `AllocateSubnet` pattern).

### 2.2 Local implementation — NEW (`internal/state/local/hostports.go`)

```go
const (
	firstAutoHostPort = 30000
	lastAutoHostPort  = 32767
)

type HostPortStore struct {
	baseDir  string
	ports    map[string]int        // "team/app" → port
	taken    map[int]struct{}      // includes reserved gateway ports
	mu       sync.Mutex
	// file-op funcs injected for unit tests (writeFile/rename/remove/read),
	// following the local-store test pattern already used by this package
}

func NewHostPortStore(baseDir string, reserved []int) (state.HostPortStore, error)
```

- Eager load at construction (forgiving parser: skip `#` comments and malformed lines).
- Reserved ports seed `taken` but are never written to the file.

### 2.3 Persistence format — `<stateDir>/hostports.txt`

```text
# team/app=port
media/jellyfin=30000
ops/dashboard=8080
```

- One `team/app=port` line per allocation, sorted by key for determinism.
- Written atomically via `os.CreateTemp` + `os.Rename`.
- Explicit and automatic allocations share the format (mode is not persisted;
  the manifest is the source of intent, the file is the source of occupancy).

### 2.4 Aggregate store — MODIFIED

```go
// internal/state/state.go
type Store struct {
	Teams       TeamStore
	Deployments DeploymentStore
	Subnets     SubnetStore
	HostPorts   HostPortStore // NEW
}
```

Wired in `internal/state/local/local.go` (`NewLocalStore`), with the reserved
gateway ports supplied by the composition root (`internal/app`), which already
holds both config and store construction.

`deployments.txt` is **unchanged** (see research R5 for why the port does not
live there).

## 3. Engine op types (`internal/engine/backends.go`)

```go
type PortBinding struct {
	HostIP        string // NEW — "" keeps today's 0.0.0.0 behavior (Traefik unchanged)
	HostPort      string
	ContainerPort string
	Protocol      string
}

type PublishPort struct { // NEW
	HostPort      int // 0 = automatic
	ContainerPort int
}

type CreateContainerOp struct {
	// ...existing fields...
	Publish *PublishPort // NEW — nil for non-published workloads
}
```

`Publish` is a *request* (intent); `PortBindings` remains the *resolved* binding
list. Only the container backend converts the former into the latter.

## 4. Projections

### 4.1 Manifest → op (`internal/engine/engine.go`, `deployApplication`)

```text
op.ExposeToPlatform = spec.Networking.shouldAttachToPlatform()   // derived
op.Publish          = {HostPort: spec.Networking.Publish.HostPort,
                       ContainerPort: spec.Port}                 // when Publish != nil
```

Unchanged on purpose:
- routing gate (`engine.go:187`) keeps reading the raw `ExposeToPlatform`
- planner cross-team gates (`resolve.go:164,194`) keep reading the raw field

### 4.2 Op → Docker (`internal/engine/local/dockercontainer/docker_container.go`)

```text
resolvePublishBinding(op) →
  explicit (HostPort > 0): state.HostPorts.ClaimHostPort(team, name, port)
  automatic (HostPort == 0): port = state.HostPorts.AllocateHostPort(team, name)
  → append PortBinding{HostIP: "127.0.0.1", HostPort: port,
                       ContainerPort: op.Publish.ContainerPort, Protocol: "tcp"}
```

Called before `buildPortBindings` and before `configHash` in both
`createFreshContainer` and `isContainerUpToDate` (idempotent store calls make the
double invocation safe). `buildPortBindings` passes `HostIP` through to
`nat.PortBinding`.

### 4.3 Config hash (`configHash`, `docker_container.go:253-257`)

Port spec string gains a HostIP segment **only when non-empty**:

```text
""        → "8080:3000/tcp"            (unchanged — existing hashes stable)
"127.0.0.1" → "127.0.0.1:8080:3000/tcp"
```

## 5. Planner (`internal/planner`)

```go
// internal/planner/hostports.go — NEW
type PortContext struct {
	Reserved  []int          // gateway host ports from config (HTTP, TLS, dashboard)
	Persisted state.HostPortMap
}

func DetectHostPortCollisions(set *ManifestSet, ports PortContext) error
```

- Deterministic walk (sorted app refs), accumulate all messages, sorted joined
  error — the `DetectRoutingCollisions` discipline.
- Cases: duplicate explicit in set; explicit = reserved; explicit = persisted
  entry of a different app. Self-adoption (`team/app`'s own persisted port) passes.
- `planner.Plan` gains the `PortContext` parameter and calls the detector for
  **all** filters (including single-manifest apply).
- Handlers (`deploy.go` Deploy/DryRun, `apply.go`) assemble `PortContext` from
  config + `ListHostPorts()`.

## 6. State transitions (allocation lifecycle)

```text
                       ┌─────────────────────────────────────────────┐
                       │                 no entry                    │
                       └──────┬──────────────────────────┬───────────┘
              deploy(auto)    │                          │  deploy(explicit N)
              Allocate → P    ▼                          ▼  Claim(N)
                       ┌─────────────┐            ┌─────────────┐
                       │ team/app=P  │◄──────────►│ team/app=N  │
                       └──────┬──────┘  redeploy  └──────┬──────┘
                              │        mode switch        │
   redeploy / recreate /      │      (Claim overwrites /  │
   teardown / publish removed │       Allocate returns    │
        → entry UNCHANGED     │       existing)           │
                              ▼                          ▼
                       delete application / delete team → entry removed
```

## 7. Key entity ↔ spec mapping

| Spec entity | Code artifact |
|---|---|
| Publish declaration | `manifest.Publish` on `Networking` |
| Port allocation record | `hostports.txt` line via `HostPortStore` |
| Platform-reserved ports | `PortContext.Reserved` from Traefik config; seeds store `taken` set |
