# Data Model: Integration Coverage for TLS Alias Routers and the Routing Finalize Phase

No product data changes. The "entities" are test artifacts: fixtures, scenarios, and the mapping back to the deferred tasks.

## Fixtures (`tests/testdata/deploy/`)

| Fixture | Status | App name | Primary domain | Aliases |
|---------|--------|----------|----------------|---------|
| `traefik-alias-tls` | NEW | `whoami-tls` | `whoami-tls.shrine.lab` | `{host: alias.shrine.lab, pathPrefix: /tls, stripPrefix: false, tls: true}` |
| `traefik-alias-tls-removed` | NEW | `whoami-tls` (same) | `whoami-tls.shrine.lab` | `{host: alias.shrine.lab, pathPrefix: /tls, stripPrefix: false}` |
| `traefik-alias-tls-mixed` | NEW | `whoami-tls-mixed` | `whoami-tls-mixed.shrine.lab` | `[0] {host: lan.shrine.lab, pathPrefix: /lan}`, `[1] {host: ext.shrine.lab, pathPrefix: /ext, stripPrefix: false, tls: true}` |
| `traefik-alias-prefix` | reused | `whoami-prefix` | `whoami-primary.shrine.lab` | one non-TLS alias |
| `traefik` | reused | `hello-eligible` (routed) + `hello-internal` (not routed) | `hello-eligible.shrine.lab` | none |

Each new fixture is `{app.yaml, team.yaml}`. `team.yaml` is a copy of `traefik-alias-prefix/team.yaml` (team `shrine-alias-test`). `app.yaml` follows `traefik-alias-prefix/app.yaml`: `image: traefik/whoami`, `port: 80`, `replicas: 1`, `networking.exposeToPlatform: true`.

## Generated routing file shape under assertion

`<routing-dir>/dynamic/<team>-<app>.yml`:

```yaml
http:
  routers:
    <team>-<app>:            # primary — always plain
      entryPoints: [web]
    <team>-<app>-alias-<i>:  # plain alias
      entryPoints: [web]
    <team>-<app>-alias-<j>:  # TLS alias
      entryPoints: [web, websecure]
      tls: {}
  services:
    <team>-<app>: …
```

- **Plain router**: `entryPoints == [web]`, no `tls` key.
- **TLS router**: `entryPoints == [web, websecure]`, `tls` key present with an empty mapping.
- All routers of one app have `service == <team>-<app>`.

## Scenarios (appended to `TestTraefikPlugin`)

| # | Scenario name | Fixture(s) | `port` / `tlsPort` | Deploys | Story |
|---|---------------|-----------|--------------------|---------|-------|
| S1 | should publish alias router with tls block when alias sets tls: true | `tls` | 8119 / 8447 | 1 | US1 |
| S2 | should give only the opted-in alias a tls router when aliases are mixed | `tls-mixed` | 8120 / 8448 | 1 | US2 |
| S3 | should revert alias router to plain when tls is removed and re-deployed | `tls` → `tls-removed` | 8121 / 8449 | 2 | US2 |
| S4 | should keep non-tls alias routing byte-stable and silent about tls | `prefix` | 8122 / unset | 3 | US3 |
| S5 | should exit non-zero and attribute the error to routing finalize when the gateway cannot start | `traefik` | 8123 / unset, `image: localhost:1/shrine-test/unpullable-gateway:0` | 1 (fails) | US4 |
| S6 | should print the finalize route operation last among route operations on dry-run | `traefik` | 8124 / unset | 1 dry-run | US5 |
| S7 | should leave static config and gateway container in place after the finalize phase | `traefik` | 8125 / unset | 1 | US5 |

State transitions exercised:

- **S3**: TLS file on disk → operator removes file → regenerated as plain.
- **S4**: generated → preserved (unchanged) → removed → regenerated (identical).
- **S5**: app steps succeed → finalize fails → exit 1, gateway container absent.

## Traceability: deferred task → scenario

| Deferred task | Delivered by | Notes |
|---------------|--------------|-------|
| 012 T016 (fixture) | `traefik-alias-tls` | Path corrected to `tests/testdata/deploy/`; `.yaml` naming; untagged image |
| 012 T017 | S1 | |
| 012 T023 (fixture) | `traefik-alias-tls-mixed` | |
| 012 T024 | S2 + S3 | Revert split out and corrected for the preserve policy (file removal, twin fixture) |
| 012 T026 | S4 | Plus a regenerate-and-compare step (research D5) |
| 018 T012 | S5 | Failure provoked by unpullable gateway image instead of an injected backend (research D1) |
| 018 T014 | S7 | |
| 018 T018 | S6 | Asserts ordering ("after the last WriteRoute, exactly once") rather than "output ends with" |
| 018 T019 | S7 (log assertions) | Effects-based: `routing.finalize` started/info entries in `shrine.log` |

Spec requirement coverage: FR-001/002/003 → S1; FR-004/005 → S2; FR-006 → S3; FR-007/008 → S4; FR-009/010/011 → S5; FR-012 → S6; FR-013 → S7; FR-014/015/016 → all (harness + research Part 3); FR-017 → doc updates in plan.
