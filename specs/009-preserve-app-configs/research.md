# Phase 0 Research — Preserve Operator-Edited Per-App Routing Files

**Date**: 2026-05-01

This file resolves three load-bearing design questions that, if answered wrongly, would force rework during implementation. It also records two reuse opportunities verified against the current `main`.

---

## Decision 1: Where does the observer live for `WriteRoute` / `RemoveRoute`?

**Decision**: On the traefik `RoutingBackend` struct (`internal/plugins/gateway/traefik/routing.go`). The struct is extended with an `observer engine.Observer` field. The plugin's existing observer (`Plugin.observer`, set in `New`) is passed through `Plugin.RoutingBackend()` into the constructed `RoutingBackend`.

**Rationale**: This matches the established pattern in the same package — `generateStaticConfig` (`config_gen.go:29`) already accepts an `engine.Observer` and emits `gateway.config.preserved` / `gateway.config.generated`. Putting the observer on the struct lets `WriteRoute` and `RemoveRoute` emit the parallel `gateway.route.*` events without changing the `engine.RoutingBackend` interface and without requiring every call site (engine, dryrun, future backends) to thread an observer through.

**Alternatives considered**:

- **(a) Extend the `engine.RoutingBackend` interface** to accept an `Observer` per call (e.g., `WriteRoute(op WriteRouteOp, observer Observer) error`). Rejected: it forces the dryrun backend (`internal/engine/dryrun/dry_run_routing.go`) to grow a no-op parameter, couples the routing-backend contract to the gateway-warning event taxonomy, and ripples to any future routing backend (e.g., a hypothetical NGINX or Caddy backend). The benefit — a backend without internal observer state — is not worth the contract surface.
- **(b) Return a structured `WriteRouteResult` with events** for the engine to emit. Rejected: every call site at `engine.go:189` and `engine.go:RemoveRoute-once-wired` would need to translate result-events into `Observer.OnEvent` calls. The engine grows a translation layer it does not need; the backend grows a result type it does not need.
- **(c) Emit events through a package-level singleton observer**. Rejected on Constitution IV (Simplicity & YAGNI): singletons are an abstraction that needs ≥3 concrete usages to justify, and there is exactly one consumer (the traefik backend). Also breaks test isolation — every test would need to swap the singleton.

**Implication for tests**: `routing_test.go` introduces a `recordingObserver` mirroring the one already at `config_gen_test.go:16-19`. Existing tests in `routing_test.go` that construct `&RoutingBackend{routingDir: "/fake"}` are extended to `&RoutingBackend{routingDir: "/fake", observer: rec}` where `rec` is a fresh recorder per test.

---

## Decision 2: What is the error-vs-warning contract for `WriteRoute` / `RemoveRoute` after this change?

**Decision**: A non-nil error return continues to mean "abort this app's deploy step" (preserves today's engine behavior at `engine.go:189`). The new "preserved", "stat-error", and "orphan-warn" outcomes internally emit a warning- or info-level event via the struct's observer and **return nil** to the engine. They are not errors from the engine's perspective and do not gate deploy success.

**The split**:

| Outcome | Event emitted | Return |
|---|---|---|
| File absent on deploy → fresh write succeeds | `gateway.route.generated` (Info) | nil |
| File absent on deploy → mkdir or `os.WriteFile` fails | (none — error propagates) | non-nil |
| File present on deploy → write skipped | `gateway.route.preserved` (Info) | nil |
| Stat returns non-`IsNotExist` error on deploy | `gateway.route.stat_error` (Warning) | nil |
| File present on teardown → not removed | `gateway.route.orphan` (Warning) | nil |
| File absent on teardown | (none) | nil |

**Rationale**: This is the load-bearing implementation choice for FR-007 and FR-011 (`SC-007`). The user's clarification was explicit: gateway-file outcomes do not gate deploy success once the first per-app template has shipped. Translating that into code means the methods must be able to internally swallow specific failure modes and surface them as warnings without changing their return signature. The method signatures are unchanged — `error` still flows for true I/O failures on first writes (which the operator must see surface as a deploy failure).

**Alternatives considered**:

- **(a) Add a `WarningError` type** that the engine recognizes and downgrades. Rejected: Constitution IV — no other backend needs this distinction, so it's an abstraction with one usage. Also adds type-assertion noise at every error return.
- **(b) A callback-style "emit-then-fail"** API — `WriteRoute(op, onWarning func(Event)) error`. Rejected: Constitution VII — uglier than two return paths; readers have to trace whether a callback fires before or after the error.

**Implication for `engine.go:189`**: no change. `engine.Routing.WriteRoute(routingOp)` still returns `error`. The `emitErr` path at engine.go:190 only fires for true I/O failures. Per-app preserve-skips and stat-errors never reach this branch.

---

## Decision 3: How does `RemoveRoute` get an observer-event surface, given it currently has no callers?

**Decision**: Wire `engine.teardownKind` (engine.go:216) to call `engine.Routing.RemoveRoute(team, step.Name)` for `ApplicationKind` steps after `RemoveContainer` succeeds, gated on `engine.Routing != nil` (mirrors the `engine.Routing != nil` gate at engine.go:165 for the deploy path). The `host` argument passed to `RemoveRoute` is the step's `step.Name` — i.e., the application name — because the per-app file name is constructed from `(team, name)` via `routeFileName(team, name)` in `internal/plugins/gateway/traefik/config_gen.go:103`.

**Audit confirmation that `RemoveRoute` is currently uncalled**:

```text
$ grep -rn 'RemoveRoute\b' internal/ cmd/
internal/engine/backends.go:71:    RemoveRoute(team string, host string) error
internal/engine/dryrun/dry_run_routing.go:20:func (d *DryRunRoutingBackend) RemoveRoute(...
internal/engine/dryrun/dry_run_routing.go:21:    fmt.Fprintf(d.Out, "[ROUTE]  RemoveRoute: ...
internal/plugins/gateway/traefik/routing.go:97:func (r *RoutingBackend) RemoveRoute(...
```

Three definition sites; zero call sites. This is significant because it means **today's behavior is already that per-app files are orphaned silently on teardown**. The user's chosen FR-009 ("preserve the file but emit a warning") is therefore a small additive change: we add the warning that's missing today, and we make the silent orphan an observable orphan.

**Rationale**: The Constitution VI rule that state updates must happen *after* Docker operations is preserved — the new `RemoveRoute` call is sequenced after `RemoveContainer`. The Constitution III separation is preserved — the orphan-detection logic lives in the traefik backend, not in the engine; the engine just calls the backend and lets it decide whether to emit a warning.

**Alternatives considered**:

- **(a) Add a new "audit orphan files" CLI command** (e.g., `shrine audit gateway`). Rejected: FR-005 forbids new commands or flags. Also a new command would surface the warning only when explicitly run; the wiring approach surfaces it on every teardown automatically.
- **(b) Emit the orphan warning on the next deploy** when an unreferenced per-app file is found in the dynamic directory. Rejected: "next deploy" may never happen for an app that's been torn down (the team may move off the app entirely), so the warning fires too late or never. Also requires the engine to know which per-app files belong to which apps, which requires either a directory listing (slow) or a state file (Constitution VI: state files are caches; we'd be growing a new one).

**Implication for the dryrun backend**: `DryRunRoutingBackend.RemoveRoute` (dry_run_routing.go:20) already prints `[ROUTE]  RemoveRoute: domain=%s (team=%s)`. The new engine wiring will exercise this print path on `shrine teardown --dry-run`, producing one new line per torn-down app. This is a behavior addition for dry-run output but is the correct behavior — operators running `--dry-run` on a teardown should see that the routing teardown step would be invoked.

**Implication for `cmd/cmd_test.go`**: there is an existing assertion at `cmd/cmd_test.go:71` that asserts dry-run apply output contains `"[ROUTE]  WriteRoute: domain=test.home.lab → test-app:80"`. There is no existing dry-run teardown assertion that would break; the new `[ROUTE]  RemoveRoute: ...` line is additive. If the test suite includes a dry-run teardown case, it will need to assert the new line; this falls out at test-update time during Phase 2.

---

## Reuse Opportunity 1: `isStaticConfigPresent` already implements the FR-008 existence semantics

**Current**: `internal/plugins/gateway/traefik/config_gen.go:18-27`:

```go
var lstatFn = os.Lstat

func isStaticConfigPresent(routingDir string) (bool, error) {
    path := filepath.Join(routingDir, "traefik.yml")
    if _, err := lstatFn(path); err != nil {
        if os.IsNotExist(err) {
            return false, nil
        }
        return false, fmt.Errorf("traefik plugin: checking traefik.yml at %q: %w", path, err)
    }
    return true, nil
}
```

This is exactly what FR-008 requires: `Lstat` (no symlink follow), any entry counts as present, non-`IsNotExist` errors surface. The body is path-agnostic except for the hardcoded `traefik.yml` join.

**Generalization**: extract the body into `isPathPresent(path string) (bool, error)`; `isStaticConfigPresent` becomes a one-liner wrapper:

```go
func isPathPresent(path string) (bool, error) {
    if _, err := lstatFn(path); err != nil {
        if os.IsNotExist(err) {
            return false, nil
        }
        return false, fmt.Errorf("checking path %q: %w", path, err)
    }
    return true, nil
}

func isStaticConfigPresent(routingDir string) (bool, error) {
    return isPathPresent(filepath.Join(routingDir, "traefik.yml"))
}
```

The `lstatFn` test seam (used by `config_gen_test.go`) is preserved — it's still the single injection point for the entire package. Existing assertions on `gateway.config.preserved` / `gateway.config.generated` stand byte-for-byte.

**Constitution IV check**: three call sites after this change — `isStaticConfigPresent` (existing), `WriteRoute`'s pre-check (new), `RemoveRoute`'s orphan-detect (new). Exactly the threshold for extract-rule. Not premature.

---

## Reuse Opportunity 2: `recordingObserver` test pattern from `config_gen_test.go`

`internal/plugins/gateway/traefik/config_gen_test.go:16-19` already defines:

```go
type recordingObserver struct {
    events []engine.Event
}

func (r *recordingObserver) OnEvent(e engine.Event) {
    r.events = append(r.events, e)
}
```

This pattern is reused directly in `routing_test.go` for the new event-emission tests. Either move the type into a `traefik_test.go`-level shared helper file, or define it locally in `routing_test.go` (Go tests in the same package can share unexported types if both are `_test.go` files, but only if compiled together — they are, since both are in `package traefik`). **Decision**: keep the local definition in `config_gen_test.go` and add an identical local definition in `routing_test.go`. Two definitions in the same `_test.go` package is a compile error, so the **actual** decision is to move the type into a shared `helpers_test.go` file inside the same package. The move is mechanical and Constitution VII (DRY) requires it once there are two consumers. Per [memory: integration tests are isolated from internal packages], this internal package-level helper does not affect the integration test suite.

---

## Audit Verification Matrix

| FR | Status before this feature | Verification source |
|---|---|---|
| FR-001 (existence check before write) | **Gap** — `WriteRoute` (`routing.go:37-95`) writes unconditionally. | `routing.go:94` calls `writeFileFn` with no pre-check. |
| FR-002 (write when absent) | Implicit-Pass (covered by current happy-path tests). | `routing_test.go:164-264` cover the unconditional-write path. New behavior (write only when absent) is the same on first deploy. |
| FR-003 (no write when present) | **Gap** — same as FR-001. | Same. |
| FR-004 (mkdir dynamic dir) | Verified | `routing.go:38` `mkdirAllFn(r.dynamicDir(), 0o755)` runs before any write. |
| FR-005 (no new flag/env/manifest) | N/A — design constraint, not code. | Plan section "Project Structure" confirms zero new fields. |
| FR-006 (preserved log signal) | **Gap** | No existing event for per-app file outcomes. Spec 004's `gateway.config.preserved` is gateway-wide. |
| FR-007 (stat-error → warn, no abort) | **Gap** | `WriteRoute` has no stat call today; the Lstat happens only in `isStaticConfigPresent`. |
| FR-008 (Lstat semantics) | Reuse opportunity (see above) | `config_gen.go:16` already uses `Lstat`, satisfies symlink/non-regular-file requirements. |
| FR-009 (orphan warn on teardown) | **Gap** — `RemoveRoute` is uncalled today. | Audit above; no call sites. |
| FR-010 (per-app independence) | Implicit-Pass | The engine loops over steps at `engine.go:65-79`; each step's `WriteRoute` failure aborts only that app today (per `emitErr` at engine.go:190 returning the error from `deployApplication`). After this feature, preserve-skips and stat-errors return nil, so the loop continues unaffected. |
| FR-011 (deploy success governed by Docker) | **Gap** — needs Decision 2 to land. | The error-vs-warning contract is the implementation. |

**Summary**: 6 Gaps to close (FR-001, 003, 006, 007, 009, 011), 1 Reuse opportunity (FR-008 via `isPathPresent` extraction), 4 Implicit-Pass items (FR-002, 004, 005, 010).
