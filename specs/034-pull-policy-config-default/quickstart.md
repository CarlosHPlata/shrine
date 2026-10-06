# Quickstart: A Configuration Default for the Image Pull Policy, and Manifests Generated to Match

**Feature**: 034-pull-policy-config-default | **Date**: 2026-10-06

A manual round-trip over the behaviour this feature adds. Steps 1 to 6 need no Docker daemon. Step 7 needs a daemon and is what the CI integration suite automates; do not run it on a daemon another agent shares.

## 0. Pre-flight

```bash
go build -o shrine .
go test ./...
go vet -tags integration ./tests/integration/...
export STATE=$(mktemp -d) CFG=$(mktemp -d) SPECS=$(mktemp -d)
./shrine apply teams --path tests/testdata/deploy/team --state-dir "$STATE"
```

## 1. No setting: the derived rule, as before

```bash
./shrine deploy --dry-run --config-dir "$CFG" --path tests/testdata/pull-policy-default/versioned --state-dir "$STATE"
```

Expect success and, in the `[DOCKER] ImageResolve:` lines, `policy=Always` for `app-latest`, `policy=IfNotPresent` for `app-fixed` and `res-fixed`, and `policy=IfNotPresent` for `app-own-ifnotpresent`, every one `-> manifest-owned`.

## 2. An invalid value stops every command before it acts

```bash
echo 'imagePullPolicy: pinned' > "$CFG/config.yml"
./shrine get deployed --config-dir "$CFG" --state-dir "$STATE"; echo "exit=$?"
```

Expect a non-zero exit and exactly `Error: loading config: imagePullPolicy: must be one of Always, IfNotPresent, Pinned` on stderr. Nothing else runs.

## 3. The house rule: manifests that name no policy are pinned

```bash
echo 'imagePullPolicy: Pinned' > "$CFG/config.yml"
./shrine deploy --dry-run --config-dir "$CFG" --path tests/testdata/pull-policy-default/pinned-shape --state-dir "$STATE"
```

Expect success; `app-untagged`, `res-noversion`, and `res-own-pinned` all print `policy=Pinned -> would resolve newest and pin`. No `pins.txt` appears under `$STATE` (dry run writes nothing).

## 4. Existing manifests that contradict the house rule fail loudly, naming the setting

```bash
./shrine deploy --dry-run --config-dir "$CFG" --path tests/testdata/pull-policy-default/versioned --state-dir "$STATE"; echo "exit=$?"
```

Expect a non-zero exit and, under `Validation errors:`:

```text
application "app-fixed": spec.image "traefik/whoami:v1.10.2" names a fixed version but the image pull policy is Pinned (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default
resource "res-fixed": spec.version "16" names a fixed version but the image pull policy is Pinned (from config.yml imagePullPolicy); set spec.imagePullPolicy on the manifest or change the default
```

`app-latest` and `app-own-ifnotpresent` are not named. Then confirm the manifest-sourced message is untouched when the manifest sets the field itself:

```bash
./shrine deploy --dry-run --config-dir "$CFG" --path tests/testdata/pull-policy-default/own-pinned-fixed --state-dir "$STATE"
```

Expect `…is Pinned; use "traefik/whoami" or "traefik/whoami:latest"` with no `from config.yml`.

## 5. The other two values as a default

```bash
echo 'imagePullPolicy: IfNotPresent' > "$CFG/config.yml"
./shrine deploy --dry-run --config-dir "$CFG" --path tests/testdata/pull-policy-default/versioned --state-dir "$STATE"
```

Expect `policy=IfNotPresent` for `app-latest` (the derived rule would have said `Always`). Then:

```bash
./shrine deploy --dry-run --config-dir "$CFG" --path tests/testdata/pull-policy-default/pinned-shape --state-dir "$STATE"; echo "exit=$?"
```

Expect failure with `resource "res-noversion": spec.version is required` and nothing about `res-own-pinned`, whose own `Pinned` makes its version optional.

## 6. Generated manifests follow the default

```bash
echo 'imagePullPolicy: Pinned' > "$CFG/config.yml"
./shrine generate application web --config-dir "$CFG" --path "$SPECS" --team shrine-deploy-test
./shrine generate resource db --config-dir "$CFG" --path "$SPECS" --team shrine-deploy-test
grep -n 'image:\|version:\|imagePullPolicy' "$SPECS"/web.yml "$SPECS"/db.yml
```

Expect `image: web` (no tag), no `version:` line in `db.yml`, and no `imagePullPolicy` line in either. Then with the setting removed:

```bash
rm "$CFG/config.yml"; rm "$SPECS"/web.yml "$SPECS"/db.yml
./shrine generate application web --config-dir "$CFG" --path "$SPECS" --team shrine-deploy-test
./shrine generate resource db --config-dir "$CFG" --path "$SPECS" --team shrine-deploy-test
grep -n 'image:\|version:' "$SPECS"/web.yml "$SPECS"/db.yml
./shrine generate resource --help | grep -- --version
```

Expect `image: web:latest`, `version: "16"`, and the help line `Version of the resource (defaults to 16; omitted when imagePullPolicy in config.yml is Pinned)`.

## 7. Generate then deploy under the house rule (daemon required; CI runs this)

```bash
docker run -d --rm --name quickstart-registry -p 127.0.0.1:5000:5000 registry:2
docker pull traefik/whoami:v1.10.1 && docker tag traefik/whoami:v1.10.1 127.0.0.1:5000/shrine/whoami:latest && docker push 127.0.0.1:5000/shrine/whoami:latest
echo 'imagePullPolicy: Pinned' > "$CFG/config.yml"
rm -f "$SPECS"/*.yml
./shrine generate application whoami-gen --config-dir "$CFG" --path "$SPECS" --team shrine-deploy-test --image 127.0.0.1:5000/shrine/whoami --port 80
./shrine generate resource cache-gen --config-dir "$CFG" --path "$SPECS" --team shrine-deploy-test --type 127.0.0.1:5000/shrine/whoami
./shrine deploy --config-dir "$CFG" --path "$SPECS" --state-dir "$STATE"
cat "$STATE/shrine-deploy-test/pins.txt"
```

Expect `📌 Pinned shrine-deploy-test.whoami-gen at latest@…` and `📌 Pinned shrine-deploy-test.cache-gen at latest@…`, and two lines in `pins.txt`. Then change the default and watch the pins go:

```bash
echo 'imagePullPolicy: IfNotPresent' > "$CFG/config.yml"
rm "$SPECS"/cache-gen.yml
./shrine deploy --config-dir "$CFG" --path "$SPECS" --state-dir "$STATE"
cat "$STATE/shrine-deploy-test/pins.txt"
```

Expect the application to resolve as manifest-owned and its line to be gone from `pins.txt` (the resource's line stays until its manifest is deployed again or deleted). Clean up:

```bash
./shrine teardown shrine-deploy-test --state-dir "$STATE"
docker stop quickstart-registry
```
