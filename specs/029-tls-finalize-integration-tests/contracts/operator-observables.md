# Contract: Operator-Visible Observables Pinned by This Feature

This feature adds no interface. It pins existing ones. Each row is something an operator can see today (verified in [research.md](../research.md) Part 1) and the scenario that now fails if it changes. Changing any of these later is a product decision that must update the scenario deliberately.

## Generated routing file — `<routing-dir>/dynamic/<team>-<app>.yml`

| Observable | Value | Scenario |
|------------|-------|----------|
| Alias with `tls: true` → `http.routers.<team>-<app>-alias-<i>.entryPoints` | `[web, websecure]` | S1, S2 |
| Alias with `tls: true` → `…alias-<i>.tls` | present, empty mapping | S1, S2 |
| Alias without `tls` → `entryPoints` / `tls` | `[web]` / key absent | S2, S3, S4 |
| Primary router → `entryPoints` / `tls` | `[web]` / key absent | S1, S2 |
| Every router's `service` | `<team>-<app>` | S2 |
| After removing `tls: true`, removing the file, redeploying | alias router is plain | S3 |
| Unchanged redeploy | file bytes identical | S4 |
| File removed and regenerated from the same manifest | file bytes identical | S4 |
| Non-TLS manifest | file contains neither `websecure` nor `tls:` | S4 |

## Deploy stdout

| Observable | Value | Scenario |
|------------|-------|----------|
| Per-alias TLS marker | alias entry in the `↳ Aliases:` line ends with ` (tls)` | S1 (present); S3 second deploy, S4 (absent) |
| Missing-websecure alias warning | `alias tls: true but websecure entrypoint missing` | S1 (absent, `tlsPort` set); S4 (absent, no TLS alias) |
| Finalize failure | line containing `Error [routing.finalize]` | S5 |
| Per-app step ran before the failure | `Deploying Application: hello-eligible` | S5 |
| Dry-run finalize operation | `[ROUTE]  Finalize` exactly once, after the last `[ROUTE]  WriteRoute` | S6 |

## Exit code

| Condition | Exit code | Scenario |
|-----------|-----------|----------|
| Routing finalize fails | non-zero | S5 |
| Dry-run with gateway configured | zero | S6 |

## Log file — `<state-dir>/logs/shrine.log`

| Observable | Value | Scenario |
|------------|-------|----------|
| Finalize ran (success) | `[started] routing.finalize` and `[info] routing.finalize` | S7 |
| Finalize failed | `[error] routing.finalize` | S5 |

## Filesystem and containers

| Condition | Observable | Scenario |
|-----------|------------|----------|
| Successful deploy with gateway configured | `<routing-dir>/traefik.yml` exists; `platform.traefik` running | S7 |
| Finalize fails at gateway start | application container running; its dynamic route file exists; `platform.traefik` does not exist | S5 |
| Dry-run | no `platform.traefik`; no `traefik.yml`; no `dynamic/` | S6 |

## Explicit non-contracts

Not asserted, deliberately:

- The wording of the underlying image-pull error inside the finalize failure (Docker-owned text).
- Whether stderr carries a message on failure.
- That `[ROUTE]  Finalize` is the final line of dry-run stdout (only its position relative to other route operations).
- Any stdout line for a *successful* finalize — none exists today.
