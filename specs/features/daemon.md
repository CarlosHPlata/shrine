# Feature: Shrine Daemon + Remote CLI

**Status**: planned
**Goal**: Run `shrined` as a long-running process that reconciles manifests continuously, while keeping the `shrine` CLI working identically — locally via Unix socket, remotely via HTTPS.

---

## Context

Today the CLI is the whole tool: `cmd/` → `handler/` → `engine/` runs in one shot and exits. This works fine for manual deploys but means nothing happens unless a human runs a command. Converting to a daemon model gives:

- Auto-reconcile on manifest file changes (no `shrine deploy` needed after editing a YAML)
- Remote `shrine status` / `shrine deploy` from a laptop without SSH-ing to the server
- A foundation for future watch-mode, drift detection, and audit logging

The internal architecture (pluggable backends, idempotent engine, level-triggered reconciliation) was already built for this — the main gap is the client/server transport layer and the daemon lifecycle.

---

## Two-Mode CLI

The `shrine` binary stays identical. Transport is selected at startup:

| Condition | Transport | Auth |
|---|---|---|
| No `server:` in config | Unix socket (`/run/shrine/shrine.sock`) | OS file permissions (group `shrine`) |
| `server: https://...` in config | HTTPS | Bearer token |

Same commands, same flags, same output format in both modes.

---

## Architecture Overview

```
shrine (CLI)
  │
  ├─ local ──────► Unix socket ──────► shrined
  │                                      │
  └─ remote ─────► HTTPS ────► Traefik ──┘
                                (TLS termination)
```

`shrined` exposes one HTTP API server bound to the Unix socket. The same server is also reachable over TCP when remote access is enabled — Traefik terminates TLS and proxies to it, exactly like any other `kind: Application` in the platform. The daemon registers itself through Traefik automatically.

### Layer split

| Layer | Today | With daemon |
|---|---|---|
| `cmd/` | calls `handler.*` directly | calls `transport.Client` (socket or HTTPS) |
| `internal/handler/` | business logic functions | HTTP handlers (same logic, new wiring) |
| `internal/daemon/` | doesn't exist | server loop, reconcile loop, token middleware |
| `internal/transport/` | doesn't exist | `Client` interface; Unix and HTTPS implementations |

`internal/engine/`, `internal/planner/`, `internal/resolver/` — **unchanged**.

---

## New Commands

```bash
# Daemon lifecycle
shrine daemon start          # start in foreground (systemd ExecStart)
shrine daemon stop           # send SIGTERM to running daemon
shrine daemon status         # is it running? uptime, last reconcile
shrine daemon install        # write /etc/systemd/system/shrine.service and enable

# Token management (server-side, local only)
shrine token create --name laptop   # prints a new token once
shrine token list                   # id, name, created, last-used
shrine token revoke <id>
```

All existing commands (`deploy`, `apply`, `status`, `describe`, `teardown`, `generate`) are unchanged from the user's perspective.

---

## Reconcile Loop

The daemon watches `specsDir` with `fsnotify`. On any `.yml` change:

1. `planner.LoadDir` + `planner.Resolve` + `planner.Order` (plan the desired state)
2. Diff against last known plan (skip if identical)
3. `engine.ExecuteDeploy` (idempotent — safe to re-run)
4. Emit events to the observer stream

A fallback poll ticker (default 60 s) catches changes that fsnotify misses (NFS mounts, symlink targets, etc.).

Remote `shrine deploy` bypasses the debounce and triggers an immediate reconcile.

---

## Auth & Security

**Local**: Unix socket at `/run/shrine/shrine.sock`, owned by `root:shrine`, mode `0660`. Any user in group `shrine` can connect. No token needed.

**Remote**: Bearer token in `Authorization: Bearer <token>` header. Tokens are random 32-byte values stored in `<state-dir>/tokens.db` (line-delimited, same pattern as other state files). Token validation is middleware on every handler.

TLS is not handled by the daemon directly — Traefik terminates it. The daemon listens on a plain TCP port (default `127.0.0.1:7734`) that Traefik reaches over the Docker bridge. Operators who don't use Traefik can still reach the daemon via SSH tunnel + Unix socket.

**Client config** on a remote machine:
```yaml
# ~/.config/shrine/config.yml
server: https://shrine.home.lab
token: <token from shrine token create>
```

---

## Remote `shrine apply -f` Behaviour

When called from a remote machine, `shrine apply -f ./my-app.yml` streams the file content in the request body. The daemon writes it to a temp path, plans against its own `specsDir` as the resolution context, then deploys. This lets operators push a single manifest without SSH access to the server. The manifest must not have unresolvable `valueFrom` references — the daemon validates at plan time and returns the error to the CLI.

`shrine deploy` (no `-f`) always triggers a server-side reconcile against `specsDir`. The client sends no file content.

---

## Phased Implementation

| Phase | Deliverable | Gate |
|---|---|---|
| **A — Transport layer** | Unix socket server skeleton; CLI sends all commands via socket instead of direct call; no behaviour change locally | `go run . status` works via socket |
| **B — Handler migration** | `internal/handler/` functions become HTTP handlers; `cmd/` becomes thin client | All existing commands pass through the socket |
| **C — Daemon lifecycle** | `shrine daemon start/stop/status/install`; PID file; graceful shutdown on SIGTERM; systemd unit | `systemctl start shrine` deploys on boot |
| **D — Token auth** | `TokenStore`; token middleware; `shrine token` commands; HTTPS transport in CLI | Remote `shrine status` works with token |
| **E — Remote HTTPS** | Traefik route for daemon; CLI reads `server:` from config; TLS end-to-end | `shrine status` from laptop |
| **F — Reconcile loop** | fsnotify watch + poll fallback; debounce; `shrine daemon status` shows last reconcile time | Edit a manifest → container restarts without manual deploy |

Phases A–C can ship as a local-only daemon (no network exposure). D–E add remote access. F adds the GitOps-style reconcile loop. Each phase is independently useful and deployable.

---

## Files Added / Changed

### New files
```
cmd/daemon.go                        shrine daemon subcommand
cmd/token.go                         shrine token subcommand
internal/daemon/server.go            HTTP server (Unix socket + optional TCP)
internal/daemon/reconcile.go         fsnotify watch loop + poll fallback
internal/daemon/middleware.go        token auth middleware
internal/transport/client.go         Client interface
internal/transport/unix.go           Unix socket implementation
internal/transport/https.go          HTTPS implementation
internal/state/local/tokens.go       TokenStore (create, list, revoke, validate)
```

### Modified files
```
cmd/*.go                             replace direct handler calls with transport.Client calls
internal/handler/*.go                wrap existing functions as net/http handlers
internal/config/config.go            add Server, Token fields
```

### Unchanged
```
internal/engine/          no changes
internal/planner/         no changes
internal/resolver/        no changes
internal/manifest/        no changes
internal/state/local/     only tokens.go is new; existing stores untouched
```

---

## Open Questions

- **Daemon identity in Traefik**: Should the daemon register itself as a first-class `kind: Application` manifest (self-describing) or should `shrine daemon install` hard-code the route? Self-describing is elegant but requires the daemon to be running to register itself — a chicken-and-egg problem on first boot. Likely: hard-coded route written by `shrine daemon install`, documented as the bootstrapping contract.
- **Concurrent reconcile + manual deploy**: If a file-watch reconcile fires while a manual `shrine deploy` is in flight, the second run will block on the engine mutex. Acceptable for homelab scale; document it.
- **Multi-user tokens vs. single shared token**: For homelab a single admin token is probably fine. YAGNI on per-user RBAC unless explicitly requested.
