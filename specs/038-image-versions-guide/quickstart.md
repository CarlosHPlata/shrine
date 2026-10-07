# Quickstart: verifying the image-versions guide

Run from the repository root. No step needs a container runtime.

## 0. Unit tests and the integration build

```bash
go test ./...
```

```bash
go vet -tags integration ./tests/integration/...
```

Both must pass unchanged. The only code changes are `Long` help strings.

## 1. Regenerate the command pages and check for drift

```bash
make docs-gen-cli
```

```bash
git status --short docs/content/cli
```

Only `get_*.md`, `describe_app.md`, `describe_resource.md`, and `status*.md` may change.

## 2. Full documentation gate

```bash
make docs-check
```

This runs the front-matter lint, the docsgen tests, the CLI drift check, the Hugo build, the companion check, and the shape check. Each must report zero errors.

## 3. Link check against the built site

```bash
grep -c 'id="?a-deploy-stops-because-a-pinned-version-is-no-longer-served' docs/public/troubleshooting/index.html
```

```bash
grep -c 'id="?image-pull-policy' docs/public/reference/manifest-schema/index.html
```

```bash
grep -o 'href="[^"]*image-versions/"' docs/public/guides/index.html docs/public/reference/manifest-schema/index.html docs/public/troubleshooting/index.html
```

```bash
for p in bump bump_application bump_resource delete_resource describe_resource get_deployed status generate_application; do test -f docs/public/cli/$p/index.html && echo "ok $p" || echo "MISSING $p"; done
```

Each anchor count must be at least 1. All three pages must link the guide, and every command page must exist.

## 4. Re-capture the daemon-free blocks

```bash
go build -o /tmp/shrine-038 . && bash specs/038-image-versions-guide/capture.sh /tmp/shrine-038 > /tmp/capture-038.out
```

The script writes its manifests and seeded state into a temporary directory and removes it on exit. It sets `DOCKER_HOST=tcp://127.0.0.1:1`, so no daemon is contacted, prints the manifest directory as `/home/me/shop/manifests`, and drops trailing spaces. Every output line in the guide, the troubleshooting entry, and the manifest reference must then be either in `/tmp/capture-038.out` or one of the assembled lines listed in research R2:

```bash
for f in docs/content/guides/image-versions.md docs/content/troubleshooting/_index.md docs/content/reference/manifest-schema.md; do awk '/^```text/{b=1;next} /^```/{b=0} b' "$f"; done | grep -vE '^(\$ |…$)' | grep -vxFf /tmp/capture-038.out
```

The second command prints only the assembled lines. Each must trace to a format source in research R2.

## 5. Reader check (SC-001)

Using only the guide, the manifest reference, and the command pages, answer the six M7 questions and point to the heading that answers each:

1. How do I pin an artifact? → "Pin a service at its newest version"
2. What happens when I prune images or rebuild the host? → "Rebuild the host"
3. How do I see which version runs? → "See which version runs"
4. How do I upgrade one artifact? → "Upgrade one artifact"
5. How do I roll back? → "Roll back"
6. What does the configuration default change? → "Make pinning the house rule"
