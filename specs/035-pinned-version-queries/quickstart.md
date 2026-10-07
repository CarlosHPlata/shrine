# Quickstart: Pinned Versions in `get`, `describe`, and `status`

**Feature**: 035-pinned-version-queries | **Date**: 2026-10-07

A manual round-trip over the behaviour this feature adds. Steps 1 and 2 need no Docker daemon. Steps 3 to 7 need a daemon and a loopback registry, which is what the CI integration suite automates; do not run them on a daemon another agent shares.

## 0. Pre-flight

```bash
go build -o shrine .
go test ./...
go vet -tags integration ./tests/integration/...
export STATE=$(mktemp -d) SPECS=$(mktemp -d)
./shrine apply teams --path tests/testdata/deploy/team --state-dir "$STATE"
```

## 1. Legacy and manifest-owned records are unchanged, and `describe` survives without a runtime

Seed a record by hand, the way `SeedLegacyDeploymentRecords` does:

```bash
mkdir -p "$STATE/shrine-deploy-test"
echo 'Application whoami 0123456789abcdef 3a7b - -' > "$STATE/shrine-deploy-test/deployments.txt"
./shrine get deployed --state-dir "$STATE"
./shrine describe app whoami --state-dir "$STATE"; echo "exit=$?"
```

Expect the row to show `-` in VERSION, and `describe` to exit zero with `Image:        -`, `Pull policy:  -`, no `Pinned:` line, and `Running image: unavailable (…)`.

## 2. A `Pinned` record reads its pin from state alone

```bash
D=sha256:3f2a9c1b4d7e0000000000000000000000000000000000000000000000000000
echo 'Application api 0123456789abcdef 3a7b ghcr.io/me/api Pinned' > "$STATE/shrine-deploy-test/deployments.txt"
echo "Application api ghcr.io/me/api:latest ghcr.io/me/api@$D 2026-10-07T10:42:17Z" > "$STATE/shrine-deploy-test/pins.txt"
./shrine get deployed --state-dir "$STATE"
./shrine describe app api --state-dir "$STATE"
```

Expect VERSION `latest@3f2a9c1b4d7e`, and in `describe`: `Pull policy:  Pinned`, `Pinned:       ghcr.io/me/api@sha256:3f2a… (latest@3f2a9c1b4d7e, 2026-10-07)`, `Running image: unavailable (…)` with exit zero. Remove `pins.txt` and run `describe` again: `Pinned:       -` and VERSION falls back to `ghcr.io/me/api`.

## 3. Start a loopback registry and deploy the pinned fixture

```bash
docker run -d --name quickstart-registry -p 127.0.0.1:5000:5000 registry:2
docker pull traefik/whoami:v1.10.1 && docker tag traefik/whoami:v1.10.1 127.0.0.1:5000/shrine/whoami:latest && docker push 127.0.0.1:5000/shrine/whoami:latest
rm -f "$STATE/shrine-deploy-test/deployments.txt" "$STATE/shrine-deploy-test/pins.txt"
cat > "$SPECS/app.yml" <<'YAML'
apiVersion: shrine/v1
kind: Application
metadata: {name: whoami-pinned, owner: shrine-deploy-test}
spec:
  image: 127.0.0.1:5000/shrine/whoami
  port: 80
  imagePullPolicy: Pinned
YAML
cat > "$SPECS/owned.yml" <<'YAML'
apiVersion: shrine/v1
kind: Application
metadata: {name: whoami-owned, owner: shrine-deploy-test}
spec:
  image: traefik/whoami:v1.10.1
  port: 80
YAML
./shrine deploy --path "$SPECS" --state-dir "$STATE"
```

Expect `📌 Pinned shrine-deploy-test.whoami-pinned at latest@<12hex>` for the pinned app and `🔎 Resolved …` for the other.

## 4. The listing shows the readable form beside a manifest-owned row

```bash
./shrine get deployed --state-dir "$STATE"
./shrine get apps --team shrine-deploy-test --state-dir "$STATE"
```

Expect `whoami-pinned … latest@<12hex>` and `whoami-owned … traefik/whoami:v1.10.1`, the same twelve characters the deploy printed. Stop the daemon briefly (`systemctl stop docker` or equivalent) and repeat: same output, exit zero. Start it again.

## 5. `describe` shows the pin and the running image in agreement

```bash
./shrine describe app whoami-pinned --state-dir "$STATE"
./shrine describe app whoami-owned --state-dir "$STATE"
```

Expect, for the pinned app, `Pinned:` and `Running image:` carrying the same `127.0.0.1:5000/shrine/whoami@sha256:…`; for the owned app no `Pinned:` line and `Running image: docker.io/traefik/whoami:v1.10.1` (the expanded reference).

## 6. A replaced pin shows as a difference, and `status` shows the running image

```bash
docker pull traefik/whoami:v1.10.2 && docker tag traefik/whoami:v1.10.2 127.0.0.1:5000/shrine/whoami:latest && docker push 127.0.0.1:5000/shrine/whoami:latest
NEW=$(docker inspect --format '{{index .RepoDigests 0}}' 127.0.0.1:5000/shrine/whoami:latest | cut -d@ -f2)
OLD=$(grep whoami-pinned "$STATE/shrine-deploy-test/pins.txt" | awk '{print $4}' | cut -d@ -f2)
sed -i "s|$OLD|$NEW|" "$STATE/shrine-deploy-test/pins.txt"
./shrine describe app whoami-pinned --state-dir "$STATE"
./shrine status shrine-deploy-test --state-dir "$STATE"
```

Expect `Pinned:` with the new digest and `Running image:` with the old one. In `status`, an IMAGE column between STATUS and IMAGE ID: `127.0.0.1:5000/shrine/whoami@<old 12hex>` for the pinned row and `docker.io/traefik/whoami:v1.10.1` for the other. (Deploying now would apply the new pin, which is T6's bump flow; not needed here.)

## 7. After teardown nothing shows the pin

```bash
./shrine teardown shrine-deploy-test --state-dir "$STATE"
./shrine get deployed --state-dir "$STATE"
./shrine describe app whoami-pinned --state-dir "$STATE"; echo "exit=$?"
cat "$STATE/shrine-deploy-test/pins.txt"
```

Expect `No deployments found.`, a non-zero `describe` with `not found` and no `Pinned:` output, and `pins.txt` still holding the line.

## Cleanup

```bash
docker rm -f quickstart-registry
docker rmi 127.0.0.1:5000/shrine/whoami:latest || true
rm -rf "$STATE" "$SPECS"
```
