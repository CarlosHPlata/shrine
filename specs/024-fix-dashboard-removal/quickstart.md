# Quickstart: Verify Dashboard Removal on Redeploy

**Feature**: `024-fix-dashboard-removal`

Manual end-to-end verification of the fix, mirroring the reproduction steps in GitHub issue #35. Requires Docker and a built `shrine` binary.

## 1. Set up an isolated environment

```bash
work=$(mktemp -d)
mkdir -p "$work/config" "$work/state" "$work/traefik"
go build -o "$work/shrine" .

cat > "$work/config/config.yml" <<EOF
plugins:
  gateway:
    traefik:
      routing-dir: $work/traefik
      port: 8180
      dashboard:
        port: 8181
        username: admin
        password: hunter2
EOF
```

Point `--path` at any team fixture (e.g. `tests/integration/fixtures/traefik`) or your own manifests.

## 2. Deploy with the dashboard configured

```bash
"$work/shrine" deploy --config-dir "$work/config" --state-dir "$work/state" --path <manifests>
ls "$work/traefik/dynamic/"
```

Expected: `__shrine-dashboard.yml` exists; deploy output includes `gateway.dashboard.generated`; `curl -u admin:hunter2 http://localhost:8181/dashboard/` reaches the dashboard.

## 3. Remove the dashboard block and redeploy

```bash
cat > "$work/config/config.yml" <<EOF
plugins:
  gateway:
    traefik:
      routing-dir: $work/traefik
      port: 8180
EOF
"$work/shrine" deploy --config-dir "$work/config" --state-dir "$work/state" --path <manifests>
```

Expected (this is the fix):
- Deploy output includes `gateway.dashboard.removed` with the file path.
- `ls "$work/traefik/dynamic/"` no longer lists `__shrine-dashboard.yml`; per-app route files are untouched.
- `curl -u admin:hunter2 http://localhost:8181/dashboard/` fails — the port is no longer published and the router is gone. The old credentials grant access to nothing.

Before the fix, the file survived and the dashboard kept serving with the old credentials.

## 4. Idempotence and dry-run checks

```bash
# A third deploy with no dashboard: silent — no removal event, no error.
"$work/shrine" deploy --config-dir "$work/config" --state-dir "$work/state" --path <manifests>

# Dry-run never touches the file: re-add the dashboard block, deploy, remove it again, then:
"$work/shrine" deploy --dry-run --config-dir "$work/config" --state-dir "$work/state" --path <manifests>
ls "$work/traefik/dynamic/"   # __shrine-dashboard.yml still present after dry-run
```

## 5. Automated equivalents

- Unit: `go test ./internal/plugins/gateway/traefik/`
- Dead-code check: `grep -rn "func (p \*Plugin) portBindings" internal/` returns nothing.
- Integration (CI is the gate; compile-check locally): `go vet -tags integration ./tests/integration/...`

## 6. Cleanup

```bash
docker rm -f platform.traefik
rm -rf "$work"
```
