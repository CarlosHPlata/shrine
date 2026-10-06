# Quickstart: Resolve Every Image Before Touching Any Container

**Feature**: 032-preflight-image-resolve | **Date**: 2026-10-06

A manual round-trip over the behaviour this feature adds. Steps 1 and 2 need no Docker daemon and can run anywhere; steps 3 to 5 need a daemon and are what the CI integration suite automates. Do not run steps 3 to 5 on a daemon another agent shares.

## 0. Pre-flight

```bash
go build -o shrine .
go test ./...
go vet -tags integration ./tests/integration/...
export STATE=$(mktemp -d)
./shrine apply teams --path tests/testdata/deploy/team --state-dir "$STATE"
```

## 1. Dry run shows the step before any operation

```bash
./shrine deploy --dry-run --path tests/testdata/deploy/resources --state-dir "$STATE"
```

Expect, in this order:

```text
[DOCKER] ImageResolve: name=shrine-deploy-test.test-cache image=traefik/whoami policy=Always -> manifest-owned
[DOCKER] ImageResolve: name=shrine-deploy-test.whoami-res image=traefik/whoami policy=Always -> manifest-owned
[DOCKER] CreatePlatformNetwork name=shrine.platform
```

Run it twice and diff `$STATE`: byte-identical.

## 2. Dry run keeps an alias visible

```bash
./shrine deploy --dry-run --path tests/testdata/deploy/registry-alias --config-dir tests/testdata/deploy/registry-alias --state-dir "$STATE"
```

Expect `image=reg:myregistry/traefik/whoami:latest` on both the `ImageResolve` and the `ContainerCreate` line.

## 3. An unresolvable reference changes nothing

```bash
./shrine deploy --path tests/testdata/deploy/preflight-unresolvable --state-dir "$STATE"; echo "exit=$?"
docker ps -a --filter name=shrine-deploy-test.
docker network ls --filter name=shrine.shrine-deploy-test.private
```

Expect a non-zero exit, the error naming `application "zz-broken"` and `localhost:1/shrine/unresolvable:1.0.0`, no container of the team, and no team network. The healthy artifacts' `🔎 Resolving` and `🔎 Resolved` lines precede the failure.

## 4. Healthy deploy states the version per artifact

```bash
./shrine deploy --path tests/testdata/deploy/resources --state-dir "$STATE"
```

Expect, before `📦 Deploying Resource: test-cache`:

```text
🔎 Resolving image for shrine-deploy-test.test-cache (traefik/whoami)
    ✅ Pulled image traefik/whoami
  🔎 Resolved shrine-deploy-test.test-cache traefik/whoami@<12 hex>
```

and the same pair for `whoami-res`. Both containers run as before.

## 5. Pull semantics unchanged

```bash
./shrine deploy --path tests/testdata/deploy/preflight-fixed-tag --state-dir "$STATE"
./shrine deploy --path tests/testdata/deploy/preflight-fixed-tag --state-dir "$STATE"
```

The second run prints no `Pulling image` line, prints `🔎 Resolved shrine-deploy-test.whoami-fixed traefik/whoami:v1.10.2@<12 hex>`, and `docker inspect shrine-deploy-test.whoami-fixed --format '{{.Id}}'` is unchanged between the runs.

```bash
./shrine deploy --path tests/testdata/deploy/basic --state-dir "$STATE"
./shrine deploy --path tests/testdata/deploy/basic --state-dir "$STATE"
```

Both runs print `✅ Pulled image traefik/whoami`.

## 6. Cleanup

```bash
./shrine teardown shrine-deploy-test --state-dir "$STATE"
rm -rf "$STATE" shrine
```
