# Quickstart: Verifying the Dashboard Fix

**Feature**: Fix Traefik Dashboard Generated in Static Config
**Audience**: Operators verifying the fix on a homelab host; reviewers reproducing the bug and the fix manually.

## Prereqs

- A host with Docker running and `shrine` built from this branch (`010-fix-dashboard-static-config`).
- A writable directory for Traefik routing files (referred to below as `$ROUTING_DIR`). The integration tests use a per-test temp dir; for a manual run use something like `/var/lib/shrine/traefik` or any path the user account can write.

## Scenario A — clean host, dashboard works on first deploy (User Story 1)

1. Ensure no prior Traefik state on the host:
   ```sh
   rm -rf "$ROUTING_DIR"
   docker rm -f platform.traefik 2>/dev/null
   ```
2. Write a Shrine config that enables the Traefik plugin with a dashboard password:
   ```yaml
   plugins:
     gateway:
       traefik:
         routing-dir: /absolute/path/to/$ROUTING_DIR
         port: 80
         dashboard:
           port: 8080
           username: admin
           password: hunter2
   ```
3. Run `shrine deploy --config-dir /path/to/config --path /path/to/manifests`.
4. **Expected file shape** after the deploy:
   - `$ROUTING_DIR/traefik.yml` exists and contains **no** `http:` block. Its top-level keys are `entryPoints`, `api`, `providers`.
   - `$ROUTING_DIR/dynamic/__shrine-dashboard.yml` exists and contains the dashboard router and `dashboard-auth` middleware under a single `http:` block.
5. **Expected dashboard reachability**:
   ```sh
   curl -i http://localhost:8080/dashboard/
   ```
   Returns `HTTP/1.1 401 Unauthorized` with a `WWW-Authenticate: Basic …` header — **not** `404 page not found`.
   ```sh
   curl -i -u admin:hunter2 http://localhost:8080/dashboard/
   ```
   Returns `HTTP/1.1 200 OK` with the Traefik dashboard HTML.

## Scenario B — operator-edited dashboard file is preserved across redeploys (User Story 3)

1. Run scenario A first.
2. Edit `$ROUTING_DIR/dynamic/__shrine-dashboard.yml` — for example, add a comment line at the top, or replace the router rule with a tighter one:
   ```yaml
   # operator: locked dashboard to internal subnet
   http:
     middlewares:
       dashboard-auth:
         basicAuth:
           users:
             - "admin:{SHA}…"
       dashboard-allowlist:
         ipAllowList:
           sourceRange:
             - 192.168.0.0/16
     routers:
       dashboard:
         rule: "PathPrefix(`/dashboard`) || PathPrefix(`/api`)"
         service: api@internal
         entryPoints: [traefik]
         middlewares: [dashboard-auth, dashboard-allowlist]
   ```
3. Run `shrine deploy …` again with no Shrine-side configuration changes.
4. **Expected**:
   - The deploy output contains a `gateway.dashboard.preserved` event (info-level) naming the file path.
   - `$ROUTING_DIR/dynamic/__shrine-dashboard.yml` is **byte-identical** to the operator-edited version.
5. To force regeneration after rotating the dashboard password in Shrine config:
   ```sh
   rm "$ROUTING_DIR/dynamic/__shrine-dashboard.yml"
   shrine deploy …
   ```
   The next deploy regenerates the file with the current credentials and emits `gateway.dashboard.generated`.

## Scenario C — pre-existing buggy `traefik.yml` triggers the warning (FR-010)

1. Pre-stage a static file that simulates the artefact of an earlier buggy Shrine version:
   ```sh
   mkdir -p "$ROUTING_DIR" "$ROUTING_DIR/dynamic"
   cat > "$ROUTING_DIR/traefik.yml" <<'EOF'
   entryPoints:
     web:
       address: ":80"
     traefik:
       address: ":8080"
   api:
     dashboard: true
   providers:
     file:
       directory: /etc/traefik/dynamic
       watch: true
   http:
     middlewares:
       dashboard-auth:
         basicAuth:
           users:
             - "admin:{SHA}old-hash"
     routers:
       dashboard:
         rule: "PathPrefix(`/dashboard`) || PathPrefix(`/api`)"
         service: api@internal
         entryPoints: [traefik]
         middlewares: [dashboard-auth]
   EOF
   ```
2. Run `shrine deploy …` with the same config as scenario A.
3. **Expected**:
   - The deploy output contains a `gateway.config.preserved` event (the static file was kept) **and** a `gateway.config.legacy_http_block` warning event naming the file path and a one-line cleanup hint.
   - `$ROUTING_DIR/traefik.yml` is byte-identical to the pre-staged content (Shrine did not modify it).
   - `$ROUTING_DIR/dynamic/__shrine-dashboard.yml` exists and contains the current credentials from Shrine config.
   - The dashboard is reachable on `http://localhost:8080/dashboard/` because Traefik silently drops the legacy `http:` block in the static file and serves the dynamic file's router.
4. To clear the warning, remove the `http:` block from `$ROUTING_DIR/traefik.yml` by hand and redeploy. The next deploy emits no `legacy_http_block` event.

## Scenario D — dashboard not configured, no dashboard surface generated (FR-005)

1. Use a Shrine config with the Traefik plugin but no `dashboard:` block.
2. Run `shrine deploy …`.
3. **Expected**:
   - `$ROUTING_DIR/traefik.yml` exists with no `http:` block, no `api:` key, and no `entryPoints.traefik`.
   - `$ROUTING_DIR/dynamic/__shrine-dashboard.yml` does **not** exist.
   - No `gateway.dashboard.*` events are emitted.

## Smoke check before merging

Minimum acceptance for the fix to be considered done locally:

```sh
go test ./internal/plugins/gateway/traefik/...
make test-integration
```

Both must pass. The unit suite covers the new helpers' branch logic with stubbed `lstatFn`; the integration suite exercises the three new scenarios end-to-end with a real Docker daemon and a real shrine binary built in `TestMain`.
