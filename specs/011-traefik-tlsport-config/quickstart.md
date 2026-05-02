# Quickstart: Verifying the Traefik `tlsPort` Config Option

**Feature**: 011-traefik-tlsport-config
**Audience**: An operator validating that the feature works end-to-end on their host, OR a contributor running the integration suite locally before pushing.

This script walks the User Story 1 acceptance scenarios on a real host. It is not a substitute for the integration suite (`make test-integration`), which is the canonical gate per Constitution Principle V — but it is the fastest way to manually convince yourself the feature behaves as specified.

## Prerequisites

- Docker daemon running on the host.
- A built `shrine` binary (`make build` from the repo root, or `go build -o bin/shrine ./cmd/shrine`).
- A scratch directory for the test config and routing dir; the script below uses `/tmp/shrine-tlsport`.
- No process currently bound to host port `443` (else the container start will fail; pick another `tlsPort` value such as `8443`).

## Step 1 — Clean deploy with `tlsPort: 443`

Create a config and deploy:

```bash
mkdir -p /tmp/shrine-tlsport/config /tmp/shrine-tlsport/state /tmp/shrine-tlsport/manifests /tmp/shrine-tlsport/traefik

cat > /tmp/shrine-tlsport/config/config.yml <<'YAML'
plugins:
  gateway:
    traefik:
      routing-dir: /tmp/shrine-tlsport/traefik
      port: 8081
      tlsPort: 443
YAML

# A minimal team manifest so deploy has something to apply
cat > /tmp/shrine-tlsport/manifests/team.yml <<'YAML'
apiVersion: shrine/v1
kind: Team
metadata:
  name: tlsport-demo
YAML

./bin/shrine apply teams \
  --path /tmp/shrine-tlsport/manifests \
  --state-dir /tmp/shrine-tlsport/state

./bin/shrine deploy \
  --config-dir /tmp/shrine-tlsport/config \
  --state-dir /tmp/shrine-tlsport/state \
  --path /tmp/shrine-tlsport/manifests
```

**Expected deploy output** includes:

- `📝 Generated default traefik.yml: …/traefik.yml`
- `✨ Creating fresh container: platform.traefik`
- `✅ Container platform.traefik is running`

## Step 2 — Verify the host port is published to container 443/tcp

```bash
docker inspect platform.traefik --format '{{json .HostConfig.PortBindings}}'
```

**Expected**: a JSON object containing `"443/tcp": [{"HostIp": "", "HostPort": "443"}]`.

```bash
docker inspect platform.traefik --format '{{range $p, $b := .NetworkSettings.Ports}}{{$p}} -> {{(index $b 0).HostPort}}{{"\n"}}{{end}}'
```

**Expected**: a line `443/tcp -> 443` alongside the existing `8081/tcp -> 8081` line.

## Step 3 — Verify the `websecure` entrypoint shape

```bash
cat /tmp/shrine-tlsport/traefik/traefik.yml
```

**Expected** — exactly one `websecure` entrypoint with only an `address` field (no `tls`, `http.tls`, `certResolver`, etc.):

```yaml
entryPoints:
  web:
    address: :8081
  websecure:
    address: :443
providers:
  file:
    directory: /etc/traefik/dynamic
    watch: true
```

## Step 4 — Verify TCP-level reachability (TLS layer is operator-owned, see FR-010)

```bash
# The connection must complete TCP-handshake. The TLS handshake will use
# Traefik's default behaviour (likely a self-signed cert) — that is operator-
# managed territory and out of scope for this feature.
nc -zv 127.0.0.1 443
```

**Expected**: `Connection to 127.0.0.1 443 port [tcp/*] succeeded!`.

## Step 5 — Validation rejection (collision)

```bash
cat > /tmp/shrine-tlsport/config/config.yml <<'YAML'
plugins:
  gateway:
    traefik:
      routing-dir: /tmp/shrine-tlsport/traefik
      port: 443
      tlsPort: 443
YAML

./bin/shrine deploy \
  --config-dir /tmp/shrine-tlsport/config \
  --state-dir /tmp/shrine-tlsport/state \
  --path /tmp/shrine-tlsport/manifests
```

**Expected**: deploy fails with an error message that names `tlsPort` and `port`, e.g. `traefik plugin: tlsPort 443 collides with port 443`. Container is **not** modified.

Restore the working config from Step 1 before continuing.

## Step 6 — Drift detection on `tlsPort` change

```bash
sed -i 's/tlsPort: 443/tlsPort: 8443/' /tmp/shrine-tlsport/config/config.yml

./bin/shrine deploy \
  --config-dir /tmp/shrine-tlsport/config \
  --state-dir /tmp/shrine-tlsport/state \
  --path /tmp/shrine-tlsport/manifests
```

**Expected**: deploy output includes `🔄 Image changed for platform.traefik, replacing container...` (the existing `container.recreate` event fires because the new port-binding spec changes the config hash).

```bash
docker inspect platform.traefik --format '{{json .HostConfig.PortBindings}}'
```

**Expected**: `443/tcp` mapped to host port `8443` now, not `443`.

## Step 7 — Backward-compat sanity check (`tlsPort` removed)

```bash
sed -i '/tlsPort:/d' /tmp/shrine-tlsport/config/config.yml

./bin/shrine deploy \
  --config-dir /tmp/shrine-tlsport/config \
  --state-dir /tmp/shrine-tlsport/state \
  --path /tmp/shrine-tlsport/manifests

cat /tmp/shrine-tlsport/traefik/traefik.yml
docker inspect platform.traefik --format '{{json .HostConfig.PortBindings}}'
```

**Expected**:
- The container is recreated.
- `traefik.yml` no longer contains a `websecure` entrypoint.
- `PortBindings` no longer contains a `443/tcp` mapping.

## Step 8 — Operator-preserved `traefik.yml` warning (FR-008)

```bash
# Re-set tlsPort, then hand-edit traefik.yml to remove the websecure entrypoint
# (simulating an operator who has preserved their own static config).
cat >> /tmp/shrine-tlsport/config/config.yml <<'YAML'
      tlsPort: 443
YAML

# Hand-write a preserved traefik.yml WITHOUT a websecure entrypoint:
cat > /tmp/shrine-tlsport/traefik/traefik.yml <<'YAML'
entryPoints:
  web:
    address: :8081
providers:
  file:
    directory: /etc/traefik/dynamic
    watch: true
YAML

./bin/shrine deploy \
  --config-dir /tmp/shrine-tlsport/config \
  --state-dir /tmp/shrine-tlsport/state \
  --path /tmp/shrine-tlsport/manifests
```

**Expected**: deploy succeeds; output includes both:
- `📄 Preserving operator-owned traefik.yml: …`
- `⚠️  tlsPort set but traefik.yml is missing websecure entrypoint at … — …`

The container's `443/tcp` host mapping is still applied (per FR-007); the warning is purely advisory.

## Cleanup

```bash
docker rm -f platform.traefik
rm -rf /tmp/shrine-tlsport
```

## Pointer to the canonical gate

These manual steps mirror the Phase-2 integration scenarios in `tests/integration/traefik_plugin_test.go` (added by `/speckit-tasks`). Run them with:

```bash
make test-integration
# or:
go test -tags integration ./tests/integration/... -run TestTraefikPlugin
```

The integration suite is the canonical gate per Constitution Principle V; this quickstart is a manual sanity-check script, not a substitute.
