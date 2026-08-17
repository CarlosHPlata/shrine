# Contract: `HostPortStore` interface and `hostports.txt`

**Feature**: 023-publish-host-ports

## Interface (`internal/state/hostports.go`)

```go
type HostPortMap map[string]int // "team/app" → host port

type HostPortStore interface {
	AllocateHostPort(team, app string) (int, error)
	ClaimHostPort(team, app string, port int) error
	GetHostPort(team, app string) (int, error)
	ReleaseHostPort(team, app string) error
	ReleaseTeamHostPorts(team string) error
	ListHostPorts() (HostPortMap, error)
}
```

Sentinels: `ErrHostPortNotFound`, `ErrNoAvailableHostPorts`, `ErrHostPortTaken`.

## Behavioral contract

| Operation | Contract |
|---|---|
| `AllocateHostPort` | **Idempotent**: existing entry (any origin) is returned unchanged — this is the redeploy-stability guarantee (FR-008). New allocation: lowest free port in `30000–32767`, skipping reserved and persisted ports; persists before returning; `ErrNoAvailableHostPorts` on exhaustion with no state change. |
| `ClaimHostPort` | **Idempotent upsert** for the caller's own key (overwrite releases any previous value — the automatic→explicit switch, FR-010); `ErrHostPortTaken` if a *different* key holds the port. |
| `GetHostPort` | Read-only; `ErrHostPortNotFound` when absent. |
| `ReleaseHostPort` | Idempotent; absent key is a soft success (mirrors subnet release). |
| `ReleaseTeamHostPorts` | Removes every `team/*` entry; idempotent. |
| `ListHostPorts` | Read-only snapshot copy; callers cannot mutate store state through it. |

Concurrency: `sync.Mutex` for in-process serialization (same guarantee level as
`SubnetStore`; no cross-process locking).

Failure atomicity: every mutation writes via temp-file + `os.Rename`; on save
failure the in-memory maps roll back so memory never diverges from disk.

Reserved ports: supplied at construction (`NewHostPortStore(baseDir, reserved)`),
seed the taken-set, are never persisted, and are never allocatable or claimable.

## File format — `<stateDir>/hostports.txt`

```text
# team/app=port
media/jellyfin=30000
ops/dashboard=8080
```

- One `key=port` line per allocation; key is `team/app`.
- Sorted by key on every save (deterministic output; SC-005's byte-for-byte
  dry-run guarantee relies on this).
- Lines starting with `#` and malformed lines are skipped on load (forgiving
  parser, matching `subnets.txt`).
- Absent file = empty store; the file is created on first allocation.

## Lifecycle integration points

| Event | Store call | Site |
|---|---|---|
| Deploy, automatic mode | `AllocateHostPort` | Docker backend, `resolvePublishBinding` |
| Deploy, explicit mode | `ClaimHostPort` | Docker backend, `resolvePublishBinding` |
| Redeploy / recreate / teardown / publish removed | none — entry untouched | — |
| `shrine delete application` | `ReleaseHostPort` | new handler |
| `shrine delete team` | `ReleaseTeamHostPorts` | `handler.DeleteTeam`, beside subnet release |
| Dry-run | `ListHostPorts` only | `handler.DryRun` snapshot |
