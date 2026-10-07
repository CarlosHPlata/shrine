# Quickstart: `shrine bump`

**Feature**: 036-bump-command | **Date**: 2026-10-07

A manual round-trip over the behaviour this feature adds. Steps 1 and 2 need no Docker daemon. Steps 3 to 8 need a daemon and a loopback registry, which is what the CI integration suite automates; do not run them on a daemon another agent shares.

## 0. Pre-flight

```bash
go build -o shrine .
go test ./...
go vet -tags integration ./tests/integration/...
export STATE=$(mktemp -d) SPECS=$(mktemp -d)
./shrine apply teams --path tests/testdata/deploy/team --state-dir "$STATE"
```

## 1. Refusals need no registry and no daemon

```bash
cat > "$SPECS/owned.yml" <<'YAML'
apiVersion: shrine/v1
kind: Application
metadata: {name: whoami-owned, owner: shrine-deploy-test}
spec:
  image: traefik/whoami:v1.10.1
  port: 80
YAML
./shrine bump app whoami-owned -v v1.10.2 --path "$SPECS" --state-dir "$STATE"; echo "exit=$?"
./shrine bump app nope -v 1 --path "$SPECS" --state-dir "$STATE"; echo "exit=$?"
./shrine bump app whoami-owned -v 'not valid!' --path "$SPECS" --state-dir "$STATE"; echo "exit=$?"
./shrine bump app whoami-owned --team other -v 1 --path "$SPECS" --state-dir "$STATE"; echo "exit=$?"
ls "$STATE/shrine-deploy-test/"
```

Expect, in order: `application "whoami-owned": its version is manifest-owned (imagePullPolicy IfNotPresent); edit the manifest to change it`; `application "nope": no manifest found in <SPECS>`; `invalid version "not valid!": use a tag (…) or an exact version "sha256:<64 hex>"`; `application "whoami-owned" not found in team "other" (its manifest in <SPECS> is owned by "shrine-deploy-test")`; every exit non-zero; no `pins.txt`.

## 2. Dry run builds nothing

```bash
cat > "$SPECS/app.yml" <<'YAML'
apiVersion: shrine/v1
kind: Application
metadata: {name: whoami-pinned, owner: shrine-deploy-test}
spec:
  image: 127.0.0.1:5000/shrine/whoami
  port: 80
  imagePullPolicy: Pinned
YAML
DOCKER_HOST=tcp://127.0.0.1:1 ./shrine bump app whoami-pinned -v v2 --dry-run --path "$SPECS" --state-dir "$STATE"; echo "exit=$?"
./shrine bump app whoami-pinned --dry-run --path "$SPECS" --state-dir "$STATE"
ls "$STATE/shrine-deploy-test/"
```

Expect `[dry-run] would resolve 127.0.0.1:5000/shrine/whoami:v2 and pin shrine-deploy-test/whoami-pinned` with exit zero even with an unreachable daemon, then `… would resolve 127.0.0.1:5000/shrine/whoami and pin …`, and still no `pins.txt` and no `shrine.log` line for the bump.

## 3. Start a loopback registry with two versions and deploy on the first

```bash
docker run -d --name quickstart-registry -p 127.0.0.1:5000:5000 registry:2
docker pull traefik/whoami:v1.10.1 && docker tag traefik/whoami:v1.10.1 127.0.0.1:5000/shrine/whoami:latest && docker push 127.0.0.1:5000/shrine/whoami:latest
docker pull traefik/whoami:v1.10.2 && docker tag traefik/whoami:v1.10.2 127.0.0.1:5000/shrine/whoami:v2 && docker push 127.0.0.1:5000/shrine/whoami:v2
./shrine deploy --path "$SPECS" --state-dir "$STATE"
OLD=$(docker inspect --format '{{index .RepoDigests 0}}' 127.0.0.1:5000/shrine/whoami:latest | cut -d@ -f2)
NEW=$(docker inspect --format '{{index .RepoDigests 0}}' 127.0.0.1:5000/shrine/whoami:v2 | cut -d@ -f2)
BEFORE=$(docker inspect --format '{{.Id}}' shrine-deploy-test.whoami-pinned)
```

Expect `📌 Pinned shrine-deploy-test.whoami-pinned at latest@<old 12 hex>` and `🔎 Resolved … whoami-owned …`.

## 4. Upgrade: bump records, deploy applies

```bash
./shrine bump app whoami-pinned -v v2 --path "$SPECS" --state-dir "$STATE"
cat "$STATE/shrine-deploy-test/pins.txt"
docker inspect --format '{{.Id}} {{.Config.Image}}' shrine-deploy-test.whoami-pinned
./shrine describe app whoami-pinned --state-dir "$STATE"
```

Expect `🔎 Resolving image for shrine-deploy-test.whoami-pinned (127.0.0.1:5000/shrine/whoami:v2)`, a pull, `  📌 Bumped shrine-deploy-test.whoami-pinned to v2@<new 12 hex>`, and `Bumped shrine-deploy-test/whoami-pinned: latest@<old 12> -> v2@<new 12>; run "shrine deploy" to apply`. The pin line now holds `…/whoami:v2 …/whoami@sha256:$NEW`; the container id equals `$BEFORE` and its image is still `…@sha256:$OLD`; `describe` shows `Pinned:` with `$NEW` and `Running image:` with `$OLD`.

```bash
./shrine deploy --path "$SPECS" --state-dir "$STATE"
docker inspect --format '{{.Id}} {{.Config.Image}}' shrine-deploy-test.whoami-pinned
```

Expect `📌 Using pinned shrine-deploy-test.whoami-pinned v2@<new 12>`, a new container id, and `…@sha256:$NEW`.

## 5. Roll back to the exact version the first bump printed

```bash
./shrine bump app whoami-pinned -v "sha256:$OLD" --path "$SPECS" --state-dir "$STATE"
./shrine deploy --path "$SPECS" --state-dir "$STATE"
docker inspect --format '{{.Config.Image}}' shrine-deploy-test.whoami-pinned
```

Expect `Bumped shrine-deploy-test/whoami-pinned: v2@<new 12> -> <old 12>; run "shrine deploy" to apply` (the twelve hex characters alone for a digest request), then the container back on `…@sha256:$OLD`.

## 6. A version that does not exist is rejected with nothing recorded

```bash
cp "$STATE/shrine-deploy-test/pins.txt" /tmp/pins.before
./shrine bump app whoami-pinned -v v9 --path "$SPECS" --state-dir "$STATE"; echo "exit=$?"
diff /tmp/pins.before "$STATE/shrine-deploy-test/pins.txt" && echo "pins unchanged"
```

Expect `❌ Error [image.pull]: pulling image "127.0.0.1:5000/shrine/whoami:v9": …`, `Error: application "whoami-pinned": pulling image …`, a non-zero exit, and `pins unchanged`.

## 7. Take the newest again

```bash
docker tag traefik/whoami:v1.10.2 127.0.0.1:5000/shrine/whoami:latest && docker push 127.0.0.1:5000/shrine/whoami:latest
./shrine bump app whoami-pinned --path "$SPECS" --state-dir "$STATE"
```

Expect `Bumped shrine-deploy-test/whoami-pinned: <old 12> -> latest@<new 12>; run "shrine deploy" to apply`.

## 8. Bump before the first deploy, and the vanished-version message

```bash
./shrine teardown shrine-deploy-test --state-dir "$STATE"
./shrine delete application whoami-pinned --state-dir "$STATE"
./shrine bump app whoami-pinned -v v2 --path "$SPECS" --state-dir "$STATE"
./shrine deploy --path "$SPECS" --state-dir "$STATE"
docker inspect --format '{{.Config.Image}}' shrine-deploy-test.whoami-pinned
```

Expect `Pinned shrine-deploy-test/whoami-pinned at v2@<new 12>; run "shrine deploy" to apply` (no previous), then `📌 Using pinned … v2@` and the container on `…@sha256:$NEW`.

```bash
./shrine teardown shrine-deploy-test --state-dir "$STATE"
sed -i "s|@sha256:$NEW|@sha256:$(printf '0%.0s' {1..64})|" "$STATE/shrine-deploy-test/pins.txt"
./shrine deploy --path "$SPECS" --state-dir "$STATE"; echo "exit=$?"
```

Expect a non-zero exit with `is no longer served by the registry; run "shrine bump application whoami-pinned" to choose another version` and no container of the team.

## Cleanup

```bash
docker rm -f quickstart-registry
docker rmi 127.0.0.1:5000/shrine/whoami:latest 127.0.0.1:5000/shrine/whoami:v2 || true
rm -rf "$STATE" "$SPECS" /tmp/pins.before
```
