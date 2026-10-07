# Quickstart: `shrine delete resource`

**Feature**: 037-delete-resource | **Date**: 2026-10-07

A manual round-trip over the behaviour this feature adds. Steps 1 and 2 need no Docker daemon. Steps 3 to 5 need a daemon and a loopback registry, which is what `TestDeleteResource` automates in CI; do not run them on a daemon another agent shares.

## 0. Pre-flight

```bash
go build -o shrine .
go test ./...
go vet -tags integration ./tests/integration/...
export STATE=$(mktemp -d)
./shrine apply teams --path tests/testdata/deploy/team --state-dir "$STATE"
```

## 1. The command exists, is documented, and is idempotent on nothing

```bash
./shrine delete --help                         # lists application, resource, team
./shrine delete resource --help                # -t/--team, --dry-run; Long says a live container blocks the delete
./shrine delete resource ghost --state-dir "$STATE"; echo "exit=$?"
# Nothing to delete for resource "ghost".        exit=0
./shrine delete resource ghost --team shrine-deploy-test --state-dir "$STATE"; echo "exit=$?"
# Nothing to delete for resource "ghost" in team "shrine-deploy-test".        exit=0
./shrine delete resource; echo "exit=$?"       # accepts 1 arg(s)   exit=1
```

## 2. Kind-aware state, no daemon needed

Seed an application pin and record by hand and show that `delete resource` does not touch them, while `delete application` still does. `NewQueryContainerBackend` needs a daemon only to inspect; with none, step 2 is best read as the unit tests `TestDeleteResource_IgnoresAnApplicationOfTheSameName` and `TestDeleteApplication_IgnoresAResourceOfTheSameName`:

```bash
go test ./internal/handler/ -run 'TestDeleteResource|TestDeleteApplication' -v
```

## 3. Retire a resource (daemon and loopback registry)

The integration world: a registry serving `shrine/whoami:latest`, a manifest directory with `whoami-pinned` (Application) and `cache-pinned` (Resource) under `Pinned`. Reproduce by hand with any registry you can push to, or read `TestDeleteResource` scenario 1.

```bash
./shrine deploy --path "$SPECS" --state-dir "$STATE"
./shrine teardown shrine-deploy-test --state-dir "$STATE"
cat "$STATE/shrine-deploy-test/pins.txt"                 # two lines: Application whoami-pinned …, Resource cache-pinned …
cat "$STATE/shrine-deploy-test/deployments.txt"          # empty: teardown already dropped both records
./shrine delete resource cache-pinned --dry-run --state-dir "$STATE"
# [dry-run] would release image pin <host>/shrine/whoami@sha256:… for shrine-deploy-test/cache-pinned
cat "$STATE/shrine-deploy-test/pins.txt"                 # unchanged
./shrine delete resource cache-pinned --state-dir "$STATE"
# Released image pin for shrine-deploy-test/cache-pinned.
# (a "Removed deployment record" line appears only when the container was removed outside Shrine, so the record outlived it)
cat "$STATE/shrine-deploy-test/pins.txt"                 # only the Application line remains
```

Push a new `latest`, deploy again, and read `📌 Pinned shrine-deploy-test.cache-pinned at latest@…` beside `📌 Using pinned shrine-deploy-test.whoami-pinned latest@…`.

## 4. A live container blocks the delete

```bash
./shrine deploy --path "$SPECS" --state-dir "$STATE"
./shrine delete resource cache-pinned --state-dir "$STATE"; echo "exit=$?"
# Error: resource "shrine-deploy-test/cache-pinned" still has a container; run "shrine teardown shrine-deploy-test" first    exit=1
./shrine delete resource cache-pinned --dry-run --state-dir "$STATE"; echo "exit=$?"   # same refusal
```

## 5. Every delete verb releases pins

```bash
./shrine teardown shrine-deploy-test --state-dir "$STATE"
./shrine delete application whoami-pinned --state-dir "$STATE"   # Released image pin for shrine-deploy-test/whoami-pinned.
./shrine delete resource cache-pinned --state-dir "$STATE"       # Released image pin for shrine-deploy-test/cache-pinned.
./shrine deploy --path "$SPECS" --state-dir "$STATE"             # both 📌 Pinned afresh
./shrine teardown shrine-deploy-test --state-dir "$STATE"
./shrine delete team shrine-deploy-test --state-dir "$STATE"     # Released 2 image pin(s) for team "shrine-deploy-test".
```

## 6. Docs

```bash
make docs-gen-cli && git status --short docs/content/cli   # delete.md modified, delete_resource.md new
grep -n "four things release a pin" docs/content/reference/manifest-schema.md
grep -n "delete application/resource" AGENTS.md
```
