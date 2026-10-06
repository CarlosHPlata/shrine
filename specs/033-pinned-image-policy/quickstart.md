# Quickstart: The Pinned Policy: Resolve Once, Keep the Exact Version

**Feature**: 033-pinned-image-policy | **Date**: 2026-10-06

A manual round-trip over the behaviour this feature adds. Steps 1 and 2 need no Docker daemon. Steps 3 onward need a daemon and are what the CI integration suite automates; do not run them on a daemon another agent shares.

## 0. Pre-flight

```bash
go build -o shrine .
go test ./...
go vet -tags integration ./tests/integration/...
export STATE=$(mktemp -d) SPECS=$(mktemp -d)
./shrine apply teams --path tests/testdata/deploy/team --state-dir "$STATE"
```

## 1. A fixed version under Pinned is rejected before any change

```bash
./shrine deploy --dry-run --path tests/testdata/pinned/fixed-version --state-dir "$STATE"; echo "exit=$?"
```

Expect a non-zero exit and, under `Validation errors:`, one line per artifact naming `spec.image` or `spec.version` and saying it names a fixed version but the image pull policy is `Pinned`, plus one line for the manifest with an unknown policy value. Nothing under `$STATE` changes.

## 2. Dry run previews the pin without writing

```bash
cat > "$SPECS/app.yml" <<'EOF'
apiVersion: shrine/v1
kind: Application
metadata:
  name: whoami-pinned
  owner: shrine-deploy-test
spec:
  image: traefik/whoami
  port: 80
  imagePullPolicy: Pinned
EOF
./shrine deploy --dry-run --path "$SPECS" --state-dir "$STATE"
```

Expect `[DOCKER] ImageResolve: name=shrine-deploy-test.whoami-pinned image=traefik/whoami policy=Pinned -> would resolve newest and pin` before `CreatePlatformNetwork`, and no `pins.txt` under `$STATE/shrine-deploy-test/`.

## 3. First deploy pins; redeploy reuses

```bash
./shrine deploy --path "$SPECS" --state-dir "$STATE"
cat "$STATE/shrine-deploy-test/pins.txt"
docker inspect shrine-deploy-test.whoami-pinned --format '{{.Config.Image}} {{.Id}}'
```

Expect `📌 Pinned shrine-deploy-test.whoami-pinned at latest@<12 hex>`, one `Application whoami-pinned traefik/whoami traefik/whoami@sha256:… <date>` line, and `Config.Image` equal to that digest reference.

```bash
./shrine deploy --path "$SPECS" --state-dir "$STATE"
```

Expect `📌 Using pinned shrine-deploy-test.whoami-pinned latest@<12 hex> (since <date>)`, no `Pulling image` line, and the same container id.

## 4. The pin survives recreation, teardown, and a wiped cache

```bash
sed -i 's/port: 80/port: 80\n  env:\n    - name: ROUND\n      value: "2"/' "$SPECS/app.yml"
./shrine deploy --path "$SPECS" --state-dir "$STATE"          # recreated, same Config.Image
./shrine teardown shrine-deploy-test --state-dir "$STATE"
./shrine deploy --path "$SPECS" --state-dir "$STATE"          # 📌 Using pinned
./shrine teardown shrine-deploy-test --state-dir "$STATE"
docker image rm -f "$(cut -d' ' -f4 "$STATE/shrine-deploy-test/pins.txt")"
./shrine deploy --path "$SPECS" --state-dir "$STATE"          # 📥 Pulling image traefik/whoami@sha256:… then 📌 Using pinned
```

After each deploy, `docker inspect … --format '{{.Config.Image}}'` prints the same digest reference as in step 3.

## 5. Release paths

```bash
./shrine teardown shrine-deploy-test --state-dir "$STATE"
./shrine delete application whoami-pinned --dry-run --state-dir "$STATE"   # [dry-run] would release image pin …; pins.txt unchanged
./shrine delete application whoami-pinned --state-dir "$STATE"             # Released image pin for shrine-deploy-test/whoami-pinned.
./shrine deploy --path "$SPECS" --state-dir "$STATE"                       # 📌 Pinned … again
```

Then switch the manifest to a fixed tag, deploy, and switch back:

```bash
sed -i 's|image: traefik/whoami|image: traefik/whoami:v1.10.2|; /imagePullPolicy/d' "$SPECS/app.yml"
./shrine deploy --path "$SPECS" --state-dir "$STATE"          # 🔎 Resolved …; pins.txt no longer lists whoami-pinned
sed -i 's|image: traefik/whoami:v1.10.2|image: traefik/whoami|; s/port: 80/port: 80\n  imagePullPolicy: Pinned/' "$SPECS/app.yml"
./shrine deploy --path "$SPECS" --state-dir "$STATE"          # 📌 Pinned … (a first deploy again)
```

## 6. A pin the registry no longer serves

```bash
./shrine teardown shrine-deploy-test --state-dir "$STATE"
sed -i 's|@sha256:[0-9a-f]*|@sha256:0000000000000000000000000000000000000000000000000000000000000000|' "$STATE/shrine-deploy-test/pins.txt"
./shrine deploy --path "$SPECS" --state-dir "$STATE"; echo "exit=$?"
docker ps -a --filter name=shrine-deploy-test.
```

Expect a non-zero exit, `is no longer served by the registry` naming `shrine-deploy-test/whoami-pinned` and pointing at deploying under `Always` or `IfNotPresent`, and no container of the team.

## 7. Dry-run stability with a pin on record

Step 6 left a pin the registry cannot serve. Release it first so the next deploy pins afresh:

```bash
./shrine delete application whoami-pinned --state-dir "$STATE"
./shrine deploy --path "$SPECS" --state-dir "$STATE"           # 📌 Pinned … again
before=$(cat "$STATE/shrine-deploy-test/pins.txt" "$STATE/shrine-deploy-test/deployments.txt" | sha256sum)
./shrine deploy --dry-run --path "$SPECS" --state-dir "$STATE"  # -> pinned traefik/whoami@sha256:… (latest, <date>)
./shrine deploy --dry-run --path "$SPECS" --state-dir "$STATE"
after=$(cat "$STATE/shrine-deploy-test/pins.txt" "$STATE/shrine-deploy-test/deployments.txt" | sha256sum)
[ "$before" = "$after" ] && echo byte-identical
```

## 8. Cleanup

```bash
./shrine teardown shrine-deploy-test --state-dir "$STATE"
rm -rf "$STATE" "$SPECS" shrine
```
